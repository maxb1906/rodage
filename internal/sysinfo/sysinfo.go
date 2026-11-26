// Package sysinfo détecte la configuration de la machine testée.
package sysinfo

import (
	"os"
	"runtime"
	"strings"

	"github.com/maxb1906/rodage/internal/cpu/kernels"
)

// SMBIOS reprend les champs d'identification du firmware.
type SMBIOS struct {
	Manufacturer string `json:"manufacturer,omitempty"`
	Product      string `json:"product,omitempty"`
	Serial       string `json:"serial,omitempty"`
	BoardVendor  string `json:"board_vendor,omitempty"`
	BoardProduct string `json:"board_product,omitempty"`
	BoardSerial  string `json:"board_serial,omitempty"`
	BIOSVendor   string `json:"bios_vendor,omitempty"`
	BIOSVersion  string `json:"bios_version,omitempty"`
	BIOSDate     string `json:"bios_date,omitempty"`
}

// Info décrit la machine.
type Info struct {
	Hostname     string   `json:"hostname"`
	OS           string   `json:"os"`
	OSVersion    string   `json:"os_version"`
	Arch         string   `json:"arch"`
	CPUModel     string   `json:"cpu_model"`
	CPUVendor    string   `json:"cpu_vendor,omitempty"`
	LogicalCPUs  int      `json:"logical_cpus"`
	SIMD         []string `json:"simd"`
	RAMTotal     uint64   `json:"ram_total"`
	RAMAvailable uint64   `json:"ram_available"`
	SMBIOS       SMBIOS   `json:"smbios"`
	HasBattery   bool     `json:"has_battery"`
	Admin        bool     `json:"admin"`
}

// Collect rassemble les informations disponibles sans droits particuliers.
func Collect() Info {
	h, _ := os.Hostname()
	in := Info{
		Hostname:    h,
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		LogicalCPUs: runtime.NumCPU(),
		Admin:       IsAdmin(),
	}
	for _, k := range kernels.Available() {
		in.SIMD = append(in.SIMD, string(k.Level))
	}
	collectPlatform(&in)
	in.CPUModel = strings.TrimSpace(in.CPUModel)
	if in.CPUModel == "" {
		in.CPUModel = "CPU " + runtime.GOARCH
	}
	return in
}

// MemAvailable renvoie la mémoire disponible actuelle, en octets.
func MemAvailable() uint64 { return memAvailable() }

// clean supprime les valeurs de remplissage courantes des firmwares.
func clean(s string) string {
	s = strings.TrimSpace(strings.Trim(s, "\x00"))
	switch strings.ToLower(s) {
	case "", "to be filled by o.e.m.", "default string", "system serial number",
		"not specified", "none", "n/a", "0", "0123456789", "system product name",
		"system manufacturer", "o.e.m.":
		return ""
	}
	return s
}
