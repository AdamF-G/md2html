package md2html

import (
	"path/filepath"
	"strings"
	"testing"
)

// siteFixture writes files, crawls root/docs into root/site, and builds the
// site from the shell at shellRel, relative to root.
func siteFixture(t *testing.T, files map[string]string, shellRel string) (*Site, *CrawlResult, string) {
	t.Helper()
	root := writeTree(t, files)
	opt := CrawlOptions{Entries: []string{filepath.Join(root, "docs")},
		OutDir: filepath.Join(root, "site"), Depth: -1, LinkDepth: -1}
	res := mustCrawl(t, opt)
	s, err := BuildSite(res, filepath.Join(root, shellRel), opt)
	if err != nil {
		t.Fatalf("BuildSite: %v", err)
	}
	return s, res, root
}

const minimalShell = "---\nsite: true\n---\n[[content]]\n"

var treeFiles = map[string]string{
	"shell/site.md":         minimalShell,
	"docs/index.md":         "# Home\n",
	"docs/start/index.md":   "---\nnav-order: 1\n---\n# Get started\n",
	"docs/start/install.md": "# Install\n",
	"docs/guides/index.md":  "---\nnav-order: 2\n---\n# Guides\n",
	"docs/guides/b.md":      "# Bravo\n",
	"docs/guides/a.md":      "---\nnav-title: Alpha (short)\n---\n# Alpha\n",
	"docs/guides/secret.md": "---\nnav: hidden\n---\n# Secret\n",
	"docs/reference/x.md":   "# X\n",
}

func TestSiteTreeShapeOrderAndLabels(t *testing.T) {
	s, _, _ := siteFixture(t, treeFiles, "shell/site.md")
	want := strings.Join([]string{
		"Get started",
		"  Install",
		"Guides",
		"  Alpha (short)",
		"  Bravo",
		"  Secret (hidden)",
		"Reference",
		"  X",
	}, "\n") + "\n"
	if got := s.outline(); got != want {
		t.Errorf("outline:\n%s\nwant:\n%s", got, want)
	}
	var titles []string
	for _, n := range s.order {
		titles = append(titles, n.label)
	}
	if got := strings.Join(titles, ", "); got != "Home, Get started, Install, Guides, Alpha (short), Bravo, Reference, X" {
		t.Errorf("tree order = %s", got)
	}
}

func TestSiteTreeSectionsAndWarnings(t *testing.T) {
	s, _, root := siteFixture(t, treeFiles, "shell/site.md")
	if got := s.section(filepath.Join(root, "docs/guides/a.md")); got != "guides" {
		t.Errorf("section = %q, want guides", got)
	}
	if got := s.section(filepath.Join(root, "docs/index.md")); got != "" {
		t.Errorf("the root page has section %q, want none", got)
	}
	var msgs []string
	for _, w := range s.Warnings {
		msgs = append(msgs, w.Src+": "+w.Message)
	}
	all := strings.Join(msgs, "\n")
	if strings.Contains(all, "has no index.md") {
		t.Errorf("a directory without a page warned:\n%s", all)
	}
}

func TestSiteTreeWarnsOnBadNavKeys(t *testing.T) {
	files := map[string]string{
		"shell/site.md": minimalShell,
		"docs/a.md":     "---\nnav-order: first\nnav: secret\n---\n# A\n",
	}
	s, _, _ := siteFixture(t, files, "shell/site.md")
	var all string
	for _, w := range s.Warnings {
		all += w.Message + "\n"
	}
	for _, want := range []string{`nav-order "first" is not a whole number`, `nav "secret" is not hidden`} {
		if !strings.Contains(all, want) {
			t.Errorf("no warning contains %q; got:\n%s", want, all)
		}
	}
}

