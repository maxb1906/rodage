//go:build !linux && !windows

package sysinfo

import "os"

func IsAdmin() bool            { return os.Geteuid() == 0 }
func collectPlatform(in *Info) {}
func memAvailable() uint64     { return 0 }

func PowerStatus() (onAC bool, batteryPct int, hasBattery bool, ok bool) {
	return true, -1, false, false
}
