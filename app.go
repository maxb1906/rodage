package main

// Liaison entre l'interface graphique (Wails) et le moteur Rodage.
// Toute la logique de test reste dans internal/ : ce fichier ne fait que
// relayer les appels et les événements.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/maxb1906/rodage/internal/cpu/kernels"
	"github.com/maxb1906/rodage/internal/disk"
	"github.com/maxb1906/rodage/internal/engine"
	"github.com/maxb1906/rodage/internal/i18n"
	"github.com/maxb1906/rodage/internal/netbench"
	"github.com/maxb1906/rodage/internal/report"
	"github.com/maxb1906/rodage/internal/safety"
	"github.com/maxb1906/rodage/internal/sensors"
	"github.com/maxb1906/rodage/internal/sysinfo"
)

// App est l'objet exposé au frontend.
type App struct {
	ctx  context.Context
	lang i18n.Lang

	mu        sync.Mutex
	cancel    context.CancelFunc
	running   bool
	srvCancel context.CancelFunc
	srvLog    []string
}

// NewApp crée l'application.
func NewApp() *App { return &App{lang: i18n.Detect()} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

func (a *App) shutdown(ctx context.Context) {
	a.StopRun()
	a.StopServer()
}

// ----- Vues renvoyées au frontend -----

// KV est une paire libellé / valeur.
type KV struct {
	K string `json:"k"`
	V string `json:"v"`
}

// DiskView décrit un disque pour l'interface.
type DiskView struct {
	disk.Device
	SizeText    string `json:"size_text"`
	RawEligible bool   `json:"raw_eligible"`
}

// MachineView est la configuration détectée.
type MachineView struct {
	Lang          string     `json:"lang"`
	Version       string     `json:"version"`
	Config        []KV       `json:"config"`
	SensorLevel   int        `json:"sensor_level"`
	SensorText    string     `json:"sensor_text"`
	SensorDetails []string   `json:"sensor_details"`
	TempC         float64    `json:"temp_c"`
	FreqMHz       float64    `json:"freq_mhz"`
	Kernels       []string   `json:"kernels"`
	Disks         []DiskView `json:"disks"`
	HasBattery    bool       `json:"has_battery"`
	Admin         bool       `json:"admin"`
	PrevCrash     string     `json:"prev_crash"`
	TempDir       string     `json:"temp_dir"`
	ReportsDir    string     `json:"reports_dir"`
	Addresses     []string   `json:"addresses"`
}

// FindingView est un constat traduit.
type FindingView struct {
	Sev  string `json:"sev"`
	Comp string `json:"comp"`
	Text string `json:"text"`
}

// ReportView est le résumé d'un rapport.
type ReportView struct {
	ID          string        `json:"id"`
	Created     string        `json:"created"`
	Mode        string        `json:"mode"`
	ModeText    string        `json:"mode_text"`
	Verdict     string        `json:"verdict"`
	VerdictText string        `json:"verdict_text"`
	VerdictLong string        `json:"verdict_long"`
	Findings    []FindingView `json:"findings"`
	Limits      []string      `json:"limits"`
	Elapsed     string        `json:"elapsed"`
	Host        string        `json:"host"`
	Path        string        `json:"path"`
}

// DiffView est une comparaison de deux rapports.
type DiffView struct {
	Rows     []report.DiffRow `json:"rows"`
	Mismatch string           `json:"mismatch"`
	A, B     ReportView       `json:"-"`
	HTMLPath string           `json:"html_path"`
}

func reportsDir() string { return filepath.Join(safety.StateDir(), "reports") }

func localAddresses() []string {
	var out []string
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, ad := range addrs {
			if n, ok := ad.(*net.IPNet); ok && n.IP.To4() != nil {
				out = append(out, n.IP.String())
			}
		}
	}
	return out
}

// SetLang change la langue de l'interface (fr / en).
func (a *App) SetLang(l string) { a.lang = i18n.Parse(l) }

