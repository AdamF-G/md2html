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
