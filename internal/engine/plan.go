// Package engine orchestre les tests ; la CLI et la GUI l'appellent tous deux.
package engine

import (
	"fmt"
	"time"
)

// Modes et profils.
const (
	ModeBench  = "bench"
	ModeStress = "stress"

	ProfileQuick    = "quick"
	ProfileStandard = "standard"
	ProfileFull     = "full"
	ProfileCustom   = "custom"
	ProfileBench    = "bench"
)

const (
	GiB = uint64(1) << 30
)

// Plan décrit un test à exécuter.
type Plan struct {
	Mode     string        `json:"mode"`
	Profile  string        `json:"profile"`
	Duration time.Duration `json:"duration"` // profils full et custom
	CPU      bool          `json:"cpu"`
	RAM      bool          `json:"ram"`
	Disk     bool          `json:"disk"`
	Net      bool          `json:"net"`

	Kernel   string `json:"kernel,omitempty"` // niveau SIMD imposé
	RAMBytes uint64 `json:"ram_bytes,omitempty"`

	DiskDir       string `json:"disk_dir,omitempty"`
	DiskRaw       string `json:"disk_raw,omitempty"` // nom ou chemin du périphérique (destructif)
	ConfirmSerial string `json:"confirm_serial,omitempty"`
	DiskWriteCap  uint64 `json:"disk_write_cap,omitempty"`
	DiskFileSize  uint64 `json:"disk_file_size,omitempty"`

	NetServer  string `json:"net_server,omitempty"`
	NetStreams int    `json:"net_streams,omitempty"`

	ThermalMargin float64 `json:"thermal_margin,omitempty"`
	AllowBattery  bool    `json:"allow_battery,omitempty"`
	Lang          string  `json:"lang,omitempty"`
}

// DefaultPlan renvoie des réglages sûrs.
func DefaultPlan(mode, profile string) Plan {
	p := Plan{Mode: mode, Profile: profile, CPU: true, RAM: true, Disk: true, ThermalMargin: 5}
	if mode == ModeBench {
		p.Profile = ProfileBench
	}
	if profile == ProfileFull {
		p.Duration = 8 * time.Hour
	}
	return p
}

// Total renvoie la durée prévue.
func (p Plan) Total() time.Duration {
	switch p.Profile {
	case ProfileQuick:
		return 10 * time.Minute
	case ProfileStandard:
		return time.Hour
	case ProfileBench:
		return 0
	}
	return p.Duration
}

// WriteCap renvoie le plafond d'écriture disque.
func (p Plan) WriteCap() uint64 {
	if p.DiskWriteCap > 0 {
		return p.DiskWriteCap
	}
	switch p.Profile {
	case ProfileBench:
		return 4 * GiB
	case ProfileQuick:
		return 8 * GiB
	case ProfileFull:
		return 200 * GiB
	}
	return 50 * GiB
}

// Validate vérifie la cohérence du plan.
func (p Plan) Validate() error {
	switch p.Mode {
	case ModeBench, ModeStress:
	default:
		return fmt.Errorf("mode inconnu : %q (bench ou stress)", p.Mode)
	}
	if p.Mode == ModeStress {
		switch p.Profile {
		case ProfileQuick, ProfileStandard:
		case ProfileFull:
			if p.Duration < 4*time.Hour || p.Duration > 12*time.Hour {
				return fmt.Errorf("rodage complet : durée entre 4 et 12 h")
			}
		case ProfileCustom:
			if p.Duration < 30*time.Second {
				return fmt.Errorf("durée personnalisée trop courte (30 s minimum)")
			}
		default:
			return fmt.Errorf("profil inconnu : %q", p.Profile)
		}
	}
	if !p.CPU && !p.RAM && !p.Disk && !p.Net {
		return fmt.Errorf("aucun test sélectionné")
	}
	if p.Net && p.NetServer == "" {
		return fmt.Errorf("test réseau : adresse du serveur requise")
	}
	if p.DiskRaw != "" && p.ConfirmSerial == "" {
		return fmt.Errorf("mode disque brut : saisissez le numéro de série pour confirmer")
	}
	if p.ThermalMargin < 0 || p.ThermalMargin > 40 {
		return fmt.Errorf("marge thermique entre 0 et 40 °C")
	}
	return nil
}

// Budget répartit la durée entre les tests sélectionnés.
func (p Plan) Budget() map[string]time.Duration {
	weights := map[string]float64{"cpu": 40, "ram": 35, "disk": 15, "net": 10}
	switch p.Profile {
	case ProfileQuick:
		weights = map[string]float64{"cpu": 4, "ram": 3, "disk": 2, "net": 1}
	case ProfileStandard:
		weights = map[string]float64{"cpu": 25, "ram": 20, "disk": 10, "net": 5}
	}
	on := map[string]bool{"cpu": p.CPU, "ram": p.RAM, "disk": p.Disk, "net": p.Net}
	var sum float64
	for k, w := range weights {
		if on[k] {
			sum += w
		}
	}
	out := map[string]time.Duration{}
	total := p.Total()
	for k, w := range weights {
		if on[k] {
			out[k] = time.Duration(float64(total) * w / sum).Round(time.Second)
		}
	}
	return out
}
