# Headless-Browser E2E Tests Implementation Plan

**Goal:** Verify the mermaid and image/SVG click-to-expand runtimes actually behave correctly in a real browser DOM (dialog opens, size gating skips small images, backdrop click closes it), not just that the generated HTML/JS text contains the right substrings.

**Architecture:** A build-tag-gated (`e2e_browser`) Go test file drives headless Chrome via chromedp against pages produced by the existing `Convert`/`renderPage` pipeline, served over real HTTP from a temp directory. The mermaid runtime's CDN import is made overridable so its tests run against a vendored copy of the pinned library instead of the live network.

**Tech Stack:** Go 1.27.1, `github.com/chromedp/chromedp` (test-only), stdlib `net/http/httptest`, `image`/`image/png`, `mime`.

**Spec:** `docs/specs/2026-09-11-e2e-browser-tests-design.md`

## Global Constraints

- All new browser-driving tests live behind `//go:build e2e_browser`. Plain `go test ./...` must never require a browser.
- `chromedp` is added via `go get github.com/chromedp/chromedp@latest`, confined to test files. It must not be imported by any non-test file.
- The vendored mermaid build's filename must literally embed the same version string as `mermaidCDN` in `page.go` (currently `11.17.2`).
- No fixed-duration sleeps in browser tests. Wait on real DOM state via chromedp's `WaitVisible`/`WaitNotPresent` actions, or rely on `chromedp.Navigate` blocking until the page's load event fires (window `load` always follows every image's own load, so synchronous-at-parse-time script logic and `<img>` `load`-listener fallbacks are both settled by the time `Navigate` returns).
- `gofmt` clean, `go vet ./...` clean (existing repo convention).

---

### Task 1: Vendor the pinned mermaid build

Vendors the exact library version `mermaidCDN` already points at, and adds a guard so a future version bump can't silently leave the vendored copy stale. No browser or new dependency needed for this task.

**Files:**
- Create: `testdata/vendor/mermaid-11.17.2.esm.min.mjs`
- Test: `page_test.go` (append)

**Interfaces:**
- Consumes: `mermaidCDN` (existing `const string` in `page.go`, pinned to `mermaid@11.17.2`).
- Produces: `testdata/vendor/mermaid-11.17.2.esm.min.mjs` on disk, which Task 8 serves locally.

- [ ] **Step 1: Write the failing test**

Append to `page_test.go`:

```go
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
```

Add `"os"` and `"path/filepath"` to `page_test.go`'s import block if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestVendoredMermaidVersionMatchesPinned -v`
Expected: FAIL — `read testdata/vendor: ... no such file or directory` (the directory doesn't exist yet).

- [ ] **Step 3: Vendor the file**

```bash
mkdir -p testdata/vendor
curl -sL https://cdn.jsdelivr.net/npm/mermaid@11.17.2/dist/mermaid.esm.min.mjs \
  -o testdata/vendor/mermaid-11.17.2.esm.min.mjs
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestVendoredMermaidVersionMatchesPinned -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add testdata/vendor/mermaid-11.17.2.esm.min.mjs page_test.go
git commit -m "test: vendor pinned mermaid build for offline e2e coverage"
```

---

### Task 2: Make `mermaidCDN` overridable

Converts `mermaidCDN` from a `const` to a `var`, and `mermaidRuntime` from a `const` string to a `func() string` that reads the current value — so a test can point it at a local server before calling `Convert`, with zero change to production behavior or output.

**Files:**
- Modify: `page.go`
- Test: `page_test.go` (append)

**Interfaces:**
- Consumes: nothing new.
- Produces: `var mermaidCDN string` (was `const`); `func mermaidRuntime() string` (was `const mermaidRuntime`). Task 8 reassigns `mermaidCDN` directly (same package, `t.Cleanup` restores it).

- [ ] **Step 1: Write the failing test**

Append to `page_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestMermaidCDNIsOverridable -v`
Expected: FAIL to compile — `cannot assign to mermaidCDN (neither addressable nor a map index expression)` or `cannot assign to mermaidCDN (declared const)`, since `mermaidCDN` is still a `const`. This compile error is the correct RED for a statically-typed change like this one.

- [ ] **Step 3: Change `mermaidCDN` to a var and `mermaidRuntime` to a func**

In `page.go`, change:

```go
const mermaidCDN = "https://cdn.jsdelivr.net/npm/mermaid@11.17.2/dist/mermaid.esm.min.mjs"
```

to:

```go
// mermaidCDN is a var, not a const, so e2e_browser_test.go can redirect it
// at a local, vendored copy of the same pinned version instead of the live
// CDN. Production code never reassigns it.
var mermaidCDN = "https://cdn.jsdelivr.net/npm/mermaid@11.17.2/dist/mermaid.esm.min.mjs"
```

Change:

```go
const mermaidRuntime = `<script type="module">
import mermaid from "` + mermaidCDN + `";
const explicit = document.documentElement.dataset.theme;
const dark = explicit === "dark" ||
  (explicit !== "light" && matchMedia("(prefers-color-scheme: dark)").matches);
mermaid.initialize({ startOnLoad: true, theme: dark ? "dark" : "default" });

const lightbox = document.createElement("dialog");
lightbox.className = "mermaid-lightbox";
document.body.appendChild(lightbox);
lightbox.addEventListener("click", (e) => {
  if (e.target === lightbox) lightbox.close();
});

document.querySelectorAll("pre.mermaid").forEach((diagram) => {
  diagram.addEventListener("click", () => {
    const original = diagram.querySelector("svg");
    if (!original) return;
    const svg = original.cloneNode(true);
    svg.removeAttribute("style");
    const vb = original.viewBox && original.viewBox.baseVal;
    if (vb && vb.width && vb.height) {
      svg.setAttribute("width", vb.width);
      svg.setAttribute("height", vb.height);
    } else {
      svg.removeAttribute("width");
      svg.removeAttribute("height");
    }
    lightbox.replaceChildren(svg);
    lightbox.showModal();
  });
});
</script>
`
```

to:

```go
// mermaidRuntime is a func, not a const, because it embeds mermaidCDN's
// current value — which e2e_browser_test.go reassigns before calling
// Convert. Production callers see identical output to before this change.
func mermaidRuntime() string {
	return `<script type="module">
import mermaid from "` + mermaidCDN + `";
const explicit = document.documentElement.dataset.theme;
const dark = explicit === "dark" ||
  (explicit !== "light" && matchMedia("(prefers-color-scheme: dark)").matches);
mermaid.initialize({ startOnLoad: true, theme: dark ? "dark" : "default" });

const lightbox = document.createElement("dialog");
lightbox.className = "mermaid-lightbox";
document.body.appendChild(lightbox);
lightbox.addEventListener("click", (e) => {
  if (e.target === lightbox) lightbox.close();
});

document.querySelectorAll("pre.mermaid").forEach((diagram) => {
  diagram.addEventListener("click", () => {
    const original = diagram.querySelector("svg");
    if (!original) return;
    const svg = original.cloneNode(true);
    svg.removeAttribute("style");
    const vb = original.viewBox && original.viewBox.baseVal;
    if (vb && vb.width && vb.height) {
      svg.setAttribute("width", vb.width);
      svg.setAttribute("height", vb.height);
    } else {
      svg.removeAttribute("width");
      svg.removeAttribute("height");
    }
    lightbox.replaceChildren(svg);
    lightbox.showModal();
  });
});
</script>
`
}
```

In `renderPage`, change the call site:

```go
	if hasMermaid(body) {
		b.WriteString(mermaidRuntime)
	}
```

to:

```go
	if hasMermaid(body) {
		b.WriteString(mermaidRuntime())
	}
```

- [ ] **Step 4: Run test to verify it passes, and that nothing else broke**

Run: `go test ./... -v`
Expected: PASS, including `TestMermaidCDNIsOverridable`, `TestMermaidRuntimeInjectedOnlyForPagesThatNeedIt`, `TestMermaidClickToExpand`, `TestMermaidRuntimeIsThemeAware`, and every other existing test.

- [ ] **Step 5: Commit**

```bash
git add page.go page_test.go
git commit -m "refactor: make mermaidCDN overridable for e2e tests"
```

---

### Task 3: Browser harness + large image expands on click

Adds the chromedp dependency and the shared test harness (browser context, HTTP-served temp directory, synthetic PNG fixtures), proven by the first real browser test: a large image becomes expandable and clicking it opens the dialog with a correctly-sized clone.

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `e2e_browser_test.go`

**Interfaces:**
- Consumes: `Convert`, `Options` (existing, from `md2html.go`).
- Produces (all in `e2e_browser_test.go`, package `md2html`, used by Tasks 4–8):
  - `func newBrowserCtx(t *testing.T) context.Context`
  - `func serveDir(t *testing.T) (dir, baseURL string)`
  - `func writeFiles(t *testing.T, dir string, files map[string][]byte)`
  - `func serveGenerated(t *testing.T, files map[string][]byte) (baseURL string)`
  - `func pngFixture(w, h int) []byte`

- [ ] **Step 1: Add the chromedp dependency**

```bash
go get github.com/chromedp/chromedp@latest
```

This updates `go.mod` and `go.sum`. `chromedp` is only ever imported from `e2e_browser_test.go`, a test file — it never reaches the shipped `md2html`/`cmd/md2html` binaries.

- [ ] **Step 2: Write the failing test**

Create `e2e_browser_test.go`:

```go
//go:build e2e_browser

package md2html

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
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
func newBrowserCtx(t *testing.T) context.Context {
	t.Helper()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("no-sandbox", true))
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)
	ctx, timeoutCancel := context.WithTimeout(ctx, 15*time.Second)
	t.Cleanup(func() {
		timeoutCancel()
		cancel()
		allocCancel()
	})
	return ctx
}

// serveDir starts an HTTP server over a fresh temp directory and returns
// both, so callers needing to know baseURL before every file exists (Task
// 8, which must set mermaidCDN to a URL under baseURL before calling
// Convert) can write files in after the server is already serving.
func serveDir(t *testing.T) (dir, baseURL string) {
	t.Helper()
	dir = t.TempDir()
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
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
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Evaluate(`document.querySelector("img").classList.contains("expandable")`, &expandable),
		chromedp.Click(`img`, chromedp.NodeVisible),
		chromedp.WaitVisible(`dialog.media-lightbox[open]`),
		chromedp.AttributeValue(`dialog.media-lightbox img`, "width", &width, &widthOK),
		chromedp.AttributeValue(`dialog.media-lightbox img`, "height", &height, &heightOK),
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
```

- [ ] **Step 3: Run it to confirm the harness compiles and the test is meaningful**

Run: `go test -tags e2e_browser ./... -run TestBrowserLargeImageExpandsOnClick -v`

The production behavior under test (`mediaExpandRuntime`'s `.expandable` gating and dialog) already shipped in an earlier change — this task adds verification tooling for it, not new behavior, so there's no red-then-green cycle for the assertions themselves. There is still a real correctness bar to clear here: the harness code (chromedp API calls, server wiring) is new and must actually compile and drive a real browser correctly.

If it fails to build, the most likely cause is a chromedp API mismatch — check the failing symbol against `go doc github.com/chromedp/chromedp <Name>` and fix. If it builds but fails at runtime, check the harness's server/navigation wiring before suspecting the production code, since the production code is already covered by `page_test.go`'s string-level tests.
Expected: PASS.

- [ ] **Step 4: Confirm the test can actually fail**

Temporarily change `size.width <= box.width` to `size.width >= box.width` in `mediaExpandRuntime` in `page.go` (inverting the gate), re-run the same command, and confirm it now FAILS. This proves the test exercises real browser behavior rather than passing vacuously. Revert the change.

Run: `go test -tags e2e_browser ./... -run TestBrowserLargeImageExpandsOnClick -v`
Expected: PASS again, after reverting.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum e2e_browser_test.go
git commit -m "test: add headless-browser harness and first click-to-expand e2e test"
```

---

### Task 4: Small image is not expandable

**Files:**
- Modify: `e2e_browser_test.go` (append)

**Interfaces:**
- Consumes: `serveGenerated`, `newBrowserCtx`, `pngFixture` (Task 3).
- Produces: nothing new for later tasks.

- [ ] **Step 1: Write the failing test**

Append to `e2e_browser_test.go`:

```go
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
	var expandable bool
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Evaluate(`document.querySelector("img").classList.contains("expandable")`, &expandable),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if expandable {
		t.Error("a 10x10 image got .expandable; it's already at its own size")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags e2e_browser ./... -run TestBrowserSmallImageNotExpandable -v`
Expected: PASS immediately — the production code already implements this gate (see `default.css` / `page.go`'s `mediaExpandRuntime`). A pre-existing-behavior test that passes on first run without any code change is exactly the "test passes immediately" red flag the TDD skill warns about **when you're about to write new production code**. Here there's none to write: this test's purpose is to add regression coverage for behavior that already shipped, which is a legitimate reason to see green immediately. Confirm by reading `mediaExpandRuntime` in `page.go` and checking the size-comparison gate is indeed what makes it pass, rather than a vacuous selector match.

- [ ] **Step 3: Commit**

```bash
git add e2e_browser_test.go
git commit -m "test: cover small images are never marked expandable"
```

---

### Task 5: Backdrop click closes the dialog

**Files:**
- Modify: `e2e_browser_test.go` (append)

**Interfaces:**
- Consumes: `serveGenerated`, `newBrowserCtx`, `pngFixture` (Task 3).
- Produces: nothing new for later tasks.

- [ ] **Step 1: Write the failing test**

Append to `e2e_browser_test.go`:

```go
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
		chromedp.Click(`img`, chromedp.NodeVisible),
		chromedp.WaitVisible(`dialog.media-lightbox[open]`),
		chromedp.MouseClickXY(2, 2),
		chromedp.WaitNotPresent(`dialog.media-lightbox[open]`),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags e2e_browser ./... -run TestBrowserBackdropClickCloses -v`
Expected: PASS — the click handler's `e.target === mediaLightbox` check already implements this. As in Task 4, this is regression coverage for shipped behavior; confirm the test would actually fail if the check were removed by temporarily commenting out the `if (e.target === mediaLightbox) mediaLightbox.close();` line in `mediaExpandRuntime` in `page.go`, re-running, seeing it fail (dialog still open, `WaitNotPresent` times out), then restoring the line.

- [ ] **Step 3: Restore and confirm green**

Run: `go test -tags e2e_browser ./... -run TestBrowserBackdropClickCloses -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add e2e_browser_test.go
git commit -m "test: cover backdrop click closes the media lightbox"
```

---

### Task 6: Linked image is never wired

**Files:**
- Modify: `e2e_browser_test.go` (append)

**Interfaces:**
- Consumes: `serveGenerated`, `newBrowserCtx`, `pngFixture` (Task 3).
- Produces: nothing new for later tasks.

- [ ] **Step 1: Write the failing test**

Append to `e2e_browser_test.go`:

```go
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
	var expandable bool
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Evaluate(`document.querySelector("img").classList.contains("expandable")`, &expandable),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if expandable {
		t.Error("an image wrapped in <a> got .expandable; it should be left to the link")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags e2e_browser ./... -run TestBrowserLinkedImageNotWired -v`
Expected: PASS — the `el.closest("a")` exclusion already implements this. Confirm it's a meaningful assertion (not vacuous) by temporarily changing `el.closest("a")` to `el.closest("nonexistent")` in `mediaExpandRuntime` in `page.go`, re-running to see it fail, then restoring.

- [ ] **Step 3: Restore and confirm green**

Run: `go test -tags e2e_browser ./... -run TestBrowserLinkedImageNotWired -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add e2e_browser_test.go
git commit -m "test: cover linked images are never wired for expand"
```

---

### Task 7: SVG inside a mermaid `<pre>` is excluded

**Files:**
- Modify: `e2e_browser_test.go` (append)

**Interfaces:**
- Consumes: `serveGenerated`, `newBrowserCtx` (Task 3).
- Produces: nothing new for later tasks.

- [ ] **Step 1: Write the failing test**

Append to `e2e_browser_test.go`:

```go
// mermaid's own <svg> lives inside pre.mermaid and is already wired up by
// mermaidRuntime; without this exclusion, media-expand's own click
// listener would double-wire it and a click would fight over which
// dialog opens. This fixture hand-authors the DOM shape directly, with no
// real mermaid render involved, to test the exclusion filter alone.
func TestBrowserSvgInsideMermaidPreExcluded(t *testing.T) {
	const src = `<pre class="mermaid"><svg viewBox="0 0 10 10"><circle cx="5" cy="5" r="4"/></svg></pre>
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
		chromedp.Evaluate(`document.querySelector("pre.mermaid svg").classList.contains("expandable")`, &expandable),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if expandable {
		t.Error("an svg inside pre.mermaid got .expandable; that's mermaidRuntime's element")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags e2e_browser ./... -run TestBrowserSvgInsideMermaidPreExcluded -v`
Expected: PASS — the `el.closest("pre.mermaid")` exclusion already implements this. Confirm it's meaningful by temporarily changing that selector to `el.closest("nonexistent")` in `page.go`, re-running to see it fail, then restoring.

- [ ] **Step 3: Restore and confirm green**

Run: `go test -tags e2e_browser ./... -run TestBrowserSvgInsideMermaidPreExcluded -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add e2e_browser_test.go
git commit -m "test: cover mermaid's own svg is excluded from media-expand"
```

---

### Task 8: Real mermaid diagram expands on click

Exercises the mermaid runtime end-to-end against the vendored library — an actual `mermaid.initialize()` render, not a hand-authored fixture.

**Files:**
- Modify: `e2e_browser_test.go` (append)

**Interfaces:**
- Consumes: `serveDir`, `writeFiles`, `newBrowserCtx` (Task 3); `mermaidCDN` var, `testdata/vendor/mermaid-11.17.2.esm.min.mjs` (Tasks 1–2).
- Produces: nothing new for later tasks — this is the last task in the plan.

- [ ] **Step 1: Write the failing test**

Append to `e2e_browser_test.go`:

```go
// A real ```mermaid fence, rendered by the actual vendored library (not a
// hand-authored fixture), must become expandable and open
// dialog.mermaid-lightbox on click, with the clone carrying its real
// viewBox-derived size.
func TestBrowserMermaidDiagramExpandsOnClick(t *testing.T) {
	mime.AddExtensionType(".mjs", "text/javascript") // else Chrome refuses to run the module script

	dir, baseURL := serveDir(t)

	vendored, err := os.ReadFile(filepath.Join("testdata", "vendor", "mermaid-11.17.2.esm.min.mjs"))
	if err != nil {
		t.Fatalf("read vendored mermaid: %v", err)
	}
	writeFiles(t, dir, map[string][]byte{"vendor/mermaid.esm.min.mjs": vendored})

	original := mermaidCDN
	mermaidCDN = baseURL + "/vendor/mermaid.esm.min.mjs"
	t.Cleanup(func() { mermaidCDN = original })

	page, err := Convert([]byte("```mermaid\ngraph LR\n  A --> B\n```\n"), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	writeFiles(t, dir, map[string][]byte{"index.html": page})

	ctx := newBrowserCtx(t)
	var widthOK, heightOK bool
	var width, height string
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.WaitVisible(`pre.mermaid svg`),
		chromedp.Click(`pre.mermaid`, chromedp.NodeVisible),
		chromedp.WaitVisible(`dialog.mermaid-lightbox[open]`),
		chromedp.AttributeValue(`dialog.mermaid-lightbox svg`, "width", &width, &widthOK),
		chromedp.AttributeValue(`dialog.mermaid-lightbox svg`, "height", &height, &heightOK),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !widthOK || !heightOK {
		t.Fatal("cloned <svg> in the mermaid dialog has no width/height attribute")
	}
	if width == "" || width == "0" || height == "" || height == "0" {
		t.Errorf("cloned svg size = %sx%s, want real dimensions from the viewBox", width, height)
	}
}
```

Add `"mime"` to `e2e_browser_test.go`'s import block.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags e2e_browser ./... -run TestBrowserMermaidDiagramExpandsOnClick -v`
Expected: PASS — `mermaidRuntime`, the vendoring, and the injectable `mermaidCDN` were all built for exactly this in Tasks 1–3. If it fails, the likely causes are: `mime.AddExtensionType` missing (module script rejected — check the browser console via chromedp's `chromedp.Evaluate` returning a script error, or inspect via `--headless=false` locally), or `mermaidCDN` not actually reset before `Convert` runs (diagram never renders, `WaitVisible(pre.mermaid svg)` times out).

- [ ] **Step 3: Run full opt-in suite to confirm nothing regressed**

Run: `go test -tags e2e_browser ./... -v`
Expected: All PASS, including every test from Tasks 3–8.

- [ ] **Step 4: Run the default suite to confirm it's still browser-free**

Run: `go test ./...`
Expected: PASS, and completes without needing Chrome (the `e2e_browser` build tag excludes `e2e_browser_test.go` entirely from this run).

- [ ] **Step 5: Commit**

```bash
git add e2e_browser_test.go
git commit -m "test: cover a real mermaid diagram expands on click"
```
