# Changelog

Notable changes to md2html, newest first. A version is a git tag, and the
same string is stamped into every generated file's provenance marker, so
the version a page claims and the version that wrote it are always the
same.

This is 0.x: the exported API is not stable yet, and a change marked
**breaking** can land in a minor release. From v1.0.0 on, one needs a new
major version instead.

## Unreleased

### Added

- `md2html.InstallSkill` installs the Claude Code authoring skill into a
  skills directory, and `--install-skill-user` / `--install-skill-project`
  expose it. The skill now travels inside the binary along with a copy of
  `docs/authoring.md` it defers to, so the guidance always matches the
  version installed, which a link to this repo's `main` would not. Neither
  flag creates the `.claude` directory it writes under, and the refusal
  names that directory in full. Both files carry the usual provenance
  marker, so a re-install replaces this tool's own copy silently and refuses
  one you have edited — refusing the whole install rather than leaving half
  of it in place, which is why the check lives with the files rather than in
  the command.

### Fixed

- The browser suite's first test no longer fails on a cold CI runner. Its
  15s per-step budget also had to cover launching Chrome, which on a fresh
  runner left nothing for the test itself: the first test in the file, and
  only ever that one, failed at exactly the deadline. `TestMain` now
  launches and discards a browser before any test runs, so that cost is paid
  once outside every deadline, and the per-step budget went to 60s.
- That test asserts the image became expandable before clicking it, in its
  own step, rather than reading the class alongside the click. A regression
  in the wiring used to hang until the deadline waiting for a dialog nothing
  would open; it now fails in under a second, by name.

### Repository

- CI moved to `actions/checkout@v7` and `actions/setup-go@v7`; the previous
  majors target the deprecated Node 20 and were being force-upgraded.

## v0.2.0 — 2026-09-13

### Added

- `Options.MermaidURL` names the ES module a page with a diagram imports at
  view time. Empty keeps the pinned CDN build, so nothing changes for a
  caller who does not set it. This is the supported way to serve MermaidJS
  yourself, for a docs build that must not reach a CDN.

### Changed

- **Breaking (library only).** `CrawlOptions.LinkDepth` now reads exactly as
  `CrawlOptions.Depth` does: `-1` is unlimited, and a non-negative value is
  the bound itself, so `0` follows no links at all. It previously inverted
  that — `0` meant unlimited and `-1` meant none. The CLI is unaffected:
  `--link-depth` still defaults to unlimited, having changed default
  alongside it. A caller relying on the zero value to follow links without
  limit must now say `LinkDepth: -1`.
- `Options.Warn` reaches every diagnostic whatever `Transforms` holds.
  Convert rebuilds the builtins that report against the sink before running
  them, so a caller who appends to `Builtins()` — the documented way to add
  a transform — no longer silently loses container warnings for not having
  known to pass the sink to `Containers` themselves. The caller's own slice
  is never written to.
- The browser suite moved to `e2e/`, its own Go module, and lost its
  `e2e_browser` build tag; the module boundary is the opt-in now. Run it
  with `just e2e`. This takes chromedp out of the library's requirements:
  nothing the library builds or tests compiles it. It remains an indirect
  entry in `go.mod`, inherited from `go.abhg.dev/goldmark/mermaid`.
- The `go` directive is `1.26`, down from a patch-pinned `1.27.1`. 1.26 is
  the floor the dependency graph actually imposes.
- `mermaidCDN` is a constant. It was a var only so the browser suite could
  reassign it, which `Options.MermaidURL` now covers properly.

### Fixed

- The README described two sources of conversion warnings; there are three
  (the `fig` fence's has been there since the fence shipped).

### Repository

- GitHub Actions runs `gofmt`, `go mod tidy -diff`, `go vet`, `go test
  -race` and a CLI build on every push and pull request, plus the browser
  suite against the runner's Chrome.
- `just test` and `just e2e` join `just install`.
