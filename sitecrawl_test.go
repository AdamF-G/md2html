package md2html

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// siteCrawl crawls root/repo/docs as a site wrapped in
// root/repo/site/shell.md, into root/out. files are relative to repo/, so
// "../far.txt" sits beside the repository, outside the default source
// root, which is repo/.
func siteCrawl(t *testing.T, shellFront string, files map[string]string, mod func(*CrawlOptions)) (*CrawlResult, string, error) {
	t.Helper()
	all := map[string]string{"repo/site/shell.md": "---\nsite: true\n" + shellFront + "---\n[[content]]\n"}
	for k, v := range files {
		all["repo/"+k] = v
	}
	root := writeTree(t, all)
	opt := CrawlOptions{Entries: []string{filepath.Join(root, "repo/docs")}, OutDir: filepath.Join(root, "out"),
		Depth: -1, LinkDepth: -1, SiteShell: filepath.Join(root, "repo/site/shell.md")}
	if mod != nil {
		mod(&opt)
	}
	res, err := Crawl(opt)
	return res, root, err
}

var siteLinkFiles = map[string]string{
	"docs/index.md": "# Home\n\n" +
		"![flow](img/flow.png) [guide](guide.md) [pdf](files/my%20doc.pdf)\n\n" +
		"[licence](../LICENSE) [readme](../README.md#install) [code](../site.go#L10)\n\n" +
		"![logo](../logo.png)\n\n" +
		"[far](../../far.txt) ![farpic](../../farpic.png)\n",
	"docs/guide.md":         "# Guide\n",
	"docs/img/flow.png":     "png",
	"docs/files/my doc.pdf": "pdf",
	"LICENSE":               "MIT",
	"README.md":             "# Readme\n",
	"site.go":               "package x",
	"logo.png":              "png",
	"../far.txt":            "far",
	"../farpic.png":         "png",
}

