//go:build amd64 && !purego

package kernels

import "golang.org/x/sys/cpu"

func available() []Kernel {
	var ks []Kernel
	if cpu.X86.HasSSE2 {
		ks = append(ks, Kernel{Level: LevelSSE2, Fused: false, xorshift: xorshiftSSE2, float: mulAddSSE2})
	}
	if cpu.X86.HasAVX2 && cpu.X86.HasFMA {
		ks = append(ks, Kernel{Level: LevelAVX2, Fused: true, xorshift: xorshiftAVX2, float: fmaAVX2})
	}
	if cpu.X86.HasAVX512F {
		ks = append(ks, Kernel{Level: LevelAVX512, Fused: true, xorshift: xorshiftAVX512, float: fmaAVX512})
	}
	return ks
}
