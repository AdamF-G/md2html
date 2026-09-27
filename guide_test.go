package md2html

import (
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// guide returns docs/authoring.md, the text `md2html --guide` prints.
func guide(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("docs", "authoring.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The guide says every claim in it was checked against the binary. These
// are its claims about behaviour a reader cannot see coming, the silent
// traps above all, each run through Convert. quote is text the guide must
// still contain, so a claim reworded or dropped from the guide fails here
// and gets looked at, rather than leaving a test pinning behaviour nothing
// documents any more.
func TestGuideClaims(t *testing.T) {
	text := guide(t)
	for _, c := range []struct {
		name  string
		quote string // in the guide
		src   string
		want  []string
		not   []string
		warns bool
		page  bool    // convert a full page, not a fragment
		opts  Options // further options for the conversion
	}{
		{
			name:  "a ::: inside a code block closes the container",
			quote: "An inner `:::` closes the nearest open",
			src:   "::: example\n````markdown\n::: callout\nx\n:::\n````\nafter\n:::\n",
			want:  []string{"<pre><code>after\n:::"},
		},
		{
			name:  "a longer outer colon fence holds it",
			quote: ":::: example Writing a callout",
			src:   ":::: example\n````markdown\n::: callout\nx\n:::\n````\nafter\n::::\n\n# Later\n",
			want:  []string{"<p>after</p>\n</details>", `<h1 id="later">`},
		},
		{
			name:  "an unclosed container runs to the end",
			quote: "An unclosed `:::` container runs to the end of the document",
			src:   "::: callout\nopen\n\n# Swallowed\n",
			want:  []string{"<h1 id=\"swallowed\">Swallowed", "</h1>\n</div>"},
		},
		{
			name:  "only the first class names the kind",
			quote: "`{.compact .warning}` is a plain",
			src:   "::: {.compact .warning}\nw\n:::\n",
			want:  []string{`<div class="compact warning">`},
			not:   []string{"callout-warning"},
		},
		{
			name:  "a braced unknown class is silent",
			quote: "`::: {.house-style}` emits a",
			src:   "::: {.house-style}\nx\n:::\n",
			want:  []string{`<div class="house-style">`},
		},
		{
			name:  "a bare unknown kind warns",
			quote: "`::: house-style` warns",
			src:   "::: house-style\nx\n:::\n",
			want:  []string{"<div>"},
			warns: true,
		},
		{
			name:  ":::, alone, is literal",
			quote: "`:::` alone is literal text",
			src:   ":::\n",
			want:  []string{"<p>:::</p>"},
		},
		{
			name:  "title: sets only the tab",
			quote: "sets only the browser tab",
			src:   "---\ntitle: Tab only\n---\n\ntext\n",
			want:  []string{"<title>Tab only</title>"},
			not:   []string{"<h1"},
		},
		{
			name:  "front matter after a byte order mark is body text",
			quote: "After a byte order mark",
			src:   "\ufeff---\ntitle: T\n---\n\n# H\n",
			want:  []string{"<title>H</title>", "\ntitle: T<a"},
		},
		{
			name:  "unclosed front matter is body text",
			quote: "without the closing line",
			src:   "---\ntitle: X\n\n# H\n",
			want:  []string{"<hr/>", "<p>title: X</p>"},
		},
		{
			name:  "front matter with a comment is not flat",
			quote: "A nested value, a YAML comment",
			src:   "---\n# note\ntitle: X\n---\n\n# H\n",
			want:  []string{"<hr/>"},
			warns: true,
		},
		{
			name:  "an italic line under the h1 becomes the subtitle",
			quote: "an italic line directly under the first `<h1>` becomes",
			src:   "# H\n\n*Draft, do not circulate*\n",
			want:  []string{`<p class="subtitle">Draft, do not circulate</p>`},
		},
		{
			name:  "a bracketed word touching braces becomes a span",
			quote: "renders `map<span>key</span>`",
			src:   "map[key]{value}\n",
			want:  []string{"map<span>key</span>"},
			not:   []string{"value"},
		},
		{
			name:  "a backslash does not escape a status word or §",
			quote: "`\\[draft]` is\n   still a badge",
			src:   "## 1 One\n\n\\[draft] \\§1\n",
			want:  []string{`<span class="chip chip-draft">`, `class="xref"`},
		},
		{
			name:  "status words are lowercase only",
			quote: "`[Proven]` stays literal",
			src:   "[Proven]\n",
			want:  []string{"<p>[Proven]</p>"},
		},
		{
			name:  "a span's label must be plain text",
			quote: "`[**x**]{.chip}` stays literal",
			src:   "[**x**]{.chip}\n",
			want:  []string{"[<strong>x</strong>]{.chip}"},
		},
		{
			name:  "a badge in a link's text is inert",
			quote: "a code block or a link's text",
			src:   "[see [draft]](#x)\n",
			not:   []string{"chip"},
		},
		{
			name:  "an unquoted # in a fig label starts a comment",
			quote: "`box: Step #3 of 4`",
			src:   "```fig\nitems:\n  - box: Step #3 of 4\n```\n",
			want:  []string{`<div class="fig-box">Step</div>`},
		},
		{
			name:  "an unquoted [ in a fig label fails to parse",
			quote: "starts with `` ` ``, `*`, `[`",
			src:   "```fig\nitems:\n  - box: [proven] x\n```\n",
			want:  []string{`class="language-fig"`},
			warns: true,
		},
		{
			name:  "a quoted fig label takes inline Markdown",
			quote: "only when YAML reads them as a string",
			src:   "```fig\nitems:\n  - box: \"*x* [proven] `c`\"\n```\n",
			want:  []string{"<em>x</em>", "chip-proven", "<code>c</code>"},
		},
		{
			name:  "a backtick in a caption breaks the fence",
			quote: "is invalid on a backtick fence",
			src:   "```go caption=\"a `b`\"\nx\n```\n\n```py\ny\n```\n",
			want:  []string{"```py"},
			not:   []string{"code-figure", "language-py"},
		},
		{
			name:  "a tilde fence takes a caption with a backtick",
			quote: "use `~~~` for that block",
			src:   "~~~go caption=\"a `b`\"\nx\n~~~\n",
			want:  []string{"<figcaption>a `b`</figcaption>"},
		},
		{
			name:  "the recommended caption form escapes a quote",
			quote: "```` ```go {caption=\"server.go\"} ````",
			src:   "```go {caption=\"q \\\"x\\\"\"}\ny\n```\n",
			want:  []string{"<figcaption>q &#34;x&#34;</figcaption>", `class="language-go"`},
		},
		{
			name:  "a caption on a mermaid fence is dropped",
			quote: "a `caption=` on a\nmermaid fence is dropped",
			src:   "```mermaid caption=\"m\"\ngraph LR\nA-->B\n```\n",
			want:  []string{`<pre class="mermaid">`},
			not:   []string{"figcaption"},
		},
		{
			name:  "a braced mermaid fence is a code block",
			quote: "```` ```{.mermaid} ```` is an ordinary code block",
			src:   "```{.mermaid}\ngraph LR\n```\n",
			want:  []string{`class="language-mermaid"`},
			not:   []string{`<pre class="mermaid">`},
		},
		{
			name:  "a pipe in a code span splits a table cell",
			quote: "A `|` inside a code span in a table still splits the cell",
			src:   "| a | b |\n|---|---|\n| `x|y` | z |\n",
			want:  []string{"<td>`x</td>"},
		},
		{
			name:  "Markdown inside an HTML block needs blank lines",
			quote: "Leave a blank line\nbetween an HTML tag and Markdown inside it",
			src:   "<div class=\"card\">\n**Bold**\n</div>\n",
			want:  []string{"**Bold**"},
		},
		{
			name:  "a numbered heading followed by punctuation claims no number",
			quote: "any other character after the number",
			src:   "## 4.3) B\n\n## 4.2 A\n\n§4.3 and § 4.2\n",
			want:  []string{"§4.3 and <a class=\"xref\" href=\"#42-a\">§ 4.2</a>"},
		},
		{
			name:  "duplicate explicit ids both stay",
			quote: "Two explicit `{#same}` ids both stay",
			src:   "## A {#same}\n\n## B {#same}\n",
			want:  []string{`<h2 id="same">A`, `<h2 id="same">B`},
		},
		{
			name:  "an unquoted number in a heading block is empty",
			quote: "An unquoted number becomes an empty\nvalue",
			src:   "## A {data-y=1}\n\n## B {data-y=\"1\" aria-label=\"x\"}\n",
			want:  []string{`<h2 data-y="" id="a">`, `<h2 data-y="1" id="b">`},
			not:   []string{"aria-label"},
		},
		{
			name:  "an alert takes no title",
			quote: "`> [!NOTE] Title` stay ordinary",
			src:   "> [!NOTE] Title\n> body\n",
			want:  []string{"<blockquote>"},
			not:   []string{"callout"},
		},
		{
			name:  "a link block drops an event handler",
			quote: "Anything else, such as an event handler, is\ndropped",
			src:   "[x](./b.md){onclick=\"alert(1)\" target=_blank}\n",
			want:  []string{`target="_blank"`},
			not:   []string{"onclick"},
		},
		{
			name:  "HTML comments stay in the page",
			quote: "HTML comments stay in the page",
			src:   "<!-- TODO -->\n",
			page:  true,
			want:  []string{"<!-- TODO -->", "&lt;!-- TODO --&gt;"},
		},
		{
			name:  "a heading block with a bare key is literal text",
			quote: "a bare key (`{data-flag}`) or a single-quoted value leaves the\nwhole block",
			src:   "## J {#j data-flag}\n\n## K {.a data-x='s'}\n",
			want:  []string{"J {#j data-flag}", "K {.a data-x=&#39;s&#39;}"},
			not:   []string{`id="j"`, `class="a"`},
		},
		{
			name:  "an attribute block after a paragraph is literal",
			quote: "After a paragraph or\n    list item, `{.lead}` stays literal text",
			src:   "Para. {.lead}\n\n- item {.lead}\n",
			want:  []string{"<p>Para. {.lead}</p>", "item {.lead}"},
		},
		{
			name:  "a link block drops hreflang and type",
			quote: "`hreflang` and `type` are among those dropped",
			src:   "[l](./b.md){hreflang=de type=text/html referrerpolicy=no-referrer}\n",
			want:  []string{`referrerpolicy="no-referrer"`},
			not:   []string{"hreflang", "type="},
		},
		{
			name:  "a colon after a heading's number claims none",
			quote: "Anything but a space after the\n    number, `.`, `)` or `:`",
			src:   "## 4.3: B\n\n## 4.4. C\n\n§4.3 §4.4\n",
			want:  []string{"<p>§4.3 §4.4</p>"},
		},
		{
			name:  "the first heading claiming a number wins",
			quote: "When two headings claim one number, the\nfirst wins",
			src:   "## 4.2 A\n\n## 4.2 B\n\n§4.2\n",
			want:  []string{`href="#42-a"`},
		},
		{
			name:  "--no-anchors turns off cross-references",
			quote: "`--no-anchors`\nturns cross-references off",
			src:   "## 4.2 A\n\n§4.2\n",
			opts:  Options{Transforms: without("headingAnchors")},
			want:  []string{"<p>§4.2</p>"},
		},
		{
			name:  "a [c:] badge stops at 60 characters",
			quote: "up to 60 characters",
			src:   "[c:" + strings.Repeat("x", 60) + "] [c:" + strings.Repeat("y", 61) + "]\n",
			want:  []string{`<span class="chip">` + strings.Repeat("x", 60), "[c:" + strings.Repeat("y", 61) + "]"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(text, c.quote) {
				t.Errorf("the guide no longer says %q", c.quote)
			}
			var warnings []string
			opt := c.opts
			opt.CSS = "/**/"
			opt.Warn = func(w string) { warnings = append(warnings, w) }
			// Front matter claims concern the page shell's <title>, so
			// they need a full page too.
			opt.Fragment = !c.page && !strings.Contains(c.src, "---\n") && !strings.Contains(c.src, "\ufeff")
			out, err := Convert([]byte(c.src), opt)
			if err != nil {
				t.Fatal(err)
			}
			got := string(out)
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("output lacks %q\ngot: %s", w, got)
				}
			}
			for _, n := range c.not {
				if strings.Contains(got, n) {
					t.Errorf("output has %q\ngot: %s", n, got)
				}
			}
			if c.warns != (len(warnings) > 0) {
				t.Errorf("warns = %v, want %v: %q", len(warnings) > 0, c.warns, warnings)
			}
		})
	}
}

// The guide is converted by the tool it describes and read on GitHub, so it
// can fall into its own traps: an unbalanced fence, a stray badge, an
// in-page link to a heading that was renamed. It must convert with no
// warning, and every #fragment it links must name an id on the page.
func TestGuideConvertsCleanly(t *testing.T) {
	var warnings []string
	got := convert(t, guide(t), func(w string) { warnings = append(warnings, w) })
	if len(warnings) > 0 {
		t.Errorf("the guide converts with warnings: %q", warnings)
	}
	if !strings.Contains(got, `<nav class="toc"`) {
		t.Error("the guide has no contents list")
	}
	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(` id="([^"]+)"`).FindAllStringSubmatch(got, -1) {
		ids[m[1]] = true
	}
	for _, m := range regexp.MustCompile(`href="#([^"]+)"`).FindAllStringSubmatch(got, -1) {
		if !ids[m[1]] {
			t.Errorf("the guide links #%s, which no element on the page has", m[1])
		}
	}
	prose := regexp.MustCompile(`(?s)<pre.*?</pre>|<code>.*?</code>`).ReplaceAllString(got, "")
	for _, stray := range []string{`class="chip`, `class="xref"`, "```", ":::"} {
		if strings.Contains(prose, stray) {
			t.Errorf("%s appears in the guide's prose, outside code", stray)
		}
	}
}

// The wide flag is honoured only by a rule scoped to a figure that is a
// direct child of the page's <main>, which is what makes the guide's claim
// true: inside a container the figure is not a child of <main>, and a
// fragment has no <main>. A rule naming fig-wide any other way would break
// it.
func TestGuideWideNeedsMain(t *testing.T) {
	if !strings.Contains(guide(t), "It has no effect inside a container or in `--fragment` output") {
		t.Error("the guide no longer states where wide has no effect")
	}
	rules := regexp.MustCompile(`(?m)^[^{}\n]*fig-wide[^{}\n]*\{`).FindAllString(defaultCSS, -1)
	if len(rules) == 0 {
		t.Error("no stylesheet rule names fig-wide")
	}
	for _, r := range rules {
		if !strings.HasPrefix(strings.TrimSpace(r), "main > figure.fig-wide") {
			t.Errorf("fig-wide rule %q is not scoped to main > figure", r)
		}
	}
	if strings.Contains(convert(t, "x\n", nil), "<main") {
		t.Error("fragment output has a <main>")
	}
}

// messageText returns every string literal in the package's and the
// CLI's non-test sources, with literals joined by + read as one, and
// format verbs read as "…" (%q as a quoted "…"). Comments are not
// included, so a message that survives only in a comment does not count.
func messageText(t *testing.T) string {
	t.Helper()
	var out strings.Builder
	verbs := strings.NewReplacer(`%q`, `"…"`, `%s`, `…`, `%d`, `…`, `%v`, `…`, `%w`, `…`)
	for _, dir := range []string{".", filepath.Join("cmd", "md2html")} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			fset := token.NewFileSet()
			var sc scanner.Scanner
			sc.Init(fset.AddFile(f, -1, len(b)), b, nil, 0)
			joined := false
			for {
				_, tok, lit := sc.Scan()
				if tok == token.EOF {
					break
				}
				switch tok {
				case token.STRING:
					v, err := strconv.Unquote(lit)
					if err != nil {
						continue
					}
					if !joined {
						out.WriteString("\x00")
					}
					out.WriteString(verbs.Replace(v))
					joined = false
				case token.ADD:
					joined = true
				default:
					joined = false
				}
			}
		}
	}
	return out.String()
}

