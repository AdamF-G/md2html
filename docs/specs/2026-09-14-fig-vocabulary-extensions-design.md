---
title: "fig vocabulary extensions"
subtitle: one new kind, four new modifiers, and the rule that makes a panel a group
date: 2026-09-14
---

# fig vocabulary extensions

**Date:** 2026-09-14
**Author:** AdamF-G
**Module:** `github.com/AdamF-G/md2html`
**Status:** Proposed

[[toc]]

## Purpose

Extends the `fig` vocabulary to cover diagrams a design document commonly
draws that `fig` could not yet express: an annotated file or config tree,
a secondary gloss beside a label, a footnote under a panel, an accented
item or lane, a third line on a stat tile, and a figure wider than the
text column.

It also settles where panel-level metadata lives — on the layout primitive,
or on `group` — and records what we are declining, so the decision does not
have to be made again from scratch.

This is additive to
[§4 of the standards alignment](./2026-09-13-standards-alignment.md), not a
competitor to it. Nothing here changes the `stats`/`defs`/`group` promotion
that section plans, and nothing here depends on that promotion landing
first.

## Summary of the change

| Change | Kind of thing |
|---|---|
| `tree` | a new leaf kind |
| `note` | a modifier on `box`, `result`, `rail` |
| `accent` | a modifier on `box`, `result`, `rail`, `group` |
| `foot` | a modifier on `group` |
| `detail` | a third field on a stat tile |
| `wide` | a figure-level flag |

Ten kinds instead of nine. No existing markup changes: every addition is an
appended element or an added class, so a figure that uses none of this emits
the bytes it emits today.

## 1. A panel that needs metadata is a `group`

A side-by-side layout often wants a title, an accent flag and a footnote on
each *panel*, which leaves the question of where they live. They live on
`group`, because
[the fig design](./2026-09-12-fig-fence-design.md) already said so: *"A panel
that needs a heading is a `group`; there is no separate panel kind."*

Extending that one sentence answers four separate needs:

| Need | Answer |
|---|---|
| a panel title | the group's title, which works today |
| a panel footnote | `foot` on `group` |
| an accented panel | `accent` on `group` |
| an accented lane | the same, because a lane that needs metadata is a `group` |

The last row is the one that earns the rule. `lanes` is `[][]figItem` — a
lane is a list, not an item, so there is nowhere to hang a flag. Carrying a
per-lane flag would mean respelling `lanes` as a list of maps, which is a
breaking change to a shipped fence language. Routing it through `group`
costs nothing and keeps `lanes` exactly as it is.

So `fig-panel` gains no metadata at all. It stays what it is: a weight
carrier.

::: card
**Why this is the right answer and not merely the cheap one.** Panel
metadata on the layout primitive would mean a title could be written two
ways — on the panel, or on the group inside it — with no rule for which
wins. One spelling for one thing is worth more than the two lines of YAML
the shortcut would save.
:::

## 2. The `tree` kind

An indented hierarchy where individual lines carry a de-emphasis or accent
marker and a trailing note. Nothing in `box`/`arrow`/`result`/`rail`/`group`/
`chain`/`lanes` expresses nesting-by-indentation, yet tree-shaped content —
repo layouts, config hierarchies — is among the most common things a design
document draws.

### 2.1 Why a block scalar and not nested YAML

A tree's value is a block scalar, one line per node:

```yaml
items:
  - tree: |
      fig.go -- the fence branch, the YAML model, validation
      figrender.go -- per-kind HTML emitters
      * figtree.go -- new: the tree line grammar
      - testdata/ -- fixtures, not shipped
        figures.md -- the rendered gallery
```

The alternative — each node a map of `label`/`note`/`accent`/`items` — needs
no new grammar at all, and was rejected on authoring cost. A twelve-node
repo layout is twelve lines here and roughly forty lines nested six levels
deep as YAML maps. That cost would be paid on the most common shape this
kind exists to draw.

