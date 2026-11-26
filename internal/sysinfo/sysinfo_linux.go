package sysinfo

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func readTrim(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// IsAdmin indique si le processus tourne en root.
func IsAdmin() bool { return os.Geteuid() == 0 }

func collectPlatform(in *Info) {
	in.OSVersion = osRelease()
	var u unix.Utsname
	if unix.Uname(&u) == nil {
		in.OSVersion += " (noyau " + unix.ByteSliceToString(u.Release[:]) + ")"
	}
	in.CPUModel, in.CPUVendor = cpuModel()
	in.RAMTotal, in.RAMAvailable = meminfo()
	d := "/sys/class/dmi/id/"
	in.SMBIOS = SMBIOS{
		Manufacturer: clean(readTrim(d + "sys_vendor")),
		Product:      clean(readTrim(d + "product_name")),
		Serial:       clean(readTrim(d + "product_serial")),
		BoardVendor:  clean(readTrim(d + "board_vendor")),
		BoardProduct: clean(readTrim(d + "board_name")),
		BoardSerial:  clean(readTrim(d + "board_serial")),
		BIOSVendor:   clean(readTrim(d + "bios_vendor")),
		BIOSVersion:  clean(readTrim(d + "bios_version")),
		BIOSDate:     clean(readTrim(d + "bios_date")),
	}
	// Table brute (lisible en root) : plus complète que sysfs sur certaines machines.
	if raw, err := os.ReadFile("/sys/firmware/dmi/tables/DMI"); err == nil {
		t := parseSMBIOS(raw)
		if in.SMBIOS.Serial == "" {
			in.SMBIOS.Serial = t.Serial
		}
		if in.SMBIOS.BoardSerial == "" {
			in.SMBIOS.BoardSerial = t.BoardSerial
		}
	}
	bats, _ := filepath.Glob("/sys/class/power_supply/BAT*")
	in.HasBattery = len(bats) > 0
}

func osRelease() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "Linux"
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return "Linux"
}

// Quelques cœurs ARM courants (identifiant « CPU part » de /proc/cpuinfo).
var armParts = map[string]string{
	"0xd03": "Cortex-A53", "0xd04": "Cortex-A35", "0xd05": "Cortex-A55",
	"0xd07": "Cortex-A57", "0xd08": "Cortex-A72", "0xd09": "Cortex-A73",
	"0xd0a": "Cortex-A75", "0xd0b": "Cortex-A76", "0xd0c": "Neoverse-N1",
	"0xd0d": "Cortex-A77", "0xd40": "Neoverse-V1", "0xd41": "Cortex-A78",
	"0xd44": "Cortex-X1", "0xd46": "Cortex-A510", "0xd47": "Cortex-A710",
	"0xd48": "Cortex-X2", "0xd49": "Neoverse-N2", "0xd4f": "Neoverse-V2",
	"0xd4d": "Cortex-A715", "0xd4e": "Cortex-X3", "0xd80": "Cortex-A520",
	"0xd81": "Cortex-A720", "0xd82": "Cortex-X4",
}

var armImpl = map[string]string{
	"0x41": "ARM", "0x42": "Broadcom", "0x43": "Cavium", "0x48": "HiSilicon",
	"0x4e": "NVIDIA", "0x51": "Qualcomm", "0x61": "Apple", "0xc0": "Ampere",
}

func cpuModel() (model, vendor string) {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "", ""
	}
	defer f.Close()
	var impl, part string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "model name":
			if model == "" {
				model = v
			}
		case "vendor_id":
			if vendor == "" {
				vendor = v
			}
		case "CPU implementer":
			if impl == "" {
				impl = v
			}
		case "CPU part":
			if part == "" {
				part = v
			}
		}
	}
	if model == "" && part != "" {
		vendor = armImpl[impl]
		name := armParts[part]
		if name == "" {
			name = "ARM part " + part
		}
		model = strings.TrimSpace(vendor + " " + name)
		// Le nom commercial figure parfois dans le device-tree ou le DMI.
		if dt := strings.Trim(readTrim("/proc/device-tree/model"), "\x00"); dt != "" {
			model += " (" + dt + ")"
		}
	}
	return model, vendor
}

func meminfo() (total, avail uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(fs[1], 10, 64)
		switch fs[0] {
		case "MemTotal:":
			total = v * 1024
		case "MemAvailable:":
			avail = v * 1024
		}
	}
	return total, avail
}

func memAvailable() uint64 {
	_, a := meminfo()
	return a
}

// PowerStatus renvoie l'état secteur/batterie (utilisé par la sûreté).
func PowerStatus() (onAC bool, batteryPct int, hasBattery bool, ok bool) {
	supplies, _ := filepath.Glob("/sys/class/power_supply/*")
	batteryPct = -1
	sawMains := false
	discharging := false
	for _, s := range supplies {
		switch readTrim(s + "/type") {
		case "Mains":
			sawMains = true
			if readTrim(s+"/online") == "1" {
				onAC = true
			}
		case "Battery":
			hasBattery = true
			if v, err := strconv.Atoi(readTrim(s + "/capacity")); err == nil {
				batteryPct = v
			}
			discharging = discharging || readTrim(s+"/status") == "Discharging"
		}
	}
	if !sawMains && !hasBattery {
		return true, -1, false, false
	}
	if !sawMains {
		// Pas d'information secteur : on se fie à l'état de charge.
		onAC = !discharging
	}
	return onAC, batteryPct, hasBattery, true
}
