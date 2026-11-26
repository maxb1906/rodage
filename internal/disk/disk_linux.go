package disk

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

func readTrim(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// openFile ouvre le fichier de test en E/S directes (O_DIRECT | O_DSYNC).
func openFile(path string, create bool) (target, bool, error) {
	flags := os.O_RDWR | syscall.O_DIRECT | syscall.O_DSYNC
	if create {
		flags |= os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err == nil {
		return f, true, nil
	}
	if !errors.Is(err, syscall.EINVAL) {
		return nil, false, err
	}
	f, err = os.OpenFile(path, flags&^syscall.O_DIRECT, 0o600)
	if err != nil {
		return nil, false, err
	}
	return f, false, nil
}

// openRaw ouvre le périphérique en exclusivité (échoue s'il est monté).
func openRaw(d Device) (target, error) {
	return os.OpenFile(d.Path, os.O_RDWR|syscall.O_DIRECT|syscall.O_DSYNC|syscall.O_EXCL, 0)
}

func freeSpace(dir string) (uint64, error) {
	var s unix.Statfs_t
	if err := unix.Statfs(dir, &s); err != nil {
		return 0, err
	}
	return s.Bavail * uint64(s.Bsize), nil
}

var skipPrefixes = []string{"loop", "ram", "zram", "dm-", "md", "sr", "fd", "nbd", "mtdblock"}

// ListDevices liste les disques physiques.
func ListDevices() ([]Device, error) {
	ents, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil, err
	}
	sys := systemDisks()
	mounts := mountedDevices()
	var out []Device
	for _, e := range ents {
		n := e.Name()
		skip := false
		for _, p := range skipPrefixes {
			if strings.HasPrefix(n, p) {
				skip = true
			}
		}
		if skip {
			continue
		}
		d := deviceInfo(n)
		d.System = sys[n]
		d.Mounted = mounts[n]
		out = append(out, d)
	}
	return out, nil
}

func deviceInfo(name string) Device {
	base := "/sys/block/" + name
	d := Device{Name: name, Path: "/dev/" + name}
	if v, err := strconv.ParseUint(readTrim(base+"/size"), 10, 64); err == nil {
		d.Size = v * 512
	}
	d.Model = readTrim(base + "/device/model")
	d.Serial = readTrim(base + "/device/serial")
	d.Firmware = readTrim(base + "/device/firmware_rev")
	if d.Firmware == "" {
		d.Firmware = readTrim(base + "/device/rev")
	}
	d.Rotational = readTrim(base+"/queue/rotational") == "1"
	d.Removable = readTrim(base+"/removable") == "1"
	if dev := readTrim(base + "/dev"); dev != "" {
		if f, err := os.Open("/run/udev/data/b" + dev); err == nil {
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				if v, ok := strings.CutPrefix(sc.Text(), "E:ID_SERIAL_SHORT="); ok && d.Serial == "" {
					d.Serial = v
				}
				if v, ok := strings.CutPrefix(sc.Text(), "E:ID_MODEL="); ok && d.Model == "" {
					d.Model = strings.ReplaceAll(v, "_", " ")
				}
			}
			f.Close()
		}
	}
	real, _ := filepath.EvalSymlinks(base)
	switch {
	case strings.HasPrefix(name, "nvme"):
		d.Bus = "nvme"
	case strings.Contains(real, "/usb"):
		d.Bus = "usb"
	case strings.HasPrefix(name, "mmcblk"):
		d.Bus = "mmc"
	case strings.TrimSpace(readTrim(base+"/device/vendor")) == "ATA":
		d.Bus = "sata"
	case strings.HasPrefix(name, "vd"):
		d.Bus = "virtio"
	default:
		d.Bus = "scsi"
	}
	parts, _ := filepath.Glob(base + "/" + name + "*/partition")
	d.Partitions = len(parts)
	return d
}

// diskOf remonte d'un périphérique bloc (partition, LVM, LUKS…) au disque.
func diskOf(majmin string, depth int) string {
	p, err := filepath.EvalSymlinks("/sys/dev/block/" + majmin)
	if err != nil || depth > 8 {
		return ""
	}
	if _, err := os.Stat(p + "/partition"); err == nil {
		return filepath.Base(filepath.Dir(p))
	}
	if slaves, _ := os.ReadDir(p + "/slaves"); len(slaves) > 0 {
		if mm := readTrim(p + "/slaves/" + slaves[0].Name() + "/dev"); mm != "" {
			return diskOf(mm, depth+1)
		}
	}
	return filepath.Base(p)
}

func diskOfPath(path string) string {
	var st unix.Stat_t
	if unix.Stat(path, &st) != nil {
		return ""
	}
	return diskOf(fmt.Sprintf("%d:%d", unix.Major(uint64(st.Dev)), unix.Minor(uint64(st.Dev))), 0)
}

func systemDisks() map[string]bool {
	m := map[string]bool{}
	for _, p := range []string{"/", "/boot", "/boot/efi", "/usr", "/var"} {
		if d := diskOfPath(p); d != "" {
			m[d] = true
		}
	}
	return m
}

