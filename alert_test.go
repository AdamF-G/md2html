package md2html

import (
	"strings"
	"testing"
)

// GitHub alert syntax is the only callout spelling that renders natively
// where these source files actually live. It must land on exactly the
// markup the equivalent ::: container produces.
func TestAlertWarningBecomesWarningCallout(t *testing.T) {
	got := convert(t, "> [!WARNING]\n> Overwrites state.\n", nil)
	if !strings.Contains(got, `<div class="callout callout-warning">`) {
		t.Errorf("alert did not become a warning callout\ngot: %s", got)
	}
	if strings.Contains(got, "[!WARNING]") {
		t.Errorf("marker left in the body\ngot: %s", got)
	}
	if strings.Contains(got, "<blockquote") {
		t.Errorf("blockquote not replaced\ngot: %s", got)
	}
}

func TestAlertNoteBecomesCallout(t *testing.T) {
	got := convert(t, "> [!NOTE]\n> Worth knowing.\n", nil)
	if !strings.Contains(got, `<div class="callout">`) {
		t.Errorf("note alert did not become a callout\ngot: %s", got)
	}
	if !strings.Contains(got, "Worth knowing.") {
		t.Errorf("body lost\ngot: %s", got)
	}
}

// The full GitHub vocabulary, mapped onto the two shipped kinds.
func TestAlertTypeMapping(t *testing.T) {
	cases := []struct{ marker, class string }{
		{"NOTE", `class="callout"`},
		{"TIP", `class="callout"`},
		{"IMPORTANT", `class="callout"`},
		{"WARNING", `class="callout callout-warning"`},
		{"CAUTION", `class="callout callout-warning"`},
	}
	for _, c := range cases {
		got := convert(t, "> [!"+c.marker+"]\n> Body.\n", nil)
		if !strings.Contains(got, "<div "+c.class+">") {
			t.Errorf("[!%s] did not map to %s\ngot: %s", c.marker, c.class, got)
		}
	}
}

// An alert renders identically to the container it is an alias for.
func TestAlertMatchesEquivalentContainer(t *testing.T) {
	viaAlert := convert(t, "> [!WARNING]\n> Overwrites state.\n", nil)
	viaFence := convert(t, "::: warning\nOverwrites state.\n:::\n", nil)
	if viaAlert != viaFence {
		t.Errorf("alert and container disagree\nalert:     %q\ncontainer: %q", viaAlert, viaFence)
	}
}

func TestAlertPlainBlockquoteUntouched(t *testing.T) {
	got := convert(t, "> Just a quotation.\n", nil)
	if !strings.Contains(got, "<blockquote>") {
		t.Errorf("plain blockquote was rewritten\ngot: %s", got)
	}
}

// GitHub requires the marker in upper case. Accepting other spellings
// would let a document render here and not there, which is backwards.
func TestAlertLowercaseMarkerIsNotAnAlert(t *testing.T) {
	got := convert(t, "> [!note]\n> Body.\n", nil)
	if !strings.Contains(got, "<blockquote>") {
		t.Errorf("lower-case marker was treated as an alert\ngot: %s", got)
	}
}

func TestAlertUnknownMarkerIsNotAnAlert(t *testing.T) {
	got := convert(t, "> [!HINT]\n> Body.\n", nil)
	if !strings.Contains(got, "<blockquote>") {
		t.Errorf("unknown marker was treated as an alert\ngot: %s", got)
	}
}

// The marker has to be alone on the first line, as GitHub requires.
func TestAlertMarkerMidSentenceIsNotAnAlert(t *testing.T) {
	got := convert(t, "> see [!NOTE] here\n", nil)
	if !strings.Contains(got, "<blockquote>") {
		t.Errorf("mid-sentence marker was treated as an alert\ngot: %s", got)
	}
}

func TestAlertKeepsMultipleBlocks(t *testing.T) {
	got := convert(t, "> [!NOTE]\n> First.\n>\n> Second.\n", nil)
	if !strings.Contains(got, "First.") || !strings.Contains(got, "Second.") {
		t.Errorf("alert dropped a block\ngot: %s", got)
	}
}
