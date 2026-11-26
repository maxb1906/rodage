package report

import (
	"fmt"
	"html/template"
	"math"
	"strings"
)

// Series est une courbe : abscisses (secondes) et ordonnées.
type Series struct {
	X, Y  []float64
	Color string
	Label string
}

// Band est une zone colorée sur l'axe des abscisses (phase de test).
type Band struct {
	X0, X1 float64
	Label  string
}

// SVGChart dessine un graphique en ligne autonome (sans script).
func SVGChart(title string, series []Series, bands []Band, w, h int) template.HTML {
	const ml, mr, mt, mb = 52, 12, 26, 28
	pw, ph := float64(w-ml-mr), float64(h-mt-mb)
	xmin, xmax := math.Inf(1), math.Inf(-1)
	ymin, ymax := math.Inf(1), math.Inf(-1)
	for _, s := range series {
		for i, y := range s.Y {
			if y != y {
				continue
			}
			xmin, xmax = math.Min(xmin, s.X[i]), math.Max(xmax, s.X[i])
			ymin, ymax = math.Min(ymin, y), math.Max(ymax, y)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="chart" viewBox="0 0 %d %d" width="100%%" role="img" aria-label="%s">`, w, h, template.HTMLEscapeString(title))
	fmt.Fprintf(&b, `<text x="%d" y="16" class="ct">%s</text>`, ml, template.HTMLEscapeString(title))
	if math.IsInf(xmin, 0) {
		fmt.Fprintf(&b, `<text x="%d" y="%d" class="cl">—</text></svg>`, w/2, h/2)
		return template.HTML(b.String())
	}
	if xmax == xmin {
		xmax = xmin + 1
	}
	// Échelle Y arrondie.
	span := ymax - ymin
	if span < 1e-9 {
		span = math.Max(1, math.Abs(ymax)*0.1)
	}
	step := niceStep(span / 4)
	ylo := math.Floor((ymin-span*0.05)/step) * step
	yhi := math.Ceil((ymax+span*0.05)/step) * step
	if ymin >= 0 && ylo < 0 {
		ylo = 0
	}
	px := func(x float64) float64 { return float64(ml) + (x-xmin)/(xmax-xmin)*pw }
	py := func(y float64) float64 { return float64(mt) + (1-(y-ylo)/(yhi-ylo))*ph }
	for i, bd := range bands {
		cls := "b0"
		if i%2 == 1 {
			cls = "b1"
		}
		x0, x1 := px(math.Max(bd.X0, xmin)), px(math.Min(bd.X1, xmax))
		if x1 <= x0 {
			continue
		}
		fmt.Fprintf(&b, `<rect x="%.1f" y="%d" width="%.1f" height="%.0f" class="%s"/>`, x0, mt, x1-x0, ph, cls)
		fmt.Fprintf(&b, `<text x="%.1f" y="%d" class="bl">%s</text>`, x0+3, mt+11, template.HTMLEscapeString(bd.Label))
	}
	for y := ylo; y <= yhi+step/2; y += step {
		fmt.Fprintf(&b, `<line x1="%d" x2="%d" y1="%.1f" y2="%.1f" class="grid"/>`, ml, w-mr, py(y), py(y))
		fmt.Fprintf(&b, `<text x="%d" y="%.1f" class="cl" text-anchor="end">%s</text>`, ml-6, py(y)+4, fmtNum(y))
	}
	for i := 0; i <= 4; i++ {
		x := xmin + (xmax-xmin)*float64(i)/4
		fmt.Fprintf(&b, `<text x="%.1f" y="%d" class="cl" text-anchor="middle">%s</text>`, px(x), h-8, fmtElapsed(x))
	}
	for _, s := range series {
		var pts strings.Builder
		pen := false
		for i, y := range s.Y {
			if y != y {
				if pen {
					pts.WriteString(`"/>`)
					pen = false
				}
				continue
			}
			if !pen {
				fmt.Fprintf(&pts, `<polyline fill="none" stroke="%s" stroke-width="1.6" stroke-linejoin="round" points="`, s.Color)
				pen = true
			}
			fmt.Fprintf(&pts, "%.1f,%.1f ", px(s.X[i]), py(y))
		}
		if pen {
			pts.WriteString(`"/>`)
		}
		b.WriteString(pts.String())
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

func niceStep(raw float64) float64 {
	if raw <= 0 {
		return 1
	}
	p := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if raw <= m*p {
			return m * p
		}
	}
	return 10 * p
}

func fmtNum(v float64) string {
	if math.Abs(v) >= 100 || v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

func fmtElapsed(s float64) string {
	d := int(s)
	if d >= 3600 {
		return fmt.Sprintf("%dh%02d", d/3600, d/60%60)
	}
	return fmt.Sprintf("%d:%02d", d/60, d%60)
}
