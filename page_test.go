package md2html

import (
	"os"
	"path/filepath"
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

// A standalone page has to load mermaid itself or a ```mermaid fence renders
// as inert preformatted text. Fragments must NOT: they are published as
// Artifacts, which render mermaid natively, and a second copy would conflict.
func TestMermaidRuntimeInjectedOnlyForPagesThatNeedIt(t *testing.T) {
	const diagram = "```mermaid\ngraph TD; A-->B;\n```\n"
	for _, c := range []struct {
		name string
		src  string
		opt  Options
		want bool
	}{
		{"page with a diagram", diagram, Options{}, true},
		{"page without a diagram", "# Plain\n\ntext\n", Options{}, false},
		{"fragment with a diagram", diagram, Options{Fragment: true}, false},
	} {
		got, err := Convert([]byte(c.src), c.opt)
		if err != nil {
			t.Fatalf("%s: Convert: %v", c.name, err)
		}
		if has := strings.Contains(string(got), mermaidCDN); has != c.want {
			t.Errorf("%s: mermaid runtime present = %v, want %v", c.name, has, c.want)
		}
	}
}

// The runtime must follow the same two theme signals the stylesheet uses, or
// a dark page renders a glaring light diagram.
func TestMermaidRuntimeIsThemeAware(t *testing.T) {
	got, err := Convert([]byte("```mermaid\ngraph TD; A-->B;\n```\n"), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	for _, want := range []string{"prefers-color-scheme: dark", "data-theme", "dark"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("mermaid runtime ignores %q", want)
		}
	}
}

// A diagram must be clickable and the click must land on a <dialog>, not a
// hand-rolled overlay — that's what buys Escape-to-close and a backdrop for
// free. The fragment path must stay untouched: Artifacts render mermaid
// natively and supply their own zoom, so a second copy would conflict.
func TestMermaidClickToExpand(t *testing.T) {
	const diagram = "```mermaid\ngraph TD; A-->B;\n```\n"
	page, err := Convert([]byte(diagram), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	s := string(page)
	for _, want := range []string{"mermaid-lightbox", "dialog", "showModal", `pre.mermaid`} {
		if !strings.Contains(s, want) {
			t.Errorf("page missing %q for click-to-expand", want)
		}
	}
	// The clone must get real width/height from the viewBox and drop the
	// inline style Mermaid attaches, or it resolves width:100% against a
	// fit-content <dialog> and renders as an empty box.
	for _, want := range []string{"viewBox", `removeAttribute("style")`, `setAttribute("width"`} {
		if !strings.Contains(s, want) {
			t.Errorf("page missing %q; clone would collapse to zero size in the dialog", want)
		}
	}

	frag, err := Convert([]byte(diagram), Options{Fragment: true})
	if err != nil {
		t.Fatalf("Convert fragment: %v", err)
	}
	if strings.Contains(string(frag), "showModal") {
		t.Error("fragment must not get the lightbox script; Artifacts render mermaid natively")
	}
}

// A standalone page has to supply its own click-to-expand script for images
// and inline SVG, the same reason mermaid needs one: nothing else adds it.
// Fragments must NOT get it — kept consistent with the mermaid runtime so
// Artifact output stays script-minimal.
func TestMediaExpandRuntimeInjectedOnlyForPagesThatNeedIt(t *testing.T) {
	const img = "![alt](pic.png)\n"
	const svg = "<svg viewBox=\"0 0 10 10\"><circle r=\"5\"/></svg>\n"
	for _, c := range []struct {
		name string
		src  string
		opt  Options
		want bool
	}{
		{"page with an image", img, Options{}, true},
		{"page with inline svg", svg, Options{}, true},
		{"page with neither", "# Plain\n\ntext\n", Options{}, false},
		{"fragment with an image", img, Options{Fragment: true}, false},
	} {
		got, err := Convert([]byte(c.src), c.opt)
		if err != nil {
			t.Fatalf("%s: Convert: %v", c.name, err)
		}
		if has := strings.Contains(string(got), "naturalSize("); has != c.want {
			t.Errorf("%s: media-expand runtime present = %v, want %v", c.name, has, c.want)
		}
	}
}

// An image or inline SVG must be clickable into a native <dialog>, and only
// when it's actually being scaled down — an icon rendered at its own size
// has nothing to zoom into. The click must skip elements already wrapped in
// a link (the author chose that behavior already) and mermaid's own SVG
// (that's the other runtime's job, and double-wiring it would open two
// lightboxes on one click).
func TestMediaClickToExpand(t *testing.T) {
	const img = "![alt](pic.png)\n"
	page, err := Convert([]byte(img), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	s := string(page)
	for _, want := range []string{"media-lightbox", "showModal", `closest("a")`, `closest("pre.mermaid")`} {
		if !strings.Contains(s, want) {
			t.Errorf("page missing %q for click-to-expand", want)
		}
	}
	// The scaled-down check and the clone both need the element's real size:
	// naturalWidth/Height for <img>, viewBox for inline <svg>, compared
	// against its rendered box.
	for _, want := range []string{"naturalWidth", "viewBox", "getBoundingClientRect"} {
		if !strings.Contains(s, want) {
			t.Errorf("page missing %q; can't tell a scaled-down image from one at its own size", want)
		}
	}

	frag, err := Convert([]byte(img), Options{Fragment: true})
	if err != nil {
		t.Fatalf("Convert fragment: %v", err)
	}
	if strings.Contains(string(frag), "naturalSize(") {
		t.Error("fragment must not get the media-expand lightbox script")
	}
}

// Long inline code must be able to wrap. Block code sits in a <pre> with its
// own overflow-x, but inline code has no scroll container, so without this a
// single long snippet widens the whole page — found by converting this repo's
// own plan, where `go test ./... -run 'A|B|C'` pushed a 360px page 627px wide.
func TestDefaultCSSWrapsInlineCode(t *testing.T) {
	for _, want := range []string{":not(pre) > code", "overflow-wrap: anywhere"} {
		if !strings.Contains(defaultCSS, want) {
			t.Errorf("default.css missing %q; long inline code would widen the page", want)
		}
	}
	// Block code must keep scrolling rather than wrapping.
	if !strings.Contains(defaultCSS, "overflow-x: auto") {
		t.Error("default.css: pre lost its overflow-x")
	}
}

// The zoom cursor is applied by the runtime script (only once it has decided
// an element is actually scaled down), via a class the stylesheet must know
// about — plus the dialog itself, styled the same way as mermaid's.
func TestDefaultCSSStylesMediaLightbox(t *testing.T) {
	for _, want := range []string{".expandable", "dialog.media-lightbox", "cursor: zoom-out"} {
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

// A future version bump to mermaidCDN must re-vendor the library, or every
// mermaid browser test in e2e_browser_test.go silently starts testing a
// stale build. This check needs no browser, so it runs in the default,
// non-gated suite and fails loudly the moment the two drift apart.
func TestVendoredMermaidVersionMatchesPinned(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("testdata", "vendor"))
	if err != nil {
		t.Fatalf("read testdata/vendor: %v", err)
	}
	var found string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "mermaid-") {
			found = e.Name()
		}
	}
	if found == "" {
		t.Fatal("no vendored mermaid build found in testdata/vendor")
	}
	version := strings.TrimSuffix(strings.TrimPrefix(found, "mermaid-"), ".esm.min.mjs")
	if !strings.Contains(mermaidCDN, version) {
		t.Errorf("vendored file %q (version %q) does not match pinned mermaidCDN %q", found, version, mermaidCDN)
	}
}

// mermaidCDN must be reassignable so a test can redirect the runtime's
// import at a local, vendored copy instead of the live CDN. This is the
// one production-code change the e2e browser suite depends on — proven
// here without needing a browser at all.
func TestMermaidCDNIsOverridable(t *testing.T) {
	original := mermaidCDN
	mermaidCDN = "http://example.test/mermaid.mjs"
	t.Cleanup(func() { mermaidCDN = original })

	got, err := Convert([]byte("```mermaid\ngraph TD; A-->B;\n```\n"), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	s := string(got)
	if !strings.Contains(s, "http://example.test/mermaid.mjs") {
		t.Error("mermaidRuntime did not pick up the overridden mermaidCDN")
	}
	if strings.Contains(s, "cdn.jsdelivr.net") {
		t.Error("mermaidRuntime still embedded the real CDN URL after override")
	}
}
