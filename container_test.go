package md2html

import (
	"strings"
	"testing"
)

// convert is a whole-pipeline helper: container recognition depends on
// exactly how goldmark-fences shapes the tree, so these tests must start
// from Markdown, not from hand-written HTML.
func convert(t *testing.T, src string, warn func(string)) string {
	t.Helper()
	out, err := Convert([]byte(src), Options{Fragment: true, CSS: "/**/", Warn: warn})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return string(out)
}

// The documented braced form must not change. This is the regression guard
// for every existing document in every corpus.
func TestContainerBracedCalloutUnchanged(t *testing.T) {
	got := convert(t, "::: {.callout}\nhello\n:::\n", nil)
	if !strings.Contains(got, `<div class="callout">`) {
		t.Errorf("braced callout changed\ngot: %s", got)
	}
}

// The trap docs/authoring.md warns about, fixed.
func TestContainerBareNameGetsClass(t *testing.T) {
	got := convert(t, "::: callout\nhello\n:::\n", nil)
	if !strings.Contains(got, `<div class="callout">`) {
		t.Errorf("bare name did not become a callout\ngot: %s", got)
	}
	if strings.Contains(got, "callout\nhello") || strings.Contains(got, ">callout") {
		t.Errorf("kind word left in the body\ngot: %s", got)
	}
}

func TestContainerWarningKindGetsVariantClass(t *testing.T) {
	got := convert(t, "::: warning\ncareful\n:::\n", nil)
	if !strings.Contains(got, `class="callout callout-warning"`) {
		t.Errorf("no warning variant\ngot: %s", got)
	}
}

func TestContainerCardKind(t *testing.T) {
	got := convert(t, "::: card\nhighlighted\n:::\n", nil)
	if !strings.Contains(got, `<div class="card">`) {
		t.Errorf("no card\ngot: %s", got)
	}
}

// A braced kind from the shipped vocabulary gets the same treatment as the
// bare spelling, so the two forms cannot drift apart.
func TestContainerBracedWarningGetsVariantClass(t *testing.T) {
	got := convert(t, "::: {.warning}\ncareful\n:::\n", nil)
	if !strings.Contains(got, `class="callout callout-warning"`) {
		t.Errorf("braced warning not normalized\ngot: %s", got)
	}
}

