// Package-level note for whoever appends the next test: every selector
// passed to chromedp (Click, WaitVisible, AttributeValue, etc.)
// needs the chromedp.ByQuery option. Without it, chromedp's default lookup
// is BySearch (DOM.performSearch), a fuzzy text/CSS/XPath search over the
// whole document — and it also matches selector text like "body" or "img"
// sitting inside this page's inlined <style> block, which silently hangs
// every wait until the context deadline. See the comment on
// TestBrowserLargeImageExpandsOnClick's chromedp.Run call for the full
// story.

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"io/fs"
	"log"
	"math"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AdamF-G/md2html"
	"github.com/chromedp/chromedp"
)

// Two budgets, because they bound genuinely different things.
//
// stepTimeout is the diagnostic one: it turns a selector that never matches
// into a named failure in a minute, instead of the ten-minute panic the Go
// test binary would otherwise produce. Every step in this file takes single
// -digit seconds on a developer's machine; the margin is for shared CI
// hardware, where 15s was measurably too tight.
//
// startTimeout covers getting a browser at all, which is not this suite's
// subject and is far less predictable — see TestMain.
const (
	stepTimeout  = 60 * time.Second
	startTimeout = 90 * time.Second
)

// TestMain launches and discards one browser before any test runs.
//
// The first Chrome launch on a machine that has not run it recently pays to
// fault the binary and its libraries in from cold page cache; later
// launches in the same run reuse that and take a second or two. Without
// this, that cost landed inside the first test's own stepTimeout, so the
// first test in the file — and only ever that one — failed at exactly 15s
// while the other ten passed. CI did precisely that on a fresh runner:
// passed once, then failed twice on an unchanged tree.
//
// Warming keeps that cost where it belongs — paid once, by nobody's
// assertion — but on its own it was not enough: with it in place the first
// test still exhausted a 15s budget, this time inside its own chromedp.Run
// rather than at the launch. Both changes were needed, and only the pair
// has been observed green. If a future reader is tempted to drop one,
// widening stepTimeout is the half with direct evidence behind it; this
// warm-up's evidence is narrower, that it changed the reported failure from
// "chrome failed to start" to an ordinary deadline.
func TestMain(m *testing.M) {
	warmChrome()
	os.Exit(m.Run())
}

// warmChrome starts a browser, waits for it to be ready, and throws it
// away. It never fails the run: this is cache warming, not an assertion,
// and a genuinely unusable Chrome is reported by the first real test with
// a test name attached — which is a far more legible failure than one from
// a function that runs before the suite exists.
//
// It does log how long the launch took and, if it failed, the error and
// Chrome's output. go test only shows that when the package fails, which
// is exactly when the warm-up's part in a failure needs to be known.
func warmChrome() {
	out := newChromeOutput()
	start := time.Now()
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), allocatorOptions(out)...)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(func(string, ...any) {}))
	startCtx, startCancel := context.WithTimeout(ctx, startTimeout)
	err := chromedp.Run(startCtx)
	elapsed := time.Since(start)
	startCancel()
	cancel()
	allocCancel()
	if err != nil {
		log.Printf("warmChrome: launch failed after %v: %v\nChrome output:\n%s", elapsed.Round(time.Millisecond), err, out)
		return
	}
	log.Printf("warmChrome: launched in %v", elapsed.Round(time.Millisecond))
}

// allocatorOptions is the Chrome launch configuration shared by warmChrome
// and newBrowserCtx: headless and sandboxless, with everything Chrome
// prints on stdout and stderr copied to out.
//
// chromedp otherwise discards that output, so a launch that fails with
// "websocket url timeout reached" — Chrome still running but never
// announcing its DevTools address — leaves nothing to say what Chrome was
// doing instead. CI failed that way once, in the first test, with no way
// to tell why; the output is here so the next occurrence can.
func allocatorOptions(out io.Writer) []chromedp.ExecAllocatorOption {
	return append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("no-sandbox", true),
		chromedp.CombinedOutput(out),
	)
}

// chromeOutput collects one Chrome process's output, each write stamped
// with the time since the collector was made, so a stall shows up as a gap
// between stamps. chromedp writes to it from its own goroutines, hence the
// lock.
type chromeOutput struct {
	mu    sync.Mutex
	start time.Time
	buf   bytes.Buffer
}

func newChromeOutput() *chromeOutput {
	return &chromeOutput{start: time.Now()}
}

func (o *chromeOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	fmt.Fprintf(&o.buf, "[%6.2fs] %s", time.Since(o.start).Seconds(), p)
	if len(p) > 0 && p[len(p)-1] != '\n' {
		o.buf.WriteByte('\n')
	}
	return len(p), nil
}

func (o *chromeOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.buf.Len() == 0 {
		return "(none)"
	}
	return o.buf.String()
}

