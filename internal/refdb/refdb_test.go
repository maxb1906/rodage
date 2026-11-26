package refdb

import (
	"path/filepath"
	"testing"

	"github.com/maxb1906/rodage/internal/cpu"
	"github.com/maxb1906/rodage/internal/report"
	"github.com/maxb1906/rodage/internal/sysinfo"
)

func bench(id string, v float64) *report.Report {
	return &report.Report{ID: id, System: sysinfo.Info{CPUModel: "CPU X"},
		CPU: &report.CPUSection{Bench: &cpu.BenchResult{Threads: 4, Kernel: "neon", Scores: []cpu.Score{{Name: "fft", Single: v, Multi: 4 * v}}}}}
}

func TestDB(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ref.json")
	db, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	db.Add(bench("a", 10))
	db.Add(bench("b", 12))
	if err := db.Save(); err != nil {
		t.Fatal(err)
	}
	db2, _ := Open(p)
	cs := db2.Compare(bench("c", 11))
	if len(cs) != 2 || cs[0].Count != 2 || cs[0].RefValue != 11 {
		t.Fatalf("%+v", cs)
	}
	if n := db2.Add(bench("a", 10)); n != 0 {
		t.Fatal("doublon accepté")
	}
}
