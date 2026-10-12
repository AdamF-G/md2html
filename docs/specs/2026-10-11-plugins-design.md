# Plugins — Design

**Date:** 2026-10-11
**Author:** AdamF-G
**Status:** Draft for review

## Purpose

Let anyone extend md2html with a self-contained *plugin*, a directory of
data that needs no change to md2html's Go code. In this release a plugin
provides **styles**: named looks for a page. A plugin can hold several, and
a page can select several, from one plugin or from many, stacked in order:

```yaml
---
style: claude-artifact, wide-tables
---
```

Two outcomes:

1. A **`claude-artifact`** plugin ships in the binary, so a user can adopt
   its style with one flag or one front matter line.
2. The **plugin format is a public contract**: a contributor adds a plugin by
   dropping a directory into `plugins/` (it is embedded automatically) or by
   publishing a directory others load.

The manifest is a plugin manifest rather than a style manifest, so that later
kinds of extension (see Scope) become new sections of the same file instead
of a second file format.

Not to be confused with `--fragment`, which controls output *shape* (bare
HTML a host such as claude.ai can publish as an Artifact). A style controls
*look*. The two are independent: any style works with or without
`--fragment`.

## Concepts

- A **plugin** is a directory with an `md2html-plugin.json` manifest. It is
  *loaded* (built-in plugins always are; a local one by `--plugin` or front
  matter `plugins:`) and contributes what it provides to a page.
- A **style** is one named stylesheet a plugin provides. A page *selects*
  styles by name with `--style` or front matter `style:`.

Loading and selecting are separate on purpose. Loading is about where files
come from, and is the only place a path appears. Selecting is by name only.
So `style:` never has to tell a path from a name, and a later remote source
changes how plugins are loaded without changing how styles are named.

## Scope

**In v1**

- Plugins that provide styles: CSS plus the static assets that CSS
  references (fonts, images). Data only, no code.
- Built-in plugins embedded from `plugins/`, and local plugins loaded from a
  directory with `--plugin` or front matter `plugins:`.
- Selection of one or more styles with `--style` or front matter `style:`,
  and `--list-styles`.

**Deferred, with room left for them**

- **HTML capabilities** (page shell templates, declarative element rules,
  extra container or chip vocabulary). Deferred until a plugin needs one.
  Each would be a new top-level section of the manifest, next to `styles`,
  with its own way of being selected, decided when it exists.
- **Remote plugins** (`github.com/user/repo`, a marketplace). Only plugin
  loading takes a location, and it goes through one resolver, so a remote
  source is a new branch there, not a format change. The manifest's fixed
  filename is how a remote source would recognise a repository as a plugin.

**Out of scope**

- JavaScript in plugins. Page scripts stay in core. Contributed plugins never
  run code in a reader's browser.

## Plugin format

A plugin is a directory:

```
plugins/claude-artifact/
  md2html-plugin.json
  artifact.css
  compact.css
  fonts/
    display.woff2
    body.woff2
```

The manifest's name is fixed and specific to the tool. A plugin is then
recognisable in a repo full of other tools' files, and a future remote
source can identify a repository as a plugin by this one file. Every other
file is named by the manifest, so those names are the author's choice.

`md2html-plugin.json`:

```json
{
  "format": 1,
  "name": "claude-artifact",
  "description": "Looks from pages Claude publishes as Artifacts",
  "styles": {
    "claude-artifact": {
      "description": "The full Artifact look",
      "css": ["artifact.css"]
    },
    "claude-artifact-compact": {
      "description": "Tighter spacing, for long reference pages",
      "css": ["compact.css"]
    }
  }
}
```

### Top-level keys

| Key | Required | Meaning |
|---|---|---|
| `format` | yes | Contract version. This release reads `1` and refuses any other value. |
| `name` | yes | The plugin's name: qualifies its style names and appears in diagnostics. For a built-in plugin it must equal its directory name. |
| `description` | no | One line, shown by `--list-styles`. |
| `styles` | no | Map of style name to style (below). |

A plugin must provide at least one thing this release knows about. In v1
that means a non-empty `styles`, and a manifest without it is an error,
since it would do nothing.

