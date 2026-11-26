//go:build !linux && !windows

package sensors

func open() Source { return nullSource{} }
