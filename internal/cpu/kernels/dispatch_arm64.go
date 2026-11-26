//go:build arm64 && !purego

package kernels

import "golang.org/x/sys/cpu"

func available() []Kernel {
	// ASIMD (NEON) est obligatoire en ARMv8-A ; on vérifie tout de même.
	if !cpu.ARM64.HasASIMD && !cpu.ARM64.HasFP {
		return nil
	}
	return []Kernel{{Level: LevelNEON, Fused: true, xorshift: xorshiftNEON, float: fmaNEON}}
}