Plugin names and style names are lowercase letters, digits and hyphens,
starting with a letter, so that they need no quoting in front matter and
never look like a path.

### A style

| Key | Required | Meaning |
|---|---|---|
| `description` | no | One line, shown by `--list-styles`. |
| `base` | no | `"default"` (the default value) layers the style on the built-in default stylesheet; `"none"` starts from nothing. No other value in v1. See Stacking. |
| `css` | yes | Ordered list of CSS files, relative to the plugin directory, concatenated in order. |

### Rules

- **Unknown keys**, at the top level or inside a style, produce a warning,
  not an error. A plugin written for a later release that only *adds* keys or
  capabilities still loads in this one, minus the parts this release doesn't
  know. A later format that changes the meaning of an existing key bumps
  `format`.
- **Assets are inlined.** Every URL in a style's CSS that names a relative
  path, in `url()`, `src()` or a string in `image-set()`, is replaced with a
  `data:` URI of that file, with its media
  type taken from the extension (`woff2`, `woff`, `ttf`, `otf`, `svg`, `png`,
  `jpg`/`jpeg`, `gif`, `webp`; anything else is an error). A relative `url()`
  resolves against the CSS file it appears in. Pages therefore stay single
  files and fetch nothing new. That keeps the existing guarantee that mermaid
  is the only thing a page can fetch, and keeps `--fragment` output
  publishable, since an Artifact allows no stylesheet or asset host but
  Google Fonts.
- **No reaching out.** The rule covers any construct that makes the browser
  fetch: `url()`, `src()`, `image-set()` and `-webkit-image-set()`, and
  `@import`, in any spelling CSS accepts, escapes included (`\75 rl(` is
  `url(`). The CSS is tokenized as a browser tokenizes it to find them. A
  URL naming an absolute URL (`http:`, `https:`, any scheme, `//`, a
  leading `/`), or a path that resolves outside the plugin directory (`..`,
  an absolute path, a symlink leading out), is an error. So is `@import`,
  and so is `var()` or another substitution as a direct argument of a URL
  function, since the URL it supplies cannot be checked. The stylesheet is
  checked again once its files are joined, since a file ending inside an
  open function changes what the next one means. The same containment
  applies to every path in the manifest, `css` entries and the manifest
  itself included. A plugin cannot fetch at view time, and cannot pull
  arbitrary files from disk into a page. `data:` URLs and `#id` references
  already in the CSS pass through untouched.
- **No closing the style element.** Plugin CSS containing `</style`
  anywhere, in any case and inside comments and strings too, is an error.
  The page writes its stylesheet raw into a `<style>` element, and the
  HTML parser ends that element at `</style` whatever CSS makes of it, so
  the rest would be markup, a script included. This applies to every
  plugin style, built-ins included. A stylesheet given with `--css` is the
  user's own file and is not checked.
- **Size.** A page whose assembled stylesheet exceeds 1 MB is converted with
  a warning. Every page carries its stylesheet, and an Artifact page is
  capped at 16 MB.

### The default stylesheet becomes a plugin

`default.css` moves to `plugins/default/default.css`. The `default` plugin
provides one style, `default`, with `base: "none"`. That way the built-in
default goes through the same loader as every other plugin, and contributors
have a complete working example to read. Its bytes do not change, so
existing output is identical.

The names `default` (plugin and style) are reserved.

## Loading plugins

The plugins available to a page are:

1. Every built-in plugin, always.
2. Each `--plugin DIR` given for the run (repeatable, comma-separable like
   `--exclude`), resolved against the working directory.
3. Each directory in the page's front matter `plugins:` (comma-separated),
   resolved against the directory of the page itself. This lets a docs tree
   carry its own plugin beside its pages (`plugins: ./_md2html`) without
   anyone passing a flag at build time. It adds to the run's plugins for that
   page only.

Two loaded plugins may not share a name, and a local plugin may not take a
built-in plugin's name. A clash is an error naming both directories. The
same directory loaded twice (by the flag and by front matter, or by two
spellings of one path) is one plugin, not a clash.

## Selecting styles

`--style` and front matter `style:` each take a comma-separated list of style
names.

### Names

