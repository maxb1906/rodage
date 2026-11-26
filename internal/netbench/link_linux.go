package netbench

import (
	"net"
	"os"
	"strconv"
	"strings"
)

func readU(p string) (uint64, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	return v, err == nil
}

func readCounters(i net.Interface) LinkCounters {
	base := "/sys/class/net/" + i.Name + "/"
	var c LinkCounters
	var ok bool
	c.RxErrors, ok = readU(base + "statistics/rx_errors")
	if !ok {
		return c
	}
	c.OK = true
	c.TxErrors, _ = readU(base + "statistics/tx_errors")
	c.RxDropped, _ = readU(base + "statistics/rx_dropped")
	c.TxDropped, _ = readU(base + "statistics/tx_dropped")
	c.CarrierChanges, c.HasCarrier = readU(base + "carrier_changes")
	if b, err := os.ReadFile(base + "speed"); err == nil {
		if v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil && v > 0 {
			c.SpeedMbps = float64(v)
		}
	}
	op, _ := os.ReadFile(base + "operstate")
	c.Up = strings.TrimSpace(string(op)) != "down"
	return c
}
