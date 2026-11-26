// Package memtest teste la mémoire vive allouable.
//
// Le test porte sur des adresses virtuelles et sur la mémoire que le système
// accepte d'allouer : il n'est pas exhaustif (voir le rapport).
package memtest

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// DefaultFraction est la part de la RAM libre testée par défaut (85-90 %).
const DefaultFraction = 0.87

// ErrorEvent est une cellule fausse.
type ErrorEvent struct {
	Test     string `json:"test"`
	Offset   uint64 `json:"offset"`
	Expected uint64 `json:"expected"`
	Actual   uint64 `json:"actual"`
	Pass     int    `json:"pass"`
}

// ECC rassemble les compteurs d'erreurs corrigées/non corrigées.
type ECC struct {
	Available   bool   `json:"available"`
	Source      string `json:"source,omitempty"`
	Corrected   uint64 `json:"corrected"`
	Uncorrected uint64 `json:"uncorrected"`
}

// Result est le bilan du test mémoire.
type Result struct {
	Bytes       uint64        `json:"bytes"`
	Locked      bool          `json:"locked"`
	LockNote    string        `json:"lock_note,omitempty"`
	Passes      int           `json:"passes"`
	Tests       []string      `json:"tests"`
	Duration    time.Duration `json:"duration"`
	BytesRW     uint64        `json:"bytes_rw"`
	TotalErrors uint64        `json:"total_errors"`
	Errors      []ErrorEvent  `json:"errors,omitempty"`
	ECC         ECC           `json:"ecc"`
	Aborted     string        `json:"aborted,omitempty"`
}

// Config paramètre le test.
type Config struct {
	// Bytes à tester ; 0 = DefaultFraction de la RAM libre.
	Bytes    uint64
	Duration time.Duration
	Threads  int
	Progress func(p Progress)
}

// Progress est l'avancement transmis à l'interface.
type Progress struct {
	Pass    int
	Test    string
	BytesRW uint64
	Errors  uint64
}

// Test est un test mémoire en cours.
type Test struct {
	cfg     Config
	mem     *region
	words   []uint64
	rw      atomic.Uint64
	errs    atomic.Uint64
	mu      sync.Mutex
	events  []ErrorEvent
	pass    int
	current atomic.Value
}

// New alloue et verrouille la mémoire à tester.
func New(cfg Config, available uint64) (*Test, error) {
	if cfg.Bytes == 0 {
		cfg.Bytes = uint64(float64(available) * DefaultFraction)
	}
	cfg.Bytes &^= 1<<20 - 1 // multiple de 1 Mio
	if cfg.Bytes < 1<<20 {
		return nil, fmt.Errorf("mémoire disponible insuffisante")
	}
	if cfg.Threads <= 0 {
		cfg.Threads = runtime.NumCPU()
	}
	r, err := allocate(cfg.Bytes)
	if err != nil {
		return nil, err
	}
	t := &Test{cfg: cfg, mem: r}
	t.words = unsafe.Slice((*uint64)(unsafe.Pointer(&r.buf[0])), len(r.buf)/8)
	t.current.Store("")
	return t, nil
}

// Close libère la mémoire.
func (t *Test) Close() { t.mem.free() }

// Words expose la zone testée (pour les tests unitaires).
func (t *Test) Words() []uint64 { return t.words }

func (t *Test) record(test string, idx int, exp, act uint64) {
	t.errs.Add(1)
	t.mu.Lock()
	if len(t.events) < 200 {
		t.events = append(t.events, ErrorEvent{Test: test, Offset: uint64(idx) * 8, Expected: exp, Actual: act, Pass: t.pass})
	}
	t.mu.Unlock()
}

// parallel découpe la zone en tranches et appelle f sur chacune.
func (t *Test) parallel(ctx context.Context, f func(lo, hi int)) {
	n := len(t.words)
	parts := t.cfg.Threads
	var wg sync.WaitGroup
	for p := 0; p < parts; p++ {
		lo, hi := n*p/parts, n*(p+1)/parts
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			// Tranches de 1 Mio pour rester réactif à l'annulation.
			const step = 1 << 17
			for a := lo; a < hi; a += step {
				if ctx.Err() != nil {
					return
				}
				b := min(a+step, hi)
				f(a, b)
				t.rw.Add(uint64(b-a) * 8)
			}
		}(lo, hi)
	}
	wg.Wait()
}

// parallelDown parcourt chaque tranche en ordre décroissant.
func (t *Test) parallelDown(ctx context.Context, f func(lo, hi int)) {
	n := len(t.words)
	parts := t.cfg.Threads
	var wg sync.WaitGroup
	for p := 0; p < parts; p++ {
		lo, hi := n*p/parts, n*(p+1)/parts
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			const step = 1 << 17
			for b := hi; b > lo; b -= step {
				if ctx.Err() != nil {
					return
				}
				a := max(b-step, lo)
				f(a, b)
				t.rw.Add(uint64(b-a) * 8)
			}
		}(lo, hi)
	}
	wg.Wait()
}

type testFn struct {
	name string
	run  func(ctx context.Context, pass int)
}

func (t *Test) tests() []testFn {
	return []testFn{
		{"address", t.addressInCell},
		{"walking-ones", func(ctx context.Context, p int) { t.walking(ctx, p, false) }},
		{"walking-zeros", func(ctx context.Context, p int) { t.walking(ctx, p, true) }},
		{"moving-inversions", t.movingInversions},
		{"random-inversions", t.randomInversions},
	}
}

