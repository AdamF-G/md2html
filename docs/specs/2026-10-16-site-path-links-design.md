# Site-Path Links — Design

**Date:** 2026-10-16
**Author:** AdamF-G
**Status:** Draft for review.

## Purpose

[Mounts](2026-10-16-mounts-design.md) separate where a file lives from where
it appears in the site. A link between two pages is still resolved by where
the files live: `../cli/run.md` is followed from the linking page's
directory on disk, and if no file is there the link is reported as a target
that does not exist.

That is right for a link written for the source, one that works on the
repository's hosting page and in an editor. It is wrong for a link written
for the site, to a page that exists only because something generates it.
Such a page is written to a scratch directory and mounted
([Mounts](2026-10-16-mounts-design.md), §7). It appears at a path in the
site, but there is no file at that path on disk for the link to reach, so a
site that links its hand-written pages to its generated ones, and its
generated pages back to the hand-written ones, reports every one of those
links.

This note lets a link in a mounted site that reaches nothing on disk be
read against the site's own tree.

## 1. An example {#example}

A project hand-writes guides and generates its reference pages from code.
The generator writes them to a scratch directory, and the shell mounts both:

```text
repo/
├── docs/
│   └── site/
│       ├── shell.md
│       ├── index.md
│       └── guides/
│           ├── deploy.md        links to ../reference/index.md
│           └── tuning.md        links to ../reference/engine.md#options
└── build/
    └── scratch/                 written by the generator
        └── reference/
            ├── index.md
            └── engine.md        links to ../guides/tuning.md
```

```markdown {caption="docs/site/shell.md"}
---
site: true
---
[[mount source="."]]

[[mount path="reference" source="../../build/scratch/reference"]]
```

In the site the two sit side by side:

```text
index.html
guides/deploy.html
guides/tuning.html
reference/index.html
reference/engine.html
```

`guides/deploy.md` says `[the reference](../reference/index.md)`. Read from
`docs/site/guides/` on disk that is `docs/site/reference/index.md`, which
does not exist: the page is at `build/scratch/reference/index.md`, and
`docs/site/reference/` is a place that only the site has. Likewise
`engine.md`'s `../guides/tuning.md` is `build/scratch/guides/tuning.md` on
disk, and the guide is in `docs/site/guides/`.

Read from where each page appears, both links are right: from `guides/deploy`,
`../reference/index.md` is `reference/index.md`, a page the site has.

The same shape occurs without generation whenever mounts rearrange sources
so that two pages are neighbours in the site and not on disk: a root README
and a directory mounted beside it, or a design note mounted under a section
it does not sit in.

## 2. Other ways {#alternatives}

- **Generate into the tree.** When the generator can write its pages where
  they appear in the site, such as a `docs/site/reference/` the repository
  ignores, disk and site agree, the links above resolve on disk, and
  nothing here is needed. That is the simplest arrangement and stays the
  recommended one. It is not always available: a build that runs in a
  sandbox, or writes only to a temporary directory, or one generator that
  feeds several sites with different layouts, cannot put its output
  inside the sources. Those are the builds mounts let read pages from a
  scratch directory, and the builds this note is for.
- **Link to the scratch directory on disk.** A hand-written page can say
  `../../../build/scratch/reference/index.md`, which resolves while the
  build runs. It ties the hand-written pages to the build's layout, breaks
  as soon as the scratch directory moves, and is no better on the hosting
  page, where `build/` does not exist. The generated pages would have to
  do the same in reverse.
- **A site-rooted link syntax.** A link could name its target by site path
  explicitly. A leading `/` is taken: md2html reads it as an absolute path
  on disk, and a hosting page reads it as the repository's root. A scheme
  such as `site:reference/engine.md` is free, and says what the author
  meant, but it is new syntax, it never works outside the site, and it
  cannot help a link that is right in both places because disk and site
  happen to agree there. The rule below reads ordinary relative links, so
  a link written relative to its neighbours works wherever they are
  neighbours.

## 3. Principles {#principles}

- **A link written for the source keeps its meaning.** If the file is on
  disk, the link means that file, as today, unless the site excludes it.
- **A link that reaches nothing on disk may be a link written for the
  site.** It is read in the site's tree before it is called broken.
- **An excluded file is not what the site means.** Excluding a file says
  it is not in the site, so a link that reaches one is read in the site's
  tree, as one that reaches nothing is.