// newBrowserCtx returns a context driving a headless, sandboxless Chrome
// instance, torn down automatically at the end of the test. no-sandbox is
// required in most CI containers, which run as root.
//
// The deadline is stepTimeout, and it covers this browser's launch as well
// as the test's own steps — which is safe because TestMain has already paid
// the expensive first launch for the whole package.
//
// It also installs filterChromedpNoise as chromedp's error sink, to drop
// one specific chromedp-internal log line that would otherwise print on
// every dialog.showModal() call in these tests — see that function's doc
// comment for why, and why it's narrowly scoped rather than a blanket mute.
//
// If the test fails, Chrome's output is logged after the browser is torn
// down; see allocatorOptions for why.
func newBrowserCtx(t *testing.T) context.Context {
	t.Helper()
	out := newChromeOutput()
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), allocatorOptions(out)...)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(filterChromedpNoise))
	ctx, timeoutCancel := context.WithTimeout(ctx, stepTimeout)
	t.Cleanup(func() {
		timeoutCancel()
		cancel()
		allocCancel()
		if t.Failed() {
			t.Logf("Chrome output:\n%s", out)
		}
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
// (TestBrowserMermaidDiagramExpandsOnClick, which must name a MermaidURL
// under baseURL before calling Convert) can write files in after the
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
	page, err := md2html.Convert([]byte("![big](big.png)\n"), md2html.Options{})
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
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	// Asserted before clicking, in its own Run, the way
	// TestBrowserSlowImageExpandsAfterDeferredLoad does and for the same
	// reason: an image that never got .expandable never got a click
	// listener either, so clicking it below would hang until the deadline
	// waiting for a dialog nothing is going to open — a timeout that looks
	// nothing like the real problem. Reading it in the same Run as the
	// click does not help, because the click's own wait expires first and
	// the assertion is never reached.
	//
	// No wait is needed for the class itself: chromedp.Navigate blocks on
	// the window load event, which fires after every image's own load event
	// and therefore after mediaExpandRuntime's deferred branch has wired
	// them.
	if !expandable {
		t.Fatal("a 2000x1500 image never got .expandable")
	}

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
	page, err := md2html.Convert([]byte("![slow](slow.png)\n"), md2html.Options{})
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
	page, err := md2html.Convert([]byte("![small](small.png)\n"), md2html.Options{})
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
	page, err := md2html.Convert([]byte("![big](big.png)\n"), md2html.Options{})
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
	page, err := md2html.Convert([]byte("[![big](big.png)](https://example.invalid)\n"), md2html.Options{})
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
// element rather than testing anything. Options.MermaidURL names a local,
// inert ESM stub instead: it satisfies the one call the runtime makes on it
// (initialize) and does nothing else, so the fixture DOM is left exactly as
// authored while the runtime's lightbox-wiring code still runs for real.
//
// The svg's inline width/height style (shrinking its rendered box below
// its own viewBox) is deliberate: wireExpand only wires an element when its
// natural size exceeds its rendered box, so without forcing the box
// smaller than the 10x10 viewBox, this fixture would never become a
// candidate for .expandable regardless of the mermaid exclusion, and the
// falsifiability check below would be a silent no-op.
func TestBrowserSvgInsideMermaidPreExcluded(t *testing.T) {
	dir, baseURL := serveDir(t)
	writeFiles(t, dir, map[string][]byte{
		"mermaid-stub.js": []byte("export default { initialize() {} };\n"),
	})

	const src = `<pre class="mermaid"><svg viewBox="0 0 10 10" style="width:5px;height:5px"><circle cx="5" cy="5" r="4"/></svg></pre>
`
	page, err := md2html.Convert([]byte(src), md2html.Options{
		MermaidURL: baseURL + "/mermaid-stub.js",
	})
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
	page, err := md2html.Convert([]byte(src), md2html.Options{})
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

	// Up one directory: the vendored build belongs to the library's own
	// testdata (page_test.go pins its version against the default URL there),
	// and this module only borrows it to serve offline.
	vendorRoot := filepath.Join("..", "testdata", "vendor")
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
		// (alongside the pinned URL and the vendored file itself) that goes
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

	mermaidURL := baseURL + "/vendor/mermaid.esm.min.mjs"

	page, err := md2html.Convert([]byte("```mermaid\ngraph LR\n  A --> B\n```\n"),
		md2html.Options{MermaidURL: mermaidURL})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	// The generated page's only reference to a mermaid build is this literal
	// import string, baked in by mermaidRuntime at the Convert call above.
	// Confirming it names our local server — not the live jsdelivr CDN,
	// which a test machine may well be able to reach — rules out a false
	// green from a render served over the network instead of from the
	// vendored tree actually under test.
	if !bytes.Contains(page, []byte(`import mermaid from "`+mermaidURL+`"`)) {
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
	page, err := md2html.Convert([]byte(md), md2html.Options{Transforms: md2html.Builtins()})
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
	page, err := md2html.Convert([]byte(md), md2html.Options{Transforms: md2html.Builtins()})
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

// A cols or split layout must put its panels side by side on a wide viewport
// and stack them on a phone, each stacked panel spanning its container. This
// is the one piece of figure behavior no markup assertion can observe: the
// stylesheet's only media query decides it.
//
// The rest of this suite exists for JavaScript, which figures ship none of.
// Layout under a media query is still behavior, and it still needs a real
// engine to observe, so it earns the one exception.
func TestBrowserFigPanelsStackWhenNarrow(t *testing.T) {
	for _, c := range []struct {
		name, src string
		// ratio, when set, is the first panel's weight over the second's: the
		// ratio their content widths must keep while they share a row. Every
		// panel grows from a zero basis, so the free width is shared out by
		// weight exactly, and the card's padding and border are added on top.
		ratio float64
	}{
		{name: "cols layout", src: "```fig\nlayout: cols\nitems:\n  - box: Left\n  - box: Right\n```\n"},
		// Nested layouts have to stack by the same rule: the media query
		// targets .fig-cols and .fig-split by class, not by depth.
		{name: "cols item", src: "```fig\nitems:\n  - box: Lead\n  - cols:\n      - box: Left\n      - box: Right\n```\n"},
		{name: "split layout", src: "```fig\nlayout: split\nboundary: to\nitems:\n  - box: Left\n  - box: Right\n```\n"},
		{name: "split item", src: "```fig\nitems:\n  - box: Lead\n  - split:\n      - box: Left\n      - box: Right\n    boundary: to\n```\n"},
		// A chain sizes its other steps to their content; a cols step takes
		// the width they leave, and its weights must share out all of it.
		{name: "cols item in a chain", ratio: 3,
			src: "```fig\nitems:\n  - chain:\n      - box: Request\n      - arrow: \"\"\n" +
				"      - cols:\n          - box: Primary\n            weight: 3\n          - box: Replica\n```\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			page, err := md2html.Convert([]byte(c.src), md2html.Options{})
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}
			baseURL := serveGenerated(t, map[string][]byte{"fig.html": page})

			ctx := newBrowserCtx(t)

			// Both panels' boxes and their container's width, read from the
			// live layout.
			const probe = `(() => {
				const p = [...document.querySelectorAll(".fig-cols > .fig-panel, .fig-split > .fig-panel")];
				const box = e => {
					const r = e.getBoundingClientRect(), s = getComputedStyle(e);
					const edges = ["paddingLeft", "paddingRight", "borderLeftWidth", "borderRightWidth"]
						.reduce((sum, k) => sum + parseFloat(s[k]), 0);
					return {left: r.left, right: r.right, top: r.top, bottom: r.bottom, width: r.width, content: r.width - edges,
						overflow: e.scrollWidth - e.clientWidth};
				};
				return {panels: p.map(box), container: p.length ? p[0].parentElement.getBoundingClientRect().width : 0};
			})()`

			type rect struct{ Left, Right, Top, Bottom, Width, Content, Overflow float64 }
			type sample struct {
				Panels    []rect
				Container float64
			}

			// No sleep between the resize and the read: getBoundingClientRect
			// forces a synchronous layout, and CDP applies the metrics override
			// before the next evaluation returns. Polling until the assertion
			// holds would turn a real failure into a timeout, which is worse
			// diagnostics, not better.
			var wide, narrow sample
			if err := chromedp.Run(ctx,
				chromedp.EmulateViewport(1200, 800),
				chromedp.Navigate(baseURL+"/fig.html"),
				chromedp.WaitVisible(".fig-panel", chromedp.ByQuery),
				chromedp.Evaluate(probe, &wide),
				chromedp.EmulateViewport(390, 800),
				chromedp.Evaluate(probe, &narrow),
			); err != nil {
				t.Fatalf("browser run: %v", err)
			}

			if len(wide.Panels) != 2 || len(narrow.Panels) != 2 {
				t.Fatalf("want two panels, got wide=%+v narrow=%+v", wide.Panels, narrow.Panels)
			}
			if w := wide.Panels; w[1].Left < w[0].Right {
				t.Errorf("panels should share a row when wide, got %+v then %+v", w[0], w[1])
			}
			// Weights can hold exactly while a light panel is still narrower
			// than its own text, so the ratio alone does not show the panels
			// fit: each one's content must fit inside it too.
			for i, p := range wide.Panels {
				if p.Overflow > 1 {
					t.Errorf("panel %d's content is %vpx wider than the panel when wide", i, p.Overflow)
				}
			}
			if w := wide.Panels; c.ratio != 0 {
				if got := w[0].Content / w[1].Content; math.Abs(got-c.ratio) > 0.05 {
					t.Errorf("panel content widths should keep the %v:1 weight ratio, got %.2f:1 (%v vs %v)",
						c.ratio, got, w[0].Content, w[1].Content)
				}
			}
			if n := narrow.Panels; n[1].Top < n[0].Bottom {
				t.Errorf("panels should stack when narrow, got %+v then %+v", n[0], n[1])
			}
			for i, p := range narrow.Panels {
				if math.Abs(p.Width-narrow.Container) > 0.5 {
					t.Errorf("stacked panel %d should span its container (%v wide), got %v", i, narrow.Container, p.Width)
				}
			}
		})
	}
}

// A breakout is not observable from markup: the class is there either way
// and only the live layout says whether the figure is actually wider than
// the column it sits in. This is the second thing in fig that nothing but a
// browser can see, so it rides the harness the stacking test already built.
func TestBrowserFigWideBreaksOutOfTheMeasure(t *testing.T) {
	page, err := md2html.Convert([]byte(
		"Body text.\n\n```fig\nwide: true\nitems:\n  - box: Wide\n```\n\n"+
			"```fig\nitems:\n  - box: Normal\n```\n"),
		md2html.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"wide.html": page})

	ctx := newBrowserCtx(t)

	const widths = `(() => {
		const wr = document.querySelector("figure.fig-wide").getBoundingClientRect();
		const n = document.querySelector("figure.fig:not(.fig-wide)").getBoundingClientRect().width;
		const v = document.documentElement.clientWidth;
		return [wr.width, wr.left, wr.right, n, v].join(",");
	})()`

	var wide, narrow string
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1200, 800),
		chromedp.Navigate(baseURL+"/wide.html"),
		chromedp.WaitVisible("figure.fig-wide", chromedp.ByQuery),
		chromedp.Evaluate(widths, &wide),
		chromedp.EmulateViewport(390, 800),
		chromedp.Evaluate(widths, &narrow),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	parse := func(t *testing.T, s string) (w, left, right, n, v float64) {
		t.Helper()
		parts := strings.Split(s, ",")
		if len(parts) != 5 {
			t.Fatalf("want five values, got %q", s)
		}
		for i, dst := range []*float64{&w, &left, &right, &n, &v} {
			f, err := strconv.ParseFloat(parts[i], 64)
			if err != nil {
				t.Fatalf("parsing %q: %v", parts[i], err)
			}
			*dst = f
		}
		return w, left, right, n, v
	}

	w, left, right, n, v := parse(t, wide)
	if w <= n {
		t.Errorf("a wide figure should be wider than a normal one, got %v vs %v", w, n)
	}
	if w > v {
		t.Errorf("a wide figure must never exceed the viewport, got %v > %v", w, v)
	}
	if left < 0 {
		t.Errorf("a wide figure must not run off the left edge, got left=%v", left)
	}
	if right > v {
		t.Errorf("a wide figure must not run off the right edge, got right=%v > clientWidth=%v", right, v)
	}

	w, left, right, n, v = parse(t, narrow)
	if w > v {
		t.Errorf("a wide figure must never exceed a narrow viewport, got %v > %v", w, v)
	}
	if left < 0 {
		t.Errorf("a wide figure must not run off the left edge, got left=%v", left)
	}
	if right > v {
		t.Errorf("a wide figure must not run off the right edge, got right=%v > clientWidth=%v", right, v)
	}
	if math.Abs(w-n) > 0.5 {
		t.Errorf("the breakout should collapse when narrow, got %v vs %v", w, n)
	}
}

