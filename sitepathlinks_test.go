package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInSite checks that a link is read as a site path against the pages a
// site holds, and that it answers nothing without mounts.
func TestInSite(t *testing.T) {
	docs := []Doc{
		{SitePath: "index.md", Out: "/out/index.html"},
		{SitePath: "guides/deploy.md", Out: "/out/guides/deploy.html"},
		{SitePath: "guides/my page.md", Out: "/out/guides/my page.html"},
		{SitePath: "reference/index.md", Out: "/out/reference/index.html"},
		{SitePath: "reference/engine.md", Out: "/out/reference/engine.html"},
		{SitePath: "notes/README.md", Out: "/out/notes/index.html"},
		{SitePath: "both/index.md", Out: "/out/both/index.html"},
		{SitePath: "both/README.md", Out: "/out/both/README.html"},
		{SitePath: "bare/page.md", Out: "/out/bare/page.html"},
	}
	r := &siteRules{outDir: "/out", mounts: []mountRule{{}}}
	r.indexPages(docs)

	cases := []struct {
		href, from string
		out, at    string
		ok         bool
	}{
		{"../reference/engine.md#options", "guides", "/out/reference/engine.html", "reference/engine", true},
		{"../reference/index.md", "guides", "/out/reference/index.html", "reference/index", true},
		{"../reference/", "guides", "/out/reference/index.html", "reference/", true},
		{"../reference", "guides", "/out/reference/index.html", "reference/", true},
		{"../notes/README.md", "guides", "/out/notes/index.html", "notes/README", true},
		{"../notes/index.md", "guides", "/out/notes/index.html", "notes/", true},
		{"../both/README.md", "guides", "/out/both/README.html", "both/README", true},
		{"../bare/index.md", "guides", "/out/bare/index.html", "bare/", true},
		{"../bare/README.md", "guides", "", "", false},
		{"../../x.md", "guides", "", "", false},
		{"/reference/engine.md", "guides", "", "", false},
		{"../reference/missing.md", "guides", "", "", false},
		{"my%20page.md", "guides", "/out/guides/my page.html", "guides/my page", true},
		{"reference/engine.md", "", "/out/reference/engine.html", "reference/engine", true},
		{"../", "guides", "/out/index.html", "the site's root", true},
	}
	for _, c := range cases {
		out, at, ok := r.inSite(Link{Href: c.href}, c.from)
		if out != c.out || at != c.at || ok != c.ok {
			t.Errorf("inSite(%q from %q) = %q, %q, %v; want %q, %q, %v",
				c.href, c.from, out, at, ok, c.out, c.at, c.ok)
		}
	}

	if _, _, ok := r.inSite(Link{Href: "../reference/engine.md", Loaded: true}, "guides"); ok {
		t.Error("a loaded asset was resolved as a site path")
	}

	r.mounts = nil
	for _, c := range cases {
		if _, _, ok := r.inSite(Link{Href: c.href}, c.from); ok {
			t.Errorf("inSite(%q) answered without mounts", c.href)
		}
	}
	var none *siteRules
	if _, _, ok := none.inSite(Link{Href: "a.md"}, ""); ok {
		t.Error("a nil siteRules answered")
	}
}

const sitePathShell = "[[mount source=\".\"]]\n\n[[mount path=\"reference\" source=\"../../../build/scratch/reference\"]]"

var sitePathFiles = map[string]string{
	"repo/docs/site/index.md":           "# Home\n",
	"repo/docs/site/guides/deploy.md":   "# Deploy\n\n[ref](../reference/index.md) [dir](../reference/) [bare](../reference) [opts](../reference/engine.md#options) [up](../../x.md) [abs](/reference/index.md)\n",
	"repo/docs/site/guides/tuning.md":   "# Tuning\n",
	"build/scratch/reference/index.md":  "# Reference\n",
	"build/scratch/reference/engine.md": "# Engine\n\n[tuning](../guides/tuning.md)\n",
}

func countWarnings(res *CrawlResult, sub string) int {
	n := 0
	for _, w := range res.Warnings {
		if strings.Contains(w.Message, sub) {
			n++
		}
	}
	return n
}

