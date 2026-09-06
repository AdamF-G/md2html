package md2html

import (
	"strings"
	"testing"
)

func TestExtractTitlePrefersFirstH1(t *testing.T) {
	root, _ := parseFragment([]byte(`<p>x</p><h1>Real Title</h1><h1>Second</h1>`))
	if got := extractTitle(root, "/docs/ignored.md"); got != "Real Title" {
		t.Errorf("got %q, want %q", got, "Real Title")
	}
}

func TestExtractTitleFallsBackToFilename(t *testing.T) {
	root, _ := parseFragment([]byte(`<p>no heading</p>`))
	if got := extractTitle(root, "/docs/api-reference.md"); got != "api-reference" {
		t.Errorf("got %q, want %q", got, "api-reference")
	}
}

func TestPageShellIsCompleteDocument(t *testing.T) {
	got := string(renderPage([]byte(`<p>hi</p>`), "T", "body{}"))
	for _, want := range []string{"<!doctype html>", "<html", "<head>", "<title>T</title>", "<body", "<p>hi</p>"} {
		if !strings.Contains(got, want) {
			t.Errorf("page missing %q\ngot: %s", want, got)
		}
	}
}

// Artifact shape: marker, title, style, body — and none of the document
// wrapper tags, which the Artifact tool supplies itself.
func TestFragmentShellOmitsDocumentWrapper(t *testing.T) {
	got := string(renderFragment([]byte(`<p>hi</p>`), "T", "body{}"))
	for _, banned := range []string{"<!doctype", "<html", "<head>", "<body"} {
		if strings.Contains(strings.ToLower(got), banned) {
			t.Errorf("fragment must not contain %q\ngot: %s", banned, got)
		}
	}
	for _, want := range []string{"<title>T</title>", "<style>", "<p>hi</p>"} {
		if !strings.Contains(got, want) {
			t.Errorf("fragment missing %q\ngot: %s", want, got)
		}
	}
}

func TestBothShellsStartWithMarker(t *testing.T) {
	for name, got := range map[string][]byte{
		"page":     renderPage([]byte(`<p>x</p>`), "T", ""),
		"fragment": renderFragment([]byte(`<p>x</p>`), "T", ""),
	} {
		if !strings.HasPrefix(string(got), MarkerPrefix) {
			t.Errorf("%s does not start with marker prefix\ngot: %.80s", name, got)
		}
	}
}

func TestMarkerCarriesFullRepoURL(t *testing.T) {
	if !strings.Contains(Marker(), "https://github.com/AdamF-G/md2html") {
		t.Errorf("marker must carry the full repo URL, got %q", Marker())
	}
}

func TestDefaultCSSDefinesBothThemes(t *testing.T) {
	for _, want := range []string{":root", "prefers-color-scheme: dark", `[data-theme="dark"]`, `[data-theme="light"]`} {
		if !strings.Contains(defaultCSS, want) {
			t.Errorf("default.css missing %q", want)
		}
	}
}

// The derived title must not include the "#" that HeadingAnchors appends.
func TestConvertTitleExcludesAnchorText(t *testing.T) {
	got, err := Convert([]byte("# Doc Title\n\ntext"), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(string(got), "<title>Doc Title</title>") {
		t.Errorf("title polluted by anchor text: %s", got)
	}
}

func TestConvertFragmentEndToEnd(t *testing.T) {
	got, err := Convert([]byte("# Doc Title\n\ntext"), Options{Fragment: true})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	s := string(got)
	if !strings.HasPrefix(s, MarkerPrefix) {
		t.Errorf("no marker: %.80s", s)
	}
	if !strings.Contains(s, "<title>Doc Title</title>") {
		t.Errorf("title not derived from h1: %s", s)
	}
	if strings.Contains(strings.ToLower(s), "<body") {
		t.Errorf("fragment leaked a body tag: %s", s)
	}
}
