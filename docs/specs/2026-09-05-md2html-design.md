# md2html — Design

**Date:** 2026-09-05
**Author:** AdamF-G
**Module:** `github.com/AdamF-G/md2html`
**Status:** Approved; API section revised 2026-09-05 to match the accepted
refinements in `docs/plans/2026-09-05-md2html.md`

## Purpose

A single-binary CLI that converts existing Markdown docs to HTML with no
setup ceremony. Drop it into any repo — Go, JS, Python, or plain Markdown —
and get a browsable HTML tree in one command:

```
go install github.com/AdamF-G/md2html/cmd/md2html@latest
md2html ./docs -o ./site
```

No config file, no runtime, no `node_modules`.

## Why Go / goldmark

Measured on a 500-file, 151k-line, 2.6 MB corpus (see Benchmarks):

| Engine | Time |
|---|---|
| goldmark | 78 ms |
| goldmark + full HTML5 reparse + transform | 160 ms |
| markdown-it | 529 ms |
| remark (4 workers) | 1338 ms |
| remark (1 core) | 2441 ms |

goldmark plus `golang.org/x/net/html` gives tree-level transforms over
**all** HTML — both generated and hand-written raw HTML — at 15x remark's
speed, in a static binary with no runtime dependency.

## Conformance target

The pipeline must represent everything an Artifact page uses. Verified
16/16 against a fixture covering: `<title>`, `<style>` with theme tokens
and blank lines, raw `<script>` (contents never re-parsed as Markdown),
inline SVG, ```mermaid fences, GFM tables, definition lists, footnotes,
`{#id .class}` attributes, and `:::` containers.

## Architecture

### Layout

```
go.mod                  module github.com/AdamF-G/md2html
md2html.go              Convert(), Options — the pipeline
tree.go                 parseFragment, renderTree, walk
transform.go            Transform type + built-ins
theme.go                //go:embed default.css
default.css             embedded stylesheet, both themes
page.go                 page/fragment shells, title, provenance marker
write.go                SafeWrite — the overwrite guard
links.go                link extraction and classification
paths.go                commonAncestor, isUnder, outputPath
crawl.go                seeding, link BFS, per-document link maps
cmd/md2html/main.go     flags, worker pool, reporting
testdata/               golden fixtures
```

Root package is importable as a library; `cmd/md2html` is a thin wrapper.

### Single-document pipeline

```
md → goldmark(GFM, DefinitionList, Footnote, WithAttribute, WithUnsafe,
              mermaid.Extender{RenderModeClient, NoScript:true},
              fences.Extender{})
   → x/net/html.ParseFragment into a synthetic root node
   → run []Transform over the tree
   → html.Render
   → page shell (or fragment shell if --fragment)
```

**Implementation note.** `html.ParseFragment` returns top-level nodes as a
slice, not under a parent. A walk that only recurses into children will
silently skip every top-level element. Always re-parent the returned nodes
under a synthetic root before transforming.

Dependencies: `github.com/yuin/goldmark`,
`go.abhg.dev/goldmark/mermaid`, `github.com/stefanfritsch/goldmark-fences`,
`golang.org/x/net/html`.

### API

```go
type Transform struct {
    Name string
    Fn   func(*html.Node) error
}

type Options struct {
    Fragment   bool
    Title      string            // default: first <h1>, else filename
    SourcePath string            // abs path of the source doc; title fallback
    CSS        string            // override embedded theme
    Transforms []Transform       // default: Builtins()
    LinkMap    map[string]string // href as written → replacement href
}

func Convert(src []byte, opt Options) ([]byte, error)
func Builtins() []Transform
func LinkRewrite(m map[string]string) Transform
```

`Convert` is pure and single-document. The crawler is a separate layer that
calls it. Library users converting one document ignore `LinkMap`.

`LinkMap` is **per-document and keyed by the href exactly as written** in
the source (`"./api/auth.md"` → `"../api/auth.html"`), not by resolved
absolute path. Keying it by absolute path would force every transform to
resolve hrefs against the document's directory and to know its own output
location in order to compute `filepath.Rel` — putting filesystem knowledge
inside transforms. Keying by the raw string keeps all path arithmetic in the
crawl layer, where that context already exists, and reduces the transform to
a map lookup. A `#fragment` on a document link is preserved through the
rewrite.

### Output modes

Two modes, one containment test, different consequences. `base` is the
common ancestor of all entry points.

| | `-o ./site` | no `-o` |
|---|---|---|
| Output location | under `-o` | beside each `.md` |
| Link resolving outside `base` | follow; map into `_external/` | refuse; warn; leave as `.md` |
| Overwrite guard | provenance marker (same rule) | provenance marker (same rule) |

Containment is tested by **resolving the link to an absolute path and
checking whether it is under `base`** — never by string-matching `../`. A
link like `../sibling/doc.md` may stay inside `base` and must be followed.

Recognized Markdown extensions: `.md` and `.markdown`. Any other target is
treated as an asset (see Assets), not a document.

Output path mapping under `-o`:

```
base = /docs
/docs/guide.md            → site/guide.html
/docs/api/auth.md         → site/api/auth.html
/other-repo/docs/api.md   → site/_external/other-repo/docs/api.html
```

Paths outside `base` mirror their absolute path minus the leading
separator. Deterministic, collision-free, and readable when tracing where a
stray doc came from.

All document links are rewritten with `filepath.Rel` from the current
document's output location to its target's, so document-to-document
navigation is self-contained within `-o`. Asset links point outward at the
source tree — see Assets.

### Traversal

Two independent knobs, deliberately not conflated:

| Concern | Bound | Flag |
|---|---|---|
| Following links between documents | always unlimited | none |
| Seeding unlinked files from a directory | directory levels | `--depth N` |

`--depth` defaults to unlimited (whole subtree). `--depth 0` seeds only
`.md` directly inside the directory; `--depth 1` adds one subdirectory
level. It never affects link traversal.

Two phases — mandatory, because link rewriting cannot be decided until the
full emit set is known:

```
Pass 1 (discover)  seeds (files + dirs to --depth) → goldmark
                   → extract .md links → unbounded BFS, cycle-safe
                   → emit set + LinkMap
Pass 2 (emit)      for each doc → Convert(src, Options{LinkMap}) → write
```

Converting twice costs ~150 ms on the benchmark corpus and keeps `Convert`
pure rather than splitting `Transform` into pre- and post-link phases.

**Crawl safety.** Visited set keyed on `filepath.EvalSymlinks`-resolved
absolute paths, so cycles and symlink loops terminate. Links to `.md` files
that do not exist are left untouched and reported with source file and line.

**Known trade-off.** With unbounded following and no root bound, one `../`
link into a large repo pulls that repo's reachable docs into the build.
Cycle detection stops loops, not breadth. Mitigation is visibility, not
restriction: the run prints a summary of everything pulled in from outside
`base`. Writing outside the output directory is structurally impossible.

### Assets

Documents reference local non-Markdown files — images, diagrams, downloads.
**Assets are never copied.** They stay exactly where they are, and their
links are rewritten to point back at the original file on disk.

The link is computed with `filepath.Rel` from the emitted document's output
directory to the asset's real location, emitting as many `../` levels as
needed to climb out of the output path:

```
base = /docs,  -o ./site

/docs/api/auth.md  →  site/api/auth.html
  references ./img/flow.png  (i.e. /docs/api/img/flow.png)
  becomes    ../../docs/api/img/flow.png
```

Remote URLs (`http:`, `https:`, `data:`) and fragment-only links
(`#section`) are left untouched. In-place mode needs no rewriting at all —
the HTML sits beside its source, so existing relative links already resolve.

**Consequence.** The output tree is self-contained with respect to
*documents* — every `.md` link resolves to an emitted `.html` inside `-o` —
but not with respect to *assets*, which resolve outward to the source tree.
Moving `site/` elsewhere breaks images; moving `site/` and the source tree
together preserves them. This is the deliberate trade for never duplicating
bytes we do not own.

### Output provenance

Re-running the tool must be idempotent and quiet, but it must never destroy
a file a person wrote. Both follow from making output self-identifying.

Every generated HTML file begins with a provenance marker as its first
bytes:

```html
<!-- generated by https://github.com/AdamF-G/md2html v0.1.0 - edits will be overwritten -->
```

The full repository URL, not the bare name, is the watermark. `md2html` is
an unremarkable name and other tools share it; the URL guarantees we only
ever claim files this tool wrote.

Before writing, the destination is checked:

| Destination state | Action |
|---|---|
| Does not exist | write |
| Exists, marker present | overwrite silently |
| Exists, no marker | **hard refuse** — warn, skip, non-zero exit |

There is no override flag. A file we did not write is never overwritten.

Detection reads only the first 512 bytes and matches the stable prefix
`<!-- generated by https://github.com/AdamF-G/md2html`, not the full
string, so files written by an older version are still recognized as ours —
while output from any unrelated tool named `md2html` is not.

This rule is **uniform across both modes**. Pointing `-o` at a directory
containing hand-written HTML is exactly as dangerous as writing in place,
and one rule is simpler than a mode-specific exception.

Assets need no provenance rule: they are never written, only linked.

### Built-in transforms

All on by default, each disableable:

| Name | Effect | Flag |
|---|---|---|
| `tableScroll` | wrap `<table>` in `<div class="table-scroll">` | `--no-table-scroll` |
| `headingAnchors` | add `id` + anchor link to `h1`–`h6` | `--no-anchors` |
| `externalLinks` | `target="_blank" rel="noopener noreferrer"` on off-site links | `--no-external-links` |
| `linkRewrite` | apply `LinkMap` to every `href`/`src` | `--no-md-links`, `--no-assets` |

Document links and asset links are rewritten by the **same** transform.
Because `LinkMap` is keyed by the raw href, both cases reduce to the same
lookup, and two separate tree walks would be identical code. The two flags
survive unchanged — they now control which entries the **crawler puts into
the map**, rather than selecting between two near-identical transforms:
`--no-md-links` omits document entries, `--no-assets` omits asset entries.

Mermaid is handled by the goldmark extension, not a transform. Adding a
transform is writing a `func(*html.Node) error` and appending it — no plugin
system, no registry.

### Theming

One `//go:embed`-ed `default.css` with `:root` tokens plus both
`prefers-color-scheme` and `[data-theme]` blocks, so `--fragment` output is
publishable as an Artifact unchanged. `--css mine.css` replaces it.

Both shells emit `<title>`, preceded only by the provenance marker. The
page shell wraps everything in a full `<html>/<head>/<body>` document; the
fragment shell emits marker, `<title>`, `<style>`, then body content, with
no `<!doctype>`, `<html>`, `<head>`, or `<body>` — the shape the Artifact
tool expects. The leading comment is harmless there: the Artifact tool
scans the first 8 KB for `<title>`, so a marker ahead of it is fine.

## CLI

```
md2html ./docs -o ./site                      # seed tree, follow links anywhere
md2html ./docs --depth 0 -o ./site            # top-level seeds only, still follows links
md2html ./docs/index.md ./guides -o ./site    # mixed file and directory entries
md2html ./docs                                # in-place, refuses to escape base
md2html ./docs -o ./site --fragment           # Artifact-shaped output
md2html README.md                             # one file, HTML beside it
```

md2html always writes files; there is no stdout mode. A command that
sometimes prints and sometimes writes into your source tree — depending on
whether the named file happened to link to another — is a trap, so `no -o`
means exactly one thing: write beside the source.

Emit pass runs parallel over `NumCPU`.

## Error handling

| Condition | Behavior |
|---|---|
| Link to missing `.md` | warn (file + href), leave link unchanged, continue |
| Link escapes `base`, no `-o` | warn, leave link unchanged, continue |
| Existing `.html` without our marker | **hard refuse**: warn, skip, continue; non-zero exit |
| Existing `.html` with our marker | overwrite silently |
| Entry point does not exist | error, exit non-zero, convert nothing |
| Unreadable source file | warn, skip, continue |
| Referenced asset missing | warn (file + href), leave link unchanged, continue |
| Malformed Markdown | not an error; goldmark always produces output |

Warnings name the file and the offending href, not a line number. Link
extraction happens on the HTML tree, after goldmark has discarded source
offsets; recovering a line would mean a second traversal of the goldmark AST
joined back to the DOM on href text, which is ambiguous as soon as the same
href appears twice in one document. The file plus the exact href is enough
to locate the problem.

Run ends with a summary: documents converted, pulled in from outside
`base`, warnings, files skipped.

## Testing

TDD throughout. The first and most important test is the 16-check
conformance fixture, locking in measured behavior: it asserts the sixteen
properties that matter rather than freezing a whole byte stream that would
churn on any stylesheet edit. Byte-exact golden files
(`testdata/*.md` → `testdata/*.golden.html`) are a reasonable later
addition, not a prerequisite. Crawl tests cover: cycles, symlink loops, escaping
`base` in both modes, `_external/` mapping, `--depth` seeding levels,
missing link targets, asset link rewriting in both modes (including the
`../` climb out of `-o`), and provenance: marker round-trip, refusal on
unmarked files, silent overwrite of marked ones, marker detection across
version strings, and refusal to claim a file watermarked by a different
tool named `md2html`.

## Out of scope

Config file, watch mode, content-hash cache (a full build is ~160 ms),
plugin system, server-side mermaid rendering, search index, navigation or
sidebar generation.

## Benchmarks

Corpus: 500 files, 151,000 lines, 2.6 MB, generated to resemble real
documentation (headings, tables, fenced code, footnotes, raw HTML blocks,
attributes). Machine: Apple Silicon, 10 cores.

goldmark scales 78 ms → 31 ms across 10 cores. The mermaid and fences
extensions are free (78 ms with, 81 ms without). remark peaks at 4 workers
(1338 ms) and degrades at 10 (1673 ms) — performance/efficiency core split,
no shared JIT cache.
