package stylecheck

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// The dataviz skill's reference palette, as claude-artifact sets it, with
// the page colours claude-artifact pairs it with, and claude-artifact's
// syntax colours.
var referenceLight = map[string]string{
	"bg": "#f4f6f8", "fg": "#1b2027", "muted": "#56616d", "accent": "#2c5a85", "rule": "#d9dfe6",
	"code-bg": "#e9edf1", "ok": "#2c6b45", "warn": "#a8540d",
	"chart-surface": "var(--bg)", "chart-grid": "#e1e0d9", "chart-axis": "#c3c2b7",
	"series-1": "#2a78d6", "series-2": "#eb6834", "series-3": "#1baf7a", "series-4": "#eda100",
	"series-5": "#e87ba4", "series-6": "#008300", "series-7": "#6250d6", "series-8": "#e34948",
	"status-good": "#0ca30c", "status-warning": "#fab219", "status-serious": "#ec835a", "status-critical": "#d03b3b",
	"syntax-comment": "#6e617b", "syntax-keyword": "#002cd2", "syntax-string": "#d60408", "syntax-number": "#7d6100",
	"syntax-constant": "#8b017f", "syntax-function": "#00544e", "syntax-type": "#5d01a7", "syntax-property": "#0066b7",
	"syntax-tag": "#c105aa", "syntax-attribute": "#703503", "syntax-inserted": "#09794c", "syntax-deleted": "#9c0238",
	"syntax-heading": "#660cfe",
}

var referenceDark = map[string]string{
	"bg": "#111519", "fg": "#e2e7ec", "muted": "#95a0ac", "accent": "#8db6df", "rule": "#29313a",
	"code-bg": "#1a2027", "ok": "#71c08f", "warn": "#eb9a55",
	"chart-surface": "var(--bg)", "chart-grid": "#2c2c2a", "chart-axis": "#383835",
	"series-1": "#3987e5", "series-2": "#d95926", "series-3": "#199e70", "series-4": "#c98500",
	"series-5": "#d55181", "series-6": "#008300", "series-7": "#9085e9", "series-8": "#e66767",
	"status-good": "#0ca30c", "status-warning": "#fab219", "status-serious": "#ec835a", "status-critical": "#d03b3b",
	"syntax-comment": "#89898a", "syntax-keyword": "#8ab1d8", "syntax-string": "#ff9166", "syntax-number": "#bd9402",
	"syntax-constant": "#ff81b9", "syntax-function": "#05b8a4", "syntax-type": "#ecb6ff", "syntax-property": "#0192d4",
	"syntax-tag": "#c762bb", "syntax-attribute": "#ffc66b", "syntax-inserted": "#50e3a5", "syntax-deleted": "#cf6c51",
	"syntax-heading": "#b991ff",
}

// sheet writes light and dark into the four theme blocks, as the guide
// has a style write them, each changed by its own overrides.
func sheet(light, dark map[string]string, lightOver, darkOver map[string]string) string {
	block := func(base, over map[string]string) string {
		var b strings.Builder
		for _, t := range Tokens {
			v, ok := base[t]
			if o, set := over[t]; set {
				v, ok = o, o != ""
			}
			if ok {
				fmt.Fprintf(&b, "  --%s: %s;\n", t, v)
			}
		}
		return b.String()
	}
	l, d := block(light, lightOver), block(dark, darkOver)
	return ":root {\n" + l + "}\n" +
		"@media (prefers-color-scheme: dark) {\n  :root:not([data-theme=\"light\"]) {\n" + d + "  }\n}\n" +
		":root[data-theme=\"dark\"] {\n" + d + "}\n" +
		":root[data-theme=\"light\"] {\n" + l + "}\n"
}

func messages(fs []Finding) string {
	var b strings.Builder
	for _, f := range fs {
		fmt.Fprintf(&b, "%s: %s\n", f.Severity, f.Message)
	}
	return b.String()
}

func TestReferencePaletteHasNoFindings(t *testing.T) {
	if fs := Check(sheet(referenceLight, referenceDark, nil, nil)); len(fs) != 0 {
		t.Errorf("the skill's reference palette has findings:\n%s", messages(fs))
	}
}

