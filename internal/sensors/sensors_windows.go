package sensors

import (
	"math"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ----- PDH (compteurs de performance, sans pilote) -----

var (
	pdh                             = windows.NewLazySystemDLL("pdh.dll")
	procPdhOpenQuery                = pdh.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounter        = pdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData         = pdh.NewProc("PdhCollectQueryData")
	procPdhGetFormattedCounterValue = pdh.NewProc("PdhGetFormattedCounterValue")
	procPdhGetFormattedCounterArray = pdh.NewProc("PdhGetFormattedCounterArrayW")
	procPdhCloseQuery               = pdh.NewProc("PdhCloseQuery")
)

const (
	pdhFmtDouble   = 0x00000200
	pdhMoreData    = 0x800007D2
	pdhFmtNoCap100 = 0x00008000
)

type pdhFmtCounterValueDouble struct {
	CStatus     uint32
	_           uint32
	DoubleValue float64
}

type pdhFmtCounterValueItemDouble struct {
	SzName   *uint16
	FmtValue pdhFmtCounterValueDouble
}

type pdhQuery struct {
	h        uintptr
	counters map[string]uintptr
}

func newPDH() (*pdhQuery, bool) {
	if pdh.Load() != nil {
		return nil, false
	}
	q := &pdhQuery{counters: map[string]uintptr{}}
	if r, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&q.h))); r != 0 {
		return nil, false
	}
	return q, true
}

func (q *pdhQuery) add(path string) bool {
	p, _ := windows.UTF16PtrFromString(path)
	var c uintptr
	if r, _, _ := procPdhAddEnglishCounter.Call(q.h, uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&c))); r != 0 {
		return false
	}
	q.counters[path] = c
	return true
}

func (q *pdhQuery) collect() { procPdhCollectQueryData.Call(q.h) }

func (q *pdhQuery) value(path string) (float64, bool) {
	c, ok := q.counters[path]
	if !ok {
		return NaN, false
	}
	var v pdhFmtCounterValueDouble
	r, _, _ := procPdhGetFormattedCounterValue.Call(c, pdhFmtDouble|pdhFmtNoCap100, 0, uintptr(unsafe.Pointer(&v)))
	if r != 0 || v.CStatus > 1 {
		return NaN, false
	}
	return v.DoubleValue, true
}

