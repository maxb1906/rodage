// Commande rodage-cli : interface en ligne de commande de Rodage
// (automatisation et mode serveur réseau). Elle appelle le même moteur que la GUI.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/maxb1906/rodage/internal/cpu/kernels"
	"github.com/maxb1906/rodage/internal/disk"
	"github.com/maxb1906/rodage/internal/engine"
	"github.com/maxb1906/rodage/internal/i18n"
	"github.com/maxb1906/rodage/internal/netbench"
	"github.com/maxb1906/rodage/internal/refdb"
	"github.com/maxb1906/rodage/internal/report"
	"github.com/maxb1906/rodage/internal/safety"
	"github.com/maxb1906/rodage/internal/sensors"
	"github.com/maxb1906/rodage/internal/sysinfo"
)

const (
	exitPass       = 0
	exitFail       = 1
	exitIncomplete = 2
	exitUsage      = 3
)

var lang = i18n.Detect()

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, i18n.T(lang, "cli.usage"))
		os.Exit(exitUsage)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var code int
	switch cmd {
	case "info":
		code = cmdInfo(args)
	case "bench":
		code = cmdRun(engine.ModeBench, args)
	case "stress":
		code = cmdRun(engine.ModeStress, args)
	case "server", "serveur":
		code = cmdServer(args)
	case "report", "rapport":
		code = cmdReport(args)
	case "compare", "comparer":
		code = cmdCompare(args)
	case "disks", "disques":
		code = cmdDisks(args)
	case "refs":
		code = cmdRefs(args)
	case "version", "-v", "--version":
		fmt.Println("rodage", engine.Version)
	case "help", "-h", "--help", "aide":
		fmt.Println(i18n.T(lang, "cli.usage"))
	default:
		fmt.Fprintln(os.Stderr, i18n.T(lang, "cli.usage"))
		code = exitUsage
	}
	os.Exit(code)
}

func fatal(err error) int {
	fmt.Fprintln(os.Stderr, "rodage:", err)
	return exitUsage
}

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Func("lang", "fr | en", func(s string) error { lang = i18n.Parse(s); return nil })
	return fs
}

// parseSize lit « 512M », « 50G », « 1T » ou un nombre d'octets.
func parseSize(s string) (uint64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	s = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(s, "IB"), "O"), "B")
	mult := uint64(1)
	if n := len(s); n > 0 {
		switch s[n-1] {
		case 'K':
			mult, s = 1<<10, s[:n-1]
		case 'M':
			mult, s = 1<<20, s[:n-1]
		case 'G':
			mult, s = 1<<30, s[:n-1]
		case 'T':
			mult, s = 1<<40, s[:n-1]
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("taille invalide : %q", s)
	}
	return uint64(v * float64(mult)), nil
}

type sizeFlag struct{ v *uint64 }

func (f sizeFlag) String() string {
	if f.v == nil || *f.v == 0 {
		return ""
	}
	return report.HumanBytes(*f.v)
}
func (f sizeFlag) Set(s string) error {
	v, err := parseSize(s)
	*f.v = v
	return err
}

// ----- info -----

func cmdInfo(args []string) int {
	fs := newFlags("info")
	asJSON := fs.Bool("json", false, "sortie JSON")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	in := sysinfo.Collect()
	src := sensors.Open()
	defer src.Close()
	si := sensors.Describe(src)
	s := src.Read()
	devs, _ := disk.ListDevices()
	if *asJSON {
		json.NewEncoder(os.Stdout).Encode(map[string]any{"system": in, "sensors": si, "sample": s, "disks": devs})
		return 0
	}
	r := &report.Report{System: in, Mode: "bench", Profile: "bench"}
	for _, row := range r.ConfigRows(lang)[:10] {
		fmt.Printf("%-24s %s\n", row.K, row.V)
	}
	fmt.Printf("%-24s %s — %s %s\n", i18n.T(lang, "r.confidence"), i18n.T(lang, "r.level", int(si.Level)), i18n.T(lang, fmt.Sprintf("conf.%d", si.Level)), strings.Join(si.Details, " ; "))
	if s.CPUTempC == s.CPUTempC {
		fmt.Printf("%-24s %.0f °C (TjMax %.0f)\n", i18n.T(lang, "r.temp"), s.CPUTempC, si.TjMax)
	}
	if s.FreqMHz == s.FreqMHz {
		fmt.Printf("%-24s %.0f MHz\n", i18n.T(lang, "r.freq"), s.FreqMHz)
	}
	fmt.Println()
	printDisks(devs)
	return 0
}

