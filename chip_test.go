package md2html

import (
	"regexp"
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
		{`Rollback [beta]{.chip title="a b"}`, "Rollback"},
		{`Rollback [x]{title="a } b"}`, "Rollback x"},
		// A block that stays literal on the page keeps its text in the
		// title too, so the two never disagree.
		{"A [x]{b} B", "A [x]{b} B"},
		{"A [x]{.chip hidden} B", "A [x]{.chip hidden} B"},
		{"A [x]{} B", "A [x]{} B"},
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

// Every chip-* class the guide shows an author writing must be one the
// default stylesheet colours: the guide once showed [shipped]{.chip
// .chip-ok} while no rule named chip-ok, so the example rendered as a
// plain grey chip with no warning.
func TestGuideChipClassesAreStyled(t *testing.T) {
	named := regexp.MustCompile(`\.(chip-[a-z]+)`).FindAllStringSubmatch(string(AuthoringGuide()), -1)
	if len(named) == 0 {
		t.Fatal("the guide names no chip-* class")
	}
	for _, m := range named {
		if !regexp.MustCompile(`\.` + m[1] + `\b[^{]*\{[^}]*color:`).MatchString(defaultCSS) {
			t.Errorf("the guide names .%s but no default stylesheet rule colours it", m[1])
		}
	}
}

// spanHTML renders src between two words of a paragraph through Chips.
func spanHTML(t *testing.T, src string) string {
	t.Helper()
	return apply(t, "<p>x "+src+" y</p>", Chips())
}

// A span keeps the attributes a link block keeps, so a chip can carry a
// hover explanation.
func TestChipsBracketedSpanKeepsAttributes(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"title", `[beta]{.chip title="May change before 1.0."}`,
			`<span class="chip" title="May change before 1.0.">beta</span>`},
		{"data and aria", `[x]{data-sort="name" aria-label="L"}`,
			`<span aria-label="L" data-sort="name">x</span>`},
		{"order", `[x]{#i .c title="t" lang="fr"}`,
			`<span id="i" class="c" lang="fr" title="t">x</span>`},
		{"lowercased", `[x]{.c Title="t" ID="i"}`,
			`<span id="i" class="c" title="t">x</span>`},
		{"disallowed dropped", `[x]{.chip onclick="alert(1)" href="h" title="t"}`,
			`<span class="chip" title="t">x</span>`},
		{"boolean with value", `[x]{.chip hidden=""}`,
			`<span class="chip" hidden="">x</span>`},
		{"status word", `[proven]{.chip title="t"}`,
			`<span class="chip chip-proven" title="t">proven</span>`},
		{"quoted brace", `[x]{title="a } b"}`,
			`<span title="a } b">x</span>`},
	}
	for _, c := range cases {
		if got := spanHTML(t, c.src); !strings.Contains(got, c.want) {
			t.Errorf("%s: %s\nwant: %s\ngot:  %s", c.name, c.src, c.want, got)
		}
	}
}

// Title and title are one attribute. Which value wins must not depend on
// map order.
func TestChipsBracketedSpanSameNameTwoCases(t *testing.T) {
	first := spanHTML(t, `[x]{title="a" Title="b"}`)
	if !strings.Contains(first, `<span title="a">x</span>`) {
		t.Fatalf("got %s, want title=\"a\"", first)
	}
	for i := 0; i < 20; i++ {
		if got := spanHTML(t, `[x]{title="a" Title="b"}`); got != first {
			t.Fatalf("run %d differs:\n%s\n%s", i, first, got)
		}
	}
}

// A bare key is prose or a mistake, never an attribute: Pandoc has no
// bare-key form, and today's span silently ate the braces. The whole
// match stays, so a status word inside it does not become a chip either.
func TestChipsBracketedSpanBareKeyStaysLiteral(t *testing.T) {
	for _, src := range []string{`[a]{b}`, `[x]{b c}`, `[x]{.chip hidden}`, `[proven]{.chip hidden}`} {
		got := spanHTML(t, src)
		if strings.Contains(got, "<span") || !strings.Contains(got, src) {
			t.Errorf("%s did not stay literal\ngot: %s", src, got)
		}
	}
}

// A name no attribute can have would break out of the attribute list if
// written; the block stays as text instead.
//
// on"x has an unclosed quote, so chipRe never matches it; a/b reaches
// spanBlock and is refused there.
func TestChipsBracketedSpanUnsafeNameStaysLiteral(t *testing.T) {
	for _, src := range []string{`[T]{.chip on"x=1}`, `[T]{.chip a/b=1}`} {
		got := spanHTML(t, src)
		if strings.Contains(got, "<span") || !strings.Contains(got, "[T]{.chip ") {
			t.Errorf("%s: unsafe name not left literal\ngot: %s", src, got)
		}
	}
}

// warnings converts src and returns its output and every warning raised.
func warnings(t *testing.T, src string) (string, []string) {
	t.Helper()
	var w []string
	got := convert(t, src, func(m string) { w = append(w, m) })
	return got, w
}

// A block clearly meant as attributes, with a fault the author can fix,
// stays on the page as text and says why.
func TestChipsWarnOnUnsafeName(t *testing.T) {
	_, w := warnings(t, "x [T]{.chip a/b=1} y\n")
	if len(w) != 1 || !strings.Contains(w[0], "is not an attribute name") || !strings.Contains(w[0], "`a/b`") {
		t.Errorf("warnings = %q, want one naming a/b", w)
	}
}

