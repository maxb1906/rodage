package disk

import (
	"context"
	"testing"
	"time"
)

func TestPattern(t *testing.T) {
	b := alignedBuf(seqBS)
	fill(b, 3*seqBS, 7)
	if i, _, _ := check(b, 3*seqBS, 7); i != -1 {
		t.Fatal("motif faux")
	}
	if i, _, _ := check(b, 4*seqBS, 7); i == -1 {
		t.Fatal("mauvaise adresse non détectée")
	}
	if i, _, _ := check(b, 3*seqBS, 8); i == -1 {
		t.Fatal("ancienne génération non détectée")
	}
	b[100] ^= 1
	if i, _, _ := check(b, 3*seqBS, 7); i != 12 {
		t.Fatalf("bit inversé : %d", i)
	}
}

func TestRunFileMode(t *testing.T) {
	dir := t.TempDir()
	var last Progress
	r, err := Run(context.Background(), Config{Dir: dir, FileSize: 128 << 20, WriteCap: 1 << 30,
		Duration: 3 * time.Second, RandDuration: 700 * time.Millisecond, Progress: func(p Progress) { last = p }})
	if err != nil {
		t.Fatal(err)
	}
	if r.VerifyErrors != 0 || r.IOErrors != 0 || r.SeqWriteMBs <= 0 || r.SeqReadMBs <= 0 || len(r.Random) != 4 {
		t.Fatalf("%+v", r)
	}
	t.Logf("direct=%v seqW=%.0f seqR=%.0f passes=%d written=%d Mo", r.Direct, r.SeqWriteMBs, r.SeqReadMBs, r.Passes, r.Written>>20)
	for _, x := range r.Random {
		t.Logf("%s : %.0f IOPS p99=%.0f µs", x.Name, x.IOPS, x.P99Us)
	}
	t.Logf("device=%+v health=%+v last=%+v", r.Device, r.HealthBefore.Note, last)
}

func TestRawRefusals(t *testing.T) {
	cases := []Device{
		{Name: "a", System: true, Serial: "X"},
		{Name: "b", Partitions: 1, Serial: "X"},
		{Name: "c", Mounted: true, Serial: "X"},
		{Name: "d"},
		{Name: "e", Serial: "X"},
	}
	for _, d := range cases {
		if err := CheckRawSafe(d, "Y"); err == nil {
			t.Fatalf("%s accepté", d.Name)
		}
	}
	if err := CheckRawSafe(Device{Name: "ok", Serial: "S1"}, "S1"); err != nil {
		t.Fatal(err)
	}
}

func TestSLC(t *testing.T) {
	c := []float64{}
	for i := 0; i < 40; i++ {
		v := 3000.0
		if i >= 20 {
			v = 800
		}
		c = append(c, v)
	}
	s := detectSLC(c)
	if !s.Detected || s.AfterGB < 55 || s.AfterGB > 65 {
		t.Fatalf("%+v", s)
	}
}

func TestListDevices(t *testing.T) {
	ds, err := ListDevices()
	if err != nil {
		t.Skip(err)
	}
	for _, d := range ds {
		t.Logf("%+v", d)
	}
}
