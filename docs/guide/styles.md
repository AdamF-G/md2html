---
nav-order: 14
---
# Styles

[[toc]]

A page's look comes from styles. With none chosen it gets the default.
`md2html --list-styles` lists every style available. Long descriptions are
shortened here:

```text
claude-artifact: Looks from pages Claude publishes as Artifacts
  claude-artifact  The rules every Artifact follows, …
default: The stylesheet md2html uses when no style is selected
  default  Warm neutrals, system fonts, both themes
proof: Ink on proof paper
  proof  A cool page, blue-black ink and a peacock-teal accent, in both themes
```

Each style is listed by the name to write after `--style`. Two plugins can
have styles of the same name; then each is listed as `plugin/style`, the
only name that picks it.

## Choosing styles

`--style claude-artifact` styles every page in the run. `style:
claude-artifact` in a page's front matter styles that page and replaces
the run's choice for it, the way `lang:` replaces `--lang`. `style:
default` gives one page the plain default inside a run that chose
something else.

Both take a comma-separated list, stacked in order: `style:
claude-artifact, wide-tables` puts `wide-tables` on top of
`claude-artifact`, and later styles win where they overlap. Name a style
alone, or as `plugin/style` when two plugins have a style of that name;
md2html says when you need to.

`--css mine.css` still replaces the stylesheet outright. It cannot be
combined with `--style`, and a page's `style:` overrides it.

### A light and dark toggle

Every style has a light and a dark theme, and a page follows the
reader's system. `[[theme-toggle]]` alone in its paragraph draws a
button that switches between them. It works on any page, or in a site
shell to give every page one. The choice is kept in the browser, so it
holds across a site's pages and on the next visit; choosing what the
system already prefers drops it, and the page follows the system again.
Charts and mermaid diagrams switch with the page.

A style draws the button however it likes. It is a
`button.md2html-theme-toggle` with `aria-pressed="true"` in the dark
theme, and its ☀ or ☾ sits in a `span.md2html-theme-icon`, so a style
can hide the glyph or make the span the knob of a switch. `proof` does
that.

The button is a small inline script, included only where the marker is.
Without scripts it stays hidden. A fragment ignores the marker and warns:
an Artifact's host sets the theme.

## Loading a plugin

Styles come from plugins. The built-in ones are always loaded. Load
another with `--plugin path/to/it` for a whole run, or with `plugins:
./_style` in a page's front matter, relative to the page. Both take a
comma-separated list. A page's `plugins:` widens only its own `style:`,
but the languages its plugins provide highlight its fences with or
without one.

Paths resolve in four places. `--plugin DIR` is relative to the directory
md2html runs in. `plugins:` is relative to the page. The `css` paths in a
manifest are relative to the plugin's directory. A `url()` in a CSS file is
relative to that CSS file.

## Writing a plugin

A plugin is a directory holding an `md2html-plugin.json` manifest and the
files it names. This one provides a style called `house` that changes the
accent colour and the text column's width:

```json
{
  "format": 1,
  "name": "acme",
  "description": "Acme's documentation look",
  "styles": {
    "house": {
      "description": "Acme blue, a wider column",
      "css": ["house.css"]
    }
  }
}
```

```css
:root { --accent: #1f5fa8; --measure: 72ch; }
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) { --accent: #8ab4f0; }
}
:root[data-theme="dark"] { --accent: #8ab4f0; }
:root[data-theme="light"] { --accent: #1f5fa8; }
```

| Key | Meaning |
|---|---|
| `format` | Always `1`. |
| `name` | The plugin's name: lowercase letters, digits and hyphens, starting with a letter. |
| `description` | One line for `--list-styles`. |
| `styles` | Each style by name: its `description`, its `css` files in order, and `base`. |
| `base` | Per style. `default`, the default, adds the style to the default stylesheet. `none` starts from nothing. |
| `charts` | Per style, optional. How its charts are drawn; see [Chart settings](#chart-settings). |
| `grammars` | Languages for highlighting code; see [Providing languages](#providing-languages). |

Style names follow the same rules as plugin names. `default` is reserved
for the built-in plugin and its style, so a plugin or style named `default`
fails to load. A manifest with no `format` or no `name`, with neither
`styles` nor `grammars`, or with a style that has no `css` list, fails to
load. An unknown key only warns.

A style is added on top of the default stylesheet, so it only needs to say
what differs. `"base": "none"` starts from nothing instead, and is only
allowed for the first style a page lists.

Fonts and images the CSS names with `url(fonts/x.woff2)` are copied into
every page as `data:` URIs, so the page stays one file that loads nothing.
The same goes for a file named in `src()` or as a string in `image-set()`.
Only these file types can be inlined: `woff2`, `woff`, `ttf`, `otf`, `svg`,
`png`, `jpg` or `jpeg`, `gif` and `webp`. Any other extension is an error.
A `url(#id)` reference and a `data:` URL already in the CSS are left as
written.

For the same reason a plugin cannot use `@import`, a web address, or a
file outside its own directory: each is an error naming the file. A symlink
that leads outside the plugin counts as outside. md2html reads the CSS as a
browser does, so writing one with escapes, as `\75 rl(` for `url(`, is
caught the same way. `var()`, or another function whose value comes later,
anywhere inside `url()`, `src()` or `image-set()` is an error too, since
the URL it supplies cannot be checked, even when another function such as
`light-dark()` sits between them. It is allowed only inside a gradient, a
colour function such as `rgb()` or `color-mix()`, a calculation such as
`calc()`, or `type()`, none of which can yield a URL.
Write the file's path itself.

CSS containing `</style`, even in a comment or a string, is an error: it
would end the page's `<style>` element early.

### Providing languages

A plugin provides languages for [highlighting](code-blocks.md#highlighting)
with a `grammars` key beside `styles`. A plugin can have either or both.
This one adds a language called `fabcfg`, which a fence can also name as
`fab`:

```json
{
  "format": 1,
  "name": "house-languages",
  "description": "Grammars for our in-house config formats",
  "grammars": {
    "fabcfg": {
      "file": "grammars/fabcfg.tmLanguage.json",
      "aliases": ["fab"]
    }
  }
}
```

| Key | Required | Meaning |
|---|---|---|
| `file` | yes | The TextMate grammar, relative to the plugin directory: `.json`, or `.json.gz`. |
| `aliases` | no | Other names a fence can use. Same spelling rules as the language name. |
| `scope` | no | The grammar's scope name, such as `source.fabcfg`. Read from the file when left out, and an error when it does not match the file's `scopeName`. Required in built-in plugins. |
| `license` | no | SPDX identifier, shown by `--list-languages`. Required in built-in plugins. |
| `source` | no | Where the grammar came from, shown by `--list-languages`. Required in built-in plugins. |

A language name or alias is lowercase letters, digits, `-`, `+`, `#` and
`.`, starting with a letter, since real fence tags include `c++`, `c#`
and `objective-c`. A fence matches it ignoring case. Each grammar file is
read when the plugin loads: one that is not JSON, or has no `scopeName`
or no `patterns`, fails the load.

Languages come from three places: the built-in grammars, every
`--plugin`, and a page's own `plugins:`, which add to the run's for that
page. A site shell's `plugins:` do not: they widen only the shell's
style, and supply no languages to the pages it wraps.

- **A local plugin replaces a built-in language** that has the same name,
  alias or scope name, silently. That is how to change how a language is
  highlighted.
- **Two local plugins that clash** warn once for each language name or
  alias they share, and the plugin loaded later wins, in the order
  `--plugin` and `plugins:` give. Two that share only a scope name warn
  once, the same way.
- A clash among `--plugin` plugins is reported once for the run. A page
  warns only about a clash its own `plugins:` cause.
- **A grammar can include another by scope name**, such as
  `"include": "source.go"`, and finds it among every loaded grammar, from
  any plugin, built-in ones too.

`md2html --plugin ./_langs --list-languages` checks the plugin loads and
shows its languages. `--list-styles` leaves out a plugin with no styles.

## Making your own style

A consistent look is a small set of decisions. A style built on the
default inherits every one of them, so you make only the ones you want
to change and leave the rest alone.

The settings that make a look:

| Group | Settings | Where they are listed |
|---|---|---|
| Page colours | `--bg`, `--fg`, `--muted`, `--accent`, `--rule`, `--code-bg`, `--ok`, `--warn` | [What a style can change](#what-a-style-can-change) |
| Status colours | `--status-good`, `--status-warning`, `--status-serious`, `--status-critical` | [Chart settings](#chart-settings) |
| Chart colours | `--series-1` … `--series-8`, `--chart-surface`, `--chart-grid`, `--chart-axis` | [Chart settings](#chart-settings) |
| Type and width | the `font-family` on `body` (tier 4 below), and `--measure` | [What a style can change](#what-a-style-can-change) |
| Chart text | the manifest's `charts` block | [Chart settings](#chart-settings) |
| Syntax colours | `--syntax-comment`, `--syntax-keyword` and eleven more | [Syntax colours](#syntax-colours) |

That is 36 colours, each with a light and a dark value. Mermaid diagrams
take theirs from the page colours, so in a full page they follow without
more work.

### What you decide, and what you inherit

Pick the smallest tier that gives the look you want:

1. **Just the accent.** Set `--accent` in both themes, as the example
   under [Writing a plugin](#writing-a-plugin) does. Links, callout rules,
   the contents list and the other accent touches change; the rest stays
   the default's. That includes `--series-1`, which is the default's
   accent rust set as a literal colour, not a reference to `--accent`.
   A style that changes `--accent` should set `--series-1` to a series
   version of it, in all four blocks, or single-series charts and the
   first colour of every multi-series chart keep the old accent. The
   light accent usually works as it is. A dark accent is made pale to
   read as text, which is too light and grey for a chart, so take a
   mid-tone of the same hue; `--check-style` says when one is off.
2. **The page palette.** Set all eight page colours in both themes. These
   travel together: `--fg`, `--muted`, `--accent`, `--ok` and `--warn` are
   all read on `--bg` and on `--code-bg`, so changing a background means
   checking every text colour against it. `--chart-surface` and the
   figure cards follow `--bg` by themselves. Set `--series-1` to follow
   the new `--accent`, as above. `--chart-grid` and
   `--chart-axis` are line colours like `--rule`: move them with it when
   the neutrals change hue.
3. **A chart palette.** Set `--series-1` to `--series-8` in both themes.
   The default's series were chosen for its near-white and near-black
   backgrounds, and suit a palette of similar lightness, so this tier is
   for a brand palette, not a requirement of tier 2. The four status
   colours carry meaning (good, warning, serious, critical); change them
   only if your series would be confused with them.
4. **A different typeface.** Set `font-family` on `body`, and give the
   style a `charts` block, or chart labels are measured as the default's
   system sans and can clip or overlap. For a face the plugin ships, embed
   it with `@font-face` and name the same file as `font`; for a system
   face, the inherited `metrics` stays and you raise `pad` if its letters
   run wider. Either way set the block's `font-family` to the stack the
   body uses.
   `claude-artifact` is a complete example of this tier.

You can leave alone: `--measure` unless the column feels wrong,
`--chart-surface`, the status colours, and the `charts` block while the
body keeps the system sans.

### A starter style

A complete tier 2 style, with every page colour, `--series-1`, the
chart line colours and the syntax colours set in all four blocks. It is
a fresh plugin, not the `acme` example grown, so it has no `--measure`.
Save the manifest as `_style/md2html-plugin.json` and the CSS as
`_style/house.css`, then use `--plugin ./_style --style house`.

```json {caption="_style/md2html-plugin.json"}
{
  "format": 1,
  "name": "house",
  "description": "A house look for our documents",
  "styles": {
    "house": {
      "description": "Slate neutrals and a blue accent, both themes",
      "css": ["house.css"]
    }
  }
}
```

```css {caption="_style/house.css"}
:root {
  --bg: #f8f9fb; --fg: #1b2430; --muted: #56606d; --accent: #1f5fa8;
  --rule: #dde2e8; --code-bg: #eef1f5; --ok: #2c6e45; --warn: #a8500c;
  --series-1: #1f5fa8; --chart-grid: #e4e8ed; --chart-axis: #c3cad3;
  --syntax-comment: #6e617b; --syntax-keyword: #002cd2; --syntax-string: #d60408;
  --syntax-number: #7d6100; --syntax-constant: #8b017f; --syntax-function: #00544e;
  --syntax-type: #5d01a7; --syntax-property: #0066b7; --syntax-tag: #c105aa;
  --syntax-attribute: #703503; --syntax-inserted: #09794c; --syntax-deleted: #9c0238;
  --syntax-heading: #660cfe;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #12161c; --fg: #e6eaf0; --muted: #9aa5b3; --accent: #8ab4f0;
    --rule: #2a313b; --code-bg: #1b2129; --ok: #6fbe89; --warn: #f0915a;
    --series-1: #4d7fe8; --chart-grid: #232a33; --chart-axis: #3a434f;
    --syntax-comment: #89898a; --syntax-keyword: #8ab1d8; --syntax-string: #ff9166;
    --syntax-number: #bd9402; --syntax-constant: #ff81b9; --syntax-function: #05b8a4;
    --syntax-type: #ecb6ff; --syntax-property: #0192d4; --syntax-tag: #c762bb;
    --syntax-attribute: #ffc66b; --syntax-inserted: #50e3a5; --syntax-deleted: #cf6c51;
    --syntax-heading: #b991ff;
  }
}
:root[data-theme="dark"] {
  --bg: #12161c; --fg: #e6eaf0; --muted: #9aa5b3; --accent: #8ab4f0;
  --rule: #2a313b; --code-bg: #1b2129; --ok: #6fbe89; --warn: #f0915a;
  --series-1: #4d7fe8; --chart-grid: #232a33; --chart-axis: #3a434f;
  --syntax-comment: #89898a; --syntax-keyword: #8ab1d8; --syntax-string: #ff9166;
  --syntax-number: #bd9402; --syntax-constant: #ff81b9; --syntax-function: #05b8a4;
  --syntax-type: #ecb6ff; --syntax-property: #0192d4; --syntax-tag: #c762bb;
  --syntax-attribute: #ffc66b; --syntax-inserted: #50e3a5; --syntax-deleted: #cf6c51;
  --syntax-heading: #b991ff;
}
:root[data-theme="light"] {
  --bg: #f8f9fb; --fg: #1b2430; --muted: #56606d; --accent: #1f5fa8;
  --rule: #dde2e8; --code-bg: #eef1f5; --ok: #2c6e45; --warn: #a8500c;
  --series-1: #1f5fa8; --chart-grid: #e4e8ed; --chart-axis: #c3cad3;
  --syntax-comment: #6e617b; --syntax-keyword: #002cd2; --syntax-string: #d60408;
  --syntax-number: #7d6100; --syntax-constant: #8b017f; --syntax-function: #00544e;
  --syntax-type: #5d01a7; --syntax-property: #0066b7; --syntax-tag: #c105aa;
  --syntax-attribute: #703503; --syntax-inserted: #09794c; --syntax-deleted: #9c0238;
  --syntax-heading: #660cfe;
}
```

The bare `:root` block is the light look for a browser with no theme
preference; the media block is the dark look the reader's system asks
for; the two `data-theme` blocks win when something sets `data-theme` on
the page's root, as a host page or a theme toggle does.
Leave one out and that case shows the default's colours. In this palette
`--fg` is about 15:1 on `--bg` in both themes, and `--muted` and
`--accent` are 5.6:1 or better on both `--bg` and `--code-bg`. Its
`--series-1` is the accent in light, 6.1:1 on the surface, and a mid-tone
of the accent's blue in dark, 4.8:1, since the pale dark accent is too
light for a chart. Its syntax colours are `claude-artifact`'s, made for a
code background close to this one; a different `--code-bg` means
checking them again.

### Keeping it consistent

- **Contrast.** `--fg`, `--muted`, `--accent`, `--ok` and `--warn` at
  least 4.5:1 on `--bg` and on `--code-bg`, with `--fg` well above it.
  Series need 3:1 on `--chart-surface` in dark and 1.9:1 in light, where
  the table under each chart carries the values for a reader who cannot
  pick a pale series out. Status colours need 3:1 in dark and 1.6:1 in
  light.
- **Status apart from series.** The status colours never share a chart
  with series colours and come with a label, so they may sit near a
  series, but not on one: keep each at least ΔE 4.5 from every series,
  and the four at least 13 from each other.
- **Series apart from each other.** Neighbouring series need different
  lightness as well as hue, in both themes, so that colour-blind readers
  can tell them apart too. Keep each series' OKLCH lightness within
  0.43–0.77 in light and 0.48–0.67 in dark, and its chroma at least 0.10.
- **Syntax colours apart.** Each `--syntax-*` colour at least 4.5:1 on
  `--code-bg`. Comment, keyword, string, function and type each at
  least ΔE 8 from `--fg`, so code does not read as plain text, and every
  two syntax colours at least ΔE 12 apart in each theme: embedded
  languages put any two kinds side by side.
- **All four blocks together.** Edit a colour in one block and the same
  colour in the other three, light pair and dark pair.
- **Look at it.** After changing colours, run
  `md2html --plugin ./_style --check-style`, then build
  a page with a chart, code, a callout and a diagram and view it in both
  themes at 400px wide, as [Checking a style](#checking-a-style) says.

`md2html --check-style` measures every one of these; see
[Checking a style](#checking-a-style).

## What a style can change

The default stylesheet draws every colour from these custom properties.
Redefining them is the simplest way to restyle a page:

| Property | Used for |
|---|---|
| `--bg`, `--fg` | Page background and text |
| `--muted` | Secondary text: subtitle, date, captions |
| `--accent` | Links, callout rules, the contents list |
| `--rule` | Borders and table lines |
| `--code-bg` | Code, callout and card backgrounds |
| `--warn`, `--ok` | Warning callouts, and the status badges |
| `--measure` | The text column's width |

Set each colour four times: on `:root`, in the
`@media (prefers-color-scheme: dark)` block for
`:root:not([data-theme="light"])`, on `:root[data-theme="dark"]`, and on
`:root[data-theme="light"]`. The example above shows the shape.

Figures have two more. `--fig-weight` is written by md2html onto each
figure panel from its item's `weight`, and sets the panel's share of the
row; a style can read it but should not set it. `--fig-surface` is the fill
the default stylesheet gives the card drawn around a figure panel, and an
accent ring inside the card paints its gap in the same colour. It belongs
to the default's card rules, not to md2html, so it exists only while those
rules do. A style built on the default can change the card's fill by
setting `--fig-surface` on
`.fig-panel:not(.fig-panel-arrow):not(.fig-panel-accent)`, rather than
setting `background`, so the ring still matches. The extra `:not()` leaves
accented panels their tint: without it the rule ties with the default's
accent rule and, coming later, wins.

The page's content is a `<main class="md2html">` in both full pages and
fragments. Name it as `main.md2html`, not a bare `main`: a fragment
carries its stylesheet into its host, and a bare selector would restyle
the host's own `<main>` too.

Classes you can style, beyond plain elements: `md2html`, `callout`, `card`, `group`,
`stats`, `defs`, `details.container`, `chip` and `chip-*`, `xref`,
`nav.toc`, `subtitle`, `docdate`, `eyebrow`, `code-figure`, `fig` and `fig-*`,
`table-scroll`, `anchor`, `source-tools`, and `md2html-chart` on a
chart's `<figure>`. Site mode adds `md2html-pages`, `md2html-branch`,
`md2html-label`, `md2html-breadcrumb`, `md2html-prev-next`, `md2html-prev`,
`md2html-next`, `md2html-list`, `md2html-inline`, `md2html-search`,
`md2html-search-input`, `md2html-search-results`, `md2html-search-kind`,
`md2html-copy` and `md2html-copyable`, and the `data-section` attribute on
`<html>`; see [Sites](sites.md). A class you add to your own
Markdown is inert until a style names it.

These class and custom property names are what a style is written against.
They change only with a note in the CHANGELOG.

### Syntax colours

[Highlighted code](code-blocks.md#highlighting) takes its colours from
thirteen properties, set four times like the others. Each colours one
kind of token, as the span's class names it:

| Property | Colours |
|---|---|
| `--syntax-comment` | Comments, also set in italics |
| `--syntax-keyword` | Keywords and storage words such as `func`, `if`, `return`, `const` |
| `--syntax-string` | Strings, quotes included |
| `--syntax-number` | Numbers |
| `--syntax-constant` | Other constants: `true`, `null`, escapes, `this` |
| `--syntax-function` | Function names, built-in ones included |
| `--syntax-type` | Type and class names |
| `--syntax-property` | Property names, such as JSON keys and `obj.field` |
| `--syntax-tag` | Markup tags, and YAML keys, which their grammar scopes as tags |
| `--syntax-attribute` | Markup attribute names |
| `--syntax-inserted` | Added lines in a diff |
| `--syntax-deleted` | Removed lines in a diff |
| `--syntax-heading` | Markup headings, and a diff's file and range lines |

Anything else, operators included, is plain text in `--fg`. Each built-in
style has its own syntax palette, with its own green and red for
`--syntax-inserted` and `--syntax-deleted`. A style built on the default
inherits the default's palette, so it needs to set only the colours it
changes. Each rule falls back to the surrounding text colour, so on a
`"base": "none"` style that sets none of them, code is plain and readable.

The rules are added to a page only when it has a highlighted fence,
ahead of its style, so a style's own rules for the `hl-*` classes win.
The properties are part of the style, so every page carries them, but
on a page with no highlighted fence they colour nothing.
Only comments change style; no kind changes weight, so text in a code
block keeps its metrics. Highlighted code's `<code>` carries the class
`md2html-hl`, for a style that treats it differently from plain code.

Which scopes make which kind is fixed by md2html, the same for every
language. A change to it changes pages, and is announced in the
CHANGELOG.

## Chart settings

Charts take their colours from these properties, set four times like the
others:

| Property | Used for |
|---|---|
| `--series-1` … `--series-8` | Categorical colours, in order |
| `--status-good`, `--status-warning`, `--status-serious`, `--status-critical` | Status meanings |
| `--chart-surface` | The surface under a chart, shown in rings and gaps between marks |
| `--chart-grid`, `--chart-axis` | Gridlines, and the axis or baseline |

Chart text uses `--fg` and `--muted`. A stylesheet that defines all
seventeen gets charts whose colours follow the theme. A style built on the
default inherits the default's, so it needs to set only those it changes.
On a stylesheet that lacks any of them, charts are drawn in fixed colours:
its own light values where it has them, the default's for the rest.

A style's optional `charts` block sets the face chart text is measured in
and the width charts are laid out at:

```json
"styles": {
  "house": {
    "css": ["house.css"],
    "charts": {
      "font": "fonts/house-sans.woff2",
      "font-family": "\"House Sans\", sans-serif",
      "width": 600
    }
  }
}
```

| Key | Meaning |
|---|---|
| `font` | A woff2, ttf or otf file in the plugin, usually one the stylesheet embeds. Chart text is measured with its exact widths. |
| `metrics` | `builtin:system-sans`, the one built-in: widths close to the common system sans faces, for a style whose text is the reader's system font, which md2html cannot measure. Not with `font`. |
| `pad` | 0 to 1: how much room to add to text measured by `metrics`, for a system face wider than the estimate. The default style's is 0.1; `0` adds none. Not with `font`, whose widths are exact: the pair warns. |
| `width` | The plot width in pixels, 100 to 2000, for a spec that sets none. |
| `font-family` | The CSS font stack chart text renders in. Put the face `font` measures first. |
| `tokens` | Colour roles of the style's own that a chart spec can name, such as `["inject", "source"]` for `token:inject`. Each is a custom property the stylesheet sets four times like the others, so a chart that uses it follows the theme. Up to 32 lowercase names, none already a chart or page token. |

Each key a style's block leaves out comes from the style below it, and
every page starts from the default style's block: system-sans metrics, a
0.1 pad, a 640-pixel width. A block naming `font` or `metrics` replaces
how text is measured, padding included. A `pad` applies only while text
is measured by `metrics`: stacked over a style whose block names `font`,
it does nothing. `claude-artifact` measures its
embedded IBM Plex Sans. A `font` that is missing, or is not a font, fails
the plugin's load; an unknown key warns. `tokens` add up through the stack, so a
style on top keeps the roles of the one below. A token a theme block does
not set is drawn in `--fg` wherever a chart has to be drawn in fixed
colours.

## What a plugin cannot do yet

A plugin cannot add scripts, change the page's HTML (no header bar, no
footer), or add new containers or badges. If a page needs one of those,
write it in the Markdown as raw HTML, or say it isn't possible yet.

## Checking a style

Run `md2html --plugin ./_style --list-styles` to check a plugin loads and
every style in it builds; a broken one exits with the reason.

Then run `md2html --plugin ./_style --check-style` to measure each style's
colours, stacked on the default as a page would use it. `--style a,b`
checks that stack instead, and with neither flag the built-in styles are
checked. Each style gets a line, then its findings:

    acme/plain: 1 error, 1 warning
      error    dark --muted on --code-bg is 2.60:1, under 3:1
      warning  light --muted on --code-bg is 4.21:1, under 4.5:1 (WCAG AA)

| Severity | Means |
|---|---|
| error | Broken or unreadable: a colour missing from one theme, or different when the reader picks the theme their system already shows; a value that is not a colour; text under 3:1, a syntax colour on `--code-bg` included; chart series out of the lightness band, grey, or too alike for readers with normal or colour-blind vision |
| warning | Short of a target: text under 4.5:1, a syntax colour on `--code-bg` included; a syntax colour under ΔE 8 from `--fg` (comment, keyword, string, function and type) or under ΔE 12 from another syntax colour; series or status colours faint on `--chart-surface`; status colours too close to each other or to a series; a colour the check cannot measure, such as `color-mix()`; a colour set in neither theme; a colour in a plain `:root` inside the dark media query that never shows, with the block that outranks it |
| note | Advice: neighbouring series that colour-blind readers can only just tell apart |

It exits 1 if any style has an error, 2 if a plugin or style fails to
load, and 0 otherwise. The chart rules
are the dataviz skill's, and the built-in styles pass all of them.

Syntax colours are only ever text in a code block, so they are measured
on `--code-bg` alone, by the text rules: an error under 3:1, a warning
under 4.5:1. Two more rules warn about a palette that passes contrast
and still shows little. Comment, keyword, string, function and type,
the kinds most code is made of, must each be at least ΔE 8 from `--fg`.
And every two of the thirteen must be at least ΔE 12 apart, in each
theme, since an embedded language can put any two kinds side by side:

    plain: 2 warnings
      warning  light --syntax-comment is ΔE 2.6 from --fg, under 8
      warning  dark --syntax-string and --syntax-type are ΔE 2.9 apart, under 12

A build checks each style its pages use and prints one line per style
with problems: the findings themselves when there are one or two, or
their counts. Only a style with an error adds to the run's warnings, and
a page always builds.

Then build a page and look at it at 400px wide and in both themes. A
front matter `style:` that fails warns and the page falls back to the
run's style, so read the warnings.