- A **bare name** (`claude-artifact`) selects the style of that name if
  exactly one available plugin provides it. If two do, it is an error that
  lists the qualified names to choose between.
- A **qualified name** (`claude-artifact/claude-artifact-compact`) selects a
  style from one plugin by name, and is never ambiguous.
- A name selecting no available style is an error that lists the available
  ones.

### Stacking

A page's stylesheet is built from its selected styles in the order listed:

1. The default stylesheet, unless the *first* selected style has
   `base: "none"`.
2. Then each selected style's CSS in turn.

So a broad theme can come first and small adjustments after it (`style:
claude-artifact, wide-tables`), and each later style overrides the earlier
ones where they overlap, by ordinary cascade order. A style other than the
first that declares `base: "none"` is an error: it asks to discard styles
the page also asked to keep. The same style selected twice is used once,
with a warning.

The default layer is why `base: "default"` is the default. md2html keeps
gaining constructs (chips, `fig`, floating contents, source tools), and a
style layered on the default renders the next one acceptably without being
updated.

### Precedence

For each page, the first of these that applies sets the whole style list:

1. Front matter `style:`.
2. The run-wide choice: `--style`, or `--css` (raw replacement, as today).
3. The `default` style.

Front matter *replaces* the run's list rather than adding to it, the same
way `lang:` replaces `--lang`. A page that wants the run's theme plus its own
adjustment names both. `style: default` gives one page the plain default
inside a run that set something else. `--style` and `--css` together is a
usage error.

### Failures

- A bad **`--plugin`** or **`--style`** (missing directory, invalid manifest,
  forbidden URL in any of its styles, name clash, unknown or ambiguous style name, misplaced
  `base: "none"`) fails the run before any document is converted, exit 2.
  This matches how a bad `--lang` is refused up front.
- A bad **front matter `plugins:` or `style:`** produces a per-page warning
  naming the problem, and the page uses the run's style list. The whole list
  falls back, not just the bad entry: a page that loses one of three stacked
  styles would render as a look nobody chose. The page is still written.
  This matches a bad `lang:`.

## Library API

Plugin loading is one layer and stylesheet assembly another, so a future
capability reuses the loader without touching style code.

```go
// Options gains:
//
// Plugins are the directories of local plugins every page can use, beside
// the built-in ones, resolved against the working directory. A page's own
// "plugins:" front matter adds to them for that page.
Plugins []string
// Styles selects the run's styles by name, stacked in order; see
// "Selecting styles". A page's own "style:" front matter replaces the
// list. Empty means the default style. Setting both Styles and CSS is an
// error.
Styles []string
// PluginCache caches loaded plugins and assembled stylesheets across
// conversions. A caller converting many documents in one run shares one,
// so a plugin's files are read and its assets inlined once rather than
// once per page. Nil loads afresh on every call.
PluginCache *PluginCache
```

```go
// PluginCache loads plugins and assembles stylesheets, caching both. It is
// safe for concurrent use: the CLI converts documents in parallel.
type PluginCache struct{ /* ... */ }

func NewPluginCache() *PluginCache

// Load reads and validates the plugin in dir, relative paths taken
// against baseDir. Warnings about a plugin that loaded anyway (unknown
// keys) go to warn.
func (c *PluginCache) Load(dir, baseDir string, warn func(string)) (*Plugin, error)

// Stylesheet resolves names against the built-in plugins plus plugins,
// stacks the selected styles and returns the page's final stylesheet, with
// assets inlined.
func (c *PluginCache) Stylesheet(names []string, plugins []*Plugin) (string, error)

// Plugin is one loaded plugin: its manifest's identity and what it
// provides.
type Plugin struct {
	Name, Description string
	Styles            map[string]*Style
	// unexported: where the plugin's files live (embedded or on disk)
}

// Style is one style a plugin provides.
type Style struct {
	Name, Description string
	BaseNone          bool
	// unexported: its CSS file paths
}

// BuiltinPlugins lists the embedded plugins, for --list-styles.
func BuiltinPlugins() []*Plugin
```

`Convert` reads `meta["plugins"]` and `meta["style"]`, loads the page's
plugins against `filepath.Dir(opt.SourcePath)`, and assembles the
stylesheet. On failure it warns and falls back to the run-wide choice
(`opt.CSS`, else `opt.Styles`, else default). The result is the `css` string
that `renderPage` and `renderFragment` already take, so neither changes.

