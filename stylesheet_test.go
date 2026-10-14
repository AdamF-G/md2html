package md2html

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/AdamF-G/md2html/internal/charts"
)

// twoPlugins builds plugins "acme" (styles "plain", "shared", "blank") and
// "zeta" (style "shared"), so a bare "shared" is ambiguous.
func twoPlugins(t *testing.T) (*PluginCache, []*Plugin) {
	t.Helper()
	root := pluginDir(t, map[string]string{
		"acme/md2html-plugin.json": `{"format": 1, "name": "acme", "styles": {
			"plain": {"css": ["plain.css"]},
			"shared": {"css": ["shared.css"]},
			"blank": {"base": "none", "css": ["blank.css"]}}}`,
		"acme/plain.css":           "/*plain*/",
		"acme/shared.css":          "/*acme-shared*/",
		"acme/blank.css":           "/*blank*/",
		"zeta/md2html-plugin.json": `{"format": 1, "name": "zeta", "styles": {"shared": {"css": ["s.css"]}}}`,
		"zeta/s.css":               "/*zeta-shared*/",
	})
	c := NewPluginCache()
	ps, err := c.LoadAll([]string{"acme", "zeta"}, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c, ps
}

func TestStylesheetStacksOnTheDefault(t *testing.T) {
	c, ps := twoPlugins(t)
	got, err := c.Stylesheet([]string{"plain", "zeta/shared"}, ps, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := defaultCSS + "\n/*plain*/\n/*zeta-shared*/"; got != want {
		t.Errorf("got tail %q", got[len(defaultCSS):])
	}
}

func TestStylesheetEmptyIsTheDefault(t *testing.T) {
	got, err := NewPluginCache().Stylesheet(nil, nil, nil)
	if err != nil || got != defaultCSS {
		t.Errorf("err %v; got default? %v", err, got == defaultCSS)
	}
	got, err = NewPluginCache().Stylesheet([]string{"default"}, nil, nil)
	if err != nil || got != defaultCSS {
		t.Errorf("style default: err %v; got default? %v", err, got == defaultCSS)
	}
}

func TestStylesheetBaseNoneFirstDropsTheDefault(t *testing.T) {
	c, ps := twoPlugins(t)
	got, err := c.Stylesheet([]string{"blank", "plain"}, ps, nil)
	if err != nil || got != "/*blank*/\n/*plain*/" {
		t.Errorf("got %q, err %v", got, err)
	}
}

func TestStylesheetRefuses(t *testing.T) {
	c, ps := twoPlugins(t)
	for _, c2 := range []struct {
		names []string
		want  string
	}{
		{[]string{"shared"}, `style "shared" is ambiguous; name one of: acme/shared, zeta/shared`},
		{[]string{"nope"}, `no style named "nope"; available:`},
		{[]string{"acme/nope"}, `plugin acme has no style "nope"`},
		{[]string{"nobody/plain"}, `no plugin named "nobody"`},
		{[]string{"plain", "blank"}, `style acme/blank starts from nothing (base: none), so it can only come first`},
	} {
		_, err := c.Stylesheet(c2.names, ps, nil)
		if err == nil || !strings.Contains(err.Error(), c2.want) {
			t.Errorf("%v: err = %v, want %q", c2.names, err, c2.want)
		}
	}
}

func TestStylesheetDuplicateUsedOnceWithAWarning(t *testing.T) {
	c, ps := twoPlugins(t)
	var warnings []string
	got, err := c.Stylesheet([]string{"plain", "acme/plain"}, ps, func(w string) { warnings = append(warnings, w) })
	if err != nil || got != defaultCSS+"\n/*plain*/" {
		t.Errorf("got tail %q, err %v", strings.TrimPrefix(got, defaultCSS), err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], `style acme/plain is selected twice; using it once`) {
		t.Errorf("warnings = %q", warnings)
	}
}

func TestStylesheetInlinesAssetsAndRefusesEscapes(t *testing.T) {
	root := pluginDir(t, map[string]string{
		"p/md2html-plugin.json": `{"format": 1, "name": "p", "styles": {
			"ok": {"css": ["css/ok.css"]}, "bad": {"css": ["../outside.css"]}}}`,
		"p/css/ok.css":    `@font-face{src:url(../fonts/f.woff2)}`,
		"p/fonts/f.woff2": "F",
		"outside.css":     "x",
	})
	c := NewPluginCache()
	p, err := c.Load("p", root, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Stylesheet([]string{"ok"}, []*Plugin{p}, nil)
	if err != nil || !strings.Contains(got, `url("data:font/woff2;base64,Rg==")`) {
		t.Errorf("got %q, err %v", strings.TrimPrefix(got, defaultCSS), err)
	}
	if _, err := c.Stylesheet([]string{"bad"}, []*Plugin{p}, nil); err == nil || !strings.Contains(err.Error(), "outside the plugin") {
		t.Errorf("css path outside the plugin: err = %v", err)
	}
}

// Many pages converted in parallel share one cache. Run with -race: the
// assembled sheet and the error for a broken style are each computed once
// and handed to every caller.
func TestStylesheetConcurrentSharedCache(t *testing.T) {
	root := pluginDir(t, map[string]string{
		"p/md2html-plugin.json": `{"format": 1, "name": "p", "styles": {
			"ok": {"css": ["ok.css"]}, "broken": {"css": ["missing.css"]}}}`,
		"p/ok.css": "/*ok*/",
	})
	c := NewPluginCache()
	var wg sync.WaitGroup
	errs := make([]error, 64)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p, err := c.Load("p", root, nil)
			if err != nil {
				errs[i] = err
				return
			}
			name := []string{"ok", "broken"}[i%2]
			_, errs[i] = c.Stylesheet([]string{name}, []*Plugin{p}, nil)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if (i%2 == 1) != (err != nil) {
			t.Errorf("call %d: err = %v", i, err)
		}
	}
	if fmt.Sprint(errs[1]) != fmt.Sprint(errs[3]) {
		t.Errorf("one broken style, two different errors: %v / %v", errs[1], errs[3])
	}
}

func TestSplitList(t *testing.T) {
	got := splitList(" a, b ,,c ")
	if strings.Join(got, "|") != "a|b|c" {
		t.Errorf("splitList = %q", got)
	}
	if splitList("") != nil {
		t.Error("empty should be nil")
	}
}

// convertPage converts src as the document at docPath and returns the
// page's <style> body and its warnings.
func convertPage(t *testing.T, src, docPath string, opt Options) (string, []string) {
	t.Helper()
	var warnings []string
	opt.SourcePath = docPath
	opt.Warn = func(w string) { warnings = append(warnings, w) }
	out, err := Convert([]byte(src), opt)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	start := strings.Index(s, "<style>\n") + len("<style>\n")
	end := strings.Index(s, "\n</style>")
	return s[start:end], warnings
}

func stylePlugins(t *testing.T) string {
	return pluginDir(t, map[string]string{
		"run/md2html-plugin.json":     `{"format": 1, "name": "run", "styles": {"r": {"css": ["r.css"]}}}`,
		"run/r.css":                   "/*run*/",
		"docs/_p/md2html-plugin.json": `{"format": 1, "name": "page", "styles": {"p": {"css": ["p.css"]}, "r": {"css": ["r2.css"]}}}`,
		"docs/_p/p.css":               "/*page*/",
		"docs/_p/r2.css":              "/*page-r*/",
	})
}

func TestConvertDefaultStyleUnchanged(t *testing.T) {
	css, w := convertPage(t, "# A\n", "/x/a.md", Options{})
	if css != defaultCSS || len(w) != 0 {
		t.Errorf("default changed; warnings %q", w)
	}
}

func TestConvertRunStyles(t *testing.T) {
	root := stylePlugins(t)
	css, _ := convertPage(t, "# A\n", filepath.Join(root, "docs/a.md"),
		Options{Plugins: []string{filepath.Join(root, "run")}, Styles: []string{"r"}})
	if css != defaultCSS+"\n/*run*/" {
		t.Errorf("tail %q", strings.TrimPrefix(css, defaultCSS))
	}
}

func TestConvertFrontMatterStyleReplacesRunStyles(t *testing.T) {
	root := stylePlugins(t)
	src := "---\nplugins: ./_p\nstyle: p\n---\n# A\n"
	css, w := convertPage(t, src, filepath.Join(root, "docs/a.md"),
		Options{Plugins: []string{filepath.Join(root, "run")}, Styles: []string{"run/r"}})
	if css != defaultCSS+"\n/*page*/" || len(w) != 0 {
		t.Errorf("tail %q, warnings %q", strings.TrimPrefix(css, defaultCSS), w)
	}
}

func TestConvertFrontMatterStyleReplacesCSS(t *testing.T) {
	css, _ := convertPage(t, "---\nstyle: default\n---\n# A\n", "/x/a.md", Options{CSS: "/*mine*/"})
	if css != defaultCSS {
		t.Errorf("style: default should beat --css, got %q", css)
	}
}

func TestConvertBadFrontMatterFallsBackWhole(t *testing.T) {
	root := stylePlugins(t)
	src := "---\nplugins: ./_p\nstyle: p, nope\n---\n# A\n"
	css, w := convertPage(t, src, filepath.Join(root, "docs/a.md"),
		Options{Plugins: []string{filepath.Join(root, "run")}, Styles: []string{"run/r"}})
	if css != defaultCSS+"\n/*run*/" {
		t.Errorf("want the run's whole list, got tail %q", strings.TrimPrefix(css, defaultCSS))
	}
	if len(w) != 1 || !strings.HasPrefix(w[0], `front matter style: no style named "nope"`) ||
		!strings.HasSuffix(w[0], "; using the run's style") {
		t.Errorf("warnings = %q", w)
	}
}

func TestConvertBadFrontMatterPluginsFallsBack(t *testing.T) {
	css, w := convertPage(t, "---\nplugins: ./missing\nstyle: x\n---\n# A\n", "/nowhere/a.md", Options{})
	if css != defaultCSS || len(w) != 1 || !strings.HasPrefix(w[0], "front matter plugins: plugin ./missing:") {
		t.Errorf("warnings = %q", w)
	}
}

// A page that loads a plugin but does not choose a style must keep the
// run's styles exactly, even when the page's plugin would make the run's
// bare name ambiguous: the page's plugins only widen what its own style:
// can name, so loading one alone must not change or break its look.
func TestPagePluginsWithoutStyleWarnAndKeepRunStyles(t *testing.T) {
	root := stylePlugins(t)
	css, w := convertPage(t, "---\nplugins: ./_p\n---\n# A\n", filepath.Join(root, "docs/a.md"),
		Options{Plugins: []string{filepath.Join(root, "run")}, Styles: []string{"r"}})
	if css != defaultCSS+"\n/*run*/" {
		t.Errorf("tail %q", strings.TrimPrefix(css, defaultCSS))
	}
	if len(w) != 1 || w[0] != "front matter plugins has no effect without style; ignoring it" {
		t.Errorf("warnings = %q", w)
	}
}

func TestConvertStylesAndCSSTogetherIsAnError(t *testing.T) {
	_, err := Convert([]byte("# A\n"), Options{CSS: "x", Styles: []string{"default"}})
	if err == nil || !strings.Contains(err.Error(), "CSS and Styles") {
		t.Errorf("err = %v", err)
	}
}

func TestConvertWarnsOnAHugeStylesheet(t *testing.T) {
	_, w := convertPage(t, "# A\n", "/x/a.md", Options{CSS: strings.Repeat("a", 1<<20+1)})
	if len(w) != 1 || !strings.Contains(w[0], "over 1 MB; every page carries it") {
		t.Errorf("warnings = %q", w)
	}
}

func TestConvertFragmentCarriesTheStyle(t *testing.T) {
	root := stylePlugins(t)
	out, err := Convert([]byte("# A\n"), Options{Fragment: true, SourcePath: filepath.Join(root, "docs/a.md"),
		Plugins: []string{filepath.Join(root, "run")}, Styles: []string{"r"}})
	if err != nil || !strings.Contains(string(out), "/*run*/") {
		t.Errorf("fragment lost the style (err %v)", err)
	}
}

// blockBody returns the declarations of one block, by the block's exact
// selector, and whether the CSS has such a block.
func blockBody(css, selector string) (string, bool) {
	i := strings.Index(css, selector+" {")
	if i < 0 {
		return "", false
	}
	body := css[i+len(selector)+2:]
	return body[:strings.Index(body, "}")], true
}

// themeBlock returns the custom properties one block defines, by the
// block's exact selector, or nil if the CSS has no such block.
func themeBlock(css, selector string) map[string]bool {
	body, ok := blockBody(css, selector)
	if !ok {
		return nil
	}
	got := map[string]bool{}
	for _, m := range regexp.MustCompile(`(--[a-z0-9-]+)\s*:`).FindAllStringSubmatch(body, -1) {
		got[m[1]] = true
	}
	return got
}

// Every built-in style must load without a warning, fit its budget, and
// keep the three theme states consistent. That means the two dark blocks
// and the explicit light block define the same tokens, each first defined
// on bare :root.
//
// The light block is the trap: the default stylesheet has
// :root[data-theme="light"], which outranks bare :root, so a style layered
// on it that set its light palette on :root alone would show the default's
// palette on any page stamped light.
func TestEveryBuiltinStyleIsWellFormed(t *testing.T) {
	for _, p := range BuiltinPlugins() {
		for _, n := range p.StyleNames() {
			t.Run(p.Name+"/"+n, func(t *testing.T) {
				var warnings []string
				sheet, err := NewPluginCache().Stylesheet([]string{p.Name + "/" + n}, nil,
					func(w string) { warnings = append(warnings, w) })
				if err != nil || len(warnings) > 0 {
					t.Fatalf("err %v, warnings %q", err, warnings)
				}
				if len(sheet) > maxStylesheet {
					t.Errorf("stylesheet is %d bytes", len(sheet))
				}
				var own strings.Builder
				for _, f := range p.Styles[n].css {
					b, err := p.readFile(f)
					if err != nil {
						t.Fatal(err)
					}
					own.Write(b)
				}
				css := own.String()
				root := themeBlock(css, ":root")
				media := themeBlock(css, `  :root:not([data-theme="light"])`)
				dark := themeBlock(css, `:root[data-theme="dark"]`)
				light := themeBlock(css, `:root[data-theme="light"]`)
				if root == nil || media == nil || dark == nil || light == nil {
					t.Fatalf("want :root, the dark media block, [data-theme=dark] and [data-theme=light]; got %v %v %v %v",
						root != nil, media != nil, dark != nil, light != nil)
				}
				// Both dark blocks must say color-scheme: dark, or form controls
				// and scrollbars stay light on a dark page. The default style is
				// exempt: its output must stay byte-identical to v0.9.0, which
				// never set it.
				if p.Name+"/"+n != "default/default" {
					for _, sel := range []string{`  :root:not([data-theme="light"])`, `:root[data-theme="dark"]`} {
						if body, _ := blockBody(css, sel); !regexp.MustCompile(`color-scheme:\s*dark`).MatchString(body) {
							t.Errorf("%s does not set color-scheme: dark", strings.TrimSpace(sel))
						}
					}
				}
				for name, set := range map[string]map[string]bool{"dark media": media, "dark": dark, "light": light} {
					if !reflect.DeepEqual(set, dark) {
						t.Errorf("%s block defines %v, [data-theme=dark] defines %v", name, keys(set), keys(dark))
					}
					for tok := range set {
						if !root[tok] {
							t.Errorf("%s is set in the %s block but not first on bare :root", tok, name)
						}
					}
				}
			})
		}
	}
}

func keys(m map[string]bool) []string { return sortedJSONKeys(m) }

func TestClaudeArtifactFontBudget(t *testing.T) {
	p := findPlugin(BuiltinPlugins(), "claude-artifact")
	if p == nil {
		t.Fatal("no claude-artifact plugin")
	}
	total := 0
	fs.WalkDir(p.fsys, "fonts", func(path string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(path, ".woff2") {
			b, _ := fs.ReadFile(p.fsys, path)
			total += len(b)
		}
		return nil
	})
	if total == 0 || total > 150*1024 {
		t.Errorf("fonts total %d bytes, want 1..%d", total, 150*1024)
	}
}

// Each CSS file is checked alone, but the browser reads them joined: a file
// that ends inside an open image-set( turns the next file's strings into
// URLs. The joined stylesheet is checked again, so no split across files
// carries a fetch past the check.
func TestStylesheetChecksTheJoinedFiles(t *testing.T) {
	for _, c := range []struct{ name, second, want string }{
		{"remote", `"http://h/x.png" 1x)}`, "is not inline"},
		{"plugin file", `"i.png" 1x)}`, "is not inline"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := pluginDir(t, map[string]string{
				"p/md2html-plugin.json": `{"format": 1, "name": "p", "styles": {"s": {"css": ["a.css", "b.css"]}}}`,
				"p/a.css":               `a{b:image-set(`,
				"p/b.css":               c.second,
				"p/i.png":               "PNG",
			})
			c2 := NewPluginCache()
			p, err := c2.Load("p", root, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = c2.Stylesheet([]string{"s"}, []*Plugin{p}, nil)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

// Every chart token is defined in every built-in style, on bare :root and
// in both theme blocks, so a page renders charts in every theme state.
func TestBuiltinStylesDefineEveryChartToken(t *testing.T) {
	for _, style := range []string{"default", "claude-artifact"} {
		css, err := NewPluginCache().Stylesheet([]string{style}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		light, dark, ok := charts.TokenValues(css)
		if !ok {
			t.Fatalf("%s defines no chart tokens", style)
		}
		for _, tok := range charts.Tokens {
			if light[tok] == "" || dark[tok] == "" {
				t.Errorf("%s: --%s missing in light (%q) or dark (%q)", style, tok, light[tok], dark[tok])
			}
		}
	}
}

// The two built-ins' series palettes differ (decision 9).
func TestDefaultAndClaudeArtifactSeriesDiffer(t *testing.T) {
	values := func(style string) map[string]string {
		css, err := NewPluginCache().Stylesheet([]string{style}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		light, _, _ := charts.TokenValues(css)
		return light
	}
	d, a := values("default"), values("claude-artifact")
	same := 0
	for n := 1; n <= 8; n++ {
		k := fmt.Sprintf("series-%d", n)
		if d[k] == a[k] {
			same++
		}
	}
	if same == 8 {
		t.Error("default's series palette is claude-artifact's")
	}
}

// The chart rules live in default.css, so every style built on it has them.
func TestDefaultCSSHasTheChartRules(t *testing.T) {
	css, err := NewPluginCache().Stylesheet([]string{"default"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"figure.md2html-chart {", ".md2html-tip {", ".md2html-chart-dark {",
		`[class*="md2html-legend-s"] * { pointer-events: auto; }`,
		`[class*="md2html-legend-s"]:focus-visible { outline`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("default stylesheet lacks %q", want)
		}
	}
	for n := 1; n <= 8; n++ {
		want := fmt.Sprintf("figure.md2html-chart:has(.md2html-legend-s%[1]d:hover, .md2html-legend-s%[1]d:focus-visible) .md2html-mark:not(.md2html-s%[1]d) { opacity: 0.25; }", n)
		if !strings.Contains(css, want) {
			t.Errorf("default stylesheet lacks the dimming rule for series %d", n)
		}
	}
}

// StyleChosen hears the style list each page is drawn in, by qualified
// name, and never the default chosen by default or a run's CSS.
func TestStyleChosenNamesThePageStyle(t *testing.T) {
	type call struct {
		names, dirs []string
		css         string
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
		c.opt.StyleChosen = func(names, dirs []string, css string) { got = append(got, call{names, dirs, css}) }
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
			if len(got[0].dirs) != 1 || got[0].dirs[0] != "" {
				t.Errorf("%s: dirs %q, want one empty dir for a built-in", c.name, got[0].dirs)
			}
			if !strings.HasPrefix(got[0].css, defaultCSS) || !strings.Contains(got[0].css, "IBM Plex Sans") {
				t.Errorf("%s: css is not claude-artifact stacked on the default", c.name)
			}
		}
	}
}

// StyleChosen gets each style's plugin dir beside its name: "" for a
// built-in, the real dir for a plugin loaded from disk.
func TestStyleChosenGivesPluginDirs(t *testing.T) {
	root := stylePlugins(t)
	var names, dirs []string
	opt := Options{SourcePath: filepath.Join(root, "docs/a.md"),
		StyleChosen: func(n, d []string, _ string) { names, dirs = n, d }}
	if _, err := Convert([]byte("---\nplugins: ./_p\nstyle: claude-artifact, p\n---\n# A\n"), opt); err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "claude-artifact/claude-artifact,page/p" {
		t.Errorf("names %v", names)
	}
	if len(dirs) != 2 || dirs[0] != "" || dirs[1] != filepath.Join(root, "docs/_p") {
		t.Errorf("dirs %q", dirs)
	}
}

// An explicit style: default is a choice, so the hook hears it.
func TestStyleChosenHearsExplicitDefault(t *testing.T) {
	var names, dirs []string
	calls := 0
	opt := Options{StyleChosen: func(n, d []string, _ string) { names, dirs = n, d; calls++ }}
	if _, err := Convert([]byte("---\nstyle: default\n---\n# A\n"), opt); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || strings.Join(names, ",") != "default/default" || len(dirs) != 1 || dirs[0] != "" {
		t.Errorf("%d calls, names %v, dirs %q", calls, names, dirs)
	}
}

// A run whose own style fails is an error, and nothing was chosen.
func TestStyleChosenNotCalledWhenRunStyleFails(t *testing.T) {
	called := false
	opt := Options{Styles: []string{"nosuch"}, StyleChosen: func(_, _ []string, _ string) { called = true }}
	if _, err := Convert([]byte("# A\n"), opt); err == nil {
		t.Error("no error for a style that does not exist")
	}
	if called {
		t.Error("hook called for a style that failed")
	}
}

// Pages converting in parallel share one hook; run with -race.
func TestStyleChosenConcurrent(t *testing.T) {
	var mu sync.Mutex
	var got []string
	cache := NewPluginCache()
	hook := func(n, _ []string, _ string) {
		mu.Lock()
		got = append(got, strings.Join(n, ","))
		mu.Unlock()
	}
	var wg sync.WaitGroup
	errs := make([]error, 20)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			src := "# A\n"
			if i%2 == 1 {
				src = "---\nstyle: default\n---\n# A\n"
			}
			_, errs[i] = Convert([]byte(src), Options{Styles: []string{"claude-artifact"}, PluginCache: cache, StyleChosen: hook})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("page %d: %v", i, err)
		}
	}
	count := map[string]int{}
	for _, n := range got {
		count[n]++
	}
	if len(got) != 20 || count["claude-artifact/claude-artifact"] != 10 || count["default/default"] != 10 {
		t.Errorf("calls %v", count)
	}
}
