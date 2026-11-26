//go:build !linux && !windows

package memtest

type region struct {
	buf    []byte
	locked bool
	note   string
}

func allocate(n uint64) (*region, error) {
	return &region{buf: make([]byte, n), note: "verrouillage non pris en charge sur ce système"}, nil
}

func (r *region) free() {}
func readECC() ECC      { return ECC{} }
