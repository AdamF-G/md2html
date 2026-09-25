---
title: "fig nested layouts"
subtitle: cols and split as items, and a card that panel position draws
date: 2026-09-14
---

# fig nested layouts

**Date:** 2026-09-14
**Author:** AdamF-G
**Module:** `github.com/AdamF-G/md2html`
**Status:** Shipped in v0.4.0.

[[toc]]

## Purpose

Two gaps surfaced while writing real figures against
[the fig vocabulary extensions](./2026-09-14-fig-vocabulary-extensions-design.md).

**A split cannot sit inside anything.** `layout` and `boundary` are
figure-level keys, so a figure that wants a callout above a split, a result
below it, or a split inside one column of a `cols` figure cannot be written.
`weight` is locked to the top level of a `cols` figure for the same reason.
The workaround that prompted this design — a "continuation" syntax joining
several fences into one visual figure — would have needed cross-block state
in a renderer that is currently stateless per fence, and still would not
have let a split nest.

**A panel has no look of its own.** `.fig-panel` is a bare weight carrier.
The only titled container is `group`, and a group looks the same wherever
it sits: dashed border, uppercase muted title, accent as a ring. A figure
that wanted panels to read as cards — a solid border, a tinted background
when accented — had to restyle all of that by hand.

This design makes layouts nestable as items and gives panel *position* a
card layer of its own, additive to whatever item is sitting in it.

## Summary of the change

| Change | Kind of thing |
|---|---|
| `cols` | a new kind: a list of items laid out side by side |
| `split` | a new kind: exactly two items with a boundary between |
| `boundary` | a modifier, legal only on a `split` item |
| `weight` | now also legal on the children of a `cols` item |
| panel card | a border, background and padding on `.fig-panel` |
| `fig-panel-accent` | a class on a panel whose item carries `accent` |
| `fig-panel-arrow` | a class on a panel whose item is an arrow |

Twelve kinds instead of ten.

::: warning
**Every existing `cols` and `split` figure changes appearance.** Their panels
now draw a card. This is intended — panel position owns a look — but it is
a visible change to documents nobody edited.

Markup changes in exactly two places: a panel whose item is accented gains
`fig-panel-accent`, and a panel whose item is an arrow gains
`fig-panel-arrow`. Every other figure emits the bytes it emits today.
:::

## 1. Layouts as items

### 1.1 Shape

`cols` and `split` are kinds whose value is a list of items, the same shape
`chain` already takes:

```yaml
items:
  - rail: A request crosses one trust boundary
  - split:
      - group: Client
        items: [{box: Browser}]
      - group: Server
        accent: true
        items: [{box: Handler}, {box: Store}]
    boundary: TLS
  - result: The response is signed
```

```yaml
layout: cols
items:
  - group: Before
    items: [{box: Handler}]
  - cols:
      - box: Handler
        weight: 2
      - box: Cache
```

The figure-level `layout`, `boundary`, `wide` and `caption` keys are
unchanged.

### 1.2 Legality

| Rule | Fault message names |
|---|---|
| A `split` item holds exactly 2 items | the item path and the count |
| `boundary` is legal only on a `split` item | the item path |
| `weight` is legal on the top-level items of a `cols` figure and on the children of a `cols` item, nowhere else | the item path |
| A `cols` or `split` item may not be a `rows` figure's only item | the item path and the `layout:` spelling to use |

`boundary` is optional on a `split` item, as it is at figure level.

Positional legality stays validation's job rather than the decoder's, as it
already is for `weight` and `items`: one struct carries every kind's fields,
so only validation knows where a key is legal. `validateItems` replaces its
`weightOK` "is this the top level" flag with "is this a child of a cols",
set by the figure for a `cols` layout and by the `cols` item for its own
children. `kinds()` gains the two new entries, so `split` and `box` on one
item is the existing "names 2 kinds" fault.

There is no depth limit, just as there is none for `group`. A split inside
a group inside a cols panel is legal.

### 1.3 One spelling per figure

A `rows` figure whose only item is a `split` item means exactly what
`layout: split` means. Two spellings for one figure are what the fig
design's single-vocabulary rule exists to prevent, so the sole-item form is
a fault:

```yaml
items:
  - split: [{box: A}, {box: B}]
```

degrades to a code block with the single warning

> fig fence: items[0] is the figure's only item; write layout: split
> instead; rendering it as a code block

The figure-level form stays the one spelling for a whole-figure layout, and
no existing document starts warning. The rule is scoped to the figure's own
item list: a group whose only item is a split is not a second spelling of
anything and is legal.

