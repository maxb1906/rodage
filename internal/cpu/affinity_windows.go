package cpu

import (
	"errors"

	"golang.org/x/sys/windows"
)

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procSetThreadAffinityMask = kernel32.NewProc("SetThreadAffinityMask")
	procGetCurrentThread      = kernel32.NewProc("GetCurrentThread")
)

// pin épingle le fil OS courant sur le cœur logique n (premier groupe de 64).
func pin(n int) error {
	if n >= 64 {
		return errors.New("cœur hors du premier groupe de processeurs")
	}
	th, _, _ := procGetCurrentThread.Call()
	r, _, err := procSetThreadAffinityMask.Call(th, uintptr(1)<<uint(n))
	if r == 0 {
		return err
	}
	return nil
}