// yamlMessages are the YAML parser's own messages the Warnings section
// names, each with a label that produces it. They come from a dependency,
// not this repository, so they are checked by producing them.
var yamlMessages = map[string]string{
	"did not find expected key":                      "box: [proven] x",
	"found character that cannot start any token":    "box: `code` first",
	"mapping values are not allowed in this context": "box: Note: colon",
	"unknown anchor":                                 "box: *x",
	"cannot unmarshal !!map into string":             "box: {a}",
	"!!seq":                                          "box: [a]",
}

// The Warnings section quotes each message: the start of it in bold, and
// the fig faults in italics. Every fragment between the "…" placeholders
// must occur in a message string in the code, or, for the YAML parser's
// messages, be produced by converting a label, or the section is pointing
// readers at a warning they will never see.
func TestGuideWarningsExistInTheCode(t *testing.T) {
	code := messageText(t)

	for msg, label := range yamlMessages {
		var warnings []string
		convert(t, "```fig\nitems:\n  - "+label+"\n```\n", func(w string) { warnings = append(warnings, w) })
		if len(warnings) != 1 || !strings.Contains(warnings[0], msg) {
			t.Errorf("%q gives %q, want a warning containing %q", label, warnings, msg)
		}
	}

	text := guide(t)
	_, section, ok := strings.Cut(text, "\n## Warnings\n")
	if !ok {
		t.Fatal("the guide has no Warnings section")
	}
	check := func(kind string, quoted [][]string, min int) {
		if len(quoted) < min {
			t.Fatalf("found only %d %s messages in the section; has its format changed?", len(quoted), kind)
		}
		for _, q := range quoted {
			for _, frag := range strings.Split(q[1], "…") {
				frag = strings.Join(strings.Fields(strings.Trim(frag, ` "`)), " ")
				if len(frag) < 4 {
					continue
				}
				known := strings.Contains(code, frag)
				for msg := range yamlMessages {
					known = known || strings.Contains(msg, frag) || strings.Contains(frag, msg)
				}
				if !known {
					t.Errorf("the guide quotes %q, which no message contains", frag)
				}
			}
		}
	}
	check("bold", regexp.MustCompile(`(?m)^\*\*(.+?)\*\*`).FindAllStringSubmatch(section, -1), 15)
	check("italic", regexp.MustCompile(`(?s)(?:^|[^*])\*([^*\n][^*]*?)\*`).FindAllStringSubmatch(section, -1), 15)
}