func printDisks(devs []disk.Device) {
	for _, d := range devs {
		flags := []string{d.Bus}
		if d.System {
			flags = append(flags, "système/system")
		}
		if d.Partitions > 0 {
			flags = append(flags, fmt.Sprintf("%d partition(s)", d.Partitions))
		}
		if d.Mounted {
			flags = append(flags, "monté/mounted")
		}
		if d.Removable {
			flags = append(flags, "amovible/removable")
		}
		if !d.System && d.Partitions == 0 && !d.Mounted && d.Serial != "" {
			flags = append(flags, i18n.T(lang, "cli.raw.ok"))
		}
		fmt.Printf("%-16s %10s  %-32s %-22s %s\n", d.Name, report.HumanBytes(d.Size), d.Model, d.Serial, strings.Join(flags, ", "))
	}
}

func cmdDisks(args []string) int {
	fs := newFlags("disks")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	devs, err := disk.ListDevices()
	if err != nil {
		return fatal(err)
	}
	printDisks(devs)
	return 0
}

// ----- bench / stress -----

func cmdRun(mode string, args []string) int {
	fs := newFlags(mode)
	p := engine.DefaultPlan(mode, engine.ProfileStandard)
	profile := fs.String("profile", "standard", "quick | standard | full | custom")
	dur := fs.Duration("duration", 0, "durée (full : 4h à 12h, défaut 8h ; custom : obligatoire)")
	only := fs.String("tests", "cpu,ram,disk", "tests à lancer : cpu,ram,disk,net")
	fs.StringVar(&p.Kernel, "kernel", "", "noyau SIMD imposé : "+levels())
	fs.Var(sizeFlag{&p.RAMBytes}, "ram-size", "mémoire testée (ex. 1G ; défaut 87 % de la RAM libre)")
	fs.StringVar(&p.DiskDir, "disk-dir", "", "dossier du fichier de test disque (défaut : dossier temporaire)")
	fs.Var(sizeFlag{&p.DiskFileSize}, "disk-size", "taille du fichier de test (défaut automatique)")
	fs.Var(sizeFlag{&p.DiskWriteCap}, "write-cap", "plafond d'écriture disque (défaut : 8G rapide, 50G standard, 200G complet)")
	fs.StringVar(&p.DiskRaw, "disk-raw", "", "DESTRUCTIF : périphérique entier sans partition (ex. nvme1n1, PhysicalDrive2)")
	fs.StringVar(&p.ConfirmSerial, "confirm-serial", "", "numéro de série du disque, pour confirmer le mode brut")
	fs.StringVar(&p.NetServer, "net", "", "adresse du serveur Rodage pour le test réseau (active le test)")
	fs.IntVar(&p.NetStreams, "streams", 4, "flux TCP parallèles")
	fs.Float64Var(&p.ThermalMargin, "margin", 5, "marge thermique sous TjMax (°C)")
	fs.BoolVar(&p.AllowBattery, "allow-battery", false, "autoriser le stress sur batterie")
	out := fs.String("out", ".", "dossier des rapports")
	noPDF := fs.Bool("no-pdf", false, "ne pas produire de PDF")
	asJSON := fs.Bool("json", false, "événements JSON sur la sortie standard")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if mode == engine.ModeStress {
		p.Profile = *profile
		if *dur > 0 {
			p.Duration = *dur
		} else if p.Profile == engine.ProfileFull {
			p.Duration = 8 * time.Hour
		}
	}
	set := map[string]bool{}
	for _, t := range strings.Split(*only, ",") {
		set[strings.TrimSpace(t)] = true
	}
	p.CPU, p.RAM, p.Disk, p.Net = set["cpu"], set["ram"], set["disk"] || p.DiskRaw != "", set["net"] || p.NetServer != ""
	p.Lang = string(lang)
	if err := p.Validate(); err != nil && !(p.DiskRaw != "" && p.ConfirmSerial == "") {
		return fatal(err)
	}
	if p.DiskRaw != "" {
		if code, ok := confirmRaw(&p); !ok {
			return code
		}
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return fatal(err)
	}
	if m, ok := peekCrash(); ok {
		fmt.Fprintln(os.Stderr, i18n.T(lang, "cli.prevcrash"), m)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		fmt.Fprintln(os.Stderr, "\n"+i18n.T(lang, "cli.stopping"))
		cancel()
		<-sig
		os.Exit(exitIncomplete)
	}()

	ui := newProgressUI(*asJSON)
	rep, err := engine.Run(ctx, p, ui.handle)
	ui.finish()
	if err != nil {
		return fatal(err)
	}
	base := filepath.Join(*out, "rodage-"+rep.ID)
	if err := rep.Save(base + ".json"); err != nil {
		return fatal(err)
	}
	paths := []string{base + ".json"}
	if f, err := os.Create(base + ".html"); err == nil {
		rep.WriteHTML(f, lang)
		f.Close()
		paths = append(paths, base+".html")
	}
	if !*noPDF {
		if f, err := os.Create(base + ".pdf"); err == nil {
			if err := rep.WritePDF(f, lang); err != nil {
				fmt.Fprintln(os.Stderr, "PDF :", err)
			}
			f.Close()
			paths = append(paths, base+".pdf")
		}
	}
	printSummary(rep)
	for _, p := range paths {
		fmt.Fprintln(os.Stderr, i18n.T(lang, "cli.saved", p))
	}
	switch rep.Verdict {
	case report.Fail:
		return exitFail
	case report.Incomplete:
		return exitIncomplete
	}
	return exitPass
}

