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

- In a site, a directory's `README.md` is its page when it has no
  `index.md`, and a directory with neither gets a page md2html writes,
  titled by its name in title case, with the shell's `title-case:`
  spellings or a mount's `title`, and listing its pages.
- `[[pages from="page"]]` lists from the current page, so an index page
  lists what is in its own directory.
- `nav-sort: links` in a directory's `index.md` orders the directory's
  children in the site's navigation by the index's own links, so a reading
  order written as a list is not repeated as a `nav-order:` on each page.
  An explicit `nav-order:` still comes first, and a page the index never
  links comes last, by label.
- A site's shell says where a link out of the site goes: `repo-url:`, a
  URL with `{path}`, sends it to the file's page on the repository's
  host, and `unmounted:` (`link`, `unlink` or `error`) chooses. Paths
  are measured from the source root, the deepest directory holding the
  shell and the pages, or `source-root:` and `--source-root`.
- `Options.LinkDrops` and the `LinkDrop` transform remove a link and keep
  its text or an image's alt. `Options.AnchorLinkMap` and
  `Options.AnchorLinkDrops` carry what a site build does with an `<a>`'s
  href apart from the same href on an image the page shows.
- `[[mount]]` markers in a site's shell place a directory or a Markdown
  file from anywhere in the repository at a path in the site, so a site
  built from files spread across a repository needs no copy staged
  first. Pages are written at their site paths, links between them are
  rewritten through the mounts, a deeper mount takes its files out of an
  enclosing one, and a mount sets its own label, order, child ordering
  and contents list. `--mount` adds one for a single build. A mounted
  site reads chart data and `@file#key` references only from inside its
  source root.
- `[[pages collapse="true"]]` folds each branch of a site's page tree
  under a chevron beside its link, closed but for the branch holding the
  current page, so a sidebar keeps one shape on every page. The link
  stays a link; the chevron is a `<summary>` of its own, named for the
  page it opens, and the page gains no script. Chevrons sit in a column
  on the left of each row, which a style widens with
  `--md2html-fold-col`, or at the end of it with `chevron="right"`.

### Changed

- A site directory without an `index.md` is no longer a label with a
  warning: it gets a page, from its `README.md` or written by md2html.
  In a build with `-o` the page is at the directory's `index.html`, and
  a link to the directory reaches it. A directory whose index or README
  the build did not reach stays a label, with a warning.
- A `README.md` that is its directory's page, with `nav: hidden`, now
  hides the whole directory, as a hidden `index.md` always did; before,
  it hid only itself.
- **breaking:** `Doc.Body` holds the Markdown of a page md2html writes
  itself, whose `Src` names no file; a caller that converts
  `CrawlResult.Docs` must convert `Body` when it is set.
- **breaking:** a site build (`--site` with `-o`) copies the files its
  pages show into the output, recorded in `md2html-assets.txt`, instead
  of linking back to the sources, and builds nothing outside its base:
  a page it used to write under `_external/` is no longer written, and a
  link to it goes where `unmounted:` says. Without `repo-url:`, a link
  out of the site is now unlinked, with a warning. A file already at a
  copy's destination that md2html did not write, or that was edited
  since, is refused, and the build exits non-zero. A default source root
  that would be the filesystem's root is now an error, and a `.md` under
  the base that the build does not write is left as written.
- An image's `srcset` candidates and a video's `poster` are found and
  rewritten like any other link, in every build.
- Under a site shell, a page with `toc: none` gets no contents list from
  the shell's `[[toc]]`; before, it only kept `--autotoc` away.

### Fixed

- Under the `proof` style, a `[[search]]` in a site's header showed its
  results only as far as the header's bottom edge: the clip that kept
  the header's band of ink from spilling above and below cut off the
  dropdown too. The band is now painted without a clip, and the results
  hang over the page.

## v0.13.0 — 2026-10-16

### Added

