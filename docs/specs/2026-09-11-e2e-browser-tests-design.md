# Headless-Browser E2E Tests — Design

**Date:** 2026-09-11
**Author:** AdamF-G
**Module:** `github.com/AdamF-G/md2html` (test-only addition)
**Status:** Shipped in v0.2.0. The suite now lives in `e2e/`, its own
Go module, run with `just e2e`.

## Purpose

`page_test.go` and `e2e_test.go` verify the mermaid and image/SVG
click-to-expand runtimes by asserting substrings of the generated HTML and
JavaScript text — they never execute that script. Whether the dialog
actually opens, whether the "only when scaled down" size comparison
actually skips small images, whether a backdrop click actually closes the
dialog: none of that is verified today. This spec adds a headless-Chrome
layer that runs the real script against a real DOM to close that gap, for
both click-to-expand runtimes.

## Scope

**In:** the image/SVG click-to-expand runtime (`mediaExpandRuntime`) and
the mermaid click-to-expand runtime (`mermaidRuntime`), both exercised in
an actual browser.

**Out:**
- Standing up CI. This repo has no CI today (no `.github/workflows`);
  wiring one up is a separate concern from building the framework.
- A test asserting the pinned mermaid version is "fairly recent." Any real
  check needs a live query against the npm/CDN registry — exactly the live
  network dependency this design vendors mermaid to avoid — or a hardcoded
  threshold that goes stale the moment it's written. `mermaidCDN` is
  already pinned deliberately for reproducibility ("a page generated today
  renders the same way next year"); recency isn't the goal. Addressed as a
  periodic manual/process concern, not a test.
- Cross-browser coverage. Chrome only.

## Architecture

New dependency: `github.com/chromedp/chromedp`, confined to a single
build-tag-gated test file — never imported by any file the shipped binary
builds.

```
e2e_browser_test.go        //go:build e2e_browser — the new test file
testdata/vendor/
  mermaid-11.17.2.esm.min.mjs   vendored entrypoint of the pinned CDN build
  chunks/mermaid.esm.min/...   the per-diagram chunks that entrypoint imports
```

`mermaid.esm.min.mjs` is not a self-contained bundle — it's a ~30 KB
loader that lazily imports per-diagram chunks from `./chunks/...` at
runtime. Only the chunks a `graph LR` flowchart actually needs are
vendored, a deliberate trade to keep the module zip small (since
`testdata/` ships in it), not an oversight: the full vendored tree is 27
files and 0.82 MiB, not the 30 KB the entrypoint alone would suggest.

`chromedp` lands in `go.mod`, but only test files import it, and Go never
compiles test files into a non-test build — `go install
.../cmd/md2html@latest` pulls in exactly what `cmd/md2html` needs
transitively, which doesn't change. The shipped binary's dependency graph
is unaffected.

`go test ./...` (and CI, whenever it exists) never needs a browser
installed. These tests run only via explicit opt-in:

```
go test -tags e2e_browser ./...
```

`e2e_browser_test.go` lives in package `md2html`, matching the flat
layout of the existing `e2e_test.go` — this repo has no `internal/`
packages, and there's no reason to introduce one for this.

## Components

### Browser harness

```go
func newBrowserCtx(t *testing.T) context.Context
```

Wraps `chromedp.NewContext` over a headless, no-sandbox allocator (CI
images commonly run without a sandbox available). Wires a per-test
`context.WithTimeout` and `t.Cleanup` teardown, so a stuck dialog or an
unresponsive browser can't hang the suite.

### Page server

```go
func serveGenerated(t *testing.T, files map[string][]byte) (baseURL string)
```

Writes `files` into a temp directory and serves it with
`httptest.Server` + `http.FileServer`. Used for the rendered HTML, the
generated image fixtures, and — for the mermaid tests — the vendored
mermaid bundle. Real HTTP rather than `file://`, so there's no browser
security-model difference from a real deployment to account for.

### Image fixtures

```go
func pngFixture(w, h int) []byte
```

Synthesizes a solid-color PNG at an exact pixel size with the stdlib
`image`/`image/png` packages. No binary test images are committed to the
repo; only the vendored mermaid tree needs that treatment, because it
can't be synthesized.

### `mermaidCDN` becomes injectable

`mermaidCDN` changes from a `const` to an unexported package-level `var`;
`mermaidRuntime` changes from a `const` string to `func mermaidRuntime()
string`, building the same content against the current value of
`mermaidCDN`. Production behavior and output are unchanged — the only
caller that ever reassigns the var is the new build-tag-gated test file,
pointing it at the local server's vendored copy instead of the real CDN
before calling `Convert`.

This is the one production-code change in this design. The alternative —
chromedp Fetch-domain request interception, redirecting the real CDN
request to the vendored bytes with zero production changes — was
considered and rejected; see Alternatives Considered.

### Version-drift guard

```go
func TestVendoredMermaidVersionMatchesPinned(t *testing.T)
```

A plain string comparison between the vendored filename's version and
`mermaidCDN`'s pinned version. Not build-tag gated — it needs no browser —
so a future version bump that forgets to re-vendor fails loudly in the
default `go test ./...` run, not just in the opt-in browser suite.

## Test list

1. **`TestBrowserSmallImageNotExpandable`** — a 10×10 image gets no
   `.expandable` class; clicking it does nothing.
2. **`TestBrowserLargeImageExpandsOnClick`** — a 2000×1500 image gets
   `.expandable`; clicking it opens `dialog.media-lightbox[open]`
   containing a cloned `<img>` with explicit `width`/`height` attributes.
3. **`TestBrowserBackdropClickCloses`** — after opening, a click on the
   dialog backdrop (not its content) closes it.
4. **`TestBrowserLinkedImageNotWired`** — an image wrapped in `<a href>`
   never gets `.expandable` or a click handler.
5. **`TestBrowserSvgInsideMermaidPreExcluded`** — a hand-authored
   `<pre class="mermaid"><svg>…</svg></pre>` fixture (not a real mermaid
   render — just that DOM shape) confirms the media-expand runtime's
   exclusion filter in isolation, without needing mermaid to run at all.
6. **`TestBrowserSvgInsideExpandsOnClick`** — a hand-authored inline
   `<svg>`, not inside `pre.mermaid`, rendered below its own viewBox size
   gets `.expandable`; clicking it opens `dialog.media-lightbox[open]`
   containing a cloned `<svg>` with explicit `width`/`height` attributes
   taken from the viewBox, not the displayed size — the positive
   counterpart to test 5, since that test alone would pass just as well if
   the SVG branch of `naturalSize` were removed entirely.
7. **`TestBrowserMermaidDiagramExpandsOnClick`** — a real ` ```mermaid `
   fence, served against the vendored library so it actually renders,
   opens `dialog.mermaid-lightbox[open]` on click with a cloned SVG
   carrying explicit width/height from its viewBox.

Every assertion reads live DOM state through chromedp (`dialog.open`,
attribute values, class lists) rather than sleeping and hoping.

## Error handling / robustness

| Concern | Mitigation |
|---|---|
| Hung dialog / unresponsive page | per-test `context.WithTimeout` around all browser actions |
| Browser or server left running after a failing test | `t.Cleanup` tears both down unconditionally, including on panic |
| Flaky timing (script hasn't run yet) | assert on real DOM state (`dialog.open`, computed class list) rather than fixed sleeps |

## Alternatives considered

- **CDP Fetch-domain interception** instead of the injectable
  `mermaidCDN` var. Zero production-code changes, but this repo has no
  precedent for request interception, and it adds plumbing to maintain
  for a single call site. Rejected in favor of the smaller, more
  idiomatic production change.
- **playwright-go** or **go-rod/rod** instead of chromedp. chromedp is
  the most mature pure-Go CDP driver and assumes an already-present
  Chrome/Chromium, matching this project's "one static binary, no
  runtime" ethos; playwright-go's own installer pulls in a bundled
  Node.js toolchain, which this project has never needed.
- **A mermaid-version-recency test.** See Scope — rejected as fighting
  the deliberate pinning strategy and reintroducing the live-network
  dependency this design vendors mermaid specifically to avoid.

## Testing

The harness helpers themselves (`pngFixture`, the version-drift guard)
are plain Go and get ordinary, non-gated tests. TDD applies to building
the harness the same as any other code in this repo — write the failing
assertion first, then the minimal helper that satisfies it.

## Out of scope

CI setup, cross-browser coverage, auto-updating the pinned mermaid
version, and any test of that version's recency.