// The maths matches validate_palette.js to the digits it prints, on values
// it was run on.
func TestMathsMatchTheSkill(t *testing.T) {
	c := func(s string) rgb {
		x, k := parseColour(s)
		if k != measurable {
			t.Fatalf("%s did not parse", s)
		}
		return x
	}
	for _, x := range []struct {
		name      string
		got, want float64
	}{
		{"contrast", contrast(c("#eda100"), c("#f4f6f8")), 1.9986269605917921},
		{"normal", deltaE(c("#e87ba4"), c("#eda100"), normal), 19.63442497845986},
		{"protan", deltaE(c("#eda100"), c("#1baf7a"), protan), 9.115246557144896},
		{"deutan", deltaE(c("#eda100"), c("#1baf7a"), deutan), 16.528461957439706},
	} {
		if math.Abs(x.got-x.want) > 1e-9 {
			t.Errorf("%s = %v, want %v", x.name, x.got, x.want)
		}
	}
	l, ch := lightnessChroma(c("#eda100"))
	if math.Abs(l-0.764424838486165) > 1e-9 || math.Abs(ch-0.1611804987400975) > 1e-9 {
		t.Errorf("L, C = %v, %v", l, ch)
	}
}

func TestParseColour(t *testing.T) {
	for _, x := range []struct {
		in   string
		want parsed
		hex  string
	}{
		{"#abc", measurable, "#aabbcc"},
		{"#AABBCC", measurable, "#aabbcc"},
		{"#aabbccff", measurable, "#aabbcc"},
		{"#abcf", measurable, "#aabbcc"},
		{"#aabbcc80", translucent, ""},
		{"rgb(170, 187, 204)", measurable, "#aabbcc"},
		{"rgb(170 187 204)", measurable, "#aabbcc"},
		{"rgb(170 187 204 / 1)", measurable, "#aabbcc"},
		{"rgba(170, 187, 204, 0.5)", translucent, ""},
		{"rgb(100% 0% 0%)", measurable, "#ff0000"},
		{"hsl(120, 100%, 25%)", measurable, "#008000"},
		{"hsl(120deg 100% 25%)", measurable, "#008000"},
		{"hsl(-120, 100%, 50%)", measurable, "#0000ff"},
		{"hsl(480, 100%, 50%)", measurable, "#00ff00"},
		{"hsl(0.5turn 100% 50%)", measurable, "#00ffff"},
		{"hsl(200grad 100% 50%)", measurable, "#00ffff"},
		{"hsl(3.14159265358979rad 100% 50%)", measurable, "#00ffff"},
		{"hsl(120 100 25)", measurable, "#008000"},
		{"hsl(none 100% 50%)", measurable, "#ff0000"},
		{"rgb(none 0 0 / none)", measurable, "#000000"},
		{"oklch(0.6 0.1 none)", measurable, "#b1667e"},
		{"HSL(120 100% 25% / 0.5)", translucent, ""},
		{"Canvas", unmeasurable, ""},
		{"linktext", unmeasurable, ""},
		{"AccentColorText", unmeasurable, ""},
		{"rgb(1 2 3 / )", notColour, ""},
		{"oklch(62.8% 0.2577 29.23)", measurable, "#ff0000"},
		{"oklab(0.628 0.2249 0.1258)", measurable, "#ff0000"},
		{"rebeccapurple", measurable, "#663399"},
		{"transparent", translucent, ""},
		{"currentColor", unmeasurable, ""},
		{"color-mix(in oklab, red, blue)", unmeasurable, ""},
		{"light-dark(#fff, #000)", unmeasurable, ""},
		{"rgb(from red r g b)", unmeasurable, ""},
		{"var(--nowhere)", unmeasurable, ""},
		{"#ggg", notColour, ""},
		{"#abcde", notColour, ""},
		{"redd", notColour, ""},
		{"hsl(120, 100, 25)", notColour, ""},
		{"10px", notColour, ""},
	} {
		got, kind := parseColour(x.in)
		if kind != x.want {
			t.Errorf("%q: kind %d, want %d", x.in, kind, x.want)
			continue
		}
		if x.hex != "" {
			h := fmt.Sprintf("#%02x%02x%02x", int(math.Round(got[0]*255)), int(math.Round(got[1]*255)), int(math.Round(got[2]*255)))
			if h != x.hex {
				t.Errorf("%q = %s, want %s", x.in, h, x.hex)
			}
		}
	}
}