It is worth being explicit about the tension with the original design, which
rejected *"a hand-rolled indentation format"* for the fence body as a whole.
That rejection stands and this does not contradict it: the fence body is a
tree of heterogeneous typed items needing lists, quoting and nesting, which
is where a hand-rolled parser diverges from what authors expect. A tree line
is one-dimensional — a depth, a label, an optional note, an optional flag —
and its indentation convention is the one authors already use when they type
a directory listing by hand.

### 2.2 The line grammar

- **Depth** is a stack of indent widths, measured from a line's leading
  whitespace alone — a marker is not part of the indent, so `* foo` and
  `foo` at the same indent are siblings. An indent deeper than the top of
  the stack pushes a level; equal is a sibling; shallower pops to a matching
  level. This accepts two-space, four-space or any consistent width without
  needing to know which. YAML strips a block scalar's common indent before
  we see it, so the first line is naturally the baseline.
- **Markers** are `* ` (accent) and `- ` (muted), recognised only when
  followed by a space.
- **The separator** is the first space-surrounded ` -- ` on the line that is
  not suppressed by a backslash (§2.3). Everything before it is the label,
  everything after is the note; both are trimmed. A line with no separator
  is all label.
- **Blank lines** are skipped.

The space requirement is not cosmetic. It is what stops the two sigils from
colliding with the content trees are made of:

```
*_test.go              a literal glob, not an accented "_test.go"
md2html --fragment -- writes a bare fragment
                  ^ splits here, because --fragment has no trailing space
```

Both collisions are live for this repo's own documentation, not
hypothetical.

### 2.3 Escaping, and why it costs no code at all

A backslash before a sigil suppresses it: `\--` is a literal separator, `\*`
a literal marker.

The rule needs no implementation. Both sigils are matched as literal strings
— `" -- "` and a `"* "` prefix — and a backslash breaks those strings by
sitting inside them, so an escaped sigil simply fails to match. Nothing has
to look behind, and nothing has to strip anything: CommonMark escapes any
ASCII punctuation, and every author-facing field already renders through a
real goldmark inline pass, so the backslash is removed downstream by
machinery that already exists.

The escape therefore costs zero lines in the grammar, and an author who
knows Markdown's escape already knows ours.

### 2.4 Markup

Real nested lists, following the precedent that `defs` emits a real `<dl>`
rather than a pile of divs:

```html
<ul class="fig-tree">
  <li><span class="fig-tree-label">fig.go</span><span class="fig-note">the fence branch</span>
    <ul><li class="fig-muted">…</li></ul>
  </li>
  <li class="fig-accent">…</li>
</ul>
```

Only the outer list carries `fig-tree`. `fig-note` is the same class a box
note uses — one styling concept, one class, whichever primitive it hangs on.

A tree with no content renders an empty list and raises no warning, matching
the shipped rule that empty text on an otherwise valid item is a legitimate
blank spacer.

### 2.5 Faults

A tab in a tree's indentation is a fault, because it makes depth ambiguous
against spaces. A dedent to a level that is not on the stack is a fault.

Both follow the existing validation tier: no YAML line number is available,
so the message names the item path and the tree's own line —
`items[2].tree line 4: indented to no enclosing level` — and the whole fence
degrades to a code block with exactly one warning, as every other fig fault
already does.

### 2.6 Where parsing happens

Twice. Once during validation, so a malformed tree degrades the fence before
any markup is written, and once in the renderer for the nodes themselves.

The alternative is for validation to return the parsed nodes and for the
renderer to consume them, which would make `figItem` carry derived state
rather than being a pure decode target. Parsing a dozen lines of text twice
costs microseconds; the struct's single job is worth more.

## 3. Modifiers

| Field | Legal on | Rejected where illegal by |
|---|---|---|
| `note` | `box`, `result`, `rail` | validation |
| `accent` | `box`, `result`, `rail`, `group` | validation |
| `foot` | `group` | validation |
| `detail` | a stat tile | the decoder, via `figStat` |
| `wide` | the figure | the decoder, via `figDoc` |

