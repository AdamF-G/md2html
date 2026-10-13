package md2html

import (
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
