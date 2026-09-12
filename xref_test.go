package md2html

import (
	"strings"
	"testing"
)

// The transform needs ids, so these run the pair in the real order.
func xref(t *testing.T, in string) string {
	t.Helper()
	root, err := parseFragment([]byte(in))
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	for _, tr := range []Transform{HeadingAnchors(), SectionLinks()} {
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

func TestSectionLinksResolvesAgainstNumberedHeading(t *testing.T) {
	got := xref(t, `<h2>4.2 Rollback</h2><p>see §4.2 for detail</p>`)
	if !strings.Contains(got, `<a class="xref" href="#42-rollback">§4.2</a>`) {
		t.Errorf("not autolinked\ngot: %s", got)
	}
}

func TestSectionLinksResolvesSingleLevelNumber(t *testing.T) {
	got := xref(t, `<h2>7 Appendix</h2><p>see §7</p>`)
	if !strings.Contains(got, `href="#7-appendix"`) {
		t.Errorf("single-level number not resolved\ngot: %s", got)
	}
}

// No matching heading means no link — a dangling anchor is worse than
// plain text, because it looks clickable and goes nowhere.
func TestSectionLinksLeavesUnmatchedNumberLiteral(t *testing.T) {
	got := xref(t, `<h2>4.2 Rollback</h2><p>see §9.9</p>`)
	if strings.Contains(got, "xref") {
		t.Errorf("linked an unmatched number\ngot: %s", got)
	}
	if !strings.Contains(got, "§9.9") {
		t.Errorf("lost the literal text\ngot: %s", got)
	}
}

// A reference explicitly scoped to another document must stay literal.
func TestSectionLinksSkipsPossessiveCrossDocumentReference(t *testing.T) {
	got := xref(t, `<h2>7 Appendix</h2><p>see the design doc's §7</p>`)
	if strings.Contains(got, "xref") {
		t.Errorf("linked a cross-document reference\ngot: %s", got)
	}
}

func TestSectionLinksSkipsCodeAndExistingLinks(t *testing.T) {
	got := xref(t, `<h2>7 A</h2><p><code>§7</code> <a href="#z">§7</a></p>`)
	if strings.Contains(got, "xref") {
		t.Errorf("rewrote inside code or a link\ngot: %s", got)
	}
}

// An explicit {#id} on the heading must be what the reference resolves to.
func TestSectionLinksHonorsExplicitHeadingID(t *testing.T) {
	got := xref(t, `<h2 id="rb">4.2 Rollback</h2><p>see §4.2</p>`)
	if !strings.Contains(got, `href="#rb"`) {
		t.Errorf("ignored the explicit id\ngot: %s", got)
	}
}

// A heading whose text happens to start with digits still claims that
// number for the whole document — "2026 in review" claims "2026" — because
// the rule is purely syntactic (digits followed by whitespace or end of
// text), and nothing will write "§2026" unless it does mean that heading.
func TestSectionNumbersRequiresSeparator(t *testing.T) {
	root, err := parseFragment([]byte(`<h2 id="a">4.2 Rollback</h2><h3 id="b">2026 in review</h3><h4 id="c">4.2.1</h4>`))
	if err != nil {
		t.Fatal(err)
	}
	m := sectionNumbers(root)
	if m["4.2"] != "a" {
		t.Errorf(`m["4.2"] = %q, want "a"`, m["4.2"])
	}
	if m["2026"] != "b" {
		t.Errorf(`m["2026"] = %q, want "b"`, m["2026"])
	}
	// A heading that is only a number still counts: nothing follows it.
	if m["4.2.1"] != "c" {
		t.Errorf(`m["4.2.1"] = %q, want "c"`, m["4.2.1"])
	}
}

// Two headings claiming one number is an authoring error; the first wins,
// deterministically, rather than whichever the map iteration reached last.
func TestSectionNumbersFirstHeadingWins(t *testing.T) {
	root, err := parseFragment([]byte(`<h2 id="a">3 One</h2><h2 id="b">3 Two</h2>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := sectionNumbers(root)["3"]; got != "a" {
		t.Errorf("got %q, want %q", got, "a")
	}
}
