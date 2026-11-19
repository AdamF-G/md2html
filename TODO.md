# TODO

Open work not tied to a plan. A plan's deferred review findings go in its
follow-ups file under `docs/plans/` instead.

## Build

- [ ] A `--strict` flag that fails the build on any warning, so CI can
  refuse a page with a broken link, an asset that does not exist, or an
  attribute block left literal. Today every authoring mistake warns and the
  build carries on and exits 0, so the summary's warning count is the only
  signal.
