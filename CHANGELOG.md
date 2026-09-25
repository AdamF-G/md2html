# Changelog

Notable changes to md2html, newest first. A version is a git tag, and the
same string is stamped into every generated file's provenance marker, so
the version a page claims and the version that wrote it are always the
same.

This is 0.x: the exported API is not stable yet, and a change marked
**breaking** can land in a minor release. From v1.0.0 on, one needs a new
major version instead.

## Unreleased

### Added

- **`--install-skill DIR`** installs the authoring skill into any agent's
  skills directory, not only Claude Code's. Nothing in the skill was ever
  specific to Claude Code; only the two install flags were, and they stay
  as shortcuts. Like them, it refuses a `DIR` that does not exist, naming
  it in full, rather than put the skill where no agent looks. It cannot be
  combined with either of them.

## v0.8.0 — 2026-09-25

### Added

- **Pages carry their Markdown.** Every full page now embeds its original
  source, byte for byte and front matter included, in a hidden
  `<textarea id="md2html-source">` after the content, labelled
  `data-format="text/markdown"` and
  `data-dialect="github.com/AdamF-G/md2html@<version>"`. A comment after the
  provenance marker says where it is. Whoever receives a page can pass the
  Markdown to their own agent, or convert it again. Read it through
  `textContent`, not `value`, which normalizes line endings.
  `--no-source` (`Options.NoSource`) leaves it out, and `--fragment` output
  never carries it. `SourceID` exports the element's id.
- **Copy and Download for the source.** A page that carries its Markdown
  shows two controls in the screen's bottom corner: Copy puts the source on
  the clipboard, and Download saves it as the page's name with `.md`. A
  floating contents list stops short of them, and they are hidden in
  print. Over plain `http` Copy falls back to a method that turns CRLF
  line endings into LF; Download is exact everywhere.

### Changed

- **A floating contents list starts on the left.** A page has no site
  navigation to hold that side, so the outline takes it, as Wikipedia and
  a word processor's outline pane do. The arrow moves it to the right, and
  that choice is remembered as before. The class the toggle sets is now
  `toc-right` rather than `toc-left`, so a replacement stylesheet that
  positioned `toc-left` needs the same change.

## v0.7.0 — 2026-09-25

### Added

- **A floating contents list.** `toc: float` in front matter, or `--toc float`
  (`Options.TOC`) for the whole run, pins the page's contents list beside the
  text column on a wide screen, where it stays in view while the page
  scrolls. An arrow in its corner moves it to the other side of the column,
  and the choice is remembered across pages. It sits clear of wide figures
  as well as the text. On a narrower screen, and in print, it is the inline
  list as before. The default is still `inline`, which front matter can also
  set to opt one page out of a floating run. Only a page's first list
  floats. `--fragment` output stays inline, since an Artifact carries no
  script. A value other than `inline` or `float` warns and is ignored, and a
  bad `--toc` exits with status 2 before anything is written. `IsTOCMode`
  exposes the check. The marker itself is unchanged, so `[TOC]` still works
  on GitLab.

### Changed

- **Contents list depth is shown by chevrons, not indentation.** Every
  entry now starts at the same edge. The page title and the top-level
  sections are bare, and each entry below them leads with one `›` per
  level of depth, up to three. Depth follows the outline, not the heading
  tag: a skipped level adds none, so an `h4` directly under an `h2`
  section gets one chevron, and a page titled with `##` over `###`
  sections reads like one titled with `#`. Each entry gains a
  `toc-d0`…`toc-d5` class for this, and the title `toc-title`; the
  `toc-h*` class is unchanged, so a replacement stylesheet that styled it keeps working. A
  long heading's wrapped lines hang in, so they no longer line up with, and
  read as, the next entry. The chevrons are hidden from screen readers.

## v0.6.0 — 2026-09-24

### Added

- **`stats`, `defs` and `group` container kinds.** They are the `fig` kinds
  of the same names as `:::` containers, so each entry can be block
  Markdown instead of one line of inline text, and they look the same. `stats`
  and `defs` hold a definition list. In `stats`, each term is a tile's value,
  its first definition the label and an optional second the detail line. Each
  term group is wrapped in `<div class="stat">` inside the `<dl>`. A `stats`
  or `defs` container with no definition list warns. The `fig` forms are
  unchanged. See
  [docs/specs/2026-09-13-standards-alignment.md](./docs/specs/2026-09-13-standards-alignment.md) §4.

- **The page language can be set.** A `lang:` front matter key sets a
  page's `<html lang>`. It's the key Pandoc reads for the same attribute.
  `--lang` (`Options.Lang`) sets a default for the whole run, which front
  matter overrides. Without either, pages are `en`, as before. A value not
  shaped like a language tag warns and is ignored, and a bad `--lang` exits
  with status 2 before anything is written. `IsLangTag` exposes the check.
  `--fragment` output has no `<html>` element and is unchanged.