// The fig tests below assert what markup cannot: that a note actually reads
// as secondary, that the accent ring is not clipped away by an ancestor, and
// that a tree's indentation and per-line emphasis survive the cascade.
//
// Colour assertions run under both themes, because every colour in the fig
// stylesheet comes from a token that flips with data-theme — asserting one
// theme would leave the other untested and it is the one nobody looks at.
// Geometry is asserted once: it does not depend on the palette.
//
// None of these assert a literal colour. They assert that an element's
// colour IS its token's value, resolved by the browser, so restyling the
// palette does not break them but detaching a rule from its token does.

// figResolveVar asks the browser what a custom property paints as, rather
// than parsing hex out of the stylesheet: the comparison target has to be
// the same rgb() string getComputedStyle returns for the element under test.
const figResolveVar = `
	function resolveVar(name) {
		const p = document.createElement("span");
		p.style.color = "var(" + name + ")";
		document.body.appendChild(p);
		const v = getComputedStyle(p).color;
		p.remove();
		return v;
	}`

// figSetTheme stamps an explicit theme on the root element. The stylesheet
// defines :root[data-theme="dark"], so this is the same switch a reader's
// toggle throws, not a test-only back door.
func figSetTheme(theme string) chromedp.Action {
	var ok bool
	return chromedp.Evaluate(
		fmt.Sprintf(`(document.documentElement.setAttribute("data-theme", %q), true)`, theme), &ok)
}

func TestBrowserFigNoteIsSecondary(t *testing.T) {
	page, err := md2html.Convert([]byte(
		"```fig\nitems:\n  - box: Client\n    note: retries twice, then gives up\n```\n"),
		md2html.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"note.html": page})
	ctx := newBrowserCtx(t)

	const probe = figResolveVar + `
	(() => {
		const box = document.querySelector(".fig-box");
		const note = box.querySelector(".fig-note");
		return {
			boxSize:  parseFloat(getComputedStyle(box).fontSize),
			noteSize: parseFloat(getComputedStyle(note).fontSize),
			boxColor:  getComputedStyle(box).color,
			noteColor: getComputedStyle(note).color,
			muted:     resolveVar("--muted"),
		};
	})()`

	type sample struct {
		BoxSize, NoteSize          float64
		BoxColor, NoteColor, Muted string
	}
	var light, dark sample
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1200, 800),
		chromedp.Navigate(baseURL+"/note.html"),
		chromedp.WaitVisible(".fig-note", chromedp.ByQuery),
		figSetTheme("light"),
		chromedp.Evaluate(probe, &light),
		figSetTheme("dark"),
		chromedp.Evaluate(probe, &dark),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	for _, c := range []struct {
		theme string
		got   sample
	}{{"light", light}, {"dark", dark}} {
		if c.got.NoteSize >= c.got.BoxSize {
			t.Errorf("%s: a note should be smaller than its label, got %v vs %v",
				c.theme, c.got.NoteSize, c.got.BoxSize)
		}
		if c.got.NoteColor != c.got.Muted {
			t.Errorf("%s: a note should paint in --muted (%s), got %s",
				c.theme, c.got.Muted, c.got.NoteColor)
		}
		if c.got.NoteColor == c.got.BoxColor {
			t.Errorf("%s: a note should not share its label's colour (%s)", c.theme, c.got.BoxColor)
		}
	}
	// If the palette did not actually change, both themes could pass above
	// while the dark case went untested.
	if light.Muted == dark.Muted {
		t.Errorf("--muted did not change between themes (%s); the theme switch is not taking effect", light.Muted)
	}
}

