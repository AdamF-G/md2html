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
