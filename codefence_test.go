package md2html

import (
	"strings"
	"testing"
)

func TestSplitFenceInfo(t *testing.T) {
	cases := []struct{ in, lang, caption string }{
		{"go", "go", ""},
		{`go caption="server.go"`, "go", "server.go"},
		{`go caption='server.go'`, "go", "server.go"},
		{`go caption="cmd/md2html/main.go" other=1`, "go", "cmd/md2html/main.go"},
		{`caption="just a caption"`, "", "just a caption"},
		{`go caption="with spaces in it"`, "go", "with spaces in it"},
		{"", "", ""},
		{"go someattr", "go", ""},
		// The caption must be stripped before what remains is treated as a
		// language, not just read off token index 0 — otherwise the
		// language is lost whenever caption="..." happens to come first.
		{`caption="x.go" go`, "go", "x.go"},
	}
	for _, c := range cases {
		lang, caption := splitFenceInfo(c.in)
		if lang != c.lang || caption != c.caption {
			t.Errorf("splitFenceInfo(%q) = (%q, %q), want (%q, %q)",
				c.in, lang, caption, c.lang, c.caption)
		}
	}
}

// The whole point: today this caption vanishes without a word.
func TestCodeFenceCaptionRendersAsFigcaption(t *testing.T) {
	got := convert(t, "```go caption=\"server.go\"\nfoo()\n```\n", nil)
	if !strings.Contains(got, `<figure class="code-figure"><figcaption>server.go</figcaption>`) {
		t.Errorf("no caption bar\ngot: %s", got)
	}
	if !strings.Contains(got, `<pre><code class="language-go">foo()`) {
		t.Errorf("code block changed shape\ngot: %s", got)
	}
	if !strings.Contains(got, "</pre></figure>") {
		t.Errorf("figure not closed around the block\ngot: %s", got)
	}
}

// A fence with no caption must render byte-identically to before.
func TestCodeFenceWithoutCaptionIsUnchanged(t *testing.T) {
	got := convert(t, "```go\nfoo()\n```\n", nil)
	if strings.Contains(got, "figure") {
		t.Errorf("wrapped an uncaptioned block\ngot: %s", got)
	}
	if !strings.Contains(got, `<pre><code class="language-go">foo()`) {
		t.Errorf("plain fence changed\ngot: %s", got)
	}
}

func TestCodeFenceWithoutLanguageIsUnchanged(t *testing.T) {
	got := convert(t, "```\nplain\n```\n", nil)
	if !strings.Contains(got, "<pre><code>plain") {
		t.Errorf("bare fence changed\ngot: %s", got)
	}
}

// Code must still be escaped: this replaces goldmark's renderer, so its
// escaping is now this code's responsibility.
func TestCodeFenceEscapesContent(t *testing.T) {
	got := convert(t, "```go caption=\"x\"\n<script>&\n```\n", nil)
	if strings.Contains(got, "<script>") {
		t.Errorf("emitted unescaped markup\ngot: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;&amp;") {
		t.Errorf("content not escaped\ngot: %s", got)
	}
}

// A caption is author-supplied text and must be escaped too.
func TestCodeFenceEscapesCaption(t *testing.T) {
	got := convert(t, "```go caption=\"a<b&c\"\nx\n```\n", nil)
	if strings.Contains(got, "a<b&c") {
		t.Errorf("caption not escaped\ngot: %s", got)
	}
}

// Overriding the fenced-code renderer must not disturb mermaid, which
// replaces its fences with its own AST node before rendering.
func TestCodeFenceOverrideLeavesMermaidAlone(t *testing.T) {
	got := convert(t, "```mermaid\ngraph TD;\nA-->B;\n```\n", nil)
	if !strings.Contains(got, `class="mermaid"`) {
		t.Errorf("mermaid fence broken\ngot: %s", got)
	}
}

// R10: goldmark's own fenced-code-block renderer does not honor a trailing
// {...} attribute today. CodeAttributeFilter (renderer/html) is wired up
// for inline code spans (renderCodeSpan), not for renderFencedCodeBlock —
// confirmed by reading goldmark's source and by capturing this exact output
// before this file's renderer override existed. A braced attribute is
// silently swallowed, the same way an unhandled caption is. This locks in
// that the replacement renderer keeps that pre-existing behavior rather
// than changing it in either direction.
func TestCodeFenceBracedAttributeIsUnaffected(t *testing.T) {
	got := convert(t, "```go {.wide}\nfoo()\n```\n", nil)
	if !strings.Contains(got, `<pre><code class="language-go">foo()`) {
		t.Errorf("braced-attribute fence rendering changed\ngot: %s", got)
	}
	if strings.Contains(got, "wide") {
		t.Errorf("braced attribute leaked into output unexpectedly\ngot: %s", got)
	}
}
