package md2html

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/AdamF-G/md2html/internal/highlight"
)

func convertBody(t *testing.T, src string, opt Options) (string, []string) {
	t.Helper()
	out, w, err := convertBodyErr(src, opt)
	if err != nil {
		t.Fatal(err)
	}
	return out, w
}

// convertBodyErr is convertBody for a goroutine, which must not call
// t.Fatal.
func convertBodyErr(src string, opt Options) (string, []string, error) {
	var w []string
	opt.Fragment = true
	opt.Warn = func(s string) { w = append(w, s) }
	out, err := Convert([]byte(src), opt)
	return string(out), w, err
}

var spanTags = regexp.MustCompile(`</?span[^>]*>`)

func TestGoFenceIsHighlighted(t *testing.T) {
	out, w := convertBody(t, "```go\nfunc main() { fmt.Println(\"hi\", 42) } // ok\n```\n", Options{})
	if len(w) != 0 {
		t.Errorf("warnings %q", w)
	}
	for _, want := range []string{`<code class="language-go md2html-hl">`, `<span class="hl-keyword">func</span>`,
		`<span class="hl-string">&#34;hi&#34;</span>`, `<span class="hl-number">42</span>`, `<span class="hl-comment">// ok</span>`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
}

// Only spans are added: strip them and the code is today's, byte for byte.
func TestHighlightKeepsTextExactly(t *testing.T) {
	for _, src := range []string{
		"```go\nx := \"héllo — 世界\" // <b>&amp;</b>\nreturn\n```\n",
		"```go\r\nfunc a() {}\r\n// c\r\n```\r\n",
		"```python\ns = '''multi\nline'''\n# done\n```\n",
	} {
		hl, _ := convertBody(t, src, Options{})
		plain, _ := convertBody(t, src, Options{NoHighlight: true})
		stripped := strings.Replace(spanTags.ReplaceAllString(hl, ""), " md2html-hl", "", 1)
		if stripped != plain {
			t.Errorf("text changed for %q:\n got %s\nwant %s", src, stripped, plain)
		}
	}
}

// A multi-line string is closed at each line end and reopened.
func TestSpansNeverCrossALine(t *testing.T) {
	out, _ := convertBody(t, "```python\ns = '''one\ntwo'''\n```\n", Options{})
	code := out[strings.Index(out, "<code"):strings.Index(out, "</code>")]
	for i, line := range strings.Split(code, "\n") {
		if strings.Count(line, "<span") != strings.Count(line, "</span>") {
			t.Errorf("line %d is unbalanced: %s", i, line)
		}
	}
}

// syntaxToken is one --syntax-* declaration line in a style's stylesheet.
var syntaxToken = regexp.MustCompile(`(?m)^[ \t]*--syntax-[a-z-]+:[^\n]*;\n`)

// oldMarker is the provenance marker v0.12.0 wrote at the top of each
// golden.
const oldMarker = MarkerPrefix + " v0.12.0 - edits will be overwritten -->"

// A page with no highlighted fence is what v0.12.0 wrote, byte for byte,
// once its stylesheet's --syntax-* declarations are taken out. Those are
// the only change the owner accepted (2026-10-15): the styles must define
// the tokens, and on such a page they colour nothing. The goldens are
// v0.12.0's own output for these sources; testdata/v0.12.0/README says
// how they were made. Their provenance marker names v0.12.0, so that one
// marker is swapped for today's Marker() before comparing, and the test
// does not break when Version is bumped. Nothing else is rewritten.
func TestUnchangedOutput(t *testing.T) {
	for _, c := range []struct {
		name, src string
		opt       Options
	}{
		{"no-language", "```\nfunc x\n```\n", Options{}},
		{"unknown-language", "```text\nfunc x\n```\n", Options{}},
		{"no-highlight", "```{.go .no-highlight}\nfunc x\n```\n", Options{}},
		{"front-matter-off", "---\nhighlight: false\n---\n```go\nfunc x\n```\n", Options{}},
		{"option-off", "```go\nfunc x\n```\n", Options{NoHighlight: true}},
		{"no-code", "# Title\n\nText.\n", Options{}},
		{"fig-falls-back", "```fig\nnot: [a figure\n```\n", Options{}},
		{"vega-lite-off", "```vega-lite\n{\"mark\": \"bar\"}\n```\n", Options{NoCharts: true}},
		{"mermaid-any-case", "```Mermaid\ngraph LR\n```\n", Options{}},
	} {
		out, _ := convertBody(t, c.src, c.opt)
		if strings.Contains(out, "md2html-hl") || strings.Contains(out, `class="hl-`) {
			t.Errorf("%s: highlighted:\n%s", c.name, out)
		}
		b, err := os.ReadFile(filepath.Join("testdata", "v0.12.0", c.name+".html"))
		if err != nil {
			t.Fatal(err)
		}
		golden, ok := strings.CutPrefix(string(b), oldMarker)
		if !ok {
			t.Fatalf("%s: golden does not start with v0.12.0's marker", c.name)
		}
		golden = Marker() + golden
		if got := syntaxToken.ReplaceAllString(out, ""); got != golden {
			t.Errorf("%s: differs from v0.12.0 beyond the --syntax-* declarations:\n got %s\nwant %s", c.name, got, golden)
		}
	}
}

func TestNoHighlightClassStays(t *testing.T) {
	out, _ := convertBody(t, "```{.go .no-highlight}\nfunc x\n```\n", Options{})
	if !strings.Contains(out, `class="language-go no-highlight"`) {
		t.Errorf("class lost:\n%s", out)
	}
}

func TestCaptionsIdsAndClassesSurvive(t *testing.T) {
	out, _ := convertBody(t, "```go {#srv .wide caption=\"server.go\"}\nfunc x\n```\n", Options{})
	for _, want := range []string{`<figure class="code-figure"><figcaption>server.go</figcaption>`,
		`<pre id="srv"><code class="language-go wide md2html-hl">`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
}

func TestEveryInfoStringFormHighlights(t *testing.T) {
	for _, info := range []string{"go", "{.go caption=\"x\"}", "go {.wide}", "Go"} {
		out, _ := convertBody(t, "```"+info+"\nfunc x\n```\n", Options{})
		if !strings.Contains(out, `<span class="hl-keyword">func</span>`) {
			t.Errorf("%s: not highlighted", info)
		}
	}
}

func TestEmptyFenceIsUnchanged(t *testing.T) {
	for _, src := range []string{"```go\n```\n", "```go\n\n```\n"} {
		hl, _ := convertBody(t, src, Options{})
		plain, _ := convertBody(t, src, Options{NoHighlight: true})
		if hl != plain {
			t.Errorf("%q changed:\n%s", src, hl)
		}
	}
}

// A known language whose tokens colour nothing is a plain fence.
func TestUncolouredFenceIsUnchanged(t *testing.T) {
	src := "```go\ny\n```\n"
	hl, _ := convertBody(t, src, Options{})
	plain, _ := convertBody(t, src, Options{NoHighlight: true})
	if hl != plain {
		t.Errorf("changed:\n%s", hl)
	}
}

func TestHighlightInsideContainers(t *testing.T) {
	src := "- item\n\n  ```go\n  func x() {}\n  ```\n\n::: details Code\n```go\nreturn 1\n```\n:::\n"
	out, w := convertBody(t, src, Options{})
	if len(w) != 0 {
		t.Errorf("warnings %q", w)
	}
	if strings.Count(out, "md2html-hl") != 2 {
		t.Errorf("want two highlighted fences:\n%s", out)
	}
	plain, _ := convertBody(t, src, Options{NoHighlight: true})
	if strings.ReplaceAll(spanTags.ReplaceAllString(out, ""), " md2html-hl", "") != plain {
		t.Error("text changed inside containers")
	}
}

func TestNearMissWarnsOnce(t *testing.T) {
	_, w := convertBody(t, "Text.\n\n```pyhton\nx\n```\n\n```text\nx\n```\n\n```console\n$ x\n```\n", Options{})
	want := `fence at line 3: no grammar for "pyhton"; did you mean "python"?`
	if len(w) != 1 || w[0] != want {
		t.Errorf("warnings %q, want [%q]", w, want)
	}
}

// A language the built-in grammars leave out, often for its licence, or
// one the collection lacks, is a real name rather than a typo: its fence
// stays plain and silent, as any unknown language's does.
func TestLeftOutLanguageIsPlainAndSilent(t *testing.T) {
	for _, lang := range []string{"sass", "glsl", "ahk2", "Sass", "patch", "nasm"} {
		src := "```" + lang + "\nx\n```\n"
		out, w := convertBody(t, src, Options{})
		if len(w) != 0 {
			t.Errorf("%s: warnings %q, want none", lang, w)
		}
		if strings.Contains(out, "md2html-hl") {
			t.Errorf("%s: highlighted:\n%s", lang, out)
		}
	}
}

func TestNearMissNamesTheFileLine(t *testing.T) {
	src := "---\ntitle: T\nlang: en\n---\n\nText.\n\nMore.\n```pyhton\nx\n```\n"
	_, w := convertBody(t, src, Options{})
	if len(w) != 1 || !strings.HasPrefix(w[0], "fence at line 9:") {
		t.Errorf("warnings %q, want line 9", w)
	}
}

func TestHighlightFrontMatterValue(t *testing.T) {
	_, w := convertBody(t, "---\nhighlight: no\n---\n```go\nfunc x\n```\n", Options{})
	if len(w) != 1 || w[0] != `front matter highlight "no" is not true or false; highlighting the page's fences` {
		t.Errorf("warnings %q", w)
	}
}

func TestOverBudgetFenceIsPlain(t *testing.T) {
	hl := highlightContext{langs: NewPluginCache().languages(nil, nil), budget: 1}
	var w []string
	spans := hl.spans("go", nil, []string{"func x() {}"}, 3, func(s string) { w = append(w, s) })
	if spans != nil {
		t.Error("spans returned over budget")
	}
	if len(w) != 1 || w[0] != `fence at line 3: highlighting "go" took over 2s; showing it plain` {
		t.Errorf("warnings %q", w)
	}
}

func TestEngineErrorIsPlain(t *testing.T) {
	hl := highlightContext{langs: &langSet{byName: map[string]*Grammar{"go": {Name: "go", Scope: "source.missing", plugin: &Plugin{}}},
		engine: NewPluginCache().languages(nil, nil).engine}}
	var w []string
	if hl.spans("go", nil, []string{"x"}, 1, func(s string) { w = append(w, s) }) != nil {
		t.Error("spans returned on an engine error")
	}
	if len(w) != 1 || !strings.HasPrefix(w[0], `fence at line 1: highlighting "go" failed: `) || !strings.HasSuffix(w[0], "; showing it plain") {
		t.Errorf("warnings %q", w)
	}
}

func TestPagePluginsProvideGrammars(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"langs/md2html-plugin.json": `{"format": 1, "name": "langs", "grammars": {"fab": {"file": "g.json"}}}`,
		"langs/g.json":              fabGrammar,
		"page.md":                   "---\nplugins: ./langs\n---\n```fab\nfab x\n```\n",
	})
	src, _ := os.ReadFile(filepath.Join(dir, "page.md"))
	out, w := convertBody(t, string(src), Options{SourcePath: filepath.Join(dir, "page.md"), PluginCache: NewPluginCache()})
	if len(w) != 0 {
		t.Errorf("warnings %q", w)
	}
	if !strings.Contains(out, `<span class="hl-keyword">fab</span>`) {
		t.Errorf("not highlighted:\n%s", out)
	}
}

