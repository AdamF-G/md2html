# Mounts — Design

**Date:** 2026-10-16
**Author:** AdamF-G
**Status:** Draft for review.

## Purpose

A repository's documentation is rarely one tidy directory. It is a `docs/`
tree, a root `README.md`, a `CHANGELOG.md`, perhaps a design note beside
the code it describes. Site mode builds a site from the pages it is given,
but it places them by where they sit on disk: the tree's root is the
deepest directory holding every entry point, and a page from outside that
root is written under `_external/` with its absolute path, with no place
in the tree, and a warning.

So a project that wants one site out of files spread across a repo has to
stage a copy first: copy each source into one directory under the names it
wants in the site, drop what should not be there, write the directory pages
that are missing, set the tab label and order of each section, and then run
md2html on the copy. Copying also breaks every relative link between files
that move relative to each other, so the copy step ends up rewriting links
too, or the sources are written for the site and break on the repository's
hosting page.

This note describes what md2html needs so that the copy is not needed: the
site's shell says where each source appears in the site, and md2html
handles the links, assets and directory pages of what it hosts.

## 1. Principles {#principles}

- **Where a file lives and where it appears are separate.** The site's tree
  is a property of the site. A source is placed in it by name, and keeps
  its place on disk.
- **The shell describes the site.** Mounts are markers in the shell, beside
  the markers that draw the site's navigation, so the site's structure is
  read in one file. A build that must differ from the shell, such as one
  that adds a section for a narrower audience, adds mounts with the
  `--mount` flag, in the marker's own syntax (§3), and changes nothing
  else.
- **No copy to maintain.** A page is read from its source. Nothing the
  author wrote is rewritten on disk, and no staged tree is left behind.
- **A link written for the source works in the site.** Authors write links
  that work on the repository's hosting page and in an editor. Those links
  keep working in the built site, and a link that cannot is reported, never
  left to break silently.
- **A site holds everything it shows.** A host such as GitHub Pages serves
  the output directory alone, so no link in a built site points outside it,
  and no output path carries the building machine's own paths.
- **Flat and declarative.** A mount's options are `key="value"` attributes,
  as every marker's are, and the site's own settings are flat front matter
  keys on the shell.

## 2. What exists today {#today}

The design leans on these, and changes the last two:

- A site's shell is a page with `site: true`, selected with `--site`, and
  its markers expand into the site's structure.
- Links between documents are resolved to their source files and rewritten
  to the pages those files become.
- `nav-title:`, `nav-order:` and `nav: hidden` shape the tree.
- `--exclude` takes directory prefixes and name globs such as `AUDIT_*`.
- **Assets are linked, never copied.** A page's image link is rewritten to
  climb out of the output directory back to the original. A site published
  on its own then shows no images, and links such as a README's `./LICENSE`
  lead outside the site.
- **A page from outside the base is written under `_external/`**, at a path
  that mirrors its absolute path on the building machine.

## 3. Mounts {#mounts}

````markdown {caption="docs/site/shell.md"}
---
site: true
style: proof, docs
plugins: style
repo-url: https://github.com/AdamF-G/md2html/blob/main/{path}
title-case: API, CLI
---
[[mount source="../index.md"]]

[[mount path="overview" source="../../README.md" nav-order="1"]]

[[mount path="guide" source="../guide" nav-order="2"]]

[[mount path="design-notes" source="../specs" nav-title="Design Notes" nav-order="3"]]

[[mount path="changelog" source="../../CHANGELOG.md" nav-order="4"]]

::: header {.topbar}
[[pages depth="1"]]
:::

[[content]]
````

`[[mount]]` places a source in the site. It renders nothing, and where it
sits in the shell does not matter. A `[[mount]]` in a page's own body
warns and stays as written, as `[[content]]` does.

| Attribute | Meaning |
|---|---|
| `source` | A directory or a `.md` file, relative to the shell, or absolute. Required. |
| `path` | Where the source appears in the site, with no leading or trailing slash. A file source names a page (`.md` is implied and the page is written as `.html`); a directory source names a directory. Absent, the mount is the site's root. |
| `exclude` | Comma-separated directory prefixes and name globs, as `--exclude`, but **relative to this mount's source**, so `exclude="site"` skips the source's own `site` and not one three levels down. |
| `nav-title` | The mount's label in the tree. |
| `nav-order` | The mount's position among its siblings. |
| `title` | The title of the mount's directory page when md2html writes one (§5). |
| `nav-sort` | How a directory mount's children are ordered: `links` follows its index's links ([Order by Links](2026-10-16-order-by-links-design.md)). It applies to the mounted directory and to every directory below it that does not set its own. |
| `toc` | `page` (the default) leaves a page's own contents list alone. `shell` makes the shell's `[[toc]]` the page's only list (§6). |

