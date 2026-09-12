//go:build e2e_browser

// Package-level note for whoever appends the next test: every selector
// passed to chromedp (Click, WaitVisible, AttributeValue, etc.)
// needs the chromedp.ByQuery option. Without it, chromedp's default lookup
// is BySearch (DOM.performSearch), a fuzzy text/CSS/XPath search over the
// whole document — and it also matches selector text like "body" or "img"
// sitting inside this page's inlined <style> block, which silently hangs
// every wait until the context deadline. See the comment on
// TestBrowserLargeImageExpandsOnClick's chromedp.Run call for the full
// story.

package md2html

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// newBrowserCtx returns a context driving a headless, sandboxless Chrome
// instance, torn down automatically at the end of the test. no-sandbox is
// required in most CI containers, which run as root.
//
// It also installs filterChromedpNoise as chromedp's error sink, to drop
// one specific chromedp-internal log line that would otherwise print on
// every dialog.showModal() call in these tests — see that function's doc
// comment for why, and why it's narrowly scoped rather than a blanket mute.
func newBrowserCtx(t *testing.T) context.Context {
	t.Helper()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("no-sandbox", true))
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(filterChromedpNoise))
	ctx, timeoutCancel := context.WithTimeout(ctx, 15*time.Second)
	t.Cleanup(func() {
		timeoutCancel()
		cancel()
		allocCancel()
	})
	return ctx
}

// filterChromedpNoise is chromedp's errf sink (wired in by newBrowserCtx),
// with exactly one message dropped: the literal format string
// "unhandled node event %T", logged by chromedp@v0.16.0/target.go:422 for
// any DOM event its node-event dispatcher has no case for. Every dialog
// (mermaid or media) in these tests calls showModal(), which fires exactly
// that CDP event, and chromedp's own cdproto dependency already decodes it
// — chromedp's dispatcher just hasn't grown a case for it yet, so this is
// version skew rather than a real problem. Everything else is forwarded,
// unmodified, to log.Printf: chromedp's own default sink, and safe to call
// from the arbitrary goroutines this callback fires on (unlike t.Logf,
// which would panic with "Log in goroutine after Test has completed" if a
// message arrived after the test function returned). Matching happens on
// the format string, not the rendered message, so this only ever silences
// this one dispatcher gap and nothing else — a distinct chromedp-internal
// message still reaches log.Printf, confirmed separately.
//
// When bumping chromedp, check whether target.go still logs this: if a
// newer release added a case for the event, this filter becomes dead code
// and should be removed rather than carried forward as cargo cult.
func filterChromedpNoise(format string, args ...any) {
	if format == "unhandled node event %T" {
		return
	}
	log.Printf(format, args...)
}

// statusRecordingWriter wraps an http.ResponseWriter to capture the status
// code actually written, so the guard in serveDir's handler can inspect it
// without re-deriving it by stat-ing the filesystem.
type statusRecordingWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusRecordingWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// serveDir starts an HTTP server over a fresh temp directory and returns
// both, so callers needing to know baseURL before every file exists
// (TestBrowserMermaidDiagramExpandsOnClick, which must set mermaidCDN to a
// URL under baseURL before calling Convert) can write files in after the
// server is already serving.
//
// The plain files served here include a vendored mermaid build
// (TestBrowserMermaidDiagramExpandsOnClick) that lazily imports
// per-diagram chunks at runtime: only the chunks one
// particular diagram needs are vendored, on purpose, to keep the module
// zip small. A future library bump or a differently-shaped diagram will
// therefore request a chunk that was never written to disk. Left
// unguarded, that failure mode surfaces as a WaitVisible timeout fifteen
// seconds later with no indication of what went wrong, so this wraps the
// file server to fail the test immediately, by name, on any unexpected
// 404 — except /favicon.ico, which Chrome requests unprompted on every
// navigation and which will never exist.
func serveDir(t *testing.T) (dir, baseURL string) {
	t.Helper()
	dir = t.TempDir()
	fileServer := http.FileServer(http.Dir(dir))
	guarded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecordingWriter{ResponseWriter: w}
		fileServer.ServeHTTP(rec, r)
		if rec.status == http.StatusNotFound && r.URL.Path != "/favicon.ico" {
			// Must be Errorf, not Fatalf: this handler runs on the
			// server's own goroutine, and t.Fatalf from a non-test
			// goroutine is invalid — it would only stop that
			// goroutine, silently losing the failure.
			t.Errorf("serveDir: unexpected 404 for %s", r.URL.Path)
		}
	})
	srv := httptest.NewServer(guarded)
	t.Cleanup(srv.Close)
	return dir, srv.URL
}