// Each rule, broken once on the reference palette, gives exactly its own
// finding at its own severity.
func TestEachRule(t *testing.T) {
	for _, x := range []struct {
		name        string
		light, dark map[string]string
		want        string
	}{
		{"set in light only", nil, map[string]string{"rule": ""},
			"error: --rule is set in light but not in dark\n"},
		{"not a colour", map[string]string{"rule": "#d9dfe"}, nil,
			"error: light --rule is not a colour: #d9dfe\n"},
		{"unmeasurable", map[string]string{"rule": "color-mix(in srgb, red, blue)"}, nil,
			"warning: light --rule is not checked: color-mix(in srgb, red, blue) cannot be measured\n"},
		{"translucent", map[string]string{"rule": "#d9dfe680"}, nil,
			"warning: light --rule is not checked: #d9dfe680 is translucent\n"},
		{"text under 3:1", nil, map[string]string{"muted": "#3a4048"},
			"error: dark --muted on --bg is 1.75:1, under 3:1\nerror: dark --muted on --code-bg is 1.56:1, under 3:1\n"},
		{"text under 4.5:1", map[string]string{"warn": "#b4600d"}, nil,
			"warning: light --warn on --bg is 4.19:1, under 4.5:1 (WCAG AA)\nwarning: light --warn on --code-bg is 3.86:1, under 4.5:1 (WCAG AA)\n"},
		{"series too dark", map[string]string{"series-6": "#004d00"}, nil,
			"error: light --series-6 lightness is 0.364, under 0.43\n"},
		{"series grey", map[string]string{"series-7": "#6f6f80"}, nil,
			"error: light --series-7 chroma is 0.026, under 0.10, so it reads as grey\n"},
		{"series too light", nil, map[string]string{"series-1": "#4aa0ff"},
			"error: dark --series-1 lightness is 0.698, over 0.67\n"},
		{"neighbours alike", map[string]string{"series-2": "#a8a300"}, nil,
			"error: light --series-2 and --series-3 are ΔE 13.6 apart, under 15\n"},
		{"neighbours alike to everyone", map[string]string{"series-2": "#2db27f"}, nil,
			"error: light --series-2 and --series-3 are ΔE 1.3 apart, under 15\n" +
				"error: light --series-2 and --series-3 are ΔE 1.1 apart for colour-blind readers, under 6\n"},
		{"neighbours alike when colour-blind", map[string]string{"series-2": "#7733dd"}, nil,
			"error: light --series-1 and --series-2 are ΔE 5.7 apart for colour-blind readers, under 6\n"},
		{"neighbours in the floor band", map[string]string{"series-2": "#6633dd"}, nil,
			"note: light --series-1 and --series-2 are ΔE 7.1 apart for colour-blind readers, under 8\n"},
		{"light series faint", map[string]string{"series-4": "#bbbb00"}, nil,
			"warning: light --series-4 on --chart-surface is 1.89:1, under 1.9:1\n"},
		{"dark series faint", nil, map[string]string{"series-1": "#0044ee"},
			"warning: dark --series-1 on --chart-surface is 2.67:1, under 3:1\n"},
		{"status faint", nil, map[string]string{"status-critical": "#a82e2e"},
			"warning: dark --status-critical on --chart-surface is 2.70:1, under 3:1\n"},
		{"status alike", map[string]string{"status-serious": "#008855"}, nil,
			"warning: light --status-good and --status-serious are ΔE 11.4 apart, under 13\n"},
		{"status copies a series", map[string]string{"series-8": "#d03c3c"}, nil,
			"warning: light --status-critical is ΔE 0.1 from --series-8, under 4.5\n"},
		{"syntax under 4.5:1", nil, map[string]string{"syntax-property": "#1b71bc"},
			"warning: dark --syntax-property on --code-bg is 3.23:1, under 4.5:1 (WCAG AA)\n"},
		{"syntax kinds alike", map[string]string{"syntax-type": "#d10407"}, nil,
			"warning: light --syntax-string and --syntax-type are ΔE 1.0 apart, under 12\n"},
	} {
		got := messages(Check(sheet(referenceLight, referenceDark, x.light, x.dark)))
		if got != x.want {
			t.Errorf("%s:\n got %s\nwant %s", x.name, got, x.want)
		}
	}
}

func TestNotSetAnywhereWarnsOnce(t *testing.T) {
	got := messages(Check(sheet(referenceLight, referenceDark, map[string]string{"rule": ""}, map[string]string{"rule": ""})))
	if got != "warning: --rule is not set\n" {
		t.Errorf("got %s", got)
	}
}

