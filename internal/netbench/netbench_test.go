package netbench

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestLoopback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Port libre.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	s := &Server{Version: "test"}
	go s.ListenAndServe(ctx, addr)
	time.Sleep(200 * time.Millisecond)
	r, err := Run(ctx, Config{Server: addr, Streams: 2, Step: 1500 * time.Millisecond, Duration: time.Second, UDPRateMbps: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.TCP) != 2 || len(r.UDP) != 2 || len(r.Errors) > 0 {
		t.Fatalf("%+v", r)
	}
	for _, x := range r.TCP {
		t.Logf("TCP %s %.0f Mbit/s", x.Dir, x.Mbps)
		if x.Mbps <= 0 {
			t.Fatal("débit nul")
		}
	}
	for _, x := range r.UDP {
		t.Logf("UDP %s visé %.0f reçu %.0f Mbit/s perte %.2f %% gigue %.3f ms (%d/%d)", x.Dir, x.TargetMbps, x.Mbps, x.LossPct, x.JitterMs, x.Received, x.Sent)
		if x.Received == 0 {
			t.Fatal("aucun paquet UDP reçu")
		}
	}
	t.Logf("lien %+v serveur %+v", r.Link, r.ServerLink)
}