// The actual problem the spec names: silence. An unknown bare name must
// say so.
func TestContainerUnknownBareNameWarns(t *testing.T) {
	var msgs []string
	got := convert(t, "::: kaution\noops\n:::\n", func(m string) { msgs = append(msgs, m) })
	var found bool
	for _, m := range msgs {
		if strings.Contains(m, "kaution") {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning for unknown kind, got %v", msgs)
	}
	// Still degrades to an unclassed div, exactly as today.
	if !strings.Contains(got, "<div>") {
		t.Errorf("unknown kind did not degrade to a bare div\ngot: %s", got)
	}
}

// An arbitrary braced class stays as inert as it has always been, and must
// not warn: the author chose that class deliberately and supplies their own
// CSS for it.
func TestContainerArbitraryBracedClassIsSilentAndPreserved(t *testing.T) {
	var msgs []string
	got := convert(t, "::: {.house-style}\nx\n:::\n", func(m string) { msgs = append(msgs, m) })
	if len(msgs) != 0 {
		t.Errorf("warned about an explicit class: %v", msgs)
	}
	if !strings.Contains(got, `<div class="house-style">`) {
		t.Errorf("explicit class not preserved\ngot: %s", got)
	}
}

// A braced container carrying only an id is the braced form, so its first
// word must never be sniffed as a kind.
func TestContainerIdOnlyIsNotSniffed(t *testing.T) {
	var msgs []string
	got := convert(t, "::: {#note}\ncallout is a word\n:::\n", func(m string) { msgs = append(msgs, m) })
	if len(msgs) != 0 {
		t.Errorf("warned about an id-only container: %v", msgs)
	}
	if !strings.Contains(got, "callout is a word") {
		t.Errorf("body text was eaten\ngot: %s", got)
	}
}

// The fence library's internal bookkeeping must not reach the page.
func TestContainerDropsDataFenceAttribute(t *testing.T) {
	got := convert(t, "::: {.callout}\nx\n:::\n", nil)
	if strings.Contains(got, "data-fence") {
		t.Errorf("data-fence leaked into output\ngot: %s", got)
	}
}

// Nesting already works and must keep working.
func TestContainerNestingPreserved(t *testing.T) {
	got := convert(t, ":::: callout\n::: warning\ninner\n:::\n::::\n", nil)
	if !strings.Contains(got, `<div class="callout">`) ||
		!strings.Contains(got, `class="callout callout-warning"`) {
		t.Errorf("nesting broken\ngot: %s", got)
	}
}

func TestFirstWord(t *testing.T) {
	cases := []struct{ in, word, rest string }{
		{"warning", "warning", ""},
		{"warning Be careful", "warning", "Be careful"},
		{"warning\nbody", "warning", "\nbody"},
		{"  warning  x", "warning", "x"},
		{"", "", ""},
	}
	for _, c := range cases {
		w, r := firstWord(c.in)
		if w != c.word || r != c.rest {
			t.Errorf("firstWord(%q) = (%q, %q), want (%q, %q)", c.in, w, r, c.word, c.rest)
		}
	}
}

// R28: a documented braced form combining an id with several classes must
// keep every class, not just the kind's own — goldmark-fences merges
// {#note .callout .compact} into class="callout compact", and applyKind
// must not clobber that down to just "callout".
func TestContainerBracedKindKeepsExtraClassesAndId(t *testing.T) {
	got := convert(t, "::: {#note .callout .compact}\nhello\n:::\n", nil)
	if !strings.Contains(got, `id="note"`) {
		t.Errorf("id dropped\ngot: %s", got)
	}
	if !strings.Contains(got, `class="callout compact"`) {
		t.Errorf("compact class dropped\ngot: %s", got)
	}
}

func TestContainerBareTitleBecomesTitleParagraph(t *testing.T) {
	got := convert(t, "::: callout Read this first\nbody text\n:::\n", nil)
	if !strings.Contains(got, `<p class="container-title">Read this first</p>`) {
		t.Errorf("no title paragraph\ngot: %s", got)
	}
	if !strings.Contains(got, "body text") {
		t.Errorf("body lost\ngot: %s", got)
	}
	if strings.Contains(got, "Read this first\nbody text") {
		t.Errorf("title not separated from body\ngot: %s", got)
	}
}

// Inline Markdown in the title is already parsed by the time the transform
// sees it, so it survives as markup rather than as escaped text. R14: the
// space between the code span and the following word must survive too —
// only the title's outer edges get trimmed, never whitespace between
// inline nodes — so this asserts the full rendered title, not just that a
// <code> element is present somewhere in it.
func TestContainerTitleKeepsInlineMarkdown(t *testing.T) {
	got := convert(t, "::: callout Read `this` first\nbody\n:::\n", nil)
	want := `<p class="container-title">Read <code>this</code> first</p>`
	if !strings.Contains(got, want) {
		t.Errorf("title malformed\nwant substring: %s\ngot: %s", want, got)
	}
}

// No title on the fence line means no title paragraph — not an empty one.
func TestContainerNoTitleEmitsNoTitleParagraph(t *testing.T) {
	got := convert(t, "::: callout\nbody\n:::\n", nil)
	if strings.Contains(got, "container-title") {
		t.Errorf("emitted an empty title\ngot: %s", got)
	}
}

// A title with a blank line after it leaves the first paragraph empty; it
// must be removed rather than rendered as <p></p>.
func TestContainerTitleWithBlankLineDropsEmptyParagraph(t *testing.T) {
	got := convert(t, "::: callout Heads up\n\nbody\n:::\n", nil)
	if strings.Contains(got, "<p></p>") {
		t.Errorf("empty paragraph left behind\ngot: %s", got)
	}
	if !strings.Contains(got, `<p class="container-title">Heads up</p>`) {
		t.Errorf("no title\ngot: %s", got)
	}
}

// The braced form keeps today's semantics exactly: its first line is body,
// because nothing can tell it from a title.
func TestContainerBracedFormNeverLiftsATitle(t *testing.T) {
	got := convert(t, "::: {.callout}\nfirst line\nsecond line\n:::\n", nil)
	if strings.Contains(got, "container-title") {
		t.Errorf("braced form lifted a title\ngot: %s", got)
	}
}

func TestContainerAsideIsCollapsible(t *testing.T) {
	got := convert(t, "::: aside Why this matters\nbecause\n:::\n", nil)
	if !strings.Contains(got, `<details class="container aside">`) {
		t.Errorf("not a details element\ngot: %s", got)
	}
	if !strings.Contains(got, "<summary>Why this matters</summary>") {
		t.Errorf("no summary\ngot: %s", got)
	}
	if !strings.Contains(got, "because") {
		t.Errorf("body lost\ngot: %s", got)
	}
}

// A collapsible kind with no title still needs a summary, or the disclosure
// triangle has nothing to label it.
func TestContainerAsideWithoutTitleUsesFallbackSummary(t *testing.T) {
	got := convert(t, "::: aside\nbecause\n:::\n", nil)
	if !strings.Contains(got, "<summary>Aside</summary>") {
		t.Errorf("no fallback summary\ngot: %s", got)
	}
}

// The example kind is the same shape with a different label prefix.
func TestContainerExamplePrefixesItsSummary(t *testing.T) {
	got := convert(t, "::: example Rolling back\nsteps\n:::\n", nil)
	if !strings.Contains(got, `<details class="container example">`) {
		t.Errorf("not a details element\ngot: %s", got)
	}
	if !strings.Contains(got, "<summary>Example — Rolling back</summary>") {
		t.Errorf("no prefixed summary\ngot: %s", got)
	}
}

func TestContainerExampleWithoutTitleIsJustThePrefix(t *testing.T) {
	got := convert(t, "::: example\nsteps\n:::\n", nil)
	if !strings.Contains(got, "<summary>Example</summary>") {
		t.Errorf("no prefix-only summary\ngot: %s", got)
	}
}

// R7: braced collapsible kinds get the same <details> treatment as bare
// ones — the plan calls for the shipped kinds to behave symmetrically in
// both spellings, and only the frozen callout keeps its div. The braced
// form never lifts a title (see TestContainerBracedFormNeverLiftsATitle),
// so the summary falls back exactly as a titleless bare aside's would.
func TestContainerBracedAsideIsCollapsible(t *testing.T) {
	got := convert(t, "::: {.aside}\nSomething\n:::\n", nil)
	if !strings.Contains(got, `<details class="container aside">`) {
		t.Errorf("braced aside is not a details element\ngot: %s", got)
	}
	if !strings.Contains(got, "<summary>Aside</summary>") {
		t.Errorf("no fallback summary\ngot: %s", got)
	}
	if !strings.Contains(got, "Something") {
		t.Errorf("body lost\ngot: %s", got)
	}
}

// The directive label form: :::kind[Title]. Delimiting the title is what
// lets a titled container also carry an attribute block, which the
// undelimited form structurally cannot.
func TestContainerLabelFormOnCollapsibleKind(t *testing.T) {
	got := convert(t, ":::aside[Why this matters]\nBody.\n:::\n", nil)
	if !strings.Contains(got, "<summary>Why this matters</summary>") {
		t.Errorf("label not lifted into the summary\ngot: %s", got)
	}
	if !strings.Contains(got, `class="container aside"`) {
		t.Errorf("kind classes missing\ngot: %s", got)
	}
	if strings.Contains(got, "aside[") {
		t.Errorf("fence line left in the body\ngot: %s", got)
	}
}

func TestContainerLabelFormOnDivKind(t *testing.T) {
	got := convert(t, ":::callout[A note]\nBody.\n:::\n", nil)
	if !strings.Contains(got, `<p class="container-title">A note</p>`) {
		t.Errorf("label not lifted into a title paragraph\ngot: %s", got)
	}
}

// The whole reason to adopt the form: a titled container that also carries
// classes and an id, which "::: aside Title" cannot express.
func TestContainerLabelFormWithAttributeBlock(t *testing.T) {
	got := convert(t, ":::aside[Why]{#w .compact}\nBody.\n:::\n", nil)
	if !strings.Contains(got, "<summary>Why</summary>") {
		t.Errorf("label lost when an attribute block follows\ngot: %s", got)
	}
	if !strings.Contains(got, `id="w"`) || !strings.Contains(got, "compact") {
		t.Errorf("attribute block not applied\ngot: %s", got)
	}
	// Regression: the bracketed-span transform must not claim the label.
	if strings.Contains(got, `<span class="compact">`) {
		t.Errorf("bracketed spans consumed the container label\ngot: %s", got)
	}
}

func TestContainerLabelFormKeepsInlineMarkupInLabel(t *testing.T) {
	got := convert(t, ":::aside[Why `code` matters]\nBody.\n:::\n", nil)
	if !strings.Contains(got, "<summary>Why <code>code</code> matters</summary>") {
		t.Errorf("inline markup in the label not preserved\ngot: %s", got)
	}
}

func TestContainerLabelFormEmptyLabelUsesFallback(t *testing.T) {
	got := convert(t, ":::aside[]\nBody.\n:::\n", nil)
	if !strings.Contains(got, "<summary>Aside</summary>") {
		t.Errorf("empty label did not fall back\ngot: %s", got)
	}
}

// An unknown kind in the label form warns like the bare form does.
func TestContainerLabelFormUnknownKindWarns(t *testing.T) {
	var warnings []string
	convert(t, ":::housestyle[Title]\nBody.\n:::\n", func(s string) { warnings = append(warnings, s) })
	if len(warnings) == 0 || !strings.Contains(warnings[0], "housestyle") {
		t.Errorf("unknown kind in label form did not warn: %v", warnings)
	}
}

// The undelimited form keeps working unchanged.
func TestContainerBareTitleFormStillWorks(t *testing.T) {
	got := convert(t, "::: aside Why this matters\nBody.\n:::\n", nil)
	if !strings.Contains(got, "<summary>Why this matters</summary>") {
		t.Errorf("bare title form regressed\ngot: %s", got)
	}
}

// Case D from the spec: a definition list as the first block used to make
// the label form's whole fence line a <dt>.
func TestContainerLabelFormBeforeDefinitionList(t *testing.T) {
	var warns []string
	got := convert(t, ":::card[Numbers]\nterm\n: def\n:::\n",
		func(s string) { warns = append(warns, s) })
	if len(warns) != 0 {
		t.Errorf("warned: %v\ngot: %s", warns, got)
	}
	if !strings.Contains(got, `<div class="card">`) {
		t.Errorf("no card class\ngot: %s", got)
	}
	if !strings.Contains(got, `<p class="container-title">Numbers</p>`) {
		t.Errorf("title missing\ngot: %s", got)
	}
	if strings.Contains(got, "<dt>card") {
		t.Errorf("fence line was captured by the definition list\ngot: %s", got)
	}
	if !strings.Contains(got, "<dt>term</dt>") {
		t.Errorf("definition list lost\ngot: %s", got)
	}
}

// The title is Markdown, parsed by goldmark rather than re-converted.
func TestContainerLabelFormTitleKeepsInlineMarkup(t *testing.T) {
	got := convert(t, ":::card[Why `code` matters]\nbody\n:::\n", nil)
	if !strings.Contains(got, "<code>code</code>") {
		t.Errorf("title inline markup lost\ngot: %s", got)
	}
}

// A collapsible kind puts the parsed title in its summary.
func TestContainerLabelFormSummaryUsesTitle(t *testing.T) {
	got := convert(t, ":::aside[Why this matters]{#w .compact}\nbody\n:::\n", nil)
	if !strings.Contains(got, "<summary>Why this matters</summary>") {
		t.Errorf("summary does not carry the title\ngot: %s", got)
	}
	if !strings.Contains(got, `id="w"`) || !strings.Contains(got, "compact") {
		t.Errorf("label attributes lost\ngot: %s", got)
	}
}

// The other half of TestParseFenceInfoUnclosedBracketIsNotALabel: a fence
// line the grammar declines stays in the content stream, so the bare-word
// path still names its first word in the existing warning and the author's
// text still reaches the output rather than being silently consumed.
func TestContainerUnclosedLabelBracketFallsThrough(t *testing.T) {
	var warns []string
	got := convert(t, ":::aside[Why this matters\nbody\n:::\n",
		func(s string) { warns = append(warns, s) })
	if len(warns) != 1 || !strings.Contains(warns[0], `unknown container kind "aside[Why"`) {
		t.Errorf("warning changed: %v\ngot: %s", warns, got)
	}
	if !strings.Contains(got, "aside[Why this matters") {
		t.Errorf("declined fence line was consumed\ngot: %s", got)
	}
}

// A fence line behind tab-expanded indentation. PeekLine hands the parser a
// padded line, so a title segment computed without subtracting the padding
// slides right by the pad width — ">\t:::card[Numbers]" rendered a title of
// "mbers]" — and at the end of the buffer reads past the source entirely.
func TestContainerLabelFormTitleInIndentedBlockquote(t *testing.T) {
	got := convert(t, ">\t:::card[Numbers]\n>\tterm\n>\t: def\n>\t:::\n", nil)
	if !strings.Contains(got, `<p class="container-title">Numbers</p>`) {
		t.Errorf("padded fence line shifted the title\ngot: %s", got)
	}
	if !strings.Contains(got, "<dt>term</dt>") {
		t.Errorf("definition list lost\ngot: %s", got)
	}
}

// The same, with no trailing newline: the offsets then run past len(source)
// and splice the reader's buffer slack into the title.
func TestContainerLabelFormTitleInIndentedBlockquoteAtEOF(t *testing.T) {
	got := convert(t, ">\t:::card[Numbers]", nil)
	if !strings.Contains(got, `<p class="container-title">Numbers</p>`) {
		t.Errorf("title read past the end of the source\ngot: %s", got)
	}
}

// An unknown kind in the label form names just the kind word, because the
// parser split the fence line before the transform ever saw it. It used to
// name the whole remainder, "housestyle[Title]".
//
// The refused container carries no kind classes, so its title must not keep
// container-title either: that would be a styling hook for a container that
// was not built. The author's words stay, as ordinary prose.
func TestContainerLabelFormUnknownKindWarnsOnKindAlone(t *testing.T) {
	var warns []string
	got := convert(t, ":::housestyle[Title]\nBody.\n:::\n",
		func(s string) { warns = append(warns, s) })
	if len(warns) != 1 || !strings.Contains(warns[0], `unknown container kind "housestyle"`) {
		t.Errorf("warning does not name the kind alone: %v", warns)
	}
	if strings.Contains(got, "container-title") {
		t.Errorf("refused container kept a title styling hook\ngot: %s", got)
	}
	if !strings.Contains(got, "<p>Title</p>") {
		t.Errorf("author's title text lost\ngot: %s", got)
	}
	if !strings.Contains(got, "<p>Body.</p>") {
		t.Errorf("body lost\ngot: %s", got)
	}
}

// A key=value pair in a label form's attribute block reaches the div. The
// old paragraph-mining path applied only id and class and dropped every
// other pair on the floor.
func TestContainerLabelFormKeyValueAttributesReachTheDiv(t *testing.T) {
	got := convert(t, `:::card[T]{data-sort="name"}`+"\nbody\n:::\n", nil)
	if !strings.Contains(got, `data-sort="name"`) {
		t.Errorf("key=value attribute dropped\ngot: %s", got)
	}
}

// The same family as the definition-list case: a setext underline on the
// next line used to reinterpret the fence line as a heading, which also
// hijacked the page title.
func TestContainerLabelFormBeforeSetextUnderline(t *testing.T) {
	var warns []string
	got := convert(t, ":::card[Under]\n===\n:::\n",
		func(s string) { warns = append(warns, s) })
	if len(warns) != 0 {
		t.Errorf("warned: %v\ngot: %s", warns, got)
	}
	if strings.Contains(got, "<h1") {
		t.Errorf("fence line was captured by the setext underline\ngot: %s", got)
	}
	if !strings.Contains(got, `<p class="container-title">Under</p>`) {
		t.Errorf("title missing\ngot: %s", got)
	}
}

// A titleless owned container must not adopt an author's own
// <p class="container-title"> as its title. ":::card[]" is such a container
// — the empty bracket is a label form with no title — and md2html renders
// with WithUnsafe, so that paragraph comes straight from the document.
//
// Adoption is not merely cosmetic: the paragraph is torn down and its
// children re-hung, so the author's own attributes on it are lost. The id is
// asserted for exactly that reason.
func TestContainerDoesNotAdoptAuthorWrittenTitleParagraph(t *testing.T) {
	got := convert(t, ":::card[]\n<p class=\"container-title\" id=\"keepme\">mine</p>\n\nbody\n:::\n", nil)
	if !strings.Contains(got, `<p class="container-title" id="keepme">mine</p>`) {
		t.Errorf("author's own title paragraph was adopted and rebuilt\ngot: %s", got)
	}
	for _, internal := range []string{"data-fence", "data-fence-kind", "data-fence-title"} {
		if strings.Contains(got, internal) {
			t.Errorf("%s reached the output\ngot: %s", internal, got)
		}
	}
}

// The same, with the marker spelled out in the document. The parser reserves
// the data-fence namespace, so an author cannot hand themselves the flag
// that makes a container adopt their paragraph.
func TestContainerForgedTitleMarkerDoesNotAdoptAParagraph(t *testing.T) {
	got := convert(t, ":::card[]{data-fence-title=\"1\"}\n<p class=\"container-title\" id=\"keepme\">mine</p>\n\nbody\n:::\n", nil)
	if !strings.Contains(got, `<p class="container-title" id="keepme">mine</p>`) {
		t.Errorf("forged marker made the container adopt the paragraph\ngot: %s", got)
	}
	if strings.Contains(got, "data-fence") {
		t.Errorf("forged data-fence attribute reached the output\ngot: %s", got)
	}
}

// A forged data-fence replaces the random id Open generated, so Continue
// finds no matching fenceData, leaves flevel at len(fdataMap) and indexes
// out of range. That panicked the converter — an author could crash a build
// with three lines of Markdown. It reproduces on the upstream extension, so
// it predates the vendoring; the namespace guard closes it.
//
// A panic here fails the package outright rather than this test alone, which
// is the correct severity: convert() returning at all is half the assertion.
func TestContainerForgedFenceIdDoesNotPanic(t *testing.T) {
	for _, src := range []string{
		":::card[T]{data-fence=\"x\"}\nbody\n:::\n",          // owned label path
		"::: {.card data-fence=\"x\"}\nbody\n:::\n",          // braced path, via ParseAttributes
		":::card[T]{data-fence-kind=\"aside\"}\nbody\n:::\n", // forged kind
	} {
		got := convert(t, src, nil)
		if strings.Contains(got, "data-fence") {
			t.Errorf("%q: reserved attribute reached the output\ngot: %s", src, got)
		}
		if !strings.Contains(got, `class="card"`) {
			t.Errorf("%q: container lost its kind\ngot: %s", src, got)
		}
	}
}

// The spec's silent failure: a title on a braced fence was absorbed into
// the body, and a collapsible kind showed its fallback instead.
func TestContainerBracedFormTakesATitle(t *testing.T) {
	got := convert(t, "::: {.card} Why this matters\nbody\n:::\n", nil)
	if !strings.Contains(got, `<p class="container-title">Why this matters</p>`) {
		t.Errorf("title not recognized\ngot: %s", got)
	}
	if strings.Contains(got, "Why this matters\nbody") {
		t.Errorf("title still absorbed into the body\ngot: %s", got)
	}
}

func TestContainerBracedCollapsibleTakesATitle(t *testing.T) {
	got := convert(t, "::: {.aside} Why this matters\nbody\n:::\n", nil)
	if !strings.Contains(got, "<summary>Why this matters</summary>") {
		t.Errorf("summary shows the fallback, not the title\ngot: %s", got)
	}
	if strings.Contains(got, "<summary>Aside</summary>") {
		t.Errorf("fallback label still used\ngot: %s", got)
	}
}

// Case G stays correct, and an unknown braced class stays inert and silent.
func TestContainerBracedUnknownClassStillInert(t *testing.T) {
	var warns []string
	got := convert(t, "::: {.house-style}\nterm\n: def\n:::\n",
		func(s string) { warns = append(warns, s) })
	if len(warns) != 0 {
		t.Errorf("warned on a deliberate custom class: %v", warns)
	}
	if !strings.Contains(got, `<div class="house-style">`) {
		t.Errorf("custom class lost\ngot: %s", got)
	}
}

// The braced form creates titleless owned containers in quantity for the
// first time. The data-fence-title gate exists precisely so a raw
// <p class="container-title"> the author wrote as a braced container's
// first block is never mistaken for a parser-emitted title — see
// TestContainerDoesNotAdoptAuthorWrittenTitleParagraph for the label-form
// counterpart. This is the braced-form exercise of that same guard.
func TestContainerBracedFormDoesNotAdoptAuthorWrittenTitleParagraph(t *testing.T) {
	got := convert(t, "::: {.card}\n<p class=\"container-title\" id=\"keepme\">mine</p>\n\nbody\n:::\n", nil)
	if !strings.Contains(got, `<p class="container-title" id="keepme">mine</p>`) {
		t.Errorf("author's own title paragraph was adopted and rebuilt\ngot: %s", got)
	}
	for _, internal := range []string{"data-fence", "data-fence-kind", "data-fence-title"} {
		if strings.Contains(got, internal) {
			t.Errorf("%s reached the output\ngot: %s", internal, got)
		}
	}
}
