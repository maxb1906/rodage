package report

import (
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"math"
	"strings"
	"time"

	"github.com/maxb1906/rodage/internal/i18n"
)

//go:embed report.html.tmpl
var htmlTmpl string

type kv struct{ K, V string }

type htmlView struct {
	R          *Report
	L          i18n.Lang
	Title      string
	Generated  string
	Verdict    string
	VerdictTxt string
	Findings   []viewFinding
	Config     []kv
	Charts     []template.HTML
	Limits     []string
	SensorRows []kv
}

type viewFinding struct {
	Sev, Comp, Text string
}

func yesNo(l i18n.Lang, b bool) string {
	if b {
		return i18n.T(l, "r.yes")
	}
	return i18n.T(l, "r.no")
}

func orUnknown(l i18n.Lang, s string) string {
	if strings.TrimSpace(s) == "" {
		return i18n.T(l, "r.unknown")
	}
	return s
}

// ConfigRows décrit la configuration détectée (partagé HTML/PDF/GUI).
func (r *Report) ConfigRows(l i18n.Lang) []kv {
	s := r.System
	sm := s.SMBIOS
	machine := strings.TrimSpace(sm.Manufacturer + " " + sm.Product)
	board := strings.TrimSpace(sm.BoardVendor + " " + sm.BoardProduct)
	if sm.BoardSerial != "" {
		board += " (" + sm.BoardSerial + ")"
	}
	bios := strings.TrimSpace(sm.BIOSVendor + " " + sm.BIOSVersion + " " + sm.BIOSDate)
	return []kv{
		{i18n.T(l, "r.machine"), orUnknown(l, machine)},
		{i18n.T(l, "r.serial"), orUnknown(l, sm.Serial)},
		{i18n.T(l, "r.board"), orUnknown(l, board)},
		{i18n.T(l, "r.bios"), orUnknown(l, bios)},
		{i18n.T(l, "r.os"), fmt.Sprintf("%s (%s/%s)", s.OSVersion, s.OS, s.Arch)},
		{i18n.T(l, "r.cpu"), s.CPUModel},
		{i18n.T(l, "r.threads"), fmt.Sprint(s.LogicalCPUs)},
		{i18n.T(l, "r.simd"), strings.Join(s.SIMD, ", ")},
		{i18n.T(l, "r.ram"), HumanBytesL(l, s.RAMTotal)},
		{i18n.T(l, "r.admin"), yesNo(l, s.Admin)},
		{i18n.T(l, "r.mode"), i18n.T(l, "mode."+r.Mode) + " — " + i18n.T(l, "profile."+r.Profile)},
		{i18n.T(l, "r.duration"), HumanDuration(r.Elapsed)},
	}
}

// Charts construit les graphiques du rapport.
func (r *Report) Charts(l i18n.Lang) []template.HTML {
	if len(r.Samples) == 0 {
		return nil
	}
	t0 := r.Samples[0].Time
	var x, temp, freq []float64
	for _, s := range r.Samples {
		x = append(x, s.Time.Sub(t0).Seconds())
		temp = append(temp, s.CPUTempC)
		freq = append(freq, s.FreqMHz)
	}
	var bands []Band
	for _, p := range r.Phases {
		bands = append(bands, Band{X0: p.Start.Sub(t0).Seconds(), X1: p.End.Sub(t0).Seconds(), Label: p.Name})
	}
	var out []template.HTML
	if hasData(temp) {
		out = append(out, SVGChart(i18n.T(l, "r.temp"), []Series{{X: x, Y: temp, Color: "#d9480f"}}, bands, 760, 220))
	}
	if hasData(freq) {
		out = append(out, SVGChart(i18n.T(l, "r.freq"), []Series{{X: x, Y: freq, Color: "#1c7ed6"}}, bands, 760, 220))
	}
	for _, d := range r.Disks {
		if len(d.WriteCurve) > 1 {
			var dx []float64
			for i := range d.WriteCurve {
				dx = append(dx, float64(i))
			}
			out = append(out, SVGChart(i18n.T(l, "r.writecurve")+" — "+diskLabel(d), []Series{{X: dx, Y: d.WriteCurve, Color: "#2f9e44"}}, nil, 760, 200))
		}
	}
	return out
}

func hasData(v []float64) bool {
	for _, x := range v {
		if x == x {
			return true
		}
	}
	return false
}

func confidence(level any) string { return fmt.Sprintf("conf.%v", level) }

var funcs = template.FuncMap{
	"t":      func(l i18n.Lang, k string, a ...any) string { return i18n.T(l, k, a...) },
	"bytes":  HumanBytes,
	"bytesL": HumanBytesL,
	"dur":    HumanDuration,
	"f0": func(v float64) string {
		if v != v || math.IsInf(v, 0) {
			return "—"
		}
		return fmt.Sprintf("%.0f", v)
	},
	"f1": func(v float64) string {
		if v != v || math.IsInf(v, 0) {
			return "—"
		}
		return fmt.Sprintf("%.1f", v)
	},
	"f2": func(v float64) string {
		if v != v || math.IsInf(v, 0) {
			return "—"
		}
		return fmt.Sprintf("%.2f", v)
	},
	"i64": func(v int64) string {
		if v < 0 {
			return "—"
		}
		return fmt.Sprint(v)
	},
	"time": func(t time.Time) string { return t.Local().Format("2006-01-02 15:04:05") },
	"conf": confidence,
	"tempOrNA": func(v float64) string {
		if v < 0 {
			return "—"
		}
		return fmt.Sprintf("%.0f °C", v)
	},
}

// WriteHTML rend le rapport en HTML autonome.
func (r *Report) WriteHTML(w io.Writer, l i18n.Lang) error {
	t, err := template.New("r").Funcs(funcs).Parse(htmlTmpl)
	if err != nil {
		return err
	}
	v := htmlView{
		R:          r,
		L:          l,
		Title:      i18n.T(l, "r.title"),
		Generated:  i18n.T(l, "r.generated", r.Created.Local().Format("2006-01-02 15:04"), r.Version),
		Verdict:    string(r.Verdict),
		VerdictTxt: i18n.T(l, "verdict."+string(r.Verdict)),
		Config:     r.ConfigRows(l),
		Charts:     r.Charts(l),
	}
	for _, f := range r.Findings {
		v.Findings = append(v.Findings, viewFinding{Sev: string(f.Severity), Comp: i18n.T(l, "comp."+f.Component), Text: f.Text(l)})
	}
	for _, k := range r.Limitations {
		v.Limits = append(v.Limits, i18n.T(l, k))
	}
	return t.Execute(w, v)
}