// GetMachine renvoie la configuration détectée.
func (a *App) GetMachine() MachineView {
	in := sysinfo.Collect()
	src := sensors.Open()
	defer src.Close()
	si := sensors.Describe(src)
	s := src.Read()
	time.Sleep(300 * time.Millisecond)
	if s2 := src.Read(); s2.FreqMHz == s2.FreqMHz {
		s = s2
	}
	r := &report.Report{System: in, Mode: engine.ModeBench, Profile: engine.ProfileBench}
	v := MachineView{
		Lang:          string(a.lang),
		Version:       engine.Version,
		SensorLevel:   int(si.Level),
		SensorText:    i18n.T(a.lang, fmt.Sprintf("lim.sensors%d", si.Level)),
		SensorDetails: si.Details,
		TempC:         -1,
		FreqMHz:       -1,
		HasBattery:    in.HasBattery,
		Admin:         in.Admin,
		TempDir:       os.TempDir(),
		ReportsDir:    reportsDir(),
		Addresses:     localAddresses(),
	}
	for _, row := range r.ConfigRows(a.lang)[:10] {
		v.Config = append(v.Config, KV{row.K, row.V})
	}
	if s.CPUTempC == s.CPUTempC {
		v.TempC = s.CPUTempC
	}
	if s.FreqMHz == s.FreqMHz {
		v.FreqMHz = s.FreqMHz
	}
	for _, k := range kernels.Available() {
		v.Kernels = append(v.Kernels, string(k.Level))
	}
	devs, _ := disk.ListDevices()
	for _, d := range devs {
		v.Disks = append(v.Disks, DiskView{Device: d, SizeText: report.HumanBytesL(a.lang, d.Size),
			RawEligible: !d.System && d.Partitions == 0 && !d.Mounted && d.Serial != ""})
	}
	if b, err := os.ReadFile(filepath.Join(safety.StateDir(), "running.json")); err == nil && len(b) > 0 && !a.isRunning() {
		v.PrevCrash = i18n.T(a.lang, "cli.prevcrash")
	}
	return v
}

func (a *App) isRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

// IsRunning indique si un test est en cours.
func (a *App) IsRunning() bool { return a.isRunning() }

// DefaultPlan renvoie un plan par défaut pour un mode et un profil.
func (a *App) DefaultPlan(mode, profile string) engine.Plan {
	p := engine.DefaultPlan(mode, profile)
	p.Lang = string(a.lang)
	return p
}

// Budget renvoie la répartition prévue (en secondes) d'un plan.
func (a *App) Budget(p engine.Plan) map[string]float64 {
	out := map[string]float64{}
	for k, d := range p.Budget() {
		out[k] = d.Seconds()
	}
	return out
}

// ValidatePlan renvoie l'erreur de validation (chaîne vide si valide).
func (a *App) ValidatePlan(p engine.Plan) string {
	if err := p.Validate(); err != nil {
		return err.Error()
	}
	if p.DiskRaw != "" {
		devs, _ := disk.ListDevices()
		for _, d := range devs {
			if d.Name == p.DiskRaw || d.Path == p.DiskRaw {
				if err := disk.CheckRawSafe(d, p.ConfirmSerial); err != nil {
					return err.Error()
				}
				return ""
			}
		}
		return "périphérique introuvable"
	}
	return ""
}

// StartRun démarre un test ; les événements arrivent sur « rodage:event »
// et le résultat sur « rodage:done ».
func (a *App) StartRun(p engine.Plan) error {
	if msg := a.ValidatePlan(p); msg != "" {
		return errors.New(msg)
	}
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return errors.New("un test est déjà en cours")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel, a.running = cancel, true
	a.mu.Unlock()
	p.Lang = string(a.lang)
	go func() {
		defer func() {
			a.mu.Lock()
			a.running, a.cancel = false, nil
			a.mu.Unlock()
		}()
		rep, err := engine.Run(ctx, p, func(e engine.Event) {
			runtime.EventsEmit(a.ctx, "rodage:event", e)
		})
		if err != nil {
			runtime.EventsEmit(a.ctx, "rodage:error", err.Error())
			return
		}
		path, err := a.saveReport(rep)
		if err != nil {
			runtime.EventsEmit(a.ctx, "rodage:error", err.Error())
		}
		v := a.view(rep, path)
		runtime.EventsEmit(a.ctx, "rodage:done", v)
	}()
	return nil
}