- Syntax highlighting at build time. A fence whose language names a
  grammar, ignoring case, is coloured when the page is built: each token
  is wrapped in a span classed `hl-<kind>`, one of thirteen kinds, and the
  page gains no script. The code's text is unchanged, so copying it and
  site search see what they did. A fence can opt out with
  `{.no-highlight}`, and a page with `highlight: false`. A language md2html
  does not know stays plain, and a near miss of a known one warns, as in
  `fence at line 12: no grammar for "pyhton"; did you mean "python"?`.
  Fences inside a Markdown fence are coloured too, in any language the
  page knows. `md2html --guide code-blocks` documents it.
- The built-in `grammars` plugin: 201 TextMate grammars from
  `tm-grammars` 1.32.22, embedded compressed, each under a permissive
  licence. Each that has a sample in the collection, all but four, gives
  exactly what VS Code's own tokenizer gives on it. `md2html --licenses`
  carries their notices.
- A `grammars` key in the plugin manifest, so a local plugin can add a
  language or replace a built-in one. A plugin can now provide languages
  and no styles.
- `--syntax-comment`, `--syntax-keyword` and eleven more `--syntax-*`
  colour tokens, with a palette of its own in every built-in style, in
  both themes.
- `--no-highlight` (`Options.NoHighlight`) leaves every fence plain, and
  `--list-languages` lists every language with its aliases, plugin,
  licence and source.
- `--check-style` checks syntax colours: each at least 4.5:1 on
  `--code-bg`, comment, keyword, string, function and type each at least
  ΔE 8 from `--fg`, and every two at least ΔE 12 apart.

### Changed

- A fence in a language md2html knows now renders with spans, and its
  `<code>` gains the class `md2html-hl`. A page that has one carries the
  highlighting rules ahead of its style. A page with none has the same
  body as before. Its stylesheet gains only the style's `--syntax-*`
  colour settings, which style nothing on such a page. That adds about
  1.5 KB to a page in the default style and 3 KB in proof or
  claude-artifact.
- `--list-styles` leaves out a plugin that provides no styles.
- The binary is larger: highlighting adds about 2.4 MB, from 16.75 to
  19.12 MB stripped on darwin/arm64, and a stripped build is 18.75 to
  20.27 MB depending on the platform.
- Highlighting costs build time. This repository's docs, 35 pages with 78
  highlighted fences, build in about 0.08 s, against 0.05 s with
  `--no-highlight` (a built binary, Apple Silicon, warm, best of five,
  process start included). A page with one Markdown fence takes 0.04 s
  and one with a Go fence 0.03 s, against 0.02 s for a page with no
  fence.
- The scope-to-kind table decides which kind a token takes, from the
  first scope in its stack, innermost first, that a selector matches;
  the longest selector wins. It is part of the output, and a later change
  to it will be announced here. As shipped:

  | Selector | Kind |
  |---|---|
  | `comment` | comment |
  | `keyword`, `storage` | keyword |
  | `keyword.operator` | plain |
  | `string` | string |
  | `constant.numeric` | number |
  | `constant`, `support.constant`, `variable.language` | constant |
  | `entity.name.function`, `support.function` | function |
  | `entity.name.type`, `entity.name.class`, `support.type`, `support.class`, `entity.other.inherited-class` | type |
  | `support.type.property-name`, `variable.other.property` | property |
  | `entity.name.tag` | tag |
  | `entity.other.attribute-name` | attribute |
  | `markup.inserted` | inserted |
  | `markup.deleted` | deleted |
  | `markup.heading`, `meta.diff.header`, `meta.diff.range` | heading |

  A `meta.embedded` scope, or a `source` scope below the root, ends the
  walk with no kind, so code embedded in a string is not coloured as the
  string.

## v0.12.0 — 2026-10-14

### Added

- `[[theme-toggle]]`, a button that switches a page between its style's
  light and dark themes, on any page or in a site shell. The choice is
  remembered in the browser, applied before the page paints, and dropped
  when it matches the system's. Mermaid diagrams redraw in the new
  theme's colours. Its glyph sits in its own span, so a style can draw
  the button as a switch, and `proof` does.

