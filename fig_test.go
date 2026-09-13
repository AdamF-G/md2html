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
			name:    "unknown key",
			src:     "```fig\nitems:\n  - boxes: A\n```\n",
			wantMsg: "field boxes not found",
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
	var warnings []string
	got, err := Convert(src, Options{Warn: func(m string) { warnings = append(warnings, m) }})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out := string(got)

	for _, want := range []string{
		"fig-rail", "fig-box", "fig-arrow", "fig-result",
		"fig-stat-value", "fig-defs", "fig-group", "fig-chain",
		"fig-lane", "fig-cols", "fig-split", "fig-boundary",
		"--fig-weight:3",
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
	if !strings.Contains(out, `<div class="fig-panel" style="--fig-weight:1"><div class="fig-arrow" aria-hidden="true">`) {
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
