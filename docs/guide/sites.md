---
nav-order: 12
---
# Sites

[[toc]]

Site mode wraps every page of a build in a shell and draws the site's
navigation and search from the pages themselves. There is no config file:
the structure comes from the pages and their flat front matter. Without
`--site`, a build is exactly what it was.

## The shell

A page becomes a shell when its front matter says `site: true`; the file
can have any name and live outside the entry points. Select it with
`--site PATH`:

```text
md2html --site shell.md -o site ./docs
```

The shell is rendered around every page, with `[[content]]` replaced by
that page's body. It is a template, not a page: it is never written out
and never appears in the tree. It needs exactly one `[[content]]` and
`site: true`, or the run stops. `--site` is refused with `--fragment`, a
fragment having no page to wrap, and `--toc float` is refused with
`--site`.

````markdown {caption="shell.md"}
---
site: true
style: house-site
search-facets: kind
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

::: footer
Built with md2html.
:::
````

The shell's content goes directly in `<body>`, and `[[content]]` becomes
the page's `<main class="md2html">`, so a sidebar or top bar is not
confined to the reading column. A shell may put `[[content]]` inside its
own containers, for a grid. A `[[toc]]` in the shell lists the current
page's headings, not the shell's, and a contents list under a shell is
always inline, where its marker is. A page's `toc: float` warns and is
drawn inline, and `--autotoc` adds no second list to a page when the
shell has a `[[toc]]`.

The shell's links are rewritten for each page, so a link in it reaches its
target from every page: a link to a page is to its HTML, and any other
relative link, an image beside the shell, say, is to the file itself. A
link to a `.md` the build does not write reaches the Markdown file, and
warns. In a mounted site, a shell link that reaches nothing on disk is
read from the site's root. The shell's `style:` (and `plugins:`, relative to
the shell) is the site's style: a page's own `style:` wins for that page,
and `--style` applies where neither sets one. The shell's plugins
supply no languages to the pages it wraps: those come from `--plugin`
and each page's own `plugins:`. Its other keys describe the shell:
`title` and `lang` stay each page's own. A page with `shell: none`
is built without the shell and stays in the tree, in search and in
previous and next.

Shell and page are one document, so their ids share one namespace. The
page's ids are kept, since other pages link to them; a shell id that a
page also uses is renumbered as a repeated heading is.

A problem in the shell's own source, such as a marker it misuses, is
reported once per run and attributed to the shell file, not to every page
it wraps.

## The tree

The tree is derived from the pages being built. Its root is the run's
base, the deepest directory holding every entry point.

- A directory is a node, and its page is its `index.md`, else its
  `README.md`, which then is not listed again inside it, else a page
  md2html writes: its name in title case as its title, and a list of its
  pages. The shell's `title-case:` lists words that keep a fixed
  spelling, as `title-case: API, CLI`; a mount's `title` names its own
  directory's page. In a build with `-o`, every directory's page is
  written at its `index.html`, so a link to the directory reaches it. A
  directory whose `index.md` or `README.md` the build did not reach keeps
  its label and warns, with no page written over it.
- A page's label is its `# Title`, or `nav-title:` when set.
- Order is `nav-order:` (a whole number) first, then label alphabetically.
  `nav-sort: links` in a directory's page (its `index.md` or
  `README.md`) puts its children in the order that page links them
  instead: those with `nav-order:` still come first, then each page or
  subdirectory it links, by its first link, then the rest by label. A
  link counts when it reaches the child's own page, or the subdirectory
  itself; one deeper into a subdirectory does not. A subdirectory orders
  its own children by its own page.
- `nav: hidden` keeps a page out of the tree, with every page below it,
  and out of previous and next; a hidden page shows no previous and next
  of its own. It stays in search and is reachable by links. On the root's `index.md` it hides only that page.
- `[[pages]]` lists what is below the root, never the root's own page:
  link the root's page from the shell.
