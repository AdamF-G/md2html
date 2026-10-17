# Follow-ups from Order by Links, Site Links, Mounts and Directory Pages

Findings the reviews of four plans graded minor and deferred, kept for a
later fix pass. Each says what a reader or user meets; none blocked the
work it was found in. Tick one off by fixing it, or by deciding it stands
and saying why.

- Plans: [Order by Links](2026-10-16-order-by-links.md),
  [Site Links and Assets](2026-10-16-site-links-and-assets.md),
  [Mounts](2026-10-17-mounts.md),
  [Directory Pages](2026-10-17-directory-pages.md)
- Spec: [Mounts](../specs/2026-10-16-mounts-design.md)

## Order by links (`nav-sort: links`)

- [x] A link to a subdirectory written as a directory, `[Sub](sub/)`, does
  not rank it; only a link to its `index.md` does (`site.go`,
  `rankByLinks`). Fixed by Directory Pages: a link to a directory reaches
  its page and ranks it.
- [ ] `rankByLinks` falls back to label order without a warning when it
  cannot re-read or parse the index.
- [ ] The ordering test does not pin a deep link into a subdirectory
  appearing before that subdirectory's own direct link (the behaviour is
  right; the test does not prove it).
- [ ] `sites.md` does not name `nav-sort: label` as the default, or say the
  key on a page that is not an index warns.

## Site links and assets: behaviour

- [ ] With `--no-md-links` and `repo-url:`, a `.md` link out of the site is
  left as written, while under `unlink` the same link is dropped: the drop
  is not gated by `--no-md-links` (`crawl.go`, `planLinks`).
- [ ] Under `unmounted: error`, the page-against-asset collision check is
  skipped once there are failures, so one run does not report every
  problem.
- [ ] A dropped image with an empty `alt` inside a kept `<a>` leaves an
  empty link (`transform.go`, `linkDrop`).
- [ ] An `<img>` or `<source>` whose every `srcset` candidate is dropped is
  left with no source at all.
- [ ] A rebuild reports an unchanged copy as `WriteOverwritten`; nothing
  shows it today, but a count of "overwritten" files would mislead.
- [ ] A ledger at the output root that is not md2html's is found only
  after the copies are made; a check before copying would refuse first
  (`assetcopy.go`).
- [ ] A file someone else put at a copy's destination that already holds
  the source's bytes is adopted into the ledger and later replaced; worth a
  line in the sites guide.
- [ ] Copies are written with `os.WriteFile`: not atomic, and it follows a
  symlink planted at the destination.
- [ ] `CopyAssets` does not guard two assets with one output path itself
  (the crawl refuses that case before it gets there).
- [ ] `CopyAssets`' check for an asset aimed at the ledger compares cleaned
  paths only, so a relative `Out` against an absolute ledger path would
  miss it.
- [ ] A page's link rewritten to `repo-url:` is not marked external
  (`target`, `rel`), while the same link in the shell is: the page's
  rewrite runs after the external-link transform (`md2html.go`).
- [ ] A query string is lost in a site's rewrites: `?plain=1` on a
  `repo-url:` link (needed for a line anchor on a `.md` file on GitHub),
  and a cache-buster on an in-site stylesheet. Keep `?query#fragment`, not
  only the fragment (`sitelinks.go`).
- [ ] A hand-edited copy is refused as "not copied by md2html"; it was,
  then edited. Say "changed since md2html copied it, or not md2html's"
  (`cmd/md2html/main.go`).
- [ ] A file in the site named `md2html-search.js` is refused on every
  build rather than caught as a collision with the search index up front.

## Site links and assets: messages

- [ ] A `source-root` error prints the joined absolute path, not the value
  as written in the shell.
- [ ] The "site shell: …" error does not name the setting it came from.
- [ ] `siteRules.outDir` is stored as given; `Crawl` makes it absolute
  first, but a direct caller could pass a relative one.

## Site links and assets: tests

- [ ] `LinkDrop`: an `<a>` around a dropped `<img>`, an `srcset` fully
  dropped, an `srcset` beside a dropped `src`, a node with no parent.
- [ ] The shell's warning-once test discards per-page warnings and renders
  one page, so a per-page warning would pass it; nothing tests that a shell
  link the rules leave as written is still rebased per page, or that a
  second page gets its own relative path to the shell's copied logo.
- [ ] The CLI: that a refused file is left untouched; a ledger that cannot
  be written; a site built in place; `--no-assets` under `--site`; that the
  ledger lists a copy after a rebuild.

## Site links and assets: docs and code

- [ ] `sites.md` does not say that `unmounted: link` without `repo-url:`,
  or a `repo-url:` without `{path}`, is refused, or that `unmounted: error`
  also stops the build for an image from outside the source root.
- [ ] `sites.md` puts the note about a site built in place at the end of
  "Links and files", not where a reader meets the rules.
- [ ] The quick reference shows `repo-url:` but not `unmounted:` or
  `--source-root`.
