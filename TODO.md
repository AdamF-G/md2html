# TODO

Open work not tied to a plan. A plan's deferred review findings go in its
follow-ups file under `docs/plans/` instead.

## Bracketed spans

- [ ] A span at the end of a heading becomes the heading's attributes.
  goldmark's heading-attribute parser (`parser.WithAttribute`) claims a
  trailing `{…}` before Chips sees it, so `## V [z]{.chip}` renders
  `<h2 class="chip">V [z]</h2>` and `# T [x]{.chip title="a b"}` puts the
  title on the `<h1>`. The guide says chips work "in prose or a heading",
  which holds only for the bare `[proven]` form. A span mid-heading works.
  A fix has to tell a span's block from a heading's own `{#id .class}`
  (a `]` just before the `{`, say) without breaking existing heading
  attributes.
- [ ] A styled tooltip for a chip's hover note, and perhaps for links and
  other marks. A native `title` shows after the browser's own delay, in the
  browser's own style, and no CSS can change either, so a custom tooltip
  means a component in the default stylesheet: something like `data-tip`,
  shown by `:hover` with `::after { content: attr(data-tip) }` and a
  `transition-delay`. Open questions: keyboard users, since a span takes no
  focus without `tabindex`; touch, which has no hover; screen readers, which
  don't reliably read a pseudo-element, so the note belongs in
  `aria-describedby` or `aria-label` too; clipping inside a scrolling table;
  print; and which elements it covers. Spans keeping `title`, `data-` and
  `aria-` names is the groundwork, so this adds CSS and perhaps a rewrite
  of `title` rather than any parsing.

## Build

- [ ] A `--strict` flag that fails the build on any warning, so CI can
  refuse a page with a broken link, an asset that does not exist, or an
  attribute block left literal. Today every authoring mistake warns and the
  build carries on and exits 0, so the summary's warning count is the only
  signal.
