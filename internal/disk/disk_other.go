//go:build !linux && !windows

package disk

import (
	"errors"
	"os"
)

var errUnsupported = errors.New("non pris en charge sur ce système")

func openFile(path string, create bool) (target, bool, error) {
	fl := os.O_RDWR
	if create {
		fl |= os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(path, fl, 0o600)
	return f, false, err
}
func openRaw(d Device) (target, error)     { return nil, errUnsupported }
func freeSpace(string) (uint64, error)     { return 1 << 40, nil }
func ListDevices() ([]Device, error)       { return nil, errUnsupported }
func DeviceForPath(string) (Device, error) { return Device{}, errUnsupported }
func ReadHealth(Device) Health             { h := unknownHealth(); h.Note = "non pris en charge"; return h }