func TestBrowserFigAccentRingNotClipped(t *testing.T) {
	page, err := md2html.Convert([]byte(
		"```fig\nitems:\n  - box: Accented\n    accent: true\n  - box: Neighbour\n```\n"),
		md2html.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"ring.html": page})
	ctx := newBrowserCtx(t)

	// The ring is drawn OUTSIDE the box's border box, so it is the one fig
	// decoration an ancestor's overflow can silently erase and a sibling can
	// silently sit on top of. Both are checked here because neither is
	// visible in the markup.
	const probe = figResolveVar + `
	(() => {
		const boxes = [...document.querySelectorAll(".fig-box")];
		const accented = boxes[0], neighbour = boxes[1];
		let clipper = null;
		for (let el = accented.parentElement; el && el !== document.documentElement; el = el.parentElement) {
			const s = getComputedStyle(el);
			if (s.overflowX !== "visible" || s.overflowY !== "visible") {
				clipper = (el.tagName + "." + (el.className || "")).trim();
				break;
			}
		}
		return {
			shadow:   getComputedStyle(accented).boxShadow,
			plain:    getComputedStyle(neighbour).boxShadow,
			clipper:  clipper || "",
			gap:      neighbour.getBoundingClientRect().top - accented.getBoundingClientRect().bottom,
			bg:       resolveVar("--bg"),
			accent:   resolveVar("--accent"),
		};
	})()`

	type sample struct {
		Shadow, Plain, Clipper string
		Gap                    float64
		Bg, Accent             string
	}
	var light, dark sample
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1200, 800),
		chromedp.Navigate(baseURL+"/ring.html"),
		chromedp.WaitVisible(".fig-accent", chromedp.ByQuery),
		figSetTheme("light"),
		chromedp.Evaluate(probe, &light),
		figSetTheme("dark"),
		chromedp.Evaluate(probe, &dark),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	// Geometry does not depend on the palette, so it is asserted once.
	if light.Clipper != "" {
		t.Errorf("an ancestor clips the accent ring: %s", light.Clipper)
	}
	// The ring extends 3px beyond the border box; a neighbour closer than
	// that overlaps it.
	const ringOuter = 3.0
	if light.Gap < ringOuter {
		t.Errorf("the ring needs %vpx of clearance, the next box is %vpx away", ringOuter, light.Gap)
	}
	if light.Plain != "none" {
		t.Errorf("an unaccented box should carry no ring, got %q", light.Plain)
	}

	for _, c := range []struct {
		theme string
		got   sample
	}{{"light", light}, {"dark", dark}} {
		if c.got.Shadow == "none" || c.got.Shadow == "" {
			t.Fatalf("%s: the accented box has no ring at all", c.theme)
		}
		for _, want := range []struct{ name, val string }{{"--bg", c.got.Bg}, {"--accent", c.got.Accent}} {
			if !strings.Contains(c.got.Shadow, want.val) {
				t.Errorf("%s: the ring should be drawn from %s (%s), got %q",
					c.theme, want.name, want.val, c.got.Shadow)
			}
		}
	}
	if light.Accent == dark.Accent {
		t.Errorf("--accent did not change between themes (%s)", light.Accent)
	}
}

// figColorMath parses the two serializations getComputedStyle uses for an
// sRGB colour — rgb()/rgba() for a token, color(srgb ...) for a color-mix()
// result — and computes WCAG contrast from them. The card tests assert
// floors rather than colours: a palette change that keeps a panel readable
// passes, and one that does not fails.
const figColorMath = `
	function channels(s) {
		let m = s.match(/^rgba?\(([^)]*)\)$/);
		if (m) return m[1].split(/[\s,\/]+/).filter(Boolean).slice(0, 3).map(v => parseFloat(v) / 255);
		m = s.match(/^color\(srgb ([^)]*)\)$/);
		if (m) return m[1].split(/[\s\/]+/).filter(Boolean).slice(0, 3).map(parseFloat);
		throw new Error("unparsed colour: " + s);
	}
	function luminance(s) {
		const lin = c => c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
		const [r, g, b] = channels(s).map(lin);
		return 0.2126 * r + 0.7152 * g + 0.0722 * b;
	}
	function contrast(a, b) {
		const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
		return (hi + 0.05) / (lo + 0.05);
	}
	function channelDelta(a, b) {
		const x = channels(a), y = channels(b);
		return Math.max(...x.map((v, i) => Math.abs(v - y[i])));
	}`

// Panel position draws a card around whatever item sits in it, an accented
// item tints that card, and an arrow panel draws none. The tint is the one
// fig surface that is not a plain token, so it is also the one place
// contrast is measured instead of assumed: the muted title on it must stay
// readable, a box's fill must stay distinguishable from it, and an accent
// ring inside it must draw its gap in the tint rather than in the page.
func TestBrowserFigPanelCard(t *testing.T) {
	page, err := md2html.Convert([]byte(
		"```fig\nlayout: cols\nitems:\n"+
			"  - group: Plain\n    items:\n      - box: Inside\n"+
			"  - arrow: \"\"\n"+
			"  - group: Marked\n    accent: true\n    items:\n      - box: Ringed\n        accent: true\n```\n"),
		md2html.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"card.html": page})
	ctx := newBrowserCtx(t)

	const probe = figResolveVar + figColorMath + `
	(() => {
		const [plain, arrow, marked] = [...document.querySelectorAll(".fig-panel")];
		const cs = e => getComputedStyle(e);
		const title = marked.querySelector(".fig-group-title");
		const ringed = marked.querySelector(".fig-box");
		const shadow = cs(ringed).boxShadow;
		const gap = shadow.match(/^(rgba?\([^)]*\)|color\([^)]*\))/);
		return {
			plainStyle:    cs(plain).borderTopStyle,
			plainBorder:   cs(plain).borderTopColor,
			plainBg:       cs(plain).backgroundColor,
			arrowStyle:    cs(arrow).borderTopStyle,
			markedBorder:  cs(marked).borderTopColor,
			markedBg:      cs(marked).backgroundColor,
			titleContrast: contrast(cs(title).color, cs(marked).backgroundColor),
			fillContrast:  contrast(cs(ringed).backgroundColor, cs(marked).backgroundColor),
			gapDelta:      gap ? channelDelta(gap[1], cs(marked).backgroundColor) : -1,
			shadow:        shadow,
			rule:          resolveVar("--rule"),
			accent:        resolveVar("--accent"),
			bg:            resolveVar("--bg"),
		};
	})()`

	type sample struct {
		PlainStyle, PlainBorder, PlainBg, ArrowStyle string
		MarkedBorder, MarkedBg                       string
		TitleContrast, FillContrast, GapDelta        float64
		Shadow, Rule, Accent, Bg                     string
	}
	var light, dark sample
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1200, 800),
		chromedp.Navigate(baseURL+"/card.html"),
		chromedp.WaitVisible(".fig-panel", chromedp.ByQuery),
		figSetTheme("light"),
		chromedp.Evaluate(probe, &light),
		figSetTheme("dark"),
		chromedp.Evaluate(probe, &dark),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	for _, c := range []struct {
		theme string
		got   sample
	}{{"light", light}, {"dark", dark}} {
		g := c.got
		if g.PlainStyle != "solid" {
			t.Errorf("%s: a panel should draw a solid card border, got %q", c.theme, g.PlainStyle)
		}
		if g.PlainBorder != g.Rule {
			t.Errorf("%s: a card border should paint in --rule (%s), got %s", c.theme, g.Rule, g.PlainBorder)
		}
		if g.PlainBg != g.Bg {
			t.Errorf("%s: a plain card should paint --bg (%s), got %s", c.theme, g.Bg, g.PlainBg)
		}
		if g.ArrowStyle != "none" {
			t.Errorf("%s: an arrow panel should draw no card, got border style %q", c.theme, g.ArrowStyle)
		}
		if g.MarkedBorder != g.Accent {
			t.Errorf("%s: an accented card border should paint in --accent (%s), got %s", c.theme, g.Accent, g.MarkedBorder)
		}
		if g.MarkedBg == g.Bg {
			t.Errorf("%s: an accented panel should tint its card, got plain --bg (%s)", c.theme, g.Bg)
		}
		if g.TitleContrast < 4.5 {
			t.Errorf("%s: a muted title on the tint must keep 4.5:1 contrast, got %.2f", c.theme, g.TitleContrast)
		}
		if g.FillContrast < 1.05 {
			t.Errorf("%s: a box's fill must stay distinguishable from the tint (>= 1.05), got %.3f", c.theme, g.FillContrast)
		}
		switch {
		case g.GapDelta < 0:
			t.Errorf("%s: could not read the ring's gap colour from %q", c.theme, g.Shadow)
		case g.GapDelta > 0.01:
			t.Errorf("%s: the ring's gap should be drawn in the card's tint (%s), got %q", c.theme, g.MarkedBg, g.Shadow)
		}
	}
	if light.Accent == dark.Accent {
		t.Errorf("--accent did not change between themes (%s); the theme switch is not taking effect", light.Accent)
	}
}

