// Package stylecheck measures a stylesheet's theme colours against the
// rules md2html holds its styles to: WCAG contrast for text, and the
// dataviz skill's checks for chart colours.
package stylecheck

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

// Severity is how bad a finding is. Only an Error makes --check-style
// exit non-zero.
type Severity int

const (
	Error Severity = iota
	Warning
	Note
)

func (s Severity) String() string {
	if s < 0 || s > Note {
		return "unknown"
	}
	return [...]string{"error", "warning", "note"}[s]
}

// Finding is one problem: its severity, the theme it is in ("light",
// "dark", or "" for one that spans both), the tokens it concerns, without
// their "--", and a sentence saying what is wrong.
type Finding struct {
	Severity Severity
	Theme    string
	Tokens   []string
	Message  string
}

// Tokens are the colours a style sets, in the order findings are listed.
// It is read-only: the rules below are built from it.
var Tokens = []string{"bg", "fg", "muted", "accent", "rule", "code-bg", "ok", "warn",
	"chart-surface", "chart-grid", "chart-axis",
	"series-1", "series-2", "series-3", "series-4", "series-5", "series-6", "series-7", "series-8",
	"status-good", "status-warning", "status-serious", "status-critical"}

var (
	textTokens   = []string{"fg", "muted", "accent", "ok", "warn"}
	textGrounds  = []string{"bg", "code-bg"}
	seriesTokens = append([]string(nil), Tokens[11:19]...)
	statusTokens = append([]string(nil), Tokens[19:23]...)
)

// The thresholds. Those marked skill are the dataviz skill's own; the rest
// are the strictest its reference palette passes, rounded down to one
// decimal. See docs/specs/2026-10-13-style-check-design.md.
const (
	textError       = 3.0  // WCAG, large text
	textWarning     = 4.5  // WCAG AA
	chromaFloor     = 0.10 // skill
	normalFloor     = 15.0 // skill
	cvdFloor        = 6.0  // skill
	cvdTarget       = 8.0  // skill
	statusApart     = 13.0
	statusFromSerie = 4.5
)

type band struct{ lo, hi float64 }

// Per theme: the series lightness band (skill), and the series and status
// contrast floors on --chart-surface.
var (
	lightnessBand  = map[string]band{"light": {0.43, 0.77}, "dark": {0.48, 0.67}}
	seriesContrast = map[string]float64{"light": 1.9, "dark": 3.0}
	statusContrast = map[string]float64{"light": 1.6, "dark": 3.0}
)

