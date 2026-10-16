package md2html

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/AdamF-G/md2html/internal/highlight"
)

// builtinMarkdown is the built-in Markdown grammar's file.
func builtinMarkdown(t *testing.T) []byte {
	t.Helper()
	ls := NewPluginCache().languages(nil, nil)
	raw, err := ls.byScope[markdownScope].raw()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// conformanceDump reads a built-in grammar's conformance golden.
func conformanceDump(t *testing.T, name string) highlight.Dump {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("internal", "highlight", "testdata", "conformance", name+".json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	b, err = io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	var d highlight.Dump
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestHostGrammarDropsTheEmbeddedRules(t *testing.T) {
	host := hostGrammar(builtinMarkdown(t))
	if host == nil {
		t.Fatal("no host grammar from the built-in Markdown grammar")
	}
	var g struct {
		ScopeName  string                     `json:"scopeName"`
		Repository map[string]json.RawMessage `json:"repository"`
	}
	if err := json.Unmarshal(host, &g); err != nil {
		t.Fatal(err)
	}
	if g.ScopeName != hostScope {
		t.Errorf("scope %q", g.ScopeName)
	}
	if got := string(g.Repository["fenced_code_block"]); got != `{"patterns":[{"include":"#fenced_code_block_unknown"}]}` {
		t.Errorf("fenced_code_block %s", got)
	}
	for k := range g.Repository {
		if strings.HasPrefix(k, "fenced_code_block") && k != "fenced_code_block" && k != "fenced_code_block_unknown" {
			t.Errorf("repository keeps %s", k)
		}
	}
	for _, m := range regexp.MustCompile(`"include":"([^"#$][^"#]*)`).FindAllSubmatch(host, -1) {
		if s := string(m[1]); s != "text.html.derivative" && s != "source.yaml" {
			t.Errorf("the host still includes %s", s)
		}
	}
}

func TestHostGrammarNeedsTheShape(t *testing.T) {
	for name, raw := range map[string]string{
		"no repository": fabGrammar,
		"not JSON":      `{`,
		"no unknown rule last": `{"scopeName": "text.html.markdown", "patterns": [], "repository": {
			"fenced_code_block": {"patterns": [{"include": "#fenced_code_block_go"}]}, "fenced_code_block_go": {}}}`,
		"a removed rule still named": `{"scopeName": "text.html.markdown", "patterns": [{"include": "#fenced_code_block_go"}], "repository": {
			"fenced_code_block": {"patterns": [{"include": "#fenced_code_block_go"}, {"include": "#fenced_code_block_unknown"}]},
			"fenced_code_block_go": {}, "fenced_code_block_unknown": {}}}`,
		"a removed rule named by scope": `{"scopeName": "text.html.markdown", "patterns": [{"include": "text.html.markdown#fenced_code_block_go"}], "repository": {
			"fenced_code_block": {"patterns": [{"include": "#fenced_code_block_go"}, {"include": "#fenced_code_block_unknown"}]},
			"fenced_code_block_go": {}, "fenced_code_block_unknown": {}}}`,
	} {
		if hostGrammar([]byte(raw)) != nil {
			t.Errorf("%s: a host grammar was derived", name)
		}
	}
}

// outsideInner reports, per line, whether it is outside every inner
// fence's body.
func outsideInner(lines []string) []bool {
	out := make([]bool, len(lines))
	for i := range out {
		out[i] = true
	}
	for _, f := range innerFences(lines) {
		for _, b := range f.body {
			out[b.line] = false
		}
	}
	return out
}

// The host grammar is the full one but for inner fences' bodies.
func TestHostMatchesMarkdownOutsideInnerBodies(t *testing.T) {
	ls := NewPluginCache().languages(nil, nil)
	if ls.markdownHost() != hostScope {
		t.Fatal("no host grammar")
	}
	lines := conformanceDump(t, "markdown").Lines
	full, err := ls.engine.Tokenize(markdownScope, lines)
	if err != nil {
		t.Fatal(err)
	}
	host, err := ls.engine.Tokenize(hostScope, lines)
	if err != nil {
		t.Fatal(err)
	}
	compared := 0
	for i, outside := range outsideInner(lines) {
		if !outside {
			continue
		}
		compared++
		if f, h := highlight.Spans(full[i]), highlight.Spans(host[i]); !slices.Equal(f, h) {
			t.Errorf("line %d %q:\nfull %v\nhost %v", i+1, lines[i], f, h)
		}
	}
	if compared < 150 {
		t.Errorf("only %d lines compared", compared)
	}
}

func TestPluginMarkdownWithoutTheShapeIsUsedAsIs(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"md/md2html-plugin.json": `{"format": 1, "name": "md", "grammars": {"markdown": {"file": "g.json"}}}`,
		"md/g.json":              `{"scopeName": "text.html.markdown", "patterns": [{"match": "^#.*", "name": "markup.heading.x"}]}`,
	})
	opt := Options{Plugins: []string{filepath.Join(dir, "md")}, PluginCache: NewPluginCache()}
	out, w := convertBody(t, "````markdown\n# t\n````\n", opt)
	if len(w) != 0 {
		t.Errorf("warnings %q", w)
	}
	if !strings.Contains(out, `<span class="hl-heading"># t</span>`) {
		t.Errorf("not highlighted:\n%s", out)
	}
	run, err := opt.PluginCache.LoadAll(opt.Plugins, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if s := opt.PluginCache.languages(run, nil).markdownHost(); s != markdownScope {
		t.Errorf("host %q for a grammar without the shape", s)
	}
}