// The tint is the only card rule behind @supports, because an engine without
// color-mix() would keep the unparseable custom property and paint no card
// background at all. No current Chrome lacks color-mix(), so the test stands
// in for one by deleting the @supports block from the live stylesheet: what
// is left must still draw an accented card, in --bg with an --accent border,
// with the ring's gap falling back to --bg along with it.
func TestBrowserFigPanelCardWithoutColorMix(t *testing.T) {
	page, err := md2html.Convert([]byte(
		"```fig\nlayout: cols\nitems:\n"+
			"  - group: Plain\n    items:\n      - box: Inside\n"+
			"  - group: Marked\n    accent: true\n    items:\n      - box: Ringed\n        accent: true\n```\n"),
		md2html.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"fallback.html": page})
	ctx := newBrowserCtx(t)

	// Deletes every @supports rule whose condition names color-mix(), and
	// reports how many it found, so a stylesheet that stops gating the tint
	// fails here rather than passing vacuously.
	const dropSupports = `(() => {
		let dropped = 0;
		for (const sheet of document.styleSheets) {
			for (let i = sheet.cssRules.length - 1; i >= 0; i--) {
				const r = sheet.cssRules[i];
				if (r instanceof CSSSupportsRule && r.conditionText.includes("color-mix")) {
					sheet.deleteRule(i);
					dropped++;
				}
			}
		}
		return dropped;
	})()`

	const probe = figResolveVar + figColorMath + `
	(() => {
		const marked = document.querySelectorAll(".fig-panel")[1];
		const cs = e => getComputedStyle(e);
		const shadow = cs(marked.querySelector(".fig-box")).boxShadow;
		const gap = shadow.match(/^(rgba?\([^)]*\)|color\([^)]*\))/);
		const bg = resolveVar("--bg");
		return {
			markedStyle:  cs(marked).borderTopStyle,
			markedBorder: cs(marked).borderTopColor,
			markedBg:     cs(marked).backgroundColor,
			gapDelta:     gap ? channelDelta(gap[1], bg) : -1,
			shadow:       shadow,
			accent:       resolveVar("--accent"),
			bg:           bg,
		};
	})()`

	type sample struct {
		MarkedStyle, MarkedBorder, MarkedBg string
		GapDelta                            float64
		Shadow, Accent, Bg                  string
	}
	var dropped int
	var light, dark sample
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1200, 800),
		chromedp.Navigate(baseURL+"/fallback.html"),
		chromedp.WaitVisible(".fig-panel", chromedp.ByQuery),
		chromedp.Evaluate(dropSupports, &dropped),
		figSetTheme("light"),
		chromedp.Evaluate(probe, &light),
		figSetTheme("dark"),
		chromedp.Evaluate(probe, &dark),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	if dropped != 1 {
		t.Fatalf("want exactly one @supports block gating color-mix(), found %d", dropped)
	}
	for _, c := range []struct {
		theme string
		got   sample
	}{{"light", light}, {"dark", dark}} {
		g := c.got
		if g.MarkedStyle != "solid" {
			t.Errorf("%s: without color-mix() an accented panel should still draw its card, got border style %q", c.theme, g.MarkedStyle)
		}
		if g.MarkedBorder != g.Accent {
			t.Errorf("%s: without color-mix() the card border should still paint in --accent (%s), got %s", c.theme, g.Accent, g.MarkedBorder)
		}
		if g.MarkedBg != g.Bg {
			t.Errorf("%s: without color-mix() the card should paint --bg (%s), got %s", c.theme, g.Bg, g.MarkedBg)
		}
		switch {
		case g.GapDelta < 0:
			t.Errorf("%s: could not read the ring's gap colour from %q", c.theme, g.Shadow)
		case g.GapDelta > 0.01:
			t.Errorf("%s: without color-mix() the ring's gap should fall back to --bg (%s), got %q", c.theme, g.Bg, g.Shadow)
		}
	}
	if light.Accent == dark.Accent {
		t.Errorf("--accent did not change between themes (%s); the theme switch is not taking effect", light.Accent)
	}
}

// A connector between panels draws no card, so its position in the row is
// the only thing that says where it points from. It must sit at the row's
// vertical middle when the panels share a row, and at the column's
// horizontal middle, pointing down, once they stack.
func TestBrowserFigArrowPanelCentresInItsRow(t *testing.T) {
	page, err := md2html.Convert([]byte(
		"```fig\nlayout: cols\nitems:\n"+
			"  - group: Tall\n    items:\n      - box: One\n      - box: Two\n      - box: Three\n"+
			"  - arrow: \"\"\n"+
			"  - group: Short\n    items:\n      - box: Four\n```\n"),
		md2html.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"arrow.html": page})
	ctx := newBrowserCtx(t)

	const probe = `(() => {
		const row = document.querySelector(".fig-cols").getBoundingClientRect();
		const el = document.querySelector(".fig-panel-arrow > .fig-arrow");
		const a = el.getBoundingClientRect();
		return {
			rowMidY:   row.top + row.height / 2,
			rowMidX:   row.left + row.width / 2,
			rowHeight: row.height,
			arrowMidY: a.top + a.height / 2,
			arrowMidX: a.left + a.width / 2,
			arrowHeight: a.height,
			glyph:     getComputedStyle(el, "::before").content,
		};
	})()`

	type sample struct {
		RowMidY, RowMidX, RowHeight       float64
		ArrowMidY, ArrowMidX, ArrowHeight float64
		Glyph                             string
	}
	var wide, narrow sample
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1200, 800),
		chromedp.Navigate(baseURL+"/arrow.html"),
		chromedp.WaitVisible(".fig-panel-arrow", chromedp.ByQuery),
		chromedp.Evaluate(probe, &wide),
		chromedp.EmulateViewport(390, 800),
		chromedp.Evaluate(probe, &narrow),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	// Without a row much taller than the glyph, top-aligned and centred
	// would be indistinguishable and the check below would pass either way.
	if wide.RowHeight < 3*wide.ArrowHeight {
		t.Fatalf("the fixture row (%vpx) should be much taller than the arrow (%vpx)", wide.RowHeight, wide.ArrowHeight)
	}
	if math.Abs(wide.ArrowMidY-wide.RowMidY) > 1 {
		t.Errorf("a panel-level arrow should sit at the row's vertical middle (%v), got %v", wide.RowMidY, wide.ArrowMidY)
	}
	if wide.Glyph != `"→"` {
		t.Errorf("a panel-level arrow should point right when wide, got %s", wide.Glyph)
	}
	if math.Abs(narrow.ArrowMidX-narrow.RowMidX) > 1 {
		t.Errorf("a stacked arrow should sit at the column's horizontal middle (%v), got %v", narrow.RowMidX, narrow.ArrowMidX)
	}
	if narrow.Glyph != `"↓"` {
		t.Errorf("a stacked arrow should point down, got %s", narrow.Glyph)
	}
}

