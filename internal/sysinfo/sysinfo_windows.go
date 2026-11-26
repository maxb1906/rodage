package sysinfo

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	kernel32                   = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx   = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetSystemFirmwareTable = kernel32.NewProc("GetSystemFirmwareTable")
	procGetSystemPowerStatus   = kernel32.NewProc("GetSystemPowerStatus")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// IsAdmin indique si le processus est élevé.
func IsAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func memStatus() (memoryStatusEx, bool) {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	return m, r != 0
}

func memAvailable() uint64 {
	m, _ := memStatus()
	return m.AvailPhys
}

func collectPlatform(in *Info) {
	v := windows.RtlGetVersion()
	in.OSVersion = fmt.Sprintf("Windows %d.%d build %d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE); err == nil {
		if name, _, err := k.GetStringValue("ProductName"); err == nil {
			if v.BuildNumber >= 22000 {
				name = strings.Replace(name, "Windows 10", "Windows 11", 1)
			}
			in.OSVersion = fmt.Sprintf("%s (build %d)", name, v.BuildNumber)
		}
		k.Close()
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		in.CPUModel, _, _ = k.GetStringValue("ProcessorNameString")
		in.CPUVendor, _, _ = k.GetStringValue("VendorIdentifier")
		k.Close()
	}
	if m, ok := memStatus(); ok {
		in.RAMTotal, in.RAMAvailable = m.TotalPhys, m.AvailPhys
	}
	in.SMBIOS = readSMBIOS()
	in.HasBattery = hasBattery()
}

func readSMBIOS() SMBIOS {
	const rsmb = 'R'<<24 | 'S'<<16 | 'M'<<8 | 'B'
	n, _, _ := procGetSystemFirmwareTable.Call(rsmb, 0, 0, 0)
	if n == 0 {
		return SMBIOS{}
	}
	buf := make([]byte, n)
	r, _, _ := procGetSystemFirmwareTable.Call(rsmb, 0, uintptr(unsafe.Pointer(&buf[0])), n)
	if r == 0 || len(buf) < 8 {
		return SMBIOS{}
	}
	// En-tête RawSMBIOSData : 4 octets de version puis la longueur (uint32).
	ln := int(uint32(buf[4]) | uint32(buf[5])<<8 | uint32(buf[6])<<16 | uint32(buf[7])<<24)
	tbl := buf[8:]
	if ln < len(tbl) {
		tbl = tbl[:ln]
	}
	return parseSMBIOS(tbl)
}

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

// PowerStatus renvoie l'état secteur/batterie (utilisé par la sûreté).
func PowerStatus() (onAC bool, batteryPct int, hasBattery bool, ok bool) {
	var s systemPowerStatus
	r, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&s)))
	if r == 0 {
		return false, 0, false, false
	}
	has := s.BatteryFlag != 128 && s.BatteryFlag != 255
	pct := int(s.BatteryLifePercent)
	if pct == 255 {
		pct = -1
	}
	return s.ACLineStatus == 1, pct, has, true
}

func hasBattery() bool {
	_, _, has, _ := PowerStatus()
	return has
}
