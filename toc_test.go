package md2html

import (
	"strings"
	"testing"
)

func toc(t *testing.T, in string) string {
	t.Helper()
	root, err := parseFragment([]byte(in))
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	for _, tr := range []Transform{Chips(), HeadingAnchors(), TOC()} {
		if err := tr.Fn(root); err != nil {
			t.Fatalf("%s: %v", tr.Name, err)
		}
	}
	out, err := renderTree(root)
	if err != nil {
		t.Fatalf("renderTree: %v", err)
	}
	return string(out)
}

func TestTOCReplacesMarkerWithNav(t *testing.T) {
	got := toc(t, `<h1>Doc</h1><p>[[toc]]</p><h2>First</h2><h2>Second</h2>`)
	if !strings.Contains(got, `<nav class="toc">`) {
		t.Errorf("no nav\ngot: %s", got)
	}
	if strings.Contains(got, "[[toc]]") {
		t.Errorf("marker left in the body\ngot: %s", got)
	}
	if !strings.Contains(got, `<a href="#first">First</a>`) ||
		!strings.Contains(got, `<a href="#second">Second</a>`) {
		t.Errorf("headings missing from nav\ngot: %s", got)
	}
}

// Flat, not nested: an irregular heading-level jump must not be able to
// produce broken list nesting. Level is carried as a class instead.
func TestTOCIsFlatAndCarriesLevelAsAClass(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><h2>A</h2><h4>B</h4>`)
	if strings.Count(got, "<ol") != 1 {
		t.Errorf("nested lists\ngot: %s", got)
	}
	if !strings.Contains(got, `class="toc-h2"`) || !strings.Contains(got, `class="toc-h4"`) {
		t.Errorf("no level classes\ngot: %s", got)
	}
}

// Link text must match what the reader sees, which means without chips.
func TestTOCExcludesChipsFromLinkText(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><h2>Rollback [proven]</h2>`)
	if !strings.Contains(got, `<a href="#rollback">Rollback</a>`) {
		t.Errorf("chip text leaked into the toc\ngot: %s", got)
	}
}

// A document with no marker must come out byte-identical.
func TestTOCWithoutMarkerIsANoOp(t *testing.T) {
	in := `<h1>Doc</h1><h2>First</h2>`
	root, err := parseFragment([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if err := TOC().Fn(root); err != nil {
		t.Fatal(err)
	}
	out, err := renderTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Errorf("got %s, want %s", out, in)
	}
}

// The marker only counts alone on its own line. Inside a sentence it is
// prose, and inside a code fence it is documentation of this feature.
func TestTOCIgnoresMarkerInProseAndCode(t *testing.T) {
	got := toc(t, `<p>write [[toc]] to get one</p><pre><code>[[toc]]</code></pre><h2>A</h2>`)
	if strings.Contains(got, "<nav") {
		t.Errorf("expanded a marker that was not alone on its line\ngot: %s", got)
	}
}

// The heading the marker sits under is itself a heading; excluding nothing
// keeps the rule simple, and the page title reads fine as the first entry.
func TestTOCIncludesEveryHeadingInOrder(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><h1>Doc</h1><h2>A</h2><h3>B</h3>`)
	iDoc := strings.Index(got, `href="#doc"`)
	iA := strings.Index(got, `href="#a"`)
	iB := strings.Index(got, `href="#b"`)
	if iDoc < 0 || iA < iDoc || iB < iA {
		t.Errorf("entries out of document order\ngot: %s", got)
	}
}

// a marker in a document with no linkable headings must degrade to
// the literal marker text, not to an empty <nav> and not to silent
// deletion — the spec's stated degradation for this feature is "ugly, but
// not misleading," and an empty element is exactly the misleading case the
// global no-empty-element constraint rules out. The marker staying put is
// the reader's only clue that something needs fixing.
func TestTOCWithNoHeadingsLeavesTheMarker(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><p>body</p>`)
	if strings.Contains(got, "<nav") {
		t.Errorf("emitted an empty nav\ngot: %s", got)
	}
	if !strings.Contains(got, "[[toc]]") {
		t.Errorf("marker was removed even though there was nothing to replace it with\ngot: %s", got)
	}
}

// headingText already excludes both chip spans and the anchor link
// HeadingAnchors appends, so a heading whose visible text genuinely ends in
// "#" must not be truncated by any hand-stripping in buildTOC.
func TestTOCPreservesTrailingHashInLabel(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><h2>Sharp C#</h2>`)
	if !strings.Contains(got, `<a href="#sharp-c">Sharp C#</a>`) {
		t.Errorf("trailing # truncated from label\ngot: %s", got)
	}
}

