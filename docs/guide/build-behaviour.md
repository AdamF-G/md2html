---
nav-order: 13
---
# Build Behaviour

## What conversion adds

- an `id` and a hover anchor on every heading
- a horizontal scroll wrapper around every table
- `target="_blank" rel="noopener noreferrer"` on off-site links
- `.md` links rewritten to `.html`, and asset links pointed back at the
  original file, except in a site, which copies them (see
  [Links and files](sites.md#links-and-files))
- a `<title>`, from front matter, the first `<h1>`, or the file name
- the Markdown source, hidden at the end of the page with Copy and Download
  controls

Flags turn each off: `--no-anchors` (which also turns off the contents list
and `§` references), `--no-table-scroll`, `--no-external-links`,
`--no-md-links`, `--no-assets` and `--no-source`.

## Following links

Every `.md` a page links to is converted too, however far away it is, and
the run reports how many it pulled in from outside the entry points.
`--depth` limits how deep directories are scanned, not how far links are
followed; `--link-depth` limits link hops. Several entry points
(`md2html -o site ./docs ./notes`) build as one set, with links between
them rewritten.

Without `-o`, each page is written beside its source, and a link that
leaves the entry tree is refused with a warning and left as written.

`--exclude DIR` never enters or writes to a directory, and `--exclude
'AUDIT_*'` skips any file or directory with a matching name at any depth.
A link into an excluded path keeps its href and warns.

## Overwriting

md2html only overwrites HTML it generated. Any other file at an output path
is refused and the run exits non-zero. `a.md` and `a.markdown` in one
directory both map to `a.html`, and the run stops before writing anything.

Without `--site`, md2html builds no site navigation: the contents list
covers one page, so link between pages yourself. See [Sites](sites.md).

## Fragment output

`--fragment` emits the provenance marker, `<title>`, `<style>` and the
body in a `<main class="md2html">`, for a host that supplies the page,
such as a Claude Artifact. The host supplies the document but not a text
column, so the `<main>` keeps the column's width, and the stylesheet styles
only that element, never the host's own `<main>`. A fragment has no script: no
mermaid runtime, no floating contents list, no click-to-expand, and no
embedded source. A mermaid fence arrives as `<pre class="mermaid">`, which
an Artifact draws and other hosts show as text. To preview a fragment in a
browser, add `--preview-runtime`: a fragment with a diagram then ends in
the same mermaid runtime a full page has. Publish without it, since the
host draws diagrams in its own theme.

md2html suits document-shaped pages: a report, a spec, a guide. A page that
wants an app-like layout or a bespoke look is better written as HTML.
