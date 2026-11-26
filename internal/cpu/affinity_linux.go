package cpu

import "golang.org/x/sys/unix"

// pin épingle le fil OS courant sur le cœur logique n (LockOSThread requis).
func pin(n int) error {
	var set unix.CPUSet
	set.Set(n)
	return unix.SchedSetaffinity(0, &set)
}
