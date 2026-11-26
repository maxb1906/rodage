// Package safety arrête les tests quand la machine est en danger.
//
// Arrêt d'urgence : température au-delà de TjMax − marge, batterie basse ou
// secteur débranché sur portable, gel d'un test (watchdog). Un marqueur sur
// disque permet de signaler, au lancement suivant, un test interrompu par un
// plantage ou un redémarrage.
package safety

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/maxb1906/rodage/internal/sensors"
	"github.com/maxb1906/rodage/internal/sysinfo"
)

// Kind est le type d'événement de sûreté.
type Kind string

const (
	KindThermal  Kind = "thermal"
	KindBattery  Kind = "battery"
	KindAC       Kind = "ac"
	KindWatchdog Kind = "watchdog"
	KindUser     Kind = "user"
)

// Event est un événement de sûreté.
type Event struct {
	Time   time.Time `json:"time"`
	Kind   Kind      `json:"kind"`
	Detail string    `json:"detail"`
	Abort  bool      `json:"abort"`
}

// ErrAbort est la cause d'annulation après un arrêt d'urgence.
type ErrAbort struct{ Event Event }

func (e ErrAbort) Error() string {
	return fmt.Sprintf("arrêt d'urgence (%s) : %s", e.Event.Kind, e.Event.Detail)
}

// IsAbort indique si err provient d'un arrêt d'urgence.
func IsAbort(err error) (Event, bool) {
	var a ErrAbort
	if errors.As(err, &a) {
		return a.Event, true
	}
	return Event{}, false
}

// Config paramètre la surveillance.
type Config struct {
	TjMax        float64       // °C (NaN = inconnu, 100 par défaut)
	Margin       float64       // °C sous TjMax (défaut 5)
	MinBattery   int           // % (défaut 25)
	RequireAC    bool          // arrêt si le secteur est débranché
	Watchdog     time.Duration // délai sans progression (défaut 60 s)
	PollInterval time.Duration // défaut 1 s
}

// Monitor surveille la machine pendant un test.
type Monitor struct {
	cfg    Config
	cancel context.CancelCauseFunc
	mu     sync.Mutex
	events []Event
	beats  map[string]func() time.Time
	hot    int
	stop   chan struct{}
	done   chan struct{}
	maxT   float64
}

