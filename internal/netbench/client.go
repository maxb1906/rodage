package netbench

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// TCPResult est une mesure TCP multi-flux.
type TCPResult struct {
	Dir      string        `json:"dir"` // up (client → serveur), down
	Streams  int           `json:"streams"`
	Mbps     float64       `json:"mbps"`
	Bytes    uint64        `json:"bytes"`
	Duration time.Duration `json:"duration"`
	Curve    []float64     `json:"curve,omitempty"` // Mbit/s par seconde
}

// UDPResult est une mesure UDP.
type UDPResult struct {
	Dir        string  `json:"dir"`
	TargetMbps float64 `json:"target_mbps"`
	Mbps       float64 `json:"mbps"`
	Sent       uint64  `json:"sent"`
	Received   uint64  `json:"received"`
	Lost       uint64  `json:"lost"`
	LossPct    float64 `json:"loss_pct"`
	JitterMs   float64 `json:"jitter_ms"`
	OutOfOrder uint64  `json:"out_of_order"`
}

// Result est le bilan réseau.
type Result struct {
	Server        string      `json:"server"`
	ServerHost    string      `json:"server_host,omitempty"`
	ServerVersion string      `json:"server_version,omitempty"`
	RTTms         float64     `json:"rtt_ms"`
	Rounds        int         `json:"rounds"`
	TCP           []TCPResult `json:"tcp"`
	UDP           []UDPResult `json:"udp"`
	Link          LinkResult  `json:"link"`
	ServerLink    LinkResult  `json:"server_link"`
	Errors        []string    `json:"errors,omitempty"`
	Aborted       string      `json:"aborted,omitempty"`
}

// Config paramètre un test réseau.
type Config struct {
	Server   string // hôte ou hôte:port
	Streams  int    // flux TCP parallèles (défaut 4)
	Step     time.Duration
	Duration time.Duration // durée totale (≥ une série complète)
	// UDPRateMbps : débit UDP visé (0 = moitié du débit TCP mesuré).
	UDPRateMbps float64
	Progress    func(Progress)
}

// Progress est l'avancement transmis à l'interface.
type Progress struct {
	Phase string  `json:"phase"`
	Mbps  float64 `json:"mbps"`
}

type client struct {
	cfg     Config
	addr    string
	session uint64
	ctrl    net.Conn
	enc     *json.Encoder
	dec     *json.Decoder
	udpPort int
}

func (c *client) progress(p Progress) {
	if c.cfg.Progress != nil {
		c.cfg.Progress(p)
	}
}

func (c *client) call(q request) (reply, error) {
	c.ctrl.SetDeadline(time.Now().Add(time.Duration(q.DurationMs)*time.Millisecond + 30*time.Second))
	if err := c.enc.Encode(q); err != nil {
		return reply{}, err
	}
	var r reply
	if err := c.dec.Decode(&r); err != nil {
		return r, err
	}
	if !r.OK {
		return r, fmt.Errorf("serveur : %s", r.Error)
	}
	return r, nil
}