func TestChipsWarnOnBareKeyBesideAttributes(t *testing.T) {
	_, w := warnings(t, "x [s]{.chip hidden} y\n")
	if len(w) != 1 || !strings.Contains(w[0], `hidden=""`) {
		t.Errorf("warnings = %q, want one suggesting hidden=\"\"", w)
	}
}

// Bare words alone are prose, a regex or terminal markup far more often
// than a span, so they stay quiet.
func TestChipsQuietOnBareKeysAlone(t *testing.T) {
	if _, w := warnings(t, "[a]{b} [c]{d e} [f]{}\n"); len(w) != 0 {
		t.Errorf("warnings = %q, want none", w)
	}
}

// A block is taken before Markdown sees it, so \" escapes a quote inside
// a double-quoted value, as attrTokens reads it.
func TestChipsBackslashQuoteInValue(t *testing.T) {
	got, w := warnings(t, "x [s]{.chip title=\"a \\\"b\\\"\"} y\n")
	if !strings.Contains(got, `title="a &#34;b&#34;"`) || len(w) != 0 {
		t.Errorf("escaped quote lost, or warned %q\ngot: %s", w, got)
	}
}

func TestChipsSingleQuotesHoldADoubleQuote(t *testing.T) {
	got, _ := warnings(t, "x [s]{.chip title='a \"b\"'} y\n")
	if !strings.Contains(got, `title="a &#34;b&#34;"`) {
		t.Errorf("double quote lost from a single-quoted value\ngot: %s", got)
	}
}

// A heading's text is read twice, for the page and for its title; only
// the page's reading warns.
func TestChipsWarnOnceInHeading(t *testing.T) {
	var w []string
	if _, err := Convert([]byte("## A [x]{.chip hidden} b\n"), Options{CSS: "/**/", Warn: func(m string) { w = append(w, m) }}); err != nil {
		t.Fatal(err)
	}
	if len(w) != 1 {
		t.Errorf("warnings = %q, want exactly one", w)
	}
}

// The CLI builds its list from Builtins(), whose Chips has no sink;
// Convert has to rebuild it against Options.Warn, as it does for the
// other builtins that warn.
func TestChipsWarnThroughBuiltins(t *testing.T) {
	var w []string
	if _, err := Convert([]byte("x [s]{.chip hidden} y\n"), Options{
		Fragment: true, CSS: "/**/", Transforms: Builtins(),
		Warn: func(m string) { w = append(w, m) },
	}); err != nil {
		t.Fatal(err)
	}
	if len(w) != 1 {
		t.Errorf("warnings = %q, want exactly one", w)
	}
}

// A quote that opens no value is an ordinary character: an apostrophe in
// a bare value, or in a word, must not cost the author the span, or join
// the block to a later one.
func TestChipsBracketedSpanStrayQuote(t *testing.T) {
	cases := []struct{ src, want string }{
		{`[x]{.c title=don't}`, `<span class="c" title="don&#39;t">x</span>`},
		{`[a]{.c data-n=O'Neil}`, `<span class="c" data-n="O&#39;Neil">a</span>`},
	}
	for _, c := range cases {
		if got := spanHTML(t, c.src); !strings.Contains(got, c.want) {
			t.Errorf("%s\nwant: %s\ngot:  %s", c.src, c.want, got)
		}
	}
}

// A stray quote that breaks a block meant as attributes says so, naming
// the block it is in, not a run joined to the next block.
func TestChipsWarnOnStrayQuote(t *testing.T) {
	cases := []struct{ src, warn string }{
		{"x [x]{.c title=\"a\"b\"} y\n", "`b\"`"},
		{"x [x]{it's .c} end. [z]{.c} y\n", "`it's`"},
		{"x [x]{.c title=\"abc} y\n", "no closing quote"},
	}
	for _, c := range cases {
		got, w := warnings(t, c.src)
		if len(w) != 1 || !strings.Contains(w[0], c.warn) || !strings.Contains(w[0], "[x]") {
			t.Errorf("%q: warnings = %q, want one for [x] containing %s", c.src, w, c.warn)
		}
		if strings.Contains(got, `<span class="c">x</span>`) {
			t.Errorf("%q: rendered a span from a broken block\ngot: %s", c.src, got)
		}
	}
}

// The other-quote hint is for a double-quoted value Markdown broke; an
// apostrophe in a word is not one, and the hint would mislead.
func TestChipsApostropheWarningHasNoQuoteHint(t *testing.T) {
	_, w := warnings(t, "x [x]{it's .c} y\n")
	if len(w) != 1 || strings.Contains(w[0], "other kind of quote") {
		t.Errorf("warnings = %q, want one without the quote hint", w)
	}
}

// The last id written wins, whatever its case.
func TestChipsBracketedSpanLastIDWins(t *testing.T) {
	for src, want := range map[string]string{
		`[x]{#b ID="a"}`:     `<span id="a">x</span>`,
		`[x]{ID="a" #b}`:     `<span id="b">x</span>`,
		`[x]{id="b" ID="a"}`: `<span id="a">x</span>`,
		`[x]{ID="a" id="b"}`: `<span id="b">x</span>`,
	} {
		if got := spanHTML(t, src); !strings.Contains(got, want) {
			t.Errorf("%s: want %s\ngot: %s", src, want, got)
		}
	}
}
