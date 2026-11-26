package disk

import (
	"context"
	"fmt"
	"math/bits"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

const (
	align   = 4096
	seqBS   = 1 << 20
	randBS  = 4096
	seqQD   = 4
	minFile = 64 << 20
	maxLat  = 1 << 21 // échantillons de latence conservés par mesure
)

// target est un fichier ou un périphérique ouvert en E/S directes.
type target interface {
	ReadAt(p []byte, off int64) (int, error)
	WriteAt(p []byte, off int64) (int, error)
	Close() error
}

// Config paramètre un test disque.
type Config struct {
	// Dir : dossier du fichier de test (mode fichier).
	Dir string
	// Raw : périphérique entier (mode brut, destructif). Prioritaire sur Dir.
	Raw *Device
	// ConfirmSerial doit être égal au numéro de série du disque en mode brut.
	ConfirmSerial string
	// FileSize : taille de la zone testée (0 = automatique).
	FileSize uint64
	// WriteCap : volume d'écriture maximal (préserve l'endurance).
	WriteCap uint64
	// Duration : durée totale ; au-delà de la première passe, le test boucle
	// en vérification (et en écriture tant que le plafond le permet).
	Duration time.Duration
	// RandDuration : durée de chaque mesure aléatoire 4K.
	RandDuration time.Duration
	Progress     func(Progress)
}

// Progress est l'avancement transmis à l'interface.
type Progress struct {
	Phase  string  `json:"phase"`
	MBs    float64 `json:"mbs"`
	Done   uint64  `json:"done"`
	Total  uint64  `json:"total"`
	Errors uint64  `json:"errors"`
}

type runner struct {
	cfg   Config
	open  func() (target, error)
	size  int64
	gen   uint64
	res   *Result
	errMu sync.Mutex
	verr  atomic.Uint64
	ioerr atomic.Uint64
	start time.Time
}

// alignedBuf renvoie un tampon aligné sur 4 Kio.
func alignedBuf(n int) []byte {
	b := make([]byte, n+align)
	off := align - int(uintptr(unsafe.Pointer(&b[0]))&(align-1))
	if off == align {
		off = 0
	}
	return b[off : off+n : off+n]
}

func genKey(gen uint64) uint64 {
	z := gen*0x9E3779B97F4A7C15 + 0x632BE59BD9B4E019
	z = (z ^ z>>30) * 0xBF58476D1CE4E5B9
	z = (z ^ z>>27) * 0x94D049BB133111EB
	return z ^ z>>31
}

// fill écrit le motif d'une zone : chaque mot dépend de son adresse et de la génération.
func fill(b []byte, off int64, gen uint64) {
	k := genKey(gen)
	w := unsafe.Slice((*uint64)(unsafe.Pointer(&b[0])), len(b)/8)
	a := uint64(off)
	for i := range w {
		w[i] = bits.RotateLeft64((a+uint64(i)*8)*0x9E3779B97F4A7C15, 17) ^ k
	}
}

// check vérifie le motif ; renvoie l'indice du premier mot faux (-1 si correct).
func check(b []byte, off int64, gen uint64) (int, uint64, uint64) {
	k := genKey(gen)
	w := unsafe.Slice((*uint64)(unsafe.Pointer(&b[0])), len(b)/8)
	a := uint64(off)
	for i := range w {
		if exp := bits.RotateLeft64((a+uint64(i)*8)*0x9E3779B97F4A7C15, 17) ^ k; w[i] != exp {
			return i, exp, w[i]
		}
	}
	return -1, 0, 0
}

func (r *runner) logErr(format string, a ...any) {
	r.errMu.Lock()
	if len(r.res.ErrorLog) < 100 {
		r.res.ErrorLog = append(r.res.ErrorLog, fmt.Sprintf(format, a...))
	}
	r.errMu.Unlock()
}

func (r *runner) progress(p Progress) {
	if r.cfg.Progress != nil {
		p.Errors = r.verr.Load() + r.ioerr.Load()
		r.cfg.Progress(p)
	}
}

// Run exécute le test disque complet.
func Run(ctx context.Context, cfg Config) (Result, error) {
	res := Result{Mode: "file", WriteCap: cfg.WriteCap}
	r := &runner{cfg: cfg, res: &res, start: time.Now()}
	if cfg.RandDuration <= 0 {
		r.cfg.RandDuration = 5 * time.Second
	}
	if cfg.WriteCap == 0 {
		r.cfg.WriteCap = 50 << 30
		res.WriteCap = r.cfg.WriteCap
	}
	var cleanup func()
	var err error
	if cfg.Raw != nil {
		err = r.prepareRaw()
	} else {
		cleanup, err = r.prepareFile()
	}
	if err != nil {
		return res, err
	}
	if cleanup != nil {
		defer cleanup()
	}
	if res.Device != nil {
		res.HealthBefore = ReadHealth(*res.Device)
	} else {
		res.HealthBefore = unknownHealth()
		res.HealthBefore.Note = "disque physique non identifié"
	}
	deadline := r.start.Add(cfg.Duration)

	// Passe 1 : écriture séquentielle (courbe + cache SLC) puis lecture vérifiée.
	r.gen = 1
	if err := r.seqWrite(ctx, true); err != nil {
		return r.finish(ctx, err)
	}
	if err := r.seqRead(ctx, true); err != nil {
		return r.finish(ctx, err)
	}
	res.Passes = 1
	// Mesures aléatoires 4K.
	for _, m := range []struct {
		qd    int
		write bool
	}{{1, false}, {1, true}, {32, false}, {32, true}} {
		if ctx.Err() != nil {
			break
		}
		if m.write && res.Written+uint64(r.cfg.RandDuration.Seconds()*200e6) > r.cfg.WriteCap {
			continue
		}
		rr, err := r.random(ctx, m.qd, m.write)
		if err != nil {
			return r.finish(ctx, err)
		}
		res.Random = append(res.Random, rr)
	}
	// Vérifie que les écritures aléatoires n'ont rien corrompu.
	if ctx.Err() == nil {
		if err := r.seqRead(ctx, false); err != nil {
			return r.finish(ctx, err)
		}
	}
	// Mode rodage : boucle jusqu'à la fin de la durée.
	for ctx.Err() == nil && time.Now().Before(deadline) {
		if res.Written+uint64(r.size) <= r.cfg.WriteCap {
			r.gen++
			if err := r.seqWrite(ctx, false); err != nil {
				return r.finish(ctx, err)
			}
		}
		if err := r.seqRead(ctx, false); err != nil {
			return r.finish(ctx, err)
		}
		if ctx.Err() == nil {
			res.Passes++
		}
	}
	return r.finish(ctx, nil)
}

func (r *runner) finish(ctx context.Context, err error) (Result, error) {
	res := r.res
	res.VerifyErrors = r.verr.Load()
	res.IOErrors = r.ioerr.Load()
	res.Duration = time.Since(r.start).Round(time.Millisecond)
	if res.Device != nil {
		res.HealthAfter = ReadHealth(*res.Device)
		res.Degraded = Degradation(res.HealthBefore, res.HealthAfter)
	} else {
		res.HealthAfter = res.HealthBefore
	}
	if ctx.Err() != nil && err == nil {
		res.Aborted = ctx.Err().Error()
	}
	if err != nil && err == ctx.Err() {
		res.Aborted = err.Error()
		err = nil
	}
	return *res, err
}

func (r *runner) prepareFile() (func(), error) {
	dir := r.cfg.Dir
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	free, err := freeSpace(dir)
	if err != nil {
		return nil, fmt.Errorf("espace libre de %s : %w", dir, err)
	}
	size := r.cfg.FileSize
	if size == 0 {
		size = min(r.cfg.WriteCap/2, 16<<30)
	}
	// Ne jamais remplir le volume : on laisse au moins 10 % et 2 Gio libres.
	if lim := int64(free) - int64(free/10) - 2<<30; lim < int64(size) {
		if lim < minFile {
			return nil, fmt.Errorf("espace libre insuffisant sur %s (%d Mio)", dir, free>>20)
		}
		size = uint64(lim)
	}
	size = max(size&^(seqBS-1), minFile)
	path := filepath.Join(dir, fmt.Sprintf("rodage-test-%d.bin", os.Getpid()))
	t, direct, err := openFile(path, true)
	if err != nil {
		return nil, err
	}
	t.Close()
	r.res.Path = path
	r.res.Direct = direct
	if !direct {
		r.res.Note = "E/S directes non prises en charge par ce système de fichiers : le cache peut fausser les mesures"
	}
	r.size = int64(size)
	r.res.FileSize = size
	r.open = func() (target, error) {
		t, _, err := openFile(path, false)
		return t, err
	}
	if d, err := DeviceForPath(dir); err == nil {
		r.res.Device = &d
	}
	return func() { os.Remove(path) }, nil
}

func (r *runner) prepareRaw() error {
	d := *r.cfg.Raw
	if err := CheckRawSafe(d, r.cfg.ConfirmSerial); err != nil {
		return err
	}
	r.res.Mode = "raw"
	r.res.Path = d.Path
	r.res.Device = &d
	r.res.Direct = true
	size := d.Size
	if r.cfg.FileSize > 0 && r.cfg.FileSize < size {
		size = r.cfg.FileSize
	}
	size = min(size, r.cfg.WriteCap)
	r.size = int64(size &^ (seqBS - 1))
	r.res.FileSize = uint64(r.size)
	r.open = func() (target, error) { return openRaw(d) }
	t, err := r.open()
	if err != nil {
		return err
	}
	t.Close()
	return nil
}

// CheckRawSafe applique les garde-fous du mode brut.
func CheckRawSafe(d Device, confirmSerial string) error {
	switch {
	case d.System:
		return fmt.Errorf("%w : %s est le disque système", ErrUnsafe, d.Name)
	case d.Partitions > 0:
		return fmt.Errorf("%w : %s contient %d partition(s)", ErrUnsafe, d.Name, d.Partitions)
	case d.Mounted:
		return fmt.Errorf("%w : %s est monté ou utilisé", ErrUnsafe, d.Name)
	case d.Serial == "":
		return fmt.Errorf("%w : numéro de série de %s inconnu, confirmation impossible", ErrUnsafe, d.Name)
	case confirmSerial != d.Serial:
		return fmt.Errorf("%w : le numéro de série saisi ne correspond pas à %s", ErrUnsafe, d.Name)
	}
	return nil
}

// seqWrite écrit toute la zone en blocs de 1 Mio (QD4).
func (r *runner) seqWrite(ctx context.Context, curve bool) error {
	return r.seq(ctx, true, curve)
}

func (r *runner) seqRead(ctx context.Context, measure bool) error {
	return r.seq(ctx, false, measure)
}

func (r *runner) seq(ctx context.Context, write, measure bool) error {
	blocks := r.size / seqBS
	var next, doneBytes atomic.Int64
	ts := make([]target, seqQD)
	for i := range ts {
		t, err := r.open()
		if err != nil {
			for _, o := range ts[:i] {
				o.Close()
			}
			return err
		}
		ts[i] = t
	}
	defer func() {
		for _, t := range ts {
			t.Close()
		}
	}()
	phase := "seq-read"
	if write {
		phase = "seq-write"
	}
	start := time.Now()
	stop := make(chan struct{})
	var curve []float64
	var tickWG sync.WaitGroup
	tickWG.Add(1)
	go func() {
		defer tickWG.Done()
		tk := time.NewTicker(time.Second)
		defer tk.Stop()
		var last int64
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
				d := doneBytes.Load()
				mbs := float64(d-last) / 1e6
				last = d
				if write && measure {
					curve = append(curve, mbs)
				}
				r.progress(Progress{Phase: phase, MBs: mbs, Done: uint64(d), Total: uint64(r.size)})
			}
		}
	}()
	var wg sync.WaitGroup
	var firstErr atomic.Value
	for _, t := range ts {
		wg.Add(1)
		go func(t target) {
			defer wg.Done()
			buf := alignedBuf(seqBS)
			for ctx.Err() == nil {
				b := next.Add(1) - 1
				if b >= blocks {
					return
				}
				off := b * seqBS
				if write {
					fill(buf, off, r.gen)
					if _, err := t.WriteAt(buf, off); err != nil {
						r.ioerr.Add(1)
						r.logErr("écriture à %d : %v", off, err)
						if r.ioerr.Load() > 50 {
							firstErr.CompareAndSwap(nil, err)
							return
						}
						continue
					}
				} else {
					if _, err := t.ReadAt(buf, off); err != nil {
						r.ioerr.Add(1)
						r.logErr("lecture à %d : %v", off, err)
						if r.ioerr.Load() > 50 {
							firstErr.CompareAndSwap(nil, err)
							return
						}
						continue
					}
					if i, exp, got := check(buf, off, r.gen); i >= 0 {
						r.verr.Add(1)
						r.logErr("donnée fausse à l'octet %d : %#x au lieu de %#x", off+int64(i)*8, got, exp)
					}
				}
				doneBytes.Add(seqBS)
			}
		}(t)
	}
	wg.Wait()
	close(stop)
	tickWG.Wait()
	el := time.Since(start).Seconds()
	d := uint64(doneBytes.Load())
	if write {
		r.res.Written += d
	} else {
		r.res.ReadBytes += d
	}
	if measure && el > 0 && ctx.Err() == nil {
		mbs := float64(d) / el / 1e6
		if write {
			r.res.SeqWriteMBs = mbs
			r.res.WriteCurve = curve
			r.res.SLC = detectSLC(curve)
		} else {
			r.res.SeqReadMBs = mbs
		}
	}
	if e, ok := firstErr.Load().(error); ok {
		return fmt.Errorf("trop d'erreurs d'E/S : %w", e)
	}
	return ctx.Err()
}

