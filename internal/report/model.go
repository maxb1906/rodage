// Package report construit, évalue et rend les rapports de validation.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/maxb1906/rodage/internal/cpu"
	"github.com/maxb1906/rodage/internal/disk"
	"github.com/maxb1906/rodage/internal/memtest"
	"github.com/maxb1906/rodage/internal/netbench"
	"github.com/maxb1906/rodage/internal/safety"
	"github.com/maxb1906/rodage/internal/sensors"
	"github.com/maxb1906/rodage/internal/sysinfo"
)

// Verdict global.
type Verdict string

const (
	Pass       Verdict = "pass"
	Warn       Verdict = "warn"
	Fail       Verdict = "fail"
	Incomplete Verdict = "incomplete"
)

// Severity d'un constat.
type Severity string

const (
	SevFail Severity = "fail"
	SevWarn Severity = "warn"
	SevInfo Severity = "info"
	SevOK   Severity = "ok"
)

// Finding est un constat traduisible (clé i18n + arguments).
type Finding struct {
	Severity  Severity `json:"severity"`
	Component string   `json:"component"`
	Key       string   `json:"key"`
	Args      []any    `json:"args,omitempty"`
}

// Phase est une étape horodatée du test.
type Phase struct {
	Name  string    `json:"name"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// CPUSection regroupe les résultats processeur.
type CPUSection struct {
	Stress   *cpu.StressResult       `json:"stress,omitempty"`
	Bench    *cpu.BenchResult        `json:"bench,omitempty"`
	Throttle *sensors.ThrottleReport `json:"throttle,omitempty"`
}

// RAMSection regroupe les résultats mémoire.
type RAMSection struct {
	memtest.Result
	BandwidthGBs float64 `json:"bandwidth_gbs"`
}

// Report est un rapport de validation complet.
type Report struct {
	Tool        string           `json:"tool"`
	Version     string           `json:"version"`
	ID          string           `json:"id"`
	Created     time.Time        `json:"created"`
	Lang        string           `json:"lang"`
	Mode        string           `json:"mode"`    // bench, stress
	Profile     string           `json:"profile"` // quick, standard, full, custom, bench
	Planned     time.Duration    `json:"planned"`
	Elapsed     time.Duration    `json:"elapsed"`
	System      sysinfo.Info     `json:"system"`
	Sensors     sensors.Info     `json:"sensors"`
	ThermalStop float64          `json:"thermal_stop"`
	PrevCrash   *safety.Marker   `json:"prev_crash,omitempty"`
	CPU         *CPUSection      `json:"cpu,omitempty"`
	RAM         *RAMSection      `json:"ram,omitempty"`
	Disks       []disk.Result    `json:"disks,omitempty"`
	Net         *netbench.Result `json:"net,omitempty"`
	Errors      []string         `json:"errors,omitempty"` // échecs de lancement d'un test
	Safety      []safety.Event   `json:"safety,omitempty"`
	Phases      []Phase          `json:"phases"`
	Samples     []sensors.Sample `json:"samples"`
	MaxTempC    float64          `json:"max_temp_c"`
	Verdict     Verdict          `json:"verdict"`
	Findings    []Finding        `json:"findings"`
	Limitations []string         `json:"limitations"` // clés i18n
	References  []RefComparison  `json:"references,omitempty"`
}

// RefComparison compare un score à la base locale de références.
type RefComparison struct {
	Name     string  `json:"name"`
	Value    float64 `json:"value"`
	RefValue float64 `json:"ref_value"`
	Count    int     `json:"count"`
	DeltaPct float64 `json:"delta_pct"`
}

// Save écrit le rapport en JSON.
func (r *Report) Save(path string) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// Load lit un rapport JSON.
func Load(path string) (*Report, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Report
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s : rapport illisible : %w", path, err)
	}
	if r.Tool != "rodage" {
		return nil, fmt.Errorf("%s : ce n'est pas un rapport Rodage", path)
	}
	return &r, nil
}

// Downsample réduit une série de relevés à au plus n points (température max,
// fréquence moyenne par intervalle).
func Downsample(ss []sensors.Sample, n int) []sensors.Sample {
	if len(ss) <= n || n <= 0 {
		out := make([]sensors.Sample, len(ss))
		for i, s := range ss {
			out[i] = sensors.Sample{Time: s.Time, CPUTempC: s.CPUTempC, FreqMHz: s.FreqMHz, PowerW: s.PowerW, Throttle: s.Throttle}
		}
		return out
	}
	out := make([]sensors.Sample, 0, n)
	per := float64(len(ss)) / float64(n)
	for i := 0; i < n; i++ {
		a, b := int(float64(i)*per), int(float64(i+1)*per)
		b = min(b, len(ss))
		agg := sensors.Sample{Time: ss[a].Time, CPUTempC: sensors.NaN, FreqMHz: sensors.NaN, PowerW: sensors.NaN}
		var fs, ps float64
		var nf, np int
		for _, s := range ss[a:b] {
			if s.CPUTempC == s.CPUTempC && (agg.CPUTempC != agg.CPUTempC || s.CPUTempC > agg.CPUTempC) {
				agg.CPUTempC = s.CPUTempC
			}
			if s.FreqMHz == s.FreqMHz {
				fs += s.FreqMHz
				nf++
			}
			if s.PowerW == s.PowerW {
				ps += s.PowerW
				np++
			}
			agg.Throttle.Merge(s.Throttle)
		}
		if nf > 0 {
			agg.FreqMHz = fs / float64(nf)
		}
		if np > 0 {
			agg.PowerW = ps / float64(np)
		}
		out = append(out, agg)
	}
	return out
}
