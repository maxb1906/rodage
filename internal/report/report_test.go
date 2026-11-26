package report

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maxb1906/rodage/internal/cpu"
	"github.com/maxb1906/rodage/internal/disk"
	"github.com/maxb1906/rodage/internal/i18n"
	"github.com/maxb1906/rodage/internal/memtest"
	"github.com/maxb1906/rodage/internal/sensors"
	"github.com/maxb1906/rodage/internal/sysinfo"
)

func sample() *Report {
	t0 := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	r := &Report{Tool: "rodage", Version: "test", ID: "abc", Created: t0, Lang: "fr", Mode: "stress", Profile: "quick",
		System:  sysinfo.Info{Hostname: "pc", CPUModel: "CPU X", LogicalCPUs: 2, RAMTotal: 8 << 30, SMBIOS: sysinfo.SMBIOS{Serial: "S1"}},
		Sensors: sensors.Info{Level: 3, TjMax: 100, Details: []string{"hwmon coretemp"}}, ThermalStop: 95}
	for i := 0; i < 120; i++ {
		r.Samples = append(r.Samples, sensors.Sample{Time: t0.Add(time.Duration(i) * time.Second), CPUTempC: 50 + float64(i%30), FreqMHz: 3000 - float64(i), PowerW: sensors.NaN})
	}
	r.Phases = []Phase{{"cpu", t0, t0.Add(60 * time.Second)}, {"ram", t0.Add(60 * time.Second), t0.Add(120 * time.Second)}}
	r.CPU = &CPUSection{Stress: &cpu.StressResult{Kernel: "avx2", Threads: 2, Duration: time.Minute, Cores: []cpu.CoreResult{{CPU: 0, Pinned: true, Iterations: 10}, {CPU: 1, Pinned: true, Iterations: 10, Errors: 2}}, TotalErrors: 2},
		Bench:    &cpu.BenchResult{Scores: []cpu.Score{{Name: "fft", Unit: "GFLOPS", Single: 2, Multi: 4}}},
		Throttle: &sensors.ThrottleReport{Detected: true, BaselineMHz: 3000, SustainedMHz: 2500, DropPct: 16.7, Probable: "thermal"}}
	r.RAM = &RAMSection{Result: memtest.Result{Bytes: 1 << 30, Passes: 2, Locked: true}, BandwidthGBs: 10}
	r.Disks = []disk.Result{{Mode: "file", Path: "/tmp/x", Direct: true, ReadBytes: 1 << 30, SeqReadMBs: 500, SeqWriteMBs: 400, WriteCurve: []float64{400, 410, 390, 100, 90, 95, 100, 98, 97}, Random: []disk.RandResult{{Name: "read-4k-qd1", IOPS: 10000, P99Us: 200}}}}
	r.MaxTempC = 79
	r.Evaluate()
	return r
}

func TestEvaluateAndRender(t *testing.T) {
	r := sample()
	if r.Verdict != Fail {
		t.Fatalf("verdict %s", r.Verdict)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "r.json")
	if err := r.Save(p); err != nil {
		t.Fatal(err)
	}
	r2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range []i18n.Lang{i18n.FR, i18n.EN} {
		var b bytes.Buffer
		if err := r2.WriteHTML(&b, l); err != nil {
			t.Fatal(err)
		}
		html := b.String()
		if strings.Contains(html, "%!") || !strings.Contains(html, "<svg") {
			t.Fatalf("HTML %s invalide : %s", l, html[:min(400, len(html))])
		}
		for _, f := range r2.Findings {
			if s := f.Text(l); strings.Contains(s, "%!") {
				t.Fatalf("constat mal formaté : %s", s)
			}
		}
		var pb bytes.Buffer
		if err := r2.WritePDF(&pb, l); err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(pb.Bytes(), []byte("%PDF")) {
			t.Fatal("PDF invalide")
		}
		if out := os.Getenv("RODAGE_TEST_OUT"); out != "" {
			os.WriteFile(filepath.Join(out, "rodage-test-"+string(l)+".pdf"), pb.Bytes(), 0o644)
			os.WriteFile(filepath.Join(out, "rodage-test-"+string(l)+".html"), b.Bytes(), 0o644)
		}
	}
}

func TestCompare(t *testing.T) {
	a, b := sample(), sample()
	b.Disks[0].SeqReadMBs = 250
	b.RAM.BandwidthGBs = 12
	d := Compare(a, b)
	found := false
	for _, row := range d.Rows {
		if row.Metric == "seq read" {
			found = true
			if row.Better != -1 || row.DeltaPct != -50 {
				t.Fatalf("%+v", row)
			}
		}
	}
	if !found {
		t.Fatal("ligne absente")
	}
	var buf bytes.Buffer
	d.WriteText(&buf, i18n.FR)
	if err := d.WriteHTML(&buf, i18n.EN); err != nil {
		t.Fatal(err)
	}
	t.Log(buf.String()[:600])
}