- [ ] One long, unwrapped line in `sites.md` ("On the root's `index.md` it
  hides only that page."), older than this work.
- [ ] `ExtractLinks` seeds a slice and then resets it for `srcset`; an
  `if`/`else` would read better (`links.go`).
- [ ] A link test keys its map by attribute and href, so two equal links
  would collapse into one (`links_test.go`).
- [ ] `linkDrop` skips a node with no parent without saying that cannot
  happen for parsed content.

## Mounts: behaviour

- [ ] `path=".md"` on a file mount trims to "" and silently becomes the
  root (`mounts.go`).
- [ ] A `--mount` value with no space between attributes is accepted, and
  a repeated attribute silently takes the last value.
- [ ] Two file mounts at one path pass `checkMounts`; the crawl's output
  collision check stops them later, naming the sources but not the
  `[[mount]]` declarations.
- [ ] An empty root mount, or an empty directory mount whose path another
  mount shares, does not warn "no pages": the warning fires only when the
  mount has no node in the tree (`site.go`, `applyMounts`). A directory
  mount whose pages all go to deeper mounts does warn.
- [ ] A non-site build over a tree holding the shell warns once per
  `[[mount]]` marker, not once.
- [ ] A missing reference target reached through a symlinked directory
  leading outside the root warns "cannot be read" rather than "outside
  the source root" (compared lexically; nothing leaks).
- [ ] With `--mount`, a bad shell now preempts the `--toc` float and
  `--fragment` refusals.
- [ ] `Convert` deletes `meta["toc"]` from the map later read for search
  facets (harmless unless `toc` is a facet).

## Mounts: code

- [ ] `place()` uses `path.Ext` on an OS path; `filepath.Ext` is right
  (wrong only on Windows with a dot in a directory name).
- [ ] `Crawl` discards `relUnder`'s ok without a comment.
- [ ] `Site.dirs` says "while the tree is built" but the map is kept.
- [ ] The reference reader's outside-the-root branch duplicates the
  unreadable branch's two lines.

## Mounts: tests

- [ ] `parseMount`: toc/title/nav-sort/nav-order conflicts, an error
  from `--mount`, a flag-only site, `path=".md"`.
- [ ] `buildTree` warns and skips a page with an empty `SitePath`.
- [ ] `TestMountCrawlSymlinks` runs a throwaway first crawl and does not
  assert the out-of-mount warning or that a link to the alias reaches
  `design-notes/a.html`.
- [ ] A link to the shell, or to a mount-excluded page, is left as written
  (the hoist test's comment overstates).
- [ ] A mount's `nav-title` beats the page's own title and `nav-title`; a
  doc-less directory keeps a mount's `nav-title` through `finish`;
  `toc="shell"` with a shell `[[toc]]`, and with `toc: none`; `toc: none`
  under a shell in a site without mounts.
- [ ] A symlinked reference escaping the root; the shell's reference
  warning said once.
- [ ] The CLI: a comma inside `--mount` is kept (`exclude="a,b"`); `--site`
  with no mounts and no entries prints the usage.

## Mounts: docs

- [ ] "Two mounts may share a directory" could read as one source mounted
  twice.
- [ ] The Mounts table does not say `exclude` and `nav-sort` are
  directory-only, or that a file mount with no path is the home page.
- [ ] No back-reference to `source-root:` in Mounts; "The tree" and "Links
  and files" still describe the root and "in the site" by the base alone.
- [ ] The guide does not say `--mount` needs `--site`.
- [ ] A `title` holding `*`, `<` or `&` makes the whole `[[mount]]` marker
  drop out silently (older than Directory Pages, found in its review).

## Directory pages: behaviour

- [ ] A written page carries its generated Markdown, the `[[pages …]]`
  marker, as its source wherever a page's source is shown.
- [ ] CLI messages about a written page name its `Src`, a file that does
  not exist.
- [ ] A directory named `[draft]` gets a page `<title>` of "index":
  `extractTitle` strips chip tokens from the raw source (as for any page
  so titled).
- [ ] `[[pages from="page"]]` on a page with no node, such as the shell's
  own source, falls back to the whole tree.
- [ ] Under `nav-sort: links`, a directory reached only by an image
  (`![l](lo/)`) is ranked, though the link itself is left as written; the
  rank should skip a loaded link, as `action` does.
- [ ] A query on a directory link is dropped (`sub/?x=1` becomes
  `sub/index.html`), as for file links above.
- [ ] Title case: an invalid UTF-8 first byte becomes U+FFFD;
  `unicode.ToUpper` where `ToTitle` is right for digraphs; a `title-case:`
  entry holding a separator (`e-mail`) can never match, and nothing says
  so.
- [ ] A `README.md` and `readme.md` in one directory fail with a
  different collision message than other pairs.

## Directory pages: code and tests

- [ ] The `hasIndex` loop is repeated in `buildTree` and `readmesAtIndex`;
  `rankByLinks` repeats `dirPage`'s place logic.
- [ ] No tests: a README at `index.html` colliding with a raw `index.html`
  asset or across mounts, or `readmesAtIndex` in a mounted site; a root
  README in the tree test; `from="page"` on a README page, nested depth,
  a hidden child; non-ASCII or empty items in `TestTitleCase`; directory
  links in a mounted, hoisted or excluded directory, or a loaded one.