func peekCrash() (string, bool) {
	b, err := os.ReadFile(filepath.Join(safety.StateDir(), "running.json"))
	if err != nil {
		return "", false
	}
	var m safety.Marker
	if json.Unmarshal(b, &m) != nil {
		return "", false
	}
	return fmt.Sprintf("(%s, %s)", m.Started.Local().Format("2006-01-02 15:04"), m.Phase), true
}

func levels() string {
	var s []string
	for _, k := range kernels.Available() {
		s = append(s, string(k.Level))
	}
	return strings.Join(s, " | ")
}

// confirmRaw demande la saisie du numéro de série avant le mode brut.
func confirmRaw(p *engine.Plan) (int, bool) {
	devs, err := disk.ListDevices()
	if err != nil {
		return fatal(err), false
	}
	var d *disk.Device
	for i := range devs {
		if devs[i].Name == p.DiskRaw || devs[i].Path == p.DiskRaw {
			d = &devs[i]
		}
	}
	if d == nil {
		return fatal(fmt.Errorf("périphérique %s introuvable", p.DiskRaw)), false
	}
	if err := disk.CheckRawSafe(*d, d.Serial); err != nil {
		return fatal(err), false
	}
	fmt.Fprintln(os.Stderr, i18n.T(lang, "cli.raw.warn", d.Path, d.Model, report.HumanBytes(d.Size), d.Serial))
	if p.ConfirmSerial == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fatal(errors.New("--confirm-serial requis hors terminal")), false
		}
		fmt.Fprint(os.Stderr, i18n.T(lang, "cli.raw.prompt"))
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		p.ConfirmSerial = strings.TrimSpace(line)
	}
	if err := disk.CheckRawSafe(*d, p.ConfirmSerial); err != nil {
		return fatal(err), false
	}
	return 0, true
}