## 2. Rendering

The `switch doc.Layout` in `renderFig` moves into one method:

```go
func (r *figRenderer) layout(kind string, items []figItem, boundary string) string
```

`renderFig` calls it with the figure's `layout` and `boundary`; `item()`
gains `cols` and `split` cases that call it with the item's list and its
`boundary` modifier. Both paths therefore emit the existing
`.fig-rows` / `.fig-cols` / `.fig-split` / `.fig-boundary` / `.fig-panel`
structure, and a nested split is byte-identical to a figure-level one.

`panel()` adds two classes, both derived from the item it wraps:

| Item | Panel markup |
|---|---|
| carries `accent: true` | `<div class="fig-panel fig-panel-accent" style="--fig-weight:N">` |
| is an `arrow` | `<div class="fig-panel fig-panel-arrow" style="--fig-weight:N">` |
| anything else | `<div class="fig-panel" style="--fig-weight:N">`, unchanged |

The two cannot coincide, because `accent` is not legal on an arrow.

Panels still carry no metadata of their own. The rule that a panel which
needs a title, an accent or a footnote is a `group` stands; the accent class
is read off the item, not written on the panel.

## 3. The card layer

### 3.1 Additive, by position

The card belongs to panel position, not to any kind. A `group` renders
identically wherever it appears — title line, items, dashed border — and a
group inside a panel simply sits inside a card. A `box` in a panel sits
inside a card too. The two looks stack; neither is rewritten for the other.

This also keeps the card independent of
[§4 of the standards alignment](./2026-09-13-standards-alignment.md), which
plans to promote `group` to a `:::` container. The card never names
`.fig-group`, so that promotion cannot strand it.

### 3.2 Rules

```css
.fig-panel:not(.fig-panel-arrow) {
  --fig-surface: var(--bg);
  border: 1px solid var(--rule);
  border-radius: 4px;
  background: var(--fig-surface);
  padding: 0.6rem;
}
.fig-panel.fig-panel-accent { border-color: var(--accent); }
@supports (background: color-mix(in srgb, red 50%, blue)) {
  .fig-panel.fig-panel-accent {
    --fig-surface: color-mix(in srgb, var(--accent) 12%, var(--bg));
  }
}
```

The arrow exemption is a `:not()` on the card rule rather than a reset rule,
so it never has to track which properties the card sets. `cols: [group,
arrow, group]` is the common way to draw "A → B", and a card around the
connector would put a bordered box between the two panels.

The accent rule is written `.fig-panel.fig-panel-accent`, not
`.fig-panel-accent`. The `:not()` counts as a class, so the card rule scores
two classes and its `border` shorthand would otherwise override a
one-class accent colour — the trap `.fig-group.fig-accent` already
documents in the stylesheet.

The tint sits behind `@supports` rather than behind a duplicated
declaration. A custom property accepts any token sequence, so an engine
without `color-mix()` would keep the unparseable value and fail `background`
at computed-value time, painting no background at all. Behind `@supports`,
that engine keeps `--bg` and still draws the accent border.

No new query is needed. The phone-width rules target `.fig-cols` and
`.fig-split` by class, so a nested layout stacks exactly as a top-level one
does, and the arrow-direction rules are child selectors that match at any
depth.

### 3.3 The tint

The obvious 8% tint is the one value to avoid: it lands on almost exactly
`--code-bg`, the fill every box uses, in both themes.

| Tint | Box fill vs tint, light / dark | Muted text on tint, light / dark |
|---|---|---|
| 4% | 1.05 / 1.05 | 5.28 / 5.84 |
| 8% | 1.01 / 1.00 | 4.99 / 5.53 |
| 12% | 1.07 / 1.08 | 4.71 / 5.16 |

Contrast ratios computed from the shipped palette tokens.

12% keeps a box's fill distinguishable from the card behind it and reads
clearly as emphasis. The muted group title stays above the 4.5:1 floor for
small text, with the least room in the light theme — which is why §4 tests
that floor in a browser rather than trusting this table.

### 3.4 The ring gap

`.fig-accent` draws its ring as a 2px gap in `var(--bg)` and a 1px line in
`var(--accent)`. Inside a tinted card, a `--bg` gap is a visible stripe of
page colour around the ring. The gap moves onto the surface token:

```css
.fig-accent {
  border-color: var(--accent);
  box-shadow: 0 0 0 2px var(--fig-surface, var(--bg)), 0 0 0 3px var(--accent);
}
```

