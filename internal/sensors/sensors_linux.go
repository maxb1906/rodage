package sensors

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

func readTrim(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readFloat(p string) (float64, bool) {
	v, err := strconv.ParseFloat(readTrim(p), 64)
	return v, err == nil
}

type tempInput struct {
	path  string
	label string
	core  bool
}

type linuxSource struct {
	details   []string
	temps     []tempInput
	freqFiles []string
	thrFiles  []string
	lastThr   uint64
	tjmax     float64
	msr       *intelMSR
	msrFiles  []*os.File
}

// Pilotes hwmon reconnus comme capteurs CPU, par ordre de préférence.
var cpuHwmon = []string{"coretemp", "k10temp", "zenpower", "cpu_thermal", "cpu-thermal", "soc_thermal", "scpi_sensors", "apple_cpu"}

func open() Source {
	s := &linuxSource{tjmax: NaN}
	s.findHwmon()
	if len(s.temps) == 0 {
		s.findThermalZones()
	}
	cpus, _ := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq")
	sort.Strings(cpus)
	s.freqFiles = cpus
	if len(cpus) > 0 {
		s.details = append(s.details, "cpufreq (scaling_cur_freq)")
	}
	thr, _ := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/thermal_throttle/core_throttle_count")
	pkg, _ := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/thermal_throttle/package_throttle_count")
	s.thrFiles = append(thr, pkg...)
	if len(s.thrFiles) > 0 {
		s.lastThr = s.throttleCount()
		s.details = append(s.details, "thermal_throttle (compteurs noyau)")
	}
	s.openMSR()
	return s
}

func (s *linuxSource) findHwmon() {
	dirs, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	best := -1
	var bestDir, bestName string
	for _, d := range dirs {
		name := readTrim(d + "/name")
		for i, n := range cpuHwmon {
			if name == n && (best == -1 || i < best) {
				best, bestDir, bestName = i, d, name
			}
		}
	}
	if best == -1 {
		return
	}
	inputs, _ := filepath.Glob(bestDir + "/temp*_input")
	sort.Strings(inputs)
	for _, in := range inputs {
		base := strings.TrimSuffix(in, "_input")
		label := readTrim(base + "_label")
		if label == "" {
			label = filepath.Base(base)
		}
		s.temps = append(s.temps, tempInput{path: in, label: label, core: strings.HasPrefix(label, "Core")})
		if c, ok := readFloat(base + "_crit"); ok && c > 0 && (s.tjmax != s.tjmax || c/1000 > s.tjmax) {
			s.tjmax = c / 1000
		}
	}
	s.details = append(s.details, "hwmon "+bestName)
}

func (s *linuxSource) findThermalZones() {
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	for _, z := range zones {
		typ := readTrim(z + "/type")
		lt := strings.ToLower(typ)
		if strings.Contains(lt, "cpu") || strings.Contains(lt, "x86_pkg") || strings.Contains(lt, "soc") || lt == "acpitz" {
			s.temps = append(s.temps, tempInput{path: z + "/temp", label: typ})
		}
	}
	if len(s.temps) > 0 {
		s.details = append(s.details, "zones thermiques ACPI/sysfs")
	}
}

type devMSR struct{ files []*os.File }

func (d devMSR) readMSR(cpu int, reg uint32) (uint64, error) {
	if cpu >= len(d.files) || d.files[cpu] == nil {
		return 0, fmt.Errorf("msr cpu %d indisponible", cpu)
	}
	var b [8]byte
	if _, err := d.files[cpu].ReadAt(b[:], int64(reg)); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b[:]), nil
}

func (s *linuxSource) openMSR() {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "386" {
		return
	}
	if !strings.Contains(readTrim("/proc/cpuinfo"), "GenuineIntel") {
		return
	}
	n := runtime.NumCPU()
	files := make([]*os.File, n)
	for i := 0; i < n; i++ {
		f, err := os.Open(fmt.Sprintf("/dev/cpu/%d/msr", i))
		if err != nil {
			if i == 0 {
				return
			}
			continue
		}
		files[i] = f
	}
	base := 0.0
	if v, ok := readFloat("/sys/devices/system/cpu/cpu0/cpufreq/base_frequency"); ok {
		base = v / 1000
	}
	m, ok := newIntelMSR(devMSR{files}, n, base)
	if !ok {
		for _, f := range files {
			if f != nil {
				f.Close()
			}
		}
		return
	}
	s.msr, s.msrFiles = m, files
	s.tjmax = m.tjmax
	s.details = append(s.details, "MSR (/dev/cpu/*/msr) : températures par cœur, causes de throttling, puissance")
}

func (s *linuxSource) Level() Level {
	if len(s.temps) == 0 && len(s.freqFiles) == 0 && s.msr == nil {
		return LevelNone
	}
	return LevelLinux
}

func (s *linuxSource) Details() []string { return s.details }
func (s *linuxSource) TjMax() float64 {
	if s.tjmax != s.tjmax {
		return 100
	}
	return s.tjmax
}

func (s *linuxSource) throttleCount() uint64 {
	var n uint64
	for _, f := range s.thrFiles {
		v, _ := strconv.ParseUint(readTrim(f), 10, 64)
		n += v
	}
	return n
}

func (s *linuxSource) Read() Sample {
	now := time.Now()
	sm := Sample{Time: now, CPUTempC: NaN, FreqMHz: NaN, PowerW: NaN}
	for _, t := range s.temps {
		v, ok := readFloat(t.path)
		if !ok {
			continue
		}
		v /= 1000
		if v <= -50 || v > 200 {
			continue
		}
		if t.core {
			sm.CoreTempsC = append(sm.CoreTempsC, v)
		}
		if sm.CPUTempC != sm.CPUTempC || v > sm.CPUTempC {
			sm.CPUTempC = v
		}
	}
	var fs []float64
	for _, f := range s.freqFiles {
		if v, ok := readFloat(f); ok {
			fs = append(fs, v/1000)
		}
	}
	if len(fs) > 0 {
		sm.CoreFreq = fs
		sm.FreqMHz = mean(fs)
	}
	if s.msr != nil {
		hw := sm.CPUTempC
		cores := sm.CoreTempsC
		s.msr.read(&sm, now.UnixNano())
		if sm.CPUTempC != sm.CPUTempC {
			sm.CPUTempC = hw
		}
		if len(sm.CoreTempsC) == 0 {
			sm.CoreTempsC = cores
		}
	}
	if len(s.thrFiles) > 0 {
		c := s.throttleCount()
		if c > s.lastThr {
			sm.Throttle.CountDelta = c - s.lastThr
			sm.Throttle.Thermal = true
		}
		s.lastThr = c
	}
	return sm
}

func (s *linuxSource) Close() {
	for _, f := range s.msrFiles {
		if f != nil {
			f.Close()
		}
	}
}
