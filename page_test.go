package md2html

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	var matches []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "mermaid-") {
			matches = append(matches, e.Name())
		}
	}
	if len(matches) == 0 {
		t.Fatal("no vendored mermaid build found in testdata/vendor")
	}
	// More than one match means a re-vendor added a new version's entrypoint
	// without deleting the old one — a stale build would still ship in the
	// module zip even though this check, looking only at the newest match,
	// would otherwise pass.
	if len(matches) > 1 {
		t.Fatalf("testdata/vendor has %d mermaid-* entrypoints (%v); want exactly one — delete the stale one", len(matches), matches)
	}
	found := matches[0]
	version := strings.TrimSuffix(strings.TrimPrefix(found, "mermaid-"), ".esm.min.mjs")
	// "@" + version + "/" rather than a bare Contains: mermaidCDN pins a
	// specific patch (e.g. "mermaid@11.17.2/"), and a loose substring match
	// would let version "11.17.2" match a CDN URL actually pinned at
	// "mermaid@11.17.20/".
	if !strings.Contains(mermaidCDN, "@"+version+"/") {
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

// The vendored mermaid bundle is not a standalone file: every .mjs is a chunk
// that imports other chunks statically (e.g., from"./chunk-X.mjs"). Those
// static imports must all resolve on disk, or a browser loading the bundle
// hangs waiting for 404s.
//
// Dynamic imports (parenthesized: import("./x.mjs")) are intentionally
// skipped — these are per-diagram-type lazy loads that are not vendored by
// design; a test that required them would fail by design and hide a real
// breakage. Static imports, by contrast, are what the module needs merely to
// load, and every one must be present.
//
// This intentionally does NOT skip bare-module specifiers (an import path
// with no leading "./" or "../") before resolving them as filesystem paths,
// even though a bare specifier will never resolve relative to mjsFile's
// directory and will always land in missingFiles. That's correct, not a
// false positive: a browser loading an ES module with a bare specifier and
// no import map fails outright, so if a future mermaid build ever leaves
// one unbundled, this guard firing here — loudly, by name, with no browser
// needed — is exactly the right outcome. Adding a skip for bare specifiers
// would suppress that genuine failure and just push it downstream into the
// browser suite, where it resurfaces as a much less legible timeout.
func TestVendoredMermaidImportsResolveOnDisk(t *testing.T) {
	vendorRoot := filepath.Join("testdata", "vendor")

	// Walk all .mjs files in testdata/vendor and its subdirectories.
	var mjsFiles []string
	err := filepath.Walk(vendorRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".mjs") {
			mjsFiles = append(mjsFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk testdata/vendor: %v", err)
	}
	if len(mjsFiles) == 0 {
		t.Fatal("no .mjs files found in testdata/vendor; nothing to check")
	}

	// Patterns for static imports in ES6 modules. Real mermaid is minified with no
	// whitespace, so imports are unambiguous:
	// - `}from"path"` - import with destructuring (the `}` is from the import list)
	// - `;import"path"` - side-effect imports (the `;` separates statements)
	//
	// These patterns can't occur in strings, so they safely identify real imports.
	// Dynamic imports import("...") are NOT matched because they have `(` instead of `;`.
	patternRe := regexp.MustCompile(`}from"([^"]+)"|;import"([^"]+)"`)

	var staticImportCount int
	var missingFiles []string

	for _, mjsFile := range mjsFiles {
		content, err := os.ReadFile(mjsFile)
		if err != nil {
			t.Fatalf("read %s: %v", mjsFile, err)
		}
		contentStr := string(content)

		// Track which imports we've already seen to avoid duplicates
		seen := make(map[string]bool)

		matches := patternRe.FindAllStringSubmatchIndex(contentStr, -1)
		for _, match := range matches {
			// Groups: (2,3) for }from"..." pattern, (4,5) for ;import"..." pattern
			var importPath string
			if match[2] != -1 {
				// }from"..." matched (group 1)
				importPath = contentStr[match[2]:match[3]]
			} else if match[4] != -1 {
				// ;import"..." matched (group 2)
				importPath = contentStr[match[4]:match[5]]
			}

			if importPath == "" || seen[importPath] {
				continue
			}

			seen[importPath] = true
			staticImportCount++

			importedFile := filepath.Join(filepath.Dir(mjsFile), importPath)
			importedFile = filepath.Clean(importedFile)

			if _, err := os.Stat(importedFile); err != nil {
				missingFiles = append(missingFiles, fmt.Sprintf("%s imports %q, resolved to %s (missing)", mjsFile, importPath, importedFile))
			}
		}
	}

	if staticImportCount == 0 {
		t.Fatal("no static imports found in vendored .mjs files; extraction regex may be broken")
	}

	if len(missingFiles) > 0 {
		t.Errorf("vendored mermaid has %d missing dependencies:\n  %s", len(missingFiles), strings.Join(missingFiles, "\n  "))
	}
}
