package safety

import (
	"context"
	"testing"
	"time"

	"github.com/maxb1906/rodage/internal/sensors"
)

func TestThermalAbort(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	m := Start(Config{TjMax: 100, Margin: 5, PollInterval: 20 * time.Millisecond}, nil, cancel)
	for i := 0; i < 3; i++ {
		m.onSample(sensors.Sample{CPUTempC: 97})
	}
	<-ctx.Done()
	ev, ok := IsAbort(context.Cause(ctx))
	if !ok || ev.Kind != KindThermal {
		t.Fatalf("%v", context.Cause(ctx))
	}
	m.Stop()
}

func TestWatchdog(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	m := Start(Config{Watchdog: 50 * time.Millisecond, PollInterval: 10 * time.Millisecond}, nil, cancel)
	old := time.Now().Add(-time.Second)
	m.Watch("cpu", func() time.Time { return old })
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog muet")
	}
	if ev, ok := IsAbort(context.Cause(ctx)); !ok || ev.Kind != KindWatchdog {
		t.Fatal(context.Cause(ctx))
	}
	m.Stop()
}

func TestMarker(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	if _, ok := PreviousCrash(); ok {
		t.Fatal("marqueur fantôme")
	}
	WriteMarker(Marker{Mode: "stress", Phase: "cpu"})
	// Même PID : considéré comme un plantage précédent.
	if m, ok := PreviousCrash(); !ok || m.Phase != "cpu" {
		t.Fatal("marqueur non relu")
	}
	if _, ok := PreviousCrash(); ok {
		t.Fatal("marqueur non consommé")
	}
}
