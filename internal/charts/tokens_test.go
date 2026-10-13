package charts

import (
	"strings"
	"testing"
)

func TestTokenizeRewritesEveryPlaceholder(t *testing.T) {
	in := `<path fill="` + placeholder("series-1") + `" stroke="` + placeholder("chart-surface") + `"/>`
	out, literal, err := tokenize(in)
	if err != nil || literal {
		t.Fatalf("err %v literal %v", err, literal)
	}
	if out != `<path style="fill: var(--series-1); stroke: var(--chart-surface)"/>` {
		t.Errorf("got %s", out)
	}
}

func TestTokenizeMergesIntoAnExistingStyle(t *testing.T) {
	in := `<path fill="` + placeholder("series-1") + `" d="M0 0" style="opacity: 0.5"/>`
	out, _, err := tokenize(in)
	if err != nil || out != `<path style="fill: var(--series-1); opacity: 0.5" d="M0 0"/>` {
		t.Errorf("err %v got %s", err, out)
	}
	in = `<rect fill="` + placeholder("series-2") + `" x="1" style="opacity: 0.5;" stroke="` + placeholder("fg") + `"/>`
	out, _, err = tokenize(in)
	if err != nil || strings.Count(out, "style=") != 1 ||
		!strings.Contains(out, `style="fill: var(--series-2); stroke: var(--fg); opacity: 0.5"`) {
		t.Errorf("err %v got %s", err, out)
	}
}

func TestTokenizeFlagsALiteralColour(t *testing.T) {
	_, literal, _ := tokenize(`<path fill="#ff0000"/>`)
	if !literal {
		t.Error("a literal colour was not reported")
	}
	for _, ok := range []string{`fill="none"`, `fill="transparent"`, `stroke="none"`} {
		if _, l, _ := tokenize(`<path ` + ok + `/>`); l {
			t.Errorf("%s is not a colour but was reported as one", ok)
		}
	}
}

func TestTokenNamesInASpec(t *testing.T) {
	spec := map[string]any{"mark": map[string]any{"type": "bar", "color": "token:status-critical", "stroke": "token:serie-1"}}
	out, warns := substituteTokenNames(spec)
	m := out.(map[string]any)["mark"].(map[string]any)
	if m["color"] != placeholder("status-critical") {
		t.Errorf("color %v", m["color"])
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "serie-1") {
		t.Errorf("want one warning naming serie-1, got %v", warns)
	}
}

// A spec that names a placeholder colour itself would be silently recoloured.
func TestAPlaceholderInTheSpecIsWarnedAbout(t *testing.T) {
	_, warns := substituteTokenNames(map[string]any{"mark": map[string]any{"color": placeholder("series-3")}})
	if len(warns) != 1 {
		t.Errorf("want a warning, got %v", warns)
	}
}

func TestTokenValuesReadsBothThemes(t *testing.T) {
	css := `:root { --series-1: #111111; }
:root[data-theme="light"] { --series-1: #2a78d6; --fg: #000; }
:root[data-theme="dark"] { --series-1: #3987e5; --fg: #fff; }`
	light, dark, ok := TokenValues(css)
	if !ok || light["series-1"] != "#2a78d6" || dark["series-1"] != "#3987e5" || light["fg"] != "#000" {
		t.Errorf("light %v dark %v ok %v", light, dark, ok)
	}
	if _, _, ok := TokenValues("body { color: red }"); ok {
		t.Error("a stylesheet without tokens reported some")
	}
}

func TestTokenValuesResolvesVarReferences(t *testing.T) {
	css := `:root[data-theme="light"] { --bg: #fbfaf8; --chart-surface: var(--bg); --fg: var(--missing); }
:root[data-theme="dark"] { --chart-surface: var(--bg); --bg: #1a1a1a; }`
	light, dark, ok := TokenValues(css)
	if !ok || light["chart-surface"] != "#fbfaf8" {
		t.Errorf("light %v ok %v", light, ok)
	}
	if dark["chart-surface"] != "#1a1a1a" {
		t.Errorf("a reference to a later declaration did not resolve: %v", dark)
	}
	if light["fg"] != "var(--missing)" {
		t.Errorf("an undefined reference should be left alone, got %q", light["fg"])
	}
}

// Minified CSS drops the ; before a block's closing brace.
func TestTokenValuesReadsTheLastDeclarationWithoutASemicolon(t *testing.T) {
	css := `:root[data-theme="light"]{--fg:#000;--series-1:#2a78d6}:root[data-theme="dark"]{--series-1: #3987e5 }`
	light, dark, ok := TokenValues(css)
	if !ok || light["series-1"] != "#2a78d6" || light["fg"] != "#000" || dark["series-1"] != "#3987e5" {
		t.Errorf("light %v dark %v ok %v", light, dark, ok)
	}
}

// Data is the author's, not colours: a value that reads "token:…" or
// "#0a0b01" in data, in datasets or in a layer's data is left alone and
// warns of nothing. Warnings come in a fixed order.
func TestTokenNamesInDataAreLeftAlone(t *testing.T) {
	spec := map[string]any{
		"data":     map[string]any{"values": []any{map[string]any{"k": "token:fg", "c": placeholder("fg")}}},
		"datasets": map[string]any{"d": []any{map[string]any{"k": "token:nope"}}},
		"layer":    []any{map[string]any{"data": map[string]any{"values": []any{map[string]any{"k": "token:fg"}}}, "mark": "point"}},
		"mark":     map[string]any{"color": "token:zz-b", "stroke": "token:zz-a", "fill": "token:zz-c"},
	}
	out, warns := substituteTokenNames(spec)
	o := out.(map[string]any)
	if v := o["data"].(map[string]any)["values"].([]any)[0].(map[string]any)["k"]; v != "token:fg" {
		t.Errorf("data value rewritten to %v", v)
	}
	if v := o["layer"].([]any)[0].(map[string]any)["data"].(map[string]any)["values"].([]any)[0].(map[string]any)["k"]; v != "token:fg" {
		t.Errorf("a layer's data value rewritten to %v", v)
	}
	if len(warns) != 3 || !strings.Contains(warns[0], "zz-a") || !strings.Contains(warns[1], "zz-b") || !strings.Contains(warns[2], "zz-c") {
		t.Errorf("want three warnings, for zz-a, zz-b and zz-c in that order, got %v", warns)
	}
}

// --rule is read for the tooltip box border, but is not a chart token.
func TestTokenValuesReadsTheBoxBorder(t *testing.T) {
	light, dark, ok := TokenValues(`:root[data-theme="light"] { --rule: #aaaaaa } :root[data-theme="dark"] { --rule: #222222 }`)
	if ok || light["rule"] != "#aaaaaa" || dark["rule"] != "#222222" {
		t.Errorf("ok %v light %v dark %v", ok, light, dark)
	}
}
