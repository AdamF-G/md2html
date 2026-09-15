package md2html

import (
	"os"
	"strings"
	"testing"
)

// figConvert renders src and collects every warning, so a test can assert
// on both the markup and the diagnostics in one call.
func figConvert(t *testing.T, src string) (out string, warnings []string) {
	t.Helper()
	got, err := Convert([]byte(src), Options{
		CSS:  "/**/",
		Warn: func(msg string) { warnings = append(warnings, msg) },
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return string(got), warnings
}

func TestFigMinimalFigure(t *testing.T) {
	out, warnings := figConvert(t, "```fig\ncaption: Request path\nitems:\n  - box: Client\n```\n")

	if len(warnings) != 0 {
		t.Errorf("want no warnings, got %v", warnings)
	}
	for _, want := range []string{
		`<figure class="fig">`,
		`<div class="fig-rows">`,
		`<div class="fig-box">Client</div>`,
		`<figcaption>Request path</figcaption>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, `language-fig`) {
		t.Errorf("figure fell back to a code block:\n%s", out)
	}
}

func TestFigMalformedBodyDegrades(t *testing.T) {
	out, warnings := figConvert(t, "```fig\nitems: [unclosed\n```\n")

	if !strings.Contains(out, `class="language-fig"`) {
		t.Errorf("want a code-block fallback, got:\n%s", out)
	}
	if len(warnings) != 1 {
		t.Fatalf("want exactly 1 warning, got %v", warnings)
	}
	if !strings.Contains(warnings[0], "fig") {
		t.Errorf("warning should name the fence: %q", warnings[0])
	}
}

func TestFigValidationRejects(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantMsg string
	}{
		{
			name:    "two kinds in one item",
			src:     "```fig\nitems:\n  - box: A\n    rail: B\n```\n",
			wantMsg: "names 2 kinds",
		},
		{
			name:    "no kind at all",
			src:     "```fig\nitems:\n  - weight: 2\n```\n",
			wantMsg: "names no kind",
		},
		{
			name:    "null kind value suggests the fix",
			src:     "```fig\nitems:\n  - box:\n```\n",
			wantMsg: `box: ""`,
		},
		{
			name: "unknown key",
			src:  "```fig\nitems:\n  - boxes: A\n```\n",
			// yaml.v3 phrases this as "field boxes not found in type
			// md2html.figItem"; figFault rewrites it into the fence
			// language's own vocabulary.
			wantMsg: `unknown key "boxes"`,
		},
		{
			name:    "weight outside cols",
			src:     "```fig\nitems:\n  - box: A\n    weight: 2\n```\n",
			wantMsg: "weight",
		},
		{
			name:    "unrecognized layout",
			src:     "```fig\nlayout: bogus\nitems:\n  - box: A\n```\n",
			wantMsg: "not rows, cols or split",
		},
		{
			name:    "split layout with wrong item count",
			src:     "```fig\nlayout: split\nitems:\n  - box: A\n```\n",
			wantMsg: "exactly 2 items",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, warnings := figConvert(t, c.src)
			if !strings.Contains(out, `class="language-fig"`) {
				t.Errorf("want a code-block fallback, got:\n%s", out)
			}
			if len(warnings) != 1 {
				t.Fatalf("want exactly 1 warning, got %v", warnings)
			}
			if !strings.Contains(warnings[0], c.wantMsg) {
				t.Errorf("warning %q should contain %q", warnings[0], c.wantMsg)
			}
		})
	}
}

func TestFigValidationNamesItemPath(t *testing.T) {
	_, warnings := figConvert(t,
		"```fig\nitems:\n  - box: A\n  - box: B\n    rail: C\n```\n")

	if len(warnings) != 1 {
		t.Fatalf("want exactly 1 warning, got %v", warnings)
	}
	if !strings.Contains(warnings[0], "items[1]") {
		t.Errorf("warning should locate the second item: %q", warnings[0])
	}
}

func TestFigBlankBoxIsNotAWarning(t *testing.T) {
	out, warnings := figConvert(t, "```fig\nitems:\n  - box: \"\"\n```\n")

	if len(warnings) != 0 {
		t.Errorf("a deliberately blank box must not warn, got %v", warnings)
	}
	if !strings.Contains(out, `<div class="fig-box"></div>`) {
		t.Errorf("want an empty box, got:\n%s", out)
	}
}

// The gallery is a review artifact first, but it is also the broadest
// end-to-end case there is: every kind and layout in one document.
func TestFigGalleryRenders(t *testing.T) {
	src, err := os.ReadFile("testdata/figures.md")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	out, warnings := figConvert(t, string(src))

	for _, want := range []string{
		`<div class="fig-rows">`, `<div class="fig-cols">`,
		`<div class="fig-split">`, `<div class="fig-boundary">`,
		`<div class="fig-box`, `<div class="fig-arrow`,
		`<div class="fig-result`, `<div class="fig-rail`,
		`<div class="fig-group`, `<div class="fig-chain">`,
		`<div class="fig-lane">`,
		`<span class="fig-stat-value">`, `<dl class="fig-defs">`,
		`style="--fig-weight:3"`,
		// added by this plan
		`<span class="fig-note">`, `<li class="fig-accent">`,
		`<div class="fig-group-foot">`, `<span class="fig-stat-detail">`,
		`<ul class="fig-tree">`, `<span class="fig-tree-label">`,
		`<li class="fig-muted">`, `<figure class="fig fig-wide">`,
		// nested layouts
		`A request crosses one trust boundary</div><div class="fig-split">`,
		`<div class="fig-panel" style="--fig-weight:1"><div class="fig-cols">`,
		`<div class="fig-panel fig-panel-accent" style="--fig-weight:1"><div class="fig-group fig-accent"><div class="fig-group-title">New layer</div><div class="fig-box fig-accent">Cache</div>`,
		`<div class="fig-panel" style="--fig-weight:1"><div class="fig-split"><div class="fig-panel" style="--fig-weight:1"><div class="fig-box">Old store</div>`,
		`<div class="fig-arrow" aria-hidden="true"></div><div class="fig-cols"><div class="fig-panel" style="--fig-weight:3"><div class="fig-box">Primary</div>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("gallery is missing %q", want)
		}
	}
	// The gallery has to exercise an arrow at panel level inside a cols
	// layout, because that is the nesting the stylesheet's arrow rules get
	// wrong if written as `.fig-cols > .fig-arrow`. Without this shape in
	// the corpus, a dead selector renders a glyphless box and no test
	// notices.
	if !strings.Contains(out, `<div class="fig-panel fig-panel-arrow" style="--fig-weight:1"><div class="fig-arrow" aria-hidden="true">`) {
		t.Errorf("gallery must contain a panel-level arrow in a cols layout:\n%s", out)
	}
	// The gallery ends with one deliberate failure case.
	if len(warnings) != 1 {
		t.Fatalf("want exactly the one intended warning, got %v", warnings)
	}
	if !strings.Contains(warnings[0], "boxes") {
		t.Errorf("warning should name the unknown key: %q", warnings[0])
	}
	if !strings.Contains(out, `class="language-fig"`) {
		t.Error("the degradation case should leave a visible code block")
	}
}

// A `---` separator starts a second YAML document. yaml.Decoder hands back
// one document per Decode call, so a single call renders the first and
// discards everything after it — the silent-vanishing failure KnownFields
// exists to prevent, arriving through another door. A body that says more
// than one thing is a fault like any other: warn once, show the source.
func TestFigMultiDocumentBodyDegrades(t *testing.T) {
	src := "```fig\nitems:\n  - box: A\n---\nitems:\n  - box: B\n```\n"
	out, warnings := figConvert(t, src)

	if !strings.Contains(out, `class="language-fig"`) {
		t.Errorf("want a code-block fallback, got:\n%s", out)
	}
	if strings.Contains(out, `class="fig-box"`) {
		t.Errorf("no part of a truncated body may render as a figure:\n%s", out)
	}
	if len(warnings) != 1 {
		t.Fatalf("want exactly 1 warning, got %v", warnings)
	}
	if !strings.Contains(warnings[0], "---") {
		t.Errorf("warning should name the separator: %q", warnings[0])
	}
	// Both documents' source stays visible to the author.
	if !strings.Contains(out, "box: A") || !strings.Contains(out, "box: B") {
		t.Errorf("the whole body should survive into the code block:\n%s", out)
	}
}

// An empty fence degrades correctly already; the point here is that the
// message says what is wrong in the author's terms rather than reporting
// the decoder's io.EOF.
func TestFigEmptyFenceSaysSo(t *testing.T) {
	for _, src := range []string{"```fig\n```\n", "```fig\n\n   \n```\n"} {
		out, warnings := figConvert(t, src)
		if !strings.Contains(out, `class="language-fig"`) {
			t.Errorf("want a code-block fallback, got:\n%s", out)
		}
		if len(warnings) != 1 {
			t.Fatalf("want exactly 1 warning, got %v", warnings)
		}
		if !strings.Contains(warnings[0], "empty") {
			t.Errorf("warning should say the fence is empty: %q", warnings[0])
		}
		if strings.Contains(warnings[0], "EOF") {
			t.Errorf("warning should not leak the decoder's EOF: %q", warnings[0])
		}
	}
}

// A caption in the info string is code-fence vocabulary, not figure
// vocabulary: a figure's caption is a `caption:` key in its body. The
// fence used to compute the info-string caption and then return before the
// code that renders it, so the author's text vanished without a word. It is
// not adopted as a fallback — one thing gets one spelling — but it must not
// disappear in silence either.
func TestFigInfoStringCaptionWarns(t *testing.T) {
	out, warnings := figConvert(t,
		"```fig caption=\"Request path\"\nitems:\n  - box: Client\n```\n")

	if len(warnings) != 1 {
		t.Fatalf("want exactly 1 warning, got %v", warnings)
	}
	if !strings.Contains(warnings[0], "caption:") {
		t.Errorf("warning should name the caption: key as the alternative: %q", warnings[0])
	}
	// Not adopted: the info-string text must not become the figure caption.
	if strings.Contains(out, "<figcaption>Request path</figcaption>") {
		t.Errorf("the info-string caption must not be adopted:\n%s", out)
	}
	if !strings.Contains(out, `<div class="fig-box">Client</div>`) {
		t.Errorf("the figure itself should still render:\n%s", out)
	}
}

// A body caption still works, and on its own it warns about nothing.
func TestFigBodyCaptionDoesNotWarn(t *testing.T) {
	out, warnings := figConvert(t,
		"```fig\ncaption: Request path\nitems:\n  - box: Client\n```\n")
	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	if !strings.Contains(out, "<figcaption>Request path</figcaption>") {
		t.Errorf("want the body caption rendered:\n%s", out)
	}
}

// Two faults, two warnings. The one-warning-per-fault invariant is per
// fault, not per fence: a misplaced caption and a body that will not decode
// are separate mistakes with separate fixes, and collapsing them would hide
// one of the two.
func TestFigInfoStringCaptionAndBrokenBodyAreTwoWarnings(t *testing.T) {
	out, warnings := figConvert(t,
		"```fig caption=\"Request path\"\nitems: [unclosed\n```\n")

	if len(warnings) != 2 {
		t.Fatalf("want 2 warnings, got %v", warnings)
	}
	if !strings.Contains(warnings[0], "caption:") {
		t.Errorf("first warning should be about the caption: %q", warnings[0])
	}
	if !strings.Contains(warnings[1], "code block") {
		t.Errorf("second warning should be the degradation: %q", warnings[1])
	}
	if !strings.Contains(out, `class="language-fig"`) {
		t.Errorf("want a code-block fallback, got:\n%s", out)
	}
}

// Every warning this tool emits is one line of author-facing prose: the CLI
// prefixes each with "md2html: <file>: " and a second line would escape the
// prefix entirely. yaml.v3's own error is two lines and names a Go type the
// author has never heard of.
func TestFigWarningsAreOneLineOfProse(t *testing.T) {
	for _, src := range []string{
		"```fig\nitems:\n  - boxes: A\n```\n",
		"```fig\nitems:\n  - boxes: A\n    rails: B\n```\n",
		"```fig\nitems: 3\n```\n",
		"```fig\nitems:\n  - box: A\n    weight: xyz\n```\n",
		"```fig\nitems: [unclosed\n```\n",
	} {
		_, warnings := figConvert(t, src)
		if len(warnings) != 1 {
			t.Fatalf("%q: want exactly 1 warning, got %v", src, warnings)
		}
		w := warnings[0]
		if strings.ContainsAny(w, "\n\r") {
			t.Errorf("%q: warning spans lines: %q", src, w)
		}
		if strings.Contains(w, "md2html.") {
			t.Errorf("%q: warning leaks a Go type: %q", src, w)
		}
		if strings.Contains(w, "yaml:") {
			t.Errorf("%q: warning leaks the decoder's prefix: %q", src, w)
		}
	}
}

// The unknown-key case is the common one, so it gets the phrasing an author
// can act on — and says which line numbering it is quoting, since a fence
// body's line 2 is rarely the document's line 2.
func TestFigUnknownKeyWarningIsAuthorFacing(t *testing.T) {
	_, warnings := figConvert(t, "```fig\nitems:\n  - boxes: A\n```\n")
	if len(warnings) != 1 {
		t.Fatalf("want exactly 1 warning, got %v", warnings)
	}
	for _, want := range []string{`unknown key "boxes"`, "line 2", "fence body"} {
		if !strings.Contains(warnings[0], want) {
			t.Errorf("warning %q should contain %q", warnings[0], want)
		}
	}
}

func TestFigModifierPlacementRejected(t *testing.T) {
	cases := []struct{ name, src, wantMsg string }{
		{
			name:    "note on an arrow",
			src:     "```fig\nitems:\n  - arrow: next\n    note: why\n```\n",
			wantMsg: "carries note, which only a box, result, rail or group takes",
		},
		{
			name:    "accent on an arrow",
			src:     "```fig\nitems:\n  - arrow: next\n    accent: true\n```\n",
			wantMsg: "carries accent, which only a box, result, rail or group takes",
		},
		{
			name:    "note nested inside a chain",
			src:     "```fig\nitems:\n  - chain:\n      - arrow: \"\"\n        note: why\n```\n",
			wantMsg: "items[0].chain[0] carries note",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, warnings := figConvert(t, c.src)
			if !strings.Contains(out, `class="language-fig"`) {
				t.Errorf("want a code-block fallback, got:\n%s", out)
			}
			if len(warnings) != 1 {
				t.Fatalf("want exactly 1 warning, got %v", warnings)
			}
			if !strings.Contains(warnings[0], c.wantMsg) {
				t.Errorf("warning %q should contain %q", warnings[0], c.wantMsg)
			}
		})
	}
}

// note and foot are different positions on a group, not two spellings of
// one: the note glosses the title and precedes the items, the foot follows
// them. A test that only checked both were present would pass with them
// swapped.
func TestFigGroupNoteGlossesTheTitle(t *testing.T) {
	src := "```fig\nitems:\n  - group: retry loop\n    note: N attempts, exponential backoff\n" +
		"    foot: the only caller\n    items:\n      - box: request\n```\n"
	out, warnings := figConvert(t, src)

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	want := `<div class="fig-group-title">retry loop<span class="fig-note">N attempts, exponential backoff</span></div>`
	if !strings.Contains(out, want) {
		t.Errorf("missing %q in:\n%s", want, out)
	}
	noteIdx, requestIdx := strings.Index(out, `class="fig-note"`), strings.Index(out, ">request<")
	if noteIdx < 0 || requestIdx < 0 {
		t.Fatalf("missing an anchor for the ordering check: note=%d request=%d in:\n%s", noteIdx, requestIdx, out)
	}
	if noteIdx > requestIdx {
		t.Errorf("a group's note must precede its items:\n%s", out)
	}
	footIdx := strings.Index(out, `<div class="fig-group-foot">`)
	if footIdx < 0 {
		t.Fatalf("missing an anchor for the ordering check: foot=%d in:\n%s", footIdx, out)
	}
	if footIdx < requestIdx {
		t.Errorf("a group's foot must follow its items:\n%s", out)
	}
}

func TestFigFootRejectedOffAGroup(t *testing.T) {
	out, warnings := figConvert(t,
		"```fig\nitems:\n  - box: A\n    foot: trailing\n```\n")

	if !strings.Contains(out, `class="language-fig"`) {
		t.Errorf("want a code-block fallback, got:\n%s", out)
	}
	if len(warnings) != 1 {
		t.Fatalf("want exactly 1 warning, got %v", warnings)
	}
	if !strings.Contains(warnings[0], "carries foot, which only a group takes") {
		t.Errorf("warning %q should name the rule", warnings[0])
	}
}

func TestFigWideFlag(t *testing.T) {
	out, warnings := figConvert(t,
		"```fig\nwide: true\nitems:\n  - box: A\n```\n")
	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	if !strings.Contains(out, `<figure class="fig fig-wide">`) {
		t.Errorf("want a wide figure, got:\n%s", out)
	}
}

func TestFigWithoutWideFlagIsUnchanged(t *testing.T) {
	out, _ := figConvert(t, "```fig\nitems:\n  - box: A\n```\n")
	if !strings.Contains(out, `<figure class="fig">`) {
		t.Errorf("a figure without wide should be unchanged, got:\n%s", out)
	}
}

// wide is a property of the figure, not of an item.
func TestFigWideRejectedOnAnItem(t *testing.T) {
	out, warnings := figConvert(t,
		"```fig\nitems:\n  - box: A\n    wide: true\n```\n")
	if !strings.Contains(out, `class="language-fig"`) {
		t.Errorf("want a code-block fallback, got:\n%s", out)
	}
	if len(warnings) != 1 {
		t.Fatalf("want exactly 1 warning, got %v", warnings)
	}
	if !strings.Contains(warnings[0], `unknown key "wide"`) {
		t.Errorf("warning %q should name the unknown key", warnings[0])
	}
}

// A malformed tree degrades the whole fence, like every other fig fault,
// and the message carries both the item path and the line inside the tree.
func TestFigTreeFaultDegradesWithAPath(t *testing.T) {
	src := "```fig\nitems:\n  - box: A\n  - tree: |\n      a\n          b\n        c\n```\n"
	out, warnings := figConvert(t, src)

	if !strings.Contains(out, `class="language-fig"`) {
		t.Errorf("want a code-block fallback, got:\n%s", out)
	}
	if len(warnings) != 1 {
		t.Fatalf("want exactly 1 warning, got %v", warnings)
	}
	if !strings.Contains(warnings[0], "items[1].tree line 3: indented to no enclosing level") {
		t.Errorf("warning %q should name the item path and the tree's own line", warnings[0])
	}
}

// tree is a kind, so it collides with another kind on the same item and
// takes none of the leaf modifiers.
func TestFigTreeIsAKind(t *testing.T) {
	cases := []struct{ name, src, wantMsg string }{
		{
			name:    "two kinds",
			src:     "```fig\nitems:\n  - tree: \"a\"\n    box: B\n```\n",
			wantMsg: "names 2 kinds (box, tree)",
		},
		{
			name:    "note on a tree",
			src:     "```fig\nitems:\n  - tree: \"a\"\n    note: why\n```\n",
			wantMsg: "carries note, which only a box, result, rail or group takes",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, warnings := figConvert(t, c.src)
			if len(warnings) != 1 {
				t.Fatalf("want exactly 1 warning, got %v", warnings)
			}
			if !strings.Contains(warnings[0], c.wantMsg) {
				t.Errorf("warning %q should contain %q", warnings[0], c.wantMsg)
			}
		})
	}
}

// A rows figure whose only item is a layout item means exactly what the
// figure-level layout means. One spelling per figure: the item form there
// is a fault that names the other.
func TestFigSoleLayoutItemRejected(t *testing.T) {
	cases := []struct{ name, src, wantMsg string }{
		{
			name:    "split",
			src:     "```fig\nitems:\n  - split:\n      - box: A\n      - box: B\n```\n",
			wantMsg: "items[0] is the figure's only item; write layout: split instead",
		},
		{
			name:    "cols",
			src:     "```fig\nitems:\n  - cols:\n      - box: A\n      - box: B\n```\n",
			wantMsg: "items[0] is the figure's only item; write layout: cols instead",
		},
		{
			name:    "explicit rows layout",
			src:     "```fig\nlayout: rows\nitems:\n  - split:\n      - box: A\n      - box: B\n```\n",
			wantMsg: "write layout: split instead",
		},
		{
			// An item naming two kinds gets the more precise fault.
			name:    "two kinds still reported as two kinds",
			src:     "```fig\nitems:\n  - box: A\n    split:\n      - box: B\n      - box: C\n```\n",
			wantMsg: "names 2 kinds",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, warnings := figConvert(t, c.src)
			if !strings.Contains(out, `class="language-fig"`) {
				t.Errorf("want a code-block fallback, got:\n%s", out)
			}
			if len(warnings) != 1 {
				t.Fatalf("want exactly 1 warning, got %v", warnings)
			}
			if !strings.Contains(warnings[0], c.wantMsg) {
				t.Errorf("warning %q should contain %q", warnings[0], c.wantMsg)
			}
		})
	}
}

