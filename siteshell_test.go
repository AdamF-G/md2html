package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sitePage converts one document of a fixture the way the CLI does.
func sitePage(t *testing.T, s *Site, res *CrawlResult, root, rel string) (string, []string) {
	t.Helper()
	src := filepath.Join(root, rel)
	var doc *Doc
	for i := range res.Docs {
		if res.Docs[i].Src == src {
			doc = &res.Docs[i]
		}
	}
	if doc == nil {
		t.Fatalf("%s is not in the emit set", rel)
	}
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	var warnings []string
	out, err := Convert(b, Options{SourcePath: src, LinkMap: doc.LinkMap, Site: s, NoSource: true,
		Warn: func(m string) { warnings = append(warnings, m) }})
	if err != nil {
		t.Fatalf("Convert %s: %v", rel, err)
	}
	return string(out), warnings
}

var shellFiles = map[string]string{
	"shell/site.md":         "---\nsite: true\n---\n[Home](../docs/index.md)\n\n[[toc]]\n\n[[content]]\n\nFooter\n",
	"docs/index.md":         "# Home\n",
	"docs/guides/index.md":  "# Guides\n",
	"docs/guides/deep/a.md": "# Alpha\n\n## Part one\n\n[Up](../index.md)\n",
	"docs/bare.md":          "---\nshell: none\n---\n# Bare\n",
}

func TestSiteWrapsEachPage(t *testing.T) {
	s, res, root := siteFixture(t, shellFiles, "shell/site.md")
	out, warnings := sitePage(t, s, res, root, "docs/guides/deep/a.md")
	if len(warnings) != 0 {
		t.Errorf("warnings: %q", warnings)
	}
	for _, want := range []string{
		`<html lang="en" data-section="guides">`,
		`<a href="../../index.html">Home</a>`, // the shell's link, from two levels down
		`<a href="../index.html">Up</a>`,      // the page's own link, rewritten once
		`href="#part-one"`,                    // the shell's [[toc]] lists the page
		`<h1 id="alpha"`,
		`<p>Footer</p>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[[content]]") || strings.Contains(out, "[[toc]]") {
		t.Errorf("a marker survived:\n%s", out)
	}
	if strings.Index(out, "Home</a>") > strings.Index(out, `<h1 id="alpha"`) {
		t.Error("the shell's content before [[content]] should come before the page")
	}
	// [[content]] is the page's <main>; the shell sits around it, in <body>.
	main, end := strings.Index(out, `<main class="md2html">`), strings.Index(out, "</main>")
	if strings.Count(out, "<main") != 1 || main < 0 ||
		strings.Index(out, "Home</a>") > main || strings.Index(out, "<p>Footer</p>") < end ||
		strings.Index(out, `<h1 id="alpha"`) < main {
		t.Errorf("want one <main> holding the page and the shell outside it:\n%s", out)
	}
}

func TestSiteShellIDsGiveWayToThePage(t *testing.T) {
	s, res, root := siteFixture(t, map[string]string{
		"shell/site.md": "---\nsite: true\n---\n## Part one\n\n[Back](#part-one)\n\n[[toc]]\n\n[[content]]\n",
		"docs/index.md": "# Home\n",
		"docs/a.md":     "# Alpha\n\n## Part one\n",
	}, "shell/site.md")
	out, _ := sitePage(t, s, res, root, "docs/a.md")
	for _, want := range []string{
		`<h2 id="part-one-1"`,                  // the shell's heading, renumbered
		`<a href="#part-one-1">Back</a>`,       // the shell's own link follows it
		`<a class="anchor" href="#part-one-1"`, // and so does its anchor
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	if strings.Count(out, `id="part-one"`) != 1 {
		t.Errorf("want the page's id once, on the page's heading:\n%s", out)
	}
	// The shell's contents list is the page's, so it still points at the
	// page's heading, not the renumbered one.
	toc := out[strings.Index(out, `class="toc`):]
	if toc = toc[:strings.Index(toc, "</nav>")]; !strings.Contains(toc, `href="#part-one"`) {
		t.Errorf("the contents list should link the page's heading:\n%s", toc)
	}
}

func TestSiteContentsListsAreInline(t *testing.T) {
	s, res, root := siteFixture(t, map[string]string{
		"shell/site.md": "---\nsite: true\n---\n[[toc]]\n\n[[content]]\n",
		"docs/index.md": "# Home\n",
		"docs/a.md":     "---\ntoc: float\n---\n# Alpha\n\n## One\n\n## Two\n",
	}, "shell/site.md")
	src := filepath.Join(root, "docs/a.md")
	var doc Doc
	for _, d := range res.Docs {
		if d.Src == src {
			doc = d
		}
	}
	b, _ := os.ReadFile(src)
	var warnings []string
	out, err := Convert(b, Options{SourcePath: src, LinkMap: doc.LinkMap, Site: s, NoSource: true, AutoTOC: "all",
		Warn: func(m string) { warnings = append(warnings, m) }})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(out), `<nav class="toc`); n != 1 {
		t.Errorf("%d contents lists, want 1: the shell's [[toc]] is the page's, so --autotoc adds none", n)
	}
	if strings.Contains(string(out), `<nav class="toc toc-float"`) {
		t.Error("a contents list under a shell floated")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], `toc "float"`) {
		t.Errorf("warnings = %q, want one about toc: float", warnings)
	}
}