// TestSitePathLinksAcrossMounts checks that a link to a page of another
// mount, which is not on disk where the link says, is read as a site path.
func TestSitePathLinksAcrossMounts(t *testing.T) {
	res, _, err := mountCrawl(t, sitePathShell, sitePathFiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	deploy := docByBase(res.Docs, "deploy.md")
	for href, want := range map[string]string{
		"../reference/index.md":          "../reference/index.html",
		"../reference/":                  "../reference/index.html",
		"../reference":                   "../reference/index.html",
		"../reference/engine.md#options": "../reference/engine.html#options",
	} {
		if got := deploy.AnchorLinkMap[href]; got != want {
			t.Errorf("deploy %s -> %q, want %q", href, got, want)
		}
	}
	engine := docByBase(res.Docs, "engine.md")
	if got := engine.AnchorLinkMap["../guides/tuning.md"]; got != "../guides/tuning.html" {
		t.Errorf("engine ../guides/tuning.md -> %q", got)
	}
	if n := countWarnings(res, "does not exist"); n != 2 {
		t.Errorf("%d does-not-exist warnings, want 2: %v", n, res.Warnings)
	}
	for _, href := range []string{"../../x.md", "/reference/index.md"} {
		if countWarnings(res, "does not exist: "+href) != 1 {
			t.Errorf("no warning for %s: %v", href, res.Warnings)
		}
	}
}

// TestSitePathLinksNeedTheMount checks that without the mount the same
// links reach nothing, and say so.
func TestSitePathLinksNeedTheMount(t *testing.T) {
	res, _, err := mountCrawl(t, "[[mount source=\".\"]]", sitePathFiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	if countWarnings(res, "link target does not exist: ../reference/index.md") != 1 ||
		countWarnings(res, "referenced asset does not exist: ../reference/") != 1 {
		t.Errorf("warnings: %v", res.Warnings)
	}
}

// TestSitePathLinkFromTheShell checks that a shell link is read from the
// site's root, on every page.
func TestSitePathLinkFromTheShell(t *testing.T) {
	body := sitePathShell + "\n\n[Ref](reference/index.md)"
	for rel, want := range map[string]string{
		"repo/docs/site/guides/deploy.md": `href="../reference/index.html"`,
		"repo/docs/site/index.md":         `href="reference/index.html"`,
	} {
		out, warnings, s := mountPage(t, body, sitePathFiles, rel)
		if !strings.Contains(out, want) {
			t.Errorf("%s lacks %s", rel, want)
		}
		for _, w := range append(warnings, warningTexts(s)...) {
			if strings.Contains(w, "which the build does not write") {
				t.Errorf("%s: %s", rel, w)
			}
		}
	}
}

func warningTexts(s *Site) []string {
	var out []string
	for _, w := range s.Warnings {
		out = append(out, w.Message)
	}
	return out
}

// TestSitePathLinkToAnUnreadablePage checks that a page the crawl drops is
// not a page a link can reach.
func TestSitePathLinkToAnUnreadablePage(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads any file")
	}
	res, _, err := mountCrawl(t, sitePathShell, sitePathFiles, func(opt *CrawlOptions) {
		p := filepath.Join(filepath.Dir(opt.SiteShell), "../../../build/scratch/reference/engine.md")
		if err := os.Chmod(p, 0); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	deploy := docByBase(res.Docs, "deploy.md")
	const href = "../reference/engine.md#options"
	if v, ok := deploy.AnchorLinkMap[href]; ok {
		t.Errorf("mapped to %q", v)
	}
	if deploy.AnchorLinkDrops[href] {
		t.Error("dropped")
	}
}

// TestReadmeTakesTheIndexOfAnUnreadableIndex checks that a README stands
// as its directory's page when the index beside it is dropped as unreadable.
func TestReadmeTakesTheIndexOfAnUnreadableIndex(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads any file")
	}
	files := map[string]string{
		"repo/home.md":         "# Home\n",
		"repo/notes/README.md": "# Notes\n",
		"repo/notes/index.md":  "# Index\n",
	}
	res, root, err := mountCrawl(t, "[[mount source=\"../../home.md\"]]\n\n[[mount path=\"notes\" source=\"../../notes\"]]", files, func(opt *CrawlOptions) {
		if err := os.Chmod(filepath.Join(filepath.Dir(opt.SiteShell), "../../notes/index.md"), 0); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	readme := docByBase(res.Docs, "README.md")
	if want := filepath.Join(root, "out/notes/index.html"); readme.Out != want {
		t.Errorf("README written to %s, want %s", readme.Out, want)
	}
}

// TestSitePathLinksWithoutMounts checks that a site built from entry points
// reads no link as a site path.
func TestSitePathLinksWithoutMounts(t *testing.T) {
	res, _, err := siteCrawl(t, "", map[string]string{
		"docs/index.md":    "# Home\n\n[s](sub/index.md)\n",
		"docs/sub/page.md": "# Page\n",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if countWarnings(res, "link target does not exist: sub/index.md") != 1 {
		t.Errorf("warnings: %v", res.Warnings)
	}
}

// TestSitePathLinksPassExcludedTargets checks that a link to an excluded
// file or directory reaches the site's page at that path when there is
// one, and goes to the repository's host when there is none.
func TestSitePathLinksPassExcludedTargets(t *testing.T) {
	files := map[string]string{}
	for k, v := range sitePathFiles {
		files[k] = v
	}
	files["repo/docs/site/shell.md"] = "---\nsite: true\nrepo-url: https://example.com/blob/main/{path}\n---\n" +
		"[[mount source=\".\" exclude=\"reference\"]]\n\n[[mount path=\"reference\" source=\"../../../build/scratch/reference\"]]\n\n[[content]]\n"
	files["repo/docs/site/guides/tuning.md"] = "# Tuning\n\n[e](../reference/engine.md) [r](../reference/) [o](../reference/old.md)\n"
	files["repo/docs/site/reference/engine.md"] = "# Template\n"
	files["repo/docs/site/reference/old.md"] = "# Old\n"
	res, _, err := mountCrawl(t, sitePathShell, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	tuning := docByBase(res.Docs, "tuning.md")
	for href, want := range map[string]string{
		"../reference/engine.md": "../reference/engine.html",
		"../reference/":          "../reference/index.html",
		"../reference/old.md":    "https://example.com/blob/main/repo/docs/site/reference/old.md",
	} {
		if got := tuning.AnchorLinkMap[href]; got != want {
			t.Errorf("tuning %s -> %q, want %q", href, got, want)
		}
	}
	if n := countWarnings(res, "shadows"); n != 0 {
		t.Errorf("%d shadow warnings: %v", n, res.Warnings)
	}
}

// TestMountedSiteDirectoryLinksLeaveTheSite checks that a link to a
// directory the site does not hold follows unmounted:, while one to a
// directory it holds without a page stays as written.
func TestMountedSiteDirectoryLinksLeaveTheSite(t *testing.T) {
	build := func(front string) (*CrawlResult, error) {
		files := map[string]string{}
		for k, v := range mountFiles {
			files[k] = v
		}
		files["repo/README.md"] = "# Readme\n\n[root](./) [notes](./notes/)\n"
		files["repo/notes/n.md"] = "# N\n"
		files["repo/docs/guide/sites.md"] = "# Sites\n\n[img](img/)\n"
		files["repo/docs/site/shell.md"] = "---\nsite: true\n" + front + "---\n" + mountShell + "\n\n[[content]]\n"
		res, _, err := mountCrawl(t, mountShell, files, nil)
		return res, err
	}

	res, err := build("repo-url: https://example.com/blob/main/{path}\n")
	if err != nil {
		t.Fatal(err)
	}
	readme := docByBase(res.Docs, "README.md")
	for href, want := range map[string]string{
		"./":       "https://example.com/blob/main/",
		"./notes/": "https://example.com/blob/main/notes",
	} {
		if got := readme.AnchorLinkMap[href]; got != want {
			t.Errorf("%s -> %q, want %q", href, got, want)
		}
	}
	sites := docByBase(res.Docs, "sites.md")
	if v, ok := sites.AnchorLinkMap["img/"]; ok || sites.AnchorLinkDrops["img/"] {
		t.Errorf("img/ changed: %q %v", v, sites.AnchorLinkDrops["img/"])
	}

	res, err = build("")
	if err != nil {
		t.Fatal(err)
	}
	readme = docByBase(res.Docs, "README.md")
	for _, href := range []string{"./", "./notes/"} {
		if !readme.AnchorLinkDrops[href] {
			t.Errorf("%s not dropped: %v", href, readme.AnchorLinkDrops)
		}
		if countWarnings(res, "unlinked "+href+", which is outside the site") != 1 {
			t.Errorf("no warning for %s: %v", href, res.Warnings)
		}
	}

	if _, err = build("unmounted: error\n"); err == nil || !strings.Contains(err.Error(), "./notes/") {
		t.Errorf("unmounted: error gave %v", err)
	}
}

// TestExcludedDirectoryWithNoPageLeavesTheSite checks that a link to an
// excluded directory the site has no page for follows unmounted:, as a
// link to an excluded file does.
func TestExcludedDirectoryWithNoPageLeavesTheSite(t *testing.T) {
	build := func(front string) (*CrawlResult, error) {
		files := map[string]string{}
		for k, v := range mountFiles {
			files[k] = v
		}
		files["repo/docs/guide/sites.md"] = "# Sites\n\n[d](drafts/)\n"
		files["repo/docs/site/shell.md"] = "---\nsite: true\n" + front + "---\n" + mountShell + "\n\n[[content]]\n"
		res, _, err := mountCrawl(t, mountShell, files, nil)
		return res, err
	}
	res, err := build("repo-url: https://example.com/blob/main/{path}\n")
	if err != nil {
		t.Fatal(err)
	}
	sites := docByBase(res.Docs, "sites.md")
	if got, want := sites.AnchorLinkMap["drafts/"], "https://example.com/blob/main/docs/guide/drafts"; got != want {
		t.Errorf("drafts/ -> %q, want %q", got, want)
	}
	res, err = build("")
	if err != nil {
		t.Fatal(err)
	}
	sites = docByBase(res.Docs, "sites.md")
	if !sites.AnchorLinkDrops["drafts/"] {
		t.Errorf("drafts/ not dropped: %v", sites.AnchorLinkDrops)
	}
	if countWarnings(res, "unlinked drafts/, which is outside the site") != 1 {
		t.Errorf("no warning: %v", res.Warnings)
	}
}