func printSummary(rep *report.Report) {
	fmt.Println()
	fmt.Printf("%s : %s\n", i18n.T(lang, "r.verdict"), strings.ToUpper(i18n.T(lang, "verdict."+string(rep.Verdict))))
	fmt.Println(i18n.T(lang, "verdict."+string(rep.Verdict)+".long"))
	fmt.Println()
	for _, f := range rep.Findings {
		mark := map[report.Severity]string{report.SevOK: "  ok ", report.SevWarn: "  !  ", report.SevFail: "  ✗  ", report.SevInfo: "  i  "}[f.Severity]
		fmt.Printf("%s[%s] %s\n", mark, i18n.T(lang, "comp."+f.Component), f.Text(lang))
	}
	if rep.CPU != nil && rep.CPU.Bench != nil {
		fmt.Println()
		for _, s := range rep.CPU.Bench.Scores {
			multi := "—"
			if s.Multi > 0 {
				multi = fmt.Sprintf("%.2f", s.Multi)
			}
			fmt.Printf("  %-18s %9.2f %9s %s\n", s.Name, s.Single, multi, s.Unit)
		}
	}
	fmt.Println()
}

// ----- affichage de l'avancement -----

type progressUI struct {
	json    bool
	tty     bool
	last    engine.Event
	sample  sensors.Sample
	lastLog time.Time
	planned time.Duration
	start   time.Time
}

func newProgressUI(asJSON bool) *progressUI {
	return &progressUI{json: asJSON, tty: term.IsTerminal(int(os.Stderr.Fd())), start: time.Now()}
}

func (u *progressUI) handle(e engine.Event) {
	if u.json {
		json.NewEncoder(os.Stdout).Encode(e)
		return
	}
	switch e.Kind {
	case "start":
		u.planned = e.Planned
	case "sample":
		u.sample = *e.Sample
		return
	case "phase":
		u.clear()
		fmt.Fprintf(os.Stderr, "▶ %s\n", i18n.T(lang, "comp."+e.Phase))
	case "log":
		u.clear()
		fmt.Fprintf(os.Stderr, "  %s\n", e.Message)
	case "progress":
		u.last = e
	}
	line := u.line()
	if u.tty {
		fmt.Fprintf(os.Stderr, "\r\033[K%s", line)
	} else if time.Since(u.lastLog) > 10*time.Second {
		fmt.Fprintln(os.Stderr, line)
		u.lastLog = time.Now()
	}
}

func (u *progressUI) line() string {
	e := u.last
	var b strings.Builder
	fmt.Fprintf(&b, "%3.0f %% ", e.Fraction*100)
	el := time.Since(u.start).Round(time.Second)
	if u.planned > 0 {
		fmt.Fprintf(&b, "%s/%s ", report.HumanDuration(el), report.HumanDuration(u.planned))
	} else {
		fmt.Fprintf(&b, "%s ", report.HumanDuration(el))
	}
	if e.Phase != "" {
		fmt.Fprintf(&b, "· %s %s ", e.Phase, e.Detail)
	}
	if e.Value > 0 && !math.IsInf(e.Value, 0) {
		fmt.Fprintf(&b, "· %.1f %s ", e.Value, e.Unit)
	}
	if e.Errors > 0 {
		fmt.Fprintf(&b, "· %d erreur(s) ", e.Errors)
	}
	if t := u.sample.CPUTempC; t == t && t > 0 {
		fmt.Fprintf(&b, "· %.0f °C ", t)
	}
	if f := u.sample.FreqMHz; f == f && f > 0 {
		fmt.Fprintf(&b, "· %.0f MHz", f)
	}
	return b.String()
}