// Run exécute des passes jusqu'à la fin de la durée (au moins une passe
// commencée) ou l'annulation de ctx.
func (t *Test) Run(ctx context.Context) Result {
	start := time.Now()
	ecc0 := readECC()
	ctx, cancel := context.WithTimeout(ctx, t.cfg.Duration)
	defer cancel()
	res := Result{Bytes: uint64(len(t.words)) * 8, Locked: t.mem.locked, LockNote: t.mem.note}
	tests := t.tests()
	for _, f := range tests {
		res.Tests = append(res.Tests, f.name)
	}
	stopTicker := make(chan struct{})
	go func() {
		tk := time.NewTicker(500 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-stopTicker:
				return
			case <-tk.C:
				if t.cfg.Progress != nil {
					t.cfg.Progress(Progress{Pass: t.pass + 1, Test: t.current.Load().(string), BytesRW: t.rw.Load(), Errors: t.errs.Load()})
				}
			}
		}
	}()
	completed := 0
	for p := 0; ctx.Err() == nil; p++ {
		t.pass = p + 1
		for _, f := range tests {
			if ctx.Err() != nil {
				break
			}
			t.current.Store(f.name)
			f.run(ctx, p)
		}
		if ctx.Err() == nil {
			completed++
		}
	}
	close(stopTicker)
	res.Passes = completed
	res.Duration = time.Since(start).Round(time.Millisecond)
	res.BytesRW = t.rw.Load()
	res.TotalErrors = t.errs.Load()
	t.mu.Lock()
	res.Errors = append(res.Errors, t.events...)
	t.mu.Unlock()
	ecc1 := readECC()
	res.ECC = ecc1
	if ecc1.Available && ecc0.Available {
		res.ECC.Corrected = ecc1.Corrected - ecc0.Corrected
		res.ECC.Uncorrected = ecc1.Uncorrected - ecc0.Uncorrected
	}
	if context.Cause(ctx) != context.DeadlineExceeded && ctx.Err() != nil && ctx.Err() != context.DeadlineExceeded {
		res.Aborted = ctx.Err().Error()
	}
	return res
}

// addressInCell : chaque mot contient sa propre adresse, puis son complément.
func (t *Test) addressInCell(ctx context.Context, pass int) {
	w := t.words
	for _, inv := range []uint64{0, ^uint64(0)} {
		t.parallel(ctx, func(lo, hi int) {
			for i := lo; i < hi; i++ {
				w[i] = uint64(i)*8 ^ inv
			}
		})
		t.parallel(ctx, func(lo, hi int) {
			for i := lo; i < hi; i++ {
				if exp := uint64(i)*8 ^ inv; w[i] != exp {
					t.record("address", i, exp, w[i])
				}
			}
		})
	}
}

// walking : un bit à 1 (ou à 0) qui se décale d'un mot à l'autre ;
// chaque passe utilise un décalage initial différent.
func (t *Test) walking(ctx context.Context, pass int, zeros bool) {
	w := t.words
	name := "walking-ones"
	var inv uint64
	if zeros {
		name, inv = "walking-zeros", ^uint64(0)
	}
	for k := 0; k < 4; k++ {
		shift := (pass*4 + k) % 64
		pat := func(i int) uint64 { return (1 << uint((i+shift)&63)) ^ inv }
		t.parallel(ctx, func(lo, hi int) {
			for i := lo; i < hi; i++ {
				w[i] = pat(i)
			}
		})
		t.parallel(ctx, func(lo, hi int) {
			for i := lo; i < hi; i++ {
				if exp := pat(i); w[i] != exp {
					t.record(name, i, exp, w[i])
				}
			}
		})
	}
}

// movingInversions : remplissage p, montée (vérifie p, écrit ~p),
// descente (vérifie ~p, écrit p), vérification finale.
func (t *Test) movingInversions(ctx context.Context, pass int) {
	pats := []uint64{0, ^uint64(0), 0x5555555555555555, 0xAAAAAAAAAAAAAAAA, 0x0F0F0F0F0F0F0F0F, 0x00FF00FF00FF00FF}
	p := pats[pass%len(pats)]
	t.inversions(ctx, "moving-inversions", func(int) uint64 { return p })
	q := pats[(pass+1)%len(pats)]
	t.inversions(ctx, "moving-inversions", func(int) uint64 { return q })
}

// randomInversions : comme moving inversions, avec un motif pseudo-aléatoire
// propre à chaque mot (graine différente à chaque passe).
func (t *Test) randomInversions(ctx context.Context, pass int) {
	seed := uint64(pass+1) * 0x9E3779B97F4A7C15
	t.inversions(ctx, "random-inversions", func(i int) uint64 {
		x := uint64(i)*0xBF58476D1CE4E5B9 ^ seed
		x ^= x >> 31
		x *= 0x94D049BB133111EB
		return x ^ x>>29
	})
}

func (t *Test) inversions(ctx context.Context, name string, pat func(int) uint64) {
	w := t.words
	t.parallel(ctx, func(lo, hi int) {
		for i := lo; i < hi; i++ {
			w[i] = pat(i)
		}
	})
	t.parallel(ctx, func(lo, hi int) {
		for i := lo; i < hi; i++ {
			p := pat(i)
			if w[i] != p {
				t.record(name, i, p, w[i])
			}
			w[i] = ^p
		}
	})
	t.parallelDown(ctx, func(lo, hi int) {
		for i := hi - 1; i >= lo; i-- {
			p := pat(i)
			if w[i] != ^p {
				t.record(name, i, ^p, w[i])
			}
			w[i] = p
		}
	})
	t.parallel(ctx, func(lo, hi int) {
		for i := lo; i < hi; i++ {
			if p := pat(i); w[i] != p {
				t.record(name, i, p, w[i])
			}
		}
	})
}
