package report

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/maxb1906/rodage/internal/disk"
	"github.com/maxb1906/rodage/internal/i18n"
	"github.com/maxb1906/rodage/internal/safety"
	"github.com/maxb1906/rodage/internal/sensors"
)

func joinInts(v []int) string {
	s := make([]string, len(v))
	for i, x := range v {
		s[i] = fmt.Sprint(x)
	}
	return strings.Join(s, ", ")
}

// HumanBytes formate une taille (base 1024) avec les unités françaises.
func HumanBytes(b uint64) string { return HumanBytesL(i18n.FR, b) }

// HumanBytesL formate une taille dans la langue demandée (Gio / GiB).
func HumanBytesL(l i18n.Lang, b uint64) string {
	const u = 1024
	byteU, suffix := "o", "io"
	if l == i18n.EN {
		byteU, suffix = "B", "iB"
	}
	if b < u {
		return fmt.Sprintf("%d %s", b, byteU)
	}
	div, exp := uint64(u), 0
	for n := b / u; n >= u; n /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.1f %c%s", float64(b)/float64(div), "KMGTPE"[exp], suffix)
}

// HumanDuration formate une durée lisible.
func HumanDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	switch {
	case h > 0:
		return fmt.Sprintf("%d h %02d min", h, m)
	case m > 0:
		return fmt.Sprintf("%d min %02d s", m, s)
	}
	return fmt.Sprintf("%d s", s)
}

func diskLabel(r disk.Result) string {
	if r.Device != nil {
		name := r.Device.Model
		if name == "" {
			name = r.Device.Name
		}
		return name
	}
	return r.Path
}

func throttleCause(tr *sensors.ThrottleReport, l i18n.Lang) string {
	if !tr.CauseKnown {
		return i18n.T(l, "cause.probable."+tr.Probable)
	}
	var c []string
	add := func(ok bool, k string) {
		if ok {
			c = append(c, i18n.T(l, k))
		}
	}
	add(tr.Causes.Thermal, "cause.thermal")
	add(tr.Causes.Prochot, "cause.prochot")
	add(tr.Causes.PL1, "cause.pl1")
	add(tr.Causes.PL2, "cause.pl2")
	add(tr.Causes.VRM, "cause.vrm")
	add(tr.Causes.Other, "cause.other")
	return strings.Join(c, ", ")
}

