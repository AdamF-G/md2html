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
		"reference (label)",
		"  X",
	}, "\n") + "\n"
	if got := s.outline(); got != want {
		t.Errorf("outline:\n%s\nwant:\n%s", got, want)
	}
	var titles []string
	for _, n := range s.order {
		titles = append(titles, n.label)
	}
	if got := strings.Join(titles, ", "); got != "Home, Get started, Install, Guides, Alpha (short), Bravo, X" {
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
	if !strings.Contains(all, filepath.Join("docs", "reference")+": has no index.md") {
		t.Errorf("want a warning for reference/ having no index.md; got:\n%s", all)
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
