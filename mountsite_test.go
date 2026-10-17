package md2html

import (
	"path/filepath"
	"strings"
	"testing"
)

// mountSite crawls files as mountCrawl does and lays the site out.
func mountSite(t *testing.T, body string, files map[string]string) (*Site, error) {
	t.Helper()
	res, root, err := mountCrawl(t, body, files, nil)
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	opt := CrawlOptions{Mounts: nil, SiteShell: filepath.Join(root, "repo/docs/site/shell.md"), OutDir: filepath.Join(root, "out")}
	ms, _ := ReadMounts(opt.SiteShell, nil)
	opt.Mounts = ms
	return BuildSite(res, opt.SiteShell, opt)
}

func TestMountSettingsShapeTheTree(t *testing.T) {
	files := map[string]string{
		"repo/README.md":                    "---\nnav-order: 9\n---\n# Readme\n",
		"repo/docs/index.md":                "# Home\n",
		"repo/docs/guide/index.md":          "# Guide\n\n[Zed](zed.md) [Sub](sub/index.md) [Alpha](alpha.md)\n",
		"repo/docs/guide/alpha.md":          "# Alpha\n",
		"repo/docs/guide/zed.md":            "# Zed\n",
		"repo/docs/guide/sub/index.md":      "# Sub\n\n[Y](y.md) [X](x.md)\n",
		"repo/docs/guide/sub/x.md":          "# X\n",
		"repo/docs/guide/sub/y.md":          "# Y\n",
		"repo/docs/guide/own/index.md":      "---\nnav-sort: label\n---\n# Own\n\n[Q](q.md) [P](p.md)\n",
		"repo/docs/guide/own/p.md":          "# P\n",
		"repo/docs/guide/own/q.md":          "# Q\n",
		"repo/docs/guide/own/deep/index.md": "# Deep\n\n[Zz](zz.md) [Aa](aa.md)\n",
		"repo/docs/guide/own/deep/aa.md":    "# Aa\n",
		"repo/docs/guide/own/deep/zz.md":    "# Zz\n",
		"repo/docs/specs/index.md":          "# Specs\n",
		"repo/CHANGELOG.md":                 "# Changes\n",
	}
	s, err := mountSite(t, "[[mount source=\"../index.md\"]]\n\n"+
		"[[mount path=\"overview\" source=\"../../README.md\" nav-title=\"Overview\" nav-order=\"1\"]]\n\n"+
		"[[mount path=\"guide\" source=\"../guide\" nav-order=\"2\" nav-sort=\"links\"]]\n\n"+
		"[[mount path=\"design-notes\" source=\"../specs\" nav-title=\"Design Notes\" nav-order=\"3\"]]\n\n"+
		"[[mount path=\"handbook/changes\" source=\"../../CHANGELOG.md\"]]", files)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"Overview",
		"Guide",
		"  Zed",
		"  Sub",
		"    Y",
		"    X",
		"  Alpha",
		"  Own",
		"    Deep",
		"      Aa",
		"      Zz",
		"    P",
		"    Q",
		"Design Notes",
		"handbook (label)",
		"  Changes",
	}, "\n") + "\n"
	if got := s.outline(); got != want {
		t.Errorf("outline:\n%s\nwant:\n%s", got, want)
	}
	var all string
	for _, w := range s.Warnings {
		all += w.Src + ": " + w.Message + "\n"
	}
	if !strings.Contains(all, "site directory handbook has no index.md") {
		t.Errorf("no warning for the unprovided segment:\n%s", all)
	}
}

// A root directory mount's guide/ and a mount at guide from elsewhere are
// one directory of the tree, with the mount's label.
func TestMountSharedDirectory(t *testing.T) {
	s, err := mountSite(t, "[[mount source=\"..\"]]\n\n[[mount path=\"guide\" source=\"../../extra\" nav-title=\"Guide\"]]",
		map[string]string{
			"repo/docs/index.md":       "# Home\n",
			"repo/docs/guide/index.md": "# Docs Guide\n",
			"repo/docs/guide/a.md":     "# A\n",
			"repo/extra/b.md":          "# B\n",
		})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.outline(); got != "Guide\n  A\n  B\n" {
		t.Errorf("outline:\n%s", got)
	}
}

