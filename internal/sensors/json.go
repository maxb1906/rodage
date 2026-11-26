package sensors

import (
	"encoding/json"
	"math"
	"time"
)

// nf convertit NaN/Inf en null pour JSON.
func nf(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

func fn(p *float64) float64 {
	if p == nil {
		return NaN
	}
	return *p
}

type sampleJSON struct {
	Time       time.Time `json:"t"`
	CPUTempC   *float64  `json:"temp"`
	CoreTempsC []float64 `json:"core_temps,omitempty"`
	FreqMHz    *float64  `json:"freq"`
	CoreFreq   []float64 `json:"core_freq,omitempty"`
	PowerW     *float64  `json:"power"`
	Throttle   Throttle  `json:"throttle"`
}

// MarshalJSON écrit les valeurs inconnues (NaN) sous forme de null.
func (s Sample) MarshalJSON() ([]byte, error) {
	return json.Marshal(sampleJSON{s.Time, nf(s.CPUTempC), s.CoreTempsC, nf(s.FreqMHz), s.CoreFreq, nf(s.PowerW), s.Throttle})
}

// UnmarshalJSON relit null en NaN.
func (s *Sample) UnmarshalJSON(b []byte) error {
	var j sampleJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	*s = Sample{j.Time, fn(j.CPUTempC), j.CoreTempsC, fn(j.FreqMHz), j.CoreFreq, fn(j.PowerW), j.Throttle}
	return nil
}