Plugins are cached by resolved location (embedded name, or absolute,
symlink-evaluated directory), so two pages naming one plugin by different
relative paths share an entry. Assembled stylesheets are cached by their
resolved style list, so 500 pages with the same selection inline their
fonts once. Errors are cached too, so a broken plugin named by 200 pages is
read once and reported per page.

## CLI

- `--plugin DIR`: load a local plugin for the whole run. Repeatable and
  comma-separable.
- `--style NAMES`: the run's styles, comma-separated, stacked in order.
- `--list-styles`: list every available style, one per line with its
  qualified name and description, grouped by plugin, then exit. It includes
  the styles of any `--plugin` given, so `md2html --plugin ./x
  --list-styles` checks a plugin before use. Like `--version`, it needs no
  entry.
- `--css FILE`: unchanged. Mutually exclusive with `--style`.
- The `--help` text names the three new flags. The guide's own section is
  below.

## Authoring guide: the `styles` section

Agents learn md2html through `md2html --guide`, which is cut from
`docs/authoring.md` at its `##` headings. A section's anchor is its name,
so `md2html --guide styles` prints just that section. The existing
`## Styling` section is replaced by `## Styles {#styles}`, written for an
agent that has to pick or build a look without reading the Go code. It
covers:

1. **Choosing styles.** `md2html --list-styles` lists what is available.
   `--style` applies a list to a run, and `style:` in front matter to one
   page. Stacking order, the replace-not-add precedence, and the `style:
   default` opt-out each get one sentence.
2. **Loading a local plugin.** `--plugin` and front matter `plugins:`, and
   which directory a relative path resolves against for each.
3. **Writing a plugin.** A minimal working example (an
   `md2html-plugin.json` with one style, plus a CSS file that redefines a
   few tokens), the manifest keys tables, `base`, naming rules, and the asset
   rules with what each refusal looks like.
4. **What a style can restyle.** The token inventory (`--bg`, `--fg`,
   `--muted`, `--accent`, `--rule`, `--code-bg`, `--warn`, `--ok`,
   `--measure`, and the rest), the required theme shape (bare `:root`, the
   two dark blocks), and the shipped class names. This replaces the class
   list in today's Styling section.
5. **What a plugin cannot do yet.** No scripts and no HTML changes, so an
   agent asked for a header bar, say, knows to say so instead of trying.
6. **Checking the result.** Warnings to expect, and a reminder to render at
   400px in both themes.

The front matter subsection under *Page structure* gains `plugins:` and
`style:` in its example block and key list, with a link to `#styles`. The
Quick reference gains one line for each. The Silent traps section gains one
entry if one exists, such as a style class name that doesn't match the
shipped markup and so is silently ignored. The section stays out of
`summarySections`: an agent needs it only when asked about the look.

When a later release adds a capability, it gets its own guide section; this
one stays about styles.

The guide tests (`guide_test.go`) already check the guide's claims. They
gain the section's examples: the minimal plugin loads, and `--guide styles`
resolves. The installed skill's SKILL.md gets one line pointing to
`--guide styles`.

## The `claude-artifact` plugin

Ships one style, `claude-artifact`. More (a compact variant, say) can be
added to the same plugin later without a format change.

"Match claude.ai artifacts" can't mean copying one fixed palette, because
Claude gives each Artifact its own palette and type. What every Artifact
shares is a set of rules, and the style encodes those rules plus one
deliberate identity:

- **Theme shape.** Every colour token defined on bare `:root`, redefined
  under `@media (prefers-color-scheme: dark)` guarded by
  `:root:not([data-theme="light"])`, and again under
  `:root[data-theme="dark"]`, with `color-scheme: dark` in both dark blocks.
  `body` has an explicit token background.
- **Layout.** A 16px minimum side gutter at every width, set once on `body`
  side padding with `padding-block` for the vertical. Safe-area padding is
  kept. A running measure near 65ch. The page never scrolls sideways at
  400px: only `pre`, tables and diagrams scroll, each inside its own
  container.
