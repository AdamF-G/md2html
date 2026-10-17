# Site-Path Links Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In a mounted site, a link a reader follows that reaches nothing on disk, or reaches an excluded file or directory, is read from the linking page's site path against the mounts' tree; a directory link out of the site follows `unmounted:`; and a link sent out of the site while the site has a page at its path says so.

**Architecture:** `siteRules` gains one table of the site's pages by site path (`pages`), indexed with the existing `dirs` by a new `indexPages`, and one lookup, `inSite`, that is the note's step 2. `crawlMounts` builds its pages' places before it reads any link, indexes them, and passes `discover` a hook so a link step 2 finds is not reported missing. `siteRules.action` takes the site directory a link is read from (`fromSite`): a page's own directory, or the root for the shell, and consults `inSite` where a link reaches nothing or an excluded target. `leaving` learns directories and the shadowed-link clause.

**Tech Stack:** Go 1.26; `golang.org/x/net/html`; the existing crawl (`crawl.go`), link rules (`sitelinks.go`) and shell rendering (`siteshell.go`).

**Spec:** `docs/specs/2026-10-16-site-path-links-design.md` §4 (the rule, where it applies, directories) and §5 (the excluded-file exception, the shadowed link). §2's alternatives and §3's principles decide nothing here beyond what §4 and §5 say.

## Global Constraints

- Work on the `site-path-links-spec` branch, where the spec is, in the user's checkout. Conventional prefixes. Commit bodies wrapped at 72 columns, starting from what was wrong before. No attribution trailer. Nothing is pushed.
- **A build without mounts is unchanged**, with or without a shell: every existing test passes untouched, except where a task says it edits one. `inSite` answers false whenever `r.mounts == nil`.
- Step 1 wins: a link whose target exists on disk and is not excluded keeps today's meaning. Step 2 runs only for a link a reader follows (`!l.Loaded`); an asset a page loads is resolved on disk only.
- Step 2 is never ambiguous and never guesses: an absolute href, and one whose site path climbs above the root, takes step 1 only.
- The shadowed-link clause, exactly: `; the site has a page at <at>, which the file shadows` (`the directory` for a directory). The new `unmounted: link` warning, exactly: `linked <href> to the repository's host, since the file is outside the site; the site has a page at <at>, which the file shadows`.
- Every warning md2html raises has an entry in `docs/guide/warnings.md` (`TestEveryWarningIsInTheGuide`).
- Run `gofmt -l .` (empty), `go vet ./...` and `go test ./...` before every commit.

## Review Focus

1. **A directory link to the source root itself**, such as the README's `./` when the root mount is `docs/`: under `unmounted: link` its `{path}` must be empty, not `.`, giving `…/blob/main/`. Pinned in Task 4.
2. **A page that cannot be read**: it was indexed before discovery, so a link to it must not be rewritten to an output that is never written. Pinned in Task 2.
3. **The shell's link written for the site, on a page deep in the tree**: it resolves from the root, not from the page, and is not rebased to the Markdown file with "which the build does not write". Pinned in Task 2.
4. **`x/README.md` where `x` has no README**: it must not fall back to the directory's written page, only `x/index.md` does. Pinned in Task 1.
5. **An excluded file whose link names a site path with no page**: it must still follow `unmounted:` with today's message, and no shadowed clause. Pinned in Task 3.

Out of scope: `nav-sort: links` ranks an index's links by where they resolve on disk, and a site-path link in an index does not rank its target. The spec does not ask for it.

---

### Task 1: The site's page table and step 2

**Files:**
- Modify: `sitelinks.go` (the `siteRules` struct, new methods)
- Modify: `crawl.go:631-646` (the `dirs` loop in `buildLinkMaps` becomes a call)
- Create: `sitepathlinks_test.go`

