# Figure gallery

Every kind and layout the `fig` fence ships. This file exists to be looked
at: convert it and open it after touching `default.css`.

## Leaves, in a rows layout

```fig
caption: Every leaf kind
items:
  - rail: Release 2.4
  - box: A labeled box
  - arrow: a labeled arrow
  - box: ""
  - arrow: ""
  - result: An emphasized result
  - stats:
      - value: 78ms
        label: cold start
      - value: 31ms
        label: warm
      - value: 0.82 MiB
        label: vendored
  - defs:
      - term: seed
        def: a document the crawl starts from
      - term: hop
        def: one link followed from a seed
```

## Inline Markdown in labels

```fig
caption: Labels take *inline* `Markdown`
items:
  - box: "`auth` middleware"
  - arrow: "see [authoring](../docs/authoring.md)"
  - box: "status **ready**"
```

## Groups nest

```fig
caption: A group inside a group
items:
  - box: Client
  - arrow: HTTP POST
  - group: Server
    items:
      - box: Router
      - group: Handler
        items:
          - box: Validate
          - box: Persist
      - result: 200 OK
```

## Chain and lanes

```fig
caption: A chain of steps, then two parallel lanes
items:
  - chain:
      - box: Parse
      - arrow: ""
      - box: Transform
      - arrow: ""
      - box: Render
  - lanes:
      - - box: Worker 1
        - box: Convert
      - - box: Worker 2
        - box: Convert
```

## Weighted columns

The arrows here are load-bearing for review, not decoration: a `cols` layout
wraps every top-level item in a `.fig-panel`, and an arrow nested one level
deeper inside a group must still point *down* while a panel-level one points
*right*. Both directions are visible in this one figure.

```fig
caption: A 3:1 split of the row
layout: cols
items:
  - group: Wide panel
    weight: 3
    items:
      - box: Most of the row
      - arrow: then
      - box: Still the wide panel
  - arrow: ""
  - group: Narrow
    items:
      - box: The rest
```

## A labeled split

```fig
caption: Before and after
layout: split
boundary: becomes
items:
  - group: Before
    items:
      - box: Handler
  - group: After
    items:
      - box: Handler
      - box: Cache
```

## A split inside rows

A `split` is an item too, so context can sit above one and an outcome below
it. The server panel's `accent` tints its card.

```fig
caption: One trust boundary, with context either side
items:
  - rail: A request crosses one trust boundary
  - split:
      - group: Client
        items:
          - box: Browser
      - group: Server
        accent: true
        items:
          - box: Handler
          - arrow: ""
          - box: Store
    boundary: TLS
  - result: The response is signed
```

## Nested weighted columns

```fig
caption: A cols item inside a panel, with weights of its own
layout: cols
items:
  - group: Before
    items:
      - box: Handler
  - cols:
      - box: Handler
        weight: 2
      - box: Cache
```

## An accented panel

The card tints for an accented item. An accented box inside keeps its ring,
with the ring's gap drawn against the tint rather than against the page.

```fig
layout: cols
items:
  - group: Unchanged
    items:
      - box: Handler
  - group: New layer
    accent: true
    items:
      - box: Cache
        accent: true
```

## Modifiers

```fig
caption: Notes and accents
items:
  - box: Client
    accent: true
    note: retries twice, then gives up
  - result: 200 OK
    accent: true
    note: after one retry
  - rail: Phase one
    accent: true
    note: everything below ships together
```

A `note` on a `group` sits inside the title line, before its items; a
`foot` follows them. A group may carry either, or both at once:

```fig
caption: A group's note and foot, together and apart
items:
  - group: Cache warmup
    note: skipped in CI
    items:
      - box: Check
  - group: Deploy
    note: rolls back automatically
    foot: two minutes end-to-end
    items:
      - box: Build
      - arrow: ""
      - box: Ship
```

## A box holding body text

```fig
caption: A box holding body text — the case that decided against a separate prose kind
items:
  - box: >-
      A box can hold more than a short label. This one carries a full
      paragraph instead, because the design considered and declined a
      separate prose-block kind: a box that centers a word or two and a
      box that wraps several sentences are the same element, just asked
      to hold different amounts of text.
```

## Panel metadata rides on a group

```fig
caption: A panel's title, accent and footnote all live on its group
layout: cols
items:
  - group: Before
    weight: 2
    foot: the usual pattern today
    items:
      - box: Handler
      - arrow: ""
      - box: Store
  - group: After
    accent: true
    foot: one new layer
    items:
      - box: Handler
      - arrow: ""
      - box: Cache
```

## Stat tiles with a detail line

```fig
items:
  - stats:
      - value: 3 / 3
        label: unit
        detail: parse · render · write
      - value: 12
        label: suites
```

## Trees

```fig
caption: This repo's fig implementation
items:
  - tree: |
      fig.go -- the fence branch, the YAML model, validation
      figrender.go -- per-kind HTML emitters
      * figtree.go -- the tree line grammar
      - testdata/ -- fixtures, not shipped
        figures.md -- this gallery
      *_test.go -- a literal glob, not an accented line
      md2html --fragment -- splits at the second separator
```

## A tree inside a panel

```fig
layout: cols
items:
  - group: Library
    items:
      - tree: |
          fig.go
            figtree.go
  - group: Command
    items:
      - tree: |
          cmd/md2html/
            main.go
```

## A wide figure

```fig
wide: true
caption: This one breaks out of the measure column
layout: cols
items:
  - box: One
  - box: Two
  - box: Three
  - box: Four
```

## Degradation

An unknown key falls back to the source, visibly, and warns:

```fig
items:
  - boxes: typo
```