// A style that sets only the media block for dark leaves a reader who
// picks dark on a light system with the light value.
func TestViewsMustAgree(t *testing.T) {
	css := sheet(referenceLight, referenceDark, nil, nil) +
		"@media (prefers-color-scheme: dark) { :root:not([data-theme=\"light\"]) { --rule: #333a42; } }\n"
	got := messages(Check(css))
	want := "error: dark --rule is #29313a (dark picked on a light system) but #333a42 (dark system, nothing picked; dark picked on a dark system)\n"
	if got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
	// The same colour written two ways agrees.
	css = sheet(referenceLight, referenceDark, nil, nil) + ":root { --rule: rgb(217, 223, 230); }\n"
	if fs := Check(css); len(fs) != 0 {
		t.Errorf("one colour, two spellings: %s", messages(fs))
	}
}

// Comments do not count, and a later, stacked style wins.
func TestStackingAndComments(t *testing.T) {
	base := sheet(referenceLight, referenceDark, nil, nil)
	over := sheet(map[string]string{"rule": "#cfd6de"}, map[string]string{"rule": "#2f3842"}, nil, nil)
	if fs := Check(base + "/* :root { --rule: nonsense; } */\n" + over); len(fs) != 0 {
		t.Errorf("got %s", messages(fs))
	}
}

func TestFindingsAreOrdered(t *testing.T) {
	fs := Check(sheet(referenceLight, referenceDark,
		map[string]string{"series-8": "#d03c3c", "rule": "#d9dfe"},
		map[string]string{"muted": "#3a4048", "series-1": "#0044ee"}))
	got := messages(fs)
	want := "error: dark --muted on --bg is 1.75:1, under 3:1\n" +
		"error: dark --muted on --code-bg is 1.56:1, under 3:1\n" +
		"error: light --rule is not a colour: #d9dfe\n" +
		"warning: dark --series-1 on --chart-surface is 2.67:1, under 3:1\n" +
		"warning: light --status-critical is ΔE 0.1 from --series-8, under 4.5\n"
	if got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}

func TestReportAndShort(t *testing.T) {
	one := []Finding{{Error, "light", []string{"fg"}, "light --fg on --bg is 2.10:1, under 3:1"}}
	many := append(one,
		Finding{Warning, "dark", []string{"warn"}, "w1"},
		Finding{Warning, "dark", []string{"warn"}, "w2"},
		Finding{Note, "dark", []string{"series-1"}, "n1"})
	if got := Report("acme/plain", nil); got != "acme/plain: no problems\n" {
		t.Errorf("Report none = %q", got)
	}
	if got, want := Report("acme/plain", many), "acme/plain: 1 error, 2 warnings, 1 note\n"+
		"  error    light --fg on --bg is 2.10:1, under 3:1\n  warning  w1\n  warning  w2\n  note     n1\n"; got != want {
		t.Errorf("Report = %q, want %q", got, want)
	}
	if got := Short(one); got != "light --fg on --bg is 2.10:1, under 3:1" {
		t.Errorf("Short one = %q", got)
	}
	if got := Short(many[:2]); got != "light --fg on --bg is 2.10:1, under 3:1; w1" {
		t.Errorf("Short two = %q", got)
	}
	if got := Short(many); got != "1 error, 2 warnings, 1 note" {
		t.Errorf("Short many = %q", got)
	}
	if !HasError(many) || HasError(many[1:]) {
		t.Error("HasError")
	}
}

func TestOklchHueWraps(t *testing.T) {
	a, _ := parseColour("oklch(0.6 0.1 -90)")
	b, _ := parseColour("oklch(0.6 0.1 270)")
	c, _ := parseColour("oklch(0.6 0.1 0.75turn)")
	if a != b || b != c {
		t.Errorf("one hue, three spellings: %v %v %v", a, b, c)
	}
}

// A measured colour is an 8-bit one, as the skill's validator measures hex.
func TestColoursAreQuantised(t *testing.T) {
	c, _ := parseColour("oklch(0.6 0.15 250)")
	hex := fmt.Sprintf("#%02x%02x%02x", int(math.Round(c[0]*255)), int(math.Round(c[1]*255)), int(math.Round(c[2]*255)))
	h, _ := parseColour(hex)
	white, _ := parseColour("#fff")
	if contrast(c, white) != contrast(h, white) || c != h {
		t.Errorf("oklch and %s measure differently: %v vs %v", hex, c, h)
	}
}

func TestImportantIsStripped(t *testing.T) {
	for _, v := range []string{"#d9dfe6 !important", "#d9dfe6!IMPORTANT", "#d9dfe6 !  important "} {
		got := messages(Check(sheet(referenceLight, referenceDark, map[string]string{"rule": v}, nil)))
		if got != "" {
			t.Errorf("%q: %s", v, got)
		}
	}
	d := readBlocks(":root { --rule: #fff !important; }")
	if len(d) != 1 || d[0].value != "#fff" {
		t.Errorf("declared value = %+v", d)
	}
}

