package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/maxb1906/rodage/internal/cpu"
	"github.com/maxb1906/rodage/internal/cpu/kernels"
	"github.com/maxb1906/rodage/internal/disk"
	"github.com/maxb1906/rodage/internal/i18n"
	"github.com/maxb1906/rodage/internal/memtest"
	"github.com/maxb1906/rodage/internal/netbench"
	"github.com/maxb1906/rodage/internal/refdb"
	"github.com/maxb1906/rodage/internal/report"
	"github.com/maxb1906/rodage/internal/safety"
	"github.com/maxb1906/rodage/internal/sensors"
	"github.com/maxb1906/rodage/internal/sysinfo"
)

// Version est la version de Rodage (renseignée à la compilation).
var Version = "1.0.0"

// Event est un message émis pendant un test (pour la CLI et la GUI).
type Event struct {
	Kind     string          `json:"kind"` // start, phase, sample, progress, log, done
	Phase    string          `json:"phase,omitempty"`
	Message  string          `json:"message,omitempty"`
	Fraction float64         `json:"fraction"` // avancement global 0..1
	Detail   string          `json:"detail,omitempty"`
	Value    float64         `json:"value,omitempty"` // débit courant
	Unit     string          `json:"unit,omitempty"`
	Errors   uint64          `json:"errors,omitempty"`
	Sample   *sensors.Sample `json:"sample,omitempty"`
	Planned  time.Duration   `json:"planned,omitempty"`
}

// ErrUserStop est la cause d'un arrêt demandé par l'utilisateur.
var ErrUserStop = errors.New("arrêt demandé par l'utilisateur")

type run struct {
	plan    Plan
	emit    func(Event)
	rep     *report.Report
	start   time.Time
	planned time.Duration
	sampler *sensors.Sampler
	mon     *safety.Monitor
	ctx     context.Context
	phases  []string
	idx     atomic.Int32
	emitMu  sync.Mutex
	frac    atomic.Uint64 // avancement déjà annoncé (bits float64)
}

func newID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b[:3])
}

// fraction renvoie l'avancement global ; il ne recule jamais, quel que soit
// l'événement qui le demande (relevé, journal ou progression d'une phase).
func (r *run) fraction(local float64) float64 {
	var f float64
	if r.planned > 0 {
		f = float64(time.Since(r.start)) / float64(r.planned)
	} else if n := float64(len(r.phases)); n > 0 {
		f = (float64(r.idx.Load()) + min(max(local, 0), 1)) / n
	}
	f = min(f, 0.999)
	for {
		old := r.frac.Load()
		if f <= math.Float64frombits(old) {
			return math.Float64frombits(old)
		}
		if r.frac.CompareAndSwap(old, math.Float64bits(f)) {
			return f
		}
	}
}

// send transmet un événement ; les appels sont sérialisés pour que le
// consommateur (CLI, GUI) n'ait pas à être sûr en concurrence.
func (r *run) send(e Event) {
	if r.emit != nil {
		r.emitMu.Lock()
		r.emit(e)
		r.emitMu.Unlock()
	}
}

func (r *run) progress(phase string, local float64, detail string, value float64, unit string, errs uint64) {
	r.send(Event{Kind: "progress", Phase: phase, Fraction: r.fraction(local), Detail: detail, Value: value, Unit: unit, Errors: errs})
}

func (r *run) logf(format string, a ...any) {
	r.send(Event{Kind: "log", Message: fmt.Sprintf(format, a...), Fraction: r.fraction(0)})
}

// beat suit la dernière progression d'un test pour le watchdog.
type beat struct{ t atomic.Int64 }

func (b *beat) touch()          { b.t.Store(time.Now().UnixNano()) }
func (b *beat) last() time.Time { return time.Unix(0, b.t.Load()) }

