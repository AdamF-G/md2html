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

// A blank line between the fence line and the body. Under the old scheme
// the title was mined out of the first paragraph, so this left an empty
// <p></p> behind; now the title never was a paragraph, and the assertion
// worth keeping is that the blank line costs neither the title nor the body.
func TestContainerTitleWithBlankLineKeepsTitleAndBody(t *testing.T) {
	got := convert(t, "::: callout Heads up\n\nbody\n:::\n", nil)
	if !strings.Contains(got, `<p class="container-title">Heads up</p>`) {
		t.Errorf("no title\ngot: %s", got)
	}
	if !strings.Contains(got, "<p>body</p>") {
		t.Errorf("body lost or absorbed\ngot: %s", got)
	}
}

// A braced container takes a title from the rest of its fence line
// (TestContainerBracedFormTakesATitle), and never from the first line of its
// body: nothing distinguishes that line from ordinary prose.
func TestContainerBracedFormTakesNoTitleFromItsBody(t *testing.T) {
	got := convert(t, "::: {.callout}\nfirst line\nsecond line\n:::\n", nil)
	if strings.Contains(got, "container-title") {
		t.Errorf("braced form lifted a title\ngot: %s", got)
	}
	// Both lines in one paragraph, which is the positive form of the same
	// claim: nothing was lifted out of the body at all, not merely nothing
	// wearing the title class.
	if !strings.Contains(got, "<p>first line\nsecond line</p>") {
		t.Errorf("body paragraph was split or rebuilt\ngot: %s", got)
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

// details is the bare collapsible: a titled block's summary is exactly its
// title, with no editorializing prefix the way aside and example add one.
func TestContainerDetailsSummaryIsExactlyItsTitle(t *testing.T) {
	got := convert(t, "::: details Why this matters\nbecause\n:::\n", nil)
	if !strings.Contains(got, `<details class="container">`) {
		t.Errorf("not a details element\ngot: %s", got)
	}
	if !strings.Contains(got, "<summary>Why this matters</summary>") {
		t.Errorf("summary should be exactly the title, no prefix\ngot: %s", got)
	}
	if !strings.Contains(got, "because") {
		t.Errorf("body lost\ngot: %s", got)
	}
}

func TestContainerDetailsWithoutTitleUsesFallbackSummary(t *testing.T) {
	got := convert(t, "::: details\nbecause\n:::\n", nil)
	if !strings.Contains(got, "<summary>Details</summary>") {
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

// The other half of TestParseFenceInfoUnclosedBracketIsNotALabel: an
// unclosed bracket is not a label form, so the bare path claims the line.
// Its first word — "aside[Why" — becomes the (unknown) kind and fires the
// existing warning; the rest of the fence line becomes the container's
// (declassed) title text, and the body that follows is untouched. Nothing
// falls back to raw, unconsumed text any more — there is no path left that
// does that — but the author's words all still reach the output somewhere.
func TestContainerUnclosedLabelBracketBecomesUnknownBareKind(t *testing.T) {
	var warns []string
	got := convert(t, ":::aside[Why this matters\nbody\n:::\n",
		func(s string) { warns = append(warns, s) })
	if len(warns) != 1 || !strings.Contains(warns[0], `unknown container kind "aside[Why"`) {
		t.Errorf("warning changed: %v\ngot: %s", warns, got)
	}
	if !strings.Contains(got, "this matters") {
		t.Errorf("title text lost\ngot: %s", got)
	}
	if !strings.Contains(got, "body") {
		t.Errorf("body text lost\ngot: %s", got)
	}
	if strings.Contains(got, "aside[Why") {
		t.Errorf("kind word leaked into the output\ngot: %s", got)
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
// The case variants are not decoration. An HTML attribute name is
// case-insensitive and x/net/html lowercases it when the rendered document
// is re-parsed for the transform pass, so data-Fence-Title is the marker —
// and a case-sensitive prefix test let it through, adopting the author's
// paragraph and rebuilding it without its id.
func TestContainerForgedTitleMarkerDoesNotAdoptAParagraph(t *testing.T) {
	for _, src := range []string{
		":::card[]{data-fence-title=\"1\"}\n<p class=\"container-title\" id=\"keepme\">mine</p>\n\nbody\n:::\n",
		":::card[]{data-Fence-Title=\"1\"}\n<p class=\"container-title\" id=\"keepme\">mine</p>\n\nbody\n:::\n",
		":::card[]{DATA-FENCE-TITLE=\"1\"}\n<p class=\"container-title\" id=\"keepme\">mine</p>\n\nbody\n:::\n",
		"::: {.card data-Fence-Title=\"1\"}\n<p class=\"container-title\" id=\"keepme\">mine</p>\n\nbody\n:::\n",
	} {
		got := convert(t, src, nil)
		if !strings.Contains(got, `<p class="container-title" id="keepme">mine</p>`) {
			t.Errorf("%q: forged marker made the container adopt the paragraph\ngot: %s", src, got)
		}
		if strings.Contains(strings.ToLower(got), "data-fence") {
			t.Errorf("%q: forged data-fence attribute reached the output\ngot: %s", src, got)
		}
	}
}

// A forged data-fence-kind is the parser's "this line named a kind" signal.
// A fence line that named none must not acquire one, however the attribute
// is capitalized: with a case-sensitive reservation, "::: {.house
// data-Fence-Kind=\"nav\"}" rendered <nav class="house"> — an element the
// author never asked for, from a class the transform is supposed to leave
// inert.
func TestContainerForgedFenceKindCannotChooseTheElement(t *testing.T) {
	for _, src := range []string{
		"::: {.house data-fence-kind=\"nav\"}\nbody\n:::\n",
		"::: {.house data-Fence-Kind=\"nav\"}\nbody\n:::\n",
		"::: {.house DATA-FENCE-KIND=\"nav\"}\nbody\n:::\n",
	} {
		got := convert(t, src, nil)
		if strings.Contains(got, "<nav") {
			t.Errorf("%q: forged kind chose the element\ngot: %s", src, got)
		}
		if !strings.Contains(got, `<div class="house">`) {
			t.Errorf("%q: class not left inert on a div\ngot: %s", src, got)
		}
	}
}

// data-fence is a reserved namespace rather than three literal names, and
// docs/authoring.md and the skill both say so. That claim is only true if
// the prefix test ignores case: data-Fencepost used to survive while
// data-fencepost was dropped.
func TestContainerReservedNamespaceIgnoresCase(t *testing.T) {
	for _, src := range []string{
		"::: card {data-fencepost=\"b\"}\nbody\n:::\n",
		"::: card {data-Fencepost=\"b\"}\nbody\n:::\n",
		"::: card {DATA-FENCEPOST=\"b\"}\nbody\n:::\n",
	} {
		got := convert(t, src, nil)
		if strings.Contains(strings.ToLower(got), "fencepost") {
			t.Errorf("%q: reserved namespace escaped on a case variant\ngot: %s", src, got)
		}
		if !strings.Contains(got, `<div class="card">`) {
			t.Errorf("%q: container lost its kind\ngot: %s", src, got)
		}
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
		// The same three spelled in mixed case. The reservation has to
		// hold on a fold, because HTML attribute names do not distinguish
		// case and neither does Continue's lookup of the id.
		":::card[T]{data-Fence=\"x\"}\nbody\n:::\n",
		"::: {.card DATA-FENCE=\"x\"}\nbody\n:::\n",
		":::card[T]{data-Fence-Kind=\"aside\"}\nbody\n:::\n",
	} {
		got := convert(t, src, nil)
		if strings.Contains(strings.ToLower(got), "data-fence") {
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

// Two braced spellings that main's fence library rejected outright, so the
// fence text fell through to the old mining path: goldmark's own
// parser.ParseAttributes did not accept a bare key with no value or a
// single-quoted attribute value, and either would leave the whole line as
// unrecognized body text and warn "unknown container kind" on it.
//
// This package's shared attribute parser (parseAttrs, via fenceAttrs) is
// simply more permissive on both counts, and now handles every braced
// container, not just a titled one. No document that worked before this
// change is affected — these two spellings never rendered as containers
// on main — but the widening itself needs a test, or a later refactor
// could narrow the shared parser back to goldmark's rules without anyone
// noticing the regression.
func TestContainerBracedBareKeyAcceptedByPermissiveAttrParser(t *testing.T) {
	var warns []string
	got := convert(t, "::: {.callout data-flag}\nx\n:::\n", func(s string) { warns = append(warns, s) })
	if len(warns) != 0 {
		t.Errorf("warned on a now-accepted spelling: %v", warns)
	}
	if !strings.Contains(got, `<div class="callout" data-flag="">`) {
		t.Errorf("bare key did not become an empty-valued attribute\ngot: %s", got)
	}
}

func TestContainerBracedSingleQuotedValueAcceptedByPermissiveAttrParser(t *testing.T) {
	var warns []string
	got := convert(t, "::: {.callout data-x='single'}\nx\n:::\n", func(s string) { warns = append(warns, s) })
	if len(warns) != 0 {
		t.Errorf("warned on a now-accepted spelling: %v", warns)
	}
	if !strings.Contains(got, `<div class="callout" data-x="single">`) {
		t.Errorf("single-quoted value not carried through unquoted\ngot: %s", got)
	}
}

// Cases A, B and C from the spec: a definition list marker or a setext
// underline on the next line used to capture the kind word.
func TestContainerBareKindBeforeDefinitionList(t *testing.T) {
	var warns []string
	got := convert(t, "::: card\nterm\n: def\n:::\n",
		func(s string) { warns = append(warns, s) })
	if len(warns) != 0 {
		t.Errorf("warned: %v\ngot: %s", warns, got)
	}
	if !strings.Contains(got, `<div class="card">`) {
		t.Errorf("kind word was stolen\ngot: %s", got)
	}
	if strings.Contains(got, "<dt>card</dt>") {
		t.Errorf("kind word became a term\ngot: %s", got)
	}
	if !strings.Contains(got, "<dt>term</dt>") {
		t.Errorf("definition list lost\ngot: %s", got)
	}
}

func TestContainerBareKindBeforeSetextHeading(t *testing.T) {
	for _, underline := range []string{"===", "---"} {
		var warns []string
		got := convert(t, "::: card\nHeading text\n"+underline+"\n:::\n",
			func(s string) { warns = append(warns, s) })
		if len(warns) != 0 {
			t.Errorf("%s: warned: %v\ngot: %s", underline, warns, got)
		}
		if !strings.Contains(got, `<div class="card">`) {
			t.Errorf("%s: kind word was stolen\ngot: %s", underline, got)
		}
		if strings.Contains(got, "card\nHeading text") || strings.Contains(got, `id="card-heading-text"`) {
			t.Errorf("%s: kind word reached the heading text or its id\ngot: %s", underline, got)
		}
	}
}

// The undelimited title form keeps working, and a bare ":::" still closes.
func TestContainerBareKindWithTitleUnchanged(t *testing.T) {
	got := convert(t, "::: aside Why this matters\nBecause.\n:::\n", nil)
	if !strings.Contains(got, "<summary>Why this matters</summary>") {
		t.Errorf("title lost\ngot: %s", got)
	}
}

// The kind outside the braces with a real attribute block. Today the braces
// land in the summary as literal text.
func TestContainerKindThenAttrs(t *testing.T) {
	got := convert(t, "::: aside {#w .compact}\nbody\n:::\n", nil)
	if !strings.Contains(got, `id="w"`) {
		t.Errorf("id not applied\ngot: %s", got)
	}
	if !strings.Contains(got, "compact") {
		t.Errorf("class not applied\ngot: %s", got)
	}
	if strings.Contains(got, "{#w") {
		t.Errorf("attribute block left as text\ngot: %s", got)
	}
	if !strings.Contains(got, "<summary>Aside</summary>") {
		t.Errorf("titleless collapsible lost its fallback\ngot: %s", got)
	}
}

func TestContainerKindThenTitleThenAttrs(t *testing.T) {
	got := convert(t, "::: aside Why this matters {#w}\nbody\n:::\n", nil)
	if !strings.Contains(got, "<summary>Why this matters</summary>") {
		t.Errorf("title lost\ngot: %s", got)
	}
	if !strings.Contains(got, `id="w"`) {
		t.Errorf("id not applied\ngot: %s", got)
	}
}

// The same line with a trailing no-break space. goldmark's fence-line trim
// is ASCII-only, so U+00A0 reaches the grammar and every index derived from
// the string's length is two bytes out: the title came back as "Why {#"
// while the id was still applied, so nothing downstream could notice.
func TestContainerTitleSurvivesATrailingNoBreakSpace(t *testing.T) {
	got := convert(t, "::: card Why {#w} \nbody\n:::\n", nil)
	if !strings.Contains(got, `<p class="container-title">Why</p>`) {
		t.Errorf("title truncated by the attribute block\ngot: %s", got)
	}
	if !strings.Contains(got, `id="w"`) {
		t.Errorf("id not applied\ngot: %s", got)
	}
}

// A brace inside the attribute block's own content belongs to the block.
// Pairing the last "{" with the last "}" instead made the block "{b}}" and
// left "{a" in front of it to be lifted as the container's title.
func TestContainerNestedBraceDoesNotBecomeATitle(t *testing.T) {
	got := convert(t, "::: card {a{b}}\nbody\n:::\n", nil)
	if strings.Contains(got, "container-title") {
		t.Errorf("a fragment of the attribute block became a title\ngot: %s", got)
	}
	if !strings.Contains(got, `<div class="card">`) {
		t.Errorf("kind lost\ngot: %s", got)
	}
}

// The transform's last warning, which had no test anywhere even though this
// branch changed its entire trigger set: it now fires only for a container
// the parser owned that named no kind and reached the page with no attribute
// at all. An empty block is one way; a block whose only attribute the
// renderer refuses to write — an unknown bare key is neither global, data-
// nor aria- — is the other.
func TestContainerWithNoClassAndNoKindWarns(t *testing.T) {
	for _, src := range []string{
		"::: {}\nbody\n:::\n",
		"::: {x}\nbody\n:::\n",
	} {
		var msgs []string
		got := convert(t, src, func(m string) { msgs = append(msgs, m) })
		if len(msgs) != 1 || !strings.Contains(msgs[0], "no class and no recognizable kind name") {
			t.Errorf("%q: warnings = %v, want exactly the no-class warning", src, msgs)
		}
		if !strings.Contains(got, "<div>") {
			t.Errorf("%q: want a bare div\ngot: %s", src, got)
		}
	}
}

// A titled nav exercises applyKind's non-details tail on a kind that is
// neither a div nor a collapsible: the title paragraph has to be inserted
// before the element is retagged, or it lands in the div that retag throws
// away. Every other titled kind is a div or a <details>.
func TestContainerNavKindWithTitle(t *testing.T) {
	got := convert(t, "::: nav Section links\n- [One](./a.md)\n:::\n", nil)
	if !strings.Contains(got, `<nav><p class="container-title">Section links</p>`) {
		t.Errorf("titled nav malformed\ngot: %s", got)
	}
}

// A ::: line inside a code fence is content, not a container.
func TestContainerInsideCodeFenceStaysLiteral(t *testing.T) {
	got := convert(t, "```markdown\n::: card\nbody\n:::\n```\n", nil)
	if strings.Contains(got, `<div class="card">`) {
		t.Errorf("a fenced code example became a container\ngot: %s", got)
	}
}

// A landmark that cannot be named is noise in a screen reader's landmark
// list, and aria-label is not in goldmark's global attribute allowlist.
func TestContainerTakesAnAriaLabel(t *testing.T) {
	got := convert(t, "::: card {aria-label=\"Primary\"}\nbody\n:::\n", nil)
	if !strings.Contains(got, `aria-label="Primary"`) {
		t.Errorf("aria-label dropped\ngot: %s", got)
	}
}

// And an accessible name written in the case HTML itself does not
// distinguish. The renderer's prefix test used to be case-sensitive, so
// ARIA-label was dropped and the landmark went unnamed — the very failure
// the prefix widening exists to prevent.
func TestContainerAriaPrefixIgnoresCase(t *testing.T) {
	got := convert(t, "::: card {ARIA-label=\"Primary\"}\nbody\n:::\n", nil)
	if !strings.Contains(got, `aria-label="Primary"`) {
		t.Errorf("ARIA-label dropped\ngot: %s", got)
	}
}

// An event handler must not survive, whatever else is allowed through.
func TestContainerDropsEventHandlerAttribute(t *testing.T) {
	got := convert(t, "::: card {onmouseover=\"alert(1)\"}\nbody\n:::\n", nil)
	if strings.Contains(got, "onmouseover") {
		t.Errorf("event handler survived\ngot: %s", got)
	}
}

// nav becomes a shipped kind rather than a class the renderer sniffs for.
func TestContainerNavKind(t *testing.T) {
	got := convert(t, "::: nav {aria-label=\"Section\"}\n- [One](./a.md)\n:::\n", nil)
	if !strings.Contains(got, "<nav") {
		t.Errorf("no nav element\ngot: %s", got)
	}
	if !strings.Contains(got, `aria-label="Section"`) {
		t.Errorf("nav cannot be named\ngot: %s", got)
	}
	if strings.Contains(got, "data-fence") {
		t.Errorf("internal attribute leaked\ngot: %s", got)
	}
	// nav is the one kind with no class of its own (see containerKinds), so
	// applyKind must skip the class attribute entirely rather than writing
	// class="" — a stray empty attribute a template author could easily
	// mistake for meaning something.
	if strings.Contains(got, `class=""`) {
		t.Errorf("nav kind wrote an empty class attribute\ngot: %s", got)
	}
}

// The braced spelling of the same kind, which is the one that reproduces
// the empty-class bug: removeClassToken empties the class attribute of its
// only token, so applyKind meets a class attribute that already exists and
// has to remove it rather than merely decline to write one. "::: nav {…}"
// above never had the attribute at all, so it could not have caught this.
func TestContainerBracedNavKind(t *testing.T) {
	got := convert(t, "::: {.nav}\n- [One](./a.md)\n:::\n", nil)
	if !strings.Contains(got, "<nav>") {
		t.Errorf("no bare nav element\ngot: %s", got)
	}
	if strings.Contains(got, `class=""`) {
		t.Errorf("braced nav kind wrote an empty class attribute\ngot: %s", got)
	}
}

// A braced nav that also carries a class of its own keeps it: the attribute
// is removed only when nothing is left in it.
func TestContainerBracedNavKindKeepsExtraClass(t *testing.T) {
	got := convert(t, "::: {.nav .sidebar}\n- [One](./a.md)\n:::\n", nil)
	if !strings.Contains(got, `<nav class="sidebar">`) {
		t.Errorf("extra class lost\ngot: %s", got)
	}
}

// The old magic class is now an ordinary, inert class.
func TestContainerElemNavIsAnOrdinaryDiv(t *testing.T) {
	got := convert(t, "::: {.elem-nav}\nbody\n:::\n", nil)
	if strings.Contains(got, "<nav") {
		t.Errorf("still emitting a nav element\ngot: %s", got)
	}
	if strings.Contains(got, "data-fence") {
		t.Errorf("internal data-fence attribute leaked\ngot: %s", got)
	}
	if !strings.Contains(got, `<div class="elem-nav">`) {
		t.Errorf("class lost\ngot: %s", got)
	}
}

// The end-to-end consequence of braceBlock declining after an unclosed
// brace: the attribute block stays in the title as literal text rather than
// being applied, so the author sees the problem on the page. Balanced braces
// in a title are unaffected.
func TestContainerUnbalancedBraceInTitleKeepsTheBlockLiteral(t *testing.T) {
	got := convert(t, "::: card Opening brace { {#w}\nbody\n:::\n", nil)
	if strings.Contains(got, `id="w"`) {
		t.Errorf("applied attributes from a line with an unclosed brace\ngot: %s", got)
	}
	if !strings.Contains(got, "{#w}") {
		t.Errorf("the block vanished instead of staying visible in the title\ngot: %s", got)
	}
	balanced := convert(t, "::: card Use {{ var }} here {#t}\nbody\n:::\n", nil)
	if !strings.Contains(balanced, `id="t"`) {
		t.Errorf("balanced braces in a title blocked the attributes\ngot: %s", balanced)
	}
}
