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

## Degradation

An unknown key falls back to the source, visibly, and warns:

```fig
items:
  - boxes: typo
```
