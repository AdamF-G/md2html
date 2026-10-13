package md2html

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// convert is container_test.go's helper: a fragment under the stylesheet
// "/**/", which defines no chart tokens.

func TestVegaLiteFenceRendersAChart(t *testing.T) {
	src := "```vega-lite {caption=\"Points\"}\nmark: point\ndata: {values: [{v: 1}]}\nencoding: {x: {field: v, type: quantitative}}\n```\n"
	got := convert(t, src, nil)
	if !strings.Contains(got, `<figure class="md2html-chart" id="md2html-c1">`) || strings.Contains(got, "language-vega-lite") {
		t.Errorf("fence did not render as a chart: %.300s", got)
	}
	if !strings.Contains(got, "<figcaption>Points</figcaption>") {
		t.Error("the info-string caption did not become the chart's caption")
	}
}

// Classes in the info string reach the chart's figure, as they reach a
// code block's element, after md2html-chart.
func TestVegaLiteFenceKeepsItsClasses(t *testing.T) {
	body := "mark: point\ndata: {values: [{v: 1}]}\nencoding: {x: {field: v, type: quantitative}}\n```\n"
	for _, info := range []string{`vega-lite {.wide .tall caption="Points"}`, `{.vega-lite .wide .tall caption="Points"}`} {
		got := convert(t, "```"+info+"\n"+body, nil)
		if !strings.Contains(got, `<figure class="md2html-chart wide tall" id="md2html-c1">`) {
			t.Errorf("%s: the classes did not reach the figure: %.300s", info, got)
		}
	}
	if got := convert(t, "```vega-lite {class=\"a&quot;b\"}\n"+body, nil); !strings.Contains(got, `<figure class="md2html-chart a&amp;quot;b"`) {
		t.Errorf("a class was not escaped: %.300s", got)
	}
}

func TestBadChartWarnsAndShowsTheSource(t *testing.T) {
	var warns []string
	got := convert(t, "```vega-lite\nmark: [oops\n```\n", func(s string) { warns = append(warns, s) })
	if len(warns) != 1 || !strings.Contains(warns[0], "vega-lite fence") || !strings.Contains(got, "language-vega-lite") {
		t.Errorf("warns %v; got %.200s", warns, got)
	}
}

func TestNoChartsLeavesTheFenceAsCode(t *testing.T) {
	out, _ := Convert([]byte("```vega-lite\nmark: point\n```\n"), Options{Fragment: true, CSS: "/**/", NoCharts: true})
	if !strings.Contains(string(out), "language-vega-lite") {
		t.Error("NoCharts still rendered a chart")
	}
}

func TestChartsNumberFromOneOnEachPage(t *testing.T) {
	two := strings.Repeat("```vega-lite\nmark: point\ndata: {values: [{v: 1}]}\nencoding: {x: {field: v, type: quantitative}}\n```\n\n", 2)
	got := convert(t, two, nil)
	if !strings.Contains(got, `id="md2html-c1"`) || !strings.Contains(got, `id="md2html-c2"`) {
		t.Error("two charts did not get c1 and c2")
	}
	if again := convert(t, two, nil); !strings.Contains(again, `id="md2html-c1"`) {
		t.Error("a second page did not start again at c1")
	}
}