func (c *client) dialData(dir string, stream int) (net.Conn, error) {
	d, err := net.DialTimeout("tcp", c.addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(hello{V: ProtoVersion, Role: "data", Session: c.session, Dir: dir, Stream: stream})
	if _, err := d.Write(append(b, '\n')); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

// Run exécute le test réseau contre un serveur Rodage.
func Run(ctx context.Context, cfg Config) (Result, error) {
	if cfg.Streams <= 0 {
		cfg.Streams = 4
	}
	if cfg.Step <= 0 {
		cfg.Step = 10 * time.Second
	}
	addr := cfg.Server
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, strconv.Itoa(DefaultPort))
	}
	res := Result{Server: addr}
	c := &client{cfg: cfg, addr: addr, session: rand.Uint64() | 1}
	t0 := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return res, fmt.Errorf("connexion au serveur %s : %w", addr, err)
	}
	res.RTTms = float64(time.Since(t0).Microseconds()) / 1000
	defer conn.Close()
	c.ctrl = conn
	b, _ := json.Marshal(hello{V: ProtoVersion, Role: "control", Session: c.session})
	conn.Write(append(b, '\n'))
	c.enc = json.NewEncoder(conn)
	c.dec = json.NewDecoder(bufio.NewReader(conn))
	info, err := c.call(request{Cmd: "info"})
	if err != nil {
		return res, err
	}
	res.ServerHost, res.ServerVersion, c.udpPort = info.Host, info.Version, info.UDPPort
	mon := NewLinkMonitor(conn.LocalAddr().(*net.TCPAddr).IP)
	deadline := time.Now().Add(cfg.Duration)
	for round := 0; ctx.Err() == nil; round++ {
		rates := map[string]float64{}
		for _, dir := range []string{"up", "down"} {
			if ctx.Err() != nil {
				break
			}
			t, err := c.tcp(ctx, dir)
			if err != nil {
				res.Errors = append(res.Errors, err.Error())
				continue
			}
			rates[dir] = t.Mbps
			res.TCP = append(res.TCP, t)
		}
		for _, dir := range []string{"up", "down"} {
			if ctx.Err() != nil {
				break
			}
			rate := cfg.UDPRateMbps
			if rate <= 0 {
				rate = min(max(rates[dir]/2, 10), 500)
			}
			u, err := c.udp(ctx, dir, rate)
			if err != nil {
				res.Errors = append(res.Errors, err.Error())
				continue
			}
			res.UDP = append(res.UDP, u)
		}
		res.Rounds = round + 1
		if time.Now().Add(4 * cfg.Step).After(deadline) {
			break
		}
	}
	res.Link = mon.Stop()
	if r, err := c.call(request{Cmd: "link"}); err == nil && r.Link != nil {
		res.ServerLink = *r.Link
	}
	if ctx.Err() != nil {
		res.Aborted = ctx.Err().Error()
	}
	return res, nil
}

func (c *client) tcp(ctx context.Context, dir string) (TCPResult, error) {
	d := c.cfg.Step
	res := TCPResult{Dir: dir, Streams: c.cfg.Streams}
	if _, err := c.call(request{Cmd: "tcp", Dir: dir, Streams: c.cfg.Streams, DurationMs: d.Milliseconds()}); err != nil {
		return res, err
	}
	var bytes atomic.Uint64
	var wg sync.WaitGroup
	conns := make([]net.Conn, 0, c.cfg.Streams)
	for i := 0; i < c.cfg.Streams; i++ {
		dc, err := c.dialData(dir, i)
		if err != nil {
			for _, x := range conns {
				x.Close()
			}
			return res, err
		}
		conns = append(conns, dc)
	}
	ctx2, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	start := time.Now()
	stop := make(chan struct{})
	var curve []float64
	go func() {
		tk := time.NewTicker(time.Second)
		defer tk.Stop()
		var last uint64
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
				b := bytes.Load()
				mbps := float64(b-last) * 8 / 1e6
				last = b
				curve = append(curve, mbps)
				c.progress(Progress{Phase: "tcp-" + dir, Mbps: mbps})
			}
		}
	}()
	for _, dc := range conns {
		wg.Add(1)
		go func(dc net.Conn) {
			defer wg.Done()
			defer dc.Close()
			buf := make([]byte, 128<<10)
			go func() { <-ctx2.Done(); dc.SetDeadline(time.Now()) }()
			for ctx2.Err() == nil {
				var n int
				var err error
				if dir == "up" {
					n, err = dc.Write(buf)
				} else {
					n, err = dc.Read(buf)
				}
				bytes.Add(uint64(n))
				if err != nil {
					return // fin de durée ou fin de flux côté serveur
				}
			}
		}(dc)
	}
	wg.Wait()
	close(stop)
	el := time.Since(start)
	res.Curve = curve
	if dir == "up" {
		r, err := c.call(request{Cmd: "tcp_result"})
		if err != nil {
			return res, err
		}
		res.Bytes = r.Bytes
		res.Duration = time.Duration(r.Elapsed)
	} else {
		res.Bytes = bytes.Load()
		res.Duration = el
	}
	if res.Duration > 0 {
		res.Mbps = float64(res.Bytes) * 8 / res.Duration.Seconds() / 1e6
	}
	return res, ctx.Err()
}