// The gallery is every figure shape in one document, so it is also the
// widest check that nothing breaks at either end of the width range: no
// figure widens the page, no part of a figure spills out of the figure or
// out of the panel card around it, and on a phone every side-by-side
// container has actually stacked. The per-shape tests above pin why; this
// one catches the shape nobody wrote a test for.
func TestBrowserFigGalleryFits(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "testdata", "figures.md"))
	if err != nil {
		t.Fatalf("read gallery: %v", err)
	}
	page, err := md2html.Convert(src, md2html.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"figures.html": page})
	ctx := newBrowserCtx(t)

	const probe = `(() => {
		const vw = document.documentElement.clientWidth;
		const out = {
			viewport: vw,
			scrollWidth: document.documentElement.scrollWidth,
			figures: document.querySelectorAll("figure.fig").length,
			spills: [],
			unstacked: [],
		};
		const name = e => e.tagName.toLowerCase() + "." + [...e.classList].join(".") +
			" (" + e.textContent.trim().slice(0, 30) + ")";
		document.querySelectorAll("figure.fig").forEach(f => {
			const fr = f.getBoundingClientRect();
			if (fr.left < -0.5 || fr.right > vw + 0.5) out.spills.push("figure " + name(f) + " leaves the viewport");
		});
		// A card draws its border where its box ends, so content past that
		// edge is visibly outside the card even when the figure still holds it.
		document.querySelectorAll("figure.fig, .fig-panel").forEach(c => {
			const cr = c.getBoundingClientRect();
			c.querySelectorAll("*").forEach(e => {
				const r = e.getBoundingClientRect();
				if (r.width === 0) return;
				if (r.left < cr.left - 0.5 || r.right > cr.right + 0.5) out.spills.push(name(e) + " spills out of " + name(c));
			});
		});
		// A box can fit its card while its own text runs past its border, and
		// text has no element box for the check above to measure.
		document.querySelectorAll("figure.fig *").forEach(e => {
			if (e.clientWidth > 0 && e.scrollWidth > e.clientWidth + 1) {
				out.spills.push(name(e) + " has content " + (e.scrollWidth - e.clientWidth) + "px wider than itself");
			}
		});
		document.querySelectorAll(".fig-cols, .fig-split, .fig-chain, .fig-lanes").forEach(c => {
			const kids = [...c.children];
			for (let i = 1; i < kids.length; i++) {
				if (kids[i].getBoundingClientRect().top < kids[i - 1].getBoundingClientRect().bottom - 0.5) {
					out.unstacked.push(name(kids[i]) + " sits beside its sibling in " + name(c));
				}
			}
		});
		return out;
	})()`

	type sample struct {
		Viewport, ScrollWidth, Figures float64
		Spills, Unstacked              []string
	}
	var desktop, phone sample
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1200, 800),
		chromedp.Navigate(baseURL+"/figures.html"),
		chromedp.WaitVisible("figure.fig", chromedp.ByQuery),
		chromedp.Evaluate(probe, &desktop),
		chromedp.EmulateViewport(390, 800),
		chromedp.Evaluate(probe, &phone),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	for _, c := range []struct {
		width string
		got   sample
	}{{"desktop", desktop}, {"phone", phone}} {
		if c.got.Figures == 0 {
			t.Fatalf("%s: the gallery rendered no figures", c.width)
		}
		if c.got.ScrollWidth > c.got.Viewport {
			t.Errorf("%s: the gallery scrolls sideways: %vpx of content in a %vpx viewport", c.width, c.got.ScrollWidth, c.got.Viewport)
		}
		for _, s := range c.got.Spills {
			t.Errorf("%s: %s", c.width, s)
		}
	}
	// Side by side is the point on a desktop, so stacking is a phone check.
	for _, s := range phone.Unstacked {
		t.Errorf("phone: %s", s)
	}
}

func TestBrowserFigTreeIndentationAligns(t *testing.T) {
	page, err := md2html.Convert([]byte(
		"```fig\nitems:\n  - tree: |\n      one -- first root\n"+
			"        child-a -- nested\n        child-b -- also nested\n      two -- second root\n```\n"),
		md2html.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"tree.html": page})
	ctx := newBrowserCtx(t)

	// A tree's whole purpose is that the eye can read depth off the left
	// edge. That is a rendered fact: the markup says "nested" but only the
	// layout says "indented", and a lost padding-left would flatten the tree
	// while every markup assertion kept passing.
	const probe = `
	(() => {
		const li = [...document.querySelectorAll(".fig-tree li")];
		return li.map(e => {
			const label = e.querySelector(":scope > .fig-tree-label");
			const note = e.querySelector(":scope > .fig-note");
			const lr = label.getBoundingClientRect();
			return {
				text: label.textContent,
				left: lr.left,
				labelBottom: lr.bottom,
				noteTop: note ? note.getBoundingClientRect().top : lr.top,
			};
		});
	})()`

	type node struct {
		Text                       string
		Left, LabelBottom, NoteTop float64
	}
	var nodes []node
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1200, 800),
		chromedp.Navigate(baseURL+"/tree.html"),
		chromedp.WaitVisible(".fig-tree", chromedp.ByQuery),
		chromedp.Evaluate(probe, &nodes),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	byText := map[string]node{}
	for _, n := range nodes {
		byText[n.Text] = n
	}
	for _, want := range []string{"one", "two", "child-a", "child-b"} {
		if _, ok := byText[want]; !ok {
			t.Fatalf("tree node %q did not render; got %v", want, nodes)
		}
	}
	const epsilon = 0.5
	if math.Abs(byText["one"].Left-byText["two"].Left) > epsilon {
		t.Errorf("roots should share a left edge, got %v and %v",
			byText["one"].Left, byText["two"].Left)
	}
	if math.Abs(byText["child-a"].Left-byText["child-b"].Left) > epsilon {
		t.Errorf("siblings should share a left edge, got %v and %v",
			byText["child-a"].Left, byText["child-b"].Left)
	}
	if byText["child-a"].Left <= byText["one"].Left {
		t.Errorf("a child should be indented past its parent, got %v vs %v",
			byText["child-a"].Left, byText["one"].Left)
	}
	// A tree note sits on its label's line; if it wrapped to its own line the
	// column of names the indentation exists to align would be broken up.
	//
	// The test is vertical overlap, not equal top edges: a note is set smaller
	// than its label and the two align on their shared baseline, so on one
	// line the note's top edge sits a pixel or so BELOW the label's. Only a
	// wrap pushes it past the label's bottom.
	for _, n := range nodes {
		if n.NoteTop >= n.LabelBottom {
			t.Errorf("%q: its note wrapped off the label's line (note top %v, label bottom %v)",
				n.Text, n.NoteTop, n.LabelBottom)
		}
	}
}