// A token set in the light and media blocks only is not set for a reader
// who picks dark on a light system.
func TestMainViewNotSet(t *testing.T) {
	css := sheet(referenceLight, referenceDark, map[string]string{"rule": ""}, map[string]string{"rule": ""}) +
		":root[data-theme=\"light\"] { --rule: #d9dfe6; }\n" +
		"@media (prefers-color-scheme: dark) { :root:not([data-theme=\"light\"]) { --rule: #444444; } }\n"
	got := messages(Check(css))
	want := "error: light --rule is #d9dfe6 (light picked) but not set (light system, nothing picked)\n" +
		"error: dark --rule is not set (dark picked on a light system) but #444444 (dark system, nothing picked; dark picked on a dark system)\n"
	if got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}

func TestSelectorShapes(t *testing.T) {
	// A bare :root inside the dark media query is its own block. Inside any
	// other at-rule a theme selector is not a theme block, but @supports
	// and @layer only wrap.
	d := readBlocks("@media (prefers-color-scheme: dark) { :root { --a: 1; } }\n" +
		"@media print { :root { --b: 2; } }\n@media (min-width: 40em) { :root[data-theme=\"dark\"] { --c: 3; } }\n" +
		"@media (prefers-color-scheme: light) { :root { --d: 4; } }\n@container (min-width: 1px) { :root { --e: 5; } }\n" +
		"@supports (color: red) { :root { --f: 6; } }\n@layer x { @media (prefers-color-scheme: dark) { @supports (color: red) { :root { --g: 7; } } } }")
	if len(d) != 3 || d[0].kind != darkRootBlock || d[1].kind != rootBlock || d[1].name != "f" || d[2].kind != darkRootBlock || d[2].name != "g" {
		t.Errorf("media: %+v", d)
	}
	// A selector list applies its declarations to each listed block.
	d = readBlocks(":root, :root[data-theme=\"light\"] { --a: 1; --b: 2; }")
	if len(d) != 4 || d[0].kind != rootBlock || d[1].kind != rootBlock || d[2].kind != lightBlock || d[3].kind != lightBlock {
		t.Errorf("list: %+v", d)
	}
	// A rule that is not a theme block is left alone.
	if d = readBlocks("body { --a: 1; } .x :root { --b: 2; }"); len(d) != 0 {
		t.Errorf("other: %+v", d)
	}
	// The same, through the whole check.
	css := ":root, :root[data-theme=\"light\"] { --rule: #d9dfe6; }\n" +
		"@media (prefers-color-scheme: dark) { :root { --rule: #29313a; } }\n" +
		":root[data-theme=\"dark\"] { --rule: #29313a; }\n"
	base := sheet(referenceLight, referenceDark, map[string]string{"rule": ""}, map[string]string{"rule": ""})
	if fs := Check(base + css); len(fs) != 0 {
		t.Errorf("got %s", messages(fs))
	}
}

// A later bare :root still loses to an earlier theme block.
func TestBareRootLosesToEarlierBlock(t *testing.T) {
	css := ":root[data-theme=\"dark\"] { --rule: #29313a; }\n:root { --rule: #d9dfe6; }\n"
	if got := viewD2.values(readBlocks(css))["rule"]; got != "#29313a" {
		t.Errorf("D2 --rule = %s", got)
	}
	if got := viewL1.values(readBlocks(css))["rule"]; got != "#d9dfe6" {
		t.Errorf("L1 --rule = %s", got)
	}
}

func TestSeverityStringIsTotal(t *testing.T) {
	if Severity(7).String() != "unknown" || Severity(-1).String() != "unknown" || Note.String() != "note" {
		t.Error("Severity.String")
	}
}

func TestSeriesTokensAreCopies(t *testing.T) {
	seriesTokens[0] = "x"
	defer func() { seriesTokens[0] = "series-1" }()
	if Tokens[11] != "series-1" {
		t.Error("seriesTokens aliases Tokens")
	}
}

// A bare :root inside the dark media query loses to the default's media
// block, which is more specific, so a reader on a dark system who picked
// nothing still sees the default's colour.
func TestBareRootInDarkQueryLosesToMediaBlock(t *testing.T) {
	css := sheet(referenceLight, referenceDark, nil, nil) +
		":root { --muted: #56616d; }\n:root[data-theme=\"light\"] { --muted: #56616d; }\n" +
		":root[data-theme=\"dark\"] { --muted: #a0aab4; }\n" +
		"@media (prefers-color-scheme: dark) { :root { --muted: #a0aab4; } }\n"
	got := messages(Check(css))
	want := "error: dark --muted is #a0aab4 (dark picked on a light system) but #95a0ac (dark system, nothing picked)\n" +
		"warning: " + neverShows("muted") + "\n"
	if got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}