// StopRun arrête le test en cours (le rapport partiel est conservé).
func (a *App) StopRun() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *App) saveReport(r *report.Report) (string, error) {
	dir := reportsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := filepath.Join(dir, "rodage-"+r.ID)
	if err := r.Save(base + ".json"); err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := r.WriteHTML(&b, a.lang); err == nil {
		os.WriteFile(base+".html", b.Bytes(), 0o644)
	}
	return base + ".json", nil
}

func (a *App) view(r *report.Report, path string) ReportView {
	l := a.lang
	v := ReportView{
		ID:          r.ID,
		Created:     r.Created.Local().Format("2006-01-02 15:04"),
		Mode:        r.Mode,
		ModeText:    i18n.T(l, "mode."+r.Mode) + " · " + i18n.T(l, "profile."+r.Profile),
		Verdict:     string(r.Verdict),
		VerdictText: i18n.T(l, "verdict."+string(r.Verdict)),
		VerdictLong: i18n.T(l, "verdict."+string(r.Verdict)+".long"),
		Elapsed:     report.HumanDuration(r.Elapsed),
		Host:        r.System.Hostname,
		Path:        path,
	}
	for _, f := range r.Findings {
		v.Findings = append(v.Findings, FindingView{Sev: string(f.Severity), Comp: i18n.T(l, "comp."+f.Component), Text: f.Text(l)})
	}
	for _, k := range r.Limitations {
		v.Limits = append(v.Limits, i18n.T(l, k))
	}
	return v
}

// ListReports liste les rapports enregistrés, du plus récent au plus ancien.
func (a *App) ListReports() []ReportView {
	files, _ := filepath.Glob(filepath.Join(reportsDir(), "rodage-*.json"))
	var out []ReportView
	for _, f := range files {
		r, err := report.Load(f)
		if err != nil {
			continue
		}
		out = append(out, a.view(r, f))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// GetReport relit un rapport dans la langue courante.
func (a *App) GetReport(path string) (ReportView, error) {
	r, err := report.Load(path)
	if err != nil {
		return ReportView{}, err
	}
	return a.view(r, path), nil
}

// OpenReportHTML régénère le HTML dans la langue courante et l'ouvre.
func (a *App) OpenReportHTML(path string) error {
	r, err := report.Load(path)
	if err != nil {
		return err
	}
	out := strings.TrimSuffix(path, ".json") + "." + string(a.lang) + ".html"
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	err = r.WriteHTML(f, a.lang)
	f.Close()
	if err != nil {
		return err
	}
	runtime.BrowserOpenURL(a.ctx, "file://"+filepath.ToSlash(out))
	return nil
}

// ExportPDF demande un emplacement et écrit le PDF du rapport.
func (a *App) ExportPDF(path string) (string, error) {
	r, err := report.Load(path)
	if err != nil {
		return "", err
	}
	dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "rodage-" + r.ID + ".pdf",
		Filters:         []runtime.FileFilter{{DisplayName: "PDF", Pattern: "*.pdf"}},
	})
	if err != nil || dest == "" {
		return "", err
	}
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return dest, r.WritePDF(f, a.lang)
}

// ExportJSON copie le rapport JSON à l'emplacement choisi.
func (a *App) ExportJSON(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{DefaultFilename: filepath.Base(path),
		Filters: []runtime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}}})
	if err != nil || dest == "" {
		return "", err
	}
	return dest, os.WriteFile(dest, b, 0o644)
}

// ImportReport ajoute un rapport JSON externe (ex. d'une autre machine).
func (a *App) ImportReport() (string, error) {
	src, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Filters: []runtime.FileFilter{{DisplayName: "Rodage JSON", Pattern: "*.json"}}})
	if err != nil || src == "" {
		return "", err
	}
	r, err := report.Load(src)
	if err != nil {
		return "", err
	}
	os.MkdirAll(reportsDir(), 0o755)
	dest := filepath.Join(reportsDir(), "rodage-"+r.ID+".json")
	return dest, r.Save(dest)
}

