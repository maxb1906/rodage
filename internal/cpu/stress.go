package cpu

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/maxb1906/rodage/internal/cpu/kernels"
)

// ErrorEvent est une erreur de calcul détectée.
type ErrorEvent struct {
	Time     time.Time `json:"time"`
	CPU      int       `json:"cpu"`
	Workload string    `json:"workload"`
	Detail   string    `json:"detail"`
}

// CoreResult est le bilan d'un cœur logique.
type CoreResult struct {
	CPU        int    `json:"cpu"`
	Pinned     bool   `json:"pinned"`
	Iterations uint64 `json:"iterations"`
	Errors     uint64 `json:"errors"`
}

// StressResult est le bilan du stress CPU.
type StressResult struct {
	Kernel      string        `json:"kernel"`
	Threads     int           `json:"threads"`
	Workloads   []string      `json:"workloads"`
	Duration    time.Duration `json:"duration"`
	Cores       []CoreResult  `json:"cores"`
	TotalErrors uint64        `json:"total_errors"`
	Errors      []ErrorEvent  `json:"errors,omitempty"`
	Stalled     []int         `json:"stalled,omitempty"`
	Aborted     string        `json:"aborted,omitempty"`
}

// FailingCores renvoie les cœurs ayant produit au moins une erreur.
func (r StressResult) FailingCores() []int {
	var out []int
	for _, c := range r.Cores {
		if c.Errors > 0 {
			out = append(out, c.CPU)
		}
	}
	return out
}

// StressConfig paramètre le stress CPU.
type StressConfig struct {
	Duration time.Duration
	Kernel   kernels.Kernel
	// CPUs liste les cœurs logiques à charger (vide = tous).
	CPUs []int
	// Workloads à exécuter en alternance (vide = tous).
	Workloads []Workload
}

type coreState struct {
	iter   atomic.Uint64
	errs   atomic.Uint64
	beat   atomic.Int64 // dernière progression (UnixNano)
	pinned atomic.Bool
}

// Stress est un stress CPU en cours.
type Stress struct {
	cfg    StressConfig
	cores  []*coreState
	cpus   []int
	mu     sync.Mutex
	events []ErrorEvent
	start  time.Time
}

// NewStress prépare le stress (calcule les références).
func NewStress(cfg StressConfig) (*Stress, error) {
	if len(cfg.Workloads) == 0 {
		ws, err := DefaultWorkloads(cfg.Kernel)
		if err != nil {
			return nil, err
		}
		cfg.Workloads = ws
	}
	if len(cfg.CPUs) == 0 {
		for i := 0; i < runtime.NumCPU(); i++ {
			cfg.CPUs = append(cfg.CPUs, i)
		}
	}
	s := &Stress{cfg: cfg, cpus: cfg.CPUs}
	for range cfg.CPUs {
		s.cores = append(s.cores, &coreState{})
	}
	return s, nil
}

// Progress renvoie (itérations, erreurs) cumulées.
func (s *Stress) Progress() (iter, errs uint64) {
	for _, c := range s.cores {
		iter += c.iter.Load()
		errs += c.errs.Load()
	}
	return
}

// Heartbeats renvoie, par cœur, la dernière progression.
func (s *Stress) Heartbeats() map[int]time.Time {
	m := make(map[int]time.Time, len(s.cores))
	for i, c := range s.cores {
		m[s.cpus[i]] = time.Unix(0, c.beat.Load())
	}
	return m
}

// Run exécute le stress jusqu'à la fin de la durée ou l'annulation de ctx.
func (s *Stress) Run(ctx context.Context) StressResult {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Duration)
	defer cancel()
	s.start = time.Now()
	var wg sync.WaitGroup
	for i, cpu := range s.cpus {
		st := s.cores[i]
		st.beat.Store(time.Now().UnixNano())
		wg.Add(1)
		go func(cpu int, st *coreState) {
			defer wg.Done()
			s.worker(ctx, cpu, st)
		}(cpu, st)
	}
	wg.Wait()
	res := StressResult{
		Kernel:   string(s.cfg.Kernel.Level),
		Threads:  len(s.cpus),
		Duration: time.Since(s.start).Round(time.Millisecond),
	}
	for _, w := range s.cfg.Workloads {
		res.Workloads = append(res.Workloads, w.Name())
	}
	for i, c := range s.cores {
		cr := CoreResult{CPU: s.cpus[i], Pinned: c.pinned.Load(), Iterations: c.iter.Load(), Errors: c.errs.Load()}
		res.Cores = append(res.Cores, cr)
		res.TotalErrors += cr.Errors
	}
	s.mu.Lock()
	res.Errors = append(res.Errors, s.events...)
	s.mu.Unlock()
	if err := context.Cause(ctx); err != nil && err != context.DeadlineExceeded {
		if ctx.Err() != context.DeadlineExceeded {
			res.Aborted = err.Error()
		}
	}
	return res
}

func (s *Stress) worker(ctx context.Context, cpu int, st *coreState) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	st.pinned.Store(pin(cpu) == nil)
	runners := make([]Runner, len(s.cfg.Workloads))
	for i, w := range s.cfg.Workloads {
		runners[i] = w.New()
	}
	for {
		for i, r := range runners {
			// Chaque calcul est répété pendant ~100 ms avant de passer au suivant.
			until := time.Now().Add(100 * time.Millisecond)
			for time.Now().Before(until) {
				if ctx.Err() != nil {
					return
				}
				if msg := s.safeRun(r); msg != "" {
					st.errs.Add(1)
					s.record(ErrorEvent{Time: time.Now(), CPU: cpu, Workload: s.cfg.Workloads[i].Name(), Detail: msg})
				}
				st.iter.Add(1)
				st.beat.Store(time.Now().UnixNano())
			}
		}
	}
}

// safeRun transforme une panique (ex. instruction illégale) en erreur.
func (s *Stress) safeRun(r Runner) (msg string) {
	defer func() {
		if p := recover(); p != nil {
			msg = fmt.Sprintf("panique : %v", p)
		}
	}()
	return r.Run()
}

func (s *Stress) record(e ErrorEvent) {
	s.mu.Lock()
	if len(s.events) < 200 {
		s.events = append(s.events, e)
	}
	s.mu.Unlock()
}
