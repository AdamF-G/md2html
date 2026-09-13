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

// A link that appears only inside a figure must still be crawlable.
func TestFigLinksAreExtracted(t *testing.T) {
	out, _ := figConvert(t, "```fig\nitems:\n  - box: \"[api](./api.md)\"\n```\n")
	if !strings.Contains(out, `href="./api.md"`) {
		t.Errorf("want an anchor the crawler can see, got:\n%s", out)
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