// fabPlugin is a local plugin named name whose grammar gives "fab" the
// scope name.
func fabPlugin(name, scope string) map[string]string {
	return map[string]string{
		name + "/md2html-plugin.json": `{"format": 1, "name": "` + name + `", "grammars": {"fab": {"file": "g.json"}}}`,
		name + "/g.json":              `{"scopeName": "source.fab", "patterns": [{"match": "\\bfab\\b", "name": "` + scope + `"}]}`,
	}
}

// A page's own plugin that claims a language the run's already provides
// wins, and the page says so once.
func TestPagePluginLanguageClashWarns(t *testing.T) {
	files := fabPlugin("run", "string.fab")
	maps.Copy(files, fabPlugin("page", "keyword.fab"))
	files["doc.md"] = "---\nplugins: ./page\n---\n```fab\nfab x\n```\n"
	dir := writeTree(t, files)
	src, _ := os.ReadFile(filepath.Join(dir, "doc.md"))
	out, w := convertBody(t, string(src), Options{SourcePath: filepath.Join(dir, "doc.md"),
		Plugins: []string{filepath.Join(dir, "run")}, PluginCache: NewPluginCache()})
	if len(w) != 1 || !strings.Contains(w[0], `both provide language "fab"; using`) {
		t.Errorf("warnings %q", w)
	}
	if !strings.Contains(out, `<span class="hl-keyword">fab</span>`) {
		t.Errorf("the page plugin's grammar lost:\n%s", out)
	}
}

