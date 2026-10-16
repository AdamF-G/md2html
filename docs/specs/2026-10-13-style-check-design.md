# Style Check — Design

**Date:** 2026-10-13
**Author:** AdamF-G
**Status:** Shipped in v0.10.0.

## Purpose

A style is about two dozen colours, each with a light and a dark value, and
nothing tells its author when one of them is wrong. A text colour at 2:1 on
its background, a dark value forgotten so the light one shows on a black
page, two chart series a colour-blind reader cannot tell apart: each builds
without a word.

`md2html --check-style` measures a style's colours and says what is wrong,
and a build says so too when something is badly wrong. The rules are the
dataviz skill's for chart colours and WCAG's for text, set as strictly as
the skill's own reference palette allows, so `claude-artifact`, which uses
that palette, passes, and so must every built-in style, enforced by a test.

Out of scope: a data-defined theme format. A style stays a stylesheet; the
guide's "Making your own style" section and this check are the support for
writing one.

## Decisions

| # | Decision | Why |
|---|---|---|
| 1 | An on-demand flag gives the full report; a build reports problems in one line per style | The author asks for detail; a build should not drown pages' own warnings |
| 2 | Three severities: error, warning, note. Only errors make `--check-style` exit non-zero | An error is a page that is broken or unreadable; the rest is advice of falling strength |
| 3 | Chart rules are the dataviz skill's checks, ported to Go, with its thresholds unchanged where it sets one | One rulebook for md2html and the skill; tightening them to one palette's own numbers would fail other good palettes for no reason |
| 4 | Where md2html adds a rule the skill does not gate, its threshold is the strictest the skill's reference palette passes, rounded down to one decimal | The user's rule: as strict as possible while the skill's palette is fine |
| 5 | Every built-in style has zero findings, of any severity; a test enforces it | A commit that breaks a built-in style fails `go test` |
| 6 | The default style's colours are fixed where they fail (below) | The default must pass its own check, so every finding on a user's style is about something that style changed |
| 7 | A style is checked as a page would use it: stacked on what it builds on | Inherited colours are what the reader sees; a new background can break inherited series |
| 8 | Series separation is checked between neighbouring series only | The skill's default; its own palette fails an all-pairs check, and a style cannot know which chart a page draws |
| 9 | No origin for a finding ("inherited from default") | With the default clean, a finding is about the checked style or how it meets what it inherits |

## What is checked

A style's colours are read from the assembled stylesheet, the same one a
page gets. What a reader sees depends on their system's theme and on
whether they picked one with the page's theme choice, so a token has six
views, set by the guide's four theme blocks and by a bare `:root` in the
dark query, which people often write instead of the media block:

| View | System | Picked | Blocks that apply |
|---|---|---|---|
| L1 | light | nothing | `:root` |
| L2 | either | light | `:root`, `:root[data-theme="light"]` |
| L3 | dark | light | `:root`, the dark `:root`², `:root[data-theme="light"]` |
| D1 | dark | nothing | `:root`, the dark `:root`², the media block¹ |
| D2 | light | dark | `:root`, `:root[data-theme="dark"]` |
| D3 | dark | dark | `:root`, the dark `:root`², the media block¹, `:root[data-theme="dark"]` |

¹ `@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) { … } }`
² `@media (prefers-color-scheme: dark) { :root { … } }`, which has no
`:not` guard, so it applies on a dark system whatever the reader picked.

Within a view the cascade decides: bare `:root`, in the dark query or not,
loses to the other three selectors, which tie with each other, and among
ties the later block wins. A theme selector inside any other at-rule, such
as `@media print` or `@container`, is not a theme block and is skipped;
`@supports` and `@layer` only wrap, so their contents read as if they
were not there.

The dark query is `prefers-color-scheme: dark` alone, in any case and
spacing, or after `screen and`, `all and`, `only screen and` or `only all
and`. A query with anything more, such as `not`, `print`, a comma list or
`and (min-width: 40em)`, applies only some of the time, so its blocks are
skipped like any other at-rule's. A rule nested inside a theme block is
skipped whole, and the block's declarations after it still count. A
brace, semicolon or `/*` inside a quoted string, or a `/*` inside an
unquoted `url()`, is text. `:root` and `:not` match in any case; the
`light` and `dark` theme names do not.

In a stacked stylesheet that matters: a style that sets only the media
block loses, in D2, to the default's `[data-theme="dark"]` block. A value
of `var(--x)` resolves to `--x` in the same view, as `charts.TokenValues`
does.

The light value of a token is its L2 value and the dark value its D2 value;
the rules are evaluated on those. L1 and L3 must equal L2, and D1 and D3
must equal D2, or the page changes colour when the reader picks the theme they already
see.

