package report

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/go-pdf/fpdf"

	"github.com/maxb1906/rodage/internal/i18n"
)

// Caractères absents de cp1252 (polices PDF standard).
var pdfReplacer = strings.NewReplacer("≥", ">=", "≤", "<=", "→", "->", "−", "-", "≈", "~", "✓", "OK", "✗", "X", "×", "x", "’", "'", "…", "...")

// WritePDF rend le rapport en PDF (polices standard, sans dépendance native).
func (r *Report) WritePDF(w io.Writer, l i18n.Lang) error {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	T := func(s string) string { return tr(pdfReplacer.Replace(s)) }
	pdf.SetFooterFunc(func() {
		pdf.SetY(-11)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(0, 5, T(fmt.Sprintf("Rodage %s — %s — %d", r.Version, r.ID, pdf.PageNo())), "", 0, "C", false, 0, "")
	})
	pdf.AddPage()
	width, _ := pdf.GetPageSize()
	cw := width - 30

	pdf.SetFont("Helvetica", "B", 18)
	pdf.SetTextColor(29, 36, 51)
	pdf.CellFormat(0, 9, T(i18n.T(l, "r.title")), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(92, 102, 122)
	pdf.CellFormat(0, 5, T(r.System.Hostname+" · "+i18n.T(l, "r.generated", r.Created.Local().Format("2006-01-02 15:04"), r.Version)), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	// Verdict
	col := map[Verdict][3]int{Pass: {47, 158, 68}, Warn: {230, 119, 0}, Fail: {201, 42, 42}, Incomplete: {28, 126, 214}}[r.Verdict]
	y := pdf.GetY()
	pdf.SetFillColor(col[0], col[1], col[2])
	pdf.Rect(15, y, 3, 16, "F")
	pdf.SetXY(21, y+1)
	pdf.SetFont("Helvetica", "B", 15)
	pdf.SetTextColor(col[0], col[1], col[2])
	pdf.CellFormat(0, 7, T(i18n.T(l, "r.verdict")+" : "+i18n.T(l, "verdict."+string(r.Verdict))), "", 1, "L", false, 0, "")
	pdf.SetX(21)
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(29, 36, 51)
	pdf.MultiCell(cw-6, 5, T(i18n.T(l, "verdict."+string(r.Verdict)+".long")), "", "L", false)
	pdf.SetY(math.Max(pdf.GetY(), y+16) + 4)

	h2 := func(s string) {
		if pdf.GetY() > 255 {
			pdf.AddPage()
		}
		pdf.Ln(2)
		pdf.SetFont("Helvetica", "B", 12)
		pdf.SetTextColor(29, 36, 51)
		pdf.SetDrawColor(29, 36, 51)
		pdf.SetLineWidth(0.2)
		pdf.CellFormat(0, 7, T(s), "B", 1, "L", false, 0, "")
		pdf.Ln(1.5)
	}
	body := func() {
		pdf.SetFont("Helvetica", "", 9.5)
		pdf.SetTextColor(29, 36, 51)
	}
	h2(i18n.T(l, "r.findings"))
	for _, f := range r.Findings {
		mark, c := "i", [3]int{28, 126, 214}
		switch f.Severity {
		case SevOK:
			mark, c = "OK", [3]int{47, 158, 68}
		case SevWarn:
			mark, c = "!", [3]int{230, 119, 0}
		case SevFail:
			mark, c = "X", [3]int{201, 42, 42}
		}
		pdf.SetFont("Helvetica", "B", 9.5)
		pdf.SetTextColor(c[0], c[1], c[2])
		pdf.CellFormat(9, 5, mark, "", 0, "L", false, 0, "")
		body()
		pdf.MultiCell(cw-9, 5, T(strings.ToUpper(i18n.T(l, "comp."+f.Component))+" — "+f.Text(l)), "", "L", false)
		pdf.Ln(0.8)
	}

	kvTable := func(rows []kv) {
		for _, row := range rows {
			body()
			pdf.SetTextColor(92, 102, 122)
			x, y := pdf.GetXY()
			pdf.MultiCell(55, 5, T(row.K), "", "L", false)
			y1 := pdf.GetY()
			pdf.SetXY(x+55, y)
			pdf.SetTextColor(29, 36, 51)
			pdf.MultiCell(cw-55, 5, T(row.V), "", "L", false)
			pdf.SetY(math.Max(y1, pdf.GetY()))
		}
	}
	h2(i18n.T(l, "r.config"))
	kvTable(r.ConfigRows(l))

	h2(i18n.T(l, "r.tests"))
	var rows []kv
	if c := r.CPU; c != nil {
		if s := c.Stress; s != nil {
			rows = append(rows, kv{i18n.T(l, "comp.cpu"), fmt.Sprintf("%s · %d × %s · %s : %d", s.Kernel, s.Threads, HumanDuration(s.Duration), i18n.T(l, "r.errors"), s.TotalErrors)})
		}
		if b := c.Bench; b != nil {
			for _, s := range b.Scores {
				v := fmt.Sprintf("%.2f %s", s.Single, s.Unit)
				if s.Multi > 0 {
					v += fmt.Sprintf(" · %s %.2f %s", i18n.T(l, "r.multi"), s.Multi, s.Unit)
				}
				rows = append(rows, kv{s.Name, v})
			}
		}
	}
	if m := r.RAM; m != nil {
		rows = append(rows, kv{i18n.T(l, "comp.ram"), fmt.Sprintf("%s · %s %d · %.1f %s · %s : %d", HumanBytesL(l, m.Bytes), i18n.T(l, "r.passes"), m.Passes, m.BandwidthGBs, i18n.T(l, "u.gbs"), i18n.T(l, "r.errors"), m.TotalErrors)})
	}
	for _, d := range r.Disks {
		v := fmt.Sprintf("R %.0f %s · W %.0f %s", d.SeqReadMBs, i18n.T(l, "u.mbs"), d.SeqWriteMBs, i18n.T(l, "u.mbs"))
		for _, x := range d.Random {
			v += fmt.Sprintf(" · %s %.0f IOPS (p99 %.0f µs)", x.Name, x.IOPS, x.P99Us)
		}
		v += fmt.Sprintf(" · %s : %d/%d", i18n.T(l, "r.errors"), d.VerifyErrors, d.IOErrors)
		rows = append(rows, kv{i18n.T(l, "comp.disk") + " " + diskLabel(d), v})
	}
	if n := r.Net; n != nil {
		var parts []string
		for _, t := range n.TCP {
			parts = append(parts, fmt.Sprintf("TCP %s %.0f Mbit/s", t.Dir, t.Mbps))
		}
		for _, u := range n.UDP {
			parts = append(parts, fmt.Sprintf("UDP %s %.2f %% / %.2f ms", u.Dir, u.LossPct, u.JitterMs))
		}
		rows = append(rows, kv{i18n.T(l, "comp.net") + " " + n.Server, strings.Join(parts, " · ")})
	}
	kvTable(rows)

	// Courbes
	if len(r.Samples) > 1 {
		t0 := r.Samples[0].Time
		var x, temp, freq []float64
		for _, s := range r.Samples {
			x = append(x, s.Time.Sub(t0).Seconds())
			temp = append(temp, s.CPUTempC)
			freq = append(freq, s.FreqMHz)
		}
		if hasData(temp) || hasData(freq) {
			h2(i18n.T(l, "r.charts"))
		}
		if hasData(temp) {
			pdfChart(pdf, T(i18n.T(l, "r.temp")), x, temp, [3]int{217, 72, 15}, cw)
		}
		if hasData(freq) {
			pdfChart(pdf, T(i18n.T(l, "r.freq")), x, freq, [3]int{28, 126, 214}, cw)
		}
	}
	for _, d := range r.Disks {
		if len(d.WriteCurve) > 1 {
			var x []float64
			for i := range d.WriteCurve {
				x = append(x, float64(i))
			}
			pdfChart(pdf, T(i18n.T(l, "r.writecurve")+" — "+diskLabel(d)), x, d.WriteCurve, [3]int{47, 158, 68}, cw)
		}
	}

	h2(i18n.T(l, "r.sensors"))
	srows := []kv{{i18n.T(l, "r.confidence"), i18n.T(l, "r.level", r.Sensors.Level) + " — " + i18n.T(l, confidence(int(r.Sensors.Level)))}}
	for _, d := range r.Sensors.Details {
		srows = append(srows, kv{"", d})
	}
	kvTable(srows)

	if len(r.Safety) > 0 {
		h2(i18n.T(l, "r.safetyev"))
		var ev []kv
		for _, e := range r.Safety {
			ev = append(ev, kv{e.Time.Local().Format("2006-01-02 15:04:05"), string(e.Kind) + " — " + e.Detail})
		}
		kvTable(ev)
	}

	h2(i18n.T(l, "r.limits"))
	body()
	for _, k := range r.Limitations {
		pdf.MultiCell(cw, 5, T("• "+i18n.T(l, k)), "", "L", false)
		pdf.Ln(0.8)
	}
	return pdf.Output(w)
}

func pdfChart(pdf *fpdf.Fpdf, title string, x, y []float64, c [3]int, cw float64) {
	const h = 42.0
	if pdf.GetY()+h+10 > 280 {
		pdf.AddPage()
	}
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(29, 36, 51)
	pdf.CellFormat(0, 5, title, "", 1, "L", false, 0, "")
	top := pdf.GetY()
	left := 15.0 + 12
	w := cw - 12
	ymin, ymax := math.Inf(1), math.Inf(-1)
	for _, v := range y {
		if v == v {
			ymin, ymax = math.Min(ymin, v), math.Max(ymax, v)
		}
	}
	if ymax-ymin < 1e-9 {
		ymin, ymax = ymin-1, ymax+1
	}
	pad := (ymax - ymin) * 0.08
	ymin, ymax = ymin-pad, ymax+pad
	xmin, xmax := x[0], x[len(x)-1]
	if xmax <= xmin {
		xmax = xmin + 1
	}
	pdf.SetDrawColor(220, 224, 230)
	pdf.SetLineWidth(0.2)
	pdf.Rect(left, top, w, h, "D")
	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(92, 102, 122)
	for i := 0; i <= 3; i++ {
		v := ymin + (ymax-ymin)*float64(i)/3
		yy := top + h - h*float64(i)/3
		pdf.Line(left, yy, left+w, yy)
		pdf.Text(15, yy+1, fmtNum(v))
	}
	for i := 0; i <= 4; i++ {
		xv := xmin + (xmax-xmin)*float64(i)/4
		pdf.Text(left+w*float64(i)/4-3, top+h+4, fmtElapsed(xv))
	}
	pdf.SetDrawColor(c[0], c[1], c[2])
	pdf.SetLineWidth(0.45)
	px := func(v float64) float64 { return left + (v-xmin)/(xmax-xmin)*w }
	py := func(v float64) float64 { return top + h - (v-ymin)/(ymax-ymin)*h }
	for i := 1; i < len(y); i++ {
		if y[i] == y[i] && y[i-1] == y[i-1] {
			pdf.Line(px(x[i-1]), py(y[i-1]), px(x[i]), py(y[i]))
		}
	}
	pdf.SetY(top + h + 7)
}