func (c *client) udp(ctx context.Context, dir string, rate float64) (UDPResult, error) {
	d := c.cfg.Step
	const size = 1200
	res := UDPResult{Dir: dir, TargetMbps: rate}
	host, _, _ := net.SplitHostPort(c.addr)
	ua, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(c.udpPort)))
	if err != nil {
		return res, err
	}
	uc, err := net.DialUDP("udp", nil, ua)
	if err != nil {
		return res, err
	}
	defer uc.Close()
	uc.SetReadBuffer(8 << 20)
	uc.SetWriteBuffer(8 << 20)
	c.progress(Progress{Phase: "udp-" + dir, Mbps: rate})
	if dir == "up" {
		if _, err := c.call(request{Cmd: "udp", Dir: "up"}); err != nil {
			return res, err
		}
		res.Sent = sendUDP(ctx, func(b []byte) error { _, err := uc.Write(b); return err }, c.session, rate, size, d)
		time.Sleep(500 * time.Millisecond)
		r, err := c.call(request{Cmd: "udp_result"})
		if err != nil {
			return res, err
		}
		fillUDP(&res, r.UDP)
		return res, ctx.Err()
	}
	// Sens descendant : on annonce notre adresse puis on reçoit.
	st := newUDPStats()
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 65536)
		for {
			uc.SetReadDeadline(time.Now().Add(d + 3*time.Second))
			n, err := uc.Read(buf)
			if err != nil {
				return
			}
			if n >= udpHeader {
				_, seq, ts := readUDPHeader(buf)
				st.add(seq, ts, n)
			}
		}
	}()
	hdr := make([]byte, udpHeader)
	putUDPHeader(hdr, c.session, 0, 0)
	ann := make(chan error, 1)
	go func() {
		_, err := c.call(request{Cmd: "udp", Dir: "down", RateMbps: rate, Size: size, DurationMs: d.Milliseconds()})
		ann <- err
	}()
	// Annonce répétée jusqu'au premier paquet reçu (la première peut se perdre).
	for i := 0; i < 20; i++ {
		uc.Write(hdr)
		time.Sleep(100 * time.Millisecond)
		if st.Received > 0 {
			break
		}
	}
	// L'appel de contrôle rend la main quand le serveur a fini d'envoyer.
	var callErr error
	select {
	case callErr = <-ann:
	case <-ctx.Done():
	}
	time.Sleep(500 * time.Millisecond)
	uc.SetReadDeadline(time.Now())
	<-done
	if callErr != nil {
		return res, callErr
	}
	fillUDP(&res, st)
	res.Sent = st.MaxSeq
	return res, ctx.Err()
}

func fillUDP(res *UDPResult, st *udpStats) {
	if st == nil {
		return
	}
	res.Received = st.Received
	if res.Sent == 0 {
		res.Sent = st.MaxSeq
	}
	if res.Sent > st.Received {
		res.Lost = res.Sent - st.Received
	}
	if res.Sent > 0 {
		res.LossPct = 100 * float64(res.Lost) / float64(res.Sent)
	}
	res.JitterMs = st.JitterNs / 1e6
	res.OutOfOrder = st.OutOfOrder
	if el := st.LastNs - st.FirstNs; el > 0 {
		res.Mbps = float64(st.Bytes) * 8 / (float64(el) / 1e9) / 1e6
	}
}
