package memtest

import (
	"context"
	"testing"
	"time"
)

func TestRunClean(t *testing.T) {
	m, err := New(Config{Bytes: 64 << 20, Duration: 3 * time.Second}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	r := m.Run(context.Background())
	if r.TotalErrors != 0 || r.Passes < 1 {
		t.Fatalf("%+v", r)
	}
	t.Logf("passes=%d locked=%v note=%q rw=%d Mo ecc=%+v", r.Passes, r.Locked, r.LockNote, r.BytesRW>>20, r.ECC)
}

// Une cellule « collée » doit être détectée par tous les tests.
func TestStuckBitDetected(t *testing.T) {
	m, err := New(Config{Bytes: 4 << 20, Duration: time.Hour, Threads: 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	w := m.Words()
	for _, f := range m.tests() {
		before := m.errs.Load()
		// Simule un bit collé à 1 : on le force après chaque écriture via un test dédié.
		stuck := &stuckWords{w: w, idx: 12345, bit: 1 << 7}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					stuck.apply()
				}
			}
		}()
		f.run(ctx, 0)
		close(done)
		cancel()
		if m.errs.Load() == before {
			t.Errorf("%s : bit collé non détecté", f.name)
		}
	}
}

type stuckWords struct {
	w   []uint64
	idx int
	bit uint64
}

func (s *stuckWords) apply() { s.w[s.idx] |= s.bit }