## v0.11.0 — 2026-10-14

### Added

- The built-in `proof` style, ink on proof paper: a cool page with
  blue-black text and a peacock-teal accent in both themes, headings in a
  book serif, and, on a site shell's `::: header`, a full-width band of
  ink. Pick it with `--style proof` or `style: proof`.
- Site mode. `--site shell.md` wraps every page of a build in a shell: an
  ordinary page whose front matter says `site: true` and which holds one
  `[[content]]`. Markers in the shell draw the site from the pages
  themselves, with no config: `[[pages]]` (`depth`, `from`),
  `[[breadcrumb]]` and `[[prev-next]]` follow a tree derived from the
  directories, each `index.md`, `nav-order`, `nav-title` and `nav: hidden`,
  and `[[search]]` searches an index written as `md2html-search.js`, with
  `search-facets` to show front matter keys on results. A page with
  `shell: none` is built without the shell. Pages built with a shell get
  Copy buttons on code blocks, and `<html>` carries `data-section`, the
  slug of the page's top-level node. md2html styles what the markers draw
  plainly, ahead of the page's style and only on a page that has them, so
  every style gets it and a style's own rules win. A build without
  `--site` is unchanged. `md2html --guide sites` documents it.
- `[[list]]` and `[[subdirs]]` markers, which work on any page: links built
  from a comma-separated front matter value or from a directory's
  subdirectories, with `link`, `text`, `current`, `as`, and for
  `[[subdirs]]` `match` and `sort`.
- `@file#key` front matter references: a value taken from another file's
  front matter, followed where a `[[list]]` key, a marker template's
  `{name}` or a `search-facets` key reads it.
- The library: `BuildSite`, `Site`, `Options.Site`, `Site.SearchIndex`,
  `Site.IndexPath`, `IndexName`, `Doc.Meta` and `Doc.Title`.
- `::: header` and `::: footer` containers render `<header>` and `<footer>`
  landmarks, like `::: nav`.

### Changed

- A paragraph that is only `[[content]]`, `[[pages]]`, `[[breadcrumb]]`,
  `[[prev-next]]` or `[[search]]` now warns in a build without `--site`,
  where it needs a shell. It still renders as written.
- The authoring guide is a file per section in `docs/guide`, so the
  documentation site shows each section as a page of its own.
  `md2html --guide` reads the same files and takes the same section names.
  What it prints of a section now opens with a `#` heading, and a link from
  one section to another names that section's file, as in
  `styles.md#chart-settings`.
- `--list-styles` and `--check-style` name each style the way `--style`
  takes it: `proof`, not `proof/proof`. A style whose name another
  plugin's style shares is still shown as `plugin/style`, the only name
  that picks it.

## v0.10.0 — 2026-10-14

### Added

- Plugins: a directory with an `md2html-plugin.json` manifest that
  provides styles, with no change to md2html itself. `--style` and a page's
  `style:` front matter choose styles, stacked in order. `--plugin` and a
  page's `plugins:` load local plugins, and `--list-styles` lists what is
  available. Fonts and images a style uses are inlined, so pages still load
  nothing. `md2html --guide styles` documents all of it, including how to
  write a plugin and a walkthrough for making your own style, with a
  starter to copy. The library takes the same choices as `Options.Styles`
  and `Options.Plugins`, and adds the `Plugin` and `Style` types,
  `BuiltinPlugins`, `AvailablePlugins` and `ManifestName`. A
  `PluginCache` (`NewPluginCache`, `Options.PluginCache`) loads plugins
  with `Load` and `LoadAll` and assembles stylesheets, keeping both, so a
  run that converts many pages reads each plugin and inlines its fonts
  once.
- `Options.StyleChosen` is called for each page whose style is chosen,
  with the styles' names, the directory of each style's plugin and the
  stylesheet they assemble. The CLI checks each style list's colours
  there.
- The built-in `claude-artifact` style follows the rules every page Claude
  publishes as an Artifact does.
