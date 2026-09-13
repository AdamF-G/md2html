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
	"fmt"
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

// newGuardedHandler wraps fileServer so any unexpected 404 fails the test
// immediately, by name, rather than surfacing indirectly much later (see
// serveDir's doc comment for the motivating scenario). Shared by serveDir
// and serveDirDelayed so the guard logic — including the /favicon.ico
// exemption and the t.Errorf-not-t.Fatalf rule — exists in exactly one
// place.
func newGuardedHandler(t *testing.T, fileServer http.Handler) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
// file server (via newGuardedHandler) to fail the test immediately, by
// name, on any unexpected 404 — except /favicon.ico, which Chrome requests
// unprompted on every navigation and which will never exist.
func serveDir(t *testing.T) (dir, baseURL string) {
	t.Helper()
	dir = t.TempDir()
	fileServer := http.FileServer(http.Dir(dir))
	srv := httptest.NewServer(newGuardedHandler(t, fileServer))
	t.Cleanup(srv.Close)
	return dir, srv.URL
}

// serveDirDelayed is serveDir's sibling for a test that needs one specific
// path to still be genuinely in flight when the page's end-of-body script
// runs (TestBrowserSlowImageExpandsAfterDeferredLoad). It exists as a
// separate entry point, rather than an extra parameter on serveDir, so the
// five other callers of serveDir keep an unchanged signature.
//
// Only requests for slowPath are delayed; every other request — including
// the page itself — is served at ordinary http.FileServer speed, so the
// artificial delay affects exactly the one resource the caller is trying to
// keep in flight.
func serveDirDelayed(t *testing.T, slowPath string, delay time.Duration) (dir, baseURL string) {
	t.Helper()
	dir = t.TempDir()
	fileServer := http.FileServer(http.Dir(dir))
	delayed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == slowPath {
			time.Sleep(delay)
		}
		fileServer.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(newGuardedHandler(t, delayed))
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

// mediaExpandRuntime's dispatch loop defers wiring an <img> that isn't yet
// .complete: `el.addEventListener("load", () => wireExpand(el))` instead of
// calling wireExpand(el) directly. That branch has no coverage anywhere
// else in this suite — every other fixture's image is a tiny local PNG
// that's already .complete by the time the inline end-of-body <script>
// runs, so those tests only ever take the synchronous else branch. This
// test forces the deferred branch for real, by making the image response
// arrive after the script has already started running.
//
// chromedp.Navigate blocks on page.EventLoadEventFired, which by definition
// fires only once every in-flight resource — including this image — has
// finished. So by the time Navigate returns, the image has already loaded
// and, if the deferred branch is still there, its "load" listener has
// already fired wireExpand. That's what lets this test observe the
// deferred path's effect without an explicit wait of its own.
//
// The one thing that can't be waved away with "it passed": whether the
// image was still genuinely in flight — not yet .complete — at the moment
// the inline script's dispatch loop ran. If the artificial delay below were
// ever too short, or the image cached, it would already be .complete by
// then, the script would take the *synchronous* wireExpand(el) branch
// instead, and this test would keep passing while covering nothing — which
// is exactly what happened, repeatedly, while designing this test: a bare
// `wireExpand(el);` in place of the whole if/else left every test in this
// file green. An inline <script> at the end of <body> runs strictly before
// DOMContentLoaded, so proving the image's network response finished after
// domContentLoadedEventEnd is a deterministic proof — not a timing
// assumption — that the deferred branch, not the synchronous one, is what
// wired this element up.
func TestBrowserSlowImageExpandsAfterDeferredLoad(t *testing.T) {
	page, err := Convert([]byte("![slow](slow.png)\n"), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	// This is fixture latency, not the kind of fixed sleep-instead-of-
	// waiting-on-DOM-state this suite otherwise forbids: every wait this
	// test performs is still on real load/DOM state (chromedp.Navigate
	// blocking on the page's load event, then reading Performance API
	// entries afterward) — the delay only exists inside the HTTP handler,
	// to guarantee that wait actually straddles the moment
	// mediaExpandRuntime's dispatch loop runs. 300ms has a wide margin over
	// this document's parse time (a few hundred bytes of HTML/CSS/JS —
	// sub-millisecond to parse on any machine this runs on), so there's
	// slack to spare. Do not "simplify" this into a bare fast response: that
	// would silently delete the only coverage of the deferred branch (see
	// the doc comment above).
	const slowImageDelay = 300 * time.Millisecond
	dir, baseURL := serveDirDelayed(t, "/slow.png", slowImageDelay)
	writeFiles(t, dir, map[string][]byte{
		"index.html": page,
		"slow.png":   pngFixture(2000, 1500),
	})

	ctx := newBrowserCtx(t)
	var perf struct {
		NavOK                    bool    `json:"navOK"`
		ResOK                    bool    `json:"resOK"`
		DomContentLoadedEventEnd float64 `json:"domContentLoadedEventEnd"`
		ResponseEnd              float64 `json:"responseEnd"`
	}
	var expandable bool
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Evaluate(`(() => {
			const nav = performance.getEntriesByType("navigation")[0];
			const res = performance.getEntriesByType("resource").find((e) => e.name.endsWith("/slow.png"));
			return {
				navOK: !!(nav && nav.domContentLoadedEventEnd),
				resOK: !!res,
				domContentLoadedEventEnd: nav ? nav.domContentLoadedEventEnd : 0,
				responseEnd: res ? res.responseEnd : 0,
			};
		})()`, &perf),
		chromedp.Evaluate(`document.querySelector("img").classList.contains("expandable")`, &expandable),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	// Verified rather than assumed: if either Performance entry came back
	// empty, the margin computed below would be meaningless, and the
	// failure needs to say that plainly rather than report a bogus margin.
	if !perf.NavOK || !perf.ResOK {
		t.Fatalf("Performance API entries missing (navOK=%v resOK=%v) — cannot verify slow.png was still in flight when mediaExpandRuntime ran", perf.NavOK, perf.ResOK)
	}
	// This is the load-bearing check in this test: an inline end-of-body
	// <script> runs before DOMContentLoaded, so a positive margin here is a
	// deterministic proof that slow.png's response arrived strictly after
	// the dispatch loop had already run — i.e. that the deferred branch
	// (the "load" listener), not the synchronous else branch, is what made
	// this image expandable below. A future reader whose machine makes the
	// artificial delay too short — or a regression that removes it — should
	// see this failure and this message, not a confusing pass or an
	// unrelated timeout.
	margin := perf.ResponseEnd - perf.DomContentLoadedEventEnd
	if margin <= 0 {
		t.Fatalf("fixture failed to defer: slow.png's responseEnd (%.2fms) did not finish after domContentLoadedEventEnd (%.2fms) — the image was already .complete when mediaExpandRuntime's inline script ran, so this test exercised the synchronous branch, not the deferred one, and proves nothing about the deferred branch; increase slowImageDelay", perf.ResponseEnd, perf.DomContentLoadedEventEnd)
	}
	t.Logf("deferred-load margin (responseEnd - domContentLoadedEventEnd) = %.2fms", margin)

	// Checked, and failed on, before clicking: if the deferred branch is
	// gone (or broken), the image never gets .expandable and therefore
	// never gets a click listener at all, so clicking it below would just
	// hang waiting for a dialog that never opens — a context-deadline
	// timeout that looks nothing like the real problem and, worse, would
	// stop this test from being able to tell "the deferred branch broke"
	// apart from "chromedp/Chrome timed out for an unrelated reason".
	// Fatal-ing here instead makes that regression fail on this assertion.
	if !expandable {
		t.Fatal("a 2000x1500 image that was still loading when mediaExpandRuntime ran never got .expandable")
	}

	var width, height string
	var widthOK, heightOK bool
	err = chromedp.Run(ctx,
		chromedp.Click(`img`, chromedp.ByQuery, chromedp.NodeVisible),
		chromedp.WaitVisible(`dialog.media-lightbox[open]`, chromedp.ByQuery),
		chromedp.AttributeValue(`dialog.media-lightbox img`, "width", &width, &widthOK, chromedp.ByQuery),
		chromedp.AttributeValue(`dialog.media-lightbox img`, "height", &height, &heightOK, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
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
	var boxWidth float64
	var expandable bool
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Evaluate(`document.querySelector("dialog.media-lightbox") !== null`, &runtimeRan),
		chromedp.Evaluate(`document.querySelector("img").getBoundingClientRect().width`, &boxWidth),
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
	// Pin the actual boundary wireExpand's gate sits on: a 10x10 image
	// renders at a 10px-wide box (nothing in this page's CSS shrinks it),
	// so naturalWidth (10) equals box.width (10) exactly, and the gate's
	// `size.width <= box.width` must treat that equality as "not
	// expandable" — the real reason this image is rejected, not merely
	// that some assertion elsewhere happens to read false.
	//
	// This does not, by itself, distinguish that correct equality-rejection
	// from a broken load-deferral path that left size.width read as 0
	// before the image loaded: chromedp.Navigate already blocks until the
	// page's load event fires, so within this single test the image is
	// unconditionally .complete by the time any Evaluate above runs, and
	// there is no way to force the deferred branch's failure mode here.
	// TestBrowserSlowImageExpandsAfterDeferredLoad above covers that
	// scenario directly, by forcing the image to genuinely still be in
	// flight when mediaExpandRuntime's dispatch loop executes.
	if boxWidth != 10 {
		t.Fatalf("img rendered box width = %v, want 10 — expected a 10x10 image to render at its own natural size with nothing shrinking it", boxWidth)
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

// A collapsible container hides its body, but not the way "hidden" is
// usually spelled: Chrome implements a closed <details> with
// `content-visibility: hidden` on the implicit ::details-content slot,
// which skips painting and find-in-page while still laying the subtree
// out. An element in there keeps a real, correct
// getBoundingClientRect() — its width is the width it would render at if
// the container were open, not zero.
//
// That is load-bearing for mediaExpandRuntime, which decides whether an
// element is worth wiring by comparing its natural size against exactly
// that rendered box (`size.width <= box.width` in wireExpand). Inside a
// closed container the comparison still gets a truthful answer, so a
// small image is correctly left alone and a large one is correctly wired
// — the same verdicts TestBrowserSmallImageNotExpandable and
// TestBrowserLargeImageExpandsOnClick pin at the top level.
//
// If the stylesheet ever hid container bodies with `display: none`
// instead, or if a future Chrome stopped laying the subtree out, every
// box in there would measure 0 wide, every icon and badge inside a closed
// container would satisfy the gate, and the page would sprout zoom
// cursors on things with nothing to zoom into. No Go-level test can see
// that: it is a fact about layout, not about markup, and the markup is
// byte-for-byte identical either way. Hence this test.
func TestBrowserMediaExpandInsideCollapsedContainer(t *testing.T) {
	md := "::: aside Collapsed\n\n![small](small.png)\n\n![big](big.png)\n\n:::\n"
	page, err := Convert([]byte(md), Options{Transforms: Builtins()})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	// Transforms had to be passed explicitly above: Convert with a zero
	// Options runs none, and without Containers the fence would never
	// become a <details> at all — leaving two plainly visible images and a
	// test that proves nothing about collapsed containers.
	if !bytes.Contains(page, []byte(`<details class="container aside">`)) {
		t.Fatalf("fixture did not become a collapsible container: %s", page)
	}
	baseURL := serveGenerated(t, map[string][]byte{
		"index.html": page,
		"small.png":  pngFixture(60, 40),
		"big.png":    pngFixture(2000, 1500),
	})

	ctx := newBrowserCtx(t)
	var runtimeRan, containerClosed bool
	var smallHidden, bigHidden bool
	var smallBox, bigBox float64
	var smallExpandable, bigExpandable bool
	// checkVisibility({contentVisibilityAuto: true}) is the only DOM API
	// that reports a content-visibility-skipped element as invisible;
	// offsetParent, display and getBoundingClientRect all report it as an
	// ordinary laid-out box, which is precisely the distinction this test
	// exists to pin.
	const hiddenJS = `document.querySelector(%q).checkVisibility({contentVisibilityAuto: true, visibilityProperty: true}) === false`
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Evaluate(`document.querySelector("dialog.media-lightbox") !== null`, &runtimeRan),
		chromedp.Evaluate(`!document.querySelector("details.container").open`, &containerClosed),
		chromedp.Evaluate(fmt.Sprintf(hiddenJS, `img[src="small.png"]`), &smallHidden),
		chromedp.Evaluate(fmt.Sprintf(hiddenJS, `img[src="big.png"]`), &bigHidden),
		chromedp.Evaluate(`document.querySelector('img[src="small.png"]').getBoundingClientRect().width`, &smallBox),
		chromedp.Evaluate(`document.querySelector('img[src="big.png"]').getBoundingClientRect().width`, &bigBox),
		chromedp.Evaluate(`document.querySelector('img[src="small.png"]').classList.contains("expandable")`, &smallExpandable),
		chromedp.Evaluate(`document.querySelector('img[src="big.png"]').classList.contains("expandable")`, &bigExpandable),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	// Same liveness probe as TestBrowserSmallImageNotExpandable: the
	// negative assertion below would be satisfied just as well by the
	// runtime never having run.
	if !runtimeRan {
		t.Fatal("mediaExpandRuntime never ran, so the absence assertion below proves nothing")
	}
	// Both halves of the premise, asserted rather than assumed. Without
	// the first, the container might have rendered open and this would be
	// a duplicate of the top-level tests; without the second, "laid out"
	// and "hidden" would not both be established and the whole point of
	// the test would be unproven.
	if !containerClosed {
		t.Fatal("details.container rendered open; this test only says anything while it is closed")
	}
	if !smallHidden || !bigHidden {
		t.Fatalf("images inside the closed container report as visible (small=%v big=%v); the container is not actually hiding its body", !smallHidden, !bigHidden)
	}
	// The load-bearing measurement: a hidden-but-laid-out subtree still
	// reports true widths. 60 exactly, because nothing in this page's CSS
	// shrinks a 60px image below the prose measure — the same boundary
	// TestBrowserSmallImageNotExpandable pins at the top level.
	if smallBox != 60 {
		t.Fatalf("small img box width inside a closed container = %v, want 60 — a closed <details> is no longer laying its content out, and wireExpand's size gate is now comparing against a phantom", smallBox)
	}
	if bigBox <= 60 {
		t.Fatalf("big img box width inside a closed container = %v, want the prose measure — expected the 2000px image to be shrunk to fit, not left unlaid-out", bigBox)
	}
	if smallExpandable {
		t.Error("a 60x40 image inside a closed container got .expandable; it is already at its own size, and being inside a collapsed container must not change that verdict")
	}
	if !bigExpandable {
		t.Error("a 2000x1500 image inside a closed container did not get .expandable; being inside a collapsed container must not suppress the verdict either")
	}

	// And the verdict has to survive contact with the reader: open the
	// container and click the image that was wired while hidden. Wiring
	// happened against the closed-state box, so this is the assertion that
	// the listener attached back then still opens the dialog, at the
	// image's real natural size rather than the size it was measured at.
	var dialogWidth string
	var widthOK bool
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelector("details.container").open = true`, nil),
		chromedp.Click(`img[src="big.png"]`, chromedp.ByQuery, chromedp.NodeVisible),
		chromedp.WaitVisible(`dialog.media-lightbox[open]`, chromedp.ByQuery),
		chromedp.AttributeValue(`dialog.media-lightbox img`, "width", &dialogWidth, &widthOK, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("chromedp (after opening the container): %v", err)
	}
	if !widthOK || dialogWidth != "2000" {
		t.Errorf("lightbox clone width = %q, ok=%v; want \"2000\" — the clone should carry the image's natural size, not the box it was measured at while collapsed", dialogWidth, widthOK)
	}
}

// The collapsible container kinds are a <details>, on purpose: default.css
// says so in as many words ("<details> gives open/closed state, keyboard
// access and find-in-page expansion natively; a scripted accordion would
// give up all three"), and containerKinds encodes it as tag: "details".
// Nothing asserted any of it — a container that silently stopped
// disclosing, or a summary that stopped being reachable by keyboard, would
// leave every Go-level test green, because the markup those tests inspect
// is not where the behavior lives.
//
// The three claims are checked in the order a reader meets them: closed by
// default, opens on a pointer click, and toggles from the keyboard alone.
// Find-in-page expansion is the fourth claim and is not checked here —
// CDP has no find-in-page command to drive it with — but it is the one
// behavior that follows automatically from the others being native rather
// than scripted, which is itself what this test pins.
func TestBrowserCollapsibleContainerDiscloses(t *testing.T) {
	md := "::: aside Why this matters\n\nAside body.\n\n:::\n\n" +
		"::: example Two\n\nExample body.\n\n:::\n"
	page, err := Convert([]byte(md), Options{Transforms: Builtins()})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	// Checked in the markup before the browser is involved, for the same
	// reason serveDir guards against unexpected 404s: if the kinds stopped
	// becoming <details> at all, the WaitReady below would simply never
	// find its selector and the test would die fifteen seconds later on a
	// bare "context deadline exceeded" naming nothing. Verified by
	// deliberately flipping containerKinds' tag to "div" — this is the
	// line that then reports it.
	if n := bytes.Count(page, []byte(`<details class="container `)); n != 2 {
		t.Fatalf("fixture produced %d collapsible containers, want 2: %s", n, page)
	}
	baseURL := serveGenerated(t, map[string][]byte{"index.html": page})

	// bodyState reports, for each container on the page, whether it is
	// open and whether its body paragraph is actually visible to a reader.
	// The two are read together and compared against each other below: a
	// container whose open property and whose rendered body disagree is
	// exactly the regression this test is looking for, and reading only
	// the property would miss a stylesheet that forced the body hidden (or
	// visible) regardless of state.
	const bodyState = `[...document.querySelectorAll("details.container")].map(d => ({
	  open: d.open,
	  bodyVisible: d.querySelector("p").checkVisibility({contentVisibilityAuto: true, visibilityProperty: true}),
	}))`
	type state struct {
		Open        bool `json:"open"`
		BodyVisible bool `json:"bodyVisible"`
	}
	var initial, afterClick, afterKey []state
	var summaryTabIndex int
	var summaryCursor string
	ctx := newBrowserCtx(t)
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.WaitReady(`details.container > summary`, chromedp.ByQuery),
		chromedp.Evaluate(bodyState, &initial),

		// Pointer disclosure. The click lands on the summary, which is the
		// only part of a closed <details> a reader can see or hit.
		chromedp.Click(`details.container > summary`, chromedp.ByQuery),
		chromedp.Evaluate(bodyState, &afterClick),

		// Keyboard disclosure. Enter on the focused summary is the native
		// toggle; sending it without first focusing would dispatch to the
		// body and prove nothing. chromedp.Focus and KeyEvent are enough
		// here because CDP dispatches to the renderer's focused element
		// directly — note that a :focus-dependent *style* assertion would
		// additionally need emulation.SetFocusEmulationEnabled(true),
		// since a headless page has no window focus and :focus therefore
		// never matches. Nothing below depends on a focus style, so that
		// is deliberately not turned on.
		chromedp.Focus(`details.container > summary`, chromedp.ByQuery),
		chromedp.KeyEvent("\r"),
		chromedp.Evaluate(bodyState, &afterKey),

		chromedp.Evaluate(`document.querySelector("details.container > summary").tabIndex`, &summaryTabIndex),
		chromedp.Evaluate(`getComputedStyle(document.querySelector("details.container > summary")).cursor`, &summaryCursor),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}

	if len(initial) != 2 {
		t.Fatalf("got %d collapsible containers, want 2 — the aside/example fixture did not render as <details>", len(initial))
	}
	for i, s := range initial {
		if s.Open || s.BodyVisible {
			t.Errorf("container %d starts open=%v bodyVisible=%v; both must be false — a collapsible container that ships open discloses nothing", i, s.Open, s.BodyVisible)
		}
	}
	// Only the clicked container moves. Asserted because a scripted
	// accordion — the thing default.css's comment rejects — is exactly
	// what would couple them, and because it confirms the click did
	// something specific rather than the page re-rendering wholesale.
	if !afterClick[0].Open || !afterClick[0].BodyVisible {
		t.Errorf("after clicking its summary, container 0 is open=%v bodyVisible=%v; want both true", afterClick[0].Open, afterClick[0].BodyVisible)
	}
	if afterClick[1].Open || afterClick[1].BodyVisible {
		t.Errorf("clicking container 0's summary also disclosed container 1 (open=%v bodyVisible=%v); the kinds must disclose independently", afterClick[1].Open, afterClick[1].BodyVisible)
	}
	// Enter re-collapses the one the click opened. Checking the toggle
	// back — rather than opening a fresh container with the keyboard —
	// proves the key reached the summary and not merely that something
	// somewhere ended up open.
	if afterKey[0].Open || afterKey[0].BodyVisible {
		t.Errorf("after Enter on the focused summary, container 0 is open=%v bodyVisible=%v; want both false — the keyboard toggle did not reach it", afterKey[0].Open, afterKey[0].BodyVisible)
	}
	// The affordances the stylesheet promises the reader, alongside the
	// behavior itself: a summary that works but looks inert is a
	// regression a behavioral assertion alone would miss.
	if summaryTabIndex != 0 {
		t.Errorf("summary tabIndex = %d, want 0 — a collapsible container's handle must be reachable by keyboard", summaryTabIndex)
	}
	if summaryCursor != "pointer" {
		t.Errorf("summary cursor = %q, want %q", summaryCursor, "pointer")
	}
}

// A cols layout must put its panels side by side on a wide viewport and
// stack them on a phone. This is the one piece of figure behavior no markup
// assertion can observe: the stylesheet's only media query decides it.
//
// The rest of this suite exists for JavaScript, which figures ship none of.
// Layout under a media query is still behavior, and it still needs a real
// engine to observe, so it earns the one exception.
func TestBrowserFigColsStackWhenNarrow(t *testing.T) {
	page, err := Convert([]byte(
		"```fig\nlayout: cols\nitems:\n  - box: Left\n  - box: Right\n```\n"),
		Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"fig.html": page})

	ctx := newBrowserCtx(t)

	// Both panels' left edges, read from the live layout.
	const edges = `(() => {
		const p = [...document.querySelectorAll(".fig-panel")];
		return p.map(e => e.getBoundingClientRect().left).join(",");
	})()`

	// No sleep between the resize and the read: getBoundingClientRect
	// forces a synchronous layout, and CDP applies the metrics override
	// before the next evaluation returns. Polling until the assertion
	// holds would turn a real failure into a timeout, which is worse
	// diagnostics, not better.
	var wide, narrow string
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1200, 800),
		chromedp.Navigate(baseURL+"/fig.html"),
		chromedp.WaitVisible(".fig-cols", chromedp.ByQuery),
		chromedp.Evaluate(edges, &wide),
		chromedp.EmulateViewport(390, 800),
		chromedp.Evaluate(edges, &narrow),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	wideLeft := strings.Split(wide, ",")
	narrowLeft := strings.Split(narrow, ",")
	if len(wideLeft) != 2 || len(narrowLeft) != 2 {
		t.Fatalf("want two panels, got wide=%q narrow=%q", wide, narrow)
	}
	if wideLeft[0] == wideLeft[1] {
		t.Errorf("panels should share a row when wide, both left edges at %s", wideLeft[0])
	}
	if narrowLeft[0] != narrowLeft[1] {
		t.Errorf("panels should stack when narrow, left edges %q vs %q",
			narrowLeft[0], narrowLeft[1])
	}
}
