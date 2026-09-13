// Package compat characterises md2html's Markdown dialect against Pandoc's.
//
// It is a characterisation suite, not a conformance suite. Pandoc emits no
// heading anchors, no table scroll wrapper, different footnote ids and
// different task-list markup, so a byte diff would be red permanently and
// disabled within a month. Every assertion here is structural: a tag name,
// a class token, or the presence of author text.
//
// The three buckets are the deliverable, not an implementation detail:
//
//   - agree      both tools produce the same construct
//   - inert      Pandoc passes our extension through without mangling it
//   - degrades   Pandoc renders our extension as a code block, legibly
//
// A construct that moves between buckets is a real change in how portable
// documents written for md2html are, and that is what this suite exists to
// notice.
//
// The reference dialect is commonmark_x — CommonMark plus Pandoc's
// extensions. It is the closest match by measurement: fenced divs,
// bracketed spans, fenced code attributes, header attributes, definition
// lists, footnotes, pipe tables and YAML metadata are all on by default,
// and it slugs a numbered heading the way md2html does, which Pandoc's own
// `markdown` dialect does not.
package compat

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/AdamF-G/md2html"
	"golang.org/x/net/html"
)

// pandocDialect is the single flag this suite claims md2html corresponds
// to. It is deliberately one word: every extension the shared subset needs
// is already on by default in it.
const pandocDialect = "commonmark_x"

func pandoc(t *testing.T, src string) string {
	t.Helper()
	path, err := exec.LookPath("pandoc")
	if err != nil {
		t.Skip("pandoc not on PATH; run `just compat` on a machine that has it")
	}
	cmd := exec.Command(path, "-f", pandocDialect, "-t", "html5")
	cmd.Stdin = strings.NewReader(src)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("pandoc: %v\n%s", err, errb.String())
	}
	return out.String()
}

func ours(t *testing.T, src string) string {
	t.Helper()
	out, err := md2html.Convert([]byte(src), md2html.Options{Fragment: true, CSS: "/**/"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return string(out)
}

// classesOf returns the class tokens of the first element with the given
// tag name, and whether such an element exists at all.
func classesOf(t *testing.T, doc, tag string) ([]string, bool) {
	t.Helper()
	root, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == tag {
			found = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if found == nil {
		return nil, false
	}
	for _, a := range found.Attr {
		if a.Key == "class" {
			return strings.Fields(a.Val), true
		}
	}
	return nil, true
}

func hasClass(classes []string, want string) bool {
	for _, c := range classes {
		if c == want {
			return true
		}
	}
	return false
}

// --- Bucket one: agree -----------------------------------------------
//
// Both tools produce the same construct. md2html may add to it — a heading
// anchor, a table scroll wrapper — but the element and its identifying
// class must match.

func TestAgreeOnConstructs(t *testing.T) {
	cases := []struct {
		name, src, tag string
		class          string // "" means only the tag must match
	}{
		{"fenced div, bare", "::: callout\nBody.\n:::\n", "div", "callout"},
		{"fenced div, braced", "::: {.callout}\nBody.\n:::\n", "div", "callout"},
		{"fenced div, id and classes", "::: {#note .callout .compact}\nB.\n:::\n", "div", "compact"},
		{"heading attributes", "## T {#breaking .lead}\n", "h2", "lead"},
		{"definition list", "Term\n:   The definition.\n", "dl", ""},
		{"pipe table", "| A | B |\n|---|---|\n| 1 | 2 |\n", "table", ""},
		{"bracketed span", "x [proven]{.chip} y\n", "span", "chip"},
		{"github alert", "> [!WARNING]\n> Overwrites state.\n", "div", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pc, pok := classesOf(t, pandoc(t, c.src), c.tag)
			oc, ook := classesOf(t, ours(t, c.src), c.tag)
			if !pok || !ook {
				t.Fatalf("<%s> present in pandoc=%v, md2html=%v", c.tag, pok, ook)
			}
			if c.class == "" {
				return
			}
			if !hasClass(pc, c.class) {
				t.Errorf("pandoc <%s> lacks class %q: %v", c.tag, c.class, pc)
			}
			if !hasClass(oc, c.class) {
				t.Errorf("md2html <%s> lacks class %q: %v", c.tag, c.class, oc)
			}
		})
	}
}

// The heading id both tools compute for a numbered heading. This is the one
// that decides whether an anchor written against one tool resolves in the
// other, and it is why the dialect above is commonmark_x rather than
// Pandoc's `markdown`, which strips the number.
func TestAgreeOnNumberedHeadingSlug(t *testing.T) {
	const src = "## 4.2 Rollback\n"
	for _, doc := range []struct{ name, out string }{
		{"pandoc", pandoc(t, src)},
		{"md2html", ours(t, src)},
	} {
		if !strings.Contains(doc.out, `id="42-rollback"`) {
			t.Errorf("%s did not slug to 42-rollback\ngot: %s", doc.name, doc.out)
		}
	}
}

// Both consume front matter rather than rendering it as body text.
func TestAgreeFrontMatterIsConsumed(t *testing.T) {
	const src = "---\ntitle: Reference\nsubtitle: every new convention\n---\n\n# Reference\n"
	for _, doc := range []struct{ name, out string }{
		{"pandoc", pandoc(t, src)},
		{"md2html", ours(t, src)},
	} {
		if strings.Contains(doc.out, "title: Reference") {
			t.Errorf("%s rendered front matter as body text\ngot: %s", doc.name, doc.out)
		}
	}
}

// --- Bucket two: inert -------------------------------------------------
//
// md2html extensions Pandoc knows nothing about. The requirement is not
// that Pandoc render them, but that it not corrupt them: the author's text
// survives, visible, for a reader to make sense of.

func TestPandocLeavesOurExtensionsInert(t *testing.T) {
	cases := []struct{ name, src, literal string }{
		{"status chip", "state [proven] here\n", "[proven]"},
		{"generic chip", "state [c:needs review] here\n", "[c:needs review]"},
		{"toc marker, double bracket", "# D\n\n[[toc]]\n\n## O\n", "[[toc]]"},
		{"toc marker, single bracket", "# D\n\n[TOC]\n\n## O\n", "[TOC]"},
		{"section cross-reference", "## 4.2 R\n\nSee §4.2 for details.\n", "§4.2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if out := pandoc(t, c.src); !strings.Contains(out, c.literal) {
				t.Errorf("pandoc mangled %s; %q absent\ngot: %s", c.name, c.literal, out)
			}
		})
	}
}

