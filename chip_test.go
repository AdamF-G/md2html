package md2html

import (
	"strings"
	"testing"
)

func TestChipsRecognizedStatusWords(t *testing.T) {
	for _, word := range []string{"proven", "verified", "designed", "planned", "draft", "deprecated"} {
		got := apply(t, "<p>state ["+word+"] here</p>", Chips())
		want := `<span class="chip chip-` + word + `">` + word + `</span>`
		if !strings.Contains(got, want) {
			t.Errorf("[%s] did not become a chip\ngot: %s", word, got)
		}
	}
}

func TestChipsGenericFreeTextForm(t *testing.T) {
	got := apply(t, `<p>x [c:since v2] y</p>`, Chips())
	if !strings.Contains(got, `<span class="chip">since v2</span>`) {
		t.Errorf("generic chip not rendered\ngot: %s", got)
	}
}

// Anything outside the vocabulary stays literal text. This is what keeps
// ordinary bracketed prose and reference-style link syntax intact.
func TestChipsLeaveUnknownTokensLiteral(t *testing.T) {
	got := apply(t, `<p>see [1] and [some note]</p>`, Chips())
	if !strings.Contains(got, "[1]") || !strings.Contains(got, "[some note]") {
		t.Errorf("rewrote a non-chip token\ngot: %s", got)
	}
}

// An empty label would be an empty badge; leave the source text instead.
func TestChipsEmptyGenericLabelStaysLiteral(t *testing.T) {
	got := apply(t, `<p>x [c:] y</p>`, Chips())
	if !strings.Contains(got, "[c:]") {
		t.Errorf("emitted an empty chip\ngot: %s", got)
	}
}

func TestChipsSkipCodeAndLinks(t *testing.T) {
	got := apply(t, "<p><code>[proven]</code> and <a href=\"#x\">[proven]</a></p>", Chips())
	if strings.Contains(got, "chip") {
		t.Errorf("rewrote inside code or a link\ngot: %s", got)
	}
}

func TestChipsWorkInHeadings(t *testing.T) {
	got := apply(t, `<h2>Rollback [proven]</h2>`, Chips())
	if !strings.Contains(got, `class="chip chip-proven"`) {
		t.Errorf("no chip in heading\ngot: %s", got)
	}
}

// Surrounding text must survive the split intact.
func TestChipsPreserveSurroundingText(t *testing.T) {
	got := apply(t, `<p>before [proven] between [draft] after</p>`, Chips())
	for _, want := range []string{"before ", " between ", " after"} {
		if !strings.Contains(got, want) {
			t.Errorf("lost %q\ngot: %s", want, got)
		}
	}
}