// DeleteReport supprime un rapport enregistré.
func (a *App) DeleteReport(path string) error {
	if filepath.Dir(path) != reportsDir() {
		return errors.New("chemin hors du dossier des rapports")
	}
	base := strings.TrimSuffix(path, ".json")
	matches, _ := filepath.Glob(base + "*")
	for _, m := range matches {
		os.Remove(m)
	}
	return nil
}

// OpenReportsFolder ouvre le dossier des rapports.
func (a *App) OpenReportsFolder() {
	os.MkdirAll(reportsDir(), 0o755)
	runtime.BrowserOpenURL(a.ctx, "file://"+filepath.ToSlash(reportsDir()))
}

// Compare compare deux rapports et prépare un HTML.
func (a *App) Compare(pathA, pathB string) (DiffView, error) {
	ra, err := report.Load(pathA)
	if err != nil {
		return DiffView{}, err
	}
	rb, err := report.Load(pathB)
	if err != nil {
		return DiffView{}, err
	}
	// A = le plus ancien.
	if ra.Created.After(rb.Created) {
		ra, rb = rb, ra
	}
	d := report.Compare(ra, rb)
	v := DiffView{Rows: d.Rows}
	if len(d.Mismatch) > 0 {
		v.Mismatch = i18n.T(a.lang, "r.mismatch", strings.Join(d.Mismatch, ", "))
	}
	out := filepath.Join(reportsDir(), fmt.Sprintf("compare-%s-%s.html", ra.ID, rb.ID))
	if f, err := os.Create(out); err == nil {
		d.WriteHTML(f, a.lang)
		f.Close()
		v.HTMLPath = out
	}
	return v, nil
}

// OpenFile ouvre un fichier local dans le navigateur.
func (a *App) OpenFile(path string) {
	runtime.BrowserOpenURL(a.ctx, "file://"+filepath.ToSlash(path))
}

// ChooseDirectory ouvre un sélecteur de dossier.
func (a *App) ChooseDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{CanCreateDirectories: true})
}

// ----- Serveur réseau -----

// StartServer lance le serveur pour le test réseau.
func (a *App) StartServer(addr string) error {
	a.mu.Lock()
	if a.srvCancel != nil {
		a.mu.Unlock()
		return errors.New("serveur déjà démarré")
	}
	if addr == "" {
		addr = fmt.Sprintf(":%d", netbench.DefaultPort)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.srvCancel = cancel
	a.srvLog = nil
	a.mu.Unlock()
	w := logWriter{a}
	s := &netbench.Server{Version: engine.Version, Log: newLogger(w)}
	errc := make(chan error, 1)
	go func() {
		errc <- s.ListenAndServe(ctx, addr)
		a.mu.Lock()
		a.srvCancel = nil
		a.mu.Unlock()
		runtime.EventsEmit(a.ctx, "rodage:server", false)
	}()
	select {
	case err := <-errc:
		if err != nil {
			return err
		}
	case <-time.After(300 * time.Millisecond):
	}
	runtime.EventsEmit(a.ctx, "rodage:server", true)
	return nil
}

// StopServer arrête le serveur réseau.
func (a *App) StopServer() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.srvCancel != nil {
		a.srvCancel()
		a.srvCancel = nil
	}
}

// ServerRunning indique si le serveur est actif.
func (a *App) ServerRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.srvCancel != nil
}

type logWriter struct{ a *App }

func (w logWriter) Write(p []byte) (int, error) {
	line := strings.TrimSpace(string(p))
	w.a.mu.Lock()
	w.a.srvLog = append(w.a.srvLog, line)
	if len(w.a.srvLog) > 200 {
		w.a.srvLog = w.a.srvLog[len(w.a.srvLog)-200:]
	}
	w.a.mu.Unlock()
	runtime.EventsEmit(w.a.ctx, "rodage:serverlog", line)
	return len(p), nil
}