// detectSLC repère une chute durable du débit d'écriture.
func detectSLC(curve []float64) SLC {
	var s SLC
	if len(curve) < 8 {
		return s
	}
	med := func(v []float64) float64 {
		c := append([]float64(nil), v...)
		sort.Float64s(c)
		return c[len(c)/2]
	}
	s.BeforeMBs = med(curve[1:4])
	tail := curve[len(curve)*3/4:]
	s.AfterMBs = med(tail)
	if s.AfterMBs < 0.6*s.BeforeMBs {
		s.Detected = true
		// Premier instant où 3 secondes consécutives passent sous le seuil.
		thr := (s.BeforeMBs + s.AfterMBs) / 2
		var acc float64
		for i := 0; i < len(curve); i++ {
			if i+2 < len(curve) && curve[i] < thr && curve[i+1] < thr && curve[i+2] < thr {
				s.AfterGB = acc / 1000
				break
			}
			acc += curve[i]
		}
	}
	return s
}

// random mesure des E/S aléatoires 4K à la profondeur de file qd.
func (r *runner) random(ctx context.Context, qd int, write bool) (RandResult, error) {
	name := fmt.Sprintf("read-4k-qd%d", qd)
	if write {
		name = fmt.Sprintf("write-4k-qd%d", qd)
	}
	ts := make([]target, qd)
	for i := range ts {
		t, err := r.open()
		if err != nil {
			for _, o := range ts[:i] {
				o.Close()
			}
			return RandResult{}, err
		}
		ts[i] = t
	}
	defer func() {
		for _, t := range ts {
			t.Close()
		}
	}()
	ctx2, cancel := context.WithTimeout(ctx, r.cfg.RandDuration)
	defer cancel()
	var ops atomic.Int64
	lats := make([][]time.Duration, qd)
	var wg sync.WaitGroup
	start := time.Now()
	nblk := r.size / randBS
	stop := make(chan struct{})
	go func() {
		tk := time.NewTicker(time.Second)
		defer tk.Stop()
		var last int64
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
				o := ops.Load()
				r.progress(Progress{Phase: name, MBs: float64(o-last) * randBS / 1e6, Done: uint64(time.Since(start)), Total: uint64(r.cfg.RandDuration)})
				last = o
			}
		}
	}()
	for w := 0; w < qd; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(w)+1, uint64(time.Now().UnixNano())))
			buf := alignedBuf(randBS)
			t := ts[w]
			local := make([]time.Duration, 0, 1<<14)
			for ctx2.Err() == nil {
				off := rng.Int64N(nblk) * randBS
				t0 := time.Now()
				var err error
				if write {
					fill(buf, off, r.gen)
					_, err = t.WriteAt(buf, off)
				} else {
					_, err = t.ReadAt(buf, off)
				}
				d := time.Since(t0)
				if err != nil {
					r.ioerr.Add(1)
					r.logErr("%s à %d : %v", name, off, err)
					continue
				}
				if !write {
					if i, exp, got := check(buf, off, r.gen); i >= 0 {
						r.verr.Add(1)
						r.logErr("donnée fausse à l'octet %d : %#x au lieu de %#x", off+int64(i)*8, got, exp)
					}
				}
				if len(local) < maxLat/qd {
					local = append(local, d)
				}
				ops.Add(1)
			}
			lats[w] = local
		}(w)
	}
	wg.Wait()
	close(stop)
	el := time.Since(start).Seconds()
	n := ops.Load()
	if write {
		r.res.Written += uint64(n) * randBS
	} else {
		r.res.ReadBytes += uint64(n) * randBS
	}
	var all []time.Duration
	for _, l := range lats {
		all = append(all, l...)
	}
	rr := RandResult{Name: name, QD: qd, Write: write}
	if el > 0 {
		rr.IOPS = float64(n) / el
		rr.MBs = rr.IOPS * randBS / 1e6
	}
	if len(all) > 0 {
		sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
		var sum time.Duration
		for _, d := range all {
			sum += d
		}
		us := func(d time.Duration) float64 { return float64(d) / 1e3 }
		rr.AvgUs = us(sum / time.Duration(len(all)))
		rr.P50Us = us(all[len(all)/2])
		rr.P99Us = us(all[min(len(all)-1, len(all)*99/100)])
		rr.MaxUs = us(all[len(all)-1])
	}
	if ctx.Err() != nil {
		return rr, ctx.Err()
	}
	return rr, nil
}
