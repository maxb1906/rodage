package sensors

import (
	"sort"
	"sync"
	"time"
)

// ThrottleReport est l'analyse du bridage pendant une charge constante.
type ThrottleReport struct {
	Detected     bool     `json:"detected"`
	BaselineMHz  float64  `json:"baseline_mhz"`
	SustainedMHz float64  `json:"sustained_mhz"`
	DropPct      float64  `json:"drop_pct"`
	MaxTempC     float64  `json:"max_temp_c"`
	Causes       Throttle `json:"causes"`
	// CauseKnown indique que la cause vient du matériel (niveaux 2 et 3).
	CauseKnown bool `json:"cause_known"`
	// Probable : cause déduite (température proche de TjMax) faute de mieux.
	Probable string `json:"probable,omitempty"`
}

// AnalyzeThrottle repère une chute de fréquence sous charge constante.
// samples doit couvrir la seule période de charge.
func AnalyzeThrottle(samples []Sample, tjmax float64, level Level) ThrottleReport {
	var r ThrottleReport
	var freqs []float64
	r.MaxTempC = 0
	for _, s := range samples {
		if s.FreqMHz == s.FreqMHz && s.FreqMHz > 0 {
			freqs = append(freqs, s.FreqMHz)
		}
		if s.CPUTempC == s.CPUTempC && s.CPUTempC > r.MaxTempC {
			r.MaxTempC = s.CPUTempC
		}
		r.Causes.Merge(s.Throttle)
	}
	if len(freqs) < 6 {
		r.CauseKnown = level >= LevelPawnIO && r.Causes.Any()
		return r
	}
	// Référence : médiane des premiers 20 % (hors 2 premiers relevés de montée en charge).
	start := 2
	n := len(freqs) - start
	head := freqs[start : start+max(3, n/5)]
	r.BaselineMHz = median(head)
	// Fréquence soutenue : médiane la plus basse sur une fenêtre glissante de 5 relevés.
	win := 5
	low := r.BaselineMHz
	for i := start; i+win <= len(freqs); i++ {
		if m := median(freqs[i : i+win]); m < low {
			low = m
		}
	}
	r.SustainedMHz = median(freqs[len(freqs)-max(3, n/5):])
	if low < r.SustainedMHz {
		r.SustainedMHz = low
	}
	if r.BaselineMHz > 0 {
		r.DropPct = 100 * (r.BaselineMHz - r.SustainedMHz) / r.BaselineMHz
	}
	r.Detected = r.DropPct >= 10 || (r.Causes.Any() && r.DropPct >= 3)
	hwCause := r.Causes.Thermal || r.Causes.Prochot || r.Causes.PL1 || r.Causes.PL2 || r.Causes.VRM || r.Causes.Other
	r.CauseKnown = hwCause && level >= LevelPawnIO
	if r.Detected && !r.CauseKnown {
		if tjmax == tjmax && r.MaxTempC >= tjmax-8 {
			r.Probable = "thermal"
		} else if r.Causes.LimitPct > 0 {
			r.Probable = "limit"
		} else {
			r.Probable = "unknown"
		}
	}
	return r
}

func median(v []float64) float64 {
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	if len(c) == 0 {
		return 0
	}
	return c[len(c)/2]
}

// Sampler relève une Source à intervalle régulier.
type Sampler struct {
	src  Source
	mu   sync.Mutex
	all  []Sample
	subs []func(Sample)
	stop chan struct{}
	done chan struct{}
}

// NewSampler démarre un relevé toutes les period.
func NewSampler(src Source, period time.Duration) *Sampler {
	s := &Sampler{src: src, stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		t := time.NewTicker(period)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				sm := src.Read()
				s.mu.Lock()
				s.all = append(s.all, sm)
				subs := append([]func(Sample){}, s.subs...)
				s.mu.Unlock()
				for _, f := range subs {
					f(sm)
				}
			}
		}
	}()
	return s
}

// Subscribe enregistre une fonction appelée à chaque relevé.
func (s *Sampler) Subscribe(f func(Sample)) {
	s.mu.Lock()
	s.subs = append(s.subs, f)
	s.mu.Unlock()
}

// Between renvoie les relevés compris entre a et b.
func (s *Sampler) Between(a, b time.Time) []Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Sample
	for _, x := range s.all {
		if !x.Time.Before(a) && !x.Time.After(b) {
			out = append(out, x)
		}
	}
	return out
}

// All renvoie une copie de tous les relevés.
func (s *Sampler) All() []Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Sample(nil), s.all...)
}

// Last renvoie le dernier relevé.
func (s *Sampler) Last() (Sample, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.all) == 0 {
		return Sample{}, false
	}
	return s.all[len(s.all)-1], true
}

// Stop arrête le relevé.
func (s *Sampler) Stop() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.done
}
