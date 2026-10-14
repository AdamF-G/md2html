# Site mode — Design

**Date:** 2026-10-14
**Author:** AdamF-G
**Status:** Draft for review

## Purpose

md2html builds pages; a documentation site also needs what sits around
them: a header, navigation through the whole set, a breadcrumb,
previous/next, search. Site mode adds that without adding config. One
ordinary Markdown page, marked as the site's shell, wraps every page in the
build, and markers in it expand into the site's structure, which is derived
from the pages themselves.

## 1. Principles {#principles}

- **No external config.** Structure comes from the pages themselves and
  their flat front matter. No JSON, no YAML tree, no special file names.
- **Markers, not modes.** md2html has no layout presets. A shell arranges
  markers inside its own containers, and a style places them. Any layout an
  author can write in Markdown and CSS is possible.
- **Front matter stays flat.** The one addition is a value reference
  (`@file#key`), which is itself a flat string.
- **Nothing changes without a shell.** A build without `--site` behaves
  exactly as today, warnings included, unless a page uses one of the new
  markers.

## 2. The shell {#shell}

A page becomes a shell when its front matter has `site: true`; the file can
have any name. A run selects it explicitly with `--site PATH`. The shell is
rendered around every page in the emit set, with `[[content]]` replaced by
that page's body. A page with `shell: none` is rendered without it and stays
in the tree, in search and in previous/next.

The shell is a template, not a page: it is never written out and never
appears in the tree, and it may live outside the entry points. Its own
links are rewritten for each page it wraps, so a link in the shell reaches
its target from every page. A `[[toc]]` in the shell lists the current
page's headings, not the shell's. A shell needs exactly one `[[content]]`,
and its front matter must say `site: true`; either missing stops the run.
`--site` is refused with `--fragment`: a fragment has no page to wrap.

The shell's content goes directly in `<body>`, and `[[content]]` becomes the
page's `<main class="md2html">`. The reading column's width applies to the
page alone, so a sidebar or top bar is not confined to it, and the page's
`<main>` is the one main landmark. A shell may put `[[content]]` inside its
own containers, for a grid, say.

The shell's front matter `style:` (and `plugins:`, relative to the shell)
is the site's style: a page's own `style:` still wins for that page, and
`--style` applies where neither sets one. Other keys in the shell's front
matter describe the shell, not the pages: `title`, `lang` and the rest stay
each page's own.

Shell and page are one HTML document, so their ids share one namespace.
The page's ids are kept, since other pages link to them; a shell id that a
page also uses is renumbered as a repeated heading is, with the shell's
links to it updated.

````markdown {caption="nav.md"}
---
site: true
style: house-site
search-facets: kind, area
---
::: header {.topbar}
**Acme**

[[pages depth="1"]]

[[search]]
:::

::: nav {.sidebar aria-label="Section"}
[[pages from="section"]]
:::

[[breadcrumb]]

[[content]]

[[prev-next]]
````

Each marker is a paragraph of its own, so the top bar's three items are
three paragraphs, and the style lays them out in a row.

`::: header` and `::: footer` are container kinds alongside `::: nav`, so a
shell's banner and footer are landmarks like its navigation and the page's
`<main>`. They work on any page, as `::: nav` does.

## 3. The tree {#tree}

The site's tree is derived from the emit set. Its root is the run's base,
the deepest directory holding every entry point; a document from outside
it, pulled in by a link, is still written but cannot be placed, and warns.

- A directory is a node; its `index.md` gives its title and page. A
  directory without one is a label named after the directory, with a
  warning.
- A page's label is its `# Title`, or `nav-title:` when set.
- Order is `nav-order:` (an integer) first, then label alphabetically.
- `nav: hidden` keeps a page out of the tree; it stays in search and is
  reachable by links.
- A page in the emit set that the tree cannot place warns.

## 4. Markers {#markers}

Markers are paragraphs of their own, like the existing `[[toc]]`: a blank
line before and after, nothing else in the paragraph. Markers on
consecutive lines form one paragraph and none of them expands; a marker
inside a sentence is text. Markers take double-quoted attributes. Each expands to plain HTML; the container around
it decides how it looks. A URL inside an attribute is still part of the
marker, though GFM would otherwise turn it into a link. `[[list]]` and
`[[subdirs]]` work on any page; the navigation markers need a shell, and
without one they warn and stay as written.

| Marker | Expands to |
|---|---|
| `[[content]]` | The current page's body. Exactly one per shell. |
| `[[pages]]` | The tree as a nested list of links, current page marked `aria-current="page"`. `depth="N"` limits levels; `from="section"` starts at the current page's top-level node; `from="root"` (default) at the top. |
| `[[breadcrumb]]` | The current page's ancestors as a list of links, in its own `<nav aria-label="Breadcrumb">`. A trail that would hold only the current page, as on the root page, is not drawn. |
| `[[prev-next]]` | Links to the previous and next page in tree order, in its own `<nav aria-label="Previous and next">`. |
| `[[toc]]` | The current page's contents list, as today. |
| `[[search]]` | A search box over the build's index (§6). |
| `[[list]]` | Links built from a comma-separated front-matter value (§5). |
| `[[subdirs]]` | Links built from a directory's subdirectories (§5). |