// Evaluate calcule constats, limites et verdict. lang sert aux arguments
// déjà traduits (causes de bridage).
func (r *Report) Evaluate() {
	l := i18n.Parse(r.Lang)
	r.Findings = nil
	r.Limitations = nil
	add := func(sev Severity, comp, key string, args ...any) {
		r.Findings = append(r.Findings, Finding{Severity: sev, Component: comp, Key: key, Args: args})
	}
	incomplete := false

	if r.PrevCrash != nil {
		add(SevWarn, "safety", "f.prevcrash", r.PrevCrash.Started.Local().Format("2006-01-02 15:04"), r.PrevCrash.Phase)
	}
	for _, e := range r.Safety {
		if !e.Abort {
			continue
		}
		switch e.Kind {
		case safety.KindThermal:
			add(SevFail, "safety", "f.safety.thermal", e.Detail)
		case safety.KindWatchdog:
			add(SevFail, "safety", "f.safety.watchdog", e.Detail)
		case safety.KindBattery:
			add(SevInfo, "safety", "f.safety.battery", e.Detail)
			incomplete = true
		case safety.KindAC:
			add(SevInfo, "safety", "f.safety.ac")
			incomplete = true
		case safety.KindUser:
			add(SevInfo, "safety", "f.safety.user")
			incomplete = true
		}
	}

	if c := r.CPU; c != nil {
		if s := c.Stress; s != nil {
			if s.TotalErrors > 0 {
				add(SevFail, "cpu", "f.cpu.errors", s.TotalErrors, joinInts(s.FailingCores()))
			} else if len(s.Cores) > 0 {
				add(SevOK, "cpu", "f.cpu.ok", s.Threads, HumanDuration(s.Duration), s.Kernel)
			}
			if len(s.Stalled) > 0 {
				add(SevFail, "cpu", "f.cpu.stalled", joinInts(s.Stalled))
			}
			pinned := true
			for _, x := range s.Cores {
				pinned = pinned && x.Pinned
			}
			if !pinned {
				add(SevInfo, "cpu", "f.cpu.nopin")
			}
		}
		if b := c.Bench; b != nil {
			var errs uint64
			for _, s := range b.Scores {
				errs += s.Errors
			}
			if errs > 0 {
				add(SevFail, "cpu", "f.cpu.errors", errs, "?")
			} else {
				add(SevOK, "cpu", "f.cpu.bench", len(b.Scores), b.Kernel)
			}
		}
		if tr := c.Throttle; tr != nil {
			switch {
			case tr.BaselineMHz <= 0:
				add(SevInfo, "cpu", "f.throttle.nofreq")
			case tr.Detected:
				add(SevWarn, "cpu", "f.throttle", tr.DropPct, tr.BaselineMHz, tr.SustainedMHz, throttleCause(tr, l))
			default:
				add(SevOK, "cpu", "f.throttle.none", tr.SustainedMHz)
			}
		}
		if r.MaxTempC > 0 {
			add(SevInfo, "cpu", "f.temp.max", r.MaxTempC, r.ThermalStop)
		} else {
			add(SevInfo, "cpu", "f.temp.none")
		}
	}

	if m := r.RAM; m != nil {
		r.Limitations = append(r.Limitations, "lim.ram")
		if m.TotalErrors > 0 {
			tests := map[string]bool{}
			for _, e := range m.Errors {
				tests[e.Test] = true
			}
			var ts []string
			for t := range tests {
				ts = append(ts, t)
			}
			add(SevFail, "ram", "f.ram.errors", m.TotalErrors, strings.Join(ts, ", "))
		} else if m.Passes > 0 {
			add(SevOK, "ram", "f.ram.ok", HumanBytesL(l, m.Bytes), m.Passes)
		} else {
			add(SevWarn, "ram", "f.ram.nopass")
		}
		if !m.Locked {
			add(SevInfo, "ram", "f.ram.unlocked")
		}
		switch {
		case !m.ECC.Available:
			add(SevInfo, "ram", "f.ecc.none")
		case m.ECC.Uncorrected > 0:
			add(SevFail, "ram", "f.ecc.ue", m.ECC.Uncorrected, m.ECC.Source)
		case m.ECC.Corrected > 0:
			add(SevWarn, "ram", "f.ecc.ce", m.ECC.Corrected, m.ECC.Source)
		}
	}

	for _, d := range r.Disks {
		name := diskLabel(d)
		bad := false
		if d.VerifyErrors > 0 {
			add(SevFail, "disk", "f.disk.verify", name, d.VerifyErrors)
			bad = true
		}
		if d.IOErrors > 0 {
			add(SevFail, "disk", "f.disk.io", name, d.IOErrors)
			bad = true
		}
		if len(d.Degraded) > 0 {
			add(SevFail, "disk", "f.disk.degraded", name, strings.Join(d.Degraded, ", "))
			bad = true
		}
		if p := d.HealthAfter.Problems(); len(p) > 0 {
			add(SevWarn, "disk", "f.disk.health", name, strings.Join(p, ", "))
		}
		if !d.HealthBefore.Available && d.HealthBefore.Note != "" {
			add(SevInfo, "disk", "f.disk.nohealth", name, d.HealthBefore.Note)
		}
		if !bad && d.ReadBytes > 0 {
			add(SevOK, "disk", "f.disk.ok", name, HumanBytesL(l, d.ReadBytes), d.SeqReadMBs, d.SeqWriteMBs)
		}
		if d.SLC.Detected {
			add(SevInfo, "disk", "f.disk.slc", name, d.SLC.BeforeMBs, d.SLC.AfterMBs, d.SLC.AfterGB)
		}
		if !d.Direct {
			add(SevInfo, "disk", "f.disk.nodirect", name)
		}
		for _, rr := range d.Random {
			if rr.Name == "read-4k-qd1" && rr.P99Us > 50000 && d.Device != nil && !d.Device.Rotational {
				add(SevWarn, "disk", "f.disk.lat", name, rr.P99Us/1000)
			}
		}
	}

	if n := r.Net; n != nil {
		var up, down, sent, lost float64
		var nu, nd int
		for _, t := range n.TCP {
			if t.Dir == "up" {
				up += t.Mbps
				nu++
			} else {
				down += t.Mbps
				nd++
			}
		}
		if nu > 0 {
			up /= float64(nu)
		}
		if nd > 0 {
			down /= float64(nd)
		}
		var worst float64
		var worstDir string
		for _, u := range n.UDP {
			sent += float64(u.Sent)
			lost += float64(u.Lost)
			if u.LossPct > worst {
				worst, worstDir = u.LossPct, u.Dir
			}
		}
		loss := 0.0
		if sent > 0 {
			loss = 100 * lost / sent
		}
		if len(n.TCP) > 0 {
			add(SevOK, "net", "f.net.ok", n.Server, up, down, loss)
		}
		if worst > 5 {
			add(SevFail, "net", "f.net.loss", worst, worstDir)
		} else if worst > 1 {
			add(SevWarn, "net", "f.net.loss", worst, worstDir)
		}
		for _, side := range []struct {
			name string
			l    interface{ Problems() []string }
		}{{i18n.T(l, "r.client"), n.Link}, {i18n.T(l, "r.server"), n.ServerLink}} {
			for _, p := range side.l.Problems() {
				switch p {
				case "renegotiation":
					add(SevFail, "net", "f.net.reneg", side.name)
				case "speed_change":
					add(SevWarn, "net", "f.net.speed", side.name)
				case "nic_errors":
					lr := n.Link
					if side.name != i18n.T(l, "r.client") {
						lr = n.ServerLink
					}
					add(SevWarn, "net", "f.net.nicerr", lr.RxErrors+lr.TxErrors, side.name)
				}
			}
		}
		if len(n.Errors) > 0 {
			add(SevWarn, "net", "f.net.errors", strings.Join(n.Errors, " ; "))
		}
	}

	for _, rc := range r.References {
		add(SevInfo, "system", "f.ref", rc.Name, rc.DeltaPct, rc.Count)
	}
	if len(r.Errors) > 0 {
		incomplete = true
	}

	if !r.System.Admin {
		r.Limitations = append(r.Limitations, "lim.noadmin")
	}
	r.Limitations = append(r.Limitations, fmt.Sprintf("lim.sensors%d", r.Sensors.Level))

	r.Verdict = Pass
	for _, f := range r.Findings {
		if f.Severity == SevFail {
			r.Verdict = Fail
			return
		}
	}
	if incomplete {
		r.Verdict = Incomplete
		return
	}
	for _, f := range r.Findings {
		if f.Severity == SevWarn {
			r.Verdict = Warn
		}
	}
}

var verbRe = regexp.MustCompile(`%[-+# 0]*\d*(?:\.\d+)?([a-zA-Z%])`)

// Text traduit un constat. Les arguments relus depuis JSON (float64) sont
// convertis selon le verbe attendu.
func (f Finding) Text(l i18n.Lang) string {
	format := i18n.T(l, f.Key)
	args := append([]any(nil), f.Args...)
	i := 0
	for _, m := range verbRe.FindAllStringSubmatch(format, -1) {
		if m[1] == "%" {
			continue
		}
		if i >= len(args) {
			break
		}
		switch m[1] {
		case "d":
			if v, ok := args[i].(float64); ok {
				args[i] = int64(v)
			}
		case "f":
			switch v := args[i].(type) {
			case int:
				args[i] = float64(v)
			case int64:
				args[i] = float64(v)
			case uint64:
				args[i] = float64(v)
			}
		}
		i++
	}
	return i18n.T(l, f.Key, args...)
}