// A bare :root inside the dark media query has no :not guard, so it shows
// to a reader who picked light on a dark system unless the light block
// sets the token.
func TestLightPickedOnDarkSystem(t *testing.T) {
	css := sheet(referenceLight, referenceDark, map[string]string{"rule": ""}, map[string]string{"rule": ""}) +
		":root { --rule: #d9dfe6; }\n" +
		"@media (prefers-color-scheme: dark) { :root { --rule: #29313a; } }\n" +
		":root[data-theme=\"dark\"] { --rule: #29313a; }\n"
	got := messages(Check(css))
	want := "error: light --rule is #d9dfe6 (light picked) but #29313a (light picked on a dark system)\n"
	if got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}

// A print override of the page colours is not a theme block.
func TestPrintOverrideIsSkipped(t *testing.T) {
	css := sheet(referenceLight, referenceDark, nil, nil) + "@media print { :root { --bg: #fff; --fg: #000 } }\n"
	if fs := Check(css); len(fs) != 0 {
		t.Errorf("got %s", messages(fs))
	}
}

func TestAngleRejectsInfAndNaN(t *testing.T) {
	for _, s := range []string{"oklch(0.6 0.1 inf)", "oklch(0.6 0.1 NaN)", "hsl(-Inf 50% 50%)", "oklch(0.6 0.1 1e400deg)"} {
		if _, kind := parseColour(s); kind != notColour {
			t.Errorf("%s: kind %v, want notColour", s, kind)
		}
	}
}

// shape lists decls as kind:name=value, for comparing in tests.
func shape(d []decl) string {
	var b strings.Builder
	for _, x := range d {
		fmt.Fprintf(&b, "%d:%s=%s ", x.kind, x.name, x.value)
	}
	return b.String()
}

