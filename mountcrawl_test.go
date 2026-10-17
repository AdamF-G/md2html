package md2html

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// mountCrawl writes files under a temp root, reads the mounts of the shell
// at repo/docs/site/shell.md, which holds body, and crawls them into
// root/out.
func mountCrawl(t *testing.T, body string, files map[string]string, mod func(*CrawlOptions)) (*CrawlResult, string, error) {
	t.Helper()
	all := map[string]string{"repo/docs/site/shell.md": "---\nsite: true\n---\n" + body + "\n\n[[content]]\n"}
	for k, v := range files {
		all[k] = v
	}
	root := writeTree(t, all)
	shell := filepath.Join(root, "repo/docs/site/shell.md")
	ms, err := ReadMounts(shell, nil)
	if err != nil {
		t.Fatalf("ReadMounts: %v", err)
	}
	opt := CrawlOptions{Mounts: ms, SiteShell: shell, OutDir: filepath.Join(root, "out")}
	if mod != nil {
		mod(&opt)
	}
	res, err := Crawl(opt)
	return res, root, err
}

var mountFiles = map[string]string{
	"repo/README.md":                "# Readme\n\n[sites](./docs/guide/sites.md#mounts) [licence](./LICENSE)\n",
	"repo/LICENSE":                  "MIT",
	"repo/CHANGELOG.md":             "# Changes\n",
	"repo/docs/index.md":            "# Home\n",
	"repo/docs/guide/index.md":      "# Guide\n",
	"repo/docs/guide/sites.md":      "# Sites\n\n![shot](img/shot.png) [changes](../../CHANGELOG.md) [design](../specs/a.md)\n",
	"repo/docs/guide/img/shot.png":  "png",
	"repo/docs/guide/drafts/wip.md": "# WIP\n",
	"repo/docs/specs/index.md":      "# Specs\n",
	"repo/docs/specs/a.md":          "# A\n",
}

const mountShell = "[[mount source=\"../index.md\"]]\n\n" +
	"[[mount path=\"overview\" source=\"../../README.md\"]]\n\n" +
	"[[mount path=\"guide\" source=\"../guide\" exclude=\"drafts\"]]\n\n" +
	"[[mount path=\"design-notes\" source=\"../specs\"]]"

func sitePaths(res *CrawlResult) string {
	var ps []string
	for _, d := range res.Docs {
		ps = append(ps, d.SitePath)
	}
	sort.Strings(ps)
	return strings.Join(ps, ",")
}

