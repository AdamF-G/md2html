# Style Check Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `md2html --check-style` measures a style's theme colours against WCAG and the dataviz skill's rules, builds report problems in one line, and every built-in style passes with no findings.

**Architecture:** A new dependency-free package, `internal/stylecheck`, reads the four theme blocks of an assembled stylesheet and returns findings. The CLI uses it for `--check-style` and, through a new `Options.StyleChosen` hook that `Convert` calls with each page's chosen style list, for a one-line report during builds. The default style's colours change where they fail.

**Tech Stack:** Go 1.26 (module `github.com/AdamF-G/md2html`), standard library only.

**Spec:** `docs/specs/2026-10-13-style-check-design.md`

## Global Constraints

- No new dependencies: `internal/stylecheck` imports the standard library only.
- The colour maths must match the dataviz skill's `validate_palette.js` to 1e-9 on the values pinned in Task 1's tests; the thresholds are exactly the spec's rule table.
- Every built-in style has zero findings of any severity (Task 2's test).
- `Convert` never checks a style; the library only gains `Options.StyleChosen`.
- A build's exit status never changes because of a style finding.
- Classes and ids, if any are added, are prefixed `md2html-` (none are expected).
- Commits use conventional prefixes, carry no Co-Authored-By or other attribution trailer, and are not pushed.
- Messages and docs name no product or project from other repositories; the dataviz skill is "the dataviz skill".
- Run `go vet ./...` and `go test ./...` at the repo root before each commit; the e2e module (`e2e/`) is untouched.

## Review Focus

1. **A style that sets a colour in `:root` only** (no theme blocks): readers who pick a theme see the default's value instead; the check must report the mismatch, not pass. Pinned by `TestViewsMustAgree` (Task 1) and `TestRunStyleCheckLineForFrontMatterStyle` (Task 5, a one-block style).
2. **The same colour written two ways** (`#d9dfe6` in one block, `rgb(217, 223, 230)` in another) must not be reported as a mismatch. Pinned in `TestViewsMustAgree`.
3. **A failing value shown at its threshold**, such as 1.8999 printed as "1.90:1, under 1.9:1": values are truncated toward the failing side. Pinned by the `light series faint` case (1.89) in `TestEachRule`.
4. **Many pages sharing one style list, converted in parallel**: the build line prints once, and the hook is safe under concurrency. Pinned by `TestRunStyleCheckLineForFrontMatterStyle` (two pages, one line) and run under `go test -race` in Task 5.
5. **A colour form the check cannot evaluate** (`color-mix()`, `var()` to nothing, alpha): a warning, never an error that fails `--check-style`. Pinned in `TestParseColour` and `TestEachRule`.

---

### Task 1: The `internal/stylecheck` package

**Files:**
- Create: `internal/stylecheck/colour.go`, `internal/stylecheck/maths.go`, `internal/stylecheck/views.go`, `internal/stylecheck/check.go`, `internal/stylecheck/format.go`
- Test: `internal/stylecheck/check_test.go`

**Interfaces:**
- Produces:
  - `type Severity int` with `Error`, `Warning`, `Note` and `String()` ("error", "warning", "note")
  - `type Finding struct { Severity Severity; Theme string; Tokens []string; Message string }`
  - `var Tokens []string`: the 23 checked tokens, without `--`
  - `func Check(css string) []Finding`: sorted by severity, then token order, then theme ("" < light < dark)
  - `func Counts(fs []Finding) string`: "1 error, 3 warnings, 2 notes"
  - `func HasError(fs []Finding) bool`
  - `func Report(name string, fs []Finding) string`: the full report
  - `func Short(fs []Finding) string`: the one-line form

This code was run before the plan was written: the tests below pass against it, and run on the real stylesheets it reports the five default failures that Task 2 fixes, and no findings for `claude-artifact`.

- [ ] **Step 1: Write the failing tests**

Create `internal/stylecheck/check_test.go`:

```go
package stylecheck

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// The dataviz skill's reference palette, as claude-artifact sets it, with
// the page colours claude-artifact pairs it with.
var referenceLight = map[string]string{
	"bg": "#f4f6f8", "fg": "#1b2027", "muted": "#56616d", "accent": "#2c5a85", "rule": "#d9dfe6",
	"code-bg": "#e9edf1", "ok": "#2c6b45", "warn": "#a8540d",
	"chart-surface": "var(--bg)", "chart-grid": "#e1e0d9", "chart-axis": "#c3c2b7",
	"series-1": "#2a78d6", "series-2": "#eb6834", "series-3": "#1baf7a", "series-4": "#eda100",
	"series-5": "#e87ba4", "series-6": "#008300", "series-7": "#6250d6", "series-8": "#e34948",
	"status-good": "#0ca30c", "status-warning": "#fab219", "status-serious": "#ec835a", "status-critical": "#d03b3b",
}

var referenceDark = map[string]string{
	"bg": "#111519", "fg": "#e2e7ec", "muted": "#95a0ac", "accent": "#8db6df", "rule": "#29313a",
	"code-bg": "#1a2027", "ok": "#71c08f", "warn": "#eb9a55",
	"chart-surface": "var(--bg)", "chart-grid": "#2c2c2a", "chart-axis": "#383835",
	"series-1": "#3987e5", "series-2": "#d95926", "series-3": "#199e70", "series-4": "#c98500",
	"series-5": "#d55181", "series-6": "#008300", "series-7": "#9085e9", "series-8": "#e66767",
	"status-good": "#0ca30c", "status-warning": "#fab219", "status-serious": "#ec835a", "status-critical": "#d03b3b",
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/stylecheck/`
Expected: FAIL, with build errors for the undefined `Check`, `parseColour`, `Tokens` and so on.

- [ ] **Step 3: Write the colour parser**

Create `internal/stylecheck/colour.go`:

```go
package stylecheck

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// rgb is an opaque colour in sRGB, each channel 0 to 1.
type rgb [3]float64

// parsed is what reading one value gives: a colour the rules can measure,
// a colour they cannot, or something that is no colour at all.
type parsed int

const (
	measurable   parsed = iota
	translucent         // a colour with alpha below 1
	unmeasurable        // a colour form the check does not evaluate
	notColour
)

var funcRe = regexp.MustCompile(`^([a-z-]+)\((.*)\)$`)

// unmeasurableFuncs are colour functions the check recognises but does
// not evaluate.
var unmeasurableFuncs = map[string]bool{"color-mix": true, "light-dark": true, "color": true, "var": true}

// parseColour reads a CSS colour value.
func parseColour(v string) (rgb, parsed) {
	s := strings.ToLower(strings.TrimSpace(v))
	switch {
	case s == "currentcolor":
		return rgb{}, unmeasurable
	case s == "transparent":
		return rgb{}, translucent
	case strings.HasPrefix(s, "#"):
		return parseHex(s[1:])
	}
	if hex, ok := namedColours[s]; ok {
		return parseHex(hex)
	}
	m := funcRe.FindStringSubmatch(s)
	if m == nil {
		return rgb{}, notColour
	}
	name, body := m[1], m[2]
	if unmeasurableFuncs[name] {
		return rgb{}, unmeasurable
	}
	switch name {
	case "rgb", "rgba", "hsl", "hsla", "oklch", "oklab":
	default:
		return rgb{}, notColour
	}
	if strings.HasPrefix(strings.TrimSpace(body), "from ") || strings.Contains(body, "var(") {
		return rgb{}, unmeasurable
	}
	args, alpha, ok := splitArgs(body)
	if !ok || len(args) != 3 {
		return rgb{}, notColour
	}
	if alpha != "" {
		a, ok := number(alpha, 1)
		if !ok {
			return rgb{}, notColour
		}
		if a < 1 {
			return rgb{}, translucent
		}
	}
	var c rgb
	switch name {
	case "rgb", "rgba":
		for i, a := range args {
			n, ok := number(a, 255)
			if !ok {
				return rgb{}, notColour
			}
			c[i] = clamp01(n / 255)
		}
	case "hsl", "hsla":
		h, ok1 := angle(args[0])
		sat, ok2 := number(args[1], 1)
		l, ok3 := number(args[2], 1)
		if !ok1 || !ok2 || !ok3 || !strings.HasSuffix(args[1], "%") || !strings.HasSuffix(args[2], "%") {
			return rgb{}, notColour
		}
		c = hslToRGB(h, clamp01(sat), clamp01(l))
	case "oklch":
		l, ok1 := number(args[0], 1)
		ch, ok2 := number(args[1], 0.4)
		h, ok3 := angle(args[2])
		if !ok1 || !ok2 || !ok3 {
			return rgb{}, notColour
		}
		rad := h * math.Pi / 180
		c = oklabToSRGB(l, ch*math.Cos(rad), ch*math.Sin(rad))
	case "oklab":
		l, ok1 := number(args[0], 1)
		a, ok2 := number(args[1], 0.4)
		b, ok3 := number(args[2], 0.4)
		if !ok1 || !ok2 || !ok3 {
			return rgb{}, notColour
		}
		c = oklabToSRGB(l, a, b)
	}
	return c, measurable
}

func parseHex(h string) (rgb, parsed) {
	switch len(h) {
	case 3, 4:
		var b strings.Builder
		for _, r := range h {
			b.WriteRune(r)
			b.WriteRune(r)
		}
		h = b.String()
	case 6, 8:
	default:
		return rgb{}, notColour
	}
	n, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return rgb{}, notColour
	}
	if len(h) == 8 {
		if n&0xff != 0xff {
			return rgb{}, translucent
		}
		n >>= 8
	}
	return rgb{float64(n>>16&0xff) / 255, float64(n>>8&0xff) / 255, float64(n&0xff) / 255}, measurable
}

// splitArgs splits a function body in either syntax, "a, b, c[, alpha]" or
// "a b c[ / alpha]", into its three channels and its alpha, if any.
func splitArgs(body string) (args []string, alpha string, ok bool) {
	if strings.Contains(body, ",") {
		for _, p := range strings.Split(body, ",") {
			args = append(args, strings.TrimSpace(p))
		}
		if len(args) == 4 {
			return args[:3], args[3], true
		}
		return args, "", len(args) == 3
	}
	main, a, slash := strings.Cut(body, "/")
	if slash {
		alpha = strings.TrimSpace(a)
	}
	return strings.Fields(main), alpha, true
}

// number reads a plain number, or a percentage of full.
func number(s string, full float64) (float64, bool) {
	s = strings.TrimSpace(s)
	if p, ok := strings.CutSuffix(s, "%"); ok {
		n, err := strconv.ParseFloat(p, 64)
		return n / 100 * full, err == nil
	}
	n, err := strconv.ParseFloat(s, 64)
	return n, err == nil
}

// angle reads a hue in degrees, bare or with a deg unit.
func angle(s string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "deg"), 64)
	return n, err == nil
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func hslToRGB(h, s, l float64) rgb {
	f := func(n float64) float64 {
		k := math.Mod(n+h/30, 12)
		a := s * math.Min(l, 1-l)
		return l - a*math.Max(-1, math.Min(k-3, math.Min(9-k, 1)))
	}
	return rgb{f(0), f(8), f(4)}
}

// namedColours maps each CSS named colour to its hex value.
var namedColours = func() map[string]string {
	m := map[string]string{}
	f := strings.Fields(namedColourList)
	for i := 0; i+1 < len(f); i += 2 {
		m[f[i]] = f[i+1]
	}
	return m
}()

const namedColourList = `aliceblue f0f8ff antiquewhite faebd7 aqua 00ffff aquamarine 7fffd4
azure f0ffff beige f5f5dc bisque ffe4c4 black 000000 blanchedalmond ffebcd blue 0000ff
blueviolet 8a2be2 brown a52a2a burlywood deb887 cadetblue 5f9ea0 chartreuse 7fff00
chocolate d2691e coral ff7f50 cornflowerblue 6495ed cornsilk fff8dc crimson dc143c
cyan 00ffff darkblue 00008b darkcyan 008b8b darkgoldenrod b8860b darkgray a9a9a9
darkgreen 006400 darkgrey a9a9a9 darkkhaki bdb76b darkmagenta 8b008b darkolivegreen 556b2f
darkorange ff8c00 darkorchid 9932cc darkred 8b0000 darksalmon e9967a darkseagreen 8fbc8f
darkslateblue 483d8b darkslategray 2f4f4f darkslategrey 2f4f4f darkturquoise 00ced1
darkviolet 9400d3 deeppink ff1493 deepskyblue 00bfff dimgray 696969 dimgrey 696969
dodgerblue 1e90ff firebrick b22222 floralwhite fffaf0 forestgreen 228b22 fuchsia ff00ff
gainsboro dcdcdc ghostwhite f8f8ff gold ffd700 goldenrod daa520 gray 808080 green 008000
greenyellow adff2f grey 808080 honeydew f0fff0 hotpink ff69b4 indianred cd5c5c indigo 4b0082
ivory fffff0 khaki f0e68c lavender e6e6fa lavenderblush fff0f5 lawngreen 7cfc00
lemonchiffon fffacd lightblue add8e6 lightcoral f08080 lightcyan e0ffff
lightgoldenrodyellow fafad2 lightgray d3d3d3 lightgreen 90ee90 lightgrey d3d3d3
lightpink ffb6c1 lightsalmon ffa07a lightseagreen 20b2aa lightskyblue 87cefa
lightslategray 778899 lightslategrey 778899 lightsteelblue b0c4de lightyellow ffffe0
lime 00ff00 limegreen 32cd32 linen faf0e6 magenta ff00ff maroon 800000
mediumaquamarine 66cdaa mediumblue 0000cd mediumorchid ba55d3 mediumpurple 9370db
mediumseagreen 3cb371 mediumslateblue 7b68ee mediumspringgreen 00fa9a
mediumturquoise 48d1cc mediumvioletred c71585 midnightblue 191970 mintcream f5fffa
mistyrose ffe4e1 moccasin ffe4b5 navajowhite ffdead navy 000080 oldlace fdf5e6 olive 808000
olivedrab 6b8e23 orange ffa500 orangered ff4500 orchid da70d6 palegoldenrod eee8aa
palegreen 98fb98 paleturquoise afeeee palevioletred db7093 papayawhip ffefd5
peachpuff ffdab9 peru cd853f pink ffc0cb plum dda0dd powderblue b0e0e6 purple 800080
rebeccapurple 663399 red ff0000 rosybrown bc8f8f royalblue 4169e1 saddlebrown 8b4513
salmon fa8072 sandybrown f4a460 seagreen 2e8b57 seashell fff5ee sienna a0522d silver c0c0c0
skyblue 87ceeb slateblue 6a5acd slategray 708090 slategrey 708090 snow fffafa
springgreen 00ff7f steelblue 4682b4 tan d2b48c teal 008080 thistle d8bfd8 tomato ff6347
turquoise 40e0d0 violet ee82ee wheat f5deb3 white ffffff whitesmoke f5f5f5 yellow ffff00
yellowgreen 9acd32`
```

- [ ] **Step 4: Write the colour maths**

Create `internal/stylecheck/maths.go`:

```go
package stylecheck

import "math"

// The colour maths of the dataviz skill's validate_palette.js, ported
// line for line: WCAG contrast, OKLab and OKLCH, and the Machado,
// Oliveira and Fernandes (2009) colour-vision simulation at severity 1.0.
// Its thresholds are calibrated to exactly these formulas, so a change
// here is a change to every rule.

func toLinear(s float64) float64 {
	if s <= 0.04045 {
		return s / 12.92
	}
	return math.Pow((s+0.055)/1.055, 2.4)
}

func toSRGB(l float64) float64 {
	if l <= 0.0031308 {
		return 12.92 * l
	}
	return 1.055*math.Pow(l, 1/2.4) - 0.055
}

func (c rgb) linear() [3]float64 {
	return [3]float64{toLinear(c[0]), toLinear(c[1]), toLinear(c[2])}
}

func luminance(c rgb) float64 {
	l := c.linear()
	return 0.2126*l[0] + 0.7152*l[1] + 0.0722*l[2]
}

// contrast is the WCAG contrast ratio of two colours, 1 to 21.
func contrast(a, b rgb) float64 {
	x, y := luminance(a), luminance(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

func oklabFromLinear(l [3]float64) [3]float64 {
	r, g, b := l[0], l[1], l[2]
	lc := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	mc := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	sc := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return [3]float64{
		0.2104542553*lc + 0.7936177850*mc - 0.0040720468*sc,
		1.9779984951*lc - 2.4285922050*mc + 0.4505937099*sc,
		0.0259040371*lc + 0.7827717662*mc - 0.8086757660*sc,
	}
}

// oklabToSRGB converts an OKLab colour to sRGB, clipped to the gamut.
func oklabToSRGB(L, a, b float64) rgb {
	l := math.Pow(L+0.3963377774*a+0.2158037573*b, 3)
	m := math.Pow(L-0.1055613458*a-0.0638541728*b, 3)
	s := math.Pow(L-0.0894841775*a-1.2914855480*b, 3)
	lin := [3]float64{
		4.0767416621*l - 3.3077115913*m + 0.2309699292*s,
		-1.2684380046*l + 2.6097574011*m - 0.3413193965*s,
		-0.0041960863*l - 0.7034186147*m + 1.7076147010*s,
	}
	return rgb{clamp01(toSRGB(clamp01(lin[0]))), clamp01(toSRGB(clamp01(lin[1]))), clamp01(toSRGB(clamp01(lin[2])))}
}

// lightnessChroma is a colour's OKLCH L and C.
func lightnessChroma(c rgb) (float64, float64) {
	lab := oklabFromLinear(c.linear())
	return lab[0], math.Hypot(lab[1], lab[2])
}

type vision int

const (
	normal vision = iota
	protan
	deutan
)

var machado = map[vision][3][3]float64{
	protan: {{0.152286, 1.052583, -0.204868}, {0.114503, 0.786281, 0.099216}, {-0.003882, -0.048116, 1.051998}},
	deutan: {{0.367322, 0.860646, -0.227968}, {0.280085, 0.672501, 0.047413}, {-0.011820, 0.042940, 0.968881}},
}

func simulate(c rgb, v vision) [3]float64 {
	l := c.linear()
	if v == normal {
		return l
	}
	m := machado[v]
	var out [3]float64
	for i := range out {
		out[i] = clamp01(m[i][0]*l[0] + m[i][1]*l[1] + m[i][2]*l[2])
	}
	return out
}

// deltaE is the distance between two colours in OKLab, ×100, as a reader
// with the given vision sees them.
func deltaE(a, b rgb, v vision) float64 {
	x, y := oklabFromLinear(simulate(a, v)), oklabFromLinear(simulate(b, v))
	return 100 * math.Sqrt((x[0]-y[0])*(x[0]-y[0])+(x[1]-y[1])*(x[1]-y[1])+(x[2]-y[2])*(x[2]-y[2]))
}

// colourBlind is the smaller of the protan and deutan distances.
func colourBlind(a, b rgb) float64 {
	return math.Min(deltaE(a, b, protan), deltaE(a, b, deutan))
}
```

- [ ] **Step 5: Write the theme block reader**

Create `internal/stylecheck/views.go`:

```go
package stylecheck

import (
	"regexp"
	"strings"
)

// block kinds, the four theme blocks the guide has a style write.
const (
	rootBlock  = iota // :root
	mediaBlock        // :root:not([data-theme="light"]) in the dark media query
	lightBlock        // :root[data-theme="light"]
	darkBlock         // :root[data-theme="dark"]
)

// view is one way a reader can see the page: their system's theme and
// what, if anything, they picked.
type view struct {
	name   string
	blocks []int
}

var (
	viewL1 = view{"light system, nothing picked", []int{rootBlock}}
	viewL2 = view{"light picked", []int{rootBlock, lightBlock}}
	viewD1 = view{"dark system, nothing picked", []int{rootBlock, mediaBlock}}
	viewD2 = view{"dark picked on a light system", []int{rootBlock, darkBlock}}
	viewD3 = view{"dark picked on a dark system", []int{rootBlock, mediaBlock, darkBlock}}
)

var (
	commentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	blockRe   = regexp.MustCompile(`:root(:not\(\[data-theme=["']?light["']?\]\)|\[data-theme=["']?(light|dark)["']?\])?\s*\{([^}]*)\}`)
	declRe    = regexp.MustCompile(`--([A-Za-z0-9_-]+)\s*:\s*([^;}]+)(?:;|$)`)
	varRe     = regexp.MustCompile(`^var\(\s*--([A-Za-z0-9_-]+)\s*\)$`)
)

type decl struct {
	kind  int
	name  string
	value string
}

// readBlocks lists every custom property the four theme blocks declare,
// in source order.
func readBlocks(css string) []decl {
	css = commentRe.ReplaceAllString(css, "")
	var out []decl
	for _, m := range blockRe.FindAllStringSubmatch(css, -1) {
		kind := rootBlock
		switch {
		case m[2] == "light":
			kind = lightBlock
		case m[2] == "dark":
			kind = darkBlock
		case m[1] != "":
			kind = mediaBlock
		}
		for _, d := range declRe.FindAllStringSubmatch(m[3], -1) {
			out = append(out, decl{kind, d[1], strings.TrimSpace(d[2])})
		}
	}
	return out
}

// values is what each custom property resolves to in v. A bare :root
// loses to the other three selectors, which tie, and among ties the later
// declaration wins, as the cascade decides. var() references resolve in
// the same view.
func (v view) values(decls []decl) map[string]string {
	applies := map[int]bool{}
	for _, b := range v.blocks {
		applies[b] = true
	}
	val := map[string]string{}
	spec := map[string]int{}
	for _, d := range decls {
		if !applies[d.kind] {
			continue
		}
		s := 2
		if d.kind == rootBlock {
			s = 1
		}
		if s >= spec[d.name] {
			val[d.name], spec[d.name] = d.value, s
		}
	}
	out := make(map[string]string, len(val))
	for k, x := range val {
		for range 8 { // bounded, so a reference cycle cannot loop
			m := varRe.FindStringSubmatch(x)
			if m == nil {
				break
			}
			next, ok := val[m[1]]
			if !ok {
				break
			}
			x = next
		}
		out[k] = x
	}
	return out
}
```

- [ ] **Step 6: Write the rules**

Create `internal/stylecheck/check.go`:

```go
// Package stylecheck measures a stylesheet's theme colours against the
// rules md2html holds its styles to: WCAG contrast for text, and the
// dataviz skill's checks for chart colours.
package stylecheck

import (
	"fmt"
	"math"
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
var Tokens = []string{"bg", "fg", "muted", "accent", "rule", "code-bg", "ok", "warn",
	"chart-surface", "chart-grid", "chart-axis",
	"series-1", "series-2", "series-3", "series-4", "series-5", "series-6", "series-7", "series-8",
	"status-good", "status-warning", "status-serious", "status-critical"}

var (
	textTokens   = []string{"fg", "muted", "accent", "ok", "warn"}
	textGrounds  = []string{"bg", "code-bg"}
	seriesTokens = Tokens[11:19]
	statusTokens = Tokens[19:23]
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
	for _, v := range []*view{&viewL1, &viewL2, &viewD1, &viewD2, &viewD3} {
		views[v] = v.values(decls)
	}

	var out []Finding
	add := func(sev Severity, theme string, msg string, tokens ...string) {
		out = append(out, Finding{sev, theme, tokens, msg})
	}

	// Where each token is set: bare :root and the light block set it for
	// light, the media and dark blocks for dark.
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
			{"light", &viewL2, []*view{&viewL1}},
			{"dark", &viewD2, []*view{&viewD1, &viewD3}},
		} {
			value, ok := views[p.main][t]
			if !ok {
				continue
			}
			// Views that show another value, grouped by the value they show.
			var shown []string
			where := map[string][]string{}
			for _, o := range p.others {
				ov, ok := views[o][t]
				if !ok {
					ov = "not set"
				}
				if ok && same(ov, value) {
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
```

- [ ] **Step 7: Write the output helpers**

Create `internal/stylecheck/format.go`:

```go
package stylecheck

import (
	"fmt"
	"strings"
)

// Counts says how many findings there are of each severity, worst first,
// leaving out a severity with none: "1 error, 3 warnings, 2 notes".
func Counts(fs []Finding) string {
	var n [3]int
	for _, f := range fs {
		n[f.Severity]++
	}
	var parts []string
	for s, k := range n {
		if k == 0 {
			continue
		}
		word := Severity(s).String()
		if k > 1 {
			word += "s"
		}
		parts = append(parts, fmt.Sprintf("%d %s", k, word))
	}
	return strings.Join(parts, ", ")
}

// HasError reports whether any finding is an Error.
func HasError(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == Error {
			return true
		}
	}
	return false
}

// Report is the full report on one style, as --check-style prints it: a
// heading with the counts, then every finding on its own line.
func Report(name string, fs []Finding) string {
	if len(fs) == 0 {
		return name + ": no problems\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n", name, Counts(fs))
	for _, f := range fs {
		fmt.Fprintf(&b, "  %-8s %s\n", f.Severity, f.Message)
	}
	return b.String()
}

// Short is the one-line form a build prints: the findings themselves when
// there are one or two, otherwise their counts. It is "" for none.
func Short(fs []Finding) string {
	if len(fs) > 2 {
		return Counts(fs)
	}
	msgs := make([]string, len(fs))
	for i, f := range fs {
		msgs[i] = f.Message
	}
	return strings.Join(msgs, "; ")
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `gofmt -l internal/stylecheck; go vet ./internal/stylecheck/ && go test ./internal/stylecheck/ -v`
Expected: no files listed by gofmt; every test PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/stylecheck
git commit -m "feat: a style check for theme colours"
```

### Task 2: The built-in styles pass

**Files:**
- Modify: `plugins/default/default.css` (5 values, each in two blocks)
- Modify: `docs/authoring.md:1117-1125` (tier 3) and `docs/authoring.md:1196-1221` ("Keeping it consistent")
- Test: `stylecheck_test.go` (new, repo root, package `md2html`)

**Interfaces:**
- Consumes: `stylecheck.Check`, `NewPluginCache`, `(*PluginCache).Stylesheet`, `BuiltinPlugins`, `(*Plugin).StyleNames`.

- [ ] **Step 1: Write the failing test**

Create `stylecheck_test.go`:

```go
package md2html

import (
	"testing"

	"github.com/AdamF-G/md2html/internal/stylecheck"
)

// Every built-in style passes the style check with no findings at all,
// so a commit that breaks one fails here, and every finding on a user's
// style is about something that style changed.
func TestBuiltinStylesPassTheStyleCheck(t *testing.T) {
	cache := NewPluginCache()
	n := 0
	for _, p := range BuiltinPlugins() {
		for _, s := range p.StyleNames() {
			name := p.Name + "/" + s
			css, err := cache.Stylesheet([]string{name}, nil, nil)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			n++
			if fs := stylecheck.Check(css); len(fs) != 0 {
				t.Errorf("%s", stylecheck.Report(name, fs))
			}
		}
	}
	if n < 2 {
		t.Errorf("checked %d built-in styles, want default and claude-artifact at least", n)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -run TestBuiltinStylesPassTheStyleCheck .`
Expected: FAIL with exactly these five findings for `default/default`. `claude-artifact/claude-artifact` sets every checked colour itself and already passes:

```
default/default: 5 warnings
  warning  light --warn on --code-bg is 4.34:1, under 4.5:1 (WCAG AA)
  warning  light --status-warning on --chart-surface is 1.57:1, under 1.6:1
  warning  dark --status-warning and --status-serious are ΔE 12.7 apart, under 13
  warning  light --status-serious and --status-critical are ΔE 12.9 apart, under 13
  warning  dark --status-serious and --status-critical are ΔE 11.5 apart, under 13
```

- [ ] **Step 3: Move the five colours**

Each was moved the least that passes, keeping its hue (found by search; ΔE moved in brackets). Each old value appears exactly twice in `plugins/default/default.css` (the light values in `:root` and `:root[data-theme="light"]`, the dark in the media block and `:root[data-theme="dark"]`) and nowhere else that matters:

| Token | Theme | Old | New | Now |
|---|---|---|---|---|
| `--warn` | light | `#b4530a` | `#af4f00` | 4.61:1 on `--code-bg` |
| `--status-warning` | light | `#f5c400` | `#f1c10b` | 1.63:1 on the surface |
| `--status-critical` | light | `#dd2a3c` | `#dd2942` (0.7) | status apart ≥ 13.36 |
| `--status-warning` | dark | `#f7cc3c` | `#f7ce3e` (0.5) | status apart ≥ 13.23 |
| `--status-critical` | dark | `#ff6b6b` | `#f96570` (2.0) | |

```bash
sed -i '' -e 's/#b4530a/#af4f00/g' -e 's/#f5c400/#f1c10b/g' -e 's/#dd2a3c/#dd2942/g' \
  -e 's/#f7cc3c/#f7ce3e/g' -e 's/#ff6b6b/#f96570/g' plugins/default/default.css
grep -c -e '#af4f00' -e '#f1c10b' -e '#dd2942' -e '#f7ce3e' -e '#f96570' plugins/default/default.css
```

Expected count: 10. (On Linux use `sed -i` without `''`.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run TestBuiltinStylesPassTheStyleCheck .`
Expected: PASS.

- [ ] **Step 5: Bring the guide in line**

The guide's style section still describes the default's light series 5 and 8 as a shortfall and quotes the old `--warn` figures. In `docs/authoring.md`, under "#### What you decide, and what you inherit", replace tier 3's text from "The default's series were chosen" to "change them only if your series would be confused with them." with:

```markdown
   The default's series were chosen for its near-white and near-black
   backgrounds, and suit a palette of similar lightness, so this tier is
   for a brand palette, not a requirement of tier 2. The four status
   colours carry meaning (good, warning, serious, critical); change them
   only if your series would be confused with them.
```

Under "#### Keeping it consistent", replace the **Contrast**, **Status apart from series** and **Series apart from each other** bullets with:

```markdown
- **Contrast.** `--fg`, `--muted`, `--accent`, `--ok` and `--warn` at
  least 4.5:1 on `--bg` and on `--code-bg`, with `--fg` well above it.
  Series need 3:1 on `--chart-surface` in dark and 1.9:1 in light, where
  the table under each chart carries the values for a reader who cannot
  pick a pale series out.
- **Status apart from series.** The status colours never share a chart
  with series colours and come with a label, so they may sit near a
  series, but not on one: keep each at least ΔE 4.5 from every series,
  and the four at least 13 from each other.
- **Series apart from each other.** Neighbouring series need different
  lightness as well as hue, in both themes, so that colour-blind readers
  can tell them apart too.
```

and replace the paragraph "md2html does not check a style's contrast or chart colours yet, so these checks are yours." with:

```markdown
`md2html --check-style` measures every one of these; see
[Checking a style](#checking-a-style).
```

Task 6 rewrites "Checking a style" itself; the link target already exists.

- [ ] **Step 6: Run the whole suite**

Run: `go vet ./... && go test ./...`
Expected: PASS. If a test pins one of the five old colours, update it to the new value; if a guide test pins a sentence replaced in Step 5, update its expected text to the new sentence.

- [ ] **Step 7: Commit**

```bash
git add plugins/default/default.css docs/authoring.md stylecheck_test.go
git commit -m "fix: the default style passes the style check"
```

### Task 3: `Options.StyleChosen`

**Files:**
- Modify: `md2html.go` (the `Options` struct, after `PluginCache`)
- Modify: `stylesheet.go:41-89` (`pageStyle`, `style`) and `stylesheet.go:197-245` (`choosePageStyle`)
- Test: `stylesheet_test.go` (append)

**Interfaces:**
- Produces: `Options.StyleChosen func(names []string, css string)`, called by `Convert` once per page whose style comes from `Styles` or its `style:` front matter, with the qualified names (`plugin/style`) and the assembled stylesheet. It is not called for the default chosen by default, or for `CSS`.

- [ ] **Step 1: Write the failing test**

Append to `stylesheet_test.go`:

```go
// StyleChosen hears the style list each page is drawn in, by qualified
// name, and never the default chosen by default or a run's CSS.
func TestStyleChosenNamesThePageStyle(t *testing.T) {
	type call struct {
		names []string
		css   string
	}
	for _, c := range []struct {
		name string
		opt  Options
		src  string
		want []string // nil: no call
	}{
		{"run style", Options{Styles: []string{"claude-artifact"}}, "# A\n", []string{"claude-artifact/claude-artifact"}},
		{"front matter", Options{}, "---\nstyle: claude-artifact\n---\n# A\n", []string{"claude-artifact/claude-artifact"}},
		{"front matter falls back", Options{Styles: []string{"claude-artifact"}}, "---\nstyle: nosuch\n---\n# A\n", []string{"claude-artifact/claude-artifact"}},
		{"default", Options{}, "# A\n", nil},
		{"css", Options{CSS: "body{}"}, "# A\n", nil},
	} {
		var got []call
		c.opt.StyleChosen = func(names []string, css string) { got = append(got, call{names, css}) }
		if _, err := Convert([]byte(c.src), c.opt); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		switch {
		case c.want == nil && len(got) != 0:
			t.Errorf("%s: called with %v", c.name, got[0].names)
		case c.want != nil && len(got) != 1:
			t.Errorf("%s: called %d times, want once", c.name, len(got))
		case c.want != nil:
			if strings.Join(got[0].names, ",") != strings.Join(c.want, ",") {
				t.Errorf("%s: names %v, want %v", c.name, got[0].names, c.want)
			}
			if !strings.HasPrefix(got[0].css, defaultCSS) || !strings.Contains(got[0].css, "IBM Plex Sans") {
				t.Errorf("%s: css is not claude-artifact stacked on the default", c.name)
			}
		}
	}
}
```

If `stylesheet_test.go` does not already import `strings`, add it.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -run TestStyleChosenNamesThePageStyle .`
Expected: FAIL to build: `unknown field StyleChosen in struct literal of type Options`.

- [ ] **Step 3: Add the field**

In `md2html.go`, in `Options`, directly after the `PluginCache *PluginCache` field and its comment, add:

```go
	// StyleChosen, if set, is called once per Convert after the page's
	// style is chosen from Styles or its own "style:" front matter, with
	// the styles' qualified names (plugin/style) and the stylesheet they
	// assemble. It is not called when the page gets the default stylesheet
	// by default, or CSS. The CLI checks each list's colours here. A caller
	// converting pages in parallel shares it, so it must be safe for
	// concurrent use.
	StyleChosen func(names []string, css string)
```

- [ ] **Step 4: Carry the names on pageStyle**

In `stylesheet.go`, give `pageStyle` a third field and switch every non-empty `pageStyle{…}` literal to keyed fields (positional ones no longer compile):

```go
// pageStyle is what a page's style list resolves to: its stylesheet, the
// settings its charts are drawn with, and the qualified names of the
// styles, nil for the default chosen by default or a run's CSS.
type pageStyle struct {
	css    string
	charts chartSettings
	names  []string
}
```

In `style`, the no-names return becomes:

```go
		return pageStyle{css: defaultCSS, charts: c.chartStyle(nil)}, nil
```

and the final return becomes:

```go
	qualified := make([]string, len(picks))
	for i, s := range picks {
		qualified[i] = s.qualified()
	}
	return pageStyle{css: e.css, charts: c.chartStyle(picks), names: qualified}, e.err
```

In `choosePageStyle`'s `run`, the CSS return becomes:

```go
			return pageStyle{css: opt.CSS, charts: cache.chartStyle(nil)}, nil
```

Then run `grep -n 'pageStyle{[^}]' *.go` and convert any other positional literal it shows the same way.

- [ ] **Step 5: Call the hook**

In `choosePageStyle`, after the stylesheet size warning and before `return st, nil`, add:

```go
	if opt.StyleChosen != nil && st.names != nil {
		opt.StyleChosen(st.names, st.css)
	}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go vet ./... && go test ./...`
Expected: PASS, including `TestStyleChosenNamesThePageStyle`.

- [ ] **Step 7: Commit**

```bash
git add md2html.go stylesheet.go stylesheet_test.go
git commit -m "feat: Options.StyleChosen hears each page's style list"
```

### Task 4: `--check-style`

**Files:**
- Modify: `cmd/md2html/main.go` (flag list near `listStyles`; the dispatch after `if *listStyles {…}`; the usage text's "Styling the page" paragraph; a new `checkStyles` function after `listAvailableStyles`)
- Test: `cmd/md2html/stylecheck_test.go` (new)

**Interfaces:**
- Consumes: `stylecheck.Check`, `stylecheck.Report`, `stylecheck.HasError`; `checkPlugins(cache, dirs, styles, stderr) ([]*md2html.Plugin, error)` (existing); `(*md2html.PluginCache).LoadAll(dirs []string, baseDir string, warn func(string)) ([]*md2html.Plugin, error)`; `md2html.BuiltinPlugins()`.
- Produces: `func checkStyles(dirs, styles []string, stdout, stderr io.Writer) int`, and the `stylePlugin(t)` test helper that Task 5 reuses.

- [ ] **Step 1: Write the failing tests**

Create `cmd/md2html/stylecheck_test.go`:

```go
package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

// stylePlugin is a plugin, "acme", with one style that passes, one with a
// warning, one with two errors and one with four.
func stylePlugin(t *testing.T) string {
	t.Helper()
	root := tree(t, map[string]string{
		"acme/md2html-plugin.json": `{"format": 1, "name": "acme", "styles": {
			"plain": {"css": ["plain.css"]}, "quiet": {"css": ["quiet.css"]},
			"bad": {"css": ["bad.css"]}, "worse": {"css": ["worse.css"]}}}`,
		"acme/plain.css": ":root{--accent:#2c5a85}\n:root[data-theme=\"light\"]{--accent:#2c5a85}\n",
		"acme/quiet.css": ":root{--muted:#766f67}\n:root[data-theme=\"light\"]{--muted:#766f67}\n",
		"acme/bad.css":   ":root{--muted:#aaaaaa}\n:root[data-theme=\"light\"]{--muted:#aaaaaa}\n",
		"acme/worse.css": ":root{--muted:#aaaaaa}\n:root[data-theme=\"light\"]{--muted:#aaaaaa}\n" +
			"@media (prefers-color-scheme: dark){:root:not([data-theme=\"light\"]){--muted:#555555}}\n" +
			":root[data-theme=\"dark\"]{--muted:#555555}\n",
	})
	return filepath.Join(root, "acme")
}

func TestCheckStyleBuiltinsPass(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--check-style"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s%s", code, out.String(), errb.String())
	}
	for _, want := range []string{"default/default: no problems\n", "claude-artifact/claude-artifact: no problems\n"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Errorf("stdout lacks %q:\n%s", want, out.String())
		}
	}
}

func TestCheckStyleReportsEachPluginStyle(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"--check-style", "--plugin", stylePlugin(t)}, &out, &errb)
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	want := "acme/bad: 2 errors\n" +
		"  error    light --muted on --bg is 2.22:1, under 3:1\n" +
		"  error    light --muted on --code-bg is 2.01:1, under 3:1\n" +
		"acme/plain: no problems\n" +
		"acme/quiet: 1 warning\n" +
		"  warning  light --muted on --code-bg is 4.28:1, under 4.5:1 (WCAG AA)\n" +
		"acme/worse: 4 errors\n" +
		"  error    light --muted on --bg is 2.22:1, under 3:1\n" +
		"  error    light --muted on --code-bg is 2.01:1, under 3:1\n" +
		"  error    dark --muted on --bg is 2.44:1, under 3:1\n" +
		"  error    dark --muted on --code-bg is 2.19:1, under 3:1\n"
	if out.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", out.String(), want)
	}
}

// Warnings alone exit 0; a --style list is checked as one stack.
func TestCheckStyleStackAndExitCodes(t *testing.T) {
	dir := stylePlugin(t)
	var out, errb bytes.Buffer
	if code := run([]string{"--check-style", "--plugin", dir, "--style", "acme/plain,acme/quiet"}, &out, &errb); code != 0 {
		t.Errorf("exit %d, want 0: %s", code, errb.String())
	}
	if want := "acme/plain,acme/quiet: 1 warning\n  warning  light --muted on --code-bg is 4.28:1, under 4.5:1 (WCAG AA)\n"; out.String() != want {
		t.Errorf("stdout %q, want %q", out.String(), want)
	}
	out.Reset()
	if code := run([]string{"--check-style", "--plugin", "/no/such/dir"}, &out, &errb); code != 2 {
		t.Errorf("bad plugin: exit %d, want 2", code)
	}
	if out.Len() != 0 {
		t.Errorf("bad plugin: stdout %q, want nothing", out.String())
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./cmd/md2html/ -run TestCheckStyle`
Expected: FAIL: the flag is undefined, so `run` prints usage and exits 2.

- [ ] **Step 3: Add the flag and its dispatch**

In `cmd/md2html/main.go`, after the `listStyles` flag, add:

```go
		checkStyle = fs.Bool("check-style", false, "check the colours of the styles from any --plugin, or the --style list, or every built-in style, print a report, and exit (1 if any has an error)")
```

After the `if *listStyles { … }` block, add:

```go
	// Like --list-styles, the whole invocation, and ahead of the entry
	// check: it checks styles, not documents.
	if *checkStyle {
		return checkStyles(plugins, styles, stdout, stderr)
	}
```

In the usage text, after the sentence ending "how to write a plugin that adds a style.", add a sentence so the paragraph reads:

```
Styling the page: md2html --list-styles lists the styles available, and
--style or a page's style: front matter picks them. md2html --guide styles
explains both, and how to write a plugin that adds a style.
md2html --check-style checks a style's colours.
```

- [ ] **Step 4: Write checkStyles**

Add the import `"github.com/AdamF-G/md2html/internal/stylecheck"` to `main.go`, and after `listAvailableStyles` add:

```go
// checkStyles is --check-style. It reports on each style list a run could
// draw pages in, measured by the style check: the --style list if there is
// one, stacked as a page stacks it; otherwise each style of each --plugin,
// alone on the default; otherwise every built-in style. Plugins load as
// they do for a run, so a broken one exits 2 with the reason. Otherwise it
// exits 1 if any list has an error, and 0 if none does, however many
// warnings and notes there are.
func checkStyles(dirs, styles []string, stdout, stderr io.Writer) int {
	cache := md2html.NewPluginCache()
	if _, err := checkPlugins(cache, dirs, styles, stderr); err != nil {
		fmt.Fprintf(stderr, "md2html: %v\n", err)
		return 2
	}
	loaded, err := cache.LoadAll(dirs, "", nil)
	if err != nil {
		fmt.Fprintf(stderr, "md2html: %v\n", err)
		return 2
	}
	var lists [][]string
	switch {
	case len(styles) > 0:
		lists = [][]string{styles}
	case len(dirs) > 0:
		for _, p := range loaded {
			for _, n := range p.StyleNames() {
				lists = append(lists, []string{p.Name + "/" + n})
			}
		}
	default:
		for _, p := range md2html.BuiltinPlugins() {
			for _, n := range p.StyleNames() {
				lists = append(lists, []string{p.Name + "/" + n})
			}
		}
	}
	code := 0
	for _, l := range lists {
		css, err := cache.Stylesheet(l, loaded, nil)
		if err != nil {
			fmt.Fprintf(stderr, "md2html: %v\n", err)
			return 2
		}
		fs := stylecheck.Check(css)
		fmt.Fprint(stdout, stylecheck.Report(strings.Join(l, ","), fs))
		if stylecheck.HasError(fs) {
			code = 1
		}
	}
	return code
}
```

(`strings` and `io` are already imported by `main.go`; if `go vet` says otherwise, add them.)

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go vet ./... && go test ./cmd/md2html/ -run 'TestCheckStyle|TestRunListStyles' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/md2html/main.go cmd/md2html/stylecheck_test.go
git commit -m "feat: --check-style reports on a style's colours"
```

### Task 5: The build line

**Files:**
- Modify: `cmd/md2html/main.go` (`run`: before the conversion loop, inside it where `opts` is built, and after the per-document result loop; plus a new `styleChecks` type at the end of the file)
- Test: `cmd/md2html/stylecheck_test.go` (append)

**Interfaces:**
- Consumes: `Options.StyleChosen` (Task 3), `stylecheck.Check`, `Short`, `HasError`, and `stylePlugin(t)` (Task 4).

- [ ] **Step 1: Write the failing tests**

Append to `cmd/md2html/stylecheck_test.go` (add `"os"` and `"strings"` to its imports):

```go
// buildWith builds doc.md from files with args, and returns stderr and
// the exit code.
func buildWith(t *testing.T, files map[string]string, args ...string) (string, int) {
	t.Helper()
	root := tree(t, files)
	var out, errb bytes.Buffer
	code := run(append([]string{filepath.Join(root, "doc.md")}, args...), &out, &errb)
	return errb.String(), code
}

// One or two findings are printed in full; an error counts as a warning
// in the summary, and the build still succeeds.
func TestRunStyleCheckLineShort(t *testing.T) {
	dir := stylePlugin(t)
	stderr, code := buildWith(t, map[string]string{"doc.md": "# A\n"}, "--plugin", dir, "--style", "acme/bad")
	if code != 0 {
		t.Errorf("exit %d, want 0", code)
	}
	want := "md2html: style acme/bad: light --muted on --bg is 2.22:1, under 3:1; light --muted on --code-bg is 2.01:1, under 3:1\n"
	if !strings.Contains(stderr, want) || !strings.Contains(stderr, ", 1 warning(s)\n") {
		t.Errorf("stderr:\n%s\nwant the line %q and 1 warning", stderr, want)
	}
}

func TestRunStyleCheckLineCounted(t *testing.T) {
	dir := stylePlugin(t)
	stderr, _ := buildWith(t, map[string]string{"doc.md": "# A\n"}, "--plugin", dir, "--style", "acme/worse")
	want := "md2html: style acme/worse: 4 errors (md2html --check-style --style acme/worse for details)\n"
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr:\n%s\nwant %q", stderr, want)
	}
}

// Warnings alone print the line but leave the summary's count alone.
func TestRunStyleCheckLineWarningsDoNotCount(t *testing.T) {
	dir := stylePlugin(t)
	stderr, _ := buildWith(t, map[string]string{"doc.md": "# A\n"}, "--plugin", dir, "--style", "acme/quiet")
	if !strings.Contains(stderr, "md2html: style acme/quiet: light --muted on --code-bg is 4.28:1, under 4.5:1 (WCAG AA)\n") ||
		!strings.Contains(stderr, ", 0 warning(s)\n") {
		t.Errorf("stderr:\n%s", stderr)
	}
}

// A list chosen by front matter on two pages converted in parallel is
// reported once. A style set in :root alone is reported too, since a
// reader who picks light gets the default's colour.
func TestRunStyleCheckLineForFrontMatterStyle(t *testing.T) {
	dir := stylePlugin(t)
	if err := os.WriteFile(filepath.Join(dir, "rootonly.css"), []byte(":root{--accent:#2c5a85}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"format": 1, "name": "acme", "styles": {"bad": {"css": ["bad.css"]}, "rootonly": {"css": ["rootonly.css"]}}}`
	if err := os.WriteFile(filepath.Join(dir, "md2html-plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	page := "---\nstyle: acme/bad\n---\n# A\n\n[B](b.md)\n"
	stderr, _ := buildWith(t, map[string]string{"doc.md": page, "b.md": "---\nstyle: acme/bad\n---\n# B\n"}, "--plugin", dir)
	if n := strings.Count(stderr, "md2html: style acme/bad: "); n != 1 {
		t.Errorf("acme/bad reported %d times, want once:\n%s", n, stderr)
	}
	stderr, _ = buildWith(t, map[string]string{"doc.md": "---\nstyle: acme/rootonly\n---\n# A\n"}, "--plugin", dir)
	if want := "md2html: style acme/rootonly: light --accent is #9a4f2c (light picked) but #2c5a85 (light system, nothing picked)\n"; !strings.Contains(stderr, want) {
		t.Errorf("stderr:\n%s\nwant %q", stderr, want)
	}
}

func TestRunNoStyleNoLine(t *testing.T) {
	stderr, _ := buildWith(t, map[string]string{"doc.md": "# A\n"})
	if strings.Contains(stderr, "md2html: style ") {
		t.Errorf("stderr:\n%s", stderr)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./cmd/md2html/ -run 'TestRunStyleCheck|TestRunNoStyle'`
Expected: FAIL: no `md2html: style` line is printed (`TestRunNoStyleNoLine` passes already).

- [ ] **Step 3: Add the styleChecks type**

At the end of `cmd/md2html/main.go`, add (`sort` and `sync` join the imports if absent):

```go
// styleChecks checks each style list a run's pages are drawn in, once
// however many pages share it, as Options.StyleChosen. Pages convert in
// parallel, so it locks; the check itself runs outside the lock.
type styleChecks struct {
	mu    sync.Mutex
	found map[string][]stylecheck.Finding
}

func newStyleChecks() *styleChecks {
	return &styleChecks{found: map[string][]stylecheck.Finding{}}
}

func (s *styleChecks) chosen(names []string, css string) {
	key := strings.Join(names, ",")
	s.mu.Lock()
	if _, seen := s.found[key]; seen {
		s.mu.Unlock()
		return
	}
	s.found[key] = nil
	s.mu.Unlock()
	fs := stylecheck.Check(css)
	s.mu.Lock()
	s.found[key] = fs
	s.mu.Unlock()
}

// styleLine is one style list's line for stderr, and whether it counts as
// a warning: only a list with an error does.
type styleLine struct {
	text   string
	counts bool
}

// lines are the lists with findings, in name order: each one's findings
// when it has one or two, otherwise their counts and where to see them.
func (s *styleChecks) lines() []styleLine {
	keys := make([]string, 0, len(s.found))
	for k, fs := range s.found {
		if len(fs) > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := make([]styleLine, len(keys))
	for i, k := range keys {
		fs := s.found[k]
		text := fmt.Sprintf("style %s: %s", k, stylecheck.Short(fs))
		if len(fs) > 2 {
			text += fmt.Sprintf(" (md2html --check-style --style %s for details)", k)
		}
		out[i] = styleLine{text, stylecheck.HasError(fs)}
	}
	return out
}
```

- [ ] **Step 4: Wire it into run**

In `run`, just before `var wg sync.WaitGroup`, add:

```go
	// Each style list a page is drawn in is checked once, as the pages
	// convert, and reported with the other warnings below.
	checks := newStyleChecks()
```

Where each goroutine sets `opts.Plugins, opts.Styles, opts.PluginCache = plugins, styles, cache`, add on the next line:

```go
			opts.StyleChosen = checks.chosen
```

After the loop over `results` that counts `written, refused, failed, warned` (and before the loop over `res.Warnings`), add:

```go
	for _, l := range checks.lines() {
		fmt.Fprintf(stderr, "md2html: %s\n", l.text)
		if l.counts {
			warned++
		}
	}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go vet ./... && go test -race ./cmd/md2html/ && go test ./...`
Expected: PASS, with no race reported.

- [ ] **Step 6: Commit**

```bash
git add cmd/md2html/main.go cmd/md2html/stylecheck_test.go
git commit -m "feat: a build reports a style's colour problems in one line"
```

### Task 6: Docs

**Files:**
- Modify: `docs/authoring.md` ("### Checking a style", around line 1328; the "**Look at it.**" bullet under "#### Keeping it consistent")
- Modify: `README.md:389` (flag table)
- Modify: `CHANGELOG.md` (Unreleased, Added)

- [ ] **Step 1: Rewrite "Checking a style"**

Replace the section body (from "Run `md2html --plugin ./_style --list-styles`" to "so read the warnings.") with:

```markdown
Run `md2html --plugin ./_style --list-styles` to check a plugin loads and
every style in it builds; a broken one exits with the reason.

Then run `md2html --plugin ./_style --check-style` to measure each style's
colours, stacked on the default as a page would use it. `--style a,b`
checks that stack instead, and with neither flag the built-in styles are
checked. Each style gets a line, then its findings:

    acme/plain: 1 error, 1 warning
      error    dark --muted on --code-bg is 2.60:1, under 3:1
      warning  light --muted on --code-bg is 4.21:1, under 4.5:1 (WCAG AA)

| Severity | Means |
|---|---|
| error | Broken or unreadable: a colour missing from one theme, or different when the reader picks the theme their system already shows; a value that is not a colour; text under 3:1; chart series out of the lightness band, grey, or too alike for readers with normal or colour-blind vision |
| warning | Short of a target: text under 4.5:1; series or status colours faint on `--chart-surface`; status colours too close to each other or to a series; a colour the check cannot measure, such as `color-mix()` |
| note | Advice: neighbouring series that colour-blind readers can only just tell apart |

It exits 1 if any style has an error, and 0 otherwise. The chart rules
are the dataviz skill's, and the built-in styles pass all of them.

A build checks each style its pages use and prints one line per style
with problems: the findings themselves when there are one or two, or
their counts. Only a style with an error adds to the run's warnings, and
a page always builds.

Then build a page and look at it at 400px wide and in both themes. A
front matter `style:` that fails warns and the page falls back to the
run's style, so read the warnings.
```

- [ ] **Step 2: Point "Look at it" at the check**

In the "**Look at it.**" bullet, replace "run `--plugin ./_style --list-styles`, then build" with "run `md2html --plugin ./_style --check-style`, then build".

- [ ] **Step 3: README and CHANGELOG**

In `README.md`, after the `--list-styles` row, add:

```markdown
| `--check-style` | check the colours of each `--plugin` style, the `--style` list, or the built-in styles, and exit; 1 if any has an error |
```

In `CHANGELOG.md`, under "## Unreleased" / "### Added", add as the last bullet:

```markdown
- `--check-style` measures a style's colours: text contrast against WCAG,
  and chart colours against the dataviz skill's rules, including
  colour-blind separation. A build reports a style's problems in one line.
  The built-in styles pass every rule; the default's light `--warn` and
  its status colours moved slightly to get there.
```

- [ ] **Step 4: Run the suite**

Run: `go vet ./... && go test ./...`
Expected: PASS. `TestGuideStarterStyleBuilds` and the guide tests read `docs/authoring.md`; if one pins replaced text, update its expected text to the new wording.

- [ ] **Step 5: Try it by hand**

```bash
go build -o /tmp/md2html-check ./cmd/md2html
/tmp/md2html-check --check-style; echo "exit $?"
```

Expected: `claude-artifact/claude-artifact: no problems`, `default/default: no problems`, `exit 0`.

- [ ] **Step 6: Commit**

```bash
git add docs/authoring.md README.md CHANGELOG.md
git commit -m "docs: --check-style in the guide, README and changelog"
```