// Run exécute un plan et renvoie le rapport.
func Run(parent context.Context, plan Plan, emit func(Event)) (*report.Report, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	lang := i18n.Parse(plan.Lang)
	if plan.Lang == "" {
		lang = i18n.Detect()
	}
	r := &run{plan: plan, emit: emit, start: time.Now()}
	if plan.Mode == ModeStress {
		r.planned = plan.Total()
	}
	// Liste fixée avant tout fil concurrent (échantillonneur, sûreté).
	for _, c := range []struct {
		name string
		on   bool
	}{{"cpu", plan.CPU}, {"ram", plan.RAM}, {"disk", plan.Disk}, {"net", plan.Net}} {
		if c.on {
			r.phases = append(r.phases, c.name)
		}
	}
	rep := &report.Report{Tool: "rodage", Version: Version, ID: newID(), Created: time.Now(), Lang: string(lang),
		Mode: plan.Mode, Profile: plan.Profile, Planned: r.planned}
	r.rep = rep
	if m, ok := safety.PreviousCrash(); ok {
		rep.PrevCrash = &m
	}
	rep.System = sysinfo.Collect()

	src := sensors.Open()
	defer src.Close()
	rep.Sensors = sensors.Describe(src)
	r.sampler = sensors.NewSampler(src, time.Second)
	r.sampler.Subscribe(func(s sensors.Sample) {
		s2 := s
		r.send(Event{Kind: "sample", Sample: &s2, Fraction: r.fraction(0)})
	})

	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	r.ctx = ctx
	r.mon = safety.Start(safety.Config{
		TjMax:     src.TjMax(),
		Margin:    plan.ThermalMargin,
		RequireAC: rep.System.HasBattery && !plan.AllowBattery && plan.Mode == ModeStress,
	}, r.sampler, cancel)
	rep.ThermalStop = r.mon.Limit()

	budget := plan.Budget()
	r.send(Event{Kind: "start", Planned: r.planned, Message: strings.Join(r.phases, ",")})
	safety.WriteMarker(safety.Marker{Started: r.start, Mode: plan.Mode})

	for i, ph := range r.phases {
		if ctx.Err() != nil {
			break
		}
		r.idx.Store(int32(i))
		safety.WriteMarker(safety.Marker{Started: r.start, Mode: plan.Mode, Phase: ph})
		r.send(Event{Kind: "phase", Phase: ph, Fraction: r.fraction(0)})
		t0 := time.Now()
		var err error
		switch ph {
		case "cpu":
			err = r.runCPU(budget["cpu"])
		case "ram":
			err = r.runRAM(budget["ram"])
		case "disk":
			err = r.runDisk(budget["disk"])
		case "net":
			err = r.runNet(budget["net"])
		}
		rep.Phases = append(rep.Phases, report.Phase{Name: ph, Start: t0, End: time.Now()})
		if err != nil && ctx.Err() == nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("%s : %v", ph, err))
			r.logf("%s : %v", ph, err)
		}
	}

	// Arrêt utilisateur (et non arrêt d'urgence).
	if ctx.Err() != nil {
		if _, ok := safety.IsAbort(context.Cause(ctx)); !ok {
			rep.Safety = append(rep.Safety, safety.Event{Time: time.Now(), Kind: safety.KindUser, Abort: true})
		}
	}
	r.sampler.Stop()
	rep.Safety = append(r.mon.Stop(), rep.Safety...)
	rep.Safety = dedupUser(rep.Safety)
	safety.ClearMarker()

	all := r.sampler.All()
	for _, s := range all {
		if s.CPUTempC == s.CPUTempC && s.CPUTempC > rep.MaxTempC {
			rep.MaxTempC = s.CPUTempC
		}
	}
	rep.Samples = report.Downsample(all, 1500)
	rep.Elapsed = time.Since(r.start).Round(time.Second)

	if plan.Mode == ModeBench && ctx.Err() == nil {
		if db, err := refdb.Open(refdb.DefaultPath()); err == nil {
			rep.References = db.Compare(rep)
			db.Add(rep)
			if err := db.Save(); err != nil {
				r.logf("références : %v", err)
			}
		}
	}
	rep.Evaluate()
	r.send(Event{Kind: "done", Fraction: 1, Message: string(rep.Verdict)})
	return rep, nil
}

func dedupUser(ev []safety.Event) []safety.Event {
	var out []safety.Event
	seen := false
	for _, e := range ev {
		if e.Kind == safety.KindUser {
			if seen || !e.Abort {
				continue
			}
			seen = true
		}
		out = append(out, e)
	}
	return out
}

func (r *run) kernel() kernels.Kernel {
	if r.plan.Kernel != "" {
		if k, ok := kernels.ByLevel(kernels.Level(r.plan.Kernel)); ok {
			return k
		}
		r.logf("noyau %s indisponible, choix automatique", r.plan.Kernel)
	}
	return kernels.Best()
}

func (r *run) runCPU(d time.Duration) error {
	k := r.kernel()
	if r.plan.Mode == ModeBench {
		res, err := cpu.Bench(r.ctx, cpu.BenchConfig{PerTest: 3 * time.Second, Kernel: k, Progress: func(name string, done, total int) {
			r.progress("cpu", float64(done)/float64(total), name, 0, "", 0)
		}})
		r.rep.CPU = &report.CPUSection{Bench: &res}
		return err
	}
	s, err := cpu.NewStress(cpu.StressConfig{Duration: d, Kernel: k})
	if err != nil {
		return err
	}
	for c := range s.Heartbeats() {
		c := c
		r.mon.Watch(fmt.Sprintf("cpu#%d", c), func() time.Time { return s.Heartbeats()[c] })
	}
	stop := make(chan struct{})
	go func() {
		tk := time.NewTicker(time.Second)
		defer tk.Stop()
		var lastIt uint64
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
				it, errs := s.Progress()
				r.progress("cpu", 0, fmt.Sprintf("%s · %d threads", k.Level, runtime.NumCPU()), float64(it-lastIt), "it/s", errs)
				lastIt = it
			}
		}
	}()
	t0 := time.Now()
	res := s.Run(r.ctx)
	close(stop)
	for c := range s.Heartbeats() {
		r.mon.Unwatch(fmt.Sprintf("cpu#%d", c))
	}
	// Cœurs figés : aucune progression pendant les 30 dernières secondes.
	if ev, ok := safety.IsAbort(context.Cause(r.ctx)); ok && ev.Kind == safety.KindWatchdog {
		for c, last := range s.Heartbeats() {
			if time.Since(last) > 30*time.Second {
				res.Stalled = append(res.Stalled, c)
			}
		}
	}
	tr := sensors.AnalyzeThrottle(r.sampler.Between(t0.Add(3*time.Second), time.Now()), r.rep.Sensors.TjMax, r.rep.Sensors.Level)
	r.rep.CPU = &report.CPUSection{Stress: &res, Throttle: &tr}
	return nil
}

