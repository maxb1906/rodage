package disk

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

const (
	ioctlStorageQueryProperty     = 0x002D1400
	ioctlVolumeGetDiskExtents     = 0x00560000
	ioctlDiskGetDriveGeometryEx   = 0x000700A0
	ioctlDiskGetDriveLayoutEx     = 0x00070050
	smartRcvDriveData             = 0x0007C088
	fileFlagNoBuffering           = 0x20000000
	fileFlagWriteThrough          = 0x80000000
	storageDeviceProperty         = 0
	storageDeviceProtocolSpecific = 50
	protocolTypeNvme              = 3
	nvmeDataTypeLogPage           = 2
)

func createFile(path string, access uint32, disp uint32, flags uint32) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(p, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, disp, flags, 0)
}

// openFile ouvre le fichier de test en FILE_FLAG_NO_BUFFERING | WRITE_THROUGH.
func openFile(path string, create bool) (target, bool, error) {
	disp := uint32(windows.OPEN_EXISTING)
	if create {
		disp = windows.CREATE_ALWAYS
	}
	h, err := createFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, disp, fileFlagNoBuffering|fileFlagWriteThrough)
	if err != nil {
		return nil, false, err
	}
	return os.NewFile(uintptr(h), path), true, nil
}

func openRaw(d Device) (target, error) {
	h, err := createFile(d.Path, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.OPEN_EXISTING, fileFlagNoBuffering|fileFlagWriteThrough)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), d.Path), nil
}

func freeSpace(dir string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return free, nil
}

func ioctl(h windows.Handle, code uint32, in []byte, out []byte) (int, error) {
	var ret uint32
	var inp, outp *byte
	if len(in) > 0 {
		inp = &in[0]
	}
	if len(out) > 0 {
		outp = &out[0]
	}
	err := windows.DeviceIoControl(h, code, inp, uint32(len(in)), outp, uint32(len(out)), &ret, nil)
	return int(ret), err
}

func cstr(b []byte, off uint32) string {
	if off == 0 || int(off) >= len(b) {
		return ""
	}
	e := int(off)
	for e < len(b) && b[e] != 0 {
		e++
	}
	return strings.TrimSpace(string(b[off:e]))
}

var busNames = map[uint32]string{1: "scsi", 2: "atapi", 3: "ata", 7: "usb", 8: "raid", 10: "sas", 11: "sata", 12: "sd", 13: "mmc", 14: "virtual", 15: "virtual", 17: "nvme", 18: "scm", 19: "ufs"}

