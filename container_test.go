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