- **The site's tree is the mounts' tree.** A page's place is where the
  mounts put it ([Mounts](2026-10-16-mounts-design.md), §3), not where its
  source is.
- **Nothing is hidden.** A link that reaches nothing either way is reported
  as today.

This narrows a principle of [Mounts](2026-10-16-mounts-design.md) (§1),
that a link that cannot work in the site is reported: a link that works
only in the site is now one that works, and is not reported, though it
does not work on the hosting page (§5).

## 4. The rule {#rule}

For a relative link to a page, in a mounted site's build:

1. **On disk.** Resolve the link from the linking page's source directory.
   When a file or directory exists there and is not excluded, that is the
   target, whether or not the site holds it. A file the site does not hold
   follows `unmounted:` ([Mounts](2026-10-16-mounts-design.md), §4), and
   so does a directory the site does not hold (§4, directories); when the
   site has a page at the link's site path, the report names it (§5,
   shadowed).
2. **In the site.** When nothing is there, or what is there is excluded,
   by `--exclude` or by the mount whose source holds it, resolve the link
   from the linking page's **site path**, the directory its output is
   written in, and look for a page at that site path:
   - `x/y.md` is found when a mount provides a page that appears at `x/y`.
     A `.md` in the link is the page's own name, as it is on disk.
   - `x/index.md`, `x/`, and `x` each name the directory `x`'s page, when
     the site has the directory: its `index.md`, its `README.md`, or the
     page md2html writes ([Mounts](2026-10-16-mounts-design.md), §5).
     `x/README.md` names the same page when that README is the
     directory's page, and the README as an ordinary page when an
     `index.md` is.
3. **Neither.** The link warns as today, when nothing is on disk. An
   excluded file that step 2 finds no page for is outside the site and
   follows `unmounted:`, as today.

A link found in step 2 is rewritten as any link to a page in the site is:
to the target's output, from the linking page's output. Its fragment is kept
and is not checked, as today.

A path in the site is claimed by one page at most
([Mounts](2026-10-16-mounts-design.md), §3), so step 2 has one answer and
cannot be ambiguous.

A link whose site path climbs above the site's root reaches nothing, and
warns. An absolute link is a path on disk and takes step 1 only.

### Where it applies {#scope}

Step 2 applies to every page of a mounted site: a page of any mount,
including one a mount provides from a scratch directory, whichever way the
link points. It does not depend on which mount holds the linking page, or
the target.

The shell is not a page and has no site path of its own. Its links are
resolved from the shell on disk, as today, and in step 2 from the site's
root, which is where a shell's link written for the site would be read
from: `reference/index.md` in the shell's header reaches the reference
section's page from every page the shell is drawn around.

A mounted crawl knows every page and its site path before it reads a
link, since it follows none, so one table of the site's pages, and of its
directories' pages, by site path, is built from the mounts before any link
is read. The crawl reads it where it reports a link that reaches nothing
on disk, so such a link either is rewritten or warns once, in the order of
the page's links. The rewriting of links reads the same table for a link
that reaches an excluded file and for the shadowed-link report (§5), so
it is built once.

A build without mounts is unchanged, with or without a shell. Its tree is
its base's layout on disk, and its pages are found by following links, so
it does not know its whole tree while it reads them. The one page step 2
could add there is a directory's written page, reached by `sub/index.md`
in a directory with neither `index.md` nor `README.md`. A link to `sub/`
already reaches it, so that case is left to warn.

### Directories {#directories}

A link a reader follows to a directory the site holds reaches the
directory's page, as today. A link to a directory on disk that the site
does not hold is left as written today, with nothing said, so in a mounted
site it points outside the output and breaks unreported. In a mounted site
it now takes the rule as a file does: an excluded directory is read in the
site (step 2), and any other follows `unmounted:`, with `repo-url:` filled
with the directory's path, and the shadowed-link report when the site has
a page for the directory at the link's site path. A build without mounts
leaves such a link as written, as today.

Assets are not covered. An image or other file a page loads is resolved on
disk only ([Mounts](2026-10-16-mounts-design.md), §4); a page that loads a
generated file keeps it beside the page, in the same scratch directory.

## 5. Trade-offs {#tradeoffs}