// --- Bucket three: degrades -------------------------------------------
//
// Fences only md2html understands. Pandoc must render them as a code block
// carrying the fence's own name, so a reader sees labelled source rather
// than a broken figure.

func TestPandocDegradesOurFencesToCodeBlocks(t *testing.T) {
	cases := []struct{ name, src, class string }{
		{"fig", "```fig\nitems:\n  - box: Client\n```\n", "fig"},
		{"mermaid", "```mermaid\ngraph LR\n  A --> B\n```\n", "mermaid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			classes, ok := classesOf(t, pandoc(t, c.src), "pre")
			if !ok {
				t.Fatalf("pandoc did not emit a <pre> for a %s fence", c.name)
			}
			if !hasClass(classes, c.class) {
				t.Errorf("pandoc <pre> lacks class %q: %v", c.class, classes)
			}
		})
	}
}

// A braced code fence is the one construct where both tools understand the
// syntax and render it differently: Pandoc as a data-caption attribute on
// a sourceCode div, md2html as a <figure> with a <figcaption>. What has to
// hold is that neither drops the author's two pieces of information.
func TestBracedCodeFencePreservesLanguageAndCaption(t *testing.T) {
	const src = "```{.go caption=\"server.go\"}\nfunc main() {}\n```\n"
	p, o := pandoc(t, src), ours(t, src)
	for _, d := range []struct{ name, out, lang string }{
		{"pandoc", p, "go"},
		{"md2html", o, "language-go"},
	} {
		classes, ok := classesOf(t, d.out, "code")
		if !ok || !hasClass(classes, d.lang) {
			t.Errorf("%s lost the language: %v", d.name, classes)
		}
		if !strings.Contains(d.out, "server.go") {
			t.Errorf("%s lost the caption\ngot: %s", d.name, d.out)
		}
	}
}

// --- The claim this suite backs ---------------------------------------
//
// Task lists are the one documented divergence inside the shared subset:
// commonmark_x does not implement them and +task_lists does not enable
// them on that reader, while md2html follows GFM. Pinned so the README's
// "one documented divergence" stays a measured statement.
func TestTaskListsAreTheDocumentedDivergence(t *testing.T) {
	const src = "- [ ] not done\n- [x] done\n"
	if strings.Contains(pandoc(t, src), "<input") {
		t.Error("pandoc grew task list support; the documented divergence is stale")
	}
	if !strings.Contains(ours(t, src), "<input") {
		t.Error("md2html stopped rendering task lists")
	}
}
