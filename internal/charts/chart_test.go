package charts

import (
	"regexp"
	"strings"
	"testing"
)

func TestRenderWritesAFigure(t *testing.T) {
	html, warns, err := Render(Request{Body: []byte("mark: point\ndata: {values: [{v: 1}, {v: 2}]}\nencoding: {x: {field: v, type: quantitative}, tooltip: {field: v}}\n"),
		Caption: "Two points", Index: 3, Style: Style{Measurer: fixed{}, FontFamily: "sans-serif", Width: 400}})
	if err != nil || len(warns) != 0 {
		t.Fatalf("err %v warns %v", err, warns)
	}
	for _, want := range []string{`<figure class="md2html-chart" id="md2html-c3">`, "<figcaption>Two points</figcaption>",
		`md2html-c3-m0`, `var(--series-1)`, "<details class=\"container md2html-chart-data\">", "<style>"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestRenderRejectsBadYAML(t *testing.T) {
	if _, _, err := Render(Request{Body: []byte("mark: [unclosed"), Style: Style{Measurer: fixed{}}}); err == nil {
		t.Error("bad YAML rendered")
	}
}

// A stylesheet without tokens: colours are literal and the chart still visible.
func TestRenderWithLiteralColours(t *testing.T) {
	html, _, err := Render(Request{Body: []byte(`{"mark":"point","data":{"values":[{"v":1}]},"encoding":{"x":{"field":"v","type":"quantitative"}}}`),
		Index: 1, Style: Style{Measurer: fixed{}, Width: 300, Literal: map[string]string{"series-1": "#2a78d6", "chart-surface": "#ffffff", "fg": "#000000", "muted": "#555555", "chart-grid": "#dddddd", "chart-axis": "#bbbbbb"}}})
	if err != nil || strings.Contains(html, "var(--") || !strings.Contains(html, "#2a78d6") {
		t.Errorf("err %v; literal render: %.200s", err, html)
	}
}

// An author's literal colour renders the chart once per theme.
func TestRenderTwiceForAuthorColours(t *testing.T) {
	light := map[string]string{"series-1": "#111111", "chart-surface": "#ffffff"}
	dark := map[string]string{"series-1": "#eeeeee", "chart-surface": "#000000"}
	html, _, _ := Render(Request{Body: []byte(`{"mark":{"type":"point","color":"#ff0000"},"data":{"values":[{"v":1}]},"encoding":{"x":{"field":"v","type":"quantitative"}}}`),
		Index: 1, Style: Style{Measurer: fixed{}, Width: 300, Light: light, Dark: dark}})
	if strings.Count(html, "<svg") != 2 || !strings.Contains(html, "md2html-chart-light") || !strings.Contains(html, "md2html-chart-dark") {
		t.Errorf("want a light and a dark render: %.300s", html)
	}
}

// A data value that reads like a token renders as data.
func TestRenderKeepsATokenNameInData(t *testing.T) {
	_, warns, err := Render(Request{Body: []byte(`{"data":{"values":[{"k":"token:fg","v":1}]},"mark":"bar","encoding":{"y":{"field":"k","type":"nominal"},"x":{"field":"v","type":"quantitative"}}}`),
		Index: 1, Style: Style{Measurer: fixed{}, Width: 300}})
	if err != nil || len(warns) != 0 {
		t.Errorf("err %v warns %v", err, warns)
	}
}

// A build has no container to measure, so width "container" takes the
// style's width, as a spec with no width does.
func TestContainerWidthTakesTheStylesWidth(t *testing.T) {
	plot := func(width string) string {
		html, _, err := Render(Request{Body: []byte(`{` + width + `"data":{"values":[{"v":1}]},"mark":"point","encoding":{"x":{"field":"v","type":"quantitative"}}}`),
			Index: 1, Style: Style{Measurer: fixed{}, Width: 480}})
		if err != nil {
			t.Fatal(err)
		}
		return regexp.MustCompile(`<svg[^>]* width="([0-9.]+)"`).FindStringSubmatch(html)[1]
	}
	if a, b := plot(`"width":"container",`), plot(""); a != b {
		t.Errorf("width container lays out at %s px, no width at %s", a, b)
	}
}

// Nor is "container" a height: the spec lays out at Vega-Lite's default
// height, as one with no height does, not at a fixed one.
func TestContainerHeightTakesVegaLitesDefault(t *testing.T) {
	plot := func(height string) string {
		html, _, err := Render(Request{Body: []byte(`{` + height + `"data":{"values":[{"k":"a","v":1},{"k":"b","v":2}]},"mark":"bar","encoding":{"y":{"field":"k","type":"nominal"},"x":{"field":"v","type":"quantitative"}}}`),
			Index: 1, Style: Style{Measurer: fixed{}, Width: 480}})
		if err != nil {
			t.Fatal(err)
		}
		return regexp.MustCompile(`<svg[^>]* height="([0-9.]+)"`).FindStringSubmatch(html)[1]
	}
	if a, b := plot(`"height":"container",`), plot(""); a != b {
		t.Errorf("height container lays out at %s px, no height at %s", a, b)
	}
}
