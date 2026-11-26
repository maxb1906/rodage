package netbench

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Server répond aux clients Rodage.
type Server struct {
	Version string
	Log     *log.Logger

	mu       sync.Mutex
	sessions map[uint64]*session
	udp      *net.UDPConn
}

type session struct {
	mu        sync.Mutex
	upBytes   atomic.Uint64
	upFirst   atomic.Int64
	upLast    atomic.Int64
	upActive  sync.WaitGroup
	downUntil time.Time
	udpStats  *udpStats
	udpPeer   *net.UDPAddr
	udpPeerCh chan *net.UDPAddr
}

func (s *Server) logf(f string, a ...any) {
	if s.Log != nil {
		s.Log.Printf(f, a...)
	}
}

func (s *Server) session(id uint64) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = map[uint64]*session{}
	}
	ss, ok := s.sessions[id]
	if !ok {
		ss = &session{udpPeerCh: make(chan *net.UDPAddr, 1)}
		s.sessions[id] = ss
	}
	return ss
}

// ListenAndServe écoute sur addr (ex. ":7420") jusqu'à l'annulation de ctx.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	ua, err := net.ResolveUDPAddr("udp", ln.Addr().String())
	if err != nil {
		ln.Close()
		return err
	}
	uc, err := net.ListenUDP("udp", ua)
	if err != nil {
		ln.Close()
		return fmt.Errorf("UDP %s : %w", ua, err)
	}
	s.udp = uc
	s.logf("serveur Rodage à l'écoute sur %s (TCP et UDP)", ln.Addr())
	go func() {
		<-ctx.Done()
		ln.Close()
		uc.Close()
	}()
	go s.udpLoop()
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.handle(c)
	}
}

// Addr renvoie l'adresse UDP effective (utile si le port 0 a été demandé).
func (s *Server) udpPort() int { return s.udp.LocalAddr().(*net.UDPAddr).Port }

func (s *Server) udpLoop() {
	buf := make([]byte, 65536)
	for {
		n, from, err := s.udp.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if n < udpHeader {
			continue
		}
		id, seq, ts := readUDPHeader(buf)
		ss := s.session(id)
		ss.mu.Lock()
		if seq == 0 && ts == 0 {
			// Paquet d'annonce : le client attend des données (sens descendant).
			if ss.udpPeer == nil {
				ss.udpPeer = from
				select {
				case ss.udpPeerCh <- from:
				default:
				}
			}
		} else if ss.udpStats != nil {
			ss.udpStats.add(seq, ts, n)
		}
		ss.mu.Unlock()
	}
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	br := bufio.NewReader(c)
	line, err := br.ReadBytes('\n')
	if err != nil {
		return
	}
	var h hello
	if json.Unmarshal(line, &h) != nil || h.V != ProtoVersion {
		fmt.Fprintf(c, `{"ok":false,"error":"protocole incompatible"}`+"\n")
		return
	}
	switch h.Role {
	case "control":
		s.control(c, br, h)
	case "data":
		s.data(c, br, h)
	}
}

func (s *Server) data(c net.Conn, br *bufio.Reader, h hello) {
	ss := s.session(h.Session)
	if tc, ok := c.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
	}
	switch h.Dir {
	case "up":
		ss.upActive.Add(1)
		defer ss.upActive.Done()
		buf := make([]byte, 256<<10)
		for {
			n, err := br.Read(buf)
			if n > 0 {
				now := time.Now().UnixNano()
				ss.upFirst.CompareAndSwap(0, now)
				ss.upLast.Store(now)
				ss.upBytes.Add(uint64(n))
			}
			if err != nil {
				return
			}
		}
	case "down":
		ss.mu.Lock()
		until := ss.downUntil
		ss.mu.Unlock()
		buf := make([]byte, 128<<10)
		for i := range buf {
			buf[i] = byte(i * 31)
		}
		for time.Now().Before(until) {
			c.SetWriteDeadline(until.Add(2 * time.Second))
			if _, err := c.Write(buf); err != nil {
				return
			}
		}
	}
}

