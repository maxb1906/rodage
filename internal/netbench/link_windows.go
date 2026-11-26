package netbench

import (
	"net"

	"golang.org/x/sys/windows"
)

func readCounters(i net.Interface) LinkCounters {
	row := windows.MibIfRow2{InterfaceIndex: uint32(i.Index)}
	if err := windows.GetIfEntry2Ex(windows.MibIfEntryNormal, &row); err != nil {
		return LinkCounters{}
	}
	return LinkCounters{
		OK:        true,
		SpeedMbps: float64(row.ReceiveLinkSpeed) / 1e6,
		Up:        row.MediaConnectState == 1,
		RxErrors:  row.InErrors,
		TxErrors:  row.OutErrors,
		RxDropped: row.InDiscards,
		TxDropped: row.OutDiscards,
	}
}