A directory is mounted whole: every `.md` below it is a page at its
relative path under `path`, laid out as site mode lays out any directory,
with `index.md`, `nav-title:`, `nav-order:` and `nav: hidden` working
inside it as they do today. A mount's `nav-title` and `nav-order` are its
own, so a tab is labelled and placed without front matter written into a
page the site does not own. Where the mounted page or directory index sets
the same key, the mount wins.

**Pages follow the site, not the disk.** A page is written at its site
path: the `guide` mount above writes `docs/guide/sites.md` to
`guide/sites.html`. Moving a mount in the shell moves its pages' URLs.

### The root {#root}

The mount with no `path` is the site's root: its page is the home page,
written as `index.html`. A file source is the home page itself; a directory
source contributes its whole tree at the top, with its own directory page
(§5) as the home page. A root is required: a shell whose mounts include
none is refused, naming the problem, and so is a shell with two.

### Nesting and hoisting {#nesting}

- **A path inside another mount's path nests.** `path="handbook/changes"`
  sits inside a `handbook` mount. A segment of a path that no mount
  provides, such as `handbook` with no `handbook` mount, is a directory
  with a page md2html writes (§5).
- **A source inside another mount's source is hoisted.** With `docs`
  mounted at the root and `docs/specs` mounted at `design-notes`, the files
  of `docs/specs` appear at `design-notes` alone: the deeper mount takes
  them out of the enclosing one, with no `exclude` needed.
- **Mounts share directories.** A directory is a place in the tree, not
  something one mount owns. Two mounts may provide pages in the same
  directory, as a root mount and a mount at `guide` do, or a mount at
  `reference` beside a root that has pages of its own. Their pages and
  subdirectories are merged into one directory of the tree, and the
  directory's page (§5) is the first of its own `index.md`, `README.md`, or
  a written page, from whichever mount provides one. When one mount
  provides the directory's `index.md` and another its `README.md`, the
  `index.md` is the page and the `README.md` is an ordinary page inside the
  directory.
- **A shared directory has one set of settings.** Two mounts at the same
  directory path may both set `nav-title`, `nav-order`, `nav-sort` or `toc`
  only to the same value. Two values for one directory are an error naming
  both mounts, never a silent winner.
- **A site path claimed twice is an error**, naming both sources, never a
  silent winner. The path is a page's, or a directory's own page: two mounts
  that both provide `guide/index.md` conflict, and two that provide
  different files in `guide/` do not. So is one source mounted twice.
- **A source that is missing, excluded, or a file that is not Markdown is
  an error** at startup, not a build with a hole in it.

### The command line {#cli}

A shell with mounts is the whole description of the site:

```bash
md2html --site docs/site/shell.md -o _site
```

Entries given as arguments are refused beside a shell with mounts, since
they would be a second account of what the site holds. `--depth` and
`--link-depth` are refused too: a mount's pages are all built, and links
leaving the mounts are not followed (§4). `--exclude` still applies to
every mount, relative to the source root (§4). A shell without mounts
builds as today, from its entries.

**`--mount` adds a mount for one build.** Its value is the marker's
attributes, written the same way, and repeatable:

```bash
md2html --site docs/site/shell.md -o _site \
  --mount 'path="team" source="notes" nav-title="Team" nav-order="9" exclude="drafts"'
```

A flag's `source` is relative to the working directory, as every path on
the command line is; a marker's is relative to the shell. Either may be
absolute, which is how a build driven by another program names its sources
without depending on where it runs. A shell that is committed beside its
sources should keep its markers' sources relative, since an absolute one
ties the shell to one machine. A flag's mount follows every rule a
marker's does, including conflicts: it adds to the
shell's mounts and never replaces one, so a path both claim is an error.
Mounts given only by flags, beside a shell with none, are the site's mounts
and the same refusals apply to entries. This is how one shell serves
builds that differ in what they include, such as a site for a wider
audience built from the shell alone and a team site built with a section
more, without writing a second shell.

