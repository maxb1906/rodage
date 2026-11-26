//go:build !linux && !windows

package netbench

import "net"

func readCounters(net.Interface) LinkCounters { return LinkCounters{} }
