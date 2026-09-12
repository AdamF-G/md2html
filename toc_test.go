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

// R25: a marker in a document with no linkable headings must degrade to
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

// R3: headingText already excludes both chip spans and the anchor link
// HeadingAnchors appends, so a heading whose visible text genuinely ends in
// "#" must not be truncated by any hand-stripping in buildTOC.
func TestTOCPreservesTrailingHashInLabel(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><h2>Sharp C#</h2>`)
	if !strings.Contains(got, `<a href="#sharp-c">Sharp C#</a>`) {
		t.Errorf("trailing # truncated from label\ngot: %s", got)
	}
}