- **The link breaks on the hosting page.** A link that reaches only the
  site's tree has no file to open on a repository's hosting page or in an
  editor. That is inherent: the page it names exists only in the built
  site. The rule does not ask authors to write such links, and it does not
  make them work elsewhere; it stops the site from reporting links that are
  right in the site.
- **A link broken on disk can be found in the site, and go unreported.**
  A hand-written page whose link stopped resolving on disk, because its
  target moved or was deleted, warns today, and the warning is how the
  author learns the link is broken on the hosting page too. When the
  site happens to have a page at the link's site path, the link now
  reaches it and nothing is said. That needs a page at exactly the path
  the broken link names, from where the linking page appears, which in a
  site whose layout follows the disk is the page that moved there, and in
  one that does not is a coincidence. It is accepted as rare. A link that
  matches nothing in the site, the usual result of a typo or a deletion,
  still warns.
- **Step 1 wins over step 2, unless the file is excluded.** When a file
  exists on disk at the path and the site has a different page at the
  same relative site path, the link goes to the file. Such a link is
  written for the source, and is read as one.
- **An excluded file gives way to the site's page.** A source a mount
  excludes because a generator replaces it, such as a directory of
  templates beside the pages generated from them, is on disk where the
  generated pages appear in the site. A link to a template then reaches
  the template on the hosting page and the generated page in the site,
  which is right in each place: the author excluded the template, so a
  site reader sent to its source on the repository's host is in the wrong
  place. The cost is that excluding a file can change where a link to it
  goes, which is what excluding it says.

### A shadowed link {#shadowed}

A link that step 1 sends outside the site, to a file or directory that is
not excluded, while the site has a page at the link's site path, is the one
case where the two readings disagree and the reader leaves the site for
it. Such a file is one no mount holds, such as a stale copy left in a
scratch directory beside the one the shell mounts. The link does what
`unmounted:` says, and its report, under every setting, adds one clause
naming the page the site has:

```text
unlinked ../reference/engine.md, which is outside the site; its text is
kept; the site has a page at reference/engine, which the file shadows
```

Under `unlink`, as above, and under `error`, where the build fails, the
clause is added to the report that setting already makes. Under `link`,
which today rewrites the link and says nothing, it is a new warning, and
the link still goes to the repository's host, so a link that is right on
the hosting page is not made to fail:

```text
linked ../reference/engine.md to the repository's host, since the file is
outside the site; the site has a page at reference/engine, which the file
shadows
```

The fix is the author's, and either one makes the link reach the site's
page: move or rename the shadowing file, for example `engine.md` to
`engine.md.tmpl`, so that nothing is on disk at the link's path, or
exclude it, so that step 2 reads it in the site.

## 6. Testing {#testing}

- In a site with a root mount and a mount of a directory from outside the
  root, a link from a root page to a page of the other mount, and one the
  other way, written for the site paths, resolve and are rewritten to the
  target's output.
- The same links in a build without the second mount warn, naming the link.
- A link that exists on disk and is in the site is unchanged, and a link
  that exists on disk and is not in the site follows `unmounted:`, as before.
- `x/index.md`, `x/` and `x`, with nothing at `x` on disk, each reach the
  directory's page in the site, whether it is the directory's `index.md`,
  its `README.md`, or a page md2html writes; and `x/README.md` reaches the
  README in both of its roles.
- A link in the shell written for the site resolves from the site's root,
  and is rewritten from each page the shell is drawn around.
- A link to a site path no mount provides warns, and so does one that
  climbs above the root, and an absolute link that reaches nothing on disk.
- A fragment on a link found in the site is kept and not checked.
- A link to a file excluded by its mount, or by `--exclude`, at a site
  path a mount provides, reaches that page; the same link with no page at
  that site path follows `unmounted:`, as before.
- A link to a file on disk that no mount holds and nothing excludes, at a
  site path a mount does provide, warns with the shadowed-link clause under
  `unlink`, warns with it and still links to the host under `link`, and
  fails with it under `error`; the same link with no page at that site path
  reports as before, without it.
- A link to a directory the site does not hold follows `unmounted:` in a
  mounted site, reaches the site's directory page when it is excluded, and
  carries the shadowed-link clause when it is not and the site has a page
  for it; in a build without mounts it is left as written.
- A build without mounts, with or without a shell, is unchanged: a link to
  `sub/index.md` in a directory with no index still warns.