func (r *run) runRAM(d time.Duration) error {
	avail := sysinfo.MemAvailable()
	bytes := r.plan.RAMBytes
	dur := d
	if r.plan.Mode == ModeBench {
		if bytes == 0 {
			bytes = min(GiB, avail/2)
		}
		dur = 20 * time.Second
	}
	var b beat
	b.touch()
	var lastRW uint64
	start := time.Now()
	t, err := memtest.New(memtest.Config{Bytes: bytes, Duration: dur, Progress: func(p memtest.Progress) {
		if p.BytesRW != lastRW {
			b.touch()
		}
		el := time.Since(start).Seconds()
		gbs := 0.0
		if el > 0 {
			gbs = float64(p.BytesRW) / el / 1e9
		}
		lastRW = p.BytesRW
		r.progress("ram", time.Since(start).Seconds()/dur.Seconds(), fmt.Sprintf("passe %d · %s", p.Pass, p.Test), gbs, "Go/s", p.Errors)
	}}, avail)
	if err != nil {
		return err
	}
	defer t.Close()
	r.logf("RAM : %s alloués", report.HumanBytes(uint64(len(t.Words())*8)))
	r.mon.Watch("ram", b.last)
	res := t.Run(r.ctx)
	r.mon.Unwatch("ram")
	sec := &report.RAMSection{Result: res}
	if res.Duration > 0 {
		sec.BandwidthGBs = float64(res.BytesRW) / res.Duration.Seconds() / 1e9
	}
	r.rep.RAM = sec
	return nil
}

func (r *run) runDisk(d time.Duration) error {
	cfg := disk.Config{Dir: r.plan.DiskDir, WriteCap: r.plan.WriteCap(), FileSize: r.plan.DiskFileSize, Duration: d,
		RandDuration: min(max(d/12, time.Second), 5*time.Second)}
	if r.plan.Mode == ModeBench {
		cfg.RandDuration = 5 * time.Second
		cfg.Duration = 0
		if cfg.FileSize == 0 {
			cfg.FileSize = GiB
		}
	} else if r.plan.Profile == ProfileQuick && cfg.FileSize == 0 {
		cfg.FileSize = 2 * GiB
	}
	if r.plan.DiskRaw != "" {
		devs, err := disk.ListDevices()
		if err != nil {
			return err
		}
		var found *disk.Device
		for i := range devs {
			if devs[i].Name == r.plan.DiskRaw || devs[i].Path == r.plan.DiskRaw {
				found = &devs[i]
			}
		}
		if found == nil {
			return fmt.Errorf("périphérique %s introuvable", r.plan.DiskRaw)
		}
		cfg.Raw = found
		cfg.ConfirmSerial = r.plan.ConfirmSerial
	}
	var b beat
	b.touch()
	var lastDone uint64
	var lastPhase string
	step := -1 // sous-mesure courante : écriture, lecture, 4 × aléatoire, relecture
	const steps = 7
	cfg.Progress = func(p disk.Progress) {
		if p.Done != lastDone || p.Phase != lastPhase {
			b.touch()
		}
		if p.Phase != lastPhase {
			step++
		}
		lastDone, lastPhase = p.Done, p.Phase
		within := 0.0
		if p.Total > 0 {
			within = min(float64(p.Done)/float64(p.Total), 1)
		}
		local := (float64(min(step, steps-1)) + within) / steps
		r.progress("disk", local, p.Phase, p.MBs, "Mo/s", p.Errors)
	}
	r.mon.Watch("disk", b.last)
	defer r.mon.Unwatch("disk")
	res, err := disk.Run(r.ctx, cfg)
	if err != nil {
		return err
	}
	r.rep.Disks = append(r.rep.Disks, res)
	return nil
}

func (r *run) runNet(d time.Duration) error {
	cfg := netbench.Config{Server: r.plan.NetServer, Streams: r.plan.NetStreams, Duration: d}
	if r.plan.Mode == ModeBench {
		cfg.Step = 5 * time.Second
		cfg.Duration = 0
	} else if d < 2*time.Minute {
		cfg.Step = max(d/5, 3*time.Second)
	}
	var b beat
	b.touch()
	cfg.Progress = func(p netbench.Progress) {
		b.touch()
		r.progress("net", 0, p.Phase, p.Mbps, "Mbit/s", 0)
	}
	r.mon.Watch("net", b.last)
	defer r.mon.Unwatch("net")
	res, err := netbench.Run(r.ctx, cfg)
	if err != nil {
		return err
	}
	r.rep.Net = &res
	return nil
}