func TestBrowserFigTreeEmphasisFollowsTheme(t *testing.T) {
	page, err := md2html.Convert([]byte(
		"```fig\nitems:\n  - tree: |\n      plain\n      * accented\n      - muted\n```\n"),
		md2html.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"emph.html": page})
	ctx := newBrowserCtx(t)

	const probe = figResolveVar + `
	(() => {
		const li = [...document.querySelectorAll(".fig-tree > li")];
		const colorOf = e => getComputedStyle(e).color;
		return {
			plain:    colorOf(li[0]),
			accented: colorOf(li[1]),
			muted:    colorOf(li[2]),
			accentVar: resolveVar("--accent"),
			mutedVar:  resolveVar("--muted"),
		};
	})()`

	type sample struct {
		Plain, Accented, Muted, AccentVar, MutedVar string
	}
	var light, dark sample
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1200, 800),
		chromedp.Navigate(baseURL+"/emph.html"),
		chromedp.WaitVisible(".fig-tree", chromedp.ByQuery),
		figSetTheme("light"),
		chromedp.Evaluate(probe, &light),
		figSetTheme("dark"),
		chromedp.Evaluate(probe, &dark),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	for _, c := range []struct {
		theme string
		got   sample
	}{{"light", light}, {"dark", dark}} {
		if c.got.Accented != c.got.AccentVar {
			t.Errorf("%s: an accented tree line should paint in --accent (%s), got %s",
				c.theme, c.got.AccentVar, c.got.Accented)
		}
		if c.got.Muted != c.got.MutedVar {
			t.Errorf("%s: a muted tree line should paint in --muted (%s), got %s",
				c.theme, c.got.MutedVar, c.got.Muted)
		}
		if c.got.Accented == c.got.Plain || c.got.Muted == c.got.Plain {
			t.Errorf("%s: emphasis should distinguish a line from a plain one (plain %s, accented %s, muted %s)",
				c.theme, c.got.Plain, c.got.Accented, c.got.Muted)
		}
	}
	if light.AccentVar == dark.AccentVar {
		t.Errorf("--accent did not change between themes (%s)", light.AccentVar)
	}
}

// tocFloatPage is a long page with a floating contents list and a wide
// figure, the one element that already breaks out of the text column.
func tocFloatPage(t *testing.T) string {
	t.Helper()
	var src strings.Builder
	src.WriteString("# Doc\n\n[TOC]\n\n```fig\nwide: true\nitems:\n  - box: Wide\n```\n\n")
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&src, "## Section %d\n\n%s\n\n", i, strings.Repeat("Body text runs on. ", 60))
	}
	page, err := md2html.Convert([]byte(src.String()), md2html.Options{TOC: "float"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return serveGenerated(t, map[string][]byte{"toc.html": page})
}

// tocGeometry reports the list's position mode, whether its side toggle is
// shown, and its box beside the text column's and the wide figure's.
const tocGeometry = `(() => {
	const nav = document.querySelector("nav.toc");
	const btn = nav.querySelector(".toc-side-toggle");
	const r = (el) => el.getBoundingClientRect();
	const n = r(nav), m = r(document.querySelector("main")), w = r(document.querySelector("figure.fig-wide"));
	return JSON.stringify({
		position: getComputedStyle(nav).position,
		toggle: btn !== null && getComputedStyle(btn).display !== "none",
		nav: [n.left, n.right, n.top], main: [m.left, m.right], wide: [w.left, w.right],
		viewport: document.documentElement.clientWidth,
	});
})()`

type tocBoxes struct {
	Position string
	Toggle   bool
	Nav      [3]float64
	Main     [2]float64
	Wide     [2]float64
	Viewport float64
}

func readTOC(t *testing.T, raw string) tocBoxes {
	t.Helper()
	var g tocBoxes
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		t.Fatalf("parsing %q: %v", raw, err)
	}
	return g
}

// sideOf says which side of the text column the list is on, failing the
// test if it overlaps the column or the wide figure, or leaves the screen.
func sideOf(t *testing.T, label string, g tocBoxes) string {
	t.Helper()
	l, r := g.Nav[0], g.Nav[1]
	if l < 0 || r > g.Viewport {
		t.Errorf("%s: floating list leaves the viewport: [%v, %v] in %v", label, l, r, g.Viewport)
	}
	switch {
	case l >= g.Main[1] && l >= g.Wide[1]:
		return "right"
	case r <= g.Main[0] && r <= g.Wide[0]:
		return "left"
	}
	t.Errorf("%s: floating list [%v, %v] overlaps the column %v or the wide figure %v", label, l, r, g.Main, g.Wide)
	return ""
}

// At every width the list either floats clear of the text and the wide
// figure, or has gone back inline without its side control: never a panel
// laid over the page's own content.
func TestBrowserTOCFloatNeverCoversTheText(t *testing.T) {
	baseURL := tocFloatPage(t)
	ctx := newBrowserCtx(t)
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1440, 900),
		chromedp.Navigate(baseURL+"/toc.html"),
		chromedp.WaitVisible("nav.toc", chromedp.ByQuery),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}
	floated := false
	for _, w := range []int64{390, 800, 1024, 1100, 1200, 1280, 1440, 1920} {
		var raw string
		if err := chromedp.Run(ctx, chromedp.EmulateViewport(w, 900), chromedp.Evaluate(tocGeometry, &raw)); err != nil {
			t.Fatalf("browser run at %d: %v", w, err)
		}
		g := readTOC(t, raw)
		label := fmt.Sprintf("width %d", w)
		switch g.Position {
		case "fixed":
			floated = true
			if !g.Toggle {
				t.Errorf("%s: floating list has no side toggle", label)
			}
			if side := sideOf(t, label, g); side != "" && side != "right" {
				t.Errorf("%s: floating list starts on the %s, want right", label, side)
			}
		case "static":
			if g.Toggle {
				t.Errorf("%s: inline list shows a side toggle it cannot use", label)
			}
		default:
			t.Errorf("%s: position = %q, want fixed or static", label, g.Position)
		}
	}
	if !floated {
		t.Error("the list never floated, even at 1920px")
	}
	var raw390 string
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(390, 900), chromedp.Evaluate(tocGeometry, &raw390)); err != nil {
		t.Fatalf("browser run: %v", err)
	}
	if g := readTOC(t, raw390); g.Position != "static" {
		t.Errorf("a phone-width screen must keep the list inline, got position %q", g.Position)
	}
}

