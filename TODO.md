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
