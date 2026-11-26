package cpu

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/maxb1906/rodage/internal/cpu/kernels"
)

// Score est le résultat d'une mesure.
type Score struct {
	Name   string  `json:"name"`
	Unit   string  `json:"unit"`
	Single float64 `json:"single"` // un cœur
	Multi  float64 `json:"multi"`  // tous les cœurs (0 si non mesuré)
	Errors uint64  `json:"errors"`
}

// BenchResult regroupe les scores CPU.
type BenchResult struct {
	Kernel  string  `json:"kernel"`
	Threads int     `json:"threads"`
	Scores  []Score `json:"scores"`
}

// BenchConfig paramètre le bench.
type BenchConfig struct {
	// PerTest est la durée de chaque mesure.
	PerTest time.Duration
	Kernel  kernels.Kernel
	// Progress est appelé avant chaque mesure (facultatif).
	Progress func(name string, done, total int)
}

// measure exécute w sur threads fils pendant d et renvoie un débit en unités/s.
func measure(ctx context.Context, w Workload, threads int, d time.Duration) (rate float64, errs uint64) {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	var total, bad atomic.Uint64
	var wg sync.WaitGroup
	start := time.Now()
	for t := 0; t < threads; t++ {
		wg.Add(1)
		go func(t int) {
			defer wg.Done()
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			_ = pin(t % runtime.NumCPU())
			r := w.New()
			for ctx.Err() == nil {
				if r.Run() != "" {
					bad.Add(1)
				}
				total.Add(1)
			}
		}(t)
	}
	wg.Wait()
	el := time.Since(start).Seconds()
	return float64(total.Load()) * w.Ops() / el / 1e9, bad.Load()
}

// Bench mesure les performances CPU de façon reproductible.
func Bench(ctx context.Context, cfg BenchConfig) (BenchResult, error) {
	if cfg.PerTest <= 0 {
		cfg.PerTest = 3 * time.Second
	}
	threads := runtime.NumCPU()
	res := BenchResult{Kernel: string(cfg.Kernel.Level), Threads: threads}
	ws, err := DefaultWorkloads(cfg.Kernel)
	if err != nil {
		return res, err
	}
	// Comparaison des niveaux SIMD disponibles, sur un cœur.
	var levels []Workload
	for _, k := range kernels.Available() {
		if k.Level != cfg.Kernel.Level {
			levels = append(levels, NewSIMDFloat(k))
		}
	}
	total := 2*len(ws) + len(levels)
	done := 0
	step := func(name string) {
		if cfg.Progress != nil {
			cfg.Progress(name, done, total)
		}
		done++
	}
	for _, w := range ws {
		step(w.Name())
		s := Score{Name: w.Name(), Unit: w.Unit()}
		var e1, e2 uint64
		s.Single, e1 = measure(ctx, w, 1, cfg.PerTest)
		step(w.Name())
		s.Multi, e2 = measure(ctx, w, threads, cfg.PerTest)
		s.Errors = e1 + e2
		res.Scores = append(res.Scores, s)
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
	}
	for _, w := range levels {
		step(w.Name())
		s := Score{Name: w.Name(), Unit: w.Unit()}
		s.Single, s.Errors = measure(ctx, w, 1, cfg.PerTest/2)
		res.Scores = append(res.Scores, s)
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
	}
	return res, nil
}
