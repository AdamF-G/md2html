# Order by Links Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `nav-sort: links` in a directory's `index.md` orders that directory's children in the site tree by the index's own links: explicit `nav-order:` first, then linked children in order of first link, then the rest by label.

**Architecture:** The site tree (`site.go`) reads the key in `applyMeta`. Before `buildTree`'s `finish` sorts a directory's children, a directory whose page says `nav-sort: links` re-reads that page, extracts its links in document order, and gives each child whose own page a link reaches a rank. `lessNode` sorts by order, then rank, then label. Nothing else in md2html changes.

**Tech Stack:** Go 1.26; the existing `splitFrontMatter`, `parseDoc`, `ExtractLinks` and `resolve`.

**Spec:** `docs/specs/2026-10-16-order-by-links-design.md`. Its mount attribute (`nav-sort="links"` on a mount, and its cascade) and the `README.md` directory page belong to the mounts and directory-pages plans: this plan reads the key from whatever page a directory node holds, so those plans get it for free when they give a directory a `README.md` page or a mount's setting.

## Global Constraints

- A branch `order-by-links` from `main`. Conventional prefixes. Commit bodies wrapped at 72 columns, starting from what was wrong before. No attribution trailer. Nothing is pushed.
- A site without `nav-sort:` is ordered exactly as before: `TestSiteTreeShapeOrderAndLabels` passes untouched.
- `nav-sort` values are `links` and `label`; `label` is the default and today's order.
- A link counts only when it reaches a child's own page (a page, or a subdirectory's `index.md`), and only from the index's body, never its front matter or the shell.
- The setting is the directory's own: a subdirectory orders its children by its own index.
- The guide summary (`md2html --guide`) stays under its 8500-byte budget.
- Run `gofmt -l .` (empty), `go vet ./...` and `go test ./...` before every commit.

## Review Focus

1. **A child linked twice, or linked after a deeper link into it.** Its rank is its first direct link; a link to `guides/deep.md` before `guides/index.md` must not rank `guides` early. Pinned in Task 1's ordering test.
2. **A child with both `nav-order:` and a link.** The explicit number wins, and the child sorts among the numbered ones. Pinned in Task 1.
3. **A link with a fragment or a `./` prefix** (`./alpha.md#setup`) reaches the same child as `alpha.md`. Pinned in Task 1.
4. **`nav-sort: links` on a page that is not an index.** It has no children to order; it must warn rather than be silently ignored. Pinned in Task 1's warning test.
5. **An index linking to itself or to a page outside its directory.** Neither is a child, so neither may take a rank or shift the others. Pinned in Task 1.

---

### Task 1: `nav-sort: links` orders a directory by its index

