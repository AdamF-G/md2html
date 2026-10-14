package md2html

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func builtinStyle(t *testing.T, plugin, style string) selected {
	t.Helper()
	for _, p := range BuiltinPlugins() {
		if p.Name == plugin {
			return selected{p, p.Styles[style]}
		}
	}
	t.Fatalf("no built-in plugin %s", plugin)
	return selected{}
}

func TestBuiltinStylesDeclareCharts(t *testing.T) {
	d := builtinStyle(t, "default", "default").style.Charts
	if d == nil || d.Metrics != "builtin:system-sans" || d.Pad != 0.1 || d.Width != 640 || d.FontFamily == "" {
		t.Errorf("default's charts block = %+v", d)
	}
	a := builtinStyle(t, "claude-artifact", "claude-artifact").style.Charts
	if a == nil || a.Font != "fonts/ibm-plex-sans-400-600.woff2" || a.Width != 640 || !strings.Contains(a.FontFamily, "IBM Plex Sans") {
		t.Errorf("claude-artifact's charts block = %+v", a)
	}
	got := NewPluginCache().chartStyle([]selected{builtinStyle(t, "claude-artifact", "claude-artifact")})
	if got.Font != a.Font || got.Metrics != "" || got.Pad != 0 || got.plugin == nil || got.plugin.Name != "claude-artifact" {
		t.Errorf("claude-artifact over default = %+v", got)
	}
}

