package md2html

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

const fabGrammar = `{"scopeName": "source.fab", "patterns": [{"match": "\\bfab\\b", "name": "keyword.fab"}]}`

const fabManifest = `{"format": 1, "name": "house", "grammars": {
  "fabcfg": {"file": "grammars/fab.json", "aliases": ["fab", "c++", "f#", "objective-c", "x.y"]}}}`

func TestGrammarOnlyPluginLoads(t *testing.T) {
	root := pluginDir(t, map[string]string{
		"p/md2html-plugin.json": fabManifest,
		"p/grammars/fab.json":   fabGrammar,
	})
	p, err := NewPluginCache().Load("p", root, nil)
	if err != nil {
		t.Fatal(err)
	}
	g := p.Grammars["fabcfg"]
	if g == nil || g.Scope != "source.fab" || len(g.Aliases) != 5 || g.PluginName() != "house" {
		t.Fatalf("grammar %+v", g)
	}
	if len(p.Styles) != 0 {
		t.Errorf("styles %v", p.Styles)
	}
}

func TestGzippedGrammarLoads(t *testing.T) {
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	zw.Write([]byte(fabGrammar))
	zw.Close()
	root := pluginDir(t, map[string]string{
		"p/md2html-plugin.json": `{"format": 1, "name": "house", "grammars": {"fabcfg": {"file": "fab.json.gz"}}}`,
		"p/fab.json.gz":         b.String(),
	})
	p, err := NewPluginCache().Load("p", root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Grammars["fabcfg"].Scope != "source.fab" {
		t.Errorf("scope %q", p.Grammars["fabcfg"].Scope)
	}
}

func TestGrammarManifestErrors(t *testing.T) {
	for _, c := range []struct{ name, manifest, file, want string }{
		{"no file", `{"format": 1, "name": "h", "grammars": {"x": {}}}`, "", `grammar "x" has no file`},
		{"bad name", `{"format": 1, "name": "h", "grammars": {"X Y": {"file": "g.json"}}}`, fabGrammar, `language name "X Y"`},
		{"bad alias", `{"format": 1, "name": "h", "grammars": {"x": {"file": "g.json", "aliases": ["9x"]}}}`, fabGrammar, `alias "9x"`},
		{"missing file", `{"format": 1, "name": "h", "grammars": {"x": {"file": "nope.json"}}}`, "", "nope.json"},
		{"not json", `{"format": 1, "name": "h", "grammars": {"x": {"file": "g.json"}}}`, `{`, "not valid JSON"},
		{"no scope", `{"format": 1, "name": "h", "grammars": {"x": {"file": "g.json"}}}`, `{"patterns": [{}]}`, "no scopeName"},
		{"no patterns", `{"format": 1, "name": "h", "grammars": {"x": {"file": "g.json"}}}`, `{"scopeName": "source.x"}`, "no patterns"},
		{"scope mismatch", `{"format": 1, "name": "h", "grammars": {"x": {"file": "g.json", "scope": "source.y"}}}`, fabGrammar, `scope "source.y"`},
		{"outside", `{"format": 1, "name": "h", "grammars": {"x": {"file": "../g.json"}}}`, "", "outside the plugin"},
		{"empty grammars", `{"format": 1, "name": "h", "grammars": {}}`, "", "provides nothing"},
	} {
		files := map[string]string{"p/md2html-plugin.json": c.manifest}
		if c.file != "" {
			files["p/g.json"] = c.file
		}
		_, err := NewPluginCache().Load("p", pluginDir(t, files), nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err %v, want it to mention %q", c.name, err, c.want)
		}
	}
}

func TestUnknownGrammarKeyWarns(t *testing.T) {
	root := pluginDir(t, map[string]string{
		"p/md2html-plugin.json": `{"format": 1, "name": "h", "grammars": {"x": {"file": "g.json", "colour": "red"}}}`,
		"p/g.json":              fabGrammar,
	})
	var w []string
	if _, err := NewPluginCache().Load("p", root, func(s string) { w = append(w, s) }); err != nil {
		t.Fatal(err)
	}
	if len(w) != 1 || !strings.Contains(w[0], `unknown key "colour" in grammar "x"`) {
		t.Errorf("warnings %q", w)
	}
}