func TestSiteShellStyleIsTheDefault(t *testing.T) {
	s, _, root := siteFixture(t, map[string]string{
		"shell/site.md": "---\nsite: true\nstyle: claude-artifact\n---\n[[content]]\n",
		"docs/index.md": "# Home\n",
		"docs/own.md":   "---\nstyle: default\n---\n# Own\n",
	}, "shell/site.md")
	chosen := func(rel string) []string {
		var names []string
		src := filepath.Join(root, rel)
		b, _ := os.ReadFile(src)
		if _, err := Convert(b, Options{SourcePath: src, Site: s, NoSource: true,
			StyleChosen: func(n, _ []string, _ string) { names = n }}); err != nil {
			t.Fatal(err)
		}
		return names
	}
	if got := chosen("docs/index.md"); len(got) != 1 || got[0] != "claude-artifact/claude-artifact" {
		t.Errorf("index.md styles = %q, want the shell's", got)
	}
	if got := chosen("docs/own.md"); len(got) != 1 || got[0] != "default/default" {
		t.Errorf("own.md styles = %q, want its own", got)
	}
}

func TestSiteShellNoneSkipsTheShell(t *testing.T) {
	s, res, root := siteFixture(t, shellFiles, "shell/site.md")
	out, _ := sitePage(t, s, res, root, "docs/bare.md")
	if strings.Contains(out, "Footer") {
		t.Errorf("shell: none page was wrapped:\n%s", out)
	}
}

func TestSiteRootPageHasNoSection(t *testing.T) {
	s, res, root := siteFixture(t, shellFiles, "shell/site.md")
	out, _ := sitePage(t, s, res, root, "docs/index.md")
	if !strings.Contains(out, `<html lang="en">`) {
		t.Errorf("the root page should carry no data-section:\n%s", out)
	}
}

func TestSiteShellWarningsAreTheShellsOnce(t *testing.T) {
	s, res, root := siteFixture(t, map[string]string{
		"shell/site.md": "---\nsite: true\n---\n::: bogus\nx\n:::\n\n[[content]]\n",
		"docs/index.md": "# Home\n",
		"docs/a.md":     "# Alpha\n",
	}, "shell/site.md")
	before := len(s.Warnings)
	for _, rel := range []string{"docs/index.md", "docs/a.md"} {
		if _, warnings := sitePage(t, s, res, root, rel); len(warnings) != 0 {
			t.Errorf("%s warnings = %q, want none: the problem is the shell's", rel, warnings)
		}
	}
	got := s.Warnings[before:]
	if len(got) != 1 || got[0].Src != s.shellSrc || !strings.Contains(got[0].Message, "bogus") {
		t.Errorf("site warnings = %+v, want one about bogus, from %s", got, s.shellSrc)
	}
}

