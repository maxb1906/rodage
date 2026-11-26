//go:build (!amd64 && !arm64) || purego

package kernels

func available() []Kernel { return nil }