func (q *pdhQuery) array(path string) map[string]float64 {
	c, ok := q.counters[path]
	if !ok {
		return nil
	}
	var size, count uint32
	r, _, _ := procPdhGetFormattedCounterArray.Call(c, pdhFmtDouble|pdhFmtNoCap100, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if uint32(r) != pdhMoreData || size == 0 {
		return nil
	}
	buf := make([]byte, size)
	r, _, _ = procPdhGetFormattedCounterArray.Call(c, pdhFmtDouble|pdhFmtNoCap100, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 {
		return nil
	}
	items := unsafe.Slice((*pdhFmtCounterValueItemDouble)(unsafe.Pointer(&buf[0])), count)
	out := make(map[string]float64, count)
	for _, it := range items {
		if it.FmtValue.CStatus > 1 {
			continue
		}
		out[windows.UTF16PtrToString(it.SzName)] = it.FmtValue.DoubleValue
	}
	return out
}

func (q *pdhQuery) close() { procPdhCloseQuery.Call(q.h) }

const (
	ctrPerf     = `\Processor Information(_Total)\% Processor Performance`
	ctrFreq     = `\Processor Information(_Total)\Processor Frequency`
	ctrLimit    = `\Processor Information(_Total)\% Performance Limit`
	ctrCorePerf = `\Processor Information(*)\% Processor Performance`
	ctrZones    = `\Thermal Zone Information(*)\High Precision Temperature`
	ctrZonesLow = `\Thermal Zone Information(*)\Temperature`
)

type winSource struct {
	q       *pdhQuery
	details []string
	zoneCtr string
	zoneDiv float64
	pawn    *pawnIO
	msr     *intelMSR
	tjmax   float64
	nominal float64
}

func open() Source {
	s := &winSource{tjmax: NaN}
	if q, ok := newPDH(); ok {
		s.q = q
		okPerf := q.add(ctrPerf) && q.add(ctrFreq)
		q.add(ctrLimit)
		q.add(ctrCorePerf)
		if q.add(ctrZones) {
			s.zoneCtr, s.zoneDiv = ctrZones, 10
		} else if q.add(ctrZonesLow) {
			s.zoneCtr, s.zoneDiv = ctrZonesLow, 1
		}
		q.collect()
		if okPerf {
			s.details = append(s.details, "compteurs de performance Windows (fréquence effective, % Performance Limit)")
		}
		if s.zoneCtr != "" {
			s.details = append(s.details, "zones thermiques ACPI (Thermal Zone Information)")
		}
	}
	s.openPawnIO()
	return s
}

func (s *winSource) openPawnIO() {
	p, err := openPawnIO()
	if err != nil {
		return
	}
	if !strings.Contains(cpuVendor(), "Intel") {
		s.details = append(s.details, "PawnIO présent ; module MSR Intel non applicable à ce processeur")
		p.close()
		return
	}
	if err := p.loadModule("IntelMSR.bin"); err != nil {
		s.details = append(s.details, "PawnIO présent mais module IntelMSR.bin introuvable ("+err.Error()+")")
		p.close()
		return
	}
	nominal := 0.0
	if s.q != nil {
		nominal, _ = s.q.value(ctrFreq)
	}
	m, ok := newIntelMSR(pawnMSR{p}, runtime.NumCPU(), nominal)
	if !ok {
		s.details = append(s.details, "PawnIO présent mais lecture MSR refusée")
		p.close()
		return
	}
	s.pawn, s.msr, s.tjmax = p, m, m.tjmax
	s.details = append(s.details, "PawnIO : MSR, températures par cœur, causes de throttling, puissance")
}

func (s *winSource) Level() Level {
	switch {
	case s.msr != nil:
		return LevelPawnIO
	case s.q != nil:
		return LevelNoDrv
	}
	return LevelNone
}

func (s *winSource) Details() []string { return s.details }

func (s *winSource) TjMax() float64 {
	if s.tjmax != s.tjmax {
		return 100
	}
	return s.tjmax
}

func (s *winSource) Read() Sample {
	now := time.Now()
	sm := Sample{Time: now, CPUTempC: NaN, FreqMHz: NaN, PowerW: NaN}
	if s.q != nil {
		s.q.collect()
		perf, ok1 := s.q.value(ctrPerf)
		freq, ok2 := s.q.value(ctrFreq)
		if ok1 && ok2 && freq > 0 {
			sm.FreqMHz = freq * perf / 100
		}
		if lim, ok := s.q.value(ctrLimit); ok && lim > 0 {
			sm.Throttle.LimitPct = lim
		}
		if ok2 && freq > 0 {
			for name, v := range s.q.array(ctrCorePerf) {
				if strings.Contains(name, "_Total") {
					continue
				}
				sm.CoreFreq = append(sm.CoreFreq, freq*v/100)
			}
		}
		if s.zoneCtr != "" {
			for _, k := range s.q.array(s.zoneCtr) {
				c := k/s.zoneDiv - 273.15
				if c > -50 && c < 200 && (sm.CPUTempC != sm.CPUTempC || c > sm.CPUTempC) {
					sm.CPUTempC = c
				}
			}
		}
	}
	if s.msr != nil {
		acpi := sm.CPUTempC
		s.msr.read(&sm, now.UnixNano())
		if sm.CPUTempC != sm.CPUTempC {
			sm.CPUTempC = acpi
		}
	}
	if math.IsInf(sm.FreqMHz, 0) {
		sm.FreqMHz = NaN
	}
	return sm
}

func (s *winSource) Close() {
	if s.q != nil {
		s.q.close()
	}
	if s.pawn != nil {
		s.pawn.close()
	}
}
