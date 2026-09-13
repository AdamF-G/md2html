package md2html

import (
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