// Claims about links that only a crawl can check: a leading / is a
// filesystem path, a query string is dropped, and a reference-style link
// is rewritten like an inline one.
func TestGuideLinkClaims(t *testing.T) {
	text := guide(t)
	for _, q := range []string{
		"A link starting with `/` is a filesystem path",
		"A query string on a document link is dropped",
		"Reference-style links are rewritten the same way",
	} {
		if !strings.Contains(text, q) {
			t.Errorf("the guide no longer says %q", q)
		}
	}
	root := writeTree(t, map[string]string{
		"docs/a.md": "[q](./b.md?v=2) [r][ref] [s](/b.md)\n\n[ref]: ./b.md#top\n",
		"docs/b.md": "# B\n",
	})
	res := mustCrawl(t, CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, OutDir: filepath.Join(root, "site"), Depth: -1})
	var a Doc
	for _, d := range res.Docs {
		if filepath.Base(d.Src) == "a.md" {
			a = d
		}
	}
	var slashWarned bool
	for _, w := range res.Warnings {
		slashWarned = slashWarned || strings.Contains(w.Message, "does not exist: /b.md")
	}
	if !slashWarned {
		t.Errorf("a /-rooted link did not warn: %+v", res.Warnings)
	}
	src, err := os.ReadFile(a.Src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Convert(src, Options{Fragment: true, CSS: "/**/", LinkMap: a.LinkMap})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<a href="b.html">q</a>`, `<a href="b.html#top">r</a>`, `<a href="/b.md">s</a>`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output lacks %s\ngot: %s", want, out)
		}
	}
}

// --toc float on a page with no marker adds nothing and, unlike the front
// matter key, says nothing.
func TestGuideTOCFloatWithoutAMarkerIsSilent(t *testing.T) {
	if !strings.Contains(guide(t), "Front matter warns; `--toc float` does not.") {
		t.Error("the guide no longer says --toc float is silent")
	}
	var warnings []string
	out, err := Convert([]byte("# A\n\n## B\n"), Options{CSS: "/**/", TOC: "float",
		Warn: func(w string) { warnings = append(warnings, w) }})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `class="toc`) || len(warnings) > 0 {
		t.Errorf("toc = %v, warnings = %q; want no list and no warning",
			strings.Contains(string(out), `class="toc`), warnings)
	}
}

// without returns the builtin transforms minus the named one, as the CLI's
// --no-* flags build them.
func without(name string) []Transform {
	var ts []Transform
	for _, t := range Builtins() {
		if t.Name != name {
			ts = append(ts, t)
		}
	}
	return ts
}
