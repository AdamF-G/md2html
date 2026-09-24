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

// A caption with no language must still get the figure wrapper, and the
// bare <code> (no language class) inside it must look exactly like a
// languageless fence's <code> would on its own.
func TestCodeFenceCaptionWithoutLanguage(t *testing.T) {
	got := convert(t, "```caption=\"x\"\nfoo()\n```\n", nil)
	if !strings.Contains(got, `<figure class="code-figure"><figcaption>x</figcaption><pre><code>foo()`) {
		t.Errorf("caption without language did not wrap correctly\ngot: %s", got)
	}
	if strings.Contains(got, `<code class="language-`) {
		t.Errorf("languageless caption fence got a language class anyway\ngot: %s", got)
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

// Documented in docs/authoring.md: a caption is re-parsed like any other
// prose, so a chip token or a "§" reference inside one resolves through the
// same inline rewriters as body text, not as literal escaped brackets.
func TestCodeFenceCaptionRunsThroughChips(t *testing.T) {
	got := convert(t, "```go caption=\"[proven] auth handler\"\nx\n```\n", nil)
	if !strings.Contains(got, `<figcaption><span class="chip chip-proven">proven</span> auth handler</figcaption>`) {
		t.Errorf("chip in caption not rendered as a badge\ngot: %s", got)
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

// goldmark's own fenced-code-block renderer does not honor a trailing
// {...} attribute today. CodeAttributeFilter (renderer/html) is wired up
// for inline code spans (renderCodeSpan), not for renderFencedCodeBlock —
// confirmed by reading goldmark's source and by capturing this exact output
// (byte for byte, via convert()) before this file's renderer override
// existed. A braced attribute used to be silently swallowed, the same way
// an unhandled caption was.
//
// That is no longer the behavior: the braced block is Pandoc's
// fenced_code_attributes and is now parsed, so .wide reaches the element.
// The exact-output discipline is kept — a substring check would also pass
// if the wrapping changed shape entirely — with want updated to the
// rendering the braced form is now specified to produce.
func TestCodeFenceBracedAttributeIsParsed(t *testing.T) {
	got := convert(t, "```go {.wide}\nfoo()\n```\n", nil)
	want := Marker() + "\n" +
		"<title>Untitled</title>\n<style>\n/**/\n</style>\n" +
		"<pre><code class=\"language-go wide\">foo()\n</code></pre>\n\n"
	if got != want {
		t.Errorf("braced-attribute fence rendering changed\ngot:  %q\nwant: %q", got, want)
	}
}

// Pandoc's fenced_code_attributes, which md2html previously mangled: the
// braced form yielded class="language-{.go" and a caption of `server.go"}`.
func TestSplitFenceInfoBracedForm(t *testing.T) {
	cases := []struct{ in, lang, caption string }{
		{`{.go caption="server.go"}`, "go", "server.go"},
		{`go {caption="server.go"}`, "go", "server.go"},
		{`{.go}`, "go", ""},
		{`{caption="just a caption"}`, "", "just a caption"},
		{`{.go caption="a } brace inside"}`, "go", "a } brace inside"},
		// The escape the old hand-rolled tokenizer could not express.
		{`{.go caption="has \"quote\" inside"}`, "go", `has "quote" inside`},
		// A head word wins over a class as the language, since that is
		// where a reader looks first.
		{`go {.wide}`, "go", ""},
	}
	for _, c := range cases {
		lang, caption := splitFenceInfo(c.in)
		if lang != c.lang || caption != c.caption {
			t.Errorf("splitFenceInfo(%q) = (%q, %q), want (%q, %q)",
				c.in, lang, caption, c.lang, c.caption)
		}
	}
}

func TestCodeFenceBracedCaptionRendersAsFigcaption(t *testing.T) {
	got := convert(t, "```{.go caption=\"server.go\"}\nfoo()\n```\n", nil)
	if !strings.Contains(got, "<figcaption>server.go</figcaption>") {
		t.Errorf("braced caption not rendered\ngot: %s", got)
	}
	if !strings.Contains(got, `class="language-go"`) {
		t.Errorf("language lost in braced form\ngot: %s", got)
	}
}

// An extra class in the block is the author's own styling hook and must
// reach the element, not be swallowed.
func TestCodeFenceBracedExtraClassReachesElement(t *testing.T) {
	got := convert(t, "```go {.wide}\nfoo()\n```\n", nil)
	if !strings.Contains(got, `class="language-go wide"`) {
		t.Errorf("extra class not emitted\ngot: %s", got)
	}
}

func TestCodeFenceBracedIDReachesElement(t *testing.T) {
	got := convert(t, "```{#snippet .go}\nfoo()\n```\n", nil)
	if !strings.Contains(got, `id="snippet"`) {
		t.Errorf("id not emitted\ngot: %s", got)
	}
}

// A fig fence keeps working when its kind is written as a class.
func TestFigFenceBracedForm(t *testing.T) {
	got := convert(t, "```{.fig}\nitems:\n  - box: Client\n```\n", nil)
	if !strings.Contains(got, `class="fig-box"`) {
		t.Errorf("braced fig fence did not render as a figure\ngot: %s", got)
	}
}