`--fig-surface` inherits, so an accented item takes its nearest card's
surface, a nested card resets it for its own contents, and an item in no
card falls back to `--bg` exactly as it does today. Like `--fig-breakout`,
it is declared by the stylesheet's own rules rather than emitted into the
markup, so it is not an extension hook: a replacement stylesheet that
drops the card loses the property and the fallback applies.

## 4. Testing

### 4.1 Validation

In `fig_test.go`, through `figConvert` and its placeholder stylesheet.
Each fault asserts exactly one warning, a code-block fallback, and the item
path in the message:

- a `split` item with 1 item, and with 3;
- `boundary` on a `box`, a `group` and a `cols` item;
- `weight` on a child of a `split` item and on a child of a `group`;
- `split` and `box` on one item;
- a `cols` item and a `split` item as a `rows` figure's only item, each
  warning naming its own `layout:` spelling.

Valid cases assert no warnings and rendered markup:

- `weight` on the children of a `cols` item;
- a `split` item between a `rail` and a `result`;
- a `split` item inside a `group` inside a `cols` panel;
- a `group` whose only item is a `split`.

### 4.2 Markup

In `figrender_test.go`:

- **Equivalence.** A `layout: split` figure and the same split written as
  an item after a leading `box` in a `rows` figure render the same
  `<div class="fig-split">…</div>` block. The comparison is between the two
  renders, not against a hand-copied string, so it tests the shared
  `r.layout` rather than a transcription of it.
- `fig-panel-accent` appears on a panel exactly when its item carries
  `accent`.
- `fig-panel-arrow` appears on a panel exactly when its item is an arrow.
- The existing `<div class="fig-panel" style="--fig-weight:1">` assertions
  keep passing, pinning the unchanged plain panel.

### 4.3 Gallery

`testdata/figures.md` gains three sections: a split inside rows (rail,
split, result), nested weighted cols, and an accented panel holding an
accented box. `TestFigGalleryRenders` gains a full-tag assertion for each,
and its panel-level arrow assertion is updated for `fig-panel-arrow`.

### 4.4 Browser

In `e2e/browser_test.go`, following the suite's rule that a colour is
asserted as its resolved token, never as a literal:

- **Card.** In both themes, a plain panel's border is solid and equals
  `--rule`; an accented panel's border equals `--accent`; an arrow panel
  has no border.
- **Contrast.** In both themes, the contrast ratio of a muted group title
  against the accented card's computed background is at least 4.5, and a
  box's fill against that background is at least 1.05. These are floors,
  not colours: a palette change that keeps them passes, one that breaks
  readability fails.
- **Ring gap.** An accented box inside an accented panel has a
  `box-shadow` whose inner colour equals the panel's computed background.
- **Phone width.** `TestBrowserFigColsStackWhenNarrow` gains a case for a
  `cols` item nested in a `rows` figure.

## 5. Documentation

`docs/authoring.md`, in the structured figures section:

- `cols` and `split` in the kinds table; `boundary` in the modifiers table;
- the `weight` sentence widened to the children of a `cols` item;
- the sole-item rule, with the spelling it asks for;
- one paragraph on the card: panels draw one, an accented item tints it,
  an arrow panel does not.

`.claude/skills/md2html-authoring/SKILL.md` gains one quick-reference row:
a callout above or below a split is a `split:` item between other items.

## Alternatives considered

- **A continuation syntax joining adjacent fences.** Needs state across
  render calls or a pre-render grouping pass, reopens the per-fence
  warn-once rule, and still cannot put a split inside a column.
- **A generic `layout:` item kind reusing `items`.** Reads like the
  figure-level form, but `items` is a group-only key today, and a kind
  whose value is a mode name is unlike every other kind.
- **`layout` on `group`.** Makes a titled group lay out its own items. It
  changes how a group renders depending on a key, and an untitled split
  would need a group with an empty title.
- **Restyling `.fig-group` as a card everywhere.** Changes every existing
  group, including the ones in plain rows that were never panels, and ties
  the card to a kind the standards alignment plans to promote.
- **`.fig-panel:has(> .fig-accent)` instead of a panel class.** No renderer
  change, but it moves a fact the renderer already knows into a selector
  every replacement stylesheet would have to rediscover.
- **Deprecating figure-level `layout` in favour of items.** One spelling
  going forward, at the cost of every existing `cols` and `split` figure
  warning on the next run.

## Out of scope

Continuation across fences, panel-level keys of any kind, a depth limit,
author-chosen tint strengths, and any change to how `group`, `chain` or
`lanes` render.
