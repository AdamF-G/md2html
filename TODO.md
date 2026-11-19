# TODO

Open work not tied to a plan. A plan's deferred review findings go in its
follow-ups file under `docs/plans/` instead.

## Bracketed spans

- [ ] Keep a span's `title`, so a chip can carry a hover explanation:
  `[beta]{.chip title="May change before 1.0."}` should render
  `<span class="chip" title="May change before 1.0.">`. `parseAttrs`
  already reads key=value pairs; `spanNode` (`chip.go`) keeps only the id
  and classes. Decide which other keys are safe the way `linkattrs.go` does
  for links, and update the guide's "only classes and an id are kept"
  (`docs/guide/inline-marks.md`). Until then an author writes raw
  `<span class="chip" title="…">`.
- [ ] A span at the end of a heading becomes the heading's attributes.
  goldmark's heading-attribute parser (`parser.WithAttribute`) claims a
  trailing `{…}` before Chips sees it, so `## V [z]{.chip}` renders
  `<h2 class="chip">V [z]</h2>` and `# T [x]{.chip title="a b"}` puts the
  title on the `<h1>`. The guide says chips work "in prose or a heading",
  which holds only for the bare `[proven]` form. A span mid-heading works.
  A fix has to tell a span's block from a heading's own `{#id .class}`
  (a `]` just before the `{`, say) without breaking existing heading
  attributes.

## Build

- [ ] A `--strict` flag that fails the build on any warning, so CI can
  refuse a page with a broken link, an asset that does not exist, or an
  attribute block left literal. Today every authoring mistake warns and the
  build carries on and exits 0, so the summary's warning count is the only
  signal.