**Interfaces:**
- Produces: field `pages map[string]string` on `siteRules`: a page's site path without its extension (`"reference/engine"`, `"index"`, `"notes/README"`) to its `Doc.Out`. `func (r *siteRules) indexPages(docs []Doc)`, which sets `pages` and `dirs` (as the loop in `buildLinkMaps` does today) from every doc with a `SitePath`. `func (r *siteRules) inSite(l Link, fromSite string) (out, at string, ok bool)`: `fromSite` is a slash site directory (`""` is the root); `out` is the absolute output; `at` is how a report names it: the stem path for a page, the directory path with a trailing `/` for a directory's page, `the site's root` for the root's.

- [ ] **Step 1: Write the failing tests** in `sitepathlinks_test.go`. `TestInSite` builds a `siteRules` by hand, `r := &siteRules{outDir: "/out", mounts: []mountRule{{}}}`, and calls `r.indexPages` on these docs:

```go
docs := []Doc{
	{SitePath: "index.md", Out: "/out/index.html"},
	{SitePath: "guides/deploy.md", Out: "/out/guides/deploy.html"},
	{SitePath: "reference/index.md", Out: "/out/reference/index.html"},
	{SitePath: "reference/engine.md", Out: "/out/reference/engine.html"},
	{SitePath: "notes/README.md", Out: "/out/notes/index.html"},
	{SitePath: "both/index.md", Out: "/out/both/index.html"},
	{SitePath: "both/README.md", Out: "/out/both/README.html"},
	{SitePath: "bare/page.md", Out: "/out/bare/page.html"},
}
```

Table cases, `fromSite` `"guides"` unless given, each a `Link{Href: …}` asserting `(out, at, ok)`:

| href | out | at | ok |
|---|---|---|---|
| `../reference/engine.md#options` | `/out/reference/engine.html` | `reference/engine` | true |
| `../reference/index.md` | `/out/reference/index.html` | `reference/index` | true |
| `../reference/` and `../reference` | `/out/reference/index.html` | `reference/` | true |
| `../notes/README.md` | `/out/notes/index.html` | `notes/README` | true |
| `../notes/index.md` | `/out/notes/index.html` | `notes/` | true |
| `../both/README.md` | `/out/both/README.html` | `both/README` | true |
| `../bare/index.md` | `/out/bare/index.html` | `bare/` | true |
| `../bare/README.md` | | | false |
| `../../x.md` | | | false |
| `/reference/engine.md` | | | false |
| `../reference/missing.md` | | | false |
| `my%20page.md` with a doc at `guides/my page.md` | its out | `guides/my page` | true |
| `reference/engine.md` with `fromSite` `""` | `/out/reference/engine.html` | | true |
| `../` from `"guides"` | `/out/index.html` | `the site's root` | true |

Also assert false for `Link{Href: "../reference/engine.md", Loaded: true}`, and for every case above when `r.mounts` is nil.

- [ ] **Step 2: Run** `go test -run TestInSite .`. Expected: FAIL, `r.indexPages undefined`.

- [ ] **Step 3: Implement `indexPages` and `inSite` in `sitelinks.go`**, and replace the `dirs` loop in `buildLinkMaps` with `res.rules.indexPages(res.Docs)` when `res.rules.mounts == nil` (Task 2 indexes mounted sites in the crawl). The order `inSite` reads the href in: strip from the first `#` or `?`, `url.PathUnescape`, refuse empty or absolute; remember a trailing `/`; `path.Join(fromSite, p)`, refuse `..` or a `../` prefix, and treat `.` as `""`. A path with a trailing `/` or no extension is a directory: `dirs[p]` gives `outDir/p/index.html`. A Markdown path looks up `pages[stem]` first, then, only when its base stem is `index` (`isIndexStem`), `dirs[sitePathDir(p)]`.

- [ ] **Step 4: Run** `go test ./...`. Expected: PASS, with every existing site test unchanged.

- [ ] **Step 5: Commit** `feat: a site's rules index its pages by site path`.

### Task 2: Links that reach nothing on disk are read in the site