// A directory with neither index.md nor README.md gets a page md2html writes:
// titled from its name, listing its pages, in the tree, in the order of
// previous and next, and found by search. The root too.
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
	if !strings.Contains(string(s.SearchIndex()), "api-reference/index.html") {
		t.Errorf("search does not find the written page:\n%s", s.SearchIndex())
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
		"shell/site.md":     "---\nsite: true\n---\n[[content]]\n",
		"docs/index.md":     "# Home\n\n[old](x/index.html)\n",
		"docs/x/a.md":       "# A\n",
		"docs/x/index.html": "<p>old</p>",
	})
	opt := CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, OutDir: filepath.Join(root, "out"),
		Depth: -1, LinkDepth: -1, SiteShell: filepath.Join(root, "shell/site.md")}
	res := mustCrawl(t, opt)
	if _, err := BuildSite(res, opt.SiteShell, opt); err == nil || !strings.Contains(err.Error(), "output path collision") {
		t.Errorf("err %v", err)
	}
}

// A directory's README is its page when it has no index.md, and an ordinary
// page beside one, whichever the crawl meets first. A README that is the
// page carries nav-sort: as an index does.
func TestSiteREADMEIsADirectorysPage(t *testing.T) {
	s, _, _ := siteFixture(t, map[string]string{
		"shell/site.md":       minimalShell,
		"docs/index.md":       "# Home\n",
		"docs/tool/README.md": "---\nnav-sort: links\n---\n# Tool\n\n[Zed](zed.md) [Alpha](alpha.md)\n",
		"docs/tool/alpha.md":  "# Alpha\n",
		"docs/tool/zed.md":    "# Zed\n",
		"docs/both/README.md": "# Both Readme\n",
		"docs/both/index.md":  "# Both\n",
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

func TestBuildSiteDropsAShellInsideTheEntries(t *testing.T) {
	s, res, root := siteFixture(t, map[string]string{
		"docs/site.md":  minimalShell,
		"docs/index.md": "# Home\n",
	}, "docs/site.md")
	for _, d := range res.Docs {
		if d.Src == filepath.Join(root, "docs/site.md") {
			t.Fatal("the shell is still in the emit set")
		}
	}
	if strings.Contains(s.outline(), "site") {
		t.Errorf("the shell is in the tree:\n%s", s.outline())
	}
}

func TestBuildSiteRefusesABadShell(t *testing.T) {
	for name, shell := range map[string]string{
		"no site key":  "---\ntitle: x\n---\n[[content]]\n",
		"no content":   "---\nsite: true\n---\nnothing\n",
		"two contents": "---\nsite: true\n---\n[[content]]\n\n[[content]]\n",
	} {
		root := writeTree(t, map[string]string{"shell.md": shell, "docs/a.md": "# A\n"})
		opt := CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, Depth: -1, LinkDepth: -1}
		if _, err := BuildSite(mustCrawl(t, opt), filepath.Join(root, "shell.md"), opt); err == nil {
			t.Errorf("%s: BuildSite accepted it", name)
		}
	}
}

func TestSiteTreeWarnsOnAPageOutsideTheRoot(t *testing.T) {
	root := writeTree(t, map[string]string{
		"shell.md":   minimalShell,
		"docs/a.md":  "# A\n\n[out](../outside.md)\n",
		"outside.md": "# Outside\n",
	})
	opt := CrawlOptions{Entries: []string{filepath.Join(root, "docs")},
		OutDir: filepath.Join(root, "site"), Depth: -1, LinkDepth: -1}
	s, err := BuildSite(mustCrawl(t, opt), filepath.Join(root, "shell.md"), opt)
	if err != nil {
		t.Fatal(err)
	}
	var all string
	for _, w := range s.Warnings {
		all += w.Message + "\n"
	}
	if !strings.Contains(all, "outside the site's root") {
		t.Errorf("no warning about outside.md; got:\n%s", all)
	}
}

// A hidden root page hides only itself: the rest of the site keeps its
// order, and so its previous and next links.
func TestSiteHiddenRootHidesOnlyItsOwnPage(t *testing.T) {
	s, _, _ := siteFixture(t, map[string]string{
		"shell/site.md": minimalShell,
		"docs/index.md": "---\nnav: hidden\n---\n# Home\n",
		"docs/a.md":     "# Alpha\n",
		"docs/b.md":     "# Bravo\n",
	}, "shell/site.md")
	var titles []string
	for _, n := range s.order {
		titles = append(titles, n.label)
	}
	if got := strings.Join(titles, ", "); got != "Alpha, Bravo" {
		t.Errorf("tree order = %q, want Alpha, Bravo", got)
	}
}

// A directory's name is text in its written page's heading, not Markdown or
// HTML: it opens no tag and makes no chip.
func TestSiteWrittenPageTitleIsPlainText(t *testing.T) {
	s, res, root := siteFixture(t, map[string]string{
		"shell/site.md":     minimalShell,
		"docs/index.md":     "# Home\n",
		"docs/a <b> c/x.md": "# X\n",
		"docs/[draft]/y.md": "# Y\n",
	}, "shell/site.md")
	page, _ := sitePage(t, s, res, root, "site/a <b> c/index.md")
	if !strings.Contains(page, "A &lt;b&gt; C<a class=") || strings.Contains(page, "<b>") {
		t.Errorf("tag title:\n%s", page)
	}
	page, _ = sitePage(t, s, res, root, "site/[draft]/index.md")
	if !strings.Contains(page, ">[<!---->draft]<a class=") || strings.Contains(page, `class="chip`) {
		t.Errorf("bracket title:\n%s", page)
	}
}

// Under nav-sort: links, a link to a subdirectory itself ranks it, whatever
// its page is.
func TestSiteNavSortLinksRanksADirectoryLink(t *testing.T) {
	s, _, _ := siteFixture(t, map[string]string{
		"shell/site.md": minimalShell,
		"docs/index.md": "---\nnav-sort: links\n---\n# Home\n\n[Zed](zed/) [Alpha](alpha.md)\n",
		"docs/alpha.md": "# Alpha\n",
		"docs/zed/a.md": "# A\n",
	}, "shell/site.md")
	if got := s.outline(); got != "Zed\n  A\nAlpha\n" {
		t.Errorf("outline:\n%s", got)
	}
}

// A directory whose index.md the build did not reach keeps its label and
// warns; md2html writes it no page, with -o or in place.
func TestSiteUnreachedIndexGetsNoWrittenPage(t *testing.T) {
	for _, outDir := range []string{"site", ""} {
		root := writeTree(t, map[string]string{
			"shell/site.md":     minimalShell,
			"docs/index.md":     "# Home\n\n[A](sub/a.md)\n",
			"docs/sub/a.md":     "# Alpha\n",
			"docs/sub/index.md": "# Real sub index\n",
		})
		opt := CrawlOptions{Entries: []string{filepath.Join(root, "docs/index.md")},
			Depth: -1, LinkDepth: -1}
		if outDir != "" {
			opt.OutDir = filepath.Join(root, outDir)
		}
		res := mustCrawl(t, opt)
		s, err := BuildSite(res, filepath.Join(root, "shell/site.md"), opt)
		if err != nil {
			t.Fatalf("%q: %v", outDir, err)
		}
		for _, d := range res.Docs {
			if d.Body != nil && d.SitePath == "sub/index.md" {
				t.Errorf("%q: a page is written for sub: %s", outDir, d.Out)
			}
		}
		if strings.Contains(s.outline(), "Real sub index") || strings.Contains(s.outline(), "Sub (hidden)") {
			t.Errorf("%q: outline:\n%s", outDir, s.outline())
		}
		var warned bool
		for _, w := range s.Warnings {
			if filepath.ToSlash(w.Src) == filepath.ToSlash(filepath.Join(root, "docs/sub/index.md")) &&
				w.Message == "not in the build: no page links it, so its directory has no page" {
				warned = true
			}
		}
		if !warned {
			t.Errorf("%q: no warning naming sub/index.md: %v", outDir, s.Warnings)
		}
	}
}