- Charts: a `vega-lite` fence holds a Vega-Lite spec in YAML or JSON, with
  its data inline or in a local CSV, TSV or JSON file inside the run's
  base (`Options.DataRoot` in the library). md2html draws it when it builds
  the page, as SVG in the style's colours and face that follows the
  theme, with tooltip boxes, nearest-point hover, keyboard focus and
  legend highlighting made of CSS, and a collapsed table of its data. The
  page gains no script and fetches nothing. Dates show the same in any
  time zone: as written when they name no zone, and in UTC when they name
  an instant. A spec that fails warns and stays a code block. Vega 5.30.0
  and Vega-Lite 5.21.0 run embedded. `md2html --guide charts` documents
  it.
- Charts can name the page's own colours, `token:accent`, `token:warn`,
  `token:ok`, `token:rule`, `token:bg` and `token:code-bg`, so a mark can
  match the text around it and still follow the theme. A style can add
  colour roles of its own with `tokens` in its manifest's `charts` block,
  such as `"tokens": ["inject", "source"]` for `token:inject`.
- Chart tokens a style can set: `--series-1` to `--series-8`,
  `--status-good`, `--status-warning`, `--status-serious`,
  `--status-critical`, `--chart-surface`, `--chart-grid` and
  `--chart-axis`. The default style and `claude-artifact` define them
  all, and charts in `claude-artifact` use its embedded IBM Plex Sans
  metrics.
- A style's manifest takes a `charts` block: the font chart text is
  measured in (`font`, or `metrics: builtin:system-sans` with a `pad`), the
  CSS `font-family` it renders in, and the `width` charts are laid out at.
- **`--no-charts`** (`Options.NoCharts`) leaves `vega-lite` fences as code
  blocks.
- `--check-style` measures a style's colours: text contrast against WCAG,
  and chart colours against the dataviz skill's rules, including
  colour-blind separation. A build reports a style's problems in one line.
  The built-in styles pass every rule; the default's light `--warn` and
  its status colours moved slightly to get there.
- **`--licenses`** (`ThirdPartyNotices`) prints the licences of the
  third-party software built into md2html.
- A `wide` container (`::: wide`) lets a table, code block or card extend
  past the text column, the way a wide figure does. A top-level block with
  the `wide` class, such as `::: {.card .wide}`, does the same.
- Front matter `title-block: above` puts the subtitle and date over the
  title, as an eyebrow line such as `Team A · Project B`, instead of
  under it. They keep their classes and also carry `eyebrow`.
- A container's attribute block takes `title-level=2` to `6`, which
  writes its title as a heading: in the outline, the contents list and the
  heading anchors, and styled as the paragraph title was.
- **`--preview-runtime`** (`Options.PreviewRuntime`), with `--fragment`,
  ends a fragment that has a mermaid diagram in the runtime a full page
  has, so it can be previewed in a browser. Fragments stay script-free
  without it.

### Changed

- `--version` names the commit when the build is not exactly a tagged
  release, for example `md2html v0.9.0 (db7884578809, modified)`, so two
  builds between tags can be told apart. The version itself is unchanged
  and still matches the provenance marker.
- The default stylesheet is now the built-in `default` plugin's style. Its
  content and the output are unchanged.
- The binary is larger: the chart engine and the Vega bundles take a
  stripped darwin/arm64 build from 5.1 MB to 17.2 MB.
- A diagram on a full page takes its colours from the page's style: nodes
  from the code background and accent, text from the text colour, lines
  from the muted colour, and gantt bars from tints of the accent, warning
  and muted colours. It used mermaid's stock light or dark theme, which
  matched no style, and its gantt axis and labels were hard to read in
  dark mode. A stylesheet without the colour tokens keeps the stock theme.
- Both shells wrap the content in `<main class="md2html">`, and the
  default stylesheet's `main` rules are scoped to that class. `--fragment`
  output had no `<main>`, so a host such as a Claude Artifact, which
  supplies the document but not a text column, showed running text at the
  full width of the page. A fragment's stylesheet travels into its host,
  and the bare `main` selector it carried could only ever have matched
  the host's own `<main>`. A wide figure breaks out of the measure only
  when its `<main>` is a direct child of `<body>`: on a full page, or a
  fragment published as an Artifact, but not a fragment a host places in a
  box of its own, where it would run off an off-centre page. A style that
  names a bare `main` should name `main.md2html`.

