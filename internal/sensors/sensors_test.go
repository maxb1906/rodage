package sensors

import (
	"testing"
	"time"
)

func TestOpenRead(t *testing.T) {
	s := Open()
	defer s.Close()
	a := s.Read()
	time.Sleep(50 * time.Millisecond)
	b := s.Read()
	t.Logf("niveau %d %v tjmax=%v ; %+v ; %+v", s.Level(), s.Details(), s.TjMax(), a, b)
}

func TestDecodePerfLimit(t *testing.T) {
	th := decodePerfLimit(1<<1 | 1<<10)
	if !th.Thermal || !th.PL1 || th.PL2 || th.VRM {
		t.Fatalf("%+v", th)
	}
	tmp, ok, _, _ := decodeThermStatus(1<<31|20<<16, 100)
	if !ok || tmp != 80 {
		t.Fatalf("%v %v", tmp, ok)
	}
}

func TestAnalyzeThrottle(t *testing.T) {
	var ss []Sample
	for i := 0; i < 60; i++ {
		f := 4000.0
		if i > 30 {
			f = 3000
		}
		ss = append(ss, Sample{FreqMHz: f, CPUTempC: 95})
	}
	r := AnalyzeThrottle(ss, 100, LevelNoDrv)
	if !r.Detected || r.Probable != "thermal" || r.DropPct < 20 {
		t.Fatalf("%+v", r)
	}
	var flat []Sample
	for i := 0; i < 60; i++ {
		flat = append(flat, Sample{FreqMHz: 3500, CPUTempC: 60})
	}
	if r := AnalyzeThrottle(flat, 100, LevelLinux); r.Detected {
		t.Fatalf("faux positif %+v", r)
	}
}
