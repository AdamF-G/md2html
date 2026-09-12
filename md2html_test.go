package md2html

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// normEntities makes numeric and named entity escaping compare equal.
func normEntities(s string) string {
	r := strings.NewReplacer("&#x3C;", "&lt;", "&#x26;", "&amp;", "&#x3E;", "&gt;")
	return r.Replace(s)
}

// conformanceChecks is the contract for the pipeline: every element an
// Artifact page uses must survive conversion.
var conformanceChecks = []struct {
	name string
	want *regexp.Regexp
	deny bool // if true, the pattern must NOT appear
}{
	{name: "title survives", want: regexp.MustCompile(`<title>Signal Decay</title>`)},
	{name: "style verbatim", want: regexp.MustCompile(`--accent: #b4532a`)},
	{name: "css blank line intact", want: regexp.MustCompile(`\.grid > \* \+ \*`)},
	{name: "markdown inside div parsed", want: regexp.MustCompile(`(?s)<div class="grid">.*?<h2.*?<strong>markdown</strong>`)},
	{name: "inline span passthrough", want: regexp.MustCompile(`<span class="badge">badge</span>`)},
	{name: "inline svg intact", want: regexp.MustCompile(`(?s)<svg viewBox="0 0 100 40".*?<circle cx="50"`)},
	{name: "mermaid becomes pre.mermaid", want: regexp.MustCompile(`(?s)<pre class="mermaid">\s*graph LR`)},
	{name: "js fence escaped", want: regexp.MustCompile(`const ok = a &lt; b &amp;&amp; c (&gt;|>) d;`)},
	{name: "gfm table", want: regexp.MustCompile(`(?s)<table>.*?Loss \(dB\)`)},
	{name: "definition list", want: regexp.MustCompile(`(?s)<dl>.*?<dt>Term</dt>`)},
	{name: "footnotes", want: regexp.MustCompile(`<sup`)},
	{name: "script verbatim", want: regexp.MustCompile(`if \(x &lt; 10 &amp;&amp; x (&gt;|>) 1\)|if \(x < 10 && x > 1\)`)},
	{name: "script asterisks not emphasized", want: regexp.MustCompile(`<em>not emphasis</em>`), deny: true},
	{name: "heading attributes applied", want: regexp.MustCompile(`<h2[^>]*id="custom-id"[^>]*>`)},
	{name: "heading class applied", want: regexp.MustCompile(`<h2[^>]*class="[^"]*lead[^"]*"`)},
	{name: "fenced container becomes div", want: regexp.MustCompile(`<div[^>]*class="[^"]*callout[^"]*"`)},
}

func TestConformance(t *testing.T) {
	src, err := os.ReadFile("testdata/conformance.md")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	got, err := Convert(src, Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out := normEntities(string(got))

	for _, c := range conformanceChecks {
		t.Run(c.name, func(t *testing.T) {
			found := c.want.MatchString(out)
			if c.deny && found {
				t.Errorf("pattern %q should NOT appear, but did\n\noutput:\n%s", c.want, out)
			}
			if !c.deny && !found {
				t.Errorf("pattern %q not found\n\noutput:\n%s", c.want, out)
			}
		})
	}
}

// The sink exists so a transform can report a problem without aborting the
// document. Nothing warns yet — Task 2 is the first caller — so this
// asserts only that a plain document stays silent and that Convert accepts
// and threads the callback.
func TestConvertWarnSinkSilentOnCleanInput(t *testing.T) {
	var got []string
	out, err := Convert([]byte("# Title\n\ntext\n"), Options{
		Fragment: true,
		Warn:     func(m string) { got = append(got, m) },
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("warned on clean input: %v", got)
	}
	if !strings.Contains(string(out), "<h1") {
		t.Errorf("no heading in output: %s", out)
	}
}

// A nil sink must be safe: every existing caller passes one.
func TestConvertNilWarnSinkIsSafe(t *testing.T) {
	if _, err := Convert([]byte("# T\n"), Options{Fragment: true}); err != nil {
		t.Fatalf("Convert: %v", err)
	}
}

// Builtins keeps its documented no-argument signature — README shows it —
// and the transform order it returns is a correctness constraint, not an
// incidental one, so pin both the signature and the exact ordered names.
func TestBuiltinsSignatureUnchanged(t *testing.T) {
	var _ func() []Transform = Builtins

	got := Builtins()
	want := []string{"containers", "tableScroll", "chips", "headingAnchors", "externalLinks"}
	if len(got) != len(want) {
		t.Fatalf("Builtins() returned %d transforms, want %d: %v", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("Builtins()[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
}