func mountedDevices() map[string]bool {
	m := map[string]bool{}
	for _, file := range []string{"/proc/mounts", "/proc/swaps"} {
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			dev := strings.Fields(sc.Text())
			if len(dev) == 0 || !strings.HasPrefix(dev[0], "/dev/") {
				continue
			}
			var st unix.Stat_t
			if unix.Stat(dev[0], &st) == nil {
				if d := diskOf(fmt.Sprintf("%d:%d", unix.Major(uint64(st.Rdev)), unix.Minor(uint64(st.Rdev))), 0); d != "" {
					m[d] = true
				}
			}
		}
		f.Close()
	}
	// Disques utilisés par LVM/RAID/LUKS (holders non vides).
	ents, _ := os.ReadDir("/sys/block")
	for _, e := range ents {
		hs, _ := filepath.Glob("/sys/block/" + e.Name() + "/holders/*")
		hp, _ := filepath.Glob("/sys/block/" + e.Name() + "/" + e.Name() + "*/holders/*")
		if len(hs)+len(hp) > 0 {
			m[e.Name()] = true
		}
	}
	return m
}

// DeviceForPath renvoie le disque physique qui porte path.
func DeviceForPath(path string) (Device, error) {
	n := diskOfPath(path)
	if n == "" {
		return Device{}, fmt.Errorf("disque de %s introuvable", path)
	}
	if _, err := os.Stat("/sys/block/" + n); err != nil {
		return Device{}, err
	}
	d := deviceInfo(n)
	d.System = systemDisks()[n]
	d.Mounted = true
	return d, nil
}

// ----- SMART / santé NVMe (lecture seule, droits root) -----

type nvmeAdminCmd struct {
	Opcode      uint8
	Flags       uint8
	Rsvd1       uint16
	NSID        uint32
	Cdw2, Cdw3  uint32
	Metadata    uint64
	Addr        uint64
	MetadataLen uint32
	DataLen     uint32
	Cdw10       uint32
	Cdw11       uint32
	Cdw12       uint32
	Cdw13       uint32
	Cdw14       uint32
	Cdw15       uint32
	TimeoutMs   uint32
	Result      uint32
}

const nvmeIoctlAdminCmd = 0xC0484E41

type sgIOHdr struct {
	InterfaceID    int32
	DxferDirection int32
	CmdLen         uint8
	MxSbLen        uint8
	IovecCount     uint16
	DxferLen       uint32
	Dxferp         uintptr
	Cmdp           uintptr
	Sbp            uintptr
	Timeout        uint32
	Flags          uint32
	PackID         int32
	_              int32
	UsrPtr         uintptr
	Status         uint8
	MaskedStatus   uint8
	MsgStatus      uint8
	SbLenWr        uint8
	HostStatus     uint16
	DriverStatus   uint16
	Resid          int32
	Duration       uint32
	Info           uint32
	_              uint32
}

const sgIO = 0x2285

// ReadHealth lit l'état SMART ou NVMe d'un disque (lecture seule).
func ReadHealth(d Device) Health {
	h := unknownHealth()
	if strings.HasPrefix(d.Name, "nvme") {
		ctrl := d.Name
		if i := strings.Index(ctrl[4:], "n"); i >= 0 {
			ctrl = ctrl[:4+i]
		}
		f, err := os.Open("/dev/" + ctrl)
		if err != nil {
			h.Note = "lecture santé NVMe impossible : " + permNote(err)
			return h
		}
		defer f.Close()
		buf := alignedBuf(512)
		cmd := nvmeAdminCmd{Opcode: 0x02, NSID: 0xFFFFFFFF, Addr: uint64(uintptr(unsafe.Pointer(&buf[0]))), DataLen: 512, Cdw10: (512/4-1)<<16 | 0x02}
		if _, _, e := unix.Syscall(unix.SYS_IOCTL, f.Fd(), nvmeIoctlAdminCmd, uintptr(unsafe.Pointer(&cmd))); e != 0 {
			h.Note = "commande NVMe refusée : " + e.Error()
			return h
		}
		runtime.KeepAlive(buf)
		return parseNVMeHealth(buf)
	}
	f, err := os.OpenFile(d.Path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		h.Note = "lecture SMART impossible : " + permNote(err)
		return h
	}
	defer f.Close()
	buf := alignedBuf(512)
	cdb := [16]byte{0x85, 0x08, 0x0E, 0, 0xD0, 0, 1, 0, 0, 0, 0x4F, 0, 0xC2, 0, 0xB0, 0}
	var sense [32]byte
	hdr := sgIOHdr{InterfaceID: 'S', DxferDirection: -3, CmdLen: 16, MxSbLen: 32, DxferLen: 512,
		Dxferp: uintptr(unsafe.Pointer(&buf[0])), Cmdp: uintptr(unsafe.Pointer(&cdb[0])), Sbp: uintptr(unsafe.Pointer(&sense[0])), Timeout: 10000}
	if _, _, e := unix.Syscall(unix.SYS_IOCTL, f.Fd(), sgIO, uintptr(unsafe.Pointer(&hdr))); e != 0 {
		h.Note = "SMART refusé : " + e.Error()
		return h
	}
	runtime.KeepAlive(buf)
	runtime.KeepAlive(&cdb)
	runtime.KeepAlive(&sense)
	if hdr.Status != 0 && hdr.Status != 2 || hdr.HostStatus != 0 {
		h.Note = "SMART non pris en charge par ce disque ou cet adaptateur"
		return h
	}
	return parseATASmart(buf)
}

func permNote(err error) string {
	if errors.Is(err, os.ErrPermission) {
		return "droits administrateur requis"
	}
	return err.Error()
}
