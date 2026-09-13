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
