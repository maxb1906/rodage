// Package refdb est la base locale de références par modèle de CPU et de
// disque. Elle reste sur la machine : aucune télémétrie. Elle peut être
// exportée et importée à la main (fichier JSON).
package refdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/maxb1906/rodage/internal/report"
	"github.com/maxb1906/rodage/internal/safety"
)

// Entry est une mesure de référence.
type Entry struct {
	Date     time.Time          `json:"date"`
	ReportID string             `json:"report_id"`
	Kind     string             `json:"kind"` // cpu, disk
	Model    string             `json:"model"`
	Detail   string             `json:"detail,omitempty"` // ex. 16 threads, nvme
	Scores   map[string]float64 `json:"scores"`
}

// DB est la base de références.
type DB struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
	path    string
}

// DefaultPath renvoie l'emplacement par défaut.
func DefaultPath() string { return filepath.Join(safety.StateDir(), "references.json") }

// Open charge la base (vide si absente).
func Open(path string) (*DB, error) {
	db := &DB{Version: 1, path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return db, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, db); err != nil {
		return nil, fmt.Errorf("base de références illisible : %w", err)
	}
	db.path = path
	return db, nil
}

// Save enregistre la base.
func (db *DB) Save() error {
	if err := os.MkdirAll(filepath.Dir(db.path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(db, "", "  ")
	return os.WriteFile(db.path, b, 0o644)
}

func norm(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// entriesOf extrait les mesures d'un rapport de bench.
func entriesOf(r *report.Report) []Entry {
	var out []Entry
	if r.CPU != nil && r.CPU.Bench != nil {
		e := Entry{Date: r.Created, ReportID: r.ID, Kind: "cpu", Model: r.System.CPUModel,
			Detail: fmt.Sprintf("%d threads, %s", r.CPU.Bench.Threads, r.CPU.Bench.Kernel), Scores: map[string]float64{}}
		for _, s := range r.CPU.Bench.Scores {
			e.Scores[s.Name+" (1)"] = s.Single
			if s.Multi > 0 {
				e.Scores[s.Name+" (N)"] = s.Multi
			}
		}
		if r.RAM != nil && r.RAM.BandwidthGBs > 0 {
			e.Scores["ram bandwidth"] = r.RAM.BandwidthGBs
		}
		out = append(out, e)
	}
	for _, d := range r.Disks {
		if d.Device == nil || d.Device.Model == "" || d.SeqReadMBs <= 0 {
			continue
		}
		e := Entry{Date: r.Created, ReportID: r.ID, Kind: "disk", Model: d.Device.Model, Detail: d.Device.Bus, Scores: map[string]float64{
			"seq read": d.SeqReadMBs, "seq write": d.SeqWriteMBs,
		}}
		for _, x := range d.Random {
			e.Scores[x.Name] = x.IOPS
		}
		out = append(out, e)
	}
	return out
}

func median(v []float64) float64 {
	sort.Float64s(v)
	if len(v)%2 == 1 {
		return v[len(v)/2]
	}
	return (v[len(v)/2-1] + v[len(v)/2]) / 2
}

// Compare compare un rapport aux références existantes du même modèle.
func (db *DB) Compare(r *report.Report) []report.RefComparison {
	var out []report.RefComparison
	for _, e := range entriesOf(r) {
		prev := map[string][]float64{}
		for _, x := range db.Entries {
			if x.Kind == e.Kind && norm(x.Model) == norm(e.Model) && x.ReportID != r.ID && x.Detail == e.Detail {
				for k, v := range x.Scores {
					prev[k] = append(prev[k], v)
				}
			}
		}
		keys := make([]string, 0, len(e.Scores))
		for k := range e.Scores {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := prev[k]
			if len(p) == 0 {
				continue
			}
			ref := median(append([]float64(nil), p...))
			if ref <= 0 {
				continue
			}
			out = append(out, report.RefComparison{Name: e.Kind + " " + k, Value: e.Scores[k], RefValue: ref, Count: len(p), DeltaPct: 100 * (e.Scores[k] - ref) / ref})
		}
	}
	return out
}

// Add ajoute les mesures d'un rapport de bench (sans doublon).
func (db *DB) Add(r *report.Report) int {
	n := 0
	for _, e := range entriesOf(r) {
		dup := false
		for _, x := range db.Entries {
			if x.ReportID == e.ReportID && x.Kind == e.Kind && x.Model == e.Model {
				dup = true
			}
		}
		if !dup {
			db.Entries = append(db.Entries, e)
			n++
		}
	}
	return n
}

// Import fusionne une autre base (fichier exporté).
func (db *DB) Import(path string) (int, error) {
	other, err := Open(path)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range other.Entries {
		dup := false
		for _, x := range db.Entries {
			if x.ReportID == e.ReportID && x.Kind == e.Kind && x.Model == e.Model {
				dup = true
			}
		}
		if !dup {
			db.Entries = append(db.Entries, e)
			n++
		}
	}
	return n, nil
}

// Export écrit la base vers path.
func (db *DB) Export(path string) error {
	cp := *db
	cp.path = path
	return cp.Save()
}

// Models résume la base : modèle → nombre de mesures.
func (db *DB) Models() map[string]int {
	m := map[string]int{}
	for _, e := range db.Entries {
		m[e.Kind+" · "+e.Model]++
	}
	return m
}
