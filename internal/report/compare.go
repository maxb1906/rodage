package report

import (
	"fmt"
	"html/template"
	"io"
	"math"
	"strings"
	"text/tabwriter"

	"github.com/maxb1906/rodage/internal/i18n"
)

// DiffRow est une ligne de comparaison.
type DiffRow struct {
	Section  string  `json:"section"`
	Metric   string  `json:"metric"`
	Unit     string  `json:"unit"`
	A        float64 `json:"a"`
	B        float64 `json:"b"`
	HasA     bool    `json:"has_a"`
	HasB     bool    `json:"has_b"`
	DeltaPct float64 `json:"delta_pct"`
	// Higher : une valeur plus grande est meilleure.
	Higher bool `json:"higher"`
	// Better : B est meilleur que A (seulement si l'écart dépasse 3 %).
	Better int `json:"better"` // -1 pire, 0 égal, 1 meilleur
}

// Diff est la comparaison de deux rapports (avant / après).
type Diff struct {
	A, B     *Report   `json:"-"`
	Rows     []DiffRow `json:"rows"`
	Mismatch []string  `json:"mismatch,omitempty"`
}

type metric struct {
	section, name, unit string
	higher              bool
	get                 func(r *Report) (float64, bool)
}

func metrics(a, b *Report) []metric {
	var ms []metric
	names := map[string]bool{}
	for _, r := range []*Report{a, b} {
		if r.CPU != nil && r.CPU.Bench != nil {
			for _, s := range r.CPU.Bench.Scores {
				if names[s.Name] {
					continue
				}
				names[s.Name] = true
				s := s
				for _, multi := range []bool{false, true} {
					multi := multi
					label := s.Name + " (1)"
					if multi {
						label = s.Name + " (N)"
					}
					ms = append(ms, metric{"cpu", label, s.Unit, true, func(r *Report) (float64, bool) {
						if r.CPU == nil || r.CPU.Bench == nil {
							return 0, false
						}
						for _, x := range r.CPU.Bench.Scores {
							if x.Name == s.Name {
								if multi {
									return x.Multi, x.Multi > 0
								}
								return x.Single, true
							}
						}
						return 0, false
					}})
				}
			}
		}
	}
	ms = append(ms,
		metric{"cpu", "errors", "", false, func(r *Report) (float64, bool) {
			if r.CPU == nil || r.CPU.Stress == nil {
				return 0, false
			}
			return float64(r.CPU.Stress.TotalErrors), true
		}},
		metric{"cpu", "max temp", "°C", false, func(r *Report) (float64, bool) { return r.MaxTempC, r.MaxTempC > 0 }},
		metric{"cpu", "sustained freq", "MHz", true, func(r *Report) (float64, bool) {
			if r.CPU == nil || r.CPU.Throttle == nil || r.CPU.Throttle.SustainedMHz <= 0 {
				return 0, false
			}
			return r.CPU.Throttle.SustainedMHz, true
		}},
		metric{"ram", "bandwidth", "Go/s", true, func(r *Report) (float64, bool) {
			if r.RAM == nil {
				return 0, false
			}
			return r.RAM.BandwidthGBs, r.RAM.BandwidthGBs > 0
		}},
		metric{"ram", "errors", "", false, func(r *Report) (float64, bool) {
			if r.RAM == nil {
				return 0, false
			}
			return float64(r.RAM.TotalErrors), true
		}},
	)
	diskGet := func(f func(i int, r *Report) (float64, bool)) func(r *Report) (float64, bool) {
		return func(r *Report) (float64, bool) {
			if len(r.Disks) == 0 {
				return 0, false
			}
			return f(0, r)
		}
	}
	ms = append(ms,
		metric{"disk", "seq read", "Mo/s", true, diskGet(func(i int, r *Report) (float64, bool) { return r.Disks[i].SeqReadMBs, r.Disks[i].SeqReadMBs > 0 })},
		metric{"disk", "seq write", "Mo/s", true, diskGet(func(i int, r *Report) (float64, bool) { return r.Disks[i].SeqWriteMBs, r.Disks[i].SeqWriteMBs > 0 })},
	)
	for _, n := range []string{"read-4k-qd1", "write-4k-qd1", "read-4k-qd32", "write-4k-qd32"} {
		n := n
		ms = append(ms, metric{"disk", n, "IOPS", true, diskGet(func(i int, r *Report) (float64, bool) {
			for _, x := range r.Disks[i].Random {
				if x.Name == n {
					return x.IOPS, true
				}
			}
			return 0, false
		})})
		if n == "read-4k-qd1" {
			ms = append(ms, metric{"disk", n + " p99", "µs", false, diskGet(func(i int, r *Report) (float64, bool) {
				for _, x := range r.Disks[i].Random {
					if x.Name == n {
						return x.P99Us, true
					}
				}
				return 0, false
			})})
		}
	}
	ms = append(ms, metric{"disk", "errors", "", false, diskGet(func(i int, r *Report) (float64, bool) {
		return float64(r.Disks[i].VerifyErrors + r.Disks[i].IOErrors), true
	})})
	for _, dir := range []string{"up", "down"} {
		dir := dir
		ms = append(ms, metric{"net", "TCP " + dir, "Mbit/s", true, func(r *Report) (float64, bool) {
			if r.Net == nil {
				return 0, false
			}
			var s float64
			var n int
			for _, t := range r.Net.TCP {
				if t.Dir == dir {
					s += t.Mbps
					n++
				}
			}
			return s / math.Max(1, float64(n)), n > 0
		}})
		ms = append(ms, metric{"net", "UDP loss " + dir, "%", false, func(r *Report) (float64, bool) {
			if r.Net == nil {
				return 0, false
			}
			for _, u := range r.Net.UDP {
				if u.Dir == dir {
					return u.LossPct, true
				}
			}
			return 0, false
		}})
	}
	return ms
}

