// Package disk mesure et vérifie les disques.
//
// Mode fichier (défaut) : un fichier de test en E/S directes, sans risque pour
// les données. Mode brut (destructif) : accès au périphérique entier, réservé
// aux disques sans partition, jamais au disque système, après confirmation
// par saisie du numéro de série.
package disk

import (
	"errors"
	"time"
)

// Device décrit un disque physique.
type Device struct {
	Name       string `json:"name"` // nvme0n1, sda, PhysicalDrive0…
	Path       string `json:"path"` // /dev/nvme0n1, \\.\PhysicalDrive0
	Model      string `json:"model,omitempty"`
	Serial     string `json:"serial,omitempty"`
	Firmware   string `json:"firmware,omitempty"`
	Size       uint64 `json:"size"`
	Bus        string `json:"bus,omitempty"` // nvme, sata, usb…
	Rotational bool   `json:"rotational,omitempty"`
	Removable  bool   `json:"removable,omitempty"`
	Partitions int    `json:"partitions"`
	System     bool   `json:"system"`
	Mounted    bool   `json:"mounted,omitempty"`
}

// Health est l'état SMART / NVMe d'un disque. -1 = inconnu.
type Health struct {
	Available       bool    `json:"available"`
	Protocol        string  `json:"protocol,omitempty"` // nvme, ata
	Note            string  `json:"note,omitempty"`
	TempC           float64 `json:"temp_c"`
	CriticalWarning int     `json:"critical_warning"`
	PercentUsed     int     `json:"percent_used"`
	AvailableSpare  int     `json:"available_spare"`
	MediaErrors     int64   `json:"media_errors"`
	ErrorLogEntries int64   `json:"error_log_entries"`
	PowerOnHours    int64   `json:"power_on_hours"`
	PowerCycles     int64   `json:"power_cycles"`
	UnsafeShutdowns int64   `json:"unsafe_shutdowns"`
	DataWritten     uint64  `json:"data_written"`
	DataRead        uint64  `json:"data_read"`
	Reallocated     int64   `json:"reallocated"`
	Pending         int64   `json:"pending"`
	Uncorrectable   int64   `json:"uncorrectable"`
	CRCErrors       int64   `json:"crc_errors"`
}

func unknownHealth() Health {
	return Health{TempC: -1, CriticalWarning: -1, PercentUsed: -1, AvailableSpare: -1,
		MediaErrors: -1, ErrorLogEntries: -1, PowerOnHours: -1, PowerCycles: -1,
		UnsafeShutdowns: -1, Reallocated: -1, Pending: -1, Uncorrectable: -1, CRCErrors: -1}
}

// Degradation compare deux relevés et liste les dégradations.
func Degradation(before, after Health) []string {
	if !before.Available || !after.Available {
		return nil
	}
	var out []string
	inc := func(name string, a, b int64) {
		if a >= 0 && b > a {
			out = append(out, name)
		}
	}
	inc("media_errors", before.MediaErrors, after.MediaErrors)
	inc("reallocated", before.Reallocated, after.Reallocated)
	inc("pending", before.Pending, after.Pending)
	inc("uncorrectable", before.Uncorrectable, after.Uncorrectable)
	inc("crc_errors", before.CRCErrors, after.CRCErrors)
	if after.CriticalWarning > 0 && after.CriticalWarning != before.CriticalWarning {
		out = append(out, "critical_warning")
	}
	return out
}

// Problems liste les signaux d'alerte d'un relevé isolé.
func (h Health) Problems() []string {
	if !h.Available {
		return nil
	}
	var out []string
	if h.CriticalWarning > 0 {
		out = append(out, "critical_warning")
	}
	if h.MediaErrors > 0 {
		out = append(out, "media_errors")
	}
	if h.Reallocated > 0 {
		out = append(out, "reallocated")
	}
	if h.Pending > 0 {
		out = append(out, "pending")
	}
	if h.Uncorrectable > 0 {
		out = append(out, "uncorrectable")
	}
	if h.PercentUsed >= 90 {
		out = append(out, "worn")
	}
	if h.AvailableSpare >= 0 && h.AvailableSpare < 10 {
		out = append(out, "spare_low")
	}
	return out
}

// RandResult est une mesure aléatoire 4K.
type RandResult struct {
	Name  string  `json:"name"` // ex. read-4k-qd1
	QD    int     `json:"qd"`
	Write bool    `json:"write"`
	IOPS  float64 `json:"iops"`
	MBs   float64 `json:"mbs"`
	AvgUs float64 `json:"avg_us"`
	P50Us float64 `json:"p50_us"`
	P99Us float64 `json:"p99_us"`
	MaxUs float64 `json:"max_us"`
}

// SLC décrit la chute de débit après saturation du cache d'écriture.
type SLC struct {
	Detected  bool    `json:"detected"`
	AfterGB   float64 `json:"after_gb"`
	BeforeMBs float64 `json:"before_mbs"`
	AfterMBs  float64 `json:"after_mbs"`
}

// Result est le bilan d'un disque.
type Result struct {
	Mode         string        `json:"mode"` // file, raw
	Path         string        `json:"path"`
	Device       *Device       `json:"device,omitempty"`
	Direct       bool          `json:"direct"`
	Note         string        `json:"note,omitempty"`
	FileSize     uint64        `json:"file_size"`
	WriteCap     uint64        `json:"write_cap"`
	Written      uint64        `json:"written"`
	ReadBytes    uint64        `json:"read"`
	Passes       int           `json:"passes"`
	Duration     time.Duration `json:"duration"`
	SeqWriteMBs  float64       `json:"seq_write_mbs"`
	SeqReadMBs   float64       `json:"seq_read_mbs"`
	Random       []RandResult  `json:"random"`
	WriteCurve   []float64     `json:"write_curve"` // Mo/s par seconde
	SLC          SLC           `json:"slc"`
	VerifyErrors uint64        `json:"verify_errors"`
	IOErrors     uint64        `json:"io_errors"`
	ErrorLog     []string      `json:"error_log,omitempty"`
	HealthBefore Health        `json:"health_before"`
	HealthAfter  Health        `json:"health_after"`
	Degraded     []string      `json:"degraded,omitempty"`
	Aborted      string        `json:"aborted,omitempty"`
}

// ErrUnsafe signale un refus du mode brut.
var ErrUnsafe = errors.New("disque refusé pour le mode brut")