Positional legality is validation's job, not `KnownFields`'. One struct
carries every kind's fields, so the decoder cannot tell where a key is
legal — the same split the shipped code already uses for `weight` and
`items`. `note` on an `arrow` is a fault; `foot` on a `box` is a fault.

`note`, `foot` and `accent` are plain `string`/`bool` rather than pointers.
The absent-versus-empty distinction that makes every kind key a `*string`
does not apply to a modifier: a modifier names no kind, so an empty one and
a missing one mean the same thing and render the same way.

**`note`** is a second short piece of text at a different visual weight —
the muted "why" beside the headline. It is explicitly *not* a composability
request, and §4.2's rule that diagram primitives stay inline-only is
untouched: a box may carry two short strings, never a paragraph or a list.
Chain steps get `note` for free, because a step is an item.

**`accent`** adds a class rather than changing an element. It is one flag
covering three needs that could have been three features: a highlighted box, an
accented panel, an accented lane.

**`foot`** renders after a group's items as a trailing line distinct from
them.

**`detail`** is the stat tile's third line. §4 will eventually give `stats` a
container form with full block Markdown, which supersedes the need for a
named key — but that promotion is additive and keeps the `fig` form working,
so a `detail` field on the `fig` form is permanent value rather than
scaffolding.

## 4. `wide`

A figure-level flag that lets a figure break out of the measure column:

```yaml
wide: true
layout: cols
items: [...]
```

It emits `<figure class="fig fig-wide">`. The stylesheet widens it with
negative inline margins clamped so it can never exceed the viewport, and the
existing narrow-width media query collapses the breakout along with
everything else.

Under `--fragment` there is no measure column to break out of, so the
breakout must degrade to no visible change rather than to a figure wider
than its host. The clamp is what guarantees this; it is not a separate code
path.

## 5. What we are declining

Recorded so the decision does not get relitigated.

- **A row-level `weights: 2 3` shorthand.** Sugar over the shipped
  `weight: 1-12`, which already has the underlying capability.
- **A `compact` flag on a chain.** Per-figure spacing taste with no content
  meaning. `fig` already refused this class of knob once, when it made arrow
  direction follow the enclosing layout rather than the author, on the
  grounds that one fewer per-figure decision is one fewer way to draw
  something incoherent. That reasoning has not changed.
- **A distinct full-width prose-block kind.** A rows-layout figure of `box`
  followed by `lanes` already *is* "a full-width block above a side-by-side
  comparison" — `lanes` is a side-by-side layout expressed as an item rather
  than as a document-level `layout`. The residual difference is whether a
  `box` renders a paragraph of body text legibly, which is a stylesheet
  question about an existing kind rather than a missing capability, and is
  handled in §7.

## 6. What already works

Two neighbouring needs require no code.

- **Definition values carrying inline Markdown.** Every author-facing text
  field in a figure already renders through a real goldmark inline pass, so
  links, code spans and chips inside a `def` render today. §4's promotion
  would add *block* Markdown, which is more than was asked for.
- **`group` as a labelled container.** Planned in §4 and unchanged by
  anything here.

## 7. Layout and stylesheet

```
fig.go            gains tree as a kind, four modifier fields, their validation
figrender.go      gains the tree emitter and the modifier emissions
figtree.go        new: the line grammar
figtree_test.go   new: the grammar's unit tests
default.css       .fig-tree, .fig-note, .fig-accent, .fig-muted, .fig-wide
```

A third file rather than a third responsibility in `fig.go`. `fig.go` is 283
lines and `container.go`'s 347 is this repo's established ceiling for a
feature module; the line grammar is self-contained, with its own failure
modes and its own tests. This is the same reasoning that split
`figrender.go` off from `fig.go` in the first place.

The stylesheet additions are built from existing tokens — `--accent`,
`--rule`, `--muted` — so they inherit all four theme states without defining
a colour, exactly as the shipped `fig` block does. `fig-wide` needs one new
custom property for the breakout distance so a caller's `--css` replacement
can reinterpret it, following the precedent set by `--fig-weight`.

