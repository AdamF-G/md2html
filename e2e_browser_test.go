//go:build e2e_browser

// Package-level note for whoever appends the next test (Tasks 4-8): every
// selector passed to chromedp (Click, WaitVisible, AttributeValue, etc.)
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
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
// this one dispatcher gap and nothing else (verified in the Task 3 fix-round
// report: a distinct chromedp-internal message still reaches log.Printf).
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
// both, so callers needing to know baseURL before every file exists (Task
// 8, which must set mermaidCDN to a URL under baseURL before calling
// Convert) can write files in after the server is already serving.
//
// The plain files served here include a vendored mermaid build (Task 8)
// that lazily imports per-diagram chunks at runtime: only the chunks one
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
