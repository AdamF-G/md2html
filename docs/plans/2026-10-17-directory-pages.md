# Directory Pages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In every site build, each directory has a page, written at the directory's `index.html`: its `index.md`, else its `README.md`, else a page md2html writes, titled by the mount's `title` or the directory's name in title case, listing the directory's pages with a new `[[pages from="page"]]`. A link to a directory reaches its page. The "has no index.md" warning goes.

**Architecture:** `BuildSite` decides each directory's page in one pass over the pages' site paths, before the tree is built. A `README.md` stands as its directory's page when the directory has no `index.md`. A directory with neither gets a generated `Doc` at `<dir>/index.md`, holding its Markdown in a new `Doc.Body`; it joins `res.Docs`, so the tree, search, previous and next, and the command's emit loop treat it as any page. Its title comes from a mount's `title`, or from `titleCase` over the directory's name with the shell's `title-case:` spellings. Its body is a heading and `[[pages from="page" depth="1"]]`. In a site build into an output directory, the crawl writes a README that is its directory's page at the directory's `index.html` and resolves a link to a directory to that address, so every directory is reached at its own.

**Tech Stack:** Go 1.26.

**Spec:** `docs/specs/2026-10-16-mounts-design.md` §5 (Directory pages); `docs/specs/2026-10-16-order-by-links-design.md` §1 (a README that is a directory's page carries `nav-sort:`; a written page is ordered by label).

## Global Constraints

- Work on `main` in the user's checkout. Conventional prefixes. Commit bodies wrapped at 72 columns, starting from what was wrong before. No attribution trailer. Nothing is pushed.
- This changes every site build, mounted or not, as the spec says: a directory without an `index.md` now has a page instead of a label and a warning. Tests that encoded the label or the warning are updated to the new behaviour; every other existing test passes untouched. A build without `--site` is unchanged.
- A directory's page is the first of: its `index.md` (any Markdown extension, stem `index`, any case), its `README.md` (stem `readme`, any case), a written page. A README that is the directory's page is not listed again inside it.
- A written page's title: the mount's `title` when the directory is a directory mount's own (its `Path`); else the directory's name in title case — split on `-`, `_` and spaces, each word's first letter upper-cased, a word in the shell's `title-case:` list (comma-separated, matched without regard to case) spelled as listed. The root's name is its source directory's base name.
- A written page's Markdown: `# <title>`, a blank line, `[[pages from="page" depth="1"]]`.
- A written page is written at `<dir>/index.html` under the output root, and its `Src` is the same path with `index.md` (a file that does not exist; nothing reads it). A `README.md` that is its directory's page is written at `<dir>/index.html` too, in a site build into an output directory, so every directory's page is at the directory's address; the root's is the home page.
- A written page for a directory none of whose pages is shown (each is `nav: hidden`) is itself `nav: hidden`. A title that title-cases to nothing (a directory named `_`) is the directory's raw name.
- In a site build into an output directory, a link to a directory the site holds reaches that directory's page, and under `nav-sort: links` ranks it; a link to any other directory is left as written.
- `[[pages from="page"]]` lists from the current page's own node: a directory page's children; nothing for a leaf.
- Every warning md2html raises has an entry in `docs/guide/warnings.md`, and every warning the guide quotes exists (`TestEveryWarningIsInTheGuide`, `TestGuideWarningsExistInTheCode`).
- Run `gofmt -l .` (empty), `go vet ./...` and `go test ./...` before every commit.

## Review Focus

1. **`README.md` and `index.md` in one directory**: the index is the page and the README an ordinary page, whichever the crawl met first. Pinned in Task 1.
2. **A written page whose output path an asset already takes** (a raw `index.html` copied from the directory): the build must stop naming both, not overwrite. Pinned in Task 4.
3. **The root with no `index.md`**: it gets a written page at `index.html`, titled by its directory's name. Pinned in Task 4.
4. **A written page under a mount's `nav-sort="links"`**: it has no links of its own, so its directory is ordered by label. Pinned in Task 4.
5. **The command's emit loop** must convert a written page from its `Body`, never reading its nonexistent `Src`. Pinned in Task 4's CLI test.

---

### Task 1: A `README.md` is its directory's page

**Files:**
- Modify: `site.go` (`buildTree`, new helpers), `crawl.go` (`Crawl`, `crawlMounts`, new `readmesAtIndex`)
- Test: `site_test.go`, `sitecrawl_test.go`

**Interfaces:**
- Produces: `func isIndexStem(stem string) bool`, `func isReadmeStem(stem string) bool` (case-insensitive), `func sitePathDir(sp string) string` ("" at the root), `func sitePathStem(sp string) string`, `func readmesAtIndex(docs []Doc)`, used again by Tasks 4 and 5.

- [ ] **Step 1: Write the failing test** in `site_test.go`, after `TestSiteTreeWarnsOnBadNavKeys`:

```go
// A directory's README is its page when it has no index.md, and an ordinary
// page beside one, whichever the crawl meets first. A README that is the
// page carries nav-sort: as an index does.
func TestSiteREADMEIsADirectorysPage(t *testing.T) {
	s, _, _ := siteFixture(t, map[string]string{
		"shell/site.md":         minimalShell,
		"docs/index.md":         "# Home\n",
		"docs/tool/README.md":   "---\nnav-sort: links\n---\n# Tool\n\n[Zed](zed.md) [Alpha](alpha.md)\n",
		"docs/tool/alpha.md":    "# Alpha\n",
		"docs/tool/zed.md":      "# Zed\n",
		"docs/both/README.md":   "# Both Readme\n",
		"docs/both/index.md":    "# Both\n",
	}, "shell/site.md")
	want := strings.Join([]string{
		"Both",
		"  Both Readme",
		"Tool",
		"  Zed",
		"  Alpha",
	}, "\n") + "\n"
	if got := s.outline(); got != want {
		t.Errorf("outline:\n%s\nwant:\n%s", got, want)
	}
}
```

Run `gofmt -w site_test.go`.

In `sitecrawl_test.go`, after `TestSiteCrawlLeavesAnExcludedFileAlone` (find the last `TestSiteCrawl…` test):

```go
// In a site build, a README that is its directory's page is written at the
// directory's index.html, the root's as the home page, and links to it
// follow; a README beside an index stays README.html.
func TestSiteCrawlWritesAREADMEPageAtItsIndex(t *testing.T) {
	res, root, err := siteCrawl(t, "", map[string]string{
		"docs/README.md":      "# Home\n\n[tool](tool/README.md)\n",
		"docs/tool/README.md": "# Tool\n",
		"docs/both/README.md": "# Both Readme\n",
		"docs/both/index.md":  "# Both\n",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := func(rel string) string { return filepath.Join(root, "out", rel) }
	for src, want := range map[string]string{
		"repo/docs/README.md":      out("index.html"),
		"repo/docs/tool/README.md": out("tool/index.html"),
		"repo/docs/both/README.md": out("both/README.html"),
		"repo/docs/both/index.md":  out("both/index.html"),
	} {
		var got string
		for _, d := range res.Docs {
			if d.Src == filepath.Join(root, src) {
				got = d.Out
			}
		}
		if got != want {
			t.Errorf("%s written to %q, want %q", src, got, want)
		}
	}
	var home Doc
	for _, d := range res.Docs {
		if d.Src == filepath.Join(root, "repo/docs/README.md") {
			home = d
		}
	}
	if got := home.AnchorLinkMap["tool/README.md"]; got != "tool/index.html" {
		t.Errorf("a link to a README page -> %q", got)
	}
}
```

Run `gofmt -w site_test.go sitecrawl_test.go`.

- [ ] **Step 2: Run them and see them fail.**

Run: `go test -run 'TestSiteREADMEIsADirectorysPage|TestSiteCrawlWritesAREADMEPageAtItsIndex' .`
Expected: FAIL: `tool` is a label (`tool (label)`) with `Tool` as a child, ordered by label; each README is written as `README.html`.

- [ ] **Step 3: Implement.** In `site.go`, add after `lessNode`:

```go
// isIndexStem and isReadmeStem name a directory's page by the stem of its
// file, in any case: index.md, Index.markdown, README.md, readme.md.
func isIndexStem(stem string) bool  { return strings.EqualFold(stem, "index") }
func isReadmeStem(stem string) bool { return strings.EqualFold(stem, "readme") }

// sitePathDir is the directory of a site path, "" at the root, and
// sitePathStem its file's name without the extension.
func sitePathDir(sp string) string {
	if d := path.Dir(sp); d != "." {
		return d
	}
	return ""
}

func sitePathStem(sp string) string { return strings.TrimSuffix(path.Base(sp), path.Ext(sp)) }
```

In `crawl.go`, add after `checkOutputs`:

```go
// readmesAtIndex writes each README that stands as its directory's page, in
// a directory with no index, at the directory's index.html, so a reader
// reaches it at the directory's address and the root's is the home page.
// Called for a site build into an output directory, before the link maps,
// so links to it follow.
func readmesAtIndex(docs []Doc) {
	hasIndex := map[string]bool{}
	for _, d := range docs {
		if d.SitePath != "" && isIndexStem(sitePathStem(d.SitePath)) {
			hasIndex[sitePathDir(d.SitePath)] = true
		}
	}
	for i := range docs {
		d := &docs[i]
		if d.SitePath != "" && isReadmeStem(sitePathStem(d.SitePath)) && !hasIndex[sitePathDir(d.SitePath)] {
			d.Out = filepath.Join(filepath.Dir(d.Out), "index.html")
		}
	}
}
```

In `Crawl`, just before `if err := checkOutputs(res.Docs); err != nil {`, add:

```go
	if rules != nil {
		readmesAtIndex(res.Docs)
	}
```

and in `crawlMounts`, just before its `checkOutputs` call, add `readmesAtIndex(res.Docs)`.

In `buildTree`, before the loop that places each document, find which directories have an index:

```go
	// A README stands as its directory's page only where the directory has
	// no index, whichever the crawl met first, so the indexes are found
	// before any page is placed.
	hasIndex := map[string]bool{}
	for _, d := range docs {
		if d.SitePath != "" && isIndexStem(sitePathStem(d.SitePath)) {
			hasIndex[sitePathDir(d.SitePath)] = true
		}
	}
```

and in the placing loop replace

```go
		n := dir
		if !strings.EqualFold(stem, "index") || dir.doc != nil {
```

with

```go
		n := dir
		isPage := isIndexStem(stem) || (isReadmeStem(stem) && !hasIndex[dirRel])
		if !isPage || dir.doc != nil {
```

- [ ] **Step 4: Run the tests and see them pass.**

Run: `go test ./...`
Expected: PASS, every existing site test included (no existing fixture has a README without an index).

- [ ] **Step 5: Commit.**

```bash
git add site.go crawl.go site_test.go sitecrawl_test.go
git commit -F - <<'EOF'
feat: a directory's README is its page when it has no index

A directory with a README.md and no index.md was a bare label in a
site's tree, with a warning, and its README a page inside it, though a
repository's hosting page shows that README as the directory's index.

The README is now the directory's page when there is no index.md, so
it labels the directory and orders its children with its own
nav-sort:, and a site build writes it at the directory's index.html,
so it is what a reader reaches at the directory's address and a root
README is the home page. Beside an index.md it stays an ordinary
page, whichever the crawl reads first.
EOF
```

---

### Task 2: `[[pages from="page"]]`

**Files:**
- Modify: `sitenav.go` (`pagesList`)
- Test: `sitenav_test.go`

**Interfaces:**
- Produces: `from="page"` on `[[pages]]`.

- [ ] **Step 1: Write the failing test** in `sitenav_test.go`, after `TestPagesNestsBelowItsDepth`:

```go
// from="page" lists from the current page: a directory's page lists its
// children, and a leaf lists nothing.
func TestPagesFromPage(t *testing.T) {
	files := map[string]string{"shell/site.md": minimalShell,
		"docs/index.md":        "# Home\n",
		"docs/guides/index.md": "# Guides\n\n[[pages from=\"page\" depth=\"1\"]]\n",
		"docs/guides/a.md":     "# Alpha\n\n[[pages from=\"page\"]]\n",
		"docs/guides/b.md":     "# Bravo\n",
	}
	s, res, root := siteFixture(t, files, "shell/site.md")
	idx, warnings := sitePage(t, s, res, root, "docs/guides/index.md")
	if len(warnings) != 0 || !strings.Contains(idx, `<ul class="md2html-pages"><li><a href="a.html">Alpha</a></li><li><a href="b.html">Bravo</a></li></ul>`) {
		t.Errorf("warnings %q, page:\n%s", warnings, idx)
	}
	a, warnings := sitePage(t, s, res, root, "docs/guides/a.md")
	if len(warnings) != 0 || strings.Contains(a, `class="md2html-pages"`) || strings.Contains(a, "[[pages") {
		t.Errorf("a leaf's list: warnings %q, page:\n%s", warnings, a)
	}
}
```

- [ ] **Step 2: Run it and see it fail.**

Run: `go test -run TestPagesFromPage .`
Expected: FAIL: the warning `from "page" is not root or section`, and the whole tree listed.

- [ ] **Step 3: Implement.** In `sitenav.go`, `pagesList`, add a case to the `from` switch, before `default:`:

```go
	case "page":
		// The current page's own node: a directory's page lists what is
		// in it, a leaf lists nothing.
		start = page
		if start == nil {
			start = s.root
		}
```

and change the default's warning to `"[[pages]] from %q is not root, section or page; listing from the root"`. Update `TestPagesBadAttributesWarn` if it pins the old text (it checks `from "top"`, which still matches).

In `docs/guide/warnings.md`, find the entry quoting `**… is not root or section**` and change its bold text to `**… is not root, section or page**` and its explanation to say "a `[[pages]]` `from`; it lists from the root".

- [ ] **Step 4: Run the tests and see them pass.**

Run: `go test ./...`
Expected: PASS, the guide tests included.

- [ ] **Step 5: Commit.**

```bash
git add sitenav.go sitenav_test.go docs/guide/warnings.md
git commit -F - <<'EOF'
feat: [[pages from="page"]] lists the current page's own children

[[pages]] could list from the site's root or from the current page's
section, but not from the page itself, so an index page could not
list what is in its own directory without listing everything around
it.

from="page" starts at the current page: a directory's page lists its
pages and subdirectories, labelled and ordered as the tree has them,
and a leaf lists nothing.
EOF
```

---

### Task 3: Title case and `title-case:`

**Files:**
- Create: `titlecase.go`, `titlecase_test.go`

**Interfaces:**
- Produces: `func titleCase(name string, fixed map[string]string) string`; `func titleCaseWords(v string) map[string]string` (from a `title-case:` value: lower-cased word → spelling).

- [ ] **Step 1: Write the failing tests** in `titlecase_test.go`:

```go
package md2html

import "testing"

func TestTitleCase(t *testing.T) {
	fixed := titleCaseWords("API, CLI ,SONiC")
	for in, want := range map[string]string{
		"my_dir":        "My Dir",
		"my-dir":        "My Dir",
		"design notes":  "Design Notes",
		"api-reference": "API Reference",
		"Sonic":         "SONiC",
		"cli_tools":     "CLI Tools",
		"x":             "X",
		"--":            "",
		"already Upper": "Already Upper",
	} {
		if got := titleCase(in, fixed); got != want {
			t.Errorf("titleCase(%q) = %q, want %q", in, got, want)
		}
	}
	if got := titleCase("api", nil); got != "Api" {
		t.Errorf("without a list: %q", got)
	}
	if len(titleCaseWords("")) != 0 {
		t.Error("an empty title-case: gives words")
	}
}
```

- [ ] **Step 2: Run it and see it fail.**

Run: `go test -run TestTitleCase .`
Expected: FAIL to compile: `titleCase` is undefined.

- [ ] **Step 3: Implement** `titlecase.go`:

```go
package md2html

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// titleCase reads a directory's name as a title: words split on -, _ and
// spaces, each with its first letter upper-cased, and a word fixed lists,
// by its lower-cased form, spelled as listed there.
func titleCase(name string, fixed map[string]string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' || unicode.IsSpace(r) })
	for i, w := range words {
		if f, ok := fixed[strings.ToLower(w)]; ok {
			words[i] = f
			continue
		}
		r, size := utf8.DecodeRuneInString(w)
		words[i] = string(unicode.ToUpper(r)) + w[size:]
	}
	return strings.Join(words, " ")
}

// titleCaseWords reads a title-case: value, a comma-separated list of words
// that keep a fixed spelling, into the map titleCase looks them up in.
func titleCaseWords(v string) map[string]string {
	m := map[string]string{}
	for _, w := range strings.Split(v, ",") {
		if w = strings.TrimSpace(w); w != "" {
			m[strings.ToLower(w)] = w
		}
	}
	return m
}
```

- [ ] **Step 4: Run the tests and see them pass.**

Run: `go test -run TestTitleCase . && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add titlecase.go titlecase_test.go
git commit -F - <<'EOF'
feat: a directory's name reads as a title, with fixed spellings

A directory that has no page of its own is shown by its name, my_dir
or api-reference as written, which reads as a file name, not a title.

titleCase splits a name on -, _ and spaces and capitalises each word,
and titleCaseWords reads a shell's title-case: list of words that
keep a fixed spelling, such as API or SONiC, matched in any case. The
list is empty unless a shell gives one. Nothing uses them yet.
EOF
```

---

### Task 4: md2html writes a page for a directory that has none

**Files:**
- Modify: `crawl.go` (`Doc.Body`), `site.go` (`BuildSite`, new `writtenPages`, `buildTree`'s `finish`), `cmd/md2html/main.go` (the emit loop), `siteshell_test.go` (`sitePage` reads `Body`), `e2e/browser_test.go` (`buildSite` converts `Body` when set)
- Modify: `docs/guide/warnings.md` (the "has no index.md" entry goes)
- Modify: existing tests that encode a directory label or the "has no index.md" warning (listed in Step 1)
- Test: `site_test.go`, `mountsite_test.go`, `cmd/md2html/main_test.go`

**Interfaces:**
- Consumes: `isIndexStem`, `isReadmeStem` (Task 1); `[[pages from="page"]]` (Task 2); `titleCase`, `titleCaseWords` (Task 3); `CrawlOptions.Mounts`, `Mount.Title` (mounts plan).
- Produces: `Doc.Body []byte` — a generated page's Markdown; nil for a page read from its `Src`. `func (s *Site) writtenPages(res *CrawlResult, opt CrawlOptions) error`.

- [ ] **Step 1: Write the failing tests, and update the tests the change retires.**

In `site_test.go`, after `TestSiteREADMEIsADirectorysPage`:

```go
// A directory with neither index.md nor README.md gets a page md2html
// writes: titled from its name, listing its pages, in the tree, in the
// order of previous and next, and found by search. The root too.
func TestSiteWritesADirectorysPage(t *testing.T) {
	s, res, root := siteFixture(t, map[string]string{
		"shell/site.md":           "---\nsite: true\ntitle-case: API\n---\n[[content]]\n",
		"docs/api-reference/a.md": "# Alpha\n",
		"docs/api-reference/b.md": "# Bravo\n",
	}, "shell/site.md")
	if got := s.outline(); got != "API Reference\n  Alpha\n  Bravo\n" {
		t.Errorf("outline:\n%s", got)
	}
	var titles []string
	for _, n := range s.order {
		titles = append(titles, n.label)
	}
	if got := strings.Join(titles, ", "); got != "Docs, API Reference, Alpha, Bravo" {
		t.Errorf("order %s", got)
	}
	page, warnings := sitePage(t, s, res, root, "site/api-reference/index.md")
	if len(warnings) != 0 || !strings.Contains(page, "<h1") || !strings.Contains(page, "API Reference") ||
		!strings.Contains(page, `<a href="a.html">Alpha</a>`) {
		t.Errorf("warnings %q, page:\n%s", warnings, page)
	}
	for _, w := range s.Warnings {
		if strings.Contains(w.Message, "has no index.md") {
			t.Errorf("warned: %s", w.Message)
		}
	}
	d := docByBase(res.Docs, "index.md")
	if d == nil || d.Body == nil {
		t.Fatal("no written page in the emit set")
	}
}

// A directory none of whose pages is shown gets a hidden written page, and
// a name that title-cases to nothing keeps its raw name.
func TestSiteWrittenPageEdgeCases(t *testing.T) {
	s, _, _ := siteFixture(t, map[string]string{
		"shell/site.md":       minimalShell,
		"docs/index.md":       "# Home\n",
		"docs/only/secret.md": "---\nnav: hidden\n---\n# Secret\n",
		"docs/_/a.md":         "# A\n",
	}, "shell/site.md")
	if got := s.outline(); got != "_\n  A\nOnly (hidden)\n  Secret (hidden)\n" {
		t.Errorf("outline:\n%s", got)
	}
}

// A written page's output path that an asset already takes stops the
// build, naming both.
func TestSiteWrittenPageCollidesWithAnAsset(t *testing.T) {
	root := writeTree(t, map[string]string{
		"shell/site.md":    "---\nsite: true\n---\n[[content]]\n",
		"docs/index.md":    "# Home\n\n[old](x/index.html)\n",
		"docs/x/a.md":      "# A\n",
		"docs/x/index.html": "<p>old</p>",
	})
	opt := CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, OutDir: filepath.Join(root, "out"),
		Depth: -1, LinkDepth: -1, SiteShell: filepath.Join(root, "shell/site.md")}
	res := mustCrawl(t, opt)
	if _, err := BuildSite(res, opt.SiteShell, opt); err == nil || !strings.Contains(err.Error(), "output path collision") {
		t.Errorf("err %v", err)
	}
}
```

`sitePage` in `siteshell_test.go` reads the page's source with `os.ReadFile(src)`; change it to use the document's `Body` when it has one:

```go
	b := doc.Body
	if b == nil {
		var err error
		if b, err = os.ReadFile(src); err != nil {
			t.Fatal(err)
		}
	}
```

In `mountsite_test.go`, append:

```go
// A mounted directory with no page of its own, and a path segment no mount
// provides, each get a written page; a mount's title names its own.
func TestMountedSiteWritesDirectoryPages(t *testing.T) {
	s, err := mountSite(t, "[[mount source=\"../index.md\"]]\n\n"+
		"[[mount path=\"tools\" source=\"../tools\" title=\"The Tools\" nav-sort=\"links\"]]\n\n"+
		"[[mount path=\"handbook/changes\" source=\"../../CHANGELOG.md\"]]",
		map[string]string{
			"repo/docs/index.md":   "# Home\n",
			"repo/docs/tools/z.md": "# Zed\n",
			"repo/docs/tools/a.md": "# Alpha\n",
			"repo/CHANGELOG.md":    "# Changes\n",
		})
	if err != nil {
		t.Fatal(err)
	}
	want := "Handbook\n  Changes\nThe Tools\n  Alpha\n  Zed\n"
	if got := s.outline(); got != want {
		t.Errorf("outline:\n%s\nwant:\n%s", got, want)
	}
}
```

In `cmd/md2html/main_test.go`, after `TestRunSiteFromMounts`:

```go
// The command writes a page for a directory that has none, from the page's
// own Markdown, not from a file.
func TestRunSiteWritesDirectoryPages(t *testing.T) {
	root := tree(t, map[string]string{
		"shell.md":       "---\nsite: true\n---\n[[content]]\n",
		"docs/index.md":  "# Home\n",
		"docs/notes/a.md": "# Alpha\n",
	})
	var out, errb bytes.Buffer
	if code := run([]string{"--site", filepath.Join(root, "shell.md"), filepath.Join(root, "docs"), "-o", filepath.Join(root, "site")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	page, err := os.ReadFile(filepath.Join(root, "site/notes/index.html"))
	if err != nil || !strings.Contains(string(page), `href="a.html"`) || !strings.Contains(string(page), "Notes") {
		t.Errorf("no written page (err %v):\n%s", err, page)
	}
}
```

Run `gofmt -w site_test.go mountsite_test.go cmd/md2html/main_test.go`.

**Tests the change retires** (update them to the spec's new behaviour; do not delete what they check otherwise):
- `TestSiteTreeShapeOrderAndLabels` (`site_test.go`): `reference (label)` becomes `Reference`, and the order line gains `Reference` before `X`: `Home, Get started, Install, Guides, Alpha (short), Bravo, Reference, X`.
- `TestSiteTreeSectionsAndWarnings`: instead of a warning for `reference/`, assert that no warning mentions `has no index.md`.
- `TestPagesMarkersFromANestedPage` (`sitenav_test.go`): the tabs' `<li><span class="md2html-label">reference</span></li>` becomes `<li><a href="../reference/index.html">Reference</a></li>`.
- `TestMountSettingsShapeTheTree` (`mountsite_test.go`): `handbook (label)` becomes `Handbook`, and its warning check becomes a check that no warning mentions `has no index.md`.
- Any other test that fails only because a label became a written page, or the warning is gone: update it the same way, and list it in your report.

- [ ] **Step 2: Run them and see them fail.**

Run: `go test -run 'TestSiteWrites|TestSiteWrittenPage|TestMountedSiteWrites|TestSiteTree|TestPagesMarkers|TestMountSettings' . ; go test -run TestRunSiteWritesDirectoryPages ./cmd/md2html`
Expected: the root package fails to compile: `Doc.Body` is undefined; the cmd test compiles and fails: no `site/notes/index.html`.

- [ ] **Step 3: Implement.** In `crawl.go`, add to `Doc` after `Title`:

```go
	// Body, when set, is the page's Markdown, for a page md2html writes
	// itself: a directory's page in a site, whose Src names no file. A
	// caller converts Body instead of reading Src.
	Body []byte
```

In `site.go`, `BuildSite`, between `s.outRoot = …` and `s.outBySrc = make(…)`, add:

```go
	if err := s.writtenPages(res, opt); err != nil {
		return nil, err
	}
```

Add after `BuildSite`:

```go
// writtenPages gives every directory in the site a page: one with neither an
// index nor a README gets a page md2html writes, added to res.Docs at
// <dir>/index.md, so the tree, search, and previous and next treat it as any
// page. Its title is its mount's title, for a directory mount's own
// directory, else its name in title case, with the shell's title-case:
// spellings; its body lists the directory's pages.
func (s *Site) writtenPages(res *CrawlResult, opt CrawlOptions) error {
	fixed := titleCaseWords(s.shellMeta["title-case"])
	hasPage := map[string]bool{}
	dirs := map[string]bool{"": true}
	for _, d := range res.Docs {
		if d.SitePath == "" {
			continue
		}
		dirRel := sitePathDir(d.SitePath)
		stem := sitePathStem(d.SitePath)
		if isIndexStem(stem) || isReadmeStem(stem) {
			hasPage[dirRel] = true
		}
		for r := dirRel; r != ""; r = sitePathDir(r) {
			dirs[r] = true
		}
	}
	titles := map[string]string{}
	for _, m := range opt.Mounts {
		if m.IsDir && m.Title != "" {
			titles[m.Path] = m.Title
		}
	}
	assetAt := map[string]string{}
	for _, a := range res.Assets {
		assetAt[a.Out] = a.Src
	}
	shown := map[string]bool{}
	for _, d := range res.Docs {
		if d.SitePath == "" || d.Meta["nav"] == "hidden" {
			continue
		}
		for r := sitePathDir(d.SitePath); ; r = sitePathDir(r) {
			shown[r] = true
			if r == "" {
				break
			}
		}
	}
	var rels []string
	for r := range dirs {
		if !hasPage[r] {
			rels = append(rels, r)
		}
	}
	sort.Strings(rels)
	for _, r := range rels {
		title := titles[r]
		if title == "" {
			name := path.Base(r)
			if r == "" {
				name = filepath.Base(s.rootSource(opt))
			}
			if title = titleCase(name, fixed); title == "" {
				title = name // a name of only separators, such as _
			}
		}
		out := filepath.Join(s.outRoot, filepath.FromSlash(r), "index.html")
		if src, taken := assetAt[out]; taken {
			return fmt.Errorf("output path collision: %s and the page md2html writes for %s both produce %s", src, displaySitePath(r), out)
		}
		sp := path.Join(r, "index.md")
		var meta map[string]string
		if !shown[r] {
			// Nothing in it is shown, so its page is not either.
			meta = map[string]string{"nav": "hidden"}
		}
		res.Docs = append(res.Docs, Doc{
			Src:      filepath.Join(s.outRoot, filepath.FromSlash(sp)),
			Out:      out,
			SitePath: sp,
			Meta:     meta,
			Title:    title,
			Body:     []byte("# " + title + "\n\n[[pages from=\"page\" depth=\"1\"]]\n"),
		})
	}
	return nil
}

// rootSource is the directory the site's root stands for: the root mount's
// source in a mounted site, the run's base otherwise.
func (s *Site) rootSource(opt CrawlOptions) string {
	for _, m := range opt.Mounts {
		if m.Path == "" {
			return m.Source
		}
	}
	return s.base
}
```

`site.go` needs `"sort"` (already imported), `"path"`, `"path/filepath"`, `"strings"` (already imported). A root file mount is an `index.md` page, so the root never needs a written page there.

In `buildTree`'s `finish`, the doc-less directory branch can no longer warn: every directory has a page. Replace it with:

```go
		if n.isDir && n.doc == nil && n.label == "" {
			n.label = n.name
		}
```

In `cmd/md2html/main.go`, in the emit loop, replace

```go
			src, err := os.ReadFile(d.Src)
			if err != nil {
				results[i] = outcome{src: d.Src, res: md2html.WriteRefused, err: err}
				return
			}
```

with

```go
			// A page md2html writes carries its own Markdown.
			src := d.Body
			if src == nil {
				var err error
				if src, err = os.ReadFile(d.Src); err != nil {
					results[i] = outcome{src: d.Src, res: md2html.WriteRefused, err: err}
					return
				}
			}
```

and, if `err` is no longer declared for the lines that follow, declare it where it is first assigned (`out, err := md2html.Convert(…)` already does).

In `e2e/browser_test.go`, `buildSite`, replace

```go
		b, err := os.ReadFile(d.Src)
		if err != nil {
			t.Fatal(err)
		}
```

with

```go
		b := d.Body
		if b == nil {
			var err error
			if b, err = os.ReadFile(d.Src); err != nil {
				t.Fatal(err)
			}
		}
```

In `docs/guide/warnings.md`, remove the entry whose bold text is `**… has no index.md**`: no build raises it now, and `TestGuideWarningsExistInTheCode` would fail on it.

- [ ] **Step 4: Run the tests and see them pass.**

Run: `go test ./... && just e2e`
Expected: PASS. (A Chrome hiccup can fail one e2e run; rerun before suspecting the code.)

- [ ] **Step 5: Commit.**

```bash
git add crawl.go site.go cmd/md2html/main.go site_test.go sitenav_test.go siteshell_test.go mountsite_test.go cmd/md2html/main_test.go e2e/browser_test.go docs/guide/warnings.md
git commit -F - <<'EOF'
feat: md2html writes a page for a site directory that has none

A directory with neither an index.md nor a README was a bare label in
a site's tree, with a warning: a reader could not open it, and for a
directory a mount brings in, or a segment of a mount's path, the
warning asked for a file the site cannot add.

Every such directory now gets a page md2html writes at its index.html:
titled by its mount's title, or by its name in title case with the
shell's title-case: spellings, and listing its pages with
[[pages from="page"]]. It is an ordinary page of the site, in the
tree, in search and in previous and next, and the command converts it
from its own Markdown; a directory none of whose pages is shown gets a
hidden one. The warning is gone. A page whose output path an asset
already takes stops the build.
EOF
```

---

### Task 5: A link to a directory reaches its page

**Files:**
- Modify: `sitelinks.go` (`siteRules.dirs`, `action`), `crawl.go` (`buildLinkMaps`), `site.go` (`rankByLinks`)
- Test: `sitecrawl_test.go`, `site_test.go`

**Interfaces:**
- Consumes: `sitePathDir`, `readmesAtIndex` (Task 1); every directory's page is at its `index.html` (Tasks 1 and 4).
- Produces: `siteRules.dirs map[string]bool` (the site directories that hold a page), `func (r *siteRules) dirPage(target string) (string, bool)`.

- [ ] **Step 1: Write the failing tests.** In `sitecrawl_test.go`, replace `TestSiteCrawlLeavesDirectoryLinksAlone` (and its comment) with:

```go
// A link to a directory the site holds reaches the directory's page; one to
// any other directory is left as written.
func TestSiteCrawlLinksADirectoryToItsPage(t *testing.T) {
	files := map[string]string{
		"docs/index.md":     "# Home\n\n[here](./) [sub](sub/#top) [up](../) [img](img/)\n\n![flow](img/flow.png)\n",
		"docs/sub/page.md":  "# Sub\n",
		"docs/img/flow.png": "png",
	}
	res, _, err := siteCrawl(t, "repo-url: https://example.org/r/blob/main/{path}\n", files, nil)
	if err != nil {
		t.Fatal(err)
	}
	idx := docByBase(res.Docs, "index.md")
	if got := idx.AnchorLinkMap["./"]; got != "index.html" {
		t.Errorf("./ -> %q", got)
	}
	if got := idx.AnchorLinkMap["sub/#top"]; got != "sub/index.html#top" {
		t.Errorf("sub/#top -> %q", got)
	}
	for _, href := range []string{"../", "img/"} {
		if _, ok := idx.AnchorLinkMap[href]; ok || idx.AnchorLinkDrops[href] {
			t.Errorf("%s was rewritten or dropped", href)
		}
	}
	if len(res.Assets) != 1 || !strings.HasSuffix(res.Assets[0].Src, "flow.png") {
		t.Errorf("assets %v", res.Assets)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w.Message, "../") || strings.Contains(w.Message, "sub/") {
			t.Errorf("warning %q", w.Message)
		}
	}
}
```

In `site_test.go`:

```go
// Under nav-sort: links, a link to a subdirectory itself ranks it, whatever
// its page is.
func TestSiteNavSortLinksRanksADirectoryLink(t *testing.T) {
	s, _, _ := siteFixture(t, map[string]string{
		"shell/site.md":  minimalShell,
		"docs/index.md":  "---\nnav-sort: links\n---\n# Home\n\n[Zed](zed/) [Alpha](alpha.md)\n",
		"docs/alpha.md":  "# Alpha\n",
		"docs/zed/a.md":  "# A\n",
	}, "shell/site.md")
	if got := s.outline(); got != "Zed\n  A\nAlpha\n" {
		t.Errorf("outline:\n%s", got)
	}
}
```

Run `gofmt -w sitecrawl_test.go site_test.go`.

- [ ] **Step 2: Run them and see them fail.**

Run: `go test -run 'TestSiteCrawlLinksADirectoryToItsPage|TestSiteNavSortLinksRanksADirectoryLink' .`
Expected: FAIL: `tool/` and `./` are left as written, and `Zed` sorts after `Alpha`.

- [ ] **Step 3: Implement.** In `sitelinks.go`, add a field to `siteRules` after `mounts`:

```go
	dirs       map[string]bool // site directories that hold a page, by site path
```

and a method after `place`:

```go
// dirPage is the output of the page of the directory target, when the site
// holds the directory and it holds a page: every such directory's page is
// at its index.html.
func (r *siteRules) dirPage(target string) (string, bool) {
	sp, ok := r.place(target)
	if !ok {
		return "", false
	}
	if sp == "." {
		sp = ""
	}
	if !r.dirs[sp] {
		return "", false
	}
	return filepath.Join(r.outDir, filepath.FromSlash(sp), "index.html"), true
}
```

In `action`, replace

```go
	if fi.IsDir() {
		return linkAction{} // a directory is not a file to copy or link
	}
```

with:

```go
	if fi.IsDir() {
		// A link a reader follows to a directory the site holds reaches
		// the directory's page; any other directory is left as written.
		if out, ok := r.dirPage(target); ok && !l.Loaded {
			return linkAction{rewrite: relHref(fromOut, out) + fragmentOf(l.Href)}
		}
		return linkAction{}
	}
```

In `crawl.go`, `buildLinkMaps`, before the loop that plans each document, add:

```go
	if res.rules != nil {
		// Every directory that holds a page has one at its index.html: its
		// own, a README written there, or the one BuildSite writes.
		res.rules.dirs = map[string]bool{}
		for _, d := range res.Docs {
			if d.SitePath == "" {
				continue
			}
			for r := sitePathDir(d.SitePath); ; r = sitePathDir(r) {
				res.rules.dirs[r] = true
				if r == "" {
					break
				}
			}
		}
	}
```

In `site.go`, `rankByLinks`, give a child directory a rank for a link to the directory itself. Before the loop over the links, add:

```go
	byDir := map[string]*siteNode{}
	for _, c := range dir.children {
		if c.isDir {
			byDir[c.sitePath] = c
		}
	}
```

and in the loop, before `if l.Kind != LinkDoc { continue }`, add:

```go
		if l.Kind == LinkDoc || l.Kind == LinkAsset {
			if target, err := resolve(l.Abs); err == nil {
				if fi, err := os.Stat(target); err == nil && fi.IsDir() {
					var sp string
					var ok bool
					if s.rules != nil {
						sp, ok = s.rules.place(target)
					} else {
						sp, ok = relUnder(s.base, target)
					}
					if c := byDir[sp]; ok && c != nil && !c.hasRank {
						c.linkRank, c.hasRank = rank, true
						rank++
					}
					continue
				}
			}
		}
```

- [ ] **Step 4: Run the tests and see them pass.**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add sitelinks.go crawl.go site.go sitecrawl_test.go site_test.go
git commit -F - <<'EOF'
feat: a link to a site directory reaches the directory's page

A link to a directory, sub/ as a repository's hosting page shows it,
was left as written in a site, so it led to nothing once published,
and under nav-sort: links it did not rank the directory, though every
directory in a site now has a page.

A link to a directory the site holds now reaches that directory's page
at its index.html, and ranks it under nav-sort: links; a link to any
other directory is left as written, as before.
EOF
```

---

### Task 6: The docs

**Files:**
- Modify: `docs/guide/sites.md`, `CHANGELOG.md`, `docs/guide/quick-reference.md` (if the guide budget allows)

- [ ] **Step 1: The sites guide.** In `docs/guide/sites.md`, `## The tree`, replace the whole bullet that says a directory without an `index.md` is a label named after the directory, with a warning (its last sentence, that a `README.md` is an ordinary page, is no longer true either), with:

```markdown
- A directory is a node, and its page is its `index.md`, else its
  `README.md`, which then is not listed again inside it, else a page
  md2html writes: its name in title case as its title, and a list of its
  pages. The shell's `title-case:` lists words that keep a fixed
  spelling, as `title-case: API, CLI`; a mount's `title` names its own
  directory's page. Every directory's page is written at its
  `index.html`, so a link to the directory reaches it.
```

In the same section, the bullet about `nav-sort: links` says "in a directory's `index.md`": make it "in a directory's page (its `index.md` or `README.md`)", and add that a link to a subdirectory itself ranks it. In `## Traps`, the entry saying a `README.md` beside an index is an ordinary page: say it is the directory's page when there is no index.

In the `## Mounts` section's attribute table, add a row after `toc`'s: `| \`title\` | The title of the page md2html writes for the mount's directory when it has no \`index.md\` or \`README.md\`. |`

In the `## Navigation markers` table, extend the `[[pages]]` row's text with: `` `from="page"` starts at the current page, so a directory's page lists what is in it. ``

- [ ] **Step 2: The changelog.** Under `## Unreleased` → `### Added`:

```markdown
- In a site, a directory's `README.md` is its page when it has no
  `index.md`, and a directory with neither gets a page md2html writes,
  titled by its name in title case, with the shell's `title-case:`
  spellings or a mount's `title`, and listing its pages.
- `[[pages from="page"]]` lists from the current page, so an index page
  lists what is in its own directory.
```

and under `### Changed`:

```markdown
- A site directory without an `index.md` is no longer a label with a
  warning: it gets a page, from its `README.md` or written by md2html,
  at the directory's `index.html`, and a link to the directory reaches
  it.
- **breaking:** `Doc.Body` holds the Markdown of a page md2html writes
  itself, whose `Src` names no file; a caller that converts
  `CrawlResult.Docs` must convert `Body` when it is set.
```

In `docs/guide/quick-reference.md`, the Site navigation row lists `from="section"`: make it `from="section"`/`"page"` only if `go run ./cmd/md2html --guide | wc -c` stays under 8500 afterwards; otherwise leave the row as it is.

- [ ] **Step 3: Check everything.**

Run: `gofmt -l . && go vet ./... && go test ./... && just e2e && just site`
Expected: all pass; `just site` reports `0 warning(s)`; the guide summary stays under 8500 bytes (`go run ./cmd/md2html --guide | wc -c`).

- [ ] **Step 4: Commit.**

```bash
git add docs/guide/sites.md CHANGELOG.md docs/guide/quick-reference.md
git commit -F - <<'EOF'
docs: a site directory's page, from its README or written by md2html

The sites guide said a directory without an index.md is a label named
after it, with a warning, though such a directory now has a page.

The tree section says where a directory's page comes from, how a
written page is titled, and that a link to a directory reaches it; the
navigation markers name from="page"; the changelog adds them, the
change for directories without an index, and Doc.Body for a library
caller.
EOF
```