// writeFiles drops files into dir, keyed by slash-separated path relative
// to dir. Safe to call more than once against the same dir, including
// after serveDir's server has already started — http.FileServer reads the
// directory live on each request.
func writeFiles(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

// serveGenerated is the convenience path for tests that know every file
// upfront: start a server, write the files, return the base URL.
func serveGenerated(t *testing.T, files map[string][]byte) (baseURL string) {
	t.Helper()
	dir, baseURL := serveDir(t)
	writeFiles(t, dir, files)
	return baseURL
}

// pngFixture synthesizes a solid-color PNG at an exact pixel size, so
// tests can assert on "scaled down from natural size" without committing
// binary test images to the repo.
func pngFixture(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{200, 80, 40, 255}}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic("pngFixture: " + err.Error())
	}
	return buf.Bytes()
}

// A 2000x1500 image is far wider than the page's prose measure, so it
// must become expandable, and clicking it must open the dialog with a
// clone carrying the image's real, natural size — not the shrunk-to-fit
// size the page displayed it at.
func TestBrowserLargeImageExpandsOnClick(t *testing.T) {
	page, err := Convert([]byte("![big](big.png)\n"), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{
		"index.html": page,
		"big.png":    pngFixture(2000, 1500),
	})

	ctx := newBrowserCtx(t)
	var expandable bool
	var width, height string
	var widthOK, heightOK bool
	// ByQuery is required on every selector below: chromedp's default query
	// strategy for a bare string selector is BySearch (DOM.performSearch),
	// a fuzzy text/CSS/XPath search over the whole document — not a plain
	// CSS query. Our stylesheet's text itself contains "body", "img", etc.
	// (e.g. "img, svg, video { max-width: 100%; ... }"), so BySearch also
	// matches inside the <style> element's text node. GetBoxModel on that
	// text-node match fails, and chromedp's retry loop swallows that error
	// and keeps retrying until the context deadline — surfacing as an
	// unexplained timeout with no indication a CSS query was ever the
	// problem. ByQuery pins every lookup to DOM.querySelector instead.
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Evaluate(`document.querySelector("img").classList.contains("expandable")`, &expandable),
		chromedp.Click(`img`, chromedp.ByQuery, chromedp.NodeVisible),
		chromedp.WaitVisible(`dialog.media-lightbox[open]`, chromedp.ByQuery),
		chromedp.AttributeValue(`dialog.media-lightbox img`, "width", &width, &widthOK, chromedp.ByQuery),
		chromedp.AttributeValue(`dialog.media-lightbox img`, "height", &height, &heightOK, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !expandable {
		t.Error("a 2000x1500 image never got .expandable")
	}
	if !widthOK || !heightOK {
		t.Fatal("cloned <img> in the dialog has no width/height attribute")
	}
	if width != "2000" || height != "1500" {
		t.Errorf("cloned image size = %sx%s, want 2000x1500", width, height)
	}
}

// A 10x10 image renders at its own size (nothing shrinks it), so it must
// never get .expandable — there'd be nothing to zoom into.
func TestBrowserSmallImageNotExpandable(t *testing.T) {
	page, err := Convert([]byte("![small](small.png)\n"), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{
		"index.html": page,
		"small.png":  pngFixture(10, 10),
	})

	ctx := newBrowserCtx(t)
	var runtimeRan bool
	var imgComplete bool
	var naturalWidth int
	var expandable bool
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Evaluate(`document.querySelector("dialog.media-lightbox") !== null`, &runtimeRan),
		chromedp.Evaluate(`document.querySelector("img").complete`, &imgComplete),
		chromedp.Evaluate(`document.querySelector("img").naturalWidth`, &naturalWidth),
		chromedp.Evaluate(`document.querySelector("img").classList.contains("expandable")`, &expandable),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	// mediaExpandRuntime appends dialog.media-lightbox to the body
	// unconditionally when it runs, so its presence is a free liveness
	// probe: without it, the absence assertion below would go green just
	// as happily if the runtime never ran at all (a JS syntax error, or
	// hasExpandableMedia regressing to false).
	if !runtimeRan {
		t.Fatal("mediaExpandRuntime never ran, so the absence assertion below proves nothing")
	}
	// The gate must have seen the image's real natural size, not a null
	// naturalSize from an image that hadn't loaded yet when the gate ran —
	// that would also report not-expandable, but for the wrong reason, and
	// wouldn't distinguish a correctly-rejected 10<=10 comparison from a
	// broken load-deferral path.
	if !imgComplete || naturalWidth != 10 {
		t.Fatalf("img.complete=%v naturalWidth=%d; want complete=true naturalWidth=10 — gate never saw real dimensions", imgComplete, naturalWidth)
	}
	if expandable {
		t.Error("a 10x10 image got .expandable; it's already at its own size")
	}
}

// Clicking the backdrop (anywhere outside the dialog's own box, not its
// content) must close it. chromedp.Click on a selector clicks that
// element's center, which would land on the dialog's own content — a
// coordinate click at the corner of the viewport is what actually lands
// on the ::backdrop.
func TestBrowserBackdropClickCloses(t *testing.T) {
	page, err := Convert([]byte("![big](big.png)\n"), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{
		"index.html": page,
		"big.png":    pngFixture(2000, 1500),
	})

	ctx := newBrowserCtx(t)
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Click(`img`, chromedp.ByQuery, chromedp.NodeVisible),
		chromedp.WaitVisible(`dialog.media-lightbox[open]`, chromedp.ByQuery),
		chromedp.MouseClickXY(2, 2),
		chromedp.WaitNotPresent(`dialog.media-lightbox[open]`, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
}

// An image already wrapped in a link keeps that link's behavior — the
// author chose it already, and a second, conflicting click meaning would
// override it.
func TestBrowserLinkedImageNotWired(t *testing.T) {
	page, err := Convert([]byte("[![big](big.png)](https://example.invalid)\n"), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{
		"index.html": page,
		"big.png":    pngFixture(2000, 1500),
	})

	ctx := newBrowserCtx(t)
	var runtimeRan bool
	var expandable bool
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Evaluate(`document.querySelector("dialog.media-lightbox") !== null`, &runtimeRan),
		chromedp.Evaluate(`document.querySelector("img").classList.contains("expandable")`, &expandable),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	// See TestBrowserSmallImageNotExpandable's identical probe: without
	// confirming the runtime actually ran, the absence assertion below
	// would be satisfied just as well by the runtime never running at all.
	if !runtimeRan {
		t.Fatal("mediaExpandRuntime never ran, so the absence assertion below proves nothing")
	}
	if expandable {
		t.Error("an image wrapped in <a> got .expandable; it should be left to the link")
	}
}

// mermaid's own <svg> lives inside pre.mermaid and is already wired up by
// mermaidRuntime; without this exclusion, media-expand's own click listener
// would double-wire it, and a click would fight over which dialog opens.
// This fixture hand-authors the DOM shape directly, with no real mermaid
// render involved, to test the exclusion filter alone.
//
// It still has to run on a page carrying class="mermaid", so hasMermaid
// makes renderPage inject mermaidRuntime alongside mediaExpandRuntime —
// which is exactly the double-wiring setup this exclusion exists to guard
// against, and mermaidRuntime's own import statement would otherwise reach
// across the network for the real, ~0.8MB CDN build (out of scope per the
// spec, and this suite must run offline). Worse, if that fetch succeeded,
// mermaid.initialize({startOnLoad: true}) would try to render this pre's
// content as diagram source, fail (it's SVG markup, not mermaid syntax),
// and replace the fixture's <svg> with mermaid's own error-diagram SVG —
// silently making every assertion below pass by inspecting the wrong
// element rather than testing anything. mermaidCDN is redirected at a
// local, inert ESM stub instead: it satisfies the one call the runtime
// makes on it (initialize) and does nothing else, so the fixture DOM is
// left exactly as authored while mermaidRuntime's lightbox-wiring code
// still runs for real.
//
// The svg's inline width/height style (shrinking its rendered box below
// its own viewBox) is deliberate: wireExpand only wires an element when its
// natural size exceeds its rendered box, so without forcing the box
// smaller than the 10x10 viewBox, this fixture would never become a
// candidate for .expandable regardless of the mermaid exclusion, and the
// falsifiability check below would be a silent no-op.
func TestBrowserSvgInsideMermaidPreExcluded(t *testing.T) {
	// mermaidCDN is shared package state, restored via t.Cleanup below. That
	// makes t.Parallel() unsafe on this test and on
	// TestBrowserMermaidDiagramExpandsOnClick, which does the same
	// reassignment: a concurrent run would race the two overrides against
	// each other. Six Chrome-spawning tests are a natural future candidate
	// for parallelism, so if that's added later, these two must stay
	// serial (or gain their own isolation) even though nothing enforces it
	// today — there is no t.Parallel() anywhere in this repo yet.
	original := mermaidCDN
	dir, baseURL := serveDir(t)
	writeFiles(t, dir, map[string][]byte{
		"mermaid-stub.js": []byte("export default { initialize() {} };\n"),
	})
	mermaidCDN = baseURL + "/mermaid-stub.js"
	t.Cleanup(func() { mermaidCDN = original })

	const src = `<pre class="mermaid"><svg viewBox="0 0 10 10" style="width:5px;height:5px"><circle cx="5" cy="5" r="4"/></svg></pre>
`
	page, err := Convert([]byte(src), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	writeFiles(t, dir, map[string][]byte{"index.html": page})

	ctx := newBrowserCtx(t)
	var viewBox string
	var viewBoxOK bool
	var mediaRuntimeRan bool
	var mermaidRuntimeRan bool
	var expandable bool
	var mediaDialogOpen bool
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.AttributeValue(`pre.mermaid svg`, "viewBox", &viewBox, &viewBoxOK, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector("dialog.media-lightbox") !== null`, &mediaRuntimeRan),
		chromedp.Evaluate(`document.querySelector("dialog.mermaid-lightbox") !== null`, &mermaidRuntimeRan),
		chromedp.Evaluate(`document.querySelector("pre.mermaid svg").classList.contains("expandable")`, &expandable),
		chromedp.Click(`pre.mermaid svg`, chromedp.ByQuery, chromedp.NodeVisible),
		chromedp.Evaluate(`document.querySelector("dialog.media-lightbox[open]") !== null`, &mediaDialogOpen),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	// Prove this is the hand-authored fixture, not a mermaid-generated
	// replacement, before drawing any conclusion from its class or click
	// behavior.
	if !viewBoxOK || viewBox != "0 0 10 10" {
		t.Fatalf("pre.mermaid svg viewBox = %q, ok=%v; want %q — fixture svg missing or replaced", viewBox, viewBoxOK, "0 0 10 10")
	}
	// Both runtimes append their own lightbox to the body unconditionally
	// when they run, so their presence is a free liveness probe for each —
	// without it, the absence assertions below would go green just as
	// happily if a runtime never ran at all. mermaidRuntimeRan doubles as
	// the proof that the local stub import above actually succeeded, which
	// is what makes this doc comment's claim that "mermaidRuntime's own
	// lightbox-wiring code still runs for real" true by construction
	// rather than assumed.
	if !mediaRuntimeRan {
		t.Fatal("mediaExpandRuntime never ran, so the absence assertions below prove nothing")
	}
	if !mermaidRuntimeRan {
		t.Fatal("mermaidRuntime never ran (stub import failed?), so its lightbox-wiring claim above is unverified")
	}
	if expandable {
		t.Error("an svg inside pre.mermaid got .expandable; that's mermaidRuntime's element")
	}
	if mediaDialogOpen {
		t.Error("clicking an svg inside pre.mermaid opened dialog.media-lightbox; it should be left to mermaidRuntime")
	}
}

// A hand-authored inline <svg> — not inside pre.mermaid — is the other half
// of mediaExpandRuntime's advertised scope (see its doc comment in page.go:
// "for plain <img> and hand-authored inline <svg>"), and unlike the <img>
// path covered by TestBrowserLargeImageExpandsOnClick, nothing else in this
// suite exercises the SVG branch of naturalSize positively.
// TestBrowserSvgInsideMermaidPreExcluded only proves an svg does NOT get
// wired up inside pre.mermaid; that's satisfied just as well by the SVG
// branch being entirely absent, so it can't stand in for this test.
//
// This fixture uses the same technique TestBrowserSvgInsideMermaidPreExcluded
// does, deliberately: an inline width/height style shrinks the rendered box
// (100x75) below the svg's own viewBox (800x600), which is what opens the
// size gate in wireExpand — a plain <svg> with no such style renders at its
// viewBox size, which would never exceed its own rendered box and would
// never become a candidate for .expandable regardless of whether the SVG
// branch of naturalSize works at all.
func TestBrowserSvgInsideExpandsOnClick(t *testing.T) {
	const src = `<svg viewBox="0 0 800 600" style="width:100px;height:75px"><rect x="0" y="0" width="800" height="600" fill="red"/></svg>
`
	page, err := Convert([]byte(src), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"index.html": page})

	ctx := newBrowserCtx(t)
	var expandable bool
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Evaluate(`document.querySelector("svg").classList.contains("expandable")`, &expandable),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	// Checked, and failed on, before the click: if naturalSize's svg branch
	// regresses to always-null, the svg never gets a click listener at all,
	// and clicking it below would just hang waiting for a dialog that never
	// opens — a context-deadline timeout that looks nothing like the real
	// problem. Fatal-ing here instead makes that regression fail on this
	// assertion, not a timeout.
	if !expandable {
		t.Fatal("an inline svg rendered below its viewBox size never got .expandable")
	}

	var widthOK, heightOK bool
	var width, height string
	err = chromedp.Run(ctx,
		chromedp.Click(`svg`, chromedp.ByQuery, chromedp.NodeVisible),
		chromedp.WaitVisible(`dialog.media-lightbox[open]`, chromedp.ByQuery),
		chromedp.AttributeValue(`dialog.media-lightbox svg`, "width", &width, &widthOK, chromedp.ByQuery),
		chromedp.AttributeValue(`dialog.media-lightbox svg`, "height", &height, &heightOK, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !widthOK || !heightOK {
		t.Fatal("cloned <svg> in the dialog has no width/height attribute")
	}
	// The clone must carry the viewBox-derived natural size (800x600), not
	// the 100x75 box it was displayed at — that distinction (natural size,
	// not rendered size) is the whole point of naturalSize's svg branch.
	if width != "800" || height != "600" {
		t.Errorf("cloned svg size = %sx%s, want 800x600 (the viewBox size, not the 100x75 displayed size)", width, height)
	}
}

// A real ```mermaid fence, rendered by the actual vendored library (not the
// hand-authored fixture TestBrowserSvgInsideMermaidPreExcluded uses above),
// must open dialog.mermaid-lightbox on click, with the clone carrying its
// real viewBox-derived size — proving mermaidRuntime's clone-sizing logic
// (see its doc comment in page.go) against genuine mermaid output rather
// than a fixture shaped by hand to match it.
//
// mermaid.esm.min.mjs is not a self-contained bundle: it's a ~30KB loader
// that lazily imports per-diagram chunks from ./chunks/mermaid.esm.min/ at
// runtime, as relative specifiers resolved against wherever the entrypoint
// itself was fetched from. Since the entrypoint is served here at
// vendor/mermaid.esm.min.mjs, those relative imports resolve against
// vendor/chunks/mermaid.esm.min/, so the whole testdata/vendor tree is
// walked and republished under vendor/ — renaming only the versioned
// entrypoint file — rather than hardcoding the chunk list, which would
// silently rot on a re-vendor. Only the chunks a `graph LR` flowchart needs
// are vendored (a deliberate choice to keep the module zip small), so this
// test sticks to that diagram type.
func TestBrowserMermaidDiagramExpandsOnClick(t *testing.T) {
	// mime.TypeByExtension(".mjs") already returns "text/javascript" on
	// macOS, via the system's own mime.types — so this call is a no-op
	// there. It's the portable fix for platforms without that
	// mapping: Chrome refuses to execute a module script served with the
	// wrong MIME type. mime.AddExtensionType is process-global with no way
	// to restore the previous mapping — acceptable in a test binary, but a
	// surprise if a reader assumes it's scoped to this test.
	mime.AddExtensionType(".mjs", "text/javascript")

	dir, baseURL := serveDir(t)

	vendorRoot := filepath.Join("testdata", "vendor")
	vendorFiles := map[string][]byte{}
	err := filepath.WalkDir(vendorRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(vendorRoot, path)
		if err != nil {
			return err
		}
		// Matched structurally rather than against the literal versioned
		// filename, so this doesn't become a fourth copy of the version pin
		// (alongside mermaidCDN and the vendored file itself) that goes
		// stale on a re-vendor. Only the top-level entrypoint
		// (testdata/vendor/mermaid-<version>.esm.min.mjs) matches: chunk
		// files live under chunks/ and their rel path starts with that
		// directory name, not "mermaid-".
		if strings.HasPrefix(rel, "mermaid-") && strings.HasSuffix(rel, ".esm.min.mjs") {
			rel = "mermaid.esm.min.mjs"
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		vendorFiles["vendor/"+filepath.ToSlash(rel)] = content
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", vendorRoot, err)
	}
	writeFiles(t, dir, vendorFiles)

	original := mermaidCDN
	mermaidCDN = baseURL + "/vendor/mermaid.esm.min.mjs"
	t.Cleanup(func() { mermaidCDN = original })

	page, err := Convert([]byte("```mermaid\ngraph LR\n  A --> B\n```\n"), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	// The generated page's only reference to a mermaid build is this literal
	// import string, baked in by mermaidRuntime at the Convert call above.
	// Confirming it names our local server — not the live jsdelivr CDN,
	// which a test machine may well be able to reach — rules out a false
	// green from a render served over the network instead of from the
	// vendored tree actually under test.
	if !bytes.Contains(page, []byte(`import mermaid from "`+mermaidCDN+`"`)) {
		t.Fatal("generated page does not import mermaid from the local test server")
	}
	writeFiles(t, dir, map[string][]byte{"index.html": page})

	ctx := newBrowserCtx(t)
	var roleDescription string
	var roleOK bool
	var widthOK, heightOK bool
	var width, height string
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		// Mermaid inserts the <svg> before it finishes writing this
		// attribute onto it, so waiting on the bare svg selector races the
		// attribute write; waiting on the attribute-selector variant instead
		// blocks until it's actually there. This isn't just an empirically
		// observed race: the vendored flowchart's draw() (chunk-CLS4B6BI.mjs)
		// ends with a synchronous setupViewPortForSVG call that writes
		// viewBox, and the top-level render then sets aria-roledescription
		// with no intervening await — so on JS's single-threaded runtime, any
		// observer that has seen the attribute has necessarily already seen
		// the viewBox. A future "simpler" wait on the bare svg selector would
		// silently reintroduce a race against the viewBox the width/height
		// assertions below depend on.
		chromedp.WaitVisible(`pre.mermaid svg[aria-roledescription]`, chromedp.ByQuery),
		chromedp.AttributeValue(`pre.mermaid svg`, "aria-roledescription", &roleDescription, &roleOK, chromedp.ByQuery),
		chromedp.Click(`pre.mermaid`, chromedp.ByQuery, chromedp.NodeVisible),
		chromedp.WaitVisible(`dialog.mermaid-lightbox[open]`, chromedp.ByQuery),
		chromedp.AttributeValue(`dialog.mermaid-lightbox svg`, "width", &width, &widthOK, chromedp.ByQuery),
		chromedp.AttributeValue(`dialog.mermaid-lightbox svg`, "height", &height, &heightOK, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	// aria-roledescription="flowchart-v2" is a marker only mermaid's own
	// flowchart renderer writes onto its output SVG; neither the
	// hand-authored fixture in TestBrowserSvgInsideMermaidPreExcluded nor a
	// silently-failed render would carry it, so this is the positive check
	// that the vendored library actually rendered this diagram.
	if !roleOK || roleDescription != "flowchart-v2" {
		t.Fatalf("pre.mermaid svg aria-roledescription = %q, ok=%v; want %q — real mermaid render never happened", roleDescription, roleOK, "flowchart-v2")
	}
	if !widthOK || !heightOK {
		t.Fatal("cloned <svg> in the mermaid dialog has no width/height attribute")
	}
	// Parsed as numbers, comfortably above zero: proof they came from a real
	// viewBox on a real rendered diagram, not merely non-empty strings.
	widthNum, werr := strconv.ParseFloat(width, 64)
	heightNum, herr := strconv.ParseFloat(height, 64)
	if werr != nil || herr != nil || widthNum <= 10 || heightNum <= 10 {
		t.Errorf("cloned svg size = %sx%s, want real dimensions from the viewBox", width, height)
	}
}