// Check measures the theme colours of an assembled stylesheet.
func Check(css string) []Finding {
	decls := readBlocks(css)
	views := map[*view]map[string]string{}
	for _, v := range []*view{&viewL1, &viewL2, &viewL3, &viewD1, &viewD2, &viewD3} {
		views[v] = v.values(decls)
	}

	var out []Finding
	add := func(sev Severity, theme string, msg string, tokens ...string) {
		out = append(out, Finding{sev, theme, tokens, msg})
	}

	// Where each token is set: bare :root and the light block set it for
	// light, the rest for dark.
	setIn := map[string]map[string]bool{"light": {}, "dark": {}}
	for _, d := range decls {
		if d.kind == rootBlock || d.kind == lightBlock {
			setIn["light"][d.name] = true
		} else {
			setIn["dark"][d.name] = true
		}
	}

	// Presence, agreement between views, and parsing.
	colours := map[string]map[string]rgb{"light": {}, "dark": {}}
	for _, t := range Tokens {
		inL, inD := setIn["light"][t], setIn["dark"][t]
		switch {
		case !inL && !inD:
			add(Warning, "", fmt.Sprintf("--%s is not set", t), t)
			continue
		case !inD:
			add(Error, "", fmt.Sprintf("--%s is set in light but not in dark", t), t)
		case !inL:
			add(Error, "", fmt.Sprintf("--%s is set in dark but not in light", t), t)
		}
		for _, p := range []struct {
			theme  string
			main   *view
			others []*view
		}{
			{"light", &viewL2, []*view{&viewL1, &viewL3}},
			{"dark", &viewD2, []*view{&viewD1, &viewD3}},
		} {
			value, ok := views[p.main][t]
			if !ok {
				value = "not set"
			}
			// Views that show another value, grouped by the value they show.
			var shown []string
			where := map[string][]string{}
			for _, o := range p.others {
				ov, set := views[o][t]
				if !set {
					ov = "not set"
				}
				if set == ok && (!ok || same(ov, value)) {
					continue
				}
				if where[ov] == nil {
					shown = append(shown, ov)
				}
				where[ov] = append(where[ov], o.name)
			}
			for _, ov := range shown {
				add(Error, p.theme, fmt.Sprintf("%s --%s is %s (%s) but %s (%s)",
					p.theme, t, value, p.main.name, ov, strings.Join(where[ov], "; ")), t)
			}
			if !ok { // nothing to measure; the rules skip it
				continue
			}
			c, kind := parseColour(value)
			switch kind {
			case measurable:
				colours[p.theme][t] = c
			case translucent:
				add(Warning, p.theme, fmt.Sprintf("%s --%s is not checked: %s is translucent", p.theme, t, value), t)
			case unmeasurable:
				add(Warning, p.theme, fmt.Sprintf("%s --%s is not checked: %s cannot be measured", p.theme, t, value), t)
			case notColour:
				add(Error, p.theme, fmt.Sprintf("%s --%s is not a colour: %s", p.theme, t, value), t)
			}
		}
	}

	// A plain :root in the dark query that no dark view takes is a mistake,
	// usually for the guarded media block. Other declarations that lose are
	// not: a stacked style overrides the default's on purpose.
	shows := map[int]bool{}
	var darkWinners []map[string]int
	for _, v := range []*view{&viewD1, &viewD2, &viewD3} {
		w := v.winners(decls)
		for _, i := range w {
			shows[i] = true
		}
		if v != &viewD2 { // the views a plain :root in the query is in
			darkWinners = append(darkWinners, w)
		}
	}
	lastDarkRoot := map[string]int{}
	for i, d := range decls {
		if d.kind == darkRootBlock {
			lastDarkRoot[d.name] = i
		}
	}
	for _, t := range Tokens {
		i, ok := lastDarkRoot[t]
		if !ok || shows[i] {
			continue
		}
		var by []string
		for _, w := range darkWinners {
			if b := blockName[decls[w[t]].kind]; !slices.Contains(by, b) {
				by = append(by, b)
			}
		}
		add(Warning, "dark", fmt.Sprintf("dark --%s set in a plain :root inside the dark media query never shows; it loses to %s",
			t, strings.Join(by, " and ")), t)
	}

	for _, theme := range []string{"light", "dark"} {
		c := colours[theme]
		both := func(ts ...string) bool {
			for _, t := range ts {
				if _, ok := c[t]; !ok {
					return false
				}
			}
			return true
		}

		for _, t := range textTokens {
			for _, g := range textGrounds {
				if !both(t, g) {
					continue
				}
				r := contrast(c[t], c[g])
				switch {
				case r < textError:
					add(Error, theme, fmt.Sprintf("%s --%s on --%s is %s:1, under 3:1", theme, t, g, down(r, 2)), t, g)
				case r < textWarning:
					add(Warning, theme, fmt.Sprintf("%s --%s on --%s is %s:1, under 4.5:1 (WCAG AA)", theme, t, g, down(r, 2)), t, g)
				}
			}
		}

		bd := lightnessBand[theme]
		for _, t := range seriesTokens {
			if !both(t) {
				continue
			}
			l, ch := lightnessChroma(c[t])
			if l < bd.lo {
				add(Error, theme, fmt.Sprintf("%s --%s lightness is %s, under %.2f", theme, t, down(l, 3), bd.lo), t)
			} else if l > bd.hi {
				add(Error, theme, fmt.Sprintf("%s --%s lightness is %s, over %.2f", theme, t, up(l, 3), bd.hi), t)
			}
			if ch < chromaFloor {
				add(Error, theme, fmt.Sprintf("%s --%s chroma is %s, under %.2f, so it reads as grey", theme, t, down(ch, 3), chromaFloor), t)
			}
		}
		for i := 0; i+1 < len(seriesTokens); i++ {
			a, b := seriesTokens[i], seriesTokens[i+1]
			if !both(a, b) {
				continue
			}
			if d := deltaE(c[a], c[b], normal); d < normalFloor {
				add(Error, theme, fmt.Sprintf("%s --%s and --%s are ΔE %s apart, under 15", theme, a, b, down(d, 1)), a, b)
			}
			switch d := colourBlind(c[a], c[b]); {
			case d < cvdFloor:
				add(Error, theme, fmt.Sprintf("%s --%s and --%s are ΔE %s apart for colour-blind readers, under 6", theme, a, b, down(d, 1)), a, b)
			case d < cvdTarget:
				add(Note, theme, fmt.Sprintf("%s --%s and --%s are ΔE %s apart for colour-blind readers, under 8", theme, a, b, down(d, 1)), a, b)
			}
		}

		if both("chart-surface") {
			for _, group := range []struct {
				tokens []string
				floor  float64
			}{{seriesTokens, seriesContrast[theme]}, {statusTokens, statusContrast[theme]}} {
				for _, t := range group.tokens {
					if !both(t) {
						continue
					}
					if r := contrast(c[t], c["chart-surface"]); r < group.floor {
						add(Warning, theme, fmt.Sprintf("%s --%s on --chart-surface is %s:1, under %g:1", theme, t, down(r, 2), group.floor), t, "chart-surface")
					}
				}
			}
		}
		for i, a := range statusTokens {
			for _, b := range statusTokens[i+1:] {
				if !both(a, b) {
					continue
				}
				if d := deltaE(c[a], c[b], normal); d < statusApart {
					add(Warning, theme, fmt.Sprintf("%s --%s and --%s are ΔE %s apart, under 13", theme, a, b, down(d, 1)), a, b)
				}
			}
			for _, s := range seriesTokens {
				if !both(a, s) {
					continue
				}
				if d := deltaE(c[a], c[s], normal); d < statusFromSerie {
					add(Warning, theme, fmt.Sprintf("%s --%s is ΔE %s from --%s, under 4.5", theme, a, down(d, 1), s), a, s)
				}
			}
		}
	}

	order := map[string]int{}
	for i, t := range Tokens {
		order[t] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		if order[a.Tokens[0]] != order[b.Tokens[0]] {
			return order[a.Tokens[0]] < order[b.Tokens[0]]
		}
		return themeRank[a.Theme] < themeRank[b.Theme]
	})
	return out
}

// blockName names a block that outranks a plain :root in the dark query.
// Only the last such :root is reported, so none of its own kind outranks
// it.
var blockName = map[int]string{
	rootBlock:  "a later plain :root",
	mediaBlock: `:root:not([data-theme="light"])`,
	darkBlock:  `:root[data-theme="dark"]`,
}

var themeRank = map[string]int{"": 0, "light": 1, "dark": 2}

// same reports whether two values are one colour, however written.
func same(a, b string) bool {
	if a == b {
		return true
	}
	x, kx := parseColour(a)
	y, ky := parseColour(b)
	if kx != measurable || ky != measurable {
		return false
	}
	for i := range x {
		if math.Round(x[i]*255) != math.Round(y[i]*255) {
			return false
		}
	}
	return true
}

// down and up format v to places decimals, rounding toward the side that
// keeps a failing value on the failing side of its threshold: 1.8999 under
// 1.9 shows as 1.89, never 1.90.
func down(v float64, places int) string {
	p := math.Pow(10, float64(places))
	return fmt.Sprintf("%.*f", places, math.Floor(v*p)/p)
}

func up(v float64, places int) string {
	p := math.Pow(10, float64(places))
	return fmt.Sprintf("%.*f", places, math.Ceil(v*p)/p)
}
