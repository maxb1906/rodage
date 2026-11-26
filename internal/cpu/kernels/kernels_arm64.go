//go:build arm64 && !purego

package kernels

// Noyaux NEON écrits à la main (kernels_arm64.s).

//go:noescape
func xorshiftNEON(p *uint32, n int, rounds int)

//go:noescape
func fmaNEON(p *float64, n int, rounds int, a, ainv, b, c float64)