// A real link whose entire visible text happens to be "[[toc]]" must
// survive untouched. Matching on flattened text (textOf) cannot tell this
// apart from the bare marker; matching on the paragraph having a single
// *text-node* child can, since here the paragraph's only child is an <a>
// element, not text.
func TestTOCLeavesRealLinkAlone(t *testing.T) {
	got := toc(t, `<p><a href="http://example.com">[[toc]]</a></p><h2>A</h2>`)
	if strings.Contains(got, "<nav") {
		t.Errorf("ate a real link because its rendered text matched the marker\ngot: %s", got)
	}
	if !strings.Contains(got, `<a href="http://example.com">[[toc]]</a>`) {
		t.Errorf("link was altered or removed\ngot: %s", got)
	}
}

// The single-text-node-child requirement also protects an inline code span
// whose sole content is "[[toc]]": it documents the feature, it does not
// invoke it.
func TestTOCLeavesCodeSpanAlone(t *testing.T) {
	got := toc(t, `<p><code>[[toc]]</code></p><h2>A</h2>`)
	if strings.Contains(got, "<nav") {
		t.Errorf("expanded a marker inside a code span\ngot: %s", got)
	}
	if !strings.Contains(got, `<code>[[toc]]</code>`) {
		t.Errorf("code span was altered or removed\ngot: %s", got)
	}
}

// Two markers in one document each get their own nav — the second is not
// left behind as a stray marker once the first has been expanded.
func TestTOCExpandsEveryMarker(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><h2>A</h2><p>[[toc]]</p><h2>B</h2>`)
	if strings.Count(got, `<nav class="toc">`) != 2 {
		t.Errorf("want two navs, one per marker\ngot: %s", got)
	}
	if strings.Contains(got, "[[toc]]") {
		t.Errorf("a marker was left behind\ngot: %s", got)
	}
}

// Headings can be present without ids — the --no-anchors shape, or simply
// HeadingAnchors not having run — which is a different case from no
// headings at all. TOC must still degrade to the literal marker rather
// than link to headings with nothing to link to.
func TestTOCWithHeadingsButNoIDsLeavesTheMarker(t *testing.T) {
	root, err := parseFragment([]byte(`<p>[[toc]]</p><h2>A</h2>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := TOC().Fn(root); err != nil {
		t.Fatal(err)
	}
	out, err := renderTree(root)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Contains(got, "<nav") {
		t.Errorf("emitted a nav despite no heading having an id\ngot: %s", got)
	}
	if !strings.Contains(got, "[[toc]]") {
		t.Errorf("marker was removed even though no heading had an id\ngot: %s", got)
	}
}

// The other spellings in the wild: [TOC] is Python-Markdown, MkDocs,
// Typora and StackEdit; [[_TOC_]] is GitLab. All name the same thing.
func TestTOCAlternateMarkers(t *testing.T) {
	for _, marker := range []string{"[[toc]]", "[TOC]", "[toc]", "[[TOC]]"} {
		got := convert(t, "# Doc\n\n"+marker+"\n\n## One\n", nil)
		if !strings.Contains(got, `<nav class="toc">`) {
			t.Errorf("%s did not produce a contents list\ngot: %s", marker, got)
		}
		if strings.Contains(got, marker) {
			t.Errorf("%s left in the body\ngot: %s", marker, got)
		}
	}
}

// The marker must still be the paragraph's entire content.
func TestTOCAlternateMarkerMidSentenceStaysLiteral(t *testing.T) {
	got := convert(t, "# Doc\n\nsee [TOC] here\n\n## One\n", nil)
	if strings.Contains(got, `<nav class="toc">`) {
		t.Errorf("mid-sentence marker produced a contents list\ngot: %s", got)
	}
}

// A marker inside a code span documents the feature and must stay literal.
func TestTOCAlternateMarkerInCodeStaysLiteral(t *testing.T) {
	got := convert(t, "# Doc\n\n`[TOC]`\n\n## One\n", nil)
	if strings.Contains(got, `<nav class="toc">`) {
		t.Errorf("marker in a code span produced a contents list\ngot: %s", got)
	}
}

// GitLab's [[_TOC_]] is deliberately not supported: the underscores are
// emphasis delimiters, so goldmark hands this transform a paragraph of
// "[[", <em>TOC</em>, "]]" rather than one text node. Recognizing it would
// mean matching on flattened text, which is exactly what the single-text-
// node guard exists to avoid — it would also match a link whose visible
// text happens to read that way. Pinned so the limitation is discoverable
// rather than surprising.
func TestTOCGitLabMarkerIsNotSupported(t *testing.T) {
	got := convert(t, "# Doc\n\n[[_TOC_]]\n\n## One\n", nil)
	if strings.Contains(got, `<nav class="toc">`) {
		t.Errorf("[[_TOC_]] unexpectedly produced a contents list\ngot: %s", got)
	}
}
