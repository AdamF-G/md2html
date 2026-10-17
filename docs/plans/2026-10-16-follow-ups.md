# Follow-ups from Order by Links and Site Links

Findings the reviews of two plans graded minor and deferred, kept for a
later fix pass. Each says what a reader or user meets; none blocked the
work it was found in. Tick one off by fixing it, or by deciding it stands
and saying why.

- Plans: [Order by Links](2026-10-16-order-by-links.md),
  [Site Links and Assets](2026-10-16-site-links-and-assets.md)
- Spec: [Mounts](../specs/2026-10-16-mounts-design.md)

## Order by links (`nav-sort: links`)

- [ ] A link to a subdirectory written as a directory, `[Sub](sub/)`, does
  not rank it; only a link to its `index.md` does (`site.go`,
  `rankByLinks`). Either count a link that resolves to a child directory,
  or say in `sites.md` that the index must be linked.
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