- **The contents list has an accessible name.** `[[toc]]` emits
  `<nav class="toc" aria-label="Table of Contents">`, so screen readers can
  tell it apart from a hand-written `::: nav`. Nothing visible changes. A
  `toc-title:` front matter key, Pandoc's key for the same label, renames it.

- **Attributes on links and images.** Pandoc's `link_attributes`:
  `[text](url){#id .class key=value}` and `![alt](src){width=50%}` set those
  attributes on the `<a>` or `<img>`. The usual use is
  `aria-current=page` on a link in a `::: nav`. The block must touch the
  link, as in Pandoc. `href`, `src` and `srcset` can't be set this way and warn, and
  attribute names are checked the same way a container's are.

### Changed

- A quoted front matter value loses its quotes, as in YAML: `title: "Rollback"`
  used to put the quotes in the page title. `\"` and `\\` inside double
  quotes and `''` inside single quotes are unescaped.
- Off-site links keep an author's `rel` and `target`. `rel` gets
  `noopener noreferrer` added to what was written instead of replacing it,
  so `rel="me"` survives, and a `target` already set is left alone. This
  applies to raw HTML links too.
- A generated heading id now steers clear of an id set on any element, not
  only on other headings. A `{#notes}` container or link and a `## Notes`
  heading both used to get `id="notes"`; the heading is now `#notes-1`, so
  check in-page links meant for such a heading.
- A braced `::: {.stats}`, `::: {.defs}` or `::: {.group}` used to be an
  ordinary classed div. It is now the shipped kind: a `stats` one has its
  definition list regrouped into tiles, and all three pick up the default
  stylesheet's rules. The stylesheet now also styles the class names
  `stats`, `stat`, `defs` and `group` wherever they appear, so raw HTML such
  as `<div class="group">` that used them for your own purposes changes
  look too.

## v0.5.0 — 2026-09-24

### Added

- **A `details` container kind**, the bare collapsible. It renders as
  `<details class="container">`, and a titled block's `<summary>` is exactly
  its title, with none of the prefix or fallback wording `aside` and
  `example` add; a titleless one falls back to "Details".
- **`--exclude` accepts name globs.** A value containing `*`, `?` or `[`
  is matched, with Go's `filepath.Match` syntax, against every file and
  directory name below the base instead of being taken as a directory
  prefix. `--exclude 'AUDIT_*'` skips every document named that way at any
  depth, and prunes any directory named that way. A pattern never matches
  the base or a directory above it. A pattern containing a path separator,
  or a malformed one, is warned about and ignored.

### Changed

- **breaking:** heading ids follow the rules GitHub, GitLab and Pandoc's
  `commonmark_x` share, so in-page links written against any of them now
  resolve. Each space becomes a hyphen and runs are no longer merged
  (`Why Go / goldmark` is `why-go--goldmark`, not `why-go-goldmark`),
  underscores are kept (`foo_bar`, not `foo-bar`), leading and trailing
  hyphens are kept (`— Intro` is `-intro`), and a repeated heading is
  numbered from `-1` rather than `-2`. That last change moves a link
  rather than breaking it: an old `#setup-2` now lands on the third
  `Setup`, not the second, so check links to repeated headings. Headings
  with plain words and single spaces keep their ids. Explicit `{#id}`s are
  unaffected.
- The authoring guide said no other renderer builds a contents list from
  `[TOC]`. GitLab does, so it now recommends `[TOC]` for documents also read
  there.