`[[pages]]` is a bare list because a shell may show it more than once, as
tabs and as a sidebar, and each copy needs its own label: the shell's
`::: nav {aria-label="…"}` gives it one. `[[breadcrumb]]` and
`[[prev-next]]` each appear once and have one obvious name, so they carry
their own landmark and should not be wrapped in another `::: nav`.

Contents lists under a shell are always inline, where the marker is: the
shell places them. `toc: float` positions the list against a column
centred in the viewport, which a site layout does not promise, so under a
shell a page's `toc: float` warns and is rendered inline, `--toc float`
is refused with `--site`, and `--autotoc`'s lists default to inline. When the shell has a `[[toc]]`, `--autotoc`
adds no second list to the page; a `[[toc]]` the page writes itself is
still rendered.

## 5. Lists from values and directories {#lists}

`[[list key="K" link="T"]]` expands the comma-separated front-matter value
`K` into one link per entry. `[[subdirs path="P" link="T"]]` does the same
for the subdirectories of the filesystem path `P`, absolute or relative to
the file holding the marker. Both take:

| Attribute | Meaning |
|---|---|
| `link` | URL template: `{item}` is the entry; `{name}` is any front-matter key of the file holding the marker. An unknown name warns and expands to nothing. Templates are expanded in marker attributes only. |
| `text` | Label template, default `{item}`. `[[subdirs]]` also offers `{target}`, a symlink's target name. |
| `current` | Template; the entry equal to it is marked `aria-current`. |
| `as` | `list` (default), or `inline`: one paragraph of comma-separated links. A marker is a paragraph of its own, so an inline list cannot sit inside a sentence; a label goes in the paragraph before it, or in the container's title. |

`[[subdirs]]` also takes `match` (a glob, default `*`) and `sort` (`version`,
the default, compares embedded numbers so `v0.13` sorts above `v0.9`;
`name` is alphabetical). Symlinks are aliases: they are listed first,
regardless of `match`, and a dangling one warns. Links built from a path
outside the emit set cannot be checked, so `link` should be absolute.

A dropdown is a `[[list]]` or `[[subdirs]]` inside `::: details`, styled; a
native `<select>` is not offered, because navigating from one needs a
script.

**Value references.** A front-matter value of the form `@path#key` is the
value of `key` in the front matter of the page at `path`, relative to the
referring page. It is resolved only where md2html reads the key as data:
the `key` of a `[[list]]`, a `{name}` in a marker's template, or a
`search-facets` key. Anywhere else the value is used as written, so a page
whose `author:` happens to start with `@` is unaffected, and so is a
`title:` written as a reference. A missing file or key warns. The value found is used as
written: a reference to a reference is not followed, and warns, so
references cannot loop. A referenced page is an ordinary page; it can
render its own value with `[[list]]`.

## 6. Search and the built-in script {#search}

md2html writes a search index at build time — titles, headings, `defs`
terms and each page's first paragraph — plus the values of every
front-matter key the shell names in `search-facets:`, shown as the result's
kind. The index is a build output, `md2html-search.js` at the site's output
root, never authored. It is a script that sets a global rather than a data
file, so search works on a page opened straight from disk, where a browser
blocks fetching a file; it starts with the provenance marker, so md2html's
overwrite protection covers it.

Like md2html's other runtimes — the contents list's side control, the
image lightbox, the source tools — the search box (with a keyboard
shortcut) and copy buttons on code blocks are inline scripts, included only
on pages that need them: search where `[[search]]` is, copy buttons on every
page built with a shell, so output without one is unchanged. Without a
script everything except search and copying works. Style plugins still
cannot add scripts.

## 7. Styling {#styling}

The shell's containers carry the author's classes; the markers' output
carries stable class names and uses the existing theme variables, so a style
plugin can place and restyle all of it. md2html draws that output plainly
itself, with rules of its own that belong to no style, ahead of the page's
style and only on a page that carries it (a shell's page, or one with a
`[[list]]` or `[[subdirs]]`), so every style gets them, a style's own rules
win, and a page without markers is unchanged. Each page's `<html>` element also
carries `data-section`, the slug of the page's top-level node in the tree
(absent for a page at the root), so a style can colour or lay out a whole
section differently with `:root[data-section="guides"]`.

Wide figures and `::: wide` keep breaking out of the column only when
`<main>` is the body's child and centred in the viewport, as today. In a
shell, `<main>` sits among the shell's content, so they stay at the
column's width; a site style that wants a breakout writes one for its own
layout. A layout that cannot be written as
containers and markers needs a new marker in md2html, not a plugin.

## 8. Testing {#testing}

Golden pages per marker and attribute; marker paragraphs (consecutive lines,
markers in a sentence); tree derivation (index pages, `nav-order`, hidden
pages, unplaceable pages); value references (missing file and key, and an
`@` value no marker reads, which is left alone without a warning); the
shell's `<main>` placement, its style as the default, and its ids renumbered
around the page's; contents lists under a shell; `[[subdirs]]` ordering, `match`, symlinks and dangling
symlinks; byte-identical output for identical input; and a build with no
shell, unchanged from today.