func TestMountCrawlPlacesPagesAtTheirSitePaths(t *testing.T) {
	res, root, err := mountCrawl(t, mountShell, mountFiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := sitePaths(res); got != "design-notes/a.md,design-notes/index.md,guide/index.md,guide/sites.md,index.md,overview.md" {
		t.Errorf("site paths %s", got)
	}
	if res.Base != filepath.Join(root, "repo") {
		t.Errorf("base %s, want the source root", res.Base)
	}
	for _, d := range res.Docs {
		want := filepath.Join(root, "out", filepath.FromSlash(strings.TrimSuffix(d.SitePath, ".md")+".html"))
		if d.Out != want {
			t.Errorf("%s written to %s, want %s", d.SitePath, d.Out, want)
		}
	}
	readme := docByBase(res.Docs, "README.md")
	if got := readme.AnchorLinkMap["./docs/guide/sites.md#mounts"]; got != "guide/sites.html#mounts" {
		t.Errorf("a link between mounts -> %q", got)
	}
	// The README's mount is the file alone: its LICENSE is outside the site.
	if !readme.AnchorLinkDrops["./LICENSE"] {
		t.Errorf("a file beside a file mount was not treated as outside the site: map %v drops %v", readme.LinkMap, readme.AnchorLinkDrops)
	}
	sites := docByBase(res.Docs, "sites.md")
	if sites.LinkMap["img/shot.png"] != "img/shot.png" || sites.AnchorLinkMap["../specs/a.md"] != "../design-notes/a.html" {
		t.Errorf("sites map %v anchors %v", sites.LinkMap, sites.AnchorLinkMap)
	}
	if !sites.AnchorLinkDrops["../../CHANGELOG.md"] {
		t.Error("a page no mount holds was not treated as outside the site")
	}
	if len(res.Assets) != 1 || res.Assets[0].Out != filepath.Join(root, "out/guide/img/shot.png") {
		t.Errorf("assets %v", res.Assets)
	}
}

// A mount inside another's source takes its files out of the enclosing
// one; the shell, inside the root mount's source, is not a page, so no link
// can reach a page that is never written.
func TestMountCrawlHoistsADeeperMount(t *testing.T) {
	res, _, err := mountCrawl(t, "[[mount source=\"..\" exclude=\"guide\"]]\n\n[[mount path=\"design-notes\" source=\"../specs\"]]", mountFiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := sitePaths(res); got != "design-notes/a.md,design-notes/index.md,index.md" {
		t.Errorf("site paths %s", got)
	}
}

func TestMountCrawlSymlinks(t *testing.T) {
	_, root, err := mountCrawl(t, mountShell, mountFiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A symlink in the guide to a spec, and one to a file no mount holds.
	if err := os.Symlink(filepath.Join(root, "repo/docs/specs/a.md"), filepath.Join(root, "repo/docs/guide/alias.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "repo/CHANGELOG.md"), filepath.Join(root, "repo/docs/guide/changes.md")); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(root, "repo/docs/site/shell.md")
	// The second shell's root mount also holds the guide's directory, so
	// two walks reach the symlink; only the guide's mount, which holds its
	// path, considers it.
	for _, body := range []string{mountShell, "[[mount source=\"..\" exclude=\"site\"]]\n\n[[mount path=\"guide\" source=\"../guide\"]]"} {
		if err := os.WriteFile(shell, []byte("---\nsite: true\n---\n"+body+"\n\n[[content]]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		ms, err := ReadMounts(shell, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Crawl(CrawlOptions{Mounts: ms, SiteShell: shell, OutDir: filepath.Join(root, "out")})
		if err != nil {
			t.Fatal(err)
		}
		if body == mountShell {
			if got := sitePaths(res); got != "design-notes/a.md,design-notes/index.md,guide/index.md,guide/sites.md,index.md,overview.md" {
				t.Errorf("site paths %s", got)
			}
		}
		link := filepath.Join(root, "repo/docs/guide/changes.md")
		want := "not building " + link + ": it leads to " + filepath.Join(root, "repo/CHANGELOG.md") + ", which no mount holds"
		var said []Warning
		for _, w := range res.Warnings {
			if strings.Contains(w.Message, "which no mount holds") {
				said = append(said, w)
			}
		}
		if len(said) != 1 || said[0].Message != want || said[0].Src != link {
			t.Errorf("%q: warnings for the symlink to a file no mount holds: %v; want one, %q", body, said, want)
		}
	}
}

func TestMountCrawlRefuses(t *testing.T) {
	for _, c := range []struct {
		body string
		mod  func(*CrawlOptions)
		want string
	}{
		{mountShell + "\n\n[[mount path=\"guide/sites\" source=\"../../CHANGELOG.md\"]]", nil, "output path collision"},
		{mountShell, func(o *CrawlOptions) { o.Exclude = []string{"docs/specs"} }, "is excluded"},
		{mountShell, func(o *CrawlOptions) { o.Entries = []string{"x"} }, "give no entry points"},
		{mountShell, func(o *CrawlOptions) { o.OutDir = "" }, "need a site shell and an output directory"},
	} {
		if _, _, err := mountCrawl(t, c.body, mountFiles, c.mod); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: err %v, want one containing %q", c.body, err, c.want)
		}
	}
}

// --exclude applies to every mount, relative to the source root.
func TestMountCrawlGlobalExclude(t *testing.T) {
	res, _, err := mountCrawl(t, mountShell, mountFiles, func(o *CrawlOptions) { o.Exclude = []string{"docs/specs/a.md"} })
	if err != nil {
		t.Fatal(err)
	}
	if got := sitePaths(res); strings.Contains(got, "design-notes/a.md") {
		t.Errorf("an excluded page was built: %s", got)
	}
}

// In a mounted site an excluded file is outside the site: a link to an
// excluded page leaves it, and an excluded image a page loads is copied to
// _assets/, as any file outside the site is.
func TestMountCrawlExcludedTargetsAreOutsideTheSite(t *testing.T) {
	files := map[string]string{}
	for k, v := range mountFiles {
		files[k] = v
	}
	files["repo/docs/site/shell.md"] = "---\nsite: true\nrepo-url: https://example.com/blob/main/{path}\n---\n" + mountShell + "\n\n[[content]]\n"
	files["repo/docs/guide/sites.md"] = "# Sites\n\n[design](../specs/a.md#top) ![wip](drafts/wip.png)\n"
	files["repo/docs/guide/drafts/wip.png"] = "png"
	res, root, err := mountCrawl(t, mountShell, files, func(o *CrawlOptions) { o.Exclude = []string{"docs/specs/a.md"} })
	if err != nil {
		t.Fatal(err)
	}
	sites := docByBase(res.Docs, "sites.md")
	if got := sites.AnchorLinkMap["../specs/a.md#top"]; got != "https://example.com/blob/main/docs/specs/a.md#top" {
		t.Errorf("a link to an excluded page -> %q; anchors %v drops %v", got, sites.AnchorLinkMap, sites.AnchorLinkDrops)
	}
	if got := sites.LinkMap["drafts/wip.png"]; got != "../_assets/docs/guide/drafts/wip.png" {
		t.Errorf("an excluded image -> %q; map %v", got, sites.LinkMap)
	}
	var copied bool
	for _, a := range res.Assets {
		copied = copied || a.Out == filepath.Join(root, "out/_assets/docs/guide/drafts/wip.png")
	}
	if !copied {
		t.Errorf("the excluded image was not copied: %v", res.Assets)
	}
}
