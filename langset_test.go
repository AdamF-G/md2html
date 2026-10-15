package md2html

import (
	"strings"
	"testing"
	"testing/fstest"
)

func housePlugin(t *testing.T, name, manifestGrammars string, files map[string]string) *Plugin {
	t.Helper()
	all := map[string]string{name + "/md2html-plugin.json": `{"format": 1, "name": "` + name + `", "grammars": ` + manifestGrammars + `}`}
	for k, v := range files {
		all[name+"/"+k] = v
	}
	p, err := NewPluginCache().Load(name, pluginDir(t, all), nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func fakeBuiltin() *Plugin {
	p := &Plugin{Name: "grammars", key: "builtin:grammars", Styles: map[string]*Style{},
		fsys: fstest.MapFS{"go.json": {Data: []byte(`{"scopeName": "source.go", "patterns": [{"match": "func", "name": "keyword.go"}]}`)}}}
	p.Grammars = map[string]*Grammar{"go": {Name: "go", Aliases: []string{"golang"}, Scope: "source.go",
		License: "MIT", Source: "https://example.org/go", plugin: p, file: "go.json"}}
	return p
}

func TestLookupByNameAndAliasIgnoringCase(t *testing.T) {
	p := housePlugin(t, "house", `{"fabcfg": {"file": "g.json", "aliases": ["fab"]}}`, map[string]string{"g.json": fabGrammar})
	ls := NewPluginCache().languages([]*Plugin{p}, nil)
	for _, n := range []string{"fabcfg", "FAB", "Fab"} {
		if g := ls.lookup(n); g == nil || g.Name != "fabcfg" {
			t.Errorf("lookup(%q) = %v", n, g)
		}
	}
	if ls.lookup("text") != nil {
		t.Error("lookup(text) found something")
	}
}

func TestLocalOverridesBuiltinSilently(t *testing.T) {
	g := `{"scopeName": "source.go", "patterns": [{"match": "x", "name": "keyword.x"}]}`
	for name, manifest := range map[string]string{
		"by name":  `{"go": {"file": "g.json"}}`,
		"by alias": `{"mygo": {"file": "g.json", "aliases": ["golang"]}}`,
		"by scope": `{"mygo": {"file": "g.json"}}`,
	} {
		local := housePlugin(t, "house", manifest, map[string]string{"g.json": g})
		var w []string
		ls := NewPluginCache().languages([]*Plugin{fakeBuiltin(), local}, func(s string) { w = append(w, s) })
		if len(w) != 0 {
			t.Errorf("%s: warnings %q", name, w)
		}
		got := ls.byScope["source.go"]
		if got == nil || got.plugin != local {
			t.Errorf("%s: source.go served by %v, want the local plugin", name, got)
		}
	}
}

func TestTwoLocalPluginsClashWarnsAndLaterWins(t *testing.T) {
	a := housePlugin(t, "a", `{"fab": {"file": "g.json"}}`, map[string]string{"g.json": fabGrammar})
	b := housePlugin(t, "b", `{"fab": {"file": "g.json"}}`, map[string]string{"g.json": fabGrammar})
	var w []string
	ls := NewPluginCache().languages([]*Plugin{a, b}, func(s string) { w = append(w, s) })
	if ls.lookup("fab").plugin != b {
		t.Error("the earlier plugin won")
	}
	if len(w) != 1 || !strings.Contains(w[0], `both provide language "fab"; using`) {
		t.Errorf("warnings %q", w)
	}
}

// A grammar may include another by scope name, from any plugin.
func TestCrossPluginInclude(t *testing.T) {
	inner := housePlugin(t, "inner", `{"inner": {"file": "g.json"}}`, map[string]string{
		"g.json": `{"scopeName": "source.inner", "patterns": [{"match": "zap", "name": "keyword.inner"}]}`})
	outer := housePlugin(t, "outer", `{"outer": {"file": "g.json"}}`, map[string]string{
		"g.json": `{"scopeName": "source.outer", "patterns": [{"include": "source.inner"}]}`})
	ls := NewPluginCache().languages([]*Plugin{inner, outer}, nil)
	toks, err := ls.engine.Tokenize("source.outer", []string{"zap"})
	if err != nil {
		t.Fatal(err)
	}
	if s := toks[0][0].Scopes; s[len(s)-1] != "keyword.inner" {
		t.Errorf("scopes %v, want the included grammar's keyword", s)
	}
}

func TestNearMiss(t *testing.T) {
	p := housePlugin(t, "house", `{"python": {"file": "g.json", "aliases": ["py"]}, "shellscript": {"file": "g.json", "aliases": ["bash"]}, "bat": {"file": "g.json", "aliases": ["batch"]}}`,
		map[string]string{"g.json": fabGrammar})
	ls := NewPluginCache().languages([]*Plugin{p}, nil)
	ls.leftOut = map[string]bool{"pythen": true}
	for in, want := range map[string]string{
		"pyhton":      "python", // a transposition is one edit
		"Pyton":       "python", // case ignored
		"bahs":        "bash",
		"shellscirpt": "shellscript", // 8 or more runes may be 2 away
		"pyt":         "",            // under four letters
		"text":        "",            // means "no language"
		"tree":        "",            // means "no language"
		"pythn":       "python",
		"pyth":        "", // 2 away at 4 runes
		"bashh":       "", // the suggestion would be shorter
		"console":     "",
		"python":      "", // an exact match is not a miss
		"pythen":      "", // a real language left out of the built-ins
		"patch":       "", // a real language the collection lacks
		"batc":        "batch",
	} {
		if got := ls.nearMiss(in); got != want {
			t.Errorf("nearMiss(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLanguageSetsAreCachedByPluginList(t *testing.T) {
	p := housePlugin(t, "house", `{"fab": {"file": "g.json"}}`, map[string]string{"g.json": fabGrammar})
	c := NewPluginCache()
	if c.languages([]*Plugin{p}, nil) != c.languages([]*Plugin{p, p}, nil) {
		t.Error("one plugin listed twice built a second set")
	}
}

func TestLanguagesListsWhatAFenceCanName(t *testing.T) {
	local := housePlugin(t, "house", `{"go": {"file": "g.json"}}`, map[string]string{"g.json": fabGrammar})
	// From Task 5 on, the real built-in grammars are in the set too, so
	// look only at go.
	var gos []*Grammar
	for _, g := range languagesOf(NewPluginCache().languages([]*Plugin{fakeBuiltin(), local}, nil)) {
		if g.Name == "go" {
			gos = append(gos, g)
		}
	}
	if len(gos) != 1 || gos[0].plugin != local {
		t.Errorf("go listed as %v, want only the local one", gos)
	}
}