func TestStackedStylesLaterChartsBlockWins(t *testing.T) {
	root := pluginDir(t, map[string]string{
		"a/md2html-plugin.json": `{"format": 1, "name": "a", "styles": {"a": {"css": ["a.css"], "charts": {"width": 500, "font-family": "serif"}}}}`,
		"a/a.css":               "a{}",
		"b/md2html-plugin.json": `{"format": 1, "name": "b", "styles": {"b": {"css": ["b.css"], "charts": {"width": 800, "pad": 0.2}}}}`,
		"b/b.css":               "b{}",
	})
	c := NewPluginCache()
	ps, err := c.LoadAll([]string{"a", "b"}, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := c.chartStyle([]selected{{ps[0], ps[0].Styles["a"]}, {ps[1], ps[1].Styles["b"]}})
	if got.Width != 800 || got.FontFamily != "serif" || got.Pad != 0.2 || got.Metrics != "builtin:system-sans" {
		t.Errorf("stacked = %+v", got)
	}
}

func TestManifestChartsBlockRejectsAFontOutsideThePlugin(t *testing.T) {
	root := pluginDir(t, map[string]string{
		"p/md2html-plugin.json": `{"format": 1, "name": "p", "styles": {"s": {"css": ["s.css"], "charts": {"font": "../x.woff2"}}}}`,
		"p/s.css":               "a{}",
		"x.woff2":               "not a font",
	})
	_, err := NewPluginCache().Load("p", root, nil)
	if err == nil || !strings.Contains(err.Error(), "outside the plugin") {
		t.Errorf("err = %v", err)
	}
}

func TestManifestChartsBlockRefusesBadValues(t *testing.T) {
	for _, c := range []struct{ name, charts, want string }{
		{"metrics", `{"metrics": "builtin:serif"}`, "metrics"},
		{"pad", `{"pad": 2}`, "pad"},
		{"width", `{"width": 50}`, "width"},
		{"both", `{"font": "f.woff2", "metrics": "builtin:system-sans"}`, "font and metrics"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseManifest([]byte(`{"format": 1, "name": "a", "styles": {"s": {"css": ["x.css"], "charts": `+c.charts+`}}}`), "a", false, nil)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestUnknownChartsKeyWarns(t *testing.T) {
	var warnings []string
	p, err := parseManifest([]byte(`{"format": 1, "name": "a", "styles": {"s": {"css": ["x.css"], "charts": {"width": 500, "colour": "red"}}}}`),
		"a", false, func(w string) { warnings = append(warnings, w) })
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], `"colour"`) || !strings.Contains(warnings[0], "charts") {
		t.Errorf("warnings = %q", warnings)
	}
	if p.Styles["s"].Charts.Width != 500 {
		t.Error("the known key was lost")
	}
}

// pad widens metrics-measured text only: beside a font, whose widths are
// exact, it would do nothing, so the manifest warns.
func TestChartsPadBesideAFontWarns(t *testing.T) {
	var warnings []string
	_, err := parseManifest([]byte(`{"format": 1, "name": "a", "styles": {"s": {"css": ["x.css"], "charts": {"font": "f.woff2", "pad": 0.2}}}}`),
		"a", false, func(w string) { warnings = append(warnings, w) })
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "pad") || !strings.Contains(warnings[0], "font") || !strings.Contains(warnings[0], `"s"`) {
		t.Errorf("warnings = %q", warnings)
	}
}

// pad: 0 is a value, not an absent key: it overrides the default's 0.1.
func TestChartsPadZeroOverridesTheDefault(t *testing.T) {
	for _, charts := range []string{`{"pad": 0}`, `{"metrics": "builtin:system-sans", "pad": 0}`} {
		root := pluginDir(t, map[string]string{
			"z/md2html-plugin.json": `{"format": 1, "name": "z", "styles": {"z": {"css": ["z.css"], "charts": ` + charts + `}}}`,
			"z/z.css":               "z{}",
		})
		c := NewPluginCache()
		ps, err := c.LoadAll([]string{"z"}, root, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := c.chartStyle([]selected{{ps[0], ps[0].Styles["z"]}}); got.Pad != 0 || got.Metrics != systemSansMetrics {
			t.Errorf("%s over default = %+v, want pad 0", charts, got)
		}
	}
}

const tokenBar = "```vega-lite\ndata: {values: [{a: x, b: 1}]}\nmark: {type: bar, color: \"token:%s\"}\nencoding: {x: {field: a, type: nominal}, y: {field: b, type: quantitative}}\n```\n"

// A page token is a chart token like any other: it draws as a var() and
// so the chart follows the theme in a single copy.
func TestChartsNameAPageToken(t *testing.T) {
	var warns []string
	out, err := Convert([]byte(fmt.Sprintf(tokenBar, "accent")), Options{Fragment: true, Warn: func(s string) { warns = append(warns, s) }})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	got = got[strings.Index(got, "<main"):]
	if len(warns) != 0 {
		t.Errorf("warns: %v", warns)
	}
	if !strings.Contains(got, "fill: var(--accent)") || strings.Contains(got, "md2html-chart-dark") {
		t.Errorf("token:accent did not draw as one copy in var(--accent): %.400s", got)
	}
}

// A token a style declares in its charts block can be named too, and adds
// to the tokens of the style under it.
func TestChartsNameAStyleToken(t *testing.T) {
	root := pluginDir(t, map[string]string{
		"house/md2html-plugin.json": `{"format": 1, "name": "house", "styles": {"house": {"css": ["h.css"], "charts": {"tokens": ["inject"]}}}}`,
		"house/h.css":               ":root { --inject: #c03 } :root[data-theme=\"light\"] { --inject: #c03 } :root[data-theme=\"dark\"] { --inject: #f69 }",
	})
	var warns []string
	opt := Options{Fragment: true, Plugins: []string{filepath.Join(root, "house")}, Styles: []string{"house"},
		Warn: func(s string) { warns = append(warns, s) }}
	out, err := Convert([]byte(fmt.Sprintf(tokenBar, "inject")), opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 || !strings.Contains(string(out), "fill: var(--inject)") {
		t.Errorf("warns %v; got %.400s", warns, out)
	}

	// Beside a literal colour the chart is drawn once per theme, each copy
	// in that theme's value of the style's token.
	warns = nil
	both := "```vega-lite\ndata: {values: [{a: x, b: 1}]}\nlayer:\n  - {mark: {type: bar, color: \"token:inject\"}, encoding: {x: {field: a, type: nominal}, y: {field: b, type: quantitative}}}\n  - {mark: {type: rule, color: \"#123456\"}, encoding: {y: {datum: 0}}}\n```\n"
	out, err = Convert([]byte(both), opt)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.ToLower(string(out))
	got = got[strings.Index(got, "<main"):]
	if !strings.Contains(got, "#c03") || !strings.Contains(got, "#f69") || strings.Contains(got, "#0a0b") {
		t.Errorf("warns %v; the copies do not hold each theme's value: %.600s", warns, got)
	}

	// Without the style, the name is unknown.
	warns = nil
	if _, err := Convert([]byte(fmt.Sprintf(tokenBar, "inject")), Options{Fragment: true, Warn: func(s string) { warns = append(warns, s) }}); err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], `"token:inject" names no token`) {
		t.Errorf("warns = %v", warns)
	}
}

func TestStackedStyleTokensAddUp(t *testing.T) {
	root := pluginDir(t, map[string]string{
		"a/md2html-plugin.json": `{"format": 1, "name": "a", "styles": {"a": {"css": ["a.css"], "charts": {"tokens": ["inject", "source"]}}}}`,
		"a/a.css":               "a{}",
		"b/md2html-plugin.json": `{"format": 1, "name": "b", "styles": {"b": {"css": ["b.css"], "charts": {"tokens": ["source", "sink"]}}}}`,
		"b/b.css":               "b{}",
	})
	c := NewPluginCache()
	ps, err := c.LoadAll([]string{"a", "b"}, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := c.chartStyle([]selected{{ps[0], ps[0].Styles["a"]}, {ps[1], ps[1].Styles["b"]}})
	if strings.Join(got.Tokens, ",") != "inject,source,sink" {
		t.Errorf("tokens = %v", got.Tokens)
	}
}

func TestManifestChartsTokensRefusesBadNames(t *testing.T) {
	for _, c := range []struct{ name, tokens, want string }{
		{"shape", `["Inject"]`, "lowercase"},
		{"chart token", `["series-1"]`, "already a token"},
		{"page token", `["accent"]`, "already a token"},
		{"twice", `["a", "a"]`, "named twice"},
		{"not a list", `"a"`, "tokens"},
		{"too many", `["a1","a2","a3","a4","a5","a6","a7","a8","a9","a10","a11","a12","a13","a14","a15","a16","a17","a18","a19","a20","a21","a22","a23","a24","a25","a26","a27","a28","a29","a30","a31","a32","a33"]`, "the most is 32"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseManifest([]byte(`{"format": 1, "name": "a", "styles": {"s": {"css": ["x.css"], "charts": {"tokens": `+c.tokens+`}}}}`), "a", false, nil)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
}