The **checked tokens** are the 23 the guide lists: `--bg`, `--fg`,
`--muted`, `--accent`, `--rule`, `--code-bg`, `--ok`, `--warn`,
`--chart-surface`, `--chart-grid`, `--chart-axis`, `--series-1` …
`--series-8`, and the four `--status-*`. The light and dark values checked
are evaluated in light and in dark, as above.

### Colour values

A value the check can measure is a hex colour (`#rgb`, `#rrggbb`, and the
four- and eight-digit forms when fully opaque), `rgb()`/`rgba()`,
`hsl()`/`hsla()`, `oklch()`, `oklab()`, or a CSS named colour, in either the
comma or the space syntax. A colour with alpha below 1, or a form the check
cannot evaluate (`color-mix()`, `light-dark()`, relative colours,
`currentColor`, an unresolved `var()`, a system colour such as `Canvas` or
a deprecated CSS2 one such as `Window`), is reported as a warning and left
out of every rule that needs it. A value that is none of these, such as a
typo, is an error.

### Rules

ΔE is the distance in OKLab ×100; "colour-blind" is the smaller of the
protan and deutan distances under the Machado, Oliveira and Fernandes
(2009) simulation at severity 1.0, exactly as the skill's validator
computes them. "Text colours" are `--fg`, `--muted`, `--accent`, `--ok` and
`--warn`, each on both `--bg` and `--code-bg`. "Neighbouring" means
`--series-n` and `--series-n+1`.

| Severity | Rule | Light | Dark | Source |
|---|---|---|---|---|
| error | A token defined in one theme but not the other | | | md2html |
| error | A token whose views in one theme differ (L1 or L3 ≠ L2, or D1 or D3 ≠ D2) | | | md2html |
| error | A value that is not a colour | | | md2html |
| error | Text colour contrast | ≥ 3:1 | ≥ 3:1 | WCAG large text |
| error | Series lightness, OKLCH L | 0.43–0.77 | 0.48–0.67 | skill |
| error | Series chroma, OKLCH C | ≥ 0.10 | ≥ 0.10 | skill |
| error | Neighbouring series, normal vision ΔE | ≥ 15 | ≥ 15 | skill |
| error | Neighbouring series, colour-blind ΔE | ≥ 6 | ≥ 6 | skill |
| warning | A token defined in neither theme | | | md2html |
| warning | A colour the check cannot measure (above) | | | md2html |
| warning | A token in a plain `:root` inside the dark media query that no dark view shows, since the guarded media block, the dark block or a later plain `:root` outranks it; once per token, for its last such declaration, naming what outranks it | | | md2html |
| warning | Text colour contrast | ≥ 4.5:1 | ≥ 4.5:1 | WCAG AA |
| warning | Series on `--chart-surface` | ≥ 1.9:1 | ≥ 3:1 | skill's palette: 1.9986 / 3.71 |
| warning | Status on `--chart-surface` | ≥ 1.6:1 | ≥ 3:1 | skill's palette: 1.69 / 3.82 |
| warning | Status colours from each other, normal vision ΔE | ≥ 13 | ≥ 13 | skill's palette: 13.6 |
| warning | A status colour from every series, normal vision ΔE | ≥ 4.5 | ≥ 4.5 | skill's palette: 4.8 |
| note | Neighbouring series, colour-blind ΔE, in the skill's floor band | ≥ 8 | ≥ 8 | skill |

A failure counts once at its highest severity: text at 2.5:1 is one error,
not an error and a warning. A token undefined in both themes is not also
reported by the rules that need it; they skip it.

Series below 3:1 in light mode are allowed down to 1.9:1 because the skill
allows them with a second way to read the values, and md2html draws a table
view under every chart. Status colours sit near some series by the skill's
design: they never share a chart with series colours and always come with an
icon and a label, so the reader finds "critical" by reading it. The 4.5
floor still catches a status colour that is all but a copy of a series.

### Built-in styles

`claude-artifact` measures, at its worst: text 4.53:1, series L
0.53–0.76 (light) and 0.53–0.67 (dark), chroma 0.13, neighbours 19.3 normal
and 8.4 colour-blind, series 1.9986:1 / 3.71:1, status 1.69:1 / 3.82:1,
status apart 13.6, status from series 4.8. Zero findings.

`default` fails three rules today, each in a colour the series do not
depend on:

| Token | Today | Rule |
|---|---|---|
| light `--warn` | 4.35:1 on `--code-bg` | text ≥ 4.5:1 |
| light `--status-warning` | 1.58:1 on `--chart-surface` | status ≥ 1.6:1 |
| status colours apart | 12.9 light, 11.5 dark | ≥ 13 |

