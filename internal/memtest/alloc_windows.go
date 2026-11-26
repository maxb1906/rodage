package memtest

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type region struct {
	addr   uintptr
	buf    []byte
	locked bool
	note   string
}

func allocate(n uint64) (*region, error) {
	addr, err := windows.VirtualAlloc(0, uintptr(n), windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil {
		return nil, fmt.Errorf("VirtualAlloc %d octets : %w", n, err)
	}
	// Conversion uintptr → pointeur : mémoire hors tas Go, jamais déplacée.
	p := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	r := &region{addr: addr, buf: unsafe.Slice((*byte)(p), n)}
	// VirtualLock est limité par l'ensemble de travail : on l'agrandit d'abord.
	proc := windows.CurrentProcess()
	margin := uintptr(64 << 20)
	if err := windows.SetProcessWorkingSetSizeEx(proc, uintptr(n)+margin, uintptr(n)+4*margin, 0); err != nil {
		r.note = "agrandissement de l'ensemble de travail refusé : " + err.Error()
	}
	if err := windows.VirtualLock(addr, uintptr(n)); err != nil {
		r.note = "VirtualLock refusé (" + err.Error() + ") : la mémoire peut être déplacée dans le fichier d'échange"
	} else {
		r.locked = true
	}
	return r, nil
}

func (r *region) free() {
	if r.locked {
		_ = windows.VirtualUnlock(r.addr, uintptr(len(r.buf)))
	}
	_ = windows.VirtualFree(r.addr, 0, windows.MEM_RELEASE)
}

var (
	wevtapi      = windows.NewLazySystemDLL("wevtapi.dll")
	procEvtQuery = wevtapi.NewProc("EvtQuery")
	procEvtNext  = wevtapi.NewProc("EvtNext")
	procEvtClose = wevtapi.NewProc("EvtClose")
)

const (
	evtQueryChannelPath = 0x1
	evtQueryReverse     = 0x200
)

// countEvents compte les événements correspondant à la requête XPath.
func countEvents(channel, query string) (uint64, bool) {
	if wevtapi.Load() != nil {
		return 0, false
	}
	ch, _ := windows.UTF16PtrFromString(channel)
	q, _ := windows.UTF16PtrFromString(query)
	h, _, _ := procEvtQuery.Call(0, uintptr(unsafe.Pointer(ch)), uintptr(unsafe.Pointer(q)), evtQueryChannelPath)
	if h == 0 {
		return 0, false
	}
	defer procEvtClose.Call(h)
	var total uint64
	events := make([]uintptr, 64)
	for {
		var got uint32
		r, _, _ := procEvtNext.Call(h, uintptr(len(events)), uintptr(unsafe.Pointer(&events[0])), 1000, 0, uintptr(unsafe.Pointer(&got)))
		if r == 0 || got == 0 {
			break
		}
		for i := uint32(0); i < got; i++ {
			procEvtClose.Call(events[i])
		}
		total += uint64(got)
	}
	return total, true
}

// readECC compte les erreurs matérielles WHEA journalisées (corrigées : ID 47 et 19 ;
// fatales : ID 18 et 1).
func readECC() ECC {
	base := "*[System[Provider[@Name='Microsoft-Windows-WHEA-Logger'] and (%s)]]"
	ce, ok1 := countEvents("System", fmt.Sprintf(base, "EventID=47 or EventID=19"))
	ue, ok2 := countEvents("System", fmt.Sprintf(base, "EventID=18 or EventID=1"))
	if !ok1 && !ok2 {
		return ECC{}
	}
	return ECC{Available: true, Source: "WHEA", Corrected: ce, Uncorrected: ue}
}