func TestSiteShellRepeatedIDAllGiveWay(t *testing.T) {
	s, res, root := siteFixture(t, map[string]string{
		"shell/site.md": "---\nsite: true\n---\n<div id=\"alpha\">one</div>\n\n<div id=\"alpha\">two</div>\n\n[[content]]\n",
		"docs/index.md": "# Home\n",
		"docs/a.md":     "# Alpha\n",
	}, "shell/site.md")
	out, _ := sitePage(t, s, res, root, "docs/a.md")
	if n := strings.Count(out, `id="alpha"`); n != 1 {
		t.Errorf("page id appears %d times, want once:\n%s", n, out)
	}
	if n := strings.Count(out, `id="alpha-1"`); n != 2 {
		t.Errorf("want both shell copies renumbered to alpha-1, got %d:\n%s", n, out)
	}
}

// A shell link the link map leaves alone still has to reach its target
// from every page: in place, assets are not mapped, and a document the
// build does not emit is not either.
func TestSiteShellLinksReachTheirTargetFromEveryPage(t *testing.T) {
	root := writeTree(t, map[string]string{
		"shell/site.md":           "---\nsite: true\n---\n![logo](logo.svg)\n\n[Readme](../README.md#top)\n\n[Home](../docs/index.md)\n\n[Here](#x)\n\n[[content]]\n",
		"shell/logo.svg":          "<svg/>",
		"README.md":               "# Readme\n",
		"docs/index.md":           "# Home\n",
		"docs/guide/deep/page.md": "# Page\n",
	})
	opt := CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, Depth: -1, LinkDepth: -1}
	res := mustCrawl(t, opt)
	s, err := BuildSite(res, filepath.Join(root, "shell/site.md"), opt)
	if err != nil {
		t.Fatal(err)
	}
	before := len(s.Warnings)
	for _, rel := range []string{"docs/guide/deep/page.md", "docs/index.md"} {
		out, _ := sitePage(t, s, res, root, rel)
		up := "../../../"
		if rel == "docs/index.md" {
			up = "../"
		}
		for _, want := range []string{
			`src="` + up + `shell/logo.svg"`,
			`href="` + up + `README.md#top"`,
			`href="#x"`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: missing %s in:\n%s", rel, want, out)
			}
		}
	}
	got := s.Warnings[before:]
	if len(got) != 1 || got[0].Src != s.shellSrc || !strings.Contains(got[0].Message, "../README.md#top") {
		t.Errorf("site warnings = %+v, want one about the README link, from the shell", got)
	}
}

// Under the shell's style, a page's plugins without a style of its own
// are ignored, and said so, as without a site.
func TestSitePagePluginsWithoutStyleWarn(t *testing.T) {
	s, res, root := siteFixture(t, map[string]string{
		"shell/site.md":               "---\nsite: true\nstyle: claude-artifact\n---\n[[content]]\n",
		"docs/index.md":               "---\nplugins: ./_p\n---\n# Home\n",
		"docs/_p/md2html-plugin.json": `{"format": 1, "name": "page", "styles": {"p": {"css": ["p.css"]}}}`,
		"docs/_p/p.css":               "/*page*/",
	}, "shell/site.md")
	_, warnings := sitePage(t, s, res, root, "docs/index.md")
	if len(warnings) != 1 || warnings[0] != "front matter plugins has no effect without style; ignoring it" {
		t.Errorf("warnings = %q", warnings)
	}
}

// A fragment ignores the site, so its navigation markers stay as written,
// and say so, as without a site.
func TestSiteFragmentWarnsAboutNavMarkers(t *testing.T) {
	s, _, root := siteFixture(t, shellFiles, "shell/site.md")
	src := filepath.Join(root, "docs/index.md")
	var warnings []string
	out, err := Convert([]byte("# Home\n\n[[pages]]\n"), Options{SourcePath: src, Site: s, Fragment: true,
		Warn: func(m string) { warnings = append(warnings, m) }})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "[[pages]]") {
		t.Errorf("the marker should stay as written:\n%s", out)
	}
	if len(warnings) != 1 || warnings[0] != "[[pages]] needs a site shell (--site); leaving it as written" {
		t.Errorf("warnings = %q", warnings)
	}
}