func TestChartsStayVisibleUnderAStylesheetWithoutTokens(t *testing.T) {
	out, err := Convert([]byte("```vega-lite\nmark: point\ndata: {values: [{v: 1}]}\nencoding: {x: {field: v, type: quantitative}}\n```\n"),
		Options{Fragment: true, CSS: "body { font-family: serif }"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "var(--series-1)") || !strings.Contains(string(out), "<svg") {
		t.Error("under a stylesheet with no chart tokens the chart still names var() colours, which would render black")
	}
}

// Chart text is data drawn in SVG, not prose: an inline rewriter must not
// put HTML inside an SVG <text>, which would hide the label.
func TestChartTextIsNotRewrittenAsProse(t *testing.T) {
	src := "```vega-lite\nmark: bar\ndata: {values: [{k: \"a [proven]\", v: 1}]}\nencoding: {y: {field: k, type: nominal}, x: {field: v, type: quantitative}, tooltip: {field: k}}\n```\n"
	got := convert(t, src, nil)
	if strings.Contains(got, "chip") || !strings.Contains(got, ">a [proven]</text>") {
		t.Errorf("chart text was rewritten: %s", got)
	}
}

// A stylesheet that defines --fg and --muted, as most do, but not the
// series colours, has no chart tokens to point at either. The chart is
// written in literal colours: the stylesheet's own over the default's. Both
// forms of a block's last declaration are read, with and without its ";".
func TestChartsStayVisibleUnderAStylesheetWithSomeTokens(t *testing.T) {
	for _, end := range []string{"", ";"} {
		css := `:root[data-theme="light"] { --fg: #111111; --muted: #555555` + end + ` } :root[data-theme="dark"] { --fg: #eeeeee; --muted: #aaaaaa` + end + ` }`
		out, err := Convert([]byte("```vega-lite\nmark: point\ndata: {values: [{v: 1}]}\nencoding: {x: {field: v, type: quantitative}}\n```\n"),
			Options{Fragment: true, CSS: css})
		if err != nil {
			t.Fatal(err)
		}
		// Only the chart: the page's own <style> holds the same values.
		s := string(out)
		start := strings.Index(s, `<figure class="md2html-chart"`)
		stop := strings.Index(s, "</figure>")
		if start < 0 || stop < start {
			t.Fatalf("no chart in %.300s", s)
		}
		chart := s[start:stop]
		if strings.Contains(chart, "var(--series-1)") {
			t.Errorf("end %q: the chart names var() colours the stylesheet does not define", end)
		}
		// --muted draws axis labels; the stylesheet's value, not the default's.
		if !strings.Contains(chart, "#555555") || strings.Contains(chart, "#6b645c") {
			t.Errorf("end %q: the chart did not take the stylesheet's own --muted over the default's", end)
		}
	}
}

// A data url reads beside the source document, stays inside DataRoot, and
// is refused when there is no source document to read beside.
func TestChartDataResolvesBesideTheSource(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"docs/d.csv": "v\n1\n", "outside.csv": "v\n2\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fence := func(url string) string {
		return "```vega-lite\nmark: point\ndata: {url: " + url + "}\nencoding: {x: {field: v, type: quantitative}}\n```\n"
	}
	run := func(src string, opt Options) (string, []string) {
		var warns []string
		opt.Fragment, opt.Warn = true, func(s string) { warns = append(warns, s) }
		out, err := Convert([]byte(src), opt)
		if err != nil {
			t.Fatal(err)
		}
		return string(out), warns
	}
	doc := filepath.Join(root, "docs", "a.md")
	if out, w := run(fence("d.csv"), Options{SourcePath: doc}); len(w) != 0 || !strings.Contains(out, "md2html-chart") {
		t.Errorf("data beside the page: warnings %v", w)
	}
	if _, w := run(fence("../outside.csv"), Options{SourcePath: doc}); len(w) != 1 || !strings.Contains(w[0], "vega-lite fence 1") {
		t.Errorf("data outside the page's directory: warnings %v", w)
	}
	if _, w := run(fence("../outside.csv"), Options{SourcePath: doc, DataRoot: root}); len(w) != 0 {
		t.Errorf("data inside DataRoot: warnings %v", w)
	}
	if _, w := run(fence("d.csv"), Options{}); len(w) != 1 {
		t.Errorf("data with no source path: warnings %v", w)
	}
}

// The CLI converts pages in parallel against one cache: each page numbers
// its own charts, and the shared font measurer is read concurrently.
func TestChartsConvertInParallel(t *testing.T) {
	two := strings.Repeat("```vega-lite\nmark: point\ndata: {values: [{v: 1}]}\nencoding: {x: {field: v, type: quantitative}, tooltip: {field: v}}\n```\n\n", 2)
	cache := NewPluginCache()
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			out, err := Convert([]byte(two), Options{Fragment: true, Styles: []string{"claude-artifact"}, PluginCache: cache})
			if err != nil || !strings.Contains(string(out), `id="md2html-c1"`) || !strings.Contains(string(out), `id="md2html-c2"`) || strings.Contains(string(out), `id="md2html-c3"`) {
				t.Errorf("err %v; a page's charts are not c1 and c2", err)
			}
		})
	}
	wg.Wait()
}

// Under literal colours the tooltip box's --rule border is written as a
// colour too: the stylesheet's own --rule, or the default's.
func TestLiteralChartsResolveTheBoxBorder(t *testing.T) {
	for rule, want := range map[string]string{"": "#e2ddd6", "--rule: #abcdef;": "#abcdef"} {
		css := `:root[data-theme="light"] { --fg: #111111; ` + rule + ` } :root[data-theme="dark"] { --fg: #eeeeee; }`
		out, err := Convert([]byte("```vega-lite\nmark: point\ndata: {values: [{v: 1}]}\nencoding: {x: {field: v, type: quantitative}, tooltip: {field: v}}\n```\n"),
			Options{Fragment: true, CSS: css})
		if err != nil {
			t.Fatal(err)
		}
		s := string(out)
		start, stop := strings.Index(s, `<figure class="md2html-chart"`), strings.Index(s, "</figure>")
		if start < 0 || stop < start || !strings.Contains(s[start:stop], "md2html-tip") {
			t.Fatalf("no chart with a box in %.300s", s)
		}
		if chart := s[start:stop]; strings.Contains(chart, "var(--rule)") || !strings.Contains(chart, "stroke: "+want) {
			t.Errorf("stylesheet %q: the box border is not %s: %s", rule, want, regexp.MustCompile(`<rect[^>]*rx="4"[^>]*>`).FindString(chart))
		}
	}
}