// The rule is about the figure's own item list. Anywhere else a lone layout
// item is not a second spelling of anything.
func TestFigLoneLayoutItemElsewhereIsLegal(t *testing.T) {
	for _, src := range []string{
		"```fig\nitems:\n  - group: G\n    items:\n      - split:\n          - box: A\n          - box: B\n```\n",
		"```fig\nlayout: cols\nitems:\n  - split:\n      - box: A\n      - box: B\n```\n",
	} {
		out, warnings := figConvert(t, src)
		if len(warnings) != 0 {
			t.Errorf("%q: want no warnings, got %v", src, warnings)
		}
		if !strings.Contains(out, `<div class="fig-split">`) {
			t.Errorf("%q: want a rendered split, got:\n%s", src, out)
		}
	}
}

// A layout item is validated where it sits. Every case leads with another
// item so the figure never has a layout item as its only item, which is a
// separate fault with its own test.
func TestFigLayoutItemRejects(t *testing.T) {
	cases := []struct{ name, src, wantMsg string }{
		{
			name:    "split with one item",
			src:     "```fig\nitems:\n  - box: Lead\n  - split:\n      - box: A\n```\n",
			wantMsg: "items[1]: a split needs exactly 2 items, got 1",
		},
		{
			name:    "split with three items",
			src:     "```fig\nitems:\n  - box: Lead\n  - split:\n      - box: A\n      - box: B\n      - box: C\n```\n",
			wantMsg: "items[1]: a split needs exactly 2 items, got 3",
		},
		{
			name:    "boundary on a box",
			src:     "```fig\nitems:\n  - box: Lead\n    boundary: to\n```\n",
			wantMsg: "items[0] carries boundary, which only a split takes",
		},
		{
			name:    "boundary on a group",
			src:     "```fig\nitems:\n  - group: G\n    boundary: to\n    items:\n      - box: A\n```\n",
			wantMsg: "items[0] carries boundary, which only a split takes",
		},
		{
			name:    "boundary on a cols item",
			src:     "```fig\nitems:\n  - box: Lead\n  - cols:\n      - box: A\n    boundary: to\n```\n",
			wantMsg: "items[1] carries boundary, which only a split takes",
		},
		{
			name:    "weight on a split child",
			src:     "```fig\nitems:\n  - box: Lead\n  - split:\n      - box: A\n        weight: 2\n      - box: B\n```\n",
			wantMsg: "items[1].split[0] carries weight, which only a child of a cols layout or cols item takes",
		},
		{
			name:    "weight on a group child",
			src:     "```fig\nitems:\n  - group: G\n    items:\n      - box: A\n        weight: 2\n```\n",
			wantMsg: "items[0].items[0] carries weight",
		},
		{
			// inCols must not leak past the cols item's own children.
			name:    "weight on a grandchild of a cols item",
			src:     "```fig\nitems:\n  - box: Lead\n  - cols:\n      - group: G\n        items:\n          - box: A\n            weight: 2\n```\n",
			wantMsg: "items[1].cols[0].items[0] carries weight",
		},
		{
			name:    "split and box on one item",
			src:     "```fig\nitems:\n  - box: Lead\n  - box: A\n    split:\n      - box: B\n      - box: C\n```\n",
			wantMsg: "names 2 kinds (box, split)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, warnings := figConvert(t, c.src)
			if !strings.Contains(out, `class="language-fig"`) {
				t.Errorf("want a code-block fallback, got:\n%s", out)
			}
			if len(warnings) != 1 {
				t.Fatalf("want exactly 1 warning, got %v", warnings)
			}
			if !strings.Contains(warnings[0], c.wantMsg) {
				t.Errorf("warning %q should contain %q", warnings[0], c.wantMsg)
			}
		})
	}
}