Each is moved the least that passes, keeping its hue, and the before and
after are shown to the user as swatches with the result. The guide's
paragraph on the default's light series 5 and 8 being under 3:1 is replaced
by the rule above.

## Interface

### `--check-style`

```
md2html --check-style [--plugin DIR]... [--style NAMES]
```

It checks:

- with `--style`, that list, stacked as a page would stack it;
- otherwise, each style of each `--plugin`, each stacked on the default as
  if selected alone;
- with neither, every built-in style.

Plugins load exactly as `--list-styles` loads them (`checkPlugins`), so a
plugin that fails to load exits 2 with the reason, as there. Nothing is
converted; no other flag is needed. Files or `--css` with it are a usage
error, exit 2, since a report on the built-ins would read as a pass for
them.

The report, on stdout, gives each checked style a heading line with its
counts, then each finding, errors first, in token order within a severity:

```
house/house: 1 error, 2 warnings
  error    dark --muted on --code-bg is 2.6:1, under 3:1
  warning  light --muted on --code-bg is 4.2:1, under 4.5:1 (WCAG AA)
  warning  dark --series-3 on --chart-surface is 2.7:1, under 3:1
house/print: no problems
```

A finding names the theme, the token or tokens, the measured value and the
threshold, and nothing else. Exit status: 0 with no errors, 1 with any error,
2 when a plugin or style cannot load.

### During a build

Each distinct style list a run assembles (the run's `--style`, or a page's
own `style:`) is checked once, when its stylesheet is first built, and a
list with any finding prints one line to stderr. Two plugins that give
the same qualified name make two lists, checked and reported apart, and
each such line names its plugins' directories, as `style acme/x (from
/home/me/a): …`. With one or two findings the line carries them; with
more, it carries the counts and a command for the details. The command
names the directory of each plugin the list's styles come from, built-in
ones aside, whether the run's `--plugin` or a page's `plugins:` loaded
it, and single-quotes a directory that holds anything but letters,
digits and `_./-`:

```
md2html: style house/house: dark --muted on --code-bg is 2.6:1, under 3:1
md2html: style house/house: 1 error, 3 warnings, 2 notes (md2html --check-style --plugin /home/me/site/_style --style house/house for details)
```

The line counts toward the run's warnings only when an error is among the
findings, so a style with notes alone does not change the summary. The
run's exit status is unchanged: a page with a weak style still builds. A
page with no style gets the default stylesheet without assembling one, so
nothing is checked or printed. A run's `--css` is a file, not a style, and
is not checked.

The library gains one optional hook, `Options.StyleChosen`, and nothing
else: `Convert` never checks a style.

## Architecture

- **`internal/stylecheck`** (new): `Check(css string) []Finding`, with
  `Finding{Severity, Theme, Tokens, Message}`, and `Summary`/`Format`
  helpers for the two output shapes. It holds the six-view reader, colour
  parsing, the colour maths (WCAG luminance, OKLab, OKLCH, Machado
  simulation) and the rule table. It depends on nothing in md2html; its
  tests run on hand-written CSS.
- **`charts.TokenValues`** keeps its job. The checker has its own reader
  because it needs all six views and every token, and stays independent of
  the chart package.
- **`Options.StyleChosen`** (new, exported): an optional
  `func(names, dirs []string, css string)` that `Convert` calls once,
  after it has chosen a page's style from `Styles` or the page's own
  `style:`, with the list's qualified names, the directory of each style's
  plugin (`""` for a built-in, parallel to the names), and its assembled
  stylesheet. The dirs tell apart two plugins that give one qualified
  name, and let the CLI's hint load the same plugins. It is not called
  for the default stylesheet chosen by default, or for `CSS`. Front matter
  resolves inside the library, so this is the one place that sees every
  list a page is drawn in. It is a per-page call and not a per-assembly
  one, because the CLI assembles every plugin style up front to check it
  loads, and an assembly hook would report styles no page uses. The CLI
  remembers the lists it has checked; the hook is called from parallel
  conversions, so it must be safe for concurrent use.
- **`cmd/md2html`**: the flag and its report, and a hook on the run's cache
  that checks each list and prints the build line.
- **Tests**: rules at their boundaries; colour parsing; the six-view
  cascade; the skill's reference palette, pinned with its values, giving zero
  findings (so the port cannot drift from the skill); every built-in style
  giving zero findings; the hook called with the page's list, and not for
  the default or `CSS`; the flag's output and
  exit codes; the build line in its short and counted forms, and its effect
  on the warning count.

## Docs

- The guide's "Checking a style" section gains `--check-style` and the rule
  table in brief. "Making your own style" points to it, and its 3:1 advice
  becomes the rule above.
- The README's flag list and the CHANGELOG gain the flag and the build line.
