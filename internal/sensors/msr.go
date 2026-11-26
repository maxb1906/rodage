package sensors

// Registres MSR Intel utilisés (communs aux niveaux 2 et 3).
const (
	msrThermStatus     = 0x19C // IA32_THERM_STATUS (par cœur)
	msrTempTarget      = 0x1A2 // MSR_TEMPERATURE_TARGET (TjMax en bits 23:16)
	msrPkgThermStatus  = 0x1B1 // IA32_PACKAGE_THERM_STATUS
	msrRaplPowerUnit   = 0x606
	msrPkgEnergyStatus = 0x611
	msrCorePerfLimit   = 0x64F // MSR_CORE_PERF_LIMIT_REASONS (Intel client)
	msrAPERF           = 0xE8
	msrMPERF           = 0xE7
)

// decodePerfLimit lit MSR_CORE_PERF_LIMIT_REASONS (bits d'état 0..15).
func decodePerfLimit(v uint64) Throttle {
	var t Throttle
	t.Prochot = v&(1<<0) != 0
	t.Thermal = v&(1<<1) != 0 || v&(1<<5) != 0
	t.VRM = v&(1<<6) != 0 || v&(1<<7) != 0 || v&(1<<8) != 0
	t.PL1 = v&(1<<10) != 0
	t.PL2 = v&(1<<11) != 0
	t.Other = v&(1<<4) != 0 || v&(1<<9) != 0 || v&(1<<12) != 0 || v&(1<<13) != 0
	return t
}

// decodeThermStatus renvoie la température (°C) à partir de IA32_THERM_STATUS.
func decodeThermStatus(v uint64, tjmax float64) (temp float64, valid bool, thermal, prochot bool) {
	thermal = v&1 != 0
	prochot = v&(1<<2) != 0
	if v&(1<<31) == 0 {
		return NaN, false, thermal, prochot
	}
	readout := float64((v >> 16) & 0x7F)
	return tjmax - readout, true, thermal, prochot
}

// msrReader lit un MSR sur un cœur logique.
type msrReader interface {
	readMSR(cpu int, reg uint32) (uint64, error)
}

// intelMSR regroupe la logique MSR indépendante de l'OS.
type intelMSR struct {
	r          msrReader
	cpus       int
	tjmax      float64
	energyUnit float64
	lastEnergy uint64
	lastT      int64
	lastAPERF  []uint64
	lastMPERF  []uint64
	baseMHz    float64
}

func newIntelMSR(r msrReader, cpus int, baseMHz float64) (*intelMSR, bool) {
	tt, err := r.readMSR(0, msrTempTarget)
	if err != nil {
		return nil, false
	}
	m := &intelMSR{r: r, cpus: cpus, baseMHz: baseMHz}
	m.tjmax = float64((tt >> 16) & 0xFF)
	if m.tjmax < 50 || m.tjmax > 130 {
		m.tjmax = 100
	}
	if u, err := r.readMSR(0, msrRaplPowerUnit); err == nil {
		m.energyUnit = 1 / float64(uint64(1)<<((u>>8)&0x1F))
	}
	m.lastAPERF = make([]uint64, cpus)
	m.lastMPERF = make([]uint64, cpus)
	return m, true
}

func (m *intelMSR) read(s *Sample, nowNanos int64) {
	s.CoreTempsC = make([]float64, 0, m.cpus)
	maxT := NaN
	var freqs []float64
	for c := 0; c < m.cpus; c++ {
		if v, err := m.r.readMSR(c, msrThermStatus); err == nil {
			t, ok, th, ph := decodeThermStatus(v, m.tjmax)
			if ok {
				s.CoreTempsC = append(s.CoreTempsC, t)
				if maxT != maxT || t > maxT {
					maxT = t
				}
			}
			s.Throttle.Thermal = s.Throttle.Thermal || th
			s.Throttle.Prochot = s.Throttle.Prochot || ph
		}
		if m.baseMHz > 0 {
			a, ea := m.r.readMSR(c, msrAPERF)
			mp, em := m.r.readMSR(c, msrMPERF)
			if ea == nil && em == nil {
				da, dm := a-m.lastAPERF[c], mp-m.lastMPERF[c]
				if m.lastMPERF[c] != 0 && dm > 0 {
					freqs = append(freqs, m.baseMHz*float64(da)/float64(dm))
				}
				m.lastAPERF[c], m.lastMPERF[c] = a, mp
			}
		}
	}
	if v, err := m.r.readMSR(0, msrPkgThermStatus); err == nil {
		if t, ok, th, ph := decodeThermStatus(v, m.tjmax); ok {
			if maxT != maxT || t > maxT {
				maxT = t
			}
			s.Throttle.Thermal = s.Throttle.Thermal || th
			s.Throttle.Prochot = s.Throttle.Prochot || ph
		}
	}
	if maxT == maxT {
		s.CPUTempC = maxT
	}
	if v, err := m.r.readMSR(0, msrCorePerfLimit); err == nil {
		s.Throttle.Merge(decodePerfLimit(v))
	}
	if m.energyUnit > 0 {
		if e, err := m.r.readMSR(0, msrPkgEnergyStatus); err == nil {
			e &= 0xFFFFFFFF
			if m.lastT != 0 {
				de := (e - m.lastEnergy) & 0xFFFFFFFF
				dt := float64(nowNanos-m.lastT) / 1e9
				if dt > 0 {
					s.PowerW = float64(de) * m.energyUnit / dt
				}
			}
			m.lastEnergy, m.lastT = e, nowNanos
		}
	}
	if len(freqs) > 0 {
		s.CoreFreq = freqs
		s.FreqMHz = mean(freqs)
	}
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return NaN
	}
	var s float64
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}