// Start démarre la surveillance ; cancel est appelé en cas d'arrêt d'urgence.
func Start(cfg Config, sampler *sensors.Sampler, cancel context.CancelCauseFunc) *Monitor {
	if cfg.TjMax != cfg.TjMax || cfg.TjMax <= 0 {
		cfg.TjMax = 100
	}
	if cfg.Margin <= 0 {
		cfg.Margin = 5
	}
	if cfg.MinBattery <= 0 {
		cfg.MinBattery = 25
	}
	if cfg.Watchdog <= 0 {
		cfg.Watchdog = 60 * time.Second
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	m := &Monitor{cfg: cfg, cancel: cancel, beats: map[string]func() time.Time{}, stop: make(chan struct{}), done: make(chan struct{})}
	if sampler != nil {
		sampler.Subscribe(m.onSample)
	}
	go m.loop()
	return m
}

// Limit renvoie la température d'arrêt.
func (m *Monitor) Limit() float64 { return m.cfg.TjMax - m.cfg.Margin }

func (m *Monitor) onSample(s sensors.Sample) {
	t := s.CPUTempC
	if t != t {
		return
	}
	m.mu.Lock()
	if t > m.maxT {
		m.maxT = t
	}
	if t >= m.Limit() {
		m.hot++
	} else {
		m.hot = 0
	}
	hot := m.hot
	m.mu.Unlock()
	// Trois relevés consécutifs au-delà du seuil : arrêt.
	if hot >= 3 {
		m.Abort(KindThermal, fmt.Sprintf("%.0f °C ≥ seuil %.0f °C (TjMax %.0f − %.0f)", t, m.Limit(), m.cfg.TjMax, m.cfg.Margin))
	}
}

// Watch enregistre une source de progression pour le watchdog.
func (m *Monitor) Watch(name string, last func() time.Time) {
	m.mu.Lock()
	m.beats[name] = last
	m.mu.Unlock()
}

// Unwatch retire une source.
func (m *Monitor) Unwatch(name string) {
	m.mu.Lock()
	delete(m.beats, name)
	m.mu.Unlock()
}

// Note ajoute un événement informatif.
func (m *Monitor) Note(k Kind, detail string) {
	m.mu.Lock()
	m.events = append(m.events, Event{Time: time.Now(), Kind: k, Detail: detail})
	m.mu.Unlock()
}

// Abort déclenche un arrêt d'urgence (une seule fois).
func (m *Monitor) Abort(k Kind, detail string) {
	e := Event{Time: time.Now(), Kind: k, Detail: detail, Abort: true}
	m.mu.Lock()
	for _, x := range m.events {
		if x.Abort {
			m.mu.Unlock()
			return
		}
	}
	m.events = append(m.events, e)
	m.mu.Unlock()
	m.cancel(ErrAbort{e})
}

func (m *Monitor) loop() {
	defer close(m.done)
	tk := time.NewTicker(m.cfg.PollInterval)
	defer tk.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-tk.C:
		}
		if onAC, pct, has, ok := sysinfo.PowerStatus(); ok && has {
			if m.cfg.RequireAC && !onAC {
				m.Abort(KindAC, "secteur débranché")
			} else if !onAC && pct >= 0 && pct < m.cfg.MinBattery {
				m.Abort(KindBattery, fmt.Sprintf("batterie à %d %%", pct))
			}
		}
		m.mu.Lock()
		now := time.Now()
		var stuck string
		for name, f := range m.beats {
			if last := f(); !last.IsZero() && now.Sub(last) > m.cfg.Watchdog {
				stuck = fmt.Sprintf("%s sans progression depuis %s", name, now.Sub(last).Round(time.Second))
			}
		}
		m.mu.Unlock()
		if stuck != "" {
			m.Abort(KindWatchdog, stuck)
		}
	}
}

// Stop arrête la surveillance et renvoie les événements.
func (m *Monitor) Stop() []Event {
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
	<-m.done
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Event(nil), m.events...)
}

// MaxTemp renvoie la température maximale observée.
func (m *Monitor) MaxTemp() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.maxT
}

// ----- Marqueur de test en cours -----

// Marker décrit un test en cours, écrit sur disque.
type Marker struct {
	Started time.Time `json:"started"`
	Mode    string    `json:"mode"`
	Phase   string    `json:"phase"`
	Updated time.Time `json:"updated"`
	PID     int       `json:"pid"`
}

// StateDir renvoie le dossier d'état de Rodage.
func StateDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "rodage")
}

func markerPath() string { return filepath.Join(StateDir(), "running.json") }

// WriteMarker enregistre (ou met à jour) le marqueur.
func WriteMarker(m Marker) error {
	if err := os.MkdirAll(StateDir(), 0o755); err != nil {
		return err
	}
	m.Updated = time.Now()
	m.PID = os.Getpid()
	b, _ := json.Marshal(m)
	tmp := markerPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, markerPath())
}

// ClearMarker supprime le marqueur à la fin normale d'un test.
func ClearMarker() { os.Remove(markerPath()) }

// PreviousCrash renvoie le marqueur d'un test précédent jamais terminé.
// Le marqueur est consommé.
func PreviousCrash() (Marker, bool) {
	b, err := os.ReadFile(markerPath())
	if err != nil {
		return Marker{}, false
	}
	var m Marker
	if json.Unmarshal(b, &m) != nil {
		os.Remove(markerPath())
		return Marker{}, false
	}
	if m.PID != os.Getpid() && processAlive(m.PID) {
		return Marker{}, false // un autre Rodage tourne
	}
	os.Remove(markerPath())
	return m, true
}