func TestSiteCrawlKeepsTheSiteToItself(t *testing.T) {
	res, root, err := siteCrawl(t, "repo-url: https://example.org/r/blob/main/{path}\n", siteLinkFiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	// ../../far.txt and ../../farpic.png are beside repo/, above the source
	// root, so they have no path from it.
	for _, d := range res.Docs {
		if strings.HasSuffix(d.Src, "README.md") {
			t.Error("a page outside the site was built")
		}
	}
	if len(res.External) != 0 {
		t.Errorf("External = %v", res.External)
	}
	idx := docByBase(res.Docs, "index.md")
	for href, want := range map[string]string{
		"img/flow.png":         "img/flow.png",
		"guide.md":             "guide.html",
		"files/my%20doc.pdf":   "files/my%20doc.pdf",
		"../LICENSE":           "https://example.org/r/blob/main/LICENSE",
		"../README.md#install": "https://example.org/r/blob/main/README.md#install",
		"../site.go#L10":       "https://example.org/r/blob/main/site.go#L10",
		"../logo.png":          "_assets/logo.png",
	} {
		// An image's src is planned apart from an <a>'s href.
		got := idx.AnchorLinkMap[href]
		if href == "img/flow.png" || href == "../logo.png" {
			got = idx.LinkMap[href]
		}
		if got != want {
			t.Errorf("%s -> %q, want %q", href, got, want)
		}
	}
	if !idx.AnchorLinkDrops["../../far.txt"] {
		t.Error("../../far.txt not dropped")
	}
	if !idx.LinkDrops["../../farpic.png"] {
		t.Error("../../farpic.png not dropped")
	}
	var outs []string
	for _, a := range res.Assets {
		rel, _ := filepath.Rel(filepath.Join(root, "out"), a.Out)
		outs = append(outs, filepath.ToSlash(rel))
	}
	sort.Strings(outs)
	if got := strings.Join(outs, ","); got != "_assets/logo.png,files/my doc.pdf,img/flow.png" {
		t.Errorf("assets %s", got)
	}
	var warns []string
	for _, w := range res.Warnings {
		warns = append(warns, w.Message)
	}
	all := strings.Join(warns, "\n")
	for _, want := range []string{"unlinked ../../far.txt", "dropped ../../farpic.png"} {
		if !strings.Contains(all, want) {
			t.Errorf("no warning %q in:\n%s", want, all)
		}
	}
}

func TestSiteCrawlUnlinksWithoutARepoURL(t *testing.T) {
	res, _, err := siteCrawl(t, "", siteLinkFiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	idx := docByBase(res.Docs, "index.md")
	for _, href := range []string{"../LICENSE", "../README.md#install", "../site.go#L10"} {
		if !idx.AnchorLinkDrops[href] {
			t.Errorf("%s not unlinked", href)
		}
	}
	if idx.LinkMap["../logo.png"] != "_assets/logo.png" {
		t.Error("an image from outside the site was not copied")
	}
}

func TestSiteCrawlStopsUnderUnmountedError(t *testing.T) {
	_, _, err := siteCrawl(t, "unmounted: error\n", siteLinkFiles, nil)
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"index.md", "../LICENSE", "../README.md#install", "../../farpic.png"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
}

// --no-assets copies nothing and leaves what would have been copied as
// written, while a link out of the site still follows unmounted:.
func TestSiteCrawlNoAssets(t *testing.T) {
	res, _, err := siteCrawl(t, "repo-url: https://example.org/r/blob/main/{path}\n", siteLinkFiles,
		func(o *CrawlOptions) { o.NoAssets = true })
	if err != nil {
		t.Fatal(err)
	}
	idx := docByBase(res.Docs, "index.md")
	if len(res.Assets) != 0 {
		t.Errorf("assets %v", res.Assets)
	}
	for _, href := range []string{"img/flow.png", "files/my%20doc.pdf", "../logo.png"} {
		_, ok := idx.LinkMap[href]
		_, aok := idx.AnchorLinkMap[href]
		if ok || aok {
			t.Errorf("%s was rewritten", href)
		}
	}
	if idx.AnchorLinkMap["../LICENSE"] != "https://example.org/r/blob/main/LICENSE" {
		t.Error("a link out of the site did not follow unmounted:")
	}
}

func TestSiteCrawlRefusesAnAssetOverAPage(t *testing.T) {
	_, _, err := siteCrawl(t, "", map[string]string{
		"docs/a.md":   "# A\n\n[old](a.html)\n",
		"docs/a.html": "<p>old</p>",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "output path collision") {
		t.Errorf("err %v", err)
	}
}

// An excluded file keeps its link as written, as --exclude promises.
func TestSiteCrawlLeavesAnExcludedFileAlone(t *testing.T) {
	res, _, err := siteCrawl(t, "", map[string]string{
		"docs/a.md":         "# A\n\n![x](vendor/x.png)\n",
		"docs/vendor/x.png": "png",
	}, func(o *CrawlOptions) { o.Exclude = []string{"vendor"} })
	if err != nil {
		t.Fatal(err)
	}
	a := docByBase(res.Docs, "a.md")
	if _, ok := a.LinkMap["vendor/x.png"]; ok || a.LinkDrops["vendor/x.png"] || len(res.Assets) != 0 {
		t.Errorf("map %v drops %v assets %v", a.LinkMap, a.LinkDrops, res.Assets)
	}
}

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

// A page under the base that the crawl did not reach is still in the site.
func TestSiteCrawlLeavesAnUnbuiltPageInTheSiteAlone(t *testing.T) {
	res, _, err := siteCrawl(t, "unmounted: error\n", map[string]string{
		"docs/index.md":    "# Home\n\n[deep](sub/deep.md)\n",
		"docs/sub/deep.md": "# Deep\n",
	}, func(o *CrawlOptions) { o.Depth = 0; o.LinkDepth = 0 })
	if err != nil {
		t.Fatal(err)
	}
	idx := docByBase(res.Docs, "index.md")
	if _, ok := idx.AnchorLinkMap["sub/deep.md"]; ok || idx.AnchorLinkDrops["sub/deep.md"] {
		t.Error("an unbuilt page's link was changed")
	}
	for _, w := range res.Warnings {
		if strings.Contains(w.Message, "deep.md") {
			t.Errorf("warning %q", w.Message)
		}
	}
}

func TestSiteCrawlKeepsTheFragmentOfACopiedImage(t *testing.T) {
	res, _, err := siteCrawl(t, "", map[string]string{
		"docs/index.md": "# Home\n\n![i](../icons.svg#x)\n",
		"icons.svg":     "<svg/>",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := docByBase(res.Docs, "index.md").LinkMap["../icons.svg#x"]; got != "_assets/icons.svg#x" {
		t.Errorf("got %q", got)
	}
}

// An image the page shows and a link a reader follows can name the same file
// out of the site, as a clickable image does: the image is copied and the
// link goes where unmounted: says, and neither changes what the other is.
func TestSiteCrawlKeepsAClickableImageFromOutsideTheSite(t *testing.T) {
	files := map[string]string{
		"docs/index.md": "# Home\n\n[![logo](../logo.png)](../logo.png)\n",
		"logo.png":      "png",
	}
	for _, tc := range []struct{ front, want string }{
		{"repo-url: https://example.org/r/blob/main/{path}\n", `<a href="https://example.org/r/blob/main/logo.png"><img src="_assets/logo.png" alt="logo"/></a>`},
		{"", `<p><img src="_assets/logo.png" alt="logo"/></p>`},
	} {
		var opt CrawlOptions
		res, root, err := siteCrawl(t, tc.front, files, func(o *CrawlOptions) { opt = *o })
		if err != nil {
			t.Fatal(err)
		}
		s, err := BuildSite(res, opt.SiteShell, opt)
		if err != nil {
			t.Fatal(err)
		}
		out, _ := sitePage(t, s, res, root, "repo/docs/index.md")
		if !strings.Contains(out, tc.want) {
			t.Errorf("repo-url %q: no %s in\n%s", tc.front, tc.want, mainOf(out))
		}
		if len(res.Assets) != 1 || res.Assets[0].Src != filepath.Join(root, "repo/logo.png") {
			t.Errorf("repo-url %q: assets %v", tc.front, res.Assets)
		}
	}
}

// A shell reached through a symlink is read where it really is: its
// relative image is planned from there by the crawl and by every page.
func TestSiteCrawlReadsTheShellWhereItIs(t *testing.T) {
	root := writeTree(t, map[string]string{
		"repo/site/shell.md":  "---\nsite: true\n---\n![brand](brand.png)\n\n[[content]]\n",
		"repo/site/brand.png": "png",
		"repo/docs/index.md":  "# Home\n",
	})
	if err := os.Symlink(filepath.Join(root, "repo/site/shell.md"), filepath.Join(root, "repo/shell.md")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	opt := CrawlOptions{Entries: []string{filepath.Join(root, "repo/docs")}, OutDir: filepath.Join(root, "out"),
		Depth: -1, LinkDepth: -1, SiteShell: filepath.Join(root, "repo/shell.md")}
	res, err := Crawl(opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Assets) != 1 || res.Assets[0].Src != filepath.Join(root, "repo/site/brand.png") {
		t.Fatalf("assets %v", res.Assets)
	}
	s, err := BuildSite(res, opt.SiteShell, opt)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := sitePage(t, s, res, root, "repo/docs/index.md")
	rel, _ := filepath.Rel(opt.OutDir, res.Assets[0].Out)
	if want := `src="` + filepath.ToSlash(rel) + `"`; !strings.Contains(out, want) {
		t.Errorf("no %s in\n%s", want, mainOf(out))
	}
}

// mainOf is a page's <main>, which is all a failure needs to show.
func mainOf(page string) string {
	if i := strings.Index(page, "<main"); i >= 0 {
		page = page[i:]
	}
	if i := strings.Index(page, "</main>"); i >= 0 {
		page = page[:i+len("</main>")]
	}
	return page
}