func TestStripChipTokens(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Rollback [proven]", "Rollback"},
		{"[draft] Rollback", "Rollback"},
		{"Rollback [c:since v2] plan", "Rollback plan"},
		{"Rollback [1]", "Rollback [1]"},
		{"Rollback", "Rollback"},
	}
	for _, c := range cases {
		if got := stripChipTokens(c.in); got != c.want {
			t.Errorf("stripChipTokens(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// stripChipTokens must not normalize whitespace it didn't touch: only the
// span where a token was removed may change. Collapsing every run (what
// strings.Fields/Join would do) would make the derived <title> depend on
// whether a document happens to contain a bracket at all.
func TestStripChipTokensPreservesUnrelatedWhitespace(t *testing.T) {
	if got := stripChipTokens("A  B [proven]"); got != "A  B" {
		t.Errorf("stripChipTokens(%q) = %q, want %q", "A  B [proven]", got, "A  B")
	}
}

// The seam merge must be capped at exactly one space on the side following
// the token: a run of spaces there is untouched apart from the one flanking
// space that pairs off with the single space before the token.
func TestStripChipTokensCapsTrailingSpaceSeam(t *testing.T) {
	if got := stripChipTokens("A [proven]  B"); got != "A  B" {
		t.Errorf("stripChipTokens(%q) = %q, want %q", "A [proven]  B", got, "A  B")
	}
}

// Mirror of the trailing case: a multi-space run before the token is left
// alone, and only its single space touching the token merges with the
// single space that follows.
func TestStripChipTokensPreservesLeadingSpaceRun(t *testing.T) {
	if got := stripChipTokens("A  [proven] B"); got != "A  B" {
		t.Errorf("stripChipTokens(%q) = %q, want %q", "A  [proven] B", got, "A  B")
	}
}

// A token at the very start of the string has nothing before it to merge
// with; the leading space it leaves behind is removed by the final trim,
// not by the seam logic.
func TestStripChipTokensAtStringStart(t *testing.T) {
	if got := stripChipTokens("[proven] B"); got != "B" {
		t.Errorf("stripChipTokens(%q) = %q, want %q", "[proven] B", got, "B")
	}
}

// Two tokens back to back, with and without a separating space, must not
// leave a doubled or tripled gap behind either.
func TestStripChipTokensAdjacentTokens(t *testing.T) {
	cases := []struct{ in, want string }{
		{"word [proven][draft] word2", "word word2"},
		{"word [proven] [draft] word2", "word word2"},
	}
	for _, c := range cases {
		if got := stripChipTokens(c.in); got != c.want {
			t.Errorf("stripChipTokens(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Pandoc's bracketed_spans, which md2html previously mangled: the chip
// transform fired on the bracket and left the attribute block on the page
// as literal text.
func TestChipsBracketedSpanStatusWord(t *testing.T) {
	got := apply(t, `<p>x [proven]{.chip} y</p>`, Chips())
	if !strings.Contains(got, `<span class="chip chip-proven">proven</span>`) {
		t.Errorf("bracketed span not rendered\ngot: %s", got)
	}
	if strings.Contains(got, "{.chip}") {
		t.Errorf("attribute block left on the page\ngot: %s", got)
	}
}

func TestChipsBracketedSpanFreeTextLabel(t *testing.T) {
	got := apply(t, `<p>x [needs review]{.chip} y</p>`, Chips())
	if !strings.Contains(got, `<span class="chip">needs review</span>`) {
		t.Errorf("free-text bracketed span not rendered\ngot: %s", got)
	}
}

func TestChipsBracketedSpanKeepsExplicitClasses(t *testing.T) {
	got := apply(t, `<p>x [shipped]{.chip .chip-ok} y</p>`, Chips())
	if !strings.Contains(got, `<span class="chip chip-ok">shipped</span>`) {
		t.Errorf("explicit classes not preserved\ngot: %s", got)
	}
}

// The general Pandoc form, not just chips: any class works.
func TestChipsBracketedSpanArbitraryClass(t *testing.T) {
	got := apply(t, `<p>x [lead in]{.lead} y</p>`, Chips())
	if !strings.Contains(got, `<span class="lead">lead in</span>`) {
		t.Errorf("arbitrary bracketed span not rendered\ngot: %s", got)
	}
}

func TestChipsBracketedSpanCarriesID(t *testing.T) {
	got := apply(t, `<p>x [tag]{#t .chip} y</p>`, Chips())
	if !strings.Contains(got, `id="t"`) {
		t.Errorf("id not emitted\ngot: %s", got)
	}
}

// A bracketed span with no attributes at all is not a span; leave the
// source text alone rather than emit an empty element.
func TestChipsBracketedSpanEmptyBlockStaysLiteral(t *testing.T) {
	got := apply(t, `<p>x [thing]{} y</p>`, Chips())
	if !strings.Contains(got, "[thing]{}") {
		t.Errorf("emitted a span for an empty attribute block\ngot: %s", got)
	}
}

// Ordinary bracketed prose that happens to be followed by a brace-free
// word must stay literal.
func TestChipsBracketedSpanNeedsBraces(t *testing.T) {
	got := apply(t, `<p>see [some note] here</p>`, Chips())
	if strings.Contains(got, "<span") {
		t.Errorf("rewrote a plain bracketed phrase\ngot: %s", got)
	}
}

// A chip token in a page title is stripped before transforms run; the
// bracketed form has to be stripped there too, or a title keeps the braces.
func TestStripChipTokensRemovesBracketedSpan(t *testing.T) {
	if got := stripChipTokens("Rollback [proven]{.chip}"); got != "Rollback" {
		t.Errorf("stripChipTokens = %q, want Rollback", got)
	}
}

// A span that is not a chip is ordinary prose the author wrapped for
// styling. Its words belong in the page title; only the syntax goes.
func TestStripChipTokensKeepsNonChipSpanLabel(t *testing.T) {
	if got := stripChipTokens("Rollback [in depth]{.lead}"); got != "Rollback in depth" {
		t.Errorf("stripChipTokens = %q, want %q", got, "Rollback in depth")
	}
}
