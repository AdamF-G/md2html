package md2html

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AdamF-G/md2html/internal/highlight"
)

// slowSet is the built-in language set with a budget that load cannot
// reach: these tests check what is coloured, not how fast, and -race is
// slow.
func slowSet() *langSet {
	ls := buildLangSet(builtinGrammarPlugins(), func(string) {})
	ls.engine = highlight.NewTextMate(ls.load, highlight.Options{Budget: time.Minute})
	return ls
}

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
	ls := slowSet()
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

// md wraps body in a four-backtick Markdown fence.
func md(body string) string { return "````markdown\n" + body + "````\n" }

func TestInnerFenceIsHighlighted(t *testing.T) {
	out, w := convertBody(t, md("# T\n\n```go\nfunc main() {}\n```\n"), Options{})
	if len(w) != 0 {
		t.Errorf("warnings %q", w)
	}
	for _, want := range []string{`<span class="hl-heading"># T</span>`, `<span class="hl-keyword">func</span>`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
}

func TestInnerFenceAnyLanguage(t *testing.T) {
	dir := writeTree(t, fabPlugin("langs", "keyword.fab"))
	opt := Options{Plugins: []string{filepath.Join(dir, "langs")}, PluginCache: NewPluginCache()}
	for _, c := range []struct{ fence, want string }{
		{"```zig\nconst x = 1;\n```\n", `<span class="hl-keyword">const</span>`},
		{"```fab\nfab x\n```\n", `<span class="hl-keyword">fab</span>`},
		{"```py\ndef f(): pass\n```\n", `<span class="hl-keyword">def</span>`},
	} {
		out, w := convertBody(t, md(c.fence), opt)
		if len(w) != 0 {
			t.Errorf("%q: warnings %q", c.fence, w)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("%q: missing %s in\n%s", c.fence, c.want, out)
		}
	}
}

// keepsText checks that highlighting src added only spans and the class.
func keepsText(t *testing.T, src string) string {
	t.Helper()
	hl, w := convertBody(t, src, Options{})
	if len(w) != 0 {
		t.Errorf("%q: warnings %q", src, w)
	}
	plain, _ := convertBody(t, src, Options{NoHighlight: true})
	hlBody, plain := styleBlock.ReplaceAllString(hl, ""), styleBlock.ReplaceAllString(plain, "")
	if stripped := strings.Replace(spanTags.ReplaceAllString(hlBody, ""), " md2html-hl", "", 1); stripped != plain {
		t.Errorf("%q: text changed:\n got %s\nwant %s", src, stripped, plain)
	}
	return hl
}

func TestInnerFencePlacement(t *testing.T) {
	for _, body := range []string{
		"> quoted:\n>\n> ```go\n> func x() {}\n> ```\n",
		"- item\n\n  ```go\n  func x() {}\n  ```\n",
		"~~~go\nfunc x() {}\n~~~\n",
		"text\n\n```go\nfunc x() {}\nfunc y() {}\n",
		"```{.go}\nfunc x() {}\n```\n",
		"> ```go\n> func x() {}\n> ```\n\n- ```go\n  func y() {}\n  ```\n",
	} {
		out := keepsText(t, md(body))
		if !strings.Contains(out, `<span class="hl-keyword">func</span> <span class="hl-function">x</span>`) {
			t.Errorf("%q: func x not highlighted:\n%s", body, out)
		}
	}
	out := keepsText(t, md("# T\n\n```{.go .no-highlight}\nfunc x() {}\n```\n"))
	if strings.Contains(styleBlock.ReplaceAllString(out, ""), "hl-keyword") {
		t.Errorf("no-highlight inner fence coloured:\n%s", out)
	}
}

// A tab after a list item's indent is part of the code, not padding.
func TestInnerFenceTab(t *testing.T) {
	out := keepsText(t, md("- ```go\n  \tfunc x() {}\n  ```\n"))
	if !strings.Contains(out, "\t<span class=\"hl-keyword\">func</span>") {
		t.Errorf("tab and keyword not kept apart:\n%s", out)
	}
}

func TestInnerFenceRecursion(t *testing.T) {
	out := keepsText(t, "`````markdown\n````markdown\n```go\nfunc x() {}\n```\n````\n`````\n")
	if !strings.Contains(out, `<span class="hl-keyword">func</span>`) {
		t.Errorf("not highlighted:\n%s", out)
	}
}

func TestInnerFenceUnknownIsSilent(t *testing.T) {
	out := keepsText(t, md("# T\n\n```pyhton\ndef f(): pass\n```\n"))
	if strings.Contains(styleBlock.ReplaceAllString(out, ""), "hl-keyword") {
		t.Errorf("unknown inner fence coloured:\n%s", out)
	}
}

func TestInnerFenceOverBudget(t *testing.T) {
	hl := highlightContext{langs: NewPluginCache().languages(nil, nil), budget: 1, budgetScope: "source.go"}
	var w []string
	spans := hl.spans("markdown", nil, []string{"# T", "", "```go", "func x() {}", "```"}, 10, func(s string) { w = append(w, s) })
	if len(w) != 1 || w[0] != `fence at line 13: highlighting "go" took over 2s; showing it plain` {
		t.Errorf("warnings %q", w)
	}
	if len(spans) != 5 || len(spans[0]) != 1 || spans[0][0].Kind != highlight.Heading || len(spans[3]) != 0 {
		t.Errorf("spans %v", spans)
	}
}

func TestInnerFenceLoadsOnlyWhatItUses(t *testing.T) {
	ls := buildLangSet(builtinGrammarPlugins(), func(string) {})
	var mu sync.Mutex
	asked := map[string]bool{}
	ls.engine = highlight.NewTextMate(func(s string) ([]byte, error) {
		mu.Lock()
		asked[s] = true
		mu.Unlock()
		return ls.load(s)
	}, highlight.Options{})
	hl := highlightContext{langs: ls}
	if hl.spans("markdown", nil, []string{"# T", "```go", "func x() {}", "```"}, 1, func(string) {}) == nil {
		t.Fatal("not highlighted")
	}
	for _, s := range []string{hostScope, "source.go"} {
		if !asked[s] {
			t.Errorf("%s not loaded", s)
		}
	}
	for _, s := range []string{markdownScope, "source.rust", "source.python", "source.cpp", "source.java", "source.ruby", "source.php"} {
		if asked[s] {
			t.Errorf("%s loaded", s)
		}
	}
}

// knownDifferences are spellings whose inner fence md2html colours
// otherwise than VS Code's own embedding, and why.
var knownDifferences = map[string]string{
	"json5": "VS Code's rule hands json5 to the JSON grammar; md2html uses json5's own",
	"tex":   "VS Code's rule hands tex to the LaTeX grammar; md2html uses plain TeX's",
	// The Objective-C grammar includes $base, which inside VS Code's
	// embedding is the Markdown grammar, so a class body is read as
	// Markdown paragraphs there.
	"objc":        "VS Code reads its class bodies as Markdown, through $base",
	"objective-c": "VS Code reads its class bodies as Markdown, through $base",
}

// For every spelling VS Code's Markdown grammar embeds, md2html's inner
// fence gives the same colours as the full grammar does.
func TestInnerFencesMatchTheFullGrammar(t *testing.T) {
	ls := slowSet()
	hl := highlightContext{langs: ls}
	p := findPlugin(BuiltinPlugins(), "grammars")
	compared := 0
	for _, name := range p.GrammarNames() {
		g := p.Grammars[name]
		if _, err := os.Stat(filepath.Join("internal", "highlight", "testdata", "conformance", name+".json.gz")); err != nil {
			continue
		}
		var sample []string
		for _, l := range conformanceDump(t, name).Lines {
			if len(sample) == 12 || strings.HasPrefix(strings.TrimSpace(l), "```") || strings.HasPrefix(strings.TrimSpace(l), "~~~") {
				break
			}
			sample = append(sample, l)
		}
		for _, spelling := range append([]string{g.Name}, g.Aliases...) {
			lines := append(append([]string{"```" + spelling}, sample...), "```")
			full, err := ls.engine.Tokenize(markdownScope, lines)
			if err != nil {
				t.Fatal(err)
			}
			embedded := false
			for _, toks := range full[1 : len(lines)-1] {
				for _, tok := range toks {
					embedded = embedded || slices.ContainsFunc(tok.Scopes, func(s string) bool { return strings.HasPrefix(s, "meta.embedded.block") })
				}
			}
			if !embedded {
				continue
			}
			got := hl.spans("markdown", nil, lines, 1, func(w string) { t.Errorf("%s: warning %s", spelling, w) })
			diff := ""
			for i := range lines {
				var g []highlight.Span
				if got != nil {
					g = got[i]
				}
				if want := highlight.Spans(full[i]); !slices.Equal(g, want) {
					diff = fmt.Sprintf("line %d %q:\n got %v\nwant %v", i+1, lines[i], g, want)
					break
				}
			}
			why, known := knownDifferences[spelling]
			switch {
			case known && diff == "":
				t.Errorf("%s: listed as differing (%s) but now matches; take it off the list", spelling, why)
			case known:
				t.Logf("%s: differs, as listed: %s", spelling, why)
			case diff != "":
				t.Errorf("%s, %s", spelling, diff)
			default:
				compared++
			}
		}
	}
	if compared < 50 {
		t.Errorf("only %d spellings compared", compared)
	}
	t.Logf("%d spellings compared", compared)
}

// CRLF line ends and text wider than a byte before the code leave every
// offset on the right bytes.
func TestInnerFenceCRLFAndWideText(t *testing.T) {
	for _, src := range []string{
		strings.ReplaceAll(md("> é — 世界\n>\n> ```go\n> func x() {} // é\n> ```\n"), "\n", "\r\n"),
		md("- 世界 é\n\n  ```go\n  s := \"é—世\"; func x() {}\n  ```\n"),
	} {
		out := keepsText(t, src)
		if !strings.Contains(out, `<span class="hl-keyword">func</span> <span class="hl-function">x</span>`) {
			t.Errorf("%q: not highlighted:\n%s", src, out)
		}
	}
}
