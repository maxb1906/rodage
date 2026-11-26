// Package netbench mesure un lien réseau entre deux machines Rodage.
//
// Protocole maison : la seconde machine lance le même binaire en mode
// serveur. Une connexion TCP de contrôle (lignes JSON) pilote des connexions
// TCP de données (multi-flux) et des flux UDP (pertes, gigue), dans les deux
// sens.
package netbench

import (
	"encoding/binary"
	"time"
)

// DefaultPort est le port TCP et UDP du serveur.
const DefaultPort = 7420

// ProtoVersion est la version du protocole.
const ProtoVersion = 1

type hello struct {
	V       int    `json:"v"`
	Role    string `json:"role"` // control, data
	Session uint64 `json:"session"`
	Dir     string `json:"dir,omitempty"`
	Stream  int    `json:"stream,omitempty"`
}

type request struct {
	Cmd        string  `json:"cmd"` // info, tcp, tcp_result, udp, udp_result, link
	Dir        string  `json:"dir,omitempty"`
	Streams    int     `json:"streams,omitempty"`
	DurationMs int64   `json:"duration_ms,omitempty"`
	RateMbps   float64 `json:"rate_mbps,omitempty"`
	Size       int     `json:"size,omitempty"`
}

type reply struct {
	OK      bool        `json:"ok"`
	Error   string      `json:"error,omitempty"`
	Version string      `json:"version,omitempty"`
	Host    string      `json:"host,omitempty"`
	Bytes   uint64      `json:"bytes,omitempty"`
	Elapsed int64       `json:"elapsed_ns,omitempty"`
	UDPPort int         `json:"udp_port,omitempty"`
	UDP     *udpStats   `json:"udp,omitempty"`
	Link    *LinkResult `json:"link,omitempty"`
}

// En-tête des paquets UDP : session, numéro, horodatage d'envoi (ns).
const udpHeader = 24

func putUDPHeader(b []byte, session, seq uint64, ts int64) {
	binary.BigEndian.PutUint64(b[0:], session)
	binary.BigEndian.PutUint64(b[8:], seq)
	binary.BigEndian.PutUint64(b[16:], uint64(ts))
}

func readUDPHeader(b []byte) (session, seq uint64, ts int64) {
	return binary.BigEndian.Uint64(b[0:]), binary.BigEndian.Uint64(b[8:]), int64(binary.BigEndian.Uint64(b[16:]))
}

// udpStats accumule les statistiques de réception (RFC 3550 pour la gigue).
type udpStats struct {
	Received    uint64  `json:"received"`
	MaxSeq      uint64  `json:"max_seq"`
	OutOfOrder  uint64  `json:"out_of_order"`
	Duplicates  uint64  `json:"duplicates"`
	JitterNs    float64 `json:"jitter_ns"`
	Bytes       uint64  `json:"bytes"`
	FirstNs     int64   `json:"first_ns"`
	LastNs      int64   `json:"last_ns"`
	lastTransit int64
	seen        map[uint64]struct{}
}

func newUDPStats() *udpStats { return &udpStats{seen: make(map[uint64]struct{})} }

func (s *udpStats) add(seq uint64, sent int64, n int) {
	now := time.Now().UnixNano()
	if _, dup := s.seen[seq]; dup {
		s.Duplicates++
		return
	}
	if len(s.seen) < 1<<22 {
		s.seen[seq] = struct{}{}
	}
	if s.Received == 0 {
		s.FirstNs = now
	}
	s.LastNs = now
	s.Received++
	s.Bytes += uint64(n)
	if seq < s.MaxSeq {
		s.OutOfOrder++
	} else {
		s.MaxSeq = seq
	}
	transit := now - sent
	if s.Received > 1 {
		d := float64(transit - s.lastTransit)
		if d < 0 {
			d = -d
		}
		s.JitterNs += (d - s.JitterNs) / 16
	}
	s.lastTransit = transit
}
