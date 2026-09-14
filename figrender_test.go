package md2html

import (
	"strings"
	"testing"
)

func TestFigInlineMarkdownInLabels(t *testing.T) {
	r := newFigRenderer()
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"`auth` middleware", "<code>auth</code> middleware"},
		{"*emphasis*", "<em>emphasis</em>"},
		{"[docs](./other.md)", `<a href="./other.md">docs</a>`},
		{"5 < 7 & rising", "5 &lt; 7 &amp; rising"},
	}
	for _, c := range cases {
		if got := r.inline(c.in); got != c.want {
			t.Errorf("inline(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A figure's text is ordinary tree nodes by the time transforms run, so
// chips and section links reach inside one with no figure-specific code.
func TestFigTransformsReachInsideFigures(t *testing.T) {
	src := "## 4.2 Rollback\n\n```fig\nitems:\n  - box: status [proven]\n  - box: see §4.2\n```\n"
	out, warnings := figConvert(t, src)

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	if !strings.Contains(out, `class="chip`) {
		t.Errorf("a chip in a box label should render as a chip:\n%s", out)
	}
	if !strings.Contains(out, `href="#42-rollback"`) {
		t.Errorf("a section reference in a box label should autolink:\n%s", out)
	}
}

// A link that appears only inside a figure must still be crawlable, so the
// claim is pinned against ExtractLinks itself rather than against the
// presence of an <a href> in the HTML: the crawler is what decides whether
// a document linked only from a figure label gets visited, and an anchor
// the extractor happened not to classify would pass a markup assertion.
func TestFigLinksAreExtracted(t *testing.T) {
	out, _ := figConvert(t, "```fig\nitems:\n  - box: \"[api](./api.md)\"\n```\n")
	if !strings.Contains(out, `href="./api.md"`) {
		t.Errorf("want an anchor the crawler can see, got:\n%s", out)
	}
	root, err := parseFragment([]byte(out))
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	got := kindsOf(ExtractLinks(root, "/docs"))
	if k, ok := got["./api.md"]; !ok || k != LinkDoc {
		t.Errorf("ExtractLinks should return ./api.md as a document link, got %v", got)
	}
}

// Unlike mermaid, which depends on the host rendering it natively, a figure
// is self-contained markup travelling with the stylesheet — so it looks the
// same in an Artifact fragment as on a standalone page.
func TestFigSurvivesFragmentMode(t *testing.T) {
	got, err := Convert([]byte("```fig\nitems:\n  - box: Client\n```\n"),
		Options{Fragment: true})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out := string(got)
	if !strings.Contains(out, `<div class="fig-box">Client</div>`) {
		t.Errorf("figure markup missing from a fragment:\n%s", out)
	}
}

// The other half of that claim — that a fragment carries the figure styles
// with it — is asserted in Task 8, which is where the stylesheet lands. It
// cannot be checked before then without failing for the wrong reason.

func TestFigSimpleLeaves(t *testing.T) {
	out, warnings := figConvert(t,
		"```fig\nitems:\n  - arrow: HTTP POST\n  - result: 200 OK\n  - rail: Release\n```\n")

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	for _, want := range []string{
		`<div class="fig-arrow">HTTP POST</div>`,
		`<div class="fig-result">200 OK</div>`,
		`<div class="fig-rail">Release</div>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// An unlabeled arrow carries no information a screen reader needs: the
// connection is conveyed by the surrounding boxes.
func TestFigUnlabeledArrowIsHidden(t *testing.T) {
	out, _ := figConvert(t, "```fig\nitems:\n  - box: A\n  - arrow: \"\"\n  - box: B\n```\n")

	if !strings.Contains(out, `<div class="fig-arrow" aria-hidden="true"></div>`) {
		t.Errorf("want a hidden arrow, got:\n%s", out)
	}
}

func TestFigStatsRow(t *testing.T) {
	src := "```fig\nitems:\n  - stats:\n      - value: 78ms\n        label: cold\n      - value: 31ms\n        label: warm\n```\n"
	out, warnings := figConvert(t, src)

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	for _, want := range []string{
		`<div class="fig-stats">`,
		`<div class="fig-stat"><span class="fig-stat-value">78ms</span><span class="fig-stat-label">cold</span></div>`,
		`<span class="fig-stat-value">31ms</span>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestFigGroupNests(t *testing.T) {
	src := "```fig\nitems:\n  - group: Server\n    items:\n      - box: Handler\n      - result: 200\n```\n"
	out, warnings := figConvert(t, src)

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	for _, want := range []string{
		`<div class="fig-group">`,
		`<div class="fig-group-title">Server</div>`,
		`<div class="fig-box">Handler</div>`,
		`<div class="fig-result">200</div>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestFigGroupNestsToDepth(t *testing.T) {
	src := "```fig\nitems:\n  - group: Outer\n    items:\n      - group: Inner\n        items:\n          - box: Leaf\n```\n"
	out, _ := figConvert(t, src)

	if strings.Count(out, `class="fig-group"`) != 2 {
		t.Errorf("want two nested groups, got:\n%s", out)
	}
	if !strings.Contains(out, `<div class="fig-box">Leaf</div>`) {
		t.Errorf("the innermost leaf should render:\n%s", out)
	}
}

func TestFigChainAndLanes(t *testing.T) {
	src := "```fig\nitems:\n  - chain:\n      - box: Parse\n      - box: Render\n  - lanes:\n      - - box: L1\n      - - box: L2\n```\n"
	out, warnings := figConvert(t, src)

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	for _, want := range []string{
		`<div class="fig-chain">`,
		`<div class="fig-box">Parse</div>`,
		`<div class="fig-lanes">`,
		`<div class="fig-lane">`,
		`<div class="fig-box">L2</div>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Count(out, `class="fig-lane"`) != 2 {
		t.Errorf("want two lanes, got:\n%s", out)
	}
}

func TestFigColsWrapsPanelsWithWeights(t *testing.T) {
	src := "```fig\nlayout: cols\nitems:\n  - box: Wide\n    weight: 3\n  - box: Narrow\n```\n"
	out, warnings := figConvert(t, src)

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	for _, want := range []string{
		`<div class="fig-cols">`,
		`<div class="fig-panel" style="--fig-weight:3">`,
		`<div class="fig-panel" style="--fig-weight:1">`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, `class="fig-rows"`) {
		t.Errorf("a cols layout must not emit a rows wrapper:\n%s", out)
	}
}

func TestFigWeightIsClamped(t *testing.T) {
	for _, c := range []struct {
		in   int
		want int
	}{{0, 1}, {1, 1}, {7, 7}, {12, 12}, {99, 12}, {-4, 1}} {
		if got := figWeight(c.in); got != c.want {
			t.Errorf("figWeight(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestFigSplitPutsBoundaryBetweenPanels(t *testing.T) {
	src := "```fig\nlayout: split\nboundary: before / after\nitems:\n  - box: Old\n  - box: New\n```\n"
	out, warnings := figConvert(t, src)

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	if !strings.Contains(out, `<div class="fig-split">`) {
		t.Errorf("want a split wrapper:\n%s", out)
	}
	oldAt := strings.Index(out, "Old")
	bound := strings.Index(out, `class="fig-boundary"`)
	newAt := strings.Index(out, "New")
	if !(oldAt < bound && bound < newAt) {
		t.Errorf("boundary must sit between the panels:\n%s", out)
	}
}

// defs is a real <dl>, matching what the DefinitionList extension emits for
// prose, so the same stylesheet rules and the same semantics apply.
func TestFigDefsGrid(t *testing.T) {
	src := "```fig\nitems:\n  - defs:\n      - term: seed\n        def: an entry point\n```\n"
	out, warnings := figConvert(t, src)

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	for _, want := range []string{
		`<dl class="fig-defs">`,
		`<dt>seed</dt>`,
		`<dd>an entry point</dd>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// A figure label promises *inline* Markdown, and the promise is load-bearing
// rather than cosmetic: a block-level pass lets a label mint headings that
// compete for anchor ids, join [[toc]], and enter the namespace § references
// resolve against — all silently, from text an author wrote as a label.
//
// Every case here is a block construct that must survive as literal text.
func TestFigInlineRejectsBlockConstructs(t *testing.T) {
	r := newFigRenderer()
	cases := []struct{ in, want string }{
		{"1. Validate", "1. Validate"},
		{"- parse", "- parse"},
		{"> note", "&gt; note"},
		{"# Step", "# Step"},
		{"    indented code", "indented code"},
		{"a\n\nb", "a b"},
		{"one\ntwo", "one two"},
		{"***", "***"},
		{"| a | b |", "| a | b |"},
	}
	for _, c := range cases {
		got := r.inline(c.in)
		if got != c.want {
			t.Errorf("inline(%q) = %q, want %q", c.in, got, c.want)
		}
		for _, banned := range []string{"<p>", "</p>", "<ol", "<ul", "<li", "<h1", "<blockquote", "<pre", "<hr"} {
			if strings.Contains(got, banned) {
				t.Errorf("inline(%q) = %q: contains block markup %q", c.in, got, banned)
			}
		}
	}
}

// The end-to-end half of the same claim: a heading-shaped label must not
// reach the document's heading namespace. Before the inline pass was really
// inline, `box: "## Real"` emitted an <h2> that the TOC listed and that
// anchor generation gave an id to.
func TestFigLabelCannotMintAHeading(t *testing.T) {
	src := "# Doc\n\n[[toc]]\n\n## Real\n\n```fig\nitems:\n  - box: \"## Fake\"\n```\n"
	out, warnings := figConvert(t, src)

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	if strings.Contains(out, `id="fake"`) {
		t.Errorf("a box label must not create a heading anchor:\n%s", out)
	}
	if strings.Contains(out, `href="#fake"`) {
		t.Errorf("a box label must not appear in the toc:\n%s", out)
	}
	if !strings.Contains(out, `<div class="fig-box">## Fake</div>`) {
		t.Errorf("the label should survive as literal text:\n%s", out)
	}
	// The real heading still works, so the test is not passing vacuously.
	if !strings.Contains(out, `href="#real"`) {
		t.Errorf("the real heading should still be in the toc:\n%s", out)
	}
}

func TestFigLeafNoteAndAccent(t *testing.T) {
	out, warnings := figConvert(t,
		"```fig\nitems:\n  - box: Client\n    note: retries twice\n"+
			"  - result: 200 OK\n    accent: true\n```\n")

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	for _, want := range []string{
		`<div class="fig-box">Client<span class="fig-note">retries twice</span></div>`,
		`<div class="fig-result fig-accent">200 OK</div>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// A note is a text field like any other, so it renders through the inline
// pass rather than being escaped.
func TestFigNoteTakesInlineMarkdown(t *testing.T) {
	out, _ := figConvert(t,
		"```fig\nitems:\n  - box: Client\n    note: \"see `auth`\"\n```\n")
	if !strings.Contains(out, `<span class="fig-note">see <code>auth</code></span>`) {
		t.Errorf("a note should render inline Markdown, got:\n%s", out)
	}
}

// The modifiers are purely additive. A figure that uses neither must emit
// exactly what it emitted before they existed. This test is the guard on
// that promise; do not relax it.
func TestFigLeafMarkupUnchangedWithoutModifiers(t *testing.T) {
	out, _ := figConvert(t,
		"```fig\nitems:\n  - box: Client\n  - result: 200 OK\n  - rail: Phase one\n```\n")
	for _, want := range []string{
		`<div class="fig-box">Client</div>`,
		`<div class="fig-result">200 OK</div>`,
		`<div class="fig-rail">Phase one</div>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing unchanged markup %q in:\n%s", want, out)
		}
	}
}

// The panel-metadata rule: a panel's title, accent and footnote all live on
// the group inside it, so .fig-panel stays a bare weight carrier.
func TestFigGroupCarriesPanelMetadata(t *testing.T) {
	src := "```fig\nlayout: cols\nitems:\n" +
		"  - group: Before\n    accent: true\n    foot: the usual pattern\n" +
		"    items:\n      - box: Handler\n" +
		"  - group: After\n    items:\n      - box: Handler\n```\n"
	out, warnings := figConvert(t, src)

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	for _, want := range []string{
		`<div class="fig-panel" style="--fig-weight:1"><div class="fig-group fig-accent">`,
		`<div class="fig-group-title">Before</div>`,
		`<div class="fig-group-foot">the usual pattern</div>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// The foot is a trailing line, so it must follow the group's items.
	if strings.Index(out, `<div class="fig-group-foot">`) < strings.Index(out, `>Handler<`) {
		t.Errorf("the foot should come after the group's items:\n%s", out)
	}
}

// A lane is a list with nowhere to hang a flag. A lane that needs one is a
// group — the same rule as the panel case, and the reason lanes keeps its
// shipped shape rather than being respelled as a list of maps.
func TestFigLaneAccentRidesOnAGroup(t *testing.T) {
	src := "```fig\nitems:\n  - lanes:\n" +
		"      - - group: new\n          accent: true\n          items:\n            - box: A\n" +
		"      - - box: B\n```\n"
	out, warnings := figConvert(t, src)

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	if !strings.Contains(out, `<div class="fig-lane"><div class="fig-group fig-accent">`) {
		t.Errorf("an accented lane should be a group inside the lane:\n%s", out)
	}
}

// A group with neither modifier is unchanged, the same promise Task 1 made
// for the leaves.
func TestFigGroupMarkupUnchangedWithoutModifiers(t *testing.T) {
	out, _ := figConvert(t,
		"```fig\nitems:\n  - group: Server\n    items:\n      - box: A\n```\n")
	want := `<div class="fig-group"><div class="fig-group-title">Server</div><div class="fig-box">A</div></div>`
	if !strings.Contains(out, want) {
		t.Errorf("missing unchanged markup %q in:\n%s", want, out)
	}
}

func TestFigStatDetail(t *testing.T) {
	src := "```fig\nitems:\n  - stats:\n" +
		"      - value: 3 / 3\n        label: unit\n        detail: \"parse · render\"\n" +
		"      - value: 12\n        label: suites\n```\n"
	out, warnings := figConvert(t, src)

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	if !strings.Contains(out, `<span class="fig-stat-label">unit</span><span class="fig-stat-detail">parse · render</span>`) {
		t.Errorf("a detail should follow the label:\n%s", out)
	}
	// A tile with no detail emits no empty span.
	if !strings.Contains(out, `<span class="fig-stat-label">suites</span></div>`) {
		t.Errorf("a tile without a detail should be unchanged:\n%s", out)
	}
}

func TestFigTreeRenders(t *testing.T) {
	src := "```fig\nitems:\n  - tree: |\n" +
		"      fig.go -- the fence branch\n" +
		"        * figtree.go -- the line grammar\n" +
		"      - testdata/ -- not shipped\n```\n"
	out, warnings := figConvert(t, src)

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	want := `<ul class="fig-tree">` +
		`<li><span class="fig-tree-label">fig.go</span>` +
		`<span class="fig-note">the fence branch</span>` +
		`<ul><li class="fig-accent"><span class="fig-tree-label">figtree.go</span>` +
		`<span class="fig-note">the line grammar</span></li></ul></li>` +
		`<li class="fig-muted"><span class="fig-tree-label">testdata/</span>` +
		`<span class="fig-note">not shipped</span></li></ul>`
	if !strings.Contains(out, want) {
		t.Errorf("want\n%s\ngot\n%s", want, out)
	}
}

// A tree's label and note are author text like any other field, so both go
// through the inline pass — which is also what removes the backslash from
// an escaped sigil.
func TestFigTreeTakesInlineMarkdown(t *testing.T) {
	src := "```fig\nitems:\n  - tree: |\n" +
		"      `fig.go` -- see [design](./design.md)\n" +
		"      \\* not accented\n```\n"
	out, warnings := figConvert(t, src)

	if len(warnings) != 0 {
		t.Fatalf("want no warnings, got %v", warnings)
	}
	if !strings.Contains(out, `<span class="fig-tree-label"><code>fig.go</code></span>`) {
		t.Errorf("a tree label should render inline Markdown:\n%s", out)
	}
	if !strings.Contains(out, `<a href="./design.md">design</a>`) {
		t.Errorf("a tree note should render a link:\n%s", out)
	}
	if !strings.Contains(out, `<span class="fig-tree-label">* not accented</span>`) {
		t.Errorf("an escaped marker should render as a literal asterisk:\n%s", out)
	}
}

// A link that appears only in a tree note must reach the crawler, the same
// as one in a box label.
func TestFigTreeLinksAreExtracted(t *testing.T) {
	out, _ := figConvert(t,
		"```fig\nitems:\n  - tree: |\n      api -- see [ref](./api.md)\n```\n")
	root, err := parseFragment([]byte(out))
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	got := kindsOf(ExtractLinks(root, "/docs"))
	if k, ok := got["./api.md"]; !ok || k != LinkDoc {
		t.Errorf("ExtractLinks should return ./api.md as a document link, got %v", got)
	}
}

func TestFigEmptyTreeRendersAnEmptyList(t *testing.T) {
	out, warnings := figConvert(t, "```fig\nitems:\n  - tree: \"\"\n```\n")
	if len(warnings) != 0 {
		t.Fatalf("an empty tree is a legitimate spacer, got %v", warnings)
	}
	if !strings.Contains(out, `<ul class="fig-tree"></ul>`) {
		t.Errorf("want an empty list, got:\n%s", out)
	}
}