**Files:**
- Modify: `site.go` (`siteNode`, `buildTree`'s `finish`, `applyMeta`, `lessNode`, a new `rankByLinks`)
- Test: `site_test.go`
- Modify: `docs/guide/sites.md`, `docs/guide/quick-reference.md`, `CHANGELOG.md`

**Interfaces:**
- Produces: `siteNode.navSort string` (`"links"` or empty), `siteNode.linkRank int`, `siteNode.hasRank bool`, `func (s *Site) rankByLinks(dir *siteNode)`. The mounts plan sets `navSort` from a mount attribute; the directory-pages plan relies on `rankByLinks` reading `dir.doc`, whichever file that is.

- [ ] **Step 1: Write the failing tests** in `site_test.go`, after `TestSiteTreeWarnsOnBadNavKeys`:

```go
// An index's links order its directory: an explicit nav-order first, then
// the children the index links, by first link, then the rest by label. A
// link deeper into a subdirectory does not rank it, a link to the index
// itself or outside the directory ranks nothing, and a subdirectory keeps
// its own order.
func TestSiteNavSortLinksFollowsTheIndex(t *testing.T) {
	s, _, _ := siteFixture(t, map[string]string{
		"shell/site.md": minimalShell,
		"docs/index.md": "---\nnav-sort: links\n---\n# Home\n\n" +
			"- [Self](index.md)\n" +
			"- [Outside](../outside.md)\n" +
			"- [Page](later/page.md)\n" +
			"- [Zulu](zulu.md)\n" +
			"- [Guides](guides/index.md)\n" +
			"- [Alpha](./alpha.md#setup)\n" +
			"- [Zulu again](zulu.md)\n" +
			"- [First](first.md)\n",
		"docs/alpha.md":        "# Alpha\n",
		"docs/zulu.md":         "# Zulu\n",
		"docs/mike.md":         "# Mike\n",
		"docs/first.md":        "---\nnav-order: 1\n---\n# First\n",
		"docs/guides/index.md": "# Guides\n",
		"docs/guides/deep.md":  "# Deep\n",
		"docs/guides/beta.md":  "# Beta\n",
		"docs/later/index.md":  "# Later\n",
		"docs/later/page.md":   "# Page\n",
		"outside.md":           "# Outside\n",
	}, "shell/site.md")
	want := strings.Join([]string{
		"First",
		"Zulu",
		"Guides",
		"  Beta",
		"  Deep",
		"Alpha",
		"Later",
		"  Page",
		"Mike",
	}, "\n") + "\n"
	if got := s.outline(); got != want {
		t.Errorf("outline:\n%s\nwant:\n%s", got, want)
	}
}

func TestSiteNavSortWarns(t *testing.T) {
	s, _, _ := siteFixture(t, map[string]string{
		"shell/site.md":     minimalShell,
		"docs/index.md":     "---\nnav-sort: sideways\n---\n# Home\n\n[B](b.md) [A](a.md)\n",
		"docs/a.md":         "---\nnav-sort: links\n---\n# A\n",
		"docs/b.md":         "# B\n",
		"docs/sub/index.md": "---\nnav-sort: label\n---\n# Sub\n",
	}, "shell/site.md")
	var all string
	for _, w := range s.Warnings {
		all += w.Src + ": " + w.Message + "\n"
	}
	for _, want := range []string{
		`nav-sort "sideways" is not links or label`,
		filepath.Join("docs", "a.md") + ": front matter nav-sort applies to a directory's index.md",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("no warning contains %q; got:\n%s", want, all)
		}
	}
	for _, w := range s.Warnings {
		if strings.HasSuffix(w.Src, filepath.Join("sub", "index.md")) {
			t.Errorf("nav-sort: label warned: %s", w.Message)
		}
	}
	// An unknown value orders by label.
	if got := s.outline(); got != "A\nB\nSub\n" {
		t.Errorf("outline:\n%s", got)
	}
}
```

- [ ] **Step 2: Run them and see them fail.**

Run: `go test -run 'TestSiteNavSort' .`
Expected: FAIL. The first test's outline comes out in label order (`Alpha`, `First` …); the second finds no `nav-sort` warning.

- [ ] **Step 3: Implement.** In `site.go`:

Add three fields to `siteNode`, after `order int`:

```go
	navSort  string // "links": the children are ordered by this page's links
	linkRank int    // the place of this node's first link in its parent's page
	hasRank  bool
```

In `buildTree`, in `finish`, rank a links-sorted directory's children just before they are sorted:

```go
	var finish func(n *siteNode)
	finish = func(n *siteNode) {
		if n.isDir && n.doc == nil {
			n.label = n.name
			if n != s.root {
				s.warn(n.path, "has no index.md; the site lists it by its directory name")
			}
		}
		for _, c := range n.children {
			finish(c)
		}
		if n.navSort == "links" {
			s.rankByLinks(n)
		}
		sort.SliceStable(n.children, func(i, j int) bool { return lessNode(n.children[i], n.children[j]) })
	}
```

In `applyMeta`, after the `nav:` switch:

```go
	switch v := d.Meta["nav-sort"]; v {
	case "", "label":
	case "links":
		if n.isDir {
			n.navSort = v
		} else {
			s.warn(d.Src, "front matter nav-sort applies to a directory's index.md, which orders the directory; ignoring it")
		}
	default:
		s.warn(d.Src, fmt.Sprintf("front matter nav-sort %q is not links or label; ordering by label", v))
	}
```

Add `rankByLinks` after `applyMeta`:

```go
// rankByLinks gives each child of dir that dir's page links to a rank: the
// place of its first link in the page's body. A link counts when it reaches
// the child's own page, a page or a subdirectory's index; one deeper into a
// subdirectory, or to anything that is not a child, ranks nothing. The page
// was read once by the crawl; it is read again here, and only for a
// directory that asks, so a site without nav-sort pays nothing.
func (s *Site) rankByLinks(dir *siteNode) {
	if dir.doc == nil {
		return
	}
	raw, err := os.ReadFile(dir.doc.Src)
	if err != nil {
		return
	}
	_, body, _ := splitFrontMatter(raw)
	root, err := parseDoc(body)
	if err != nil {
		return
	}
	bySrc := map[string]*siteNode{}
	for _, c := range dir.children {
		if c.doc != nil {
			bySrc[c.doc.Src] = c
		}
	}
	rank := 0
	for _, l := range ExtractLinks(root, filepath.Dir(dir.doc.Src)) {
		if l.Kind != LinkDoc {
			continue
		}
		target, err := resolve(l.Abs)
		if err != nil {
			continue
		}
		if c := bySrc[target]; c != nil && !c.hasRank {
			c.linkRank, c.hasRank = rank, true
			rank++
		}
	}
}
```

Replace `lessNode`, and its comment:

```go
// lessNode orders siblings: those with a nav-order first, by it, then those
// their directory's page ranks by its links (nav-sort: links), by rank, then
// the rest by label, case-insensitively, with the name breaking ties so the
// order never depends on the crawl's.
func lessNode(a, b *siteNode) bool {
	if a.hasOrder != b.hasOrder {
		return a.hasOrder
	}
	if a.hasOrder && a.order != b.order {
		return a.order < b.order
	}
	if a.hasRank != b.hasRank {
		return a.hasRank
	}
	if a.hasRank && a.linkRank != b.linkRank {
		return a.linkRank < b.linkRank
	}
	if la, lb := strings.ToLower(a.label), strings.ToLower(b.label); la != lb {
		return la < lb
	}
	return a.name < b.name
}
```

`site.go` already imports `fmt`, `os`, `path/filepath`, `sort` and `strings`.

- [ ] **Step 4: Run the tests and see them pass.**

Run: `go test -run 'TestSiteNavSort|TestSiteTree' .`
Expected: PASS, `TestSiteTreeShapeOrderAndLabels` included.

- [ ] **Step 5: Document it.**

In `docs/guide/sites.md`, under `## The tree`, replace the line `- Order is \`nav-order:\` (a whole number) first, then label alphabetically.` with:

```markdown
- Order is `nav-order:` (a whole number) first, then label alphabetically.
  `nav-sort: links` in a directory's `index.md` puts its children in the
  order the index links them instead: those with `nav-order:` still come
  first, then each page or subdirectory the index links, by its first
  link, then the rest by label. A link counts when it reaches the child's
  own page; one deeper into a subdirectory does not. A subdirectory orders
  its own children by its own index.
```

In `docs/guide/quick-reference.md`, change the row `| Order or hide a site page | \`nav-order: 2\`, \`nav-title: Name\`, \`nav: hidden\` |` to:

```markdown
| Order or hide a site page | `nav-order: 2`, `nav-title: Name`, `nav: hidden`; `nav-sort: links` on an index |
```

In `CHANGELOG.md`, under `## Unreleased`, add to `### Added` (create the section and heading above the newest release if absent):

```markdown
- `nav-sort: links` in a directory's `index.md` orders the directory's
  children in the site's navigation by the index's own links, so a reading
  order written as a list is not repeated as a `nav-order:` on each page.
  An explicit `nav-order:` still comes first, and a page the index never
  links comes last, by label.
```

- [ ] **Step 6: Check the guide budget and the whole suite.**

Run: `go vet ./... && go test ./...`
Expected: PASS, `TestGuideSummaryHoldsTheQuickReferenceAndTraps` included.

- [ ] **Step 7: Commit.**

```bash
git add site.go site_test.go docs/guide/sites.md docs/guide/quick-reference.md CHANGELOG.md
git commit -F - <<'EOF'
feat: nav-sort: links orders a directory by its index's links

A directory's index.md that lists its pages in reading order had to
repeat that order as a nav-order number on each page, and the two
drifted: a reader met the pages in one order on the index and another
in the sidebar and in previous and next.

nav-sort: links in the index orders the directory's children by the
index's own links: an explicit nav-order first, then each page or
subdirectory the index links, by its first link, then the rest by
label. A link counts when it reaches the child's own page. Any other
value warns and orders by label, and the key on a page that is not an
index warns, since it has nothing to order.
EOF
```
