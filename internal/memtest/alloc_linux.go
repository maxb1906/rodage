package memtest

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type region struct {
	buf    []byte
	locked bool
	note   string
}

func allocate(n uint64) (*region, error) {
	b, err := unix.Mmap(-1, 0, int(n), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANONYMOUS|unix.MAP_POPULATE)
	if err != nil {
		return nil, fmt.Errorf("allocation de %d octets : %w", n, err)
	}
	r := &region{buf: b}
	// Pas de pages géantes transparentes : on veut des pages ordinaires stables.
	_ = unix.Madvise(b, unix.MADV_NOHUGEPAGE)
	if err := unix.Mlock(b); err != nil {
		var lim unix.Rlimit
		_ = unix.Getrlimit(unix.RLIMIT_MEMLOCK, &lim)
		r.note = fmt.Sprintf("mlock refusé (%v, limite RLIMIT_MEMLOCK %d o) : la mémoire peut être déplacée en swap", err, lim.Cur)
	} else {
		r.locked = true
	}
	return r, nil
}

func (r *region) free() {
	if r.locked {
		_ = unix.Munlock(r.buf)
	}
	_ = unix.Munmap(r.buf)
}

func readUint(p string) (uint64, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	return v, err == nil
}

// readECC lit les compteurs EDAC.
func readECC() ECC {
	mcs, _ := filepath.Glob("/sys/devices/system/edac/mc/mc*")
	e := ECC{}
	for _, mc := range mcs {
		ce, ok1 := readUint(mc + "/ce_count")
		ue, ok2 := readUint(mc + "/ue_count")
		if ok1 || ok2 {
			e.Available = true
			e.Corrected += ce
			e.Uncorrected += ue
		}
	}
	if e.Available {
		e.Source = "EDAC"
	}
	return e
}