## 4. Links and assets {#links}

Links are resolved against **source** paths and written against **site**
paths. A file is **in the site** when a mount holds it; in a site built
without mounts, when it is under the run's base. The **source root** is the
directory that paths in a `repo-url:` link and in `_assets/` are written
from. By default it is the deepest directory holding the shell and every
mount's source, which for a repository's documentation is the repository's
root, found without md2html knowing about git.

That default moves when a source is somewhere else: a mount of generated
pages written to a scratch directory outside the repository would make the
deepest common directory the filesystem's root, and every path written
from it would carry the building machine's own. A build that cannot rely
on the default says where the root is, with `source-root:` in the shell's
front matter, relative to the shell, or `--source-root DIR`, relative to
the working directory, which wins over the key.

A build that forgets the setting must not leak paths quietly, so a default
that comes out as the filesystem's root is refused, asking for
`source-root:`. An explicit source root may leave a mount's source outside
it: that mount is still built, but a file near it that the site does not
hold, an image or a link's target, has no path from the root, and is
treated as one that climbs above it (links leaving the site, below).

The source root is also the bound for what a page reads while it is built
**in a build with mounts**. A chart's `data: {url}` and a front matter
`@file#key` reference are read then and their contents written into the
page, so nothing is copied for them, but such a build reads no file outside
its source root. A reference that would is handled as an unreadable one is
today, with a warning and the build carrying on: the chart shows as a code
block of its spec, and the `@file#key` value is empty, so its `@` text
never reaches the page.

A build without mounts reads as it does today: a chart's data stays
inside the run's base, and `@path#key` has no bound, so it may be absolute
or contain `..`, and a site whose shell reads a metadata file from above
its base keeps working unchanged. The bound is part of what a mount is
for: what a mounted site publishes comes from inside its source root, as a
chart's data already comes from inside a run's base. A build that does not
use mounts has not asked for it. A shell's own front matter is read the
same way as a page's, so under mounts a shell's reference to a file above
the source root is empty, with one warning under the shell's path, and the
fix is a source root that holds the file.

Styles and plugins are outside the bound. The shell's `plugins:`,
`--plugin` and `--css` configure the build rather than supply a page's
content, so they are read wherever they are, as today, and a shell whose
style lives outside the source root keeps it.

### Pages {#links-pages}

- A link from one page to another page in the site resolves by where the
  files are on disk, then is rewritten to where the target appears in the
  site. This holds across mounts, so the README's `./docs/guide/index.md`
  reaches `guide/index.html`, and a design note's `../../CHANGELOG.md`
  reaches `changelog.html`.
- A fragment is kept, and is not checked, as today.
- A `.md` outside the site is never built. A site build writes nothing
  under `_external/`, with or without mounts.

### Assets {#links-assets}

A site build copies assets into the output, and a site build alone: a
build without a shell links assets back to their sources as before.
`--no-assets` stops all copying: nothing is written beside a page or
under `_assets/`, and every link that would have led to a copy is left as
written, as `--no-assets` leaves links to files that are not Markdown
unrewritten today, so a build that serves its files some other way keeps
doing so. Today that covers an `<a>` to such a file as well as an image,
and under a shell it still does: an `<a>` to a file in the site is left as
written, not copied. A link a reader follows out of the site is not a copy,
so it still follows `unmounted:` (below), and `--no-assets` never turns
off `unmounted: error`.

An asset is anything a page loads: the `src` of an `<img>`, `<script>`,
`<source>`, `<video>`, `<audio>`, `<iframe>` or `<embed>`, each candidate
in an `srcset`, a `<video>`'s `poster`, and a `<link>`'s `href`, such as a
stylesheet. Raw HTML in a page counts, since the page is parsed whole. An
`<a>`'s `href` is a link a reader follows, not an asset: when it reaches a
page in the site it is a page link (above), when it reaches another file in
the site that file is copied as an asset, and when it leaves the site the
rules below apply. Today's link rewriting finds every one of these but
`srcset` and `poster`, which it learns.

- **An asset in the site** is copied beside its page's place in the site,
  at the same relative path, so `docs/guide/img/flow.png` linked from
  `docs/guide/diagrams.md` is written to `guide/img/flow.png`. Only assets
  a page links are copied.