// A nested rule is skipped whole; the block's own declarations after it
// still count, and in the dark query it does not end the media block.
func TestNestedRulesAreSkipped(t *testing.T) {
	css := ":root { --a: 1; .x { --b: 2; } --c: 3; }\n" +
		"@media (prefers-color-scheme: dark) { :root:not([data-theme=\"light\"]) { --d: 4; &:hover { --e: 5 } " +
		"@media (min-width: 1px) { --f: 6 } --g: 7; } :root { --h: 8; } }\n:root[data-theme=\"dark\"] { --i: 9 }"
	want := fmt.Sprintf("%d:a=1 %d:c=3 %d:d=4 %d:g=7 %d:h=8 %d:i=9 ", rootBlock, rootBlock, mediaBlock, mediaBlock, darkRootBlock, darkBlock)
	if got := shape(readBlocks(css)); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// A brace or semicolon in a quoted string is not structure.
func TestBracesInStrings(t *testing.T) {
	css := `:root { --font: "a{b;c}"; --a: 1; }` + "\n" +
		`:root[data-theme='light'] { --q: 'x}y\'z;'; --b: 2 }` + "\n" +
		`.x[title="}"] { --z: 0 } :root { --c: 3 }` + "\n" +
		`@media (prefers-color-scheme: dark) { .y[title='{'] { --z: 0 } :root { --d: 4 } }` + "\n" +
		`:root { --e: "/* not a comment */"; /* a comment { */ --f: 6 }`
	want := fmt.Sprintf(`%d:font="a{b;c}" %d:a=1 %d:q='x}y\'z;' %d:b=2 %d:c=3 %d:d=4 %d:e="/* not a comment */" %d:f=6 `,
		rootBlock, rootBlock, lightBlock, lightBlock, rootBlock, darkRootBlock, rootBlock, rootBlock)
	if got := shape(readBlocks(css)); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// Only a query that is the dark scheme and nothing more is the dark
// query; any other condition makes it an at-rule like @media print.
func TestDarkQueryShapes(t *testing.T) {
	for _, q := range []string{
		"@media (prefers-color-scheme: dark)", "@MEDIA (Prefers-Color-Scheme:DARK)", "@media(prefers-color-scheme:dark)",
		"@media screen and (prefers-color-scheme: dark)", "@media all and ( prefers-color-scheme : dark )",
		"@media only screen and (prefers-color-scheme: dark)", "@media\n\tscreen\nand\n(prefers-color-scheme: dark)",
	} {
		d := readBlocks(q + " { :root { --a: 1 } }")
		if len(d) != 1 || d[0].kind != darkRootBlock {
			t.Errorf("%q: %+v, want the dark query", q, d)
		}
	}
	for _, q := range []string{
		"@media not (prefers-color-scheme: dark)", "@media (prefers-color-scheme: dark) and (min-width: 40em)",
		"@media print and (prefers-color-scheme: dark)", "@media print, (prefers-color-scheme: dark)",
		"@media (prefers-color-scheme: dark), print", "@media not all and (prefers-color-scheme: dark)",
		"@media (prefers-color-scheme: light)", "@media (min-width: 1px) and (prefers-color-scheme: dark)",
	} {
		if d := readBlocks(q + " { :root { --a: 1 } }"); len(d) != 0 {
			t.Errorf("%q: %+v, want it skipped", q, d)
		}
	}
}

// The CSS2 system colours are deprecated but still drawn from the
// reader's settings, so they cannot be measured either.
func TestDeprecatedSystemColoursAreUnmeasurable(t *testing.T) {
	for _, n := range strings.Fields(`ActiveBorder ActiveCaption AppWorkspace Background ButtonHighlight
		ButtonShadow CaptionText InactiveBorder InactiveCaption InactiveCaptionText InfoBackground
		InfoText Menu MenuText Scrollbar ThreeDDarkShadow ThreeDFace ThreeDHighlight
		ThreeDLightShadow ThreeDShadow Window WindowFrame WindowText windowtext WINDOW`) {
		if _, kind := parseColour(n); kind != unmeasurable {
			t.Errorf("%s: kind %v, want unmeasurable", n, kind)
		}
	}
}

// neverShows is the warning for a plain :root in the dark query that
// loses to the default's guarded media block and dark block.
func neverShows(t string) string {
	return "dark --" + t + " set in a plain :root inside the dark media query never shows; " +
		"it loses to :root:not([data-theme=\"light\"]) and :root[data-theme=\"dark\"]"
}

// A plain :root in the dark query that every dark view overrides is
// reported once, for its last declaration. Other overridden declarations
// are not: a stacked style overrides the default's on purpose.
func TestPlainRootInDarkQueryThatNeverShows(t *testing.T) {
	base := sheet(referenceLight, referenceDark, nil, nil)
	css := base + "@media (prefers-color-scheme: dark) { :root { --rule: #29313a; --muted: #95a0ac } :root { --rule: #29313a } }\n" +
		":root { --rule: #d9dfe6 }\n"
	fs := Check(css)
	want := "warning: " + neverShows("muted") + "\nwarning: " + neverShows("rule") + "\n"
	if got := messages(fs); got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
	if len(fs) == 0 || fs[0].Theme != "dark" || len(fs[0].Tokens) != 1 || fs[0].Tokens[0] != "muted" {
		t.Errorf("finding %+v", fs[0])
	}
	// One that wins on a dark system with nothing picked shows.
	css = sheet(referenceLight, referenceDark, nil, map[string]string{"rule": ""}) +
		"@media (prefers-color-scheme: dark) { :root { --rule: #29313a } }\n:root[data-theme=\"dark\"] { --rule: #29313a }\n"
	if got := messages(Check(css)); got != "" {
		t.Errorf("got %s", got)
	}
	// The winner is named: here a later plain :root, outside the query.
	css = ":root{--muted:#666} @media (prefers-color-scheme: dark){:root{--muted:#aaa}} :root{--muted:#666}"
	if got := messages(Check(css)); !strings.Contains(got, "warning: dark --muted set in a plain :root inside the dark media query never shows; it loses to a later plain :root\n") {
		t.Errorf("later :root: got %s", got)
	}
	// A stacked style's blocks override the default's, which is no finding.
	if got := messages(Check(base + base)); got != "" {
		t.Errorf("stacked: got %s", got)
	}
}

// Deeply nested at-rules neither overflow the stack nor take quadratic
// time: a plugin's sheet is read on every build.
func TestDeepAtRuleNesting(t *testing.T) {
	const n = 3_000_000
	css := strings.Repeat("@a{", n) + ":root{--a:1}" + strings.Repeat("}", n) + ":root{--b:2}"
	if d := readBlocks(css); len(d) != 1 || d[0].name != "b" {
		t.Errorf("other: %+v", d)
	}
	css = strings.Repeat("@supports (x){", 40_000) + strings.Repeat(":root{--c:3}", 40_000)
	if d := readBlocks(css); len(d) != 40_000 || d[0].kind != rootBlock {
		t.Errorf("wrapper: %d decls", len(d))
	}
}

// An unquoted url() is one token, so a /* in it opens no comment.
func TestCommentMarkerInUnquotedURL(t *testing.T) {
	css := ":root{--a:url(a/*b); --b:2} :root{--c:URL( x/*y ) /* gone */; --d:4}"
	want := fmt.Sprintf("%d:a=url(a/*b) %d:b=2 %d:c=URL( x/*y ) %d:d=4 ", rootBlock, rootBlock, rootBlock, rootBlock)
	if got := shape(readBlocks(css)); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// A backslash escapes the next character in an unquoted url(), so \)
// does not end it.
func TestEscapedParenInUnquotedURL(t *testing.T) {
	css := ":root{--a:url(a\\)b/*c); --b:2}"
	want := fmt.Sprintf("%d:a=url(a\\)b/*c) %d:b=2 ", rootBlock, rootBlock)
	if got := shape(readBlocks(css)); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// :root and :not match in any case, as CSS reads them; the theme names
// are attribute values, which do not.
func TestRootSelectorCase(t *testing.T) {
	css := "@MEDIA (prefers-color-scheme: DARK) { :ROOT { --a: 1 } :Root:NOT([DATA-THEME=\"light\"]) { --b: 2 } }\n" +
		":root[Data-Theme='dark'] { --c: 3 } :root[data-theme=\"LIGHT\"] { --d: 4 }"
	want := fmt.Sprintf("%d:a=1 %d:b=2 %d:c=3 ", darkRootBlock, mediaBlock, darkBlock)
	if got := shape(readBlocks(css)); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestSyntaxTokensAreTextOnCodeBg(t *testing.T) {
	fs := Check(sheet(referenceLight, referenceDark, map[string]string{"syntax-keyword": "#e9edf1"}, nil))
	if !strings.Contains(messages(fs), "error: light --syntax-keyword on --code-bg is 1.00:1, under 3:1") {
		t.Errorf("findings:\n%s", messages(fs))
	}
}

func TestSyntaxKindsMustBeApart(t *testing.T) {
	fs := Check(sheet(referenceLight, referenceDark,
		map[string]string{"syntax-string": referenceLight["syntax-keyword"]}, nil))
	if !strings.Contains(messages(fs), "warning: light --syntax-keyword and --syntax-string are ΔE 0.0 apart, under 12") {
		t.Errorf("findings:\n%s", messages(fs))
	}
}

func TestSyntaxKindMustStandApartFromFg(t *testing.T) {
	fs := Check(sheet(referenceLight, referenceDark,
		map[string]string{"syntax-comment": referenceLight["fg"]}, nil))
	if !strings.Contains(messages(fs), "warning: light --syntax-comment is ΔE 0.0 from --fg, under 8") {
		t.Errorf("findings:\n%s", messages(fs))
	}
}

// Only the five kinds most code is made of are held apart from --fg.
func TestFgSeparationCoversFiveKinds(t *testing.T) {
	fs := Check(sheet(referenceLight, referenceDark,
		map[string]string{"syntax-tag": referenceLight["fg"]}, nil))
	if strings.Contains(messages(fs), "from --fg") {
		t.Errorf("findings:\n%s", messages(fs))
	}
}

// Every pair of syntax tokens is measured, not only the five kinds.
func TestEverySyntaxPairMustBeApart(t *testing.T) {
	got := messages(Check(sheet(referenceLight, referenceDark,
		map[string]string{"syntax-tag": referenceLight["syntax-attribute"]}, nil)))
	if want := "warning: light --syntax-tag and --syntax-attribute are ΔE 0.0 apart, under 12\n"; got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}

// A pair is measured on what each token resolves to, so two tokens
// written differently can still collide.
func TestSyntaxPairCollidesAfterVarResolution(t *testing.T) {
	got := messages(Check(sheet(referenceLight, referenceDark,
		nil, map[string]string{"syntax-deleted": "var(--syntax-heading)"})))
	if want := "warning: dark --syntax-deleted and --syntax-heading are ΔE 0.0 apart, under 12\n"; got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}

// A token that is not set is reported once as not set, and never as part
// of a pair.
func TestUnsetSyntaxTokenIsNotAPair(t *testing.T) {
	got := messages(Check(sheet(referenceLight, referenceDark,
		map[string]string{"syntax-tag": ""}, map[string]string{"syntax-tag": ""})))
	if want := "warning: --syntax-tag is not set\n"; got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}