- A page from outside the root is not built in a site written to `-o`
  (see [Links and files](#links-and-files)); built in place, it is
  written but has no place in the tree, and warns.

## Mounts {#mounts}

A site's files are rarely in one directory. `[[mount]]` markers in the
shell place sources from anywhere in the repository at paths in the
site, so the shell says what the site holds, and the build needs no
entries:

```markdown
---
site: true
---
[[mount source="../index.md"]]

[[mount path="overview" source="../../README.md" nav-order="1"]]

[[mount path="guide" source="../guide" nav-order="2"]]

[[mount path="design-notes" source="../specs" nav-title="Design Notes"]]
```

```bash
md2html --site docs/site/shell.md -o _site
```

| Attribute | Meaning |
|---|---|
| `source` | A directory or a `.md` file, relative to the shell. Required. |
| `path` | Where it appears in the site, such as `guide` or `handbook/changes`. Absent, the mount is the site's root. |
| `exclude` | Comma-separated directories and name globs, as `--exclude`, relative to the source. |
| `nav-title`, `nav-order` | The mount's label and place, over the page's own. |
| `nav-sort` | `links` orders a directory mount's children by its index's links, and every directory below that sets none. |
| `title` | The title of the page md2html writes for the mount's own directory, when that directory has no `index.md` or `README.md`; directory mounts only, a file mount refuses it. |
| `toc` | `shell` drops the mounted pages' own contents lists, so the shell's `[[toc]]` is the only one. |

- A directory mount builds every `.md` below it, at its path under the
  mount's; a file mount builds that one page. Pages are written at their
  site paths, so `docs/guide/sites.md` mounted at `guide` becomes
  `guide/sites.html`.
- One mount has no `path`: the root. A mount inside another's source
  takes its files out of the enclosing one, and two mounts may share a
  directory, with one value for each setting.
- A link between mounted pages reaches the page where it is written. A
  file a mount holds, and no exclude names, is in the site; anything
  else is outside it, and follows `unmounted:` (see
  [Links and files](#links-and-files)).
- A link that reaches nothing on disk, or an excluded file or directory,
  is read from where the page appears in the site, so a hand-written
  page and a generated one can link to each other by their places. Such
  a link breaks on the hosting page. Generating the pages into the
  source directory, where they appear in the site, remains the simpler
  arrangement.
- `--exclude` applies to every mount, relative to the source root, the
  deepest directory holding the shell and the mounts' sources. A mounted
  site reads chart data and `@file#key` references only from inside it.
- With mounts, entries, `--depth` and `--link-depth` are refused, and
  `-o` is required. `--mount 'path="team" source="notes"'` adds a mount
  for one build, its source read from the working directory.
- `[[mount]]` renders nothing; in a page's body it warns. A page with
  `toc: none` gets no contents list from the shell, in any site.

## Links and files {#links-and-files}

A site is published as its output directory alone, so it holds
everything it shows. A file is in the site when it is under the run's
base.

- A page in the site is linked where it is written. A `.md` outside the
  site is never built: nothing is written under `_external/`.
- Any other file in the site that a page shows or links, an image, a
  stylesheet, a PDF, is copied to the same place in the output.
  `md2html-assets.txt` at the output root records the copies, so a later
  build replaces its own and refuses a file someone else put there.
  A link to a directory (`[Home](./)`, `[Docs](sub/)`) is left as
  written in a site without mounts; nothing is copied for it. In a
  mounted site, a link to a directory the site has a page for reaches its
  page, and one to a directory the site holds without a page is left as
  written. A link to any other directory follows `unmounted:`.
- An image or other file a page loads from outside the site is copied to
  `_assets/`, at its path from the source root.
- A link a reader follows out of the site, to a page or any file, goes
  where the shell's `unmounted:` says:

  | `unmounted:` | The link |
  |---|---|
  | `link` | Goes to the file's page on the repository's host, from `repo-url:`. The default when `repo-url:` is set. |
  | `unlink` | Is removed, with a warning; its text stays. The default without `repo-url:`. |
  | `error` | Stops the build, naming the page and the link. |

```markdown
---
site: true
repo-url: https://github.com/acme/repo/blob/main/{path}
---
```

`{path}` is the file's path from the **source root**: the deepest
directory holding the shell and the pages, which for a repository's
docs is the repository's root. `source-root:` in the shell, relative to
it, or `--source-root`, sets it instead; a default that would be the
filesystem's root is refused. A file outside the source root has no
path, so a link to it is unlinked and an image from it dropped, with a
warning.

`--no-assets` copies nothing and leaves those links as written; a link
out of the site still follows `unmounted:`. A file under an
`--exclude`d path keeps its link as written. A site built in place,
without `-o`, keeps its files where they are.

## Navigation markers

A marker is a paragraph of its own: a blank line before and after, and
nothing else in it. Attribute values are double-quoted. These need a
shell; without one they warn and stay as written.

| Marker | Draws |
|---|---|
| `[[content]]` | The current page's body. Exactly one per shell. |
| `[[pages]]` | The tree below the root as a nested list of links, the current page marked `aria-current="page"`. `depth="N"` limits levels; `from="section"` starts at the current page's top-level node, or at the top when that node has nothing below it, as on the root page; `from="root"` (the default) at the top. `from="page"` starts at the current page, so a directory's page lists what is in it. `collapse="true"` folds each branch under a chevron beside its link, open on the current page's branch, so the tree has one shape on every page; `chevron="right"` puts the chevrons at the end of each row instead of the left, and `--md2html-fold-col` in a style widens the left column. |
| `[[breadcrumb]]` | The current page's ancestors, in a `<nav aria-label="Breadcrumb">`; nothing on the root page. |
| `[[prev-next]]` | The previous and next page in tree order, in a `<nav aria-label="Previous and next">`. |

`[[pages]]` is a bare list, since a shell may show it twice, as tabs and
as a sidebar. Wrap each in `::: nav {aria-label="…"}` to name it.
`[[breadcrumb]]` and `[[prev-next]]` carry their own landmark: do not wrap
them in another `::: nav`.

## Search

`[[search]]` draws a search box over an index md2html writes at build
time: titles, headings, `defs` terms and each page's first paragraph. The
index is `md2html-search.js` at the output root, a script rather than a
data file, so search works on a page opened straight from disk. It begins
with md2html's provenance marker, so overwrite protection covers it.
`search-facets: kind, area` in the shell's front matter adds those keys'
values to each result. `/` and Ctrl or Cmd+K focus the box.

Search and the Copy buttons on code blocks are small inline scripts,
included only where needed: search where `[[search]]` is, Copy buttons on
every page built with a shell. Without a script, everything else still
works.

## Lists from front matter and directories

`[[list]]` and `[[subdirs]]` work on any page, shell or not.

```markdown
---
version: v0.12
versions: v0.13, v0.12, v0.9
---

[[list key="versions" link="/docs/{item}/" current="{version}"]]

[[subdirs path="/srv/docs" match="v*" link="https://example.com/{item}/"]]
```

`[[list key="K"]]` makes one link per entry of the comma-separated value
`K`. `[[subdirs path="P"]]` does the same for the subdirectories of `P`,
absolute or relative to the file holding the marker. Both take:

| Attribute | Meaning |
|---|---|
| `link` | URL template. `{item}` is the entry; `{name}` is any front matter key of the file holding the marker. An unknown name warns and expands to nothing. |
| `text` | Label template, default `{item}`. `[[subdirs]]` also offers `{target}`, a symlink's target name. |
| `current` | Template; the entry equal to it is marked `aria-current="true"` (`[[pages]]` marks its page `"page"`). |
| `as` | `list` (default) or `inline`, one paragraph of comma-separated links. |

`[[subdirs]]` also takes `match`, a glob (default `*`), and `sort`:
`version` (default) compares embedded numbers, so `v0.13` sorts above `v0.9`;
`name` is alphabetical. Symlinks are aliases, listed first whatever
`match` says; a dangling one warns. A path outside the build cannot be
checked, so make `link` absolute.

Templates expand only inside a marker's attributes, never in a heading or
a container's title. A `link` is used as written: a `.md` in it is not
rewritten to `.html` or followed, so write the `.html`, or an absolute
URL. For a dropdown, put the marker inside
`::: details`; a native `<select>` is not offered, since moving from one
needs a script.

## References to other files

A front matter value of the form `@path#key` is the value of `key` in the
front matter of the file at `path`:

```markdown
---
versions: @../versions.md#all-versions
---
```

The path is relative to the referring page, and may be absolute or contain
`..`. md2html reads only that file's front matter. A reference is followed
only where md2html reads a key as data: the `key` of a `[[list]]`, a
`{name}` in a marker's template, and a `search-facets` key. Anywhere else
the value stays as written, so `title: @meta.md#title` is a literal title,
and an `author:` that happens to start with `@` is untouched. A missing
file or key warns. The value found is used as written: a reference to a
reference is not followed, and warns, so references cannot loop.

## Traps

Each of these gives valid output and no warning.

- **A directory's page is its `index.md`.** Without one it is the
  directory's `README.md`; a `README.md` beside an `index.md` is an
  ordinary page in that directory.
- **A site built in place writes into your sources.** A directory with no
  `index.md` or `README.md` gets its `index.html` beside its sources, and
  md2html refuses to overwrite an `index.html` it did not write there:
  add an `index.md`, or build with `-o`.
- **Markers on consecutive lines are one paragraph.** `[[breadcrumb]]`
  directly above `[[prev-next]]` expands neither; each marker needs a
  blank line before and after.
- **A reference resolves only where a marker or facet reads it.**
  `title: @meta.md#title` stays as written; only a `[[list]]` key, a
  `{name}` in a template and a `search-facets` key follow `@file#key`.
- **Wide figures stay in the column under a shell.** The breakout needs
  `<main>` centred in the viewport; a site style that wants one writes it
  for its own layout.

## What a style targets

The markers draw plain HTML with stable classes, using the theme's custom
properties, so a style places and recolours all of it:
`md2html-pages`, `md2html-branch`, `md2html-label`, `md2html-breadcrumb`,
`md2html-prev-next` with `md2html-prev` and `md2html-next`, `md2html-list`
and `md2html-inline`, `md2html-search` with `md2html-search-input`,
`md2html-search-results` and `md2html-search-kind`, and `md2html-copy` on
a code block's button with `md2html-copyable` on its `<pre>`. Each page's
`<html>` carries `data-section`, the slug of its top-level node (absent at
the root), for `:root[data-section="guides"]`. md2html styles these
classes plainly itself, ahead of the page's style and only on a page that
has them, so a style's own rules win. See
[What a style can change](styles.md#what-a-style-can-change).