- **An asset a page displays from outside the site**, an image's `src`, is
  copied to `_assets/` at its path from the source root: `logo.png` beside
  the root README is written to `_assets/logo.png`. Dropping its link
  would leave a broken image, and the path from the source root cannot
  collide with another copy or carry the machine's own paths.
- **An asset from outside the source root** has no such path. Its element
  is dropped with a warning, keeping an image's `alt` text, or the build
  fails under `unmounted: error`.

### Links leaving the site {#links-leaving}

A link a reader follows (an `<a>`'s `href`) to a file outside the site,
such as the README's `./LICENSE`, a design note's `../../site.go` or an
unmounted `.md`, cannot reach a page in the site. The shell's front matter
says what it becomes:

| `unmounted:` | Effect |
|---|---|
| `link` | Rewritten to the file's page on the repository's host, from `repo-url:`. The default when `repo-url:` is set. |
| `unlink` | The link is removed and its text kept, with a warning. The default without `repo-url:`. |
| `error` | The build fails, naming the page and the link. |

`repo-url:` is a URL with a `{path}` placeholder, filled with the file's
path from the source root, with `/` separators:
`https://github.com/AdamF-G/md2html/blob/main/{path}` turns `./LICENSE`
into `…/blob/main/LICENSE`. `unmounted: link` without `repo-url:` is
refused at startup. A link that climbs above the source root has no path
to fill in, so it is unlinked with a warning, or fails under `error`.

The shell's own links are resolved from the shell, as today, and follow
the same rules.

## 5. Directory pages {#directories}

Site mode gives a directory without an `index.md` a label and a warning.
For a directory a mount brings in, the warning is not actionable: the
directory is not the site's to add files to. In a site build, a directory
gets a page from the first of these it has:

1. **Its `index.md`**, as today.
2. **Its `README.md`.** A hosting page shows a directory's README as its
   index, so the README is the directory's page, and is not listed again as
   a page inside it.
3. **A page md2html writes.**

A written page has:

- **A title.** The mount's `title` for a mount's own root, else the
  directory's name in title case: `my_dir` and `my-dir` both read
  `My Dir`. The shell's `title-case:` lists words that keep a fixed
  spelling, as `title-case: API, CLI, SONiC`, matched without regard to
  case. The list is empty by default, so md2html carries no vocabulary of
  its own.
- **A body.** The title as a heading, then the directory's pages and
  subdirectories as a list of links, each labelled and ordered as the tree
  labels and orders it, with `nav: hidden` pages left out. It is drawn by
  `[[pages from="page" depth="1"]]`, a new `from` value that starts at the
  current page and is useful to any index page.

A written page is an ordinary page of the site: in search, in the tree and
in previous and next.

This applies to every site build, mounted or not, so the warning for a
directory without an `index.md` goes: such a directory now has a page.

## 6. A mounted page's own contents list {#toc}

A shell with `[[toc]]` draws the contents list of the page beside it. A
page of the build's own often has no list of its own, and one that does has
chosen to, so by default nothing changes: a page with its own `[[toc]]`
under such a shell shows both, and `toc: float` warns and is drawn inline.

A mounted directory is different. Its pages were written for a bare reader,
on a hosting page that builds its own list from `[TOC]` or shows none, and
the site's author cannot edit them. Such pages carry the marker, or
`toc: float`, because that is right where they are read. A mount with
`toc="shell"` makes the shell's list the page's only list:

- A `[[toc]]` or `[TOC]` marker in the page's body, outside a code span or
  fence, is dropped.
- The page's `toc:` front matter is ignored, so `toc: float` and
  `toc: inline` do not warn.
- `toc: none` is honoured, as it is on every page (below).

`toc="shell"` on a mount under a shell without `[[toc]]` is refused at
startup, since it would leave the pages with no list at all. Pages of
mounts without it, and every page of a build without mounts, are
untouched, so the option is the author's explicit choice and the default
stays what it was.

**`toc: none` reaches the shell.** Today a shell's `[[toc]]` lists every
page's headings whatever the page's front matter says, and `toc: none`
only stops `--autotoc` from adding a list. Under a shell, a page with
`toc: none` now gets no list from the shell either, on every site, mounted
or not, so a page such as a home page can opt out of the sidebar's list.

## 7. What stays outside {#outside}

- **Generated pages.** A build tool that writes pages from code or data
  (reference pages, command listings) still writes them. They are written
  to a scratch directory and mounted like any other source. A scratch
  directory holding the generated pages and a home page can be the root
  mount, with the hand-written directories mounted beside it at their own
  paths (§3, nesting).
- **Which mounts to use.** md2html adds no conditional markers. A build
  that differs in what it includes adds mounts with `--mount` (§3). Text on
  a page that mentions a section only when the section exists, such as a
  home page that lists a team-only area, is the page's own: two pages, or a
  step that picks one.
- **Ordering by an index's links** is independent of mounts, and has its
  own note: [Order by Links](2026-10-16-order-by-links-design.md).
- **Leak scanning, banners, version pages.** These are the project's own
  policy over its own content.

## 8. Open questions {#open}

- **Every asset under a mount.** Only linked assets are copied. A mount
  option to copy every file under its source, for downloads no page links,
  waits for a use.
- **Replacing a mount from the command line.** A `--mount` that claims a
  path the shell's mount claims is an error. A build that needs to swap a
  source for another at the same path has no way to say so; it waits for a
  use.

## 9. Testing {#testing}

- **Placement.** A directory mount and a file mount write pages at their
  site paths, in the tree, in search and in previous and next. A nested
  mount sits inside its parent, and a path segment no mount provides gets
  a written page.
- **The root.** A file root and a directory root each give `index.html`,
  and a shell with none or two is refused.
- **Hoisting.** A source inside another mount's source appears at its own
  path alone.
- **Shared directories.** A root mount and a mount at `guide` merge into
  one `guide` directory, with the directory page from whichever provides
  one, and an `index.md` from one mount makes another's `README.md` an
  ordinary page. Two mounts that provide the same page conflict, and two
  that provide different pages in a directory do not. Two mounts at one
  directory with different settings stop the build, and the same setting
  twice does not.
- **Conflicts.** A path claimed twice, a source mounted twice, and a
  missing, excluded or non-Markdown source each stop the build, naming the
  sources.
- **`--mount`.** A flag's mount is placed, merged and checked as a marker's
  is, its `source` is read from the working directory or taken as given
  when absolute, and one that claims
  a path the shell claims is an error. Mounts only from flags, beside a
  shell with none, make the site.
- **Source root.** The default is the deepest directory holding the shell
  and the sources, `source-root:` and `--source-root` move it, and the flag
  wins. A default at the filesystem's root is refused. A link to a file
  outside the source root is unlinked, and an image from outside it is
  dropped with a warning. In a mounted site, a chart whose data file is
  outside the source root warns and shows as a code block of its spec, and
  an `@file#key` reference outside it, the shell's included, warns and is
  empty, the shell's once; the build carries on. A build without mounts
  reads them as it does today, where a chart's data stays inside the run's
  base. A shell's style or plugin
  outside the source root is read in a mounted site.
- **`toc="shell"`.** A mounted page's `[[toc]]` and `[TOC]` markers are
  dropped and its `toc: float` does not warn, a marker in a code fence is
  untouched, and the option is refused under a shell without `[[toc]]`. A
  page of a mount without it is untouched.
- **`toc: none` under a shell.** A page with `toc: none` gets no list from
  the shell's `[[toc]]`, in a site with or without mounts.
- **Per-mount excludes.** `exclude="site"` skips the source's own `site`
  and leaves a deeper directory of the same name.
- **The command line.** Entries, `--depth` and `--link-depth` beside a
  shell with mounts are refused.
- **Links.** A link between two mounts, a link within one, a fragment, and
  a link from a nested directory up past its mount's root each resolve to
  the right site path.
- **Assets.** An asset in the site is copied beside its page, an image from
  outside it to `_assets/`, an unlinked asset is not copied, and a build
  without a shell copies nothing, and `--no-assets` copies nothing under a
  shell either, leaving those links as written while a link out of the
  site still follows `unmounted:`. A raw HTML `<img>`, each `srcset`
  candidate, a `poster` and a `<link>` stylesheet are copied, and an `<a>`
  to a file in the site copies the file.
- **Links leaving the site.** `link`, `unlink` and `error` each behave as
  stated, `link` without `repo-url:` is refused, and a link above the
  source root is unlinked. No site build writes under `_external/`.
- **Directory pages.** `index.md` first, then `README.md`, which is not
  listed twice, then a written page with a title-cased name, `title-case:`
  spellings, a hidden page left out, and the page found by search.
- **Nothing changes without a shell.** A build without `--site` is
  byte-for-byte what it was.
