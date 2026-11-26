package cpu

import (
	"context"
	"testing"
	"time"

	"github.com/maxb1906/rodage/internal/cpu/kernels"
)

func TestWorkloadsCorrect(t *testing.T) {
	for _, k := range kernels.Available() {
		ws, err := DefaultWorkloads(k)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range ws {
			if msg := w.New().Run(); msg != "" {
				t.Fatalf("%s : %s", w.Name(), msg)
			}
		}
	}
}

// Une erreur injectée doit être détectée.
func TestCorruptionDetected(t *testing.T) {
	w := NewSIMDFloat(kernels.Best()).(*simdFloat)
	w.golden[10] += 1e-12
	if w.New().Run() == "" {
		t.Fatal("corruption non détectée")
	}
}

func TestStressShort(t *testing.T) {
	s, err := NewStress(StressConfig{Duration: 1500 * time.Millisecond, Kernel: kernels.Best()})
	if err != nil {
		t.Fatal(err)
	}
	r := s.Run(context.Background())
	if r.TotalErrors != 0 || len(r.Cores) == 0 {
		t.Fatalf("%+v", r)
	}
	for _, c := range r.Cores {
		if c.Iterations == 0 || !c.Pinned {
			t.Fatalf("cœur %+v", c)
		}
	}
	t.Logf("%+v", r.Cores)
}

func TestBenchShort(t *testing.T) {
	r, err := Bench(context.Background(), BenchConfig{PerTest: 200 * time.Millisecond, Kernel: kernels.Best()})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range r.Scores {
		t.Logf("%-18s %8.2f %8.2f %s err=%d", s.Name, s.Single, s.Multi, s.Unit, s.Errors)
		if s.Errors != 0 || s.Single <= 0 {
			t.Fatal("score invalide")
		}
	}
}
