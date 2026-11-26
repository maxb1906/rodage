package engine

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/maxb1906/rodage/internal/i18n"
	"github.com/maxb1906/rodage/internal/netbench"
	"github.com/maxb1906/rodage/internal/report"
)

func TestBudget(t *testing.T) {
	p := DefaultPlan(ModeStress, ProfileQuick)
	b := p.Budget()
	var sum time.Duration
	for _, d := range b {
		sum += d
	}
	if sum < 9*time.Minute || sum > 11*time.Minute || b["net"] != 0 {
		t.Fatalf("%v", b)
	}
	if err := (Plan{Mode: ModeStress, Profile: ProfileFull, CPU: true, Duration: time.Hour}).Validate(); err == nil {
		t.Fatal("durée hors bornes acceptée")
	}
}

// Test complet court : stress personnalisé de 30 s, 256 Mio de RAM, disque en mode fichier, réseau local.
func TestRunStressShort(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	go (&netbench.Server{Version: "test"}).ListenAndServe(ctx, addr)
	time.Sleep(100 * time.Millisecond)

	p := DefaultPlan(ModeStress, ProfileCustom)
	p.Duration = 30 * time.Second
	p.RAMBytes = 256 << 20
	p.DiskDir = t.TempDir()
	p.DiskFileSize = 64 << 20
	p.Net = true
	p.NetServer = addr
	p.Lang = "fr"
	var kinds = map[string]int{}
	rep, err := Run(ctx, p, func(e Event) { kinds[e.Kind]++ })
	if err != nil {
		t.Fatal(err)
	}
	if rep.CPU == nil || rep.RAM == nil || len(rep.Disks) != 1 || rep.Net == nil {
		t.Fatalf("sections manquantes : %+v", rep.Errors)
	}
	if rep.Verdict == report.Fail {
		for _, f := range rep.Findings {
			t.Log(f.Text(i18n.FR))
		}
		t.Fatal("verdict échoué")
	}
	if kinds["phase"] != 4 || kinds["done"] != 1 || kinds["progress"] == 0 {
		t.Fatalf("événements %v", kinds)
	}
	var b bytes.Buffer
	if err := rep.WriteHTML(&b, i18n.FR); err != nil {
		t.Fatal(err)
	}
	t.Logf("verdict %s en %s", rep.Verdict, rep.Elapsed)
	for _, f := range rep.Findings {
		t.Log(" -", f.Text(i18n.FR))
	}
}

func TestUserStop(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	p := DefaultPlan(ModeStress, ProfileCustom)
	p.Duration = 50 * time.Second
	p.RAM, p.Disk = false, false
	go func() { time.Sleep(2 * time.Second); cancel() }()
	rep, err := Run(ctx, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != report.Incomplete {
		t.Fatalf("verdict %s", rep.Verdict)
	}
}

func TestFractionMonotonic(t *testing.T) {
	r := &run{phases: []string{"cpu", "disk"}}
	r.idx.Store(1)
	if f := r.fraction(0.6); f != 0.8 {
		t.Fatalf("%v", f)
	}
	// Un relevé de sonde (local = 0) ne doit pas faire reculer la barre.
	if f := r.fraction(0); f != 0.8 {
		t.Fatalf("recul : %v", f)
	}
}

// Bench disque réel : l'avancement émis ne doit jamais reculer.
func TestBenchDiskProgressMonotonic(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := DefaultPlan(ModeBench, ProfileBench)
	p.CPU, p.RAM = false, false
	p.DiskDir = t.TempDir()
	p.DiskFileSize = 64 << 20
	last := 0.0
	n := 0
	_, err := Run(context.Background(), p, func(e Event) {
		if e.Fraction < last {
			t.Errorf("recul %s/%s : %.3f < %.3f", e.Kind, e.Phase, e.Fraction, last)
		}
		last = e.Fraction
		n++
	})
	if err != nil {
		t.Fatal(err)
	}
	if last != 1 || n < 10 {
		t.Fatalf("fin à %.3f après %d événements", last, n)
	}
}