// On a wide screen the list stays in view while the page scrolls, its
// toggle moves it to the other side, and the choice outlives a reload.
func TestBrowserTOCFloatSideToggle(t *testing.T) {
	baseURL := tocFloatPage(t)
	ctx := newBrowserCtx(t)
	geometry := func(label string) tocBoxes {
		t.Helper()
		var raw string
		if err := chromedp.Run(ctx, chromedp.Evaluate(tocGeometry, &raw)); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		return readTOC(t, raw)
	}
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1440, 900),
		chromedp.Navigate(baseURL+"/toc.html"),
		chromedp.WaitVisible(".toc-side-toggle", chromedp.ByQuery),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}
	start := geometry("start")
	if start.Position != "fixed" || sideOf(t, "start", start) != "right" {
		t.Fatalf("want a fixed list on the right at 1440px, got %+v", start)
	}

	var ignored any
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, &ignored)); err != nil {
		t.Fatal(err)
	}
	if scrolled := geometry("scrolled"); scrolled.Nav[2] != start.Nav[2] {
		t.Errorf("the list moved while scrolling: top %v, then %v", start.Nav[2], scrolled.Nav[2])
	}

	if err := chromedp.Run(ctx, chromedp.Click(".toc-side-toggle", chromedp.ByQuery, chromedp.NodeVisible)); err != nil {
		t.Fatal(err)
	}
	if side := sideOf(t, "after toggle", geometry("after toggle")); side != "left" {
		t.Errorf("toggle left the list on the %s, want left", side)
	}

	if err := chromedp.Run(ctx,
		chromedp.Reload(),
		chromedp.WaitVisible(".toc-side-toggle", chromedp.ByQuery),
	); err != nil {
		t.Fatal(err)
	}
	if side := sideOf(t, "after reload", geometry("after reload")); side != "left" {
		t.Errorf("reload forgot the chosen side: list on the %s, want left", side)
	}

	if err := chromedp.Run(ctx, chromedp.Click(".toc-side-toggle", chromedp.ByQuery, chromedp.NodeVisible)); err != nil {
		t.Fatal(err)
	}
	if side := sideOf(t, "toggled back", geometry("toggled back")); side != "right" {
		t.Errorf("second toggle left the list on the %s, want right", side)
	}
}

// Outline depth is shown by chevrons, not indentation: every entry starts
// at the same left edge, the title and top-level sections are bare, an
// entry below them leads with one chevron per level (up to three), depth
// follows the outline rather than the tag, so a skipped heading level adds
// none, and a line a long heading wraps onto
// hangs further in than any entry's start, so it never reads as the next
// entry.
func TestBrowserTOCLevelsAreChevronsAndWrapsHang(t *testing.T) {
	var src strings.Builder
	src.WriteString("# Doc\n\n[TOC]\n\n## Short\n\ntext\n\n")
	src.WriteString("## A section heading long enough that the floating list has to wrap it onto more lines\n\ntext\n\n")
	src.WriteString("### A subsection heading also long enough to wrap inside the floating list\n\ntext\n\n")
	src.WriteString("#### Deeper\n\ntext\n\n")
	src.WriteString("## Parent\n\ntext\n\n#### Skipped to\n\ntext\n")
	page, err := md2html.Convert([]byte(src.String()), md2html.Options{TOC: "float"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"levels.html": page})
	ctx := newBrowserCtx(t)

	// Per entry: where its first line starts (the content box plus the
	// first-line indent, which is where a ::before marker sits), where the
	// link's first and last lines start, how many lines it takes, and the
	// marker text.
	const entries = `JSON.stringify([...document.querySelectorAll("nav.toc li")].map((li) => {
		const cs = getComputedStyle(li);
		const start = li.getBoundingClientRect().left + parseFloat(cs.paddingLeft) + parseFloat(cs.textIndent);
		const rects = [...li.querySelector("a").getClientRects()];
		const marker = getComputedStyle(li, "::before").content;
		return {start, first: rects[0].left, last: rects[rects.length - 1].left, lines: rects.length, marker};
	}))`
	type entry struct {
		Start, First, Last float64
		Lines              int
		Marker             string
	}
	for _, w := range []int64{1440, 390} {
		var raw string
		if err := chromedp.Run(ctx,
			chromedp.EmulateViewport(w, 900),
			chromedp.Navigate(baseURL+"/levels.html"),
			chromedp.WaitVisible("nav.toc", chromedp.ByQuery),
			chromedp.Evaluate(entries, &raw),
		); err != nil {
			t.Fatalf("browser run at %d: %v", w, err)
		}
		var got []entry
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("parsing %q: %v", raw, err)
		}
		// Doc (h1), Short (h2), long h2, long h3, Deeper (h4), Parent (h2),
		// Skipped to (h4 directly under an h2).
		if len(got) != 7 {
			t.Fatalf("width %d: want 7 entries, got %+v", w, got)
		}
		if w == 1440 && (got[2].Lines < 2 || got[3].Lines < 2) {
			t.Fatalf("width %d: the long entries did not wrap, so the test proves nothing: %+v", w, got)
		}
		// The title and the top-level sections are bare; chevrons start
		// one level below them.
		chevrons := []int{0, 0, 0, 1, 2, 0, 1}
		for i, e := range got {
			if math.Abs(e.Start-got[0].Start) > 0.5 {
				t.Errorf("width %d: entry %d starts at %v, the first at %v; levels must not indent", w, i, e.Start, got[0].Start)
			}
			if n := strings.Count(e.Marker, "\u203a"); n != chevrons[i] {
				t.Errorf("width %d: entry %d marker %s has %d chevrons, want %d", w, i, e.Marker, n, chevrons[i])
			}
			if chevrons[i] == 0 && math.Abs(e.First-e.Start) > 0.5 {
				t.Errorf("width %d: depth-0 entry %d text starts at %v, not at the edge %v", w, i, e.First, e.Start)
			}
			if chevrons[i] > 0 && e.First <= e.Start+2 {
				t.Errorf("width %d: nested entry %d text at %v leaves no room for its chevron at %v", w, i, e.First, e.Start)
			}
			if e.Lines > 1 && e.Last < e.Start+8 {
				t.Errorf("width %d: entry %d wraps back to %v from an entry start of %v; a wrapped line must hang", w, i, e.Last, e.Start)
			}
		}
	}
}

// The embedded Markdown must come back from a real browser exactly as it
// was written, including what an HTML parser would otherwise change — a
// leading newline, a closing textarea tag, entities, CRLF line endings —
// and it must take up no room on the page. It is read through textContent:
// a textarea's value normalizes line endings to LF by specification, so
// value would lose the CRs whatever the page held.
func TestBrowserEmbeddedSourceRoundTrips(t *testing.T) {
	src := "\n---\ntitle: Kept\n---\n# Doc\r\n\r\nRaw <b>x</b> &amp; </textarea> stays.\r\n"
	page, err := md2html.Convert([]byte(src), md2html.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"index.html": page})

	sel := fmt.Sprintf("document.getElementById(%q)", md2html.SourceID)
	ctx := newBrowserCtx(t)
	var value string
	var boxes int
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Evaluate(sel+".textContent", &value),
		chromedp.Evaluate(sel+".getClientRects().length", &boxes),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if value != src {
		t.Errorf("source read back from the page differs\n got: %q\nwant: %q", value, src)
	}
	if boxes != 0 {
		t.Errorf("source element renders %d box(es), want none", boxes)
	}
}