func diskNumberOf(path string) (int, error) {
	vol := make([]uint16, windows.MAX_PATH)
	p, _ := windows.UTF16PtrFromString(path)
	if err := windows.GetVolumePathName(p, &vol[0], uint32(len(vol))); err != nil {
		return -1, err
	}
	v := strings.TrimSuffix(windows.UTF16ToString(vol), `\`)
	h, err := createFile(`\\.\`+v, 0, windows.OPEN_EXISTING, 0)
	if err != nil {
		return -1, err
	}
	defer windows.CloseHandle(h)
	out := make([]byte, 1024)
	if _, err := ioctl(h, ioctlVolumeGetDiskExtents, nil, out); err != nil {
		return -1, err
	}
	if binary.LittleEndian.Uint32(out) == 0 {
		return -1, fmt.Errorf("aucune étendue disque pour %s", v)
	}
	return int(binary.LittleEndian.Uint32(out[8:])), nil
}

func deviceInfo(n int) (Device, error) {
	d := Device{Name: fmt.Sprintf("PhysicalDrive%d", n), Path: fmt.Sprintf(`\\.\PhysicalDrive%d`, n)}
	h, err := createFile(d.Path, 0, windows.OPEN_EXISTING, 0)
	if err != nil {
		return d, err
	}
	defer windows.CloseHandle(h)
	q := make([]byte, 12) // STORAGE_PROPERTY_QUERY : PropertyId, QueryType, AdditionalParameters[1]
	out := make([]byte, 4096)
	if _, err := ioctl(h, ioctlStorageQueryProperty, q, out); err == nil {
		d.Removable = out[10] != 0
		d.Model = strings.TrimSpace(cstr(out, binary.LittleEndian.Uint32(out[12:])) + " " + cstr(out, binary.LittleEndian.Uint32(out[16:])))
		d.Firmware = cstr(out, binary.LittleEndian.Uint32(out[20:]))
		d.Serial = cstr(out, binary.LittleEndian.Uint32(out[24:]))
		d.Bus = busNames[binary.LittleEndian.Uint32(out[28:])]
	}
	geo := make([]byte, 256)
	if _, err := ioctl(h, ioctlDiskGetDriveGeometryEx, nil, geo); err == nil {
		d.Size = binary.LittleEndian.Uint64(geo[24:])
	}
	lay := make([]byte, 48+144*128)
	if _, err := ioctl(h, ioctlDiskGetDriveLayoutEx, nil, lay); err == nil {
		count := int(binary.LittleEndian.Uint32(lay[4:]))
		for i := 0; i < count && 48+144*(i+1) <= len(lay); i++ {
			e := lay[48+144*i:]
			if binary.LittleEndian.Uint64(e[16:]) > 0 {
				d.Partitions++
			}
		}
	}
	return d, nil
}

func systemDisk() int {
	sd := os.Getenv("SystemDrive")
	if sd == "" {
		sd = "C:"
	}
	n, err := diskNumberOf(sd + `\`)
	if err != nil {
		return -1
	}
	return n
}

// ListDevices liste les disques physiques.
func ListDevices() ([]Device, error) {
	sys := systemDisk()
	var out []Device
	for n := 0; n < 32; n++ {
		d, err := deviceInfo(n)
		if err != nil {
			continue
		}
		d.System = n == sys
		d.Mounted = d.Partitions > 0
		out = append(out, d)
	}
	return out, nil
}

// DeviceForPath renvoie le disque physique qui porte path.
func DeviceForPath(path string) (Device, error) {
	n, err := diskNumberOf(path)
	if err != nil {
		return Device{}, err
	}
	d, err := deviceInfo(n)
	if err != nil {
		return d, err
	}
	d.System = n == systemDisk()
	d.Mounted = true
	return d, nil
}

// ReadHealth lit l'état SMART ou NVMe (IOCTL_STORAGE_QUERY_PROPERTY, sans pilote).
func ReadHealth(d Device) Health {
	h := unknownHealth()
	handle, err := createFile(d.Path, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.OPEN_EXISTING, 0)
	if err != nil {
		handle, err = createFile(d.Path, windows.GENERIC_READ, windows.OPEN_EXISTING, 0)
	}
	if err != nil {
		h.Note = "lecture santé impossible : droits administrateur requis"
		return h
	}
	defer windows.CloseHandle(handle)
	if d.Bus == "nvme" {
		// STORAGE_PROPERTY_QUERY (8 o) + STORAGE_PROTOCOL_SPECIFIC_DATA (40 o) + 512 o.
		in := make([]byte, 8+40+512)
		binary.LittleEndian.PutUint32(in[0:], storageDeviceProtocolSpecific)
		p := in[8:]
		binary.LittleEndian.PutUint32(p[0:], protocolTypeNvme)
		binary.LittleEndian.PutUint32(p[4:], nvmeDataTypeLogPage)
		binary.LittleEndian.PutUint32(p[8:], 0x02) // SMART / Health
		binary.LittleEndian.PutUint32(p[12:], 0)
		binary.LittleEndian.PutUint32(p[16:], 40)  // ProtocolDataOffset
		binary.LittleEndian.PutUint32(p[20:], 512) // ProtocolDataLength
		out := make([]byte, len(in))
		if _, err := ioctl(handle, ioctlStorageQueryProperty, in, out); err != nil {
			h.Note = "requête NVMe refusée : " + err.Error()
			return h
		}
		off := 8 + binary.LittleEndian.Uint32(out[8+16:])
		ln := binary.LittleEndian.Uint32(out[8+20:])
		if int(off)+int(ln) > len(out) || ln < 512 {
			h.Note = "réponse NVMe inattendue"
			return h
		}
		return parseNVMeHealth(out[off : off+512])
	}
	// ATA : SMART_RCV_DRIVE_DATA (structures compactées).
	in := make([]byte, 33)
	binary.LittleEndian.PutUint32(in[0:], 512)
	copy(in[4:12], []byte{0xD0, 1, 1, 0x4F, 0xC2, 0xA0, 0xB0, 0})
	out := make([]byte, 16+512)
	if _, err := ioctl(handle, smartRcvDriveData, in, out); err != nil {
		h.Note = "SMART non disponible : " + err.Error()
		return h
	}
	return parseATASmart(out[16:])
}