func (s *Server) control(c net.Conn, br *bufio.Reader, h hello) {
	ss := s.session(h.Session)
	enc := json.NewEncoder(c)
	dec := json.NewDecoder(br)
	var mon *LinkMonitor
	if la, ok := c.LocalAddr().(*net.TCPAddr); ok {
		mon = NewLinkMonitor(la.IP)
		defer mon.Stop()
	}
	remote := c.RemoteAddr().String()
	s.logf("client %s connecté (session %x)", remote, h.Session)
	defer s.logf("client %s déconnecté", remote)
	defer func() {
		s.mu.Lock()
		delete(s.sessions, h.Session)
		s.mu.Unlock()
	}()
	for {
		var q request
		if err := dec.Decode(&q); err != nil {
			if err != io.EOF {
				s.logf("contrôle %s : %v", remote, err)
			}
			return
		}
		var r reply
		switch q.Cmd {
		case "info":
			host, _ := os.Hostname()
			r = reply{OK: true, Version: s.Version, Host: host, UDPPort: s.udpPort()}
		case "tcp":
			if q.Dir == "down" {
				ss.mu.Lock()
				ss.downUntil = time.Now().Add(time.Duration(q.DurationMs)*time.Millisecond + 500*time.Millisecond)
				ss.mu.Unlock()
			} else {
				ss.upBytes.Store(0)
				ss.upFirst.Store(0)
				ss.upLast.Store(0)
			}
			r = reply{OK: true}
		case "tcp_result":
			waitTimeout(&ss.upActive, 10*time.Second)
			r = reply{OK: true, Bytes: ss.upBytes.Load(), Elapsed: ss.upLast.Load() - ss.upFirst.Load()}
		case "udp":
			if q.Dir == "up" {
				ss.mu.Lock()
				ss.udpStats = newUDPStats()
				ss.mu.Unlock()
				r = reply{OK: true, UDPPort: s.udpPort()}
			} else {
				// La réponse part une fois l'envoi terminé.
				s.udpSend(ss, h.Session, q)
				r = reply{OK: true, UDPPort: s.udpPort()}
			}
		case "udp_result":
			ss.mu.Lock()
			st := ss.udpStats
			ss.udpStats = nil
			ss.mu.Unlock()
			if st == nil {
				st = newUDPStats()
			}
			r = reply{OK: true, UDP: st}
		case "link":
			if mon != nil {
				l := mon.Stop()
				r = reply{OK: true, Link: &l}
				mon = NewLinkMonitor(c.LocalAddr().(*net.TCPAddr).IP)
			} else {
				r = reply{OK: true, Link: &LinkResult{}}
			}
		default:
			r = reply{Error: "commande inconnue : " + q.Cmd}
		}
		if err := enc.Encode(r); err != nil {
			return
		}
	}
}

// udpSend envoie un flux UDP vers le client (sens descendant).
func (s *Server) udpSend(ss *session, id uint64, q request) {
	var peer *net.UDPAddr
	select {
	case peer = <-ss.udpPeerCh:
	case <-time.After(5 * time.Second):
		return
	}
	ss.mu.Lock()
	ss.udpPeer = nil
	ss.mu.Unlock()
	sendUDP(context.Background(), func(b []byte) error {
		_, err := s.udp.WriteToUDP(b, peer)
		return err
	}, id, q.RateMbps, q.Size, time.Duration(q.DurationMs)*time.Millisecond)
}

func waitTimeout(wg *sync.WaitGroup, d time.Duration) {
	ch := make(chan struct{})
	go func() { wg.Wait(); close(ch) }()
	select {
	case <-ch:
	case <-time.After(d):
	}
}

// sendUDP envoie des paquets numérotés au débit demandé.
func sendUDP(ctx context.Context, write func([]byte) error, id uint64, rateMbps float64, size int, d time.Duration) uint64 {
	if size < udpHeader {
		size = 1200
	}
	buf := make([]byte, size)
	pps := rateMbps * 1e6 / 8 / float64(size)
	start := time.Now()
	var seq uint64
	for {
		el := time.Since(start)
		if el >= d || ctx.Err() != nil {
			return seq
		}
		due := uint64(pps * el.Seconds())
		for seq < due {
			seq++
			putUDPHeader(buf, id, seq, time.Now().UnixNano())
			if write(buf) != nil {
				// Tampon plein : on compte le paquet comme envoyé (il sera perdu).
				continue
			}
		}
		time.Sleep(500 * time.Microsecond)
	}
}