func (u *progressUI) clear() {
	if u.tty && !u.json {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
}

func (u *progressUI) finish() { u.clear() }

// ----- server -----

func cmdServer(args []string) int {
	fs := newFlags("server")
	listen := fs.String("listen", fmt.Sprintf(":%d", netbench.DefaultPort), "adresse d'écoute (TCP et UDP)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	fmt.Fprintln(os.Stderr, i18n.T(lang, "cli.server"))
	s := &netbench.Server{Version: engine.Version, Log: log.New(os.Stderr, "", log.LstdFlags)}
	if err := s.ListenAndServe(ctx, *listen); err != nil {
		return fatal(err)
	}
	return 0
}

// ----- report / compare -----

func cmdReport(args []string) int {
	fs := newFlags("report")
	html := fs.String("html", "", "fichier HTML à produire")
	pdf := fs.String("pdf", "", "fichier PDF à produire")
	if err := fs.Parse(reorder(args)); err != nil || fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage : rodage-cli report [-html f.html] [-pdf f.pdf] [-lang fr|en] rapport.json")
		return exitUsage
	}
	r, err := report.Load(fs.Arg(0))
	if err != nil {
		return fatal(err)
	}
	if *html == "" && *pdf == "" {
		base := strings.TrimSuffix(fs.Arg(0), ".json")
		*html, *pdf = base+".html", base+".pdf"
	}
	if *html != "" {
		f, err := os.Create(*html)
		if err != nil {
			return fatal(err)
		}
		r.WriteHTML(f, lang)
		f.Close()
		fmt.Fprintln(os.Stderr, i18n.T(lang, "cli.saved", *html))
	}
	if *pdf != "" {
		f, err := os.Create(*pdf)
		if err != nil {
			return fatal(err)
		}
		if err := r.WritePDF(f, lang); err != nil {
			return fatal(err)
		}
		f.Close()
		fmt.Fprintln(os.Stderr, i18n.T(lang, "cli.saved", *pdf))
	}
	return 0
}

// reorder place les options avant les arguments positionnels.
func reorder(args []string) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			if !strings.Contains(a, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && a != "-json" && a != "--json" {
				flags = append(flags, args[i+1])
				i++
			}
		} else {
			pos = append(pos, a)
		}
	}
	return append(flags, pos...)
}

func cmdCompare(args []string) int {
	fs := newFlags("compare")
	html := fs.String("html", "", "fichier HTML à produire")
	asJSON := fs.Bool("json", false, "sortie JSON")
	if err := fs.Parse(reorder(args)); err != nil || fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage : rodage-cli compare [-html f.html] avant.json apres.json")
		return exitUsage
	}
	a, err := report.Load(fs.Arg(0))
	if err != nil {
		return fatal(err)
	}
	b, err := report.Load(fs.Arg(1))
	if err != nil {
		return fatal(err)
	}
	d := report.Compare(a, b)
	if *asJSON {
		json.NewEncoder(os.Stdout).Encode(d)
	} else {
		d.WriteText(os.Stdout, lang)
	}
	if *html != "" {
		f, err := os.Create(*html)
		if err != nil {
			return fatal(err)
		}
		d.WriteHTML(f, lang)
		f.Close()
		fmt.Fprintln(os.Stderr, i18n.T(lang, "cli.saved", *html))
	}
	return 0
}

// ----- refs -----

func cmdRefs(args []string) int {
	if len(args) == 0 {
		args = []string{"list"}
	}
	db, err := refdb.Open(refdb.DefaultPath())
	if err != nil {
		return fatal(err)
	}
	switch args[0] {
	case "list":
		m := db.Models()
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Println(refdb.DefaultPath())
		for _, k := range keys {
			fmt.Printf("  %-60s %d\n", k, m[k])
		}
	case "export":
		if len(args) != 2 {
			return fatal(errors.New("usage : rodage-cli refs export fichier.json"))
		}
		if err := db.Export(args[1]); err != nil {
			return fatal(err)
		}
	case "import":
		if len(args) != 2 {
			return fatal(errors.New("usage : rodage-cli refs import fichier.json"))
		}
		n, err := db.Import(args[1])
		if err != nil {
			return fatal(err)
		}
		if err := db.Save(); err != nil {
			return fatal(err)
		}
		fmt.Printf("%d\n", n)
	default:
		return fatal(fmt.Errorf("refs : sous-commande inconnue %q", args[0]))
	}
	return 0
}