While the stylesheet is open, the rendered gallery gets checked for whether
a `box` holding a paragraph of body text reads acceptably, per §5. If it
does not, the fix is line-height and alignment on `.fig-box`, not a new kind.

## 8. Testing

TDD throughout, as everywhere else in this repo.

- **`figtree_test.go`** — the grammar as a unit: two-space and four-space
  indents, a dedent to a level that does not exist, a tab, each marker, an
  escaped marker, an escaped separator, and specifically the two collision
  cases this design exists to survive (`*_test.go` stays a literal glob;
  `md2html --fragment -- note` splits at the second `--`).
- **`fig_test.go`** — positional legality: `note` on an `arrow`, `foot` on a
  `box`, `wide` on an item rather than the figure. Each asserts the warning
  text and the code-block fallback, not merely that something failed.
- **`figrender_test.go`** — golden markup for tree nesting and for each new
  field, plus one test asserting that a box *without* a note is byte-
  identical to today's output. That test is the guard on "purely additive".
- **A full-`Convert` test** that a `.md` link inside a tree note is rewritten
  and returned by `ExtractLinks`, proving a tree's text reaches the
  transforms like every other field.
- **`testdata/figures.md`** gains a tree section, the modifier cases, and a
  wide figure. It is the page a maintainer opens in a browser after touching
  the stylesheet, which is the job markup assertions cannot do.
- **`testdata/conformance.md`** gains a tree, with a matching entry in
  `conformanceChecks`.
- **The existing `fig` browser test** gains one assertion: a wide figure's
  bounding box is wider than a sibling paragraph's. That is the second thing
  in this feature observable only in a browser, and it rides the harness
  that already exists rather than adding a test of its own.

## 9. Documentation

- `docs/authoring.md`'s "Structured figures" section gains `tree` and the
  modifiers. Its examples stay in `yaml` fences, because a `fig` fence in a
  document renders.
- The `md2html-authoring` skill's quick-reference table gains a row.
- §4 of the standards alignment gains a note recording that panel metadata
  resolved onto `group`, so that section's author does not rediscover the
  question.

## 10. Alternatives considered

- **Nested YAML nodes for `tree`.** No new grammar, full `KnownFields`
  coverage, validation paths that work unchanged — and roughly three times
  the lines for the shape that appears most often. Rejected on authoring
  cost, which is the whole reason this primitive exists.
- **A `tree` container instead of a `fig` kind.** §4's own doctrine —
  content wants composability, layout wants declaration — puts nesting-by-
  indentation on the content side, because unlike a box-to-arrow
  relationship it *is* expressible as a Markdown list. A `::: tree`
  container holding a nested list would need no new grammar at all. It is
  ruled out by the evidence rather than by the doctrine: trees appear inside
  the columns of side-by-side layouts, and a container cannot nest inside a
  fence.
- **A rarer separator than ` -- `.** Surveyed. ` :: ` collides with
  qualified names and rhymes visually with this repo's own `:::` containers.
  ` | ` is the character people type when hand-drawing tree glyphs. ` # `
  reads as a comment and invites YAML comment expectations that do not apply
  inside a block scalar. ` ~ ` and ` @ ` are collision-free precisely because
  they signify nothing to a reader. An em dash is semantically exact and
  effectively collision-free, but costs non-ASCII input in source. No
  candidate is collision-free; ` -- ` is the most legible, and the space rule
  plus a borrowed escape closes what it leaves open.
- **Panel-level metadata on `fig-panel`.** Rejected in §1: it would give a
  panel title two spellings with no rule for which wins.
- **Respelling `lanes` as a list of maps** so a lane could carry its own
  flag. A breaking change to a shipped fence language, to buy something
  `group` already provides.

## Out of scope

Unchanged from the original design: computed graph layout, user-defined
kinds, images inside figures, any runtime JavaScript, and cross-document
navigation. Block Markdown inside diagram primitives stays out too — §4.2's
rule is a constraint this design deliberately keeps.