// A clash among the run's plugins alone is not every page's to report.
func TestRunPluginLanguageClashIsQuietOnPages(t *testing.T) {
	files := fabPlugin("one", "string.fab")
	maps.Copy(files, fabPlugin("two", "keyword.fab"))
	dir := writeTree(t, files)
	_, w := convertBody(t, "```fab\nfab x\n```\n", Options{
		Plugins: []string{filepath.Join(dir, "one"), filepath.Join(dir, "two")}, PluginCache: NewPluginCache()})
	if len(w) != 0 {
		t.Errorf("warnings %q", w)
	}
}

func TestBrokenPagePluginKeepsRunGrammars(t *testing.T) {
	dir := writeTree(t, map[string]string{"page.md": "---\nplugins: ./nope\n---\n```go\nfunc x\n```\n"})
	src, _ := os.ReadFile(filepath.Join(dir, "page.md"))
	out, w := convertBody(t, string(src), Options{SourcePath: filepath.Join(dir, "page.md")})
	if len(w) != 1 || !strings.HasPrefix(w[0], "front matter plugins: ") {
		t.Errorf("warnings %q", w)
	}
	if !strings.Contains(out, "md2html-hl") {
		t.Error("the run's grammars were lost")
	}
}

func TestLocalGrammarDiagnosticsWarnOncePerPage(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"langs/md2html-plugin.json": `{"format": 1, "name": "langs", "grammars": {"bad": {"file": "g.json"}}}`,
		"langs/g.json":              `{"scopeName": "source.bad", "patterns": [{"match": "\\p{Xwd}+", "name": "keyword.bad"}, {"match": "ok", "name": "keyword.ok"}]}`,
	})
	_, w := convertBody(t, "```bad\nok\n```\n\n```bad\nok\n```\n", Options{Plugins: []string{filepath.Join(dir, "langs")}})
	if len(w) != 1 || !strings.HasPrefix(w[0], `grammar "bad" from plugin langs, or a grammar it includes: 1 pattern cannot be translated and is skipped: `) {
		t.Errorf("warnings %q", w)
	}
}