- **breaking:** an `--exclude` value containing `*`, `?` or `[` used to be
  a literal directory path, and is now a name pattern. To exclude a
  directory whose name contains one of those characters, escape it with
  `\`; that matches the name at any depth below the base.
- The warning for a link into an excluded path now ends "excluded" rather
  than "excluded directory", since the match may be a file name.
- `--fragment`'s help text says what it emits — a bare fragment for a host
  such as a Claude Artifact — instead of "Artifact-shaped".

## v0.4.0 — 2026-09-14

### Added

- The `fig` fence can now draw annotated trees, glosses, footnotes,
  accents and wide figures. Everything below is additive: a figure that uses none of it
  emits the bytes it emitted before, apart from the panel classes under
  Changed. See
  [docs/specs/2026-09-14-fig-vocabulary-extensions-design.md](./docs/specs/2026-09-14-fig-vocabulary-extensions-design.md)
  and
  [docs/specs/2026-09-14-fig-nested-layouts-design.md](./docs/specs/2026-09-14-fig-nested-layouts-design.md).
  - **`tree`**, a new kind: an indented listing, one node per line.
    ` -- ` splits a label from a trailing note, `* ` accents a line and
    `- ` de-emphasizes it. Every sigil needs its space, so `*_test.go` stays
    a literal glob.
  - **`note` and `accent`** on `box`, `result`, `rail` and `group`: a
    quieter gloss beside the label, and a ring that marks an item out from
    its siblings.
  - **`foot`** on `group`, a trailing line after the group's items, and
    **`detail`** on a stat tile, a third, quieter line beneath its label.
  - **`wide: true`** lets a figure break out of the text column. It breaks
    out of the page's `<main>` measure only; inside a container or under
    `--fragment` it renders as an ordinary figure.
  - **`cols` and `split` as item kinds**, so a layout can sit anywhere an
    item can: context above a split and an outcome below it, a split inside
    one column, weighted columns inside a chain. A `split` item takes
    `boundary`, and a `cols` item's children take `weight`. A figure whose
    only item is a `cols` or `split` item warns and names the `layout:`
    spelling to use instead.

  A panel that needs a title, an accent or a footnote is a `group` inside
  it; panels themselves carry no metadata.

### Changed

- **Every `cols` and `split` panel now draws a card** — a border, padding
  and background around whatever sits in it — so existing figures change
  appearance without being edited. A panel whose item carries `accent`
  tints its card and gains the class `fig-panel-accent`; a panel holding an
  arrow draws no card and gains `fig-panel-arrow`. Those two classes are the
  only markup change to a figure written for v0.3.0. A browser without
  `color-mix()` draws the accented card untinted.
- The card's padding sits outside the width that weights share out, so
  weighted panels' outer widths no longer hold their exact ratio: a 3:1 row
  reads as roughly 2.8:1.
- A panel-level arrow in a `cols` row sits at the row's vertical middle
  rather than level with the cards' top edges.
- On a narrow screen, a `split`'s stacked panels span the column as a
  `cols` layout's do, instead of shrinking to their content.

### Repository

- The browser suite now checks figure layout that was previously left to
  the eye: card colours and contrast floors in both themes, the
  no-`color-mix()` fallback, arrow centring, panel stacking for every
  nested layout, and the whole figure gallery at desktop and phone widths.

## v0.3.0 — 2026-09-14

### Added

- Five Markdown syntaxes from the surrounding ecosystem are now accepted,
  all additive — nothing written for the older spellings changes meaning.
  See [docs/specs/2026-09-13-standards-alignment.md](./docs/specs/2026-09-13-standards-alignment.md)
  for the survey and the measurements behind them.
  - **Bracketed spans**, Pandoc's `bracketed_spans`: `[needs review]{.chip}`
    is a chip, `[text]{.lead}` is a classed span. A status word picks up its
    `chip-<word>` class automatically unless you name a `chip-*` class
    yourself. This retires `[c:…]`, which existed only because the bare form
    has a closed vocabulary; it keeps parsing.
  - **Fenced code attributes**, Pandoc's braced form:
    ` ```{.go caption="server.go"} ` and ` ```go {caption="server.go"} `.
    An `id` reaches the `<pre>`, further classes reach the `<code>`, and a
    caption may contain a quote via a backslash escape.
  - **The directive label form**, `:::kind[Title]`, from the CommonMark
    generic directives proposal. It is the only titled container form that
    can also carry an id and classes: `:::aside[Why]{#w .compact}`.
  - **GitHub alerts.** `> [!WARNING]` produces exactly what `::: warning`
    produces. Prefer it in any file that lives in a repository — GitHub,
    Obsidian and Typora render it natively, while `::: warning` shows up
    there as literal text.
  - **`[TOC]`** alongside `[[toc]]`, case-insensitively. GitLab's
    `[[_TOC_]]` is deliberately excluded: its underscores are emphasis
    delimiters, so it never arrives as the single text node the marker
    guard requires.
- A shared `{#id .class key=value}` attribute parser behind all three of the
  constructs that use that grammar, replacing three ad-hoc readers of which
  only one — goldmark's heading attributes — was a real parser.
- `compat/`, a third test suite characterising the Markdown dialect against
  Pandoc, with a `just compat` target. It sorts every construct into one of
  three buckets: agreeing with `pandoc -f commonmark_x`, passed through
  inert, or degraded to a labelled code block. Its own module, like `e2e/`,
  so the root module still needs nothing installed. Task lists are the one
  divergence inside the shared subset.
- `md2html.InstallSkill` installs the Claude Code authoring skill into a
  skills directory, and `--install-skill-user` / `--install-skill-project`
  expose it. The skill now travels inside the binary along with a copy of
  `docs/authoring.md` it defers to, so the guidance always matches the
  version installed, which a link to this repo's `main` would not. Neither
  flag creates the `.claude` directory it writes under, and the refusal
  names that directory in full. Both files carry the usual provenance
  marker, so a re-install replaces this tool's own copy silently and refuses
  one you have edited — refusing the whole install rather than leaving half
  of it in place, which is why the check lives with the files rather than in
  the command.

### Changed

- `github.com/stefanfritsch/goldmark-fences` is vendored into
  `internal/fences` and is no longer a dependency.

### Fixed

- A container's `:::` fence line is now consumed by the parser rather than
  recovered from the rendered body. Four spellings previously lost their kind
  word to a definition list marker or a setext underline on the following
  line — the setext cases leaked it into the heading text and the generated
  anchor id — and a title written after a braced fence was silently absorbed
  into the body, with collapsible kinds showing their fallback label
  instead. A title is now accepted on every container form. See
  [docs/specs/2026-09-13-fence-line-capture.md](./docs/specs/2026-09-13-fence-line-capture.md).
- A container's braced attribute block now accepts a bare key with no
  value and a single-quoted value — `::: {.callout data-flag}` and
  `::: {.callout data-x='single'}` were previously rejected outright and
  fell through to an "unknown container kind" warning.
- `::: {.elem-nav}` no longer emits a `<nav>` carrying the fence library's
  internal `data-fence` attribute.
- A forged `data-fence` attribute — `::: {.card data-fence="x"}` or
  `:::card[Title]{data-fence="x"}` — previously panicked the converter with
  an index-out-of-range inside the (now-vendored) fence parser. This bug
  predates this change and was already reachable on `main`; it is fixed
  here because fixing it is one guard on code this work already owns. The
  attribute is now silently dropped instead, whatever its case: the whole
  `data-fence` namespace is reserved, and an HTML attribute name does not
  distinguish `data-Fence` from `data-fence`.
- `[proven]{.chip}` no longer renders broken. The chip transform fired on
  the bracket and left `{.chip}` on the page as literal text; a
  non-vocabulary label such as `[needs review]{.chip}` stayed literal in
  full. Both now render the span Pandoc renders.
- A braced code fence no longer mangles its own info string.
  ` ```{.go caption="server.go"} ` produced `class="language-{.go"` and a
  caption of `server.go"}`.
- The browser suite's first test no longer fails on a cold CI runner. Its
  15s per-step budget also had to cover launching Chrome, which on a fresh
  runner left nothing for the test itself: the first test in the file, and
  only ever that one, failed at exactly the deadline. `TestMain` now
  launches and discards a browser before any test runs, so that cost is paid
  once outside every deadline, and the per-step budget went to 60s.
- That test asserts the image became expandable before clicking it, in its
  own step, rather than reading the class alongside the click. A regression
  in the wiring used to hang until the deadline waiting for a dialog nothing
  would open; it now fails in under a second, by name.

### Repository

- CI moved to `actions/checkout@v7` and `actions/setup-go@v7`; the previous
  majors target the deprecated Node 20 and were being force-upgraded.

## v0.2.0 — 2026-09-13

### Added

- `Options.MermaidURL` names the ES module a page with a diagram imports at
  view time. Empty keeps the pinned CDN build, so nothing changes for a
  caller who does not set it. This is the supported way to serve MermaidJS
  yourself, for a docs build that must not reach a CDN.

### Changed

- **Breaking (library only).** `CrawlOptions.LinkDepth` now reads exactly as
  `CrawlOptions.Depth` does: `-1` is unlimited, and a non-negative value is
  the bound itself, so `0` follows no links at all. It previously inverted
  that — `0` meant unlimited and `-1` meant none. The CLI is unaffected:
  `--link-depth` still defaults to unlimited, having changed default
  alongside it. A caller relying on the zero value to follow links without
  limit must now say `LinkDepth: -1`.
- `Options.Warn` reaches every diagnostic whatever `Transforms` holds.
  Convert rebuilds the builtins that report against the sink before running
  them, so a caller who appends to `Builtins()` — the documented way to add
  a transform — no longer silently loses container warnings for not having
  known to pass the sink to `Containers` themselves. The caller's own slice
  is never written to.
- The browser suite moved to `e2e/`, its own Go module, and lost its
  `e2e_browser` build tag; the module boundary is the opt-in now. Run it
  with `just e2e`. This takes chromedp out of the library's requirements:
  nothing the library builds or tests compiles it. It remains an indirect
  entry in `go.mod`, inherited from `go.abhg.dev/goldmark/mermaid`.
- The `go` directive is `1.26`, down from a patch-pinned `1.27.1`. 1.26 is
  the floor the dependency graph actually imposes.
- `mermaidCDN` is a constant. It was a var only so the browser suite could
  reassign it, which `Options.MermaidURL` now covers properly.

### Fixed

- The README described two sources of conversion warnings; there are three
  (the `fig` fence's has been there since the fence shipped).

### Repository

- GitHub Actions runs `gofmt`, `go mod tidy -diff`, `go vet`, `go test
  -race` and a CLI build on every push and pull request, plus the browser
  suite against the runner's Chrome.
- `just test` and `just e2e` join `just install`.
