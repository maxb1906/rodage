package sensors

// Accès MSR via PawnIO (https://pawnio.eu), pilote signé installé séparément.
// Rodage n'embarque aucun pilote : si PawnIO et son module IntelMSR.bin sont
// présents, les mesures passent au niveau 2.

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	pawnDeviceType = 41394
	ioctlLoad      = pawnDeviceType<<16 | 0x821<<2 // METHOD_BUFFERED, FILE_ANY_ACCESS
	ioctlExecute   = pawnDeviceType<<16 | 0x841<<2
	pawnFnNameLen  = 32
)

type pawnIO struct{ h windows.Handle }

func openPawnIO() (*pawnIO, error) {
	path, _ := windows.UTF16PtrFromString(`\\?\GLOBALROOT\Device\PawnIO`)
	h, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, err
	}
	return &pawnIO{h: h}, nil
}

// moduleDirs liste les emplacements où chercher les modules PawnIO.
func moduleDirs() []string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe), filepath.Join(filepath.Dir(exe), "pawnio"))
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\PawnIO`, registry.QUERY_VALUE); err == nil {
		if loc, _, err := k.GetStringValue("InstallLocation"); err == nil {
			dirs = append(dirs, loc, filepath.Join(loc, "modules"))
		}
		k.Close()
	}
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		dirs = append(dirs, filepath.Join(pf, "PawnIO"), filepath.Join(pf, "PawnIO", "modules"))
	}
	return dirs
}

func (p *pawnIO) loadModule(name string) error {
	for _, d := range moduleDirs() {
		blob, err := os.ReadFile(filepath.Join(d, name))
		if err != nil {
			continue
		}
		var ret uint32
		return windows.DeviceIoControl(p.h, ioctlLoad, &blob[0], uint32(len(blob)), nil, 0, &ret, nil)
	}
	return errors.New(name + " absent")
}

func (p *pawnIO) execute(fn string, in []uint64, out []uint64) error {
	buf := make([]byte, pawnFnNameLen+8*len(in))
	copy(buf, fn)
	for i, v := range in {
		binary.LittleEndian.PutUint64(buf[pawnFnNameLen+8*i:], v)
	}
	var ret uint32
	var outp *byte
	if len(out) > 0 {
		outp = (*byte)(unsafe.Pointer(&out[0]))
	}
	return windows.DeviceIoControl(p.h, ioctlExecute, &buf[0], uint32(len(buf)), outp, uint32(8*len(out)), &ret, nil)
}

func (p *pawnIO) close() { windows.CloseHandle(p.h) }

// pawnMSR lit un MSR sur un cœur donné : le fil est épinglé le temps de l'appel.
type pawnMSR struct{ p *pawnIO }

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procSetThreadAffinityMask = kernel32.NewProc("SetThreadAffinityMask")
	procGetCurrentThread      = kernel32.NewProc("GetCurrentThread")
)

func (m pawnMSR) readMSR(cpu int, reg uint32) (uint64, error) {
	runtimeLock()
	defer runtimeUnlock()
	th, _, _ := procGetCurrentThread.Call()
	if cpu < 64 {
		old, _, _ := procSetThreadAffinityMask.Call(th, uintptr(1)<<uint(cpu))
		if old != 0 {
			defer procSetThreadAffinityMask.Call(th, old)
		}
	}
	out := make([]uint64, 1)
	if err := m.p.execute("ioctl_read_msr", []uint64{uint64(reg)}, out); err != nil {
		return 0, err
	}
	return out[0], nil
}

func cpuVendor() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, _ := k.GetStringValue("VendorIdentifier")
	if v == "GenuineIntel" {
		return "Intel"
	}
	return v
}