func TestConvertHighlightsInParallel(t *testing.T) {
	var b strings.Builder
	b.WriteString("```go\n")
	for i := range 400 {
		fmt.Fprintf(&b, "func f%d() string { return \"x\" } // %d\n", i, i)
	}
	b.WriteString("```\n")
	cache := NewPluginCache()
	var wg sync.WaitGroup
	outs := make([]string, 32)
	warns := make([][]string, len(outs))
	errs := make([]error, len(outs))
	for i := range outs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outs[i], warns[i], errs[i] = convertBodyErr(b.String(), Options{PluginCache: cache})
		}()
	}
	wg.Wait()
	for i := range outs {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if len(warns[i]) != 0 {
			t.Errorf("output %d: warnings %q", i, warns[i])
		}
		if outs[i] != outs[0] {
			t.Fatalf("output %d differs", i)
		}
	}
	if !strings.Contains(outs[0], "md2html-hl") {
		t.Error("not highlighted")
	}
}

// Each kind reaches the page as its own class, adjacent tokens of one kind
// share a span, a plain line gets none, and text inside a span is escaped
// as it is outside one. A local grammar names one scope per kind, so the
// expected markup does not move when a built-in grammar changes.
func TestEveryKindRendersItsSpan(t *testing.T) {
	rows := []struct{ word, scope, kind string }{
		{"cm", "comment.line.x", "comment"}, {"kw", "keyword.control.x", "keyword"},
		{"st", "string.unquoted.x", "string"}, {"nu", "constant.numeric.x", "number"},
		{"co", "constant.language.x", "constant"}, {"fn", "entity.name.function.x", "function"},
		{"ty", "entity.name.type.x", "type"}, {"pr", "variable.other.property.x", "property"},
		{"tg", "entity.name.tag.x", "tag"}, {"at", "entity.other.attribute-name.x", "attribute"},
		{"in", "markup.inserted.x", "inserted"}, {"de", "markup.deleted.x", "deleted"},
		{"hd", "markup.heading.x", "heading"},
	}
	var patterns, lines, want []string
	for _, r := range rows {
		patterns = append(patterns, fmt.Sprintf(`{"match": "^%s$", "name": %q}`, r.word, r.scope))
		lines = append(lines, r.word)
		want = append(want, fmt.Sprintf(`<span class="hl-%s">%s</span>`, r.kind, r.word))
	}
	patterns = append(patterns, `{"match": "^kw", "name": "keyword.control.x"}`, `{"match": "sto$", "name": "storage.type.x"}`,
		`{"match": "\"[^\"]*\"", "name": "string.quoted.x"}`)
	lines = append(lines, "kwsto", `"<&>"`, "plain")
	want = append(want, `<span class="hl-keyword">kwsto</span>`, `<span class="hl-string">&#34;&lt;&amp;&gt;&#34;</span>`, "plain")
	dir := writeTree(t, map[string]string{
		"langs/md2html-plugin.json": `{"format": 1, "name": "langs", "grammars": {"kinds": {"file": "g.json"}}}`,
		"langs/g.json":              `{"scopeName": "source.x", "patterns": [` + strings.Join(patterns, ", ") + `]}`,
	})
	out, w := convertBody(t, "```kinds\n"+strings.Join(lines, "\n")+"\n```\n", Options{Plugins: []string{filepath.Join(dir, "langs")}})
	if len(w) != 0 {
		t.Errorf("warnings %q", w)
	}
	if code := `<code class="language-kinds md2html-hl">` + strings.Join(want, "\n") + "\n</code>"; !strings.Contains(out, code) {
		t.Errorf("want\n%s\nin\n%s", code, out)
	}
}