### Fixed

- A directive-form container title, `:::card[…]`, ended at its first `]`,
  so a title holding a badge, a link or a `]` in code was cut short and
  the container lost its kind. Its brackets now nest, and a `]` in a code
  span or escaped does not end it.
- `.chip-ok`, which the guide's badge example used, was styled by no
  stylesheet, so `[shipped]{.chip .chip-ok}` rendered as a plain grey
  chip. The default stylesheet now gives a chip three tones, `.chip-ok`,
  `.chip-warn` and `.chip-info`, from the status colours and the accent,
  and every style built on it inherits them.
- A code span in a table cell broke anywhere, so a narrow column split an
  identifier such as `rack_validation.straggler` across lines. Inside a
  scrolling table it now stays whole and the column widens instead.

## v0.9.0 — 2026-09-28

### Changed

- The installed authoring skill is `SKILL.md` alone. It sends the agent to
  `md2html --guide` instead of an `authoring.md` copied beside it, which
  described whichever binary installed it rather than the one converting
  the page. Re-installing removes that copy if an earlier version wrote it.
- The authoring guide is reorganised from the most common needs to the
  rarest, and says which spelling to write wherever there are several. It
  opens with a quick reference and a list of the silent traps, adding ones
  it missed: a `:::` inside a code block closing its container, front
  matter after a byte order mark, `word[x]{y}` in prose, unquoted YAML in a
  `fig` label, a backtick in a code caption, and others. It now recommends
  ```` ```go {caption="…"} ```` for code captions, and ends with every
  warning and its fix. Tests hold its claims to the binary.

### Added

- **`--guide`** (`GuideSummary`) prints the Markdown authoring guide's
  quick reference and silent traps, with an index of its other sections,
  and both `--help` and the skill point at it. `--guide <section>`
  (`GuideSection`) prints one section, such as `--guide diagrams`, and
  `--guide all` (`AuthoringGuide`) the whole guide. The whole guide is
  about nine thousand tokens: more than an agent needs every time it
  writes a page, and more than Claude Code shows of a command's output
  before cutting it short. On a terminal the output opens in `$PAGER`, or
  `less`; piped or redirected it is written unchanged. An agent without
  the skill could otherwise learn the syntax only by probing the output:
  in a skill evaluation, runs without it reverse-engineered the
  stylesheet's class names or ran `strings` on the binary to find the
  embedded guide.
- **`--install-skill DIR`** installs the authoring skill into any agent's
  skills directory, not only Claude Code's. Nothing in the skill was ever
  specific to Claude Code; only the two install flags were, and they stay
  as shortcuts. Like them, it refuses a `DIR` that does not exist, naming
  it in full, rather than put the skill where no agent looks. It cannot be
  combined with either of them.
- **`--autotoc all|long`** (`Options.AutoTOC`) gives pages a contents list
  without a `[[toc]]` marker: every page, or only those of more than 1000
  source lines or more than five headings. It goes after the title block,
  floats unless `--toc` or front matter says otherwise, and never doubles a
  page's own marker. `toc: none` in front matter opts one page out.
  `IsAutoTOCMode` exposes the check.

### Fixed

- A relative `-o`, as in `md2html ./docs -o ./site`, left every image and
  other asset link as written, so it broke in the output, with no warning.
  `CrawlOptions.OutDir` is now made absolute before any path is related to
  it, as `Doc.Out` always promised.
- A link or image attribute block set any well-formed name, so
  `[x](a.md){onclick="…"}` put an event handler on the `<a>`, where a
  container drops it. Links and images now take the global attributes,
  their own element's (`target`, `rel`, `width`, `loading` and the like),
  and any `data-` or `aria-` name; anything else is dropped.

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