func TestMountTOCShellNeedsTheShellsList(t *testing.T) {
	res, root, err := mountCrawl(t, "[[mount source=\"../index.md\"]]\n\n[[mount path=\"guide\" source=\"../guide\" toc=\"shell\"]]",
		map[string]string{"repo/docs/index.md": "# Home\n", "repo/docs/guide/a.md": "# A\n"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(root, "repo/docs/site/shell.md")
	ms, _ := ReadMounts(shell, nil)
	opt := CrawlOptions{Mounts: ms, SiteShell: shell, OutDir: filepath.Join(root, "out")}
	if _, err := BuildSite(res, shell, opt); err == nil || !strings.Contains(err.Error(), `toc="shell" needs a [[toc]] in the shell`) {
		t.Errorf("err %v", err)
	}
}

// mountPage converts the page at rel (from the temp root) in a mounted site.
func mountPage(t *testing.T, body string, files map[string]string, rel string) (string, []string, *Site) {
	t.Helper()
	res, root, err := mountCrawl(t, body, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(root, "repo/docs/site/shell.md")
	ms, _ := ReadMounts(shell, nil)
	opt := CrawlOptions{Mounts: ms, SiteShell: shell, OutDir: filepath.Join(root, "out")}
	s, err := BuildSite(res, shell, opt)
	if err != nil {
		t.Fatal(err)
	}
	out, warnings := sitePage(t, s, res, root, rel)
	return out, warnings, s
}

func TestMountMarkersAndShellContents(t *testing.T) {
	files := map[string]string{
		"repo/docs/site/shell.md": "---\nsite: true\n---\n[[mount source=\"../index.md\"]]\n\n" +
			"[[mount path=\"guide\" source=\"../guide\" toc=\"shell\"]]\n\n[[mount path=\"plain\" source=\"../plain\"]]\n\n" +
			"::: nav {.side}\n[[toc]]\n:::\n\n[[content]]\n",
		"repo/docs/index.md":   "---\ntoc: none\n---\n# Home\n\n## H\n",
		"repo/docs/guide/a.md": "---\ntoc: float\n---\n# A\n\n[[toc]]\n\n## One\n\n```\n[[toc]]\n```\n",
		"repo/docs/plain/b.md": "# B\n\n[[toc]]\n\n## Two\n\n[[mount path=\"x\" source=\"y\"]]\n",
	}
	a, aw, _ := mountPage(t, "", files, "repo/docs/guide/a.md")
	if n := strings.Count(a, `<nav class="toc`); n != 1 {
		t.Errorf("a toc=\"shell\" page has %d contents lists, want the shell's one:\n%s", n, a)
	}
	if !strings.Contains(a, "[[toc]]") {
		t.Error("a [[toc]] inside a fence was touched")
	}
	if strings.Contains(a, "[[mount") {
		t.Error("the shell's [[mount]] markers were rendered")
	}
	if len(aw) != 0 {
		t.Errorf("warnings %q", aw)
	}
	b, bw, _ := mountPage(t, "", files, "repo/docs/plain/b.md")
	if n := strings.Count(b, `<nav class="toc`); n != 2 {
		t.Errorf("a page of a mount without toc=\"shell\" has %d contents lists, want its own and the shell's", n)
	}
	if !strings.Contains(strings.Join(bw, "\n"), "[[mount]] belongs in the site's shell") || !strings.Contains(b, "[[mount") {
		t.Errorf("a page's [[mount]] did not warn and stay: %q", bw)
	}
	home, _, _ := mountPage(t, "", files, "repo/docs/index.md")
	if strings.Contains(home, `<nav class="toc`) {
		t.Error("a toc: none page got the shell's contents list")
	}
}

func TestMountedSiteReadsInsideTheSourceRoot(t *testing.T) {
	files := map[string]string{
		"repo/docs/site/shell.md": "---\nsite: true\nsource-root: ..\n---\n[[mount source=\"../index.md\"]]\n\n[[content]]\n",
		"repo/docs/index.md":      "---\nversions: @../vals.md#all\n---\n# Home\n\n[[list key=\"versions\"]]\n",
		"repo/vals.md":            "---\nall: 1, 2\n---\n",
	}
	out, warnings, _ := mountPage(t, "", files, "repo/docs/index.md")
	if strings.Contains(out, ">1<") || !strings.Contains(strings.Join(warnings, "\n"), "outside the site's source root") {
		t.Errorf("a reference above the source root was read: %q\n%s", warnings, out)
	}
}

// A directory mount that yields no page is said, not left as a hole: one
// whose files are all excluded, and one with no Markdown at all.
func TestMountWithNoPagesWarns(t *testing.T) {
	s, err := mountSite(t, "[[mount source=\"../index.md\"]]\n\n"+
		"[[mount path=\"notes\" source=\"../specs\" exclude=\"*.md\"]]\n\n"+
		"[[mount path=\"art\" source=\"../art\"]]", map[string]string{
		"repo/docs/index.md":   "# Home\n",
		"repo/docs/specs/a.md": "# A\n",
		"repo/docs/art/x.txt":  "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"2", "3"} {
		want := "[[mount]] " + n + ": no pages: the mount's source holds no Markdown file the site builds"
		var count int
		for _, w := range s.Warnings {
			if strings.Contains(w.Message, want) {
				count++
				if !strings.HasSuffix(w.Src, "shell.md") {
					t.Errorf("warned under %s, want the shell", w.Src)
				}
			}
		}
		if count != 1 {
			t.Errorf("%d warnings containing %q: %v", count, want, s.Warnings)
		}
	}
}