**Files:**
- Modify: `crawl.go` (`discover`, its two callers, `crawlMounts`, `buildLinkMaps`, `planLinks`)
- Modify: `sitelinks.go` (`action`)
- Modify: `siteshell.go:60` (the per-page shell plan)
- Test: `sitepathlinks_test.go`

**Interfaces:**
- Consumes: `indexPages`, `inSite` (Task 1).
- Produces: `func discover(cur string, res *CrawlResult, follow func(l Link, target string), found func(l Link) bool) (discovered, bool)`; `found`, when not nil, is asked about a link with no file on disk before "link target does not exist" or "referenced asset does not exist" is said, and true means silence. `func (r *siteRules) action(l Link, fromOut, fromSite string, outBySrc map[string]string) linkAction`. `func planLinks(links []Link, fromOut, fromSite string, outBySrc map[string]string, opt CrawlOptions, rules *siteRules) linkPlan`. A page's `fromSite` is `sitePathDir(d.SitePath)`; the shell's is `""`, at the crawl and on every page.

- [ ] **Step 1: Write the failing tests** in `sitepathlinks_test.go`, using `mountCrawl` with the spec's example. The shell body is `[[mount source="."]]\n\n[[mount path="reference" source="../../../build/scratch/reference"]]`, and the files are:

```go
var sitePathFiles = map[string]string{
	"repo/docs/site/index.md":          "# Home\n",
	"repo/docs/site/guides/deploy.md":  "# Deploy\n\n[ref](../reference/index.md) [dir](../reference/) [bare](../reference) [opts](../reference/engine.md#options) [up](../../x.md) [abs](/reference/index.md)\n",
	"repo/docs/site/guides/tuning.md":  "# Tuning\n",
	"build/scratch/reference/index.md":  "# Reference\n",
	"build/scratch/reference/engine.md": "# Engine\n\n[tuning](../guides/tuning.md)\n",
}
```

- `TestSitePathLinksAcrossMounts`: in deploy's `AnchorLinkMap`, `../reference/index.md`, `../reference/` and `../reference` each map to `../reference/index.html`, and `../reference/engine.md#options` maps to `../reference/engine.html#options`. Engine's `../guides/tuning.md` maps to `../guides/tuning.html`. `res.Warnings` holds exactly two "does not exist" warnings, for `../../x.md` and `/reference/index.md`, and none for the others.
- `TestSitePathLinksNeedTheMount`: the same files with only `[[mount source="."]]` warn `link target does not exist: ../reference/index.md` and `referenced asset does not exist: ../reference/`.
- `TestSitePathLinkFromTheShell`: with `[Ref](reference/index.md)` added to the shell body, `mountPage` for `repo/docs/site/guides/deploy.md` renders `href="../reference/index.html"`, the root page renders `href="reference/index.html"`, and neither page's warnings, nor `s.Warnings`, contain `which the build does not write`.
- `TestSitePathLinkToAnUnreadablePage`: with `build/scratch/reference/engine.md` made unreadable (`os.Chmod(p, 0)`, skipped when running as root), deploy's `../reference/engine.md#options` is in neither `AnchorLinkMap` nor `AnchorLinkDrops`.
- `TestSitePathLinksWithoutMounts`: `siteCrawl` with `docs/index.md` linking `[s](sub/index.md)` and `docs/sub/page.md` still warns `link target does not exist: sub/index.md`.

- [ ] **Step 2: Run** `go test -run 'TestSitePathLink' .`. Expected: FAIL, with the cross-mount links reported as missing.

- [ ] **Step 3: Implement.** `discover` takes `found` (the link-following `Crawl` passes nil). `crawlMounts` builds each source's `Doc{Src, Out, SitePath, mount}` in `order` first, runs `readmesAtIndex` on them, and calls `rules.indexPages`. Then it discovers each one with `found` set to `inSite` from that doc's directory, filling `Meta` and `Title`. When a page is unreadable and dropped, it re-runs `indexPages(res.Docs)`, so the table holds only pages that get written. In `action`, a link with nothing on disk (`os.Stat` fails) returns `rewrite: relHref(fromOut, out) + fragmentOf(l.Href)` when `inSite` finds it, and the zero action otherwise. Thread `fromSite` through `planLinks` from `buildLinkMaps` (pages and shell) and `siteshell.go`.

- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.

- [ ] **Step 5: Commit** `feat: a mounted site reads a link that reaches nothing from its site path`.

### Task 3: An excluded file or directory gives way to the site's page

**Files:**
- Modify: `sitelinks.go` (`action`'s excluded branch)
- Test: `sitepathlinks_test.go`

**Interfaces:**
- Consumes: `inSite` and `action`'s `fromSite` (Tasks 1–2).

- [ ] **Step 1: Write the failing test** `TestSitePathLinksPassExcludedTargets`. Use the Task 2 files plus `repo/docs/site/reference/engine.md` ("# Template\n") and `repo/docs/site/reference/old.md`, with the root mount `exclude="reference"` and `repo-url: https://example.com/blob/main/{path}` in the shell. Assertions:
  - tuning's `[e](../reference/engine.md)` maps to `../reference/engine.html`;
  - `[r](../reference/)` maps to `../reference/index.html`;
  - `[o](../reference/old.md)`, with no generated page, maps to `https://example.com/blob/main/repo/docs/site/reference/old.md`, with no warning that contains `shadows`.

  `TestMountCrawlExcludedTargetsAreOutsideTheSite` passes unchanged.

- [ ] **Step 2: Run** `go test -run TestSitePathLinksPassExcludedTargets .`. Expected: FAIL; the engine link goes to the repository's host.

- [ ] **Step 3: Implement.** In a mounted site, when `excluded` and `!l.Loaded`, check `inSite` before anything else, file or directory, and rewrite to its page when it is found. Otherwise the code continues as today.

- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.

- [ ] **Step 5: Commit** `feat: a link to an excluded file reaches the site's page at its path`.

### Task 4: A directory outside a mounted site follows `unmounted:`

**Files:**
- Modify: `sitelinks.go` (`action`'s directory branch, `leaving`)
- Test: `sitepathlinks_test.go`

**Interfaces:**
- Produces: `func (r *siteRules) leaving(l Link, target, fromSite string) linkAction`. Task 5 adds the clause. Every caller passes `fromSite`.

- [ ] **Step 1: Write the failing test** `TestMountedSiteDirectoryLinksLeaveTheSite`, using `mountCrawl` with `mountShell` and `mountFiles`. The README gains `[root](./) [notes](./notes/)` and the file `repo/notes/n.md`. The cases:
  - under `repo-url: https://example.com/blob/main/{path}`, `./` maps to `https://example.com/blob/main/` and `./notes/` to `https://example.com/blob/main/notes`;
  - without `repo-url:`, both are in `AnchorLinkDrops`, each with the `unlinked …, which is outside the site` warning;
  - under `unmounted: error`, the crawl fails, naming `./notes/`;
  - a directory the site holds with no page, `[img](img/)` from `sites.md`, stays as written.

  Set the shell's front matter through `files["repo/docs/site/shell.md"]`, as `TestMountCrawlExcludedTargetsAreOutsideTheSite` does.

  `TestSiteCrawlLinksADirectoryToItsPage` (no mounts) passes unchanged.

- [ ] **Step 2: Run** `go test -run TestMountedSiteDirectoryLinksLeaveTheSite .`. Expected: FAIL; the directory links are left as written.

- [ ] **Step 3: Implement.** In the directory branch, a mounted site whose `place(target)` is false sends a `!l.Loaded` link to `leaving`. In `leaving`, a `relUnder` of `.` fills `{path}` with `""`.

- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.

- [ ] **Step 5: Commit** `feat: a directory link out of a mounted site follows unmounted:`.

### Task 5: A shadowed link names the page it hides

**Files:**
- Modify: `sitelinks.go` (`leaving`)
- Modify: `docs/guide/warnings.md` (the `unlinked …, which is outside the site` entry, and a new entry after it)
- Test: `sitepathlinks_test.go`

**Interfaces:**
- Consumes: `inSite` (Task 1), `leaving`'s `fromSite` (Task 4).

- [ ] **Step 1: Write the failing test** `TestShadowedLinkNamesThePage`. Use the Task 2 files plus a stale `build/scratch/guides/tuning.md` that no mount holds. Engine's `../guides/tuning.md` now finds that file on disk:
  - under `unlink`, it is dropped with the warning `unlinked ../guides/tuning.md, which is outside the site; its text is kept; the site has a page at guides/tuning, which the file shadows`;
  - under `repo-url: https://example.com/blob/main/{path}`, it maps to `https://example.com/blob/main/build/scratch/guides/tuning.md` with the warning `linked ../guides/tuning.md to the repository's host, since the file is outside the site; the site has a page at guides/tuning, which the file shadows`;
  - under `unmounted: error`, the crawl's error contains `links to ../guides/tuning.md, which is outside the site; the site has a page at guides/tuning, which the file shadows`;
  - a stale directory `build/scratch/guides/` linked as `../guides/` carries `which the directory shadows` and names `guides/`;
  - the README's `./LICENSE` from `mountFiles`, which has no page at its site path, reports exactly as before.

- [ ] **Step 2: Run** `go test -run TestShadowedLinkNamesThePage .`. Expected: FAIL; no clause.

- [ ] **Step 3: Implement the clause in `leaving`.** Ask `inSite(l, fromSite)` once; when it is found, add the clause to the `error` failure and to both `unlink` warnings. Under `link`, the found case adds the new warning beside the rewrite. In `warnings.md`, the `unlinked` entry gains a sentence on the clause, and a new entry, **linked … to the repository's host, since the file is outside the site; the site has a page at …, which … shadows**, says that a file no mount holds hides a page the site has. The fix: rename or move the file, or exclude it, after which the link reaches the page.

- [ ] **Step 4: Run** `go test ./...`. Expected: PASS, including `TestEveryWarningIsInTheGuide`.

- [ ] **Step 5: Commit** `feat: a link a file shadows names the site's page`.

### Task 6: The guide, and md2html's own site

**Files:**
- Modify: `docs/guide/sites.md` (the Mounts bullets at about lines 162-165, and the directory bullet of Links and files at about lines 186-187)

- [ ] **Step 1: Record a baseline** from the code before Task 1: `git worktree add <scratch>/base b616d86`, then, from inside it, `go run ./cmd/md2html --site docs/site/shell.md --no-source -o <scratch>/before`, then `git worktree remove <scratch>/base`. Expected: `38 written, 0 refused, 0 failed, 0 warning(s)`.

- [ ] **Step 2: Update the guide.** In Mounts, add the following after "A link between mounted pages reaches the page where it is written":
  - a link that reaches nothing on disk, or an excluded file, is read from where the page appears in the site, so a hand-written page and a generated one can link to each other by their places;
  - it breaks on the hosting page;
  - generating into the tree remains the simpler arrangement.

  In Links and files, the directory bullet keeps "left as written" for a site without mounts. It says that, in a mounted site, a link to a directory the site holds reaches its page, and one to any other directory follows `unmounted:`.

- [ ] **Step 3: Rebuild md2html's own site** into `<scratch>/after` and run `diff -r <scratch>/before <scratch>/after`. Expected: the same count and 0 warnings. Any differing page is explained by Task 4, a README directory link now going to the repository's host. Report each differing file.

- [ ] **Step 4: Run** `gofmt -l .`, `go vet ./...` and `go test ./...`. Expected: clean, PASS.

- [ ] **Step 5: Commit** `docs: the guide says how a mounted site reads a link written for it`.
