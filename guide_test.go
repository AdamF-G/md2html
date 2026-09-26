package md2html

import (
	"os"
	"path/filepath"
	"regexp"
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
			quote: "starts with `` ` ``, `*`, `_`, `[`",
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
			src:   "```go caption=\"a `b`\"\nx\n```\n",
			not:   []string{"code-figure"},
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
			quote: "an unquoted number\nbecomes an empty value",
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
			want:  []string{"<!-- TODO -->"},
		},
		{
			name:  "wide has no effect inside a container",
			quote: "It has no effect inside a container",
			src:   "::: card\n```fig\nwide: true\nitems:\n  - box: x\n```\n:::\n",
			want:  []string{"<div class=\"card\">\n<figure class=\"fig fig-wide\">"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(text, c.quote) {
				t.Errorf("the guide no longer says %q", c.quote)
			}
			var warnings []string
			got := convert(t, c.src, func(w string) { warnings = append(warnings, w) })
			if strings.Contains(c.src, "---\n") || strings.Contains(c.src, "\ufeff") {
				// Front matter claims concern the page shell's <title>.
				out, err := Convert([]byte(c.src), Options{CSS: "/**/", NoSource: true,
					Warn: func(w string) { warnings = append(warnings, w) }})
				if err != nil {
					t.Fatal(err)
				}
				got = string(out)
			}
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