- **Type.** A display face for headings and a body face, embedded as `woff2`
  Latin subsets (Google Fonts families under the OFL, so they may be
  embedded), each with a real fallback stack. A fixed type scale, `text-wrap:
  balance` on headings, letter-spacing on uppercase labels, `tabular-nums` in
  tables.
- **Cards by role.** Border, fill and radius go on callouts, figures and
  code, and are kept off ordinary blocks.
- **Neutrals with a hue bias** toward the accent. Both themes are designed
  separately, not one inverted from the other.
- **Not a templated look.** The palette avoids the looks the Artifact design
  guidance names as generic, including warm cream with terracotta, which is
  close to md2html's current default. That gives `claude-artifact` a reason to
  exist alongside it.

Font budget: at most 150 KB of inlined font data in total. If the chosen
pair doesn't fit, use one embedded face plus a system stack.

Acceptance is visual. Render `testdata/conformance.md` and `docs/authoring.md`
with the style at desktop and 400px widths, in both themes and in the
unstamped system state, with and without `--fragment`, and compare against a
real Claude-published Artifact.

## Contributing a plugin

- **Built in.** Add `plugins/<name>/` with a manifest and its files, and open
  a PR. No Go file changes: `plugins/` is embedded whole, and the plugin
  tests pick up every directory in it.
- **External.** Publish the directory anywhere. Users check it out and load
  it with `--plugin path/to/it`, or from front matter `plugins:`.

The *contract* for a style is the manifest format plus the default
stylesheet's class names and token names. Those already change only with a
CHANGELOG note for replacement stylesheets, and this design makes that
policy explicit. The guide's `styles` section (above) is the contributor
documentation: it covers the manifest, the token and class inventory, and
the asset rules. Keeping it there serves people and agents from one source.

## Testing

- **Manifest:** missing required keys, `format` other than 1, empty or
  missing `styles`, unknown top-level and style keys (warn and load), bad
  `base`, names outside the allowed characters, built-in `name` that doesn't
  match its directory, a `css` path outside the plugin.
- **Assets:** relative `url()` inlined with the correct media type, resolved
  against its own CSS file; quoted and unquoted `url()` forms; `..`, absolute
  paths, symlinks out, `http(s):`, `//`, `@import` and unknown extensions are
  all refused; `data:` passes through. Each fetch in every spelling the
  tokenizer must catch (escaped names, `image-set()` and `src()` strings,
  strings ended by `\r` or `\f`, fragments that would close their own
  `url()`, `</style`) is refused or neutralised, and a browser test
  asserts that a page styled by any accepted payload requests nothing from
  another host.
- **Loading:** `--plugin` relative to the working directory, front matter
  `plugins:` relative to the page; a name clash between two plugins, or with a
  built-in, is refused; one directory loaded twice is one plugin.
- **Selection:** bare name; ambiguous bare name refused with the qualified
  choices listed; qualified name; unknown name refused with the available
  ones listed.
- **Stacking:** default first, then styles in listed order; a first style with
  `base: "none"` drops the default; `base: "none"` anywhere later is refused;
  a duplicate is used once with a warning.
- **Precedence:** front matter `style:` replaces `--style` and `--css`;
  `style: default` opts out; front matter `plugins:` adds to `--plugin` for
  that page only.
- **Failures:** a bad `--plugin` or `--style` exits 2 before writing
  anything, including a `--plugin` whose fault is in a style the run does
  not select, and `--list-styles` exits 2 on the same faults; a bad front matter `plugins:` or `style:` warns and the page uses
  the run's whole style list.
- **Cache:** a plugin read and a stylesheet assembled once across many
  concurrent conversions (run the test with `-race`).
- **Every built-in plugin:** loads without warnings; each style defines every
  token on bare `:root` and redefines the same set in both dark blocks, and
  stays within its size budget.
- **Default unchanged:** output with no style option is byte-identical to
  today's.
- **CLI:** `--list-styles` output, with and without `--plugin`; `--style` with
  `--css` is a usage error.
- **Guide:** `--guide styles` prints the section; its minimal example plugin
  loads cleanly.
- **Browser (e2e):** the `claude-artifact` style at 400px has no horizontal
  page scroll, in both themes.
