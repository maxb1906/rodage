package netbench

import (
	"net"
	"sync"
	"time"
)

// LinkCounters est un relevé des compteurs de la carte réseau.
type LinkCounters struct {
	OK             bool
	SpeedMbps      float64
	Up             bool
	RxErrors       uint64
	TxErrors       uint64
	RxDropped      uint64
	TxDropped      uint64
	CarrierChanges uint64
	HasCarrier     bool // CarrierChanges disponible
}

// LinkResult résume la stabilité du lien pendant le test.
type LinkResult struct {
	Available      bool    `json:"available"`
	Interface      string  `json:"interface"`
	SpeedStartMbps float64 `json:"speed_start_mbps"`
	SpeedEndMbps   float64 `json:"speed_end_mbps"`
	SpeedChanges   int     `json:"speed_changes"`
	Disconnects    int     `json:"disconnects"`
	RxErrors       uint64  `json:"rx_errors"`
	TxErrors       uint64  `json:"tx_errors"`
	Dropped        uint64  `json:"dropped"`
	CarrierChanges uint64  `json:"carrier_changes"`
}

// Problems liste les signes d'instabilité.
func (l LinkResult) Problems() []string {
	var p []string
	if l.Disconnects > 0 || l.CarrierChanges > 0 {
		p = append(p, "renegotiation")
	} else if l.SpeedChanges > 0 {
		p = append(p, "speed_change")
	}
	if l.RxErrors+l.TxErrors > 0 {
		p = append(p, "nic_errors")
	}
	return p
}

// interfaceFor renvoie l'interface portant l'adresse locale ip.
func interfaceFor(ip net.IP) (net.Interface, bool) {
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.Equal(ip) {
				return i, true
			}
		}
	}
	return net.Interface{}, false
}

// LinkMonitor relève l'interface chaque seconde.
type LinkMonitor struct {
	iface net.Interface
	first LinkCounters
	last  LinkCounters
	res   LinkResult
	mu    sync.Mutex
	stop  chan struct{}
	done  chan struct{}
}

// NewLinkMonitor démarre la surveillance de l'interface portant localIP.
func NewLinkMonitor(localIP net.IP) *LinkMonitor {
	m := &LinkMonitor{stop: make(chan struct{}), done: make(chan struct{})}
	iface, ok := interfaceFor(localIP)
	if !ok {
		close(m.done)
		return m
	}
	m.iface = iface
	m.res.Interface = iface.Name
	m.first = readCounters(iface)
	m.last = m.first
	m.res.Available = m.first.OK
	m.res.SpeedStartMbps = m.first.SpeedMbps
	if !m.first.OK {
		close(m.done)
		return m
	}
	go func() {
		defer close(m.done)
		tk := time.NewTicker(time.Second)
		defer tk.Stop()
		for {
			select {
			case <-m.stop:
				return
			case <-tk.C:
				c := readCounters(iface)
				m.mu.Lock()
				if c.OK {
					if c.SpeedMbps != m.last.SpeedMbps && c.SpeedMbps > 0 && m.last.SpeedMbps > 0 {
						m.res.SpeedChanges++
					}
					if !c.Up && m.last.Up {
						m.res.Disconnects++
					}
					m.last = c
				}
				m.mu.Unlock()
			}
		}
	}()
	return m
}

// Stop termine la surveillance et renvoie le bilan.
func (m *LinkMonitor) Stop() LinkResult {
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
	<-m.done
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.res.Available {
		return m.res
	}
	end := readCounters(m.iface)
	if !end.OK {
		end = m.last
	}
	sub := func(a, b uint64) uint64 {
		if b > a {
			return b - a
		}
		return 0
	}
	m.res.SpeedEndMbps = end.SpeedMbps
	m.res.RxErrors = sub(m.first.RxErrors, end.RxErrors)
	m.res.TxErrors = sub(m.first.TxErrors, end.TxErrors)
	m.res.Dropped = sub(m.first.RxDropped, end.RxDropped) + sub(m.first.TxDropped, end.TxDropped)
	if end.HasCarrier {
		m.res.CarrierChanges = sub(m.first.CarrierChanges, end.CarrierChanges)
	}
	return m.res
}
