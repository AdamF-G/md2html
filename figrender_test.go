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