// A line past the per-line limit keeps its head coloured and the rest
// plain, with no warning and no change to its text.
func TestLongLineIsPlainPastTheLimitAndSilent(t *testing.T) {
	src := "```go\nfunc x() {}\n// " + strings.Repeat("é", highlight.DefaultMaxLineRunes) + " return\n```\n"
	out, w := convertBody(t, src, Options{})
	if len(w) != 0 {
		t.Errorf("warnings %q", w)
	}
	if !strings.Contains(out, `<span class="hl-comment">// é`) || !strings.Contains(out, "é return\n</code>") {
		t.Errorf("want the line's head a comment and its tail plain:\n%.300s ... %s", out, out[len(out)-300:])
	}
	plain, _ := convertBody(t, src, Options{NoHighlight: true})
	if strings.Replace(spanTags.ReplaceAllString(out, ""), " md2html-hl", "", 1) != plain {
		t.Error("text changed")
	}
}

// A run plugin that does not load is an error even with no style named,
// or highlighting off, since it may be there for its grammars.
func TestBrokenRunPluginIsAnError(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"langs/md2html-plugin.json": `{"format": 1, "name": "langs", "grammars": {"bad": {"file": "g.json"}}}`,
		"langs/g.json":              `{"scopeName": "source.bad",`,
	})
	for name, opt := range map[string]Options{"highlighting on": {}, "highlighting off": {NoHighlight: true}} {
		opt.Plugins = []string{filepath.Join(dir, "langs")}
		_, _, err := convertBodyErr("```bad\nx\n```\n", opt)
		if err == nil || !strings.HasPrefix(err.Error(), `plugin `) || !strings.Contains(err.Error(), `grammar "bad": g.json not valid JSON`) {
			t.Errorf("%s: err %v, want the plugin's load error", name, err)
		}
	}
}
