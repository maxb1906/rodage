// Package sensors lit températures, fréquences et causes de throttling.
//
// Trois niveaux de mesure, sans pilote noyau embarqué :
//   - Niveau 1 (Windows sans pilote) : compteurs de performance, zones ACPI ;
//   - Niveau 2 (Windows + PawnIO)    : MSR, températures par cœur, causes ;
//   - Niveau 3 (Linux)               : hwmon, coretemp/k10temp, /dev/cpu/*/msr.
package sensors

import (
	"math"
	"time"
)

// Level est le niveau de mesure atteint.
type Level int

const (
	LevelNone   Level = 0
	LevelNoDrv  Level = 1
	LevelPawnIO Level = 2
	LevelLinux  Level = 3
)

// Throttle décrit les causes de bridage signalées par le matériel.
type Throttle struct {
	Thermal    bool    `json:"thermal,omitempty"`
	Prochot    bool    `json:"prochot,omitempty"`
	PL1        bool    `json:"pl1,omitempty"`
	PL2        bool    `json:"pl2,omitempty"`
	VRM        bool    `json:"vrm,omitempty"` // VR thermique ou courant (TDC/EDP)
	Other      bool    `json:"other,omitempty"`
	LimitPct   float64 `json:"limit_pct,omitempty"` // Windows : « % Performance Limit »
	CountDelta uint64  `json:"count_delta,omitempty"`
}

// Any indique si une cause est présente.
func (t Throttle) Any() bool {
	return t.Thermal || t.Prochot || t.PL1 || t.PL2 || t.VRM || t.Other || t.LimitPct > 0 || t.CountDelta > 0
}

// Merge combine deux relevés.
func (t *Throttle) Merge(o Throttle) {
	t.Thermal = t.Thermal || o.Thermal
	t.Prochot = t.Prochot || o.Prochot
	t.PL1 = t.PL1 || o.PL1
	t.PL2 = t.PL2 || o.PL2
	t.VRM = t.VRM || o.VRM
	t.Other = t.Other || o.Other
	t.LimitPct = math.Max(t.LimitPct, o.LimitPct)
	t.CountDelta += o.CountDelta
}

// Sample est un relevé instantané. Les valeurs inconnues valent NaN.
type Sample struct {
	Time       time.Time `json:"t"`
	CPUTempC   float64   `json:"temp"`
	CoreTempsC []float64 `json:"core_temps,omitempty"`
	FreqMHz    float64   `json:"freq"`
	CoreFreq   []float64 `json:"core_freq,omitempty"`
	PowerW     float64   `json:"power"`
	Throttle   Throttle  `json:"throttle"`
}

// Source est une source de mesures.
type Source interface {
	Level() Level
	// Details décrit les sondes effectivement utilisées.
	Details() []string
	// TjMax renvoie la température maximale de jonction (°C), ou NaN.
	TjMax() float64
	Read() Sample
	Close()
}

// Open ouvre la meilleure source disponible.
func Open() Source { return open() }

// NaN raccourci.
var NaN = math.NaN()

// Info résume la confiance dans les mesures, pour le rapport.
type Info struct {
	Level   Level    `json:"level"`
	Details []string `json:"details"`
	TjMax   float64  `json:"tjmax"`
}

// Describe construit l'Info d'une source.
func Describe(s Source) Info {
	return Info{Level: s.Level(), Details: s.Details(), TjMax: s.TjMax()}
}

type nullSource struct{}

func (nullSource) Level() Level      { return LevelNone }
func (nullSource) Details() []string { return nil }
func (nullSource) TjMax() float64    { return NaN }
func (nullSource) Read() Sample {
	return Sample{Time: time.Now(), CPUTempC: NaN, FreqMHz: NaN, PowerW: NaN}
}
func (nullSource) Close() {}
