package md2html

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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

// Review Focus 4: a page that loads a plugin but does not choose a style
// must keep the run's styles exactly, even when the page's plugin would
// make the run's bare name ambiguous.
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