// Compare compare deux rapports.
func Compare(a, b *Report) Diff {
	d := Diff{A: a, B: b}
	if a.System.SMBIOS.Serial != b.System.SMBIOS.Serial && a.System.SMBIOS.Serial != "" && b.System.SMBIOS.Serial != "" {
		d.Mismatch = append(d.Mismatch, "serial")
	}
	if a.System.CPUModel != b.System.CPUModel {
		d.Mismatch = append(d.Mismatch, "cpu")
	}
	for _, m := range metrics(a, b) {
		va, oka := m.get(a)
		vb, okb := m.get(b)
		if !oka && !okb {
			continue
		}
		row := DiffRow{Section: m.section, Metric: m.name, Unit: m.unit, A: va, B: vb, HasA: oka, HasB: okb, Higher: m.higher}
		if oka && okb {
			if va != 0 {
				row.DeltaPct = 100 * (vb - va) / math.Abs(va)
			}
			diff := vb - va
			signif := math.Abs(row.DeltaPct) >= 3 || (va == 0 && vb != 0)
			if signif && diff != 0 {
				if (diff > 0) == m.higher {
					row.Better = 1
				} else {
					row.Better = -1
				}
			}
		}
		d.Rows = append(d.Rows, row)
	}
	return d
}

func fmtVal(v float64, ok bool) string {
	if !ok {
		return "—"
	}
	if math.Abs(v) >= 100 || v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.2f", v)
}

// WriteText écrit la comparaison sous forme de tableau texte.
func (d Diff) WriteText(w io.Writer, l i18n.Lang) {
	fmt.Fprintf(w, "%s\n", i18n.T(l, "r.compare"))
	fmt.Fprintf(w, "  A : %s  %s  %s  (%s)\n", d.A.Created.Local().Format("2006-01-02 15:04"), d.A.System.Hostname, d.A.ID, i18n.T(l, "verdict."+string(d.A.Verdict)))
	fmt.Fprintf(w, "  B : %s  %s  %s  (%s)\n\n", d.B.Created.Local().Format("2006-01-02 15:04"), d.B.System.Hostname, d.B.ID, i18n.T(l, "verdict."+string(d.B.Verdict)))
	if len(d.Mismatch) > 0 {
		fmt.Fprintln(w, i18n.T(l, "r.mismatch", strings.Join(d.Mismatch, ", ")))
		fmt.Fprintln(w)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t\t\n", "", i18n.T(l, "r.metric"), "A", "B", i18n.T(l, "r.delta"))
	for _, r := range d.Rows {
		mark := ""
		switch r.Better {
		case 1:
			mark = "+"
		case -1:
			mark = "!"
		}
		delta := ""
		if r.HasA && r.HasB && r.A != 0 {
			delta = fmt.Sprintf("%+.1f %%", r.DeltaPct)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Section, r.Metric, fmtVal(r.A, r.HasA), fmtVal(r.B, r.HasB), delta, r.Unit, mark)
	}
	tw.Flush()
}

const compareTmpl = `<!doctype html><html lang="{{.L}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>{{t .L "r.compare"}}</title>
<style>body{margin:0;font:15px/1.5 system-ui,"Segoe UI",Roboto,sans-serif;color:#1d2433;background:#f6f7f9}main{max-width:900px;margin:0 auto;padding:28px 20px}
table{width:100%;border-collapse:collapse;background:#fff;border:1px solid #dfe3ea;font-size:14px}th,td{padding:6px 10px;border-bottom:1px solid #dfe3ea;text-align:left}td.n{text-align:right;font-variant-numeric:tabular-nums}
th{background:#eef1f5}.up{color:#2f9e44;font-weight:600}.down{color:#c92a2a;font-weight:600}.warn{background:#fff4e6;border:1px solid #ffd8a8;padding:10px;border-radius:8px}.sub{color:#5c667a;font-size:13px}</style></head>
<body><main><h1>{{t .L "r.compare"}}</h1>
<p class="sub">A ({{t .L "r.before"}}) : {{.D.A.System.Hostname}} · {{time .D.A.Created}} · {{t .L (printf "verdict.%s" .D.A.Verdict)}}<br>
B ({{t .L "r.after"}}) : {{.D.B.System.Hostname}} · {{time .D.B.Created}} · {{t .L (printf "verdict.%s" .D.B.Verdict)}}</p>
{{if .D.Mismatch}}<p class="warn">{{t .L "r.mismatch" .Mis}}</p>{{end}}
<table><tr><th></th><th>{{t .L "r.metric"}}</th><th>A</th><th>B</th><th>{{t .L "r.delta"}}</th></tr>
{{range .D.Rows}}<tr><td>{{.Section}}</td><td>{{.Metric}}{{if .Unit}} ({{.Unit}}){{end}}</td><td class="n">{{val .A .HasA}}</td><td class="n">{{val .B .HasB}}</td>
<td class="n {{if eq .Better 1}}up{{else if eq .Better -1}}down{{end}}">{{if and .HasA .HasB}}{{if .A}}{{printf "%+.1f %%" .DeltaPct}}{{end}}{{end}}</td></tr>{{end}}
</table></main></body></html>`

// WriteHTML écrit la comparaison en HTML.
func (d Diff) WriteHTML(w io.Writer, l i18n.Lang) error {
	fm := template.FuncMap{"val": fmtVal}
	for k, v := range funcs {
		fm[k] = v
	}
	t, err := template.New("c").Funcs(fm).Parse(compareTmpl)
	if err != nil {
		return err
	}
	return t.Execute(w, map[string]any{"L": l, "D": d, "Mis": strings.Join(d.Mismatch, ", ")})
}
