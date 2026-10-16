package md2html

import (
	"strings"
	"testing"
)

func navFixture(t *testing.T, shell string) (string, []string) {
	t.Helper()
	files := map[string]string{"shell/site.md": "---\nsite: true\n---\n" + shell}
	for k, v := range treeFiles {
		if k != "shell/site.md" {
			files[k] = v
		}
	}
	s, res, root := siteFixture(t, files, "shell/site.md")
	return sitePage(t, s, res, root, "docs/guides/a.md")
}

func TestPagesMarkersFromANestedPage(t *testing.T) {
	out, warnings := navFixture(t, "[[pages depth=\"1\"]]\n\n[[pages from=\"section\"]]\n\n[[content]]\n")
	if len(warnings) != 0 {
		t.Errorf("warnings: %q", warnings)
	}
	tabs := `<ul class="md2html-pages">` +
		`<li><a href="../start/index.html">Get started</a></li>` +
		`<li class="md2html-branch"><a href="index.html">Guides</a></li>` +
		`<li><a href="../reference/index.html">Reference</a></li></ul>`
	section := `<ul class="md2html-pages">` +
		`<li class="md2html-branch"><a href="a.html" aria-current="page">Alpha (short)</a></li>` +
		`<li><a href="b.html">Bravo</a></li></ul>`
	for _, want := range []string{tabs, section} {
		if !strings.Contains(out, want) {
			t.Errorf("missing\n%s\nin:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Secret") {
		t.Error("a hidden page is listed")
	}
}

func TestPagesNestsBelowItsDepth(t *testing.T) {
	out, _ := navFixture(t, "[[pages]]\n\n[[content]]\n")
	want := `<li class="md2html-branch"><a href="index.html">Guides</a><ul class="md2html-pages">` +
		`<li class="md2html-branch"><a href="a.html" aria-current="page">Alpha (short)</a></li>`
	if !strings.Contains(out, want) {
		t.Errorf("missing\n%s\nin:\n%s", want, out)
	}
}

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
	// The page's style may mention [[pages]] itself, so only the body is
	// searched.
	a, warnings := sitePage(t, s, res, root, "docs/guides/a.md")
	_, a, _ = strings.Cut(a, "<body")
	if len(warnings) != 0 || strings.Contains(a, `class="md2html-pages"`) || strings.Contains(a, "[[pages") {
		t.Errorf("a leaf's list: warnings %q, page:\n%s", warnings, a)
	}
}

// collapse="true" folds each branch into a <details> beside its link, so
// the tree is the same on every page and only which branches are open
// differs: the one holding the current page. The link stays a link, and
// the summary is the toggle alone, named for the page it opens.
func TestPagesCollapseFoldsEachBranch(t *testing.T) {
	out, warnings := navFixture(t, "[[pages collapse=\"true\"]]\n\n[[content]]\n")
	if len(warnings) != 0 {
		t.Errorf("warnings: %q", warnings)
	}
	for _, want := range []string{
		`<li class="md2html-fold"><a href="../start/index.html">Get started</a>` +
			`<details><summary aria-label="Get started"></summary><ul class="md2html-pages">` +
			`<li><a href="../start/install.html">Install</a></li></ul></details></li>`,
		`<li class="md2html-branch md2html-fold"><a href="index.html">Guides</a>` +
			`<details open=""><summary aria-label="Guides"></summary><ul class="md2html-pages">` +
			`<li class="md2html-branch"><a href="a.html" aria-current="page">Alpha (short)</a></li>`,
		`<li class="md2html-fold"><a href="../reference/index.html">Reference</a>` +
			`<details><summary aria-label="Reference"></summary><ul class="md2html-pages">`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing\n%s\nin:\n%s", want, out)
		}
	}
	// A level depth cuts off has nothing to fold. The page's style may
	// mention <details> itself, so only the body is searched.
	folds := func(shell string) bool {
		out, _ := navFixture(t, shell)
		_, body, _ := strings.Cut(out, "<body")
		return strings.Contains(body, "<details")
	}
	if folds("[[pages depth=\"1\" collapse=\"true\"]]\n\n[[content]]\n") {
		t.Error("a depth-limited list folded")
	}
	if folds("[[pages collapse=\"false\"]]\n\n[[content]]\n") {
		t.Error(`collapse="false" folded`)
	}
}

// A folding tree's outer list says so, for the style, and says which side
// its chevrons go: the left unless chevron="right".
func TestPagesCollapseMarksTheChevronSide(t *testing.T) {
	for shell, want := range map[string]string{
		`[[pages collapse="true"]]`:                 `<ul class="md2html-pages md2html-collapse"><li class="md2html-fold">`,
		`[[pages collapse="true" chevron="left"]]`:  `<ul class="md2html-pages md2html-collapse"><li class="md2html-fold">`,
		`[[pages collapse="true" chevron="right"]]`: `<ul class="md2html-pages md2html-collapse md2html-chevron-right"><li class="md2html-fold">`,
		`[[pages]]`: `<ul class="md2html-pages"><li><a href="../start/index.html">`,
	} {
		out, warnings := navFixture(t, shell+"\n\n[[content]]\n")
		if len(warnings) != 0 || !strings.Contains(out, want) {
			t.Errorf("%s: warnings %q; missing\n%s\nin:\n%s", shell, warnings, want, out)
		}
	}
}

func TestBreadcrumbAndPrevNext(t *testing.T) {
	out, _ := navFixture(t, "[[breadcrumb]]\n\n[[content]]\n\n[[prev-next]]\n")
	crumbs := `<nav aria-label="Breadcrumb"><ol class="md2html-breadcrumb"><li><a href="../index.html">Home</a></li>` +
		`<li><a href="index.html">Guides</a></li>` +
		`<li><a href="a.html" aria-current="page">Alpha (short)</a></li></ol></nav>`
	prevNext := `<nav class="md2html-prev-next" aria-label="Previous and next">` +
		`<a href="index.html" class="md2html-prev" rel="prev"><span class="md2html-dir">Previous</span> <span class="md2html-title">Guides</span></a>` +
		`<a href="b.html" class="md2html-next" rel="next"><span class="md2html-dir">Next</span> <span class="md2html-title">Bravo</span></a></nav>`
	for _, want := range []string{crumbs, prevNext} {
		if !strings.Contains(out, want) {
			t.Errorf("missing\n%s\nin:\n%s", want, out)
		}
	}
}

func TestNavMarkersInAPageBodyUnderASite(t *testing.T) {
	files := map[string]string{"shell/site.md": minimalShell, "docs/index.md": "# Home\n\n[[pages depth=\"1\"]]\n", "docs/a.md": "# A\n"}
	s, res, root := siteFixture(t, files, "shell/site.md")
	out, warnings := sitePage(t, s, res, root, "docs/index.md")
	if len(warnings) != 0 || !strings.Contains(out, `<a href="a.html">A</a>`) {
		t.Errorf("warnings %q, page:\n%s", warnings, out)
	}
}

// The shell's warnings are the shell's: they go to the site's, once, not to
// the page that happened to be converted.
func TestPagesBadAttributesWarn(t *testing.T) {
	files := map[string]string{"shell/site.md": "---\nsite: true\n---\n[[pages depth=\"two\" from=\"top\" collapse=\"yes\" chevron=\"up\"]]\n\n[[content]]\n"}
	for k, v := range treeFiles {
		if k != "shell/site.md" {
			files[k] = v
		}
	}
	s, res, root := siteFixture(t, files, "shell/site.md")
	_, pageWarnings := sitePage(t, s, res, root, "docs/guides/a.md")
	sitePage(t, s, res, root, "docs/guides/b.md")
	if len(pageWarnings) != 0 {
		t.Errorf("the page was warned about the shell: %q", pageWarnings)
	}
	var all []string
	for _, w := range s.Warnings {
		all = append(all, w.Src+": "+w.Message)
	}
	joined := strings.Join(all, "\n")
	for _, want := range []string{`depth "two"`, `from "top"`, `collapse "yes"`, `chevron "up"`} {
		if strings.Count(joined, want) != 1 {
			t.Errorf("want one site warning mentioning %s; got %q", want, all)
		}
	}
}

func TestContentMarkerInAPageBodyWarns(t *testing.T) {
	files := map[string]string{"shell/site.md": minimalShell, "docs/index.md": "# Home\n\n[[content]]\n"}
	s, res, root := siteFixture(t, files, "shell/site.md")
	_, warnings := sitePage(t, s, res, root, "docs/index.md")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "[[content]] belongs in the site's shell") {
		t.Errorf("warnings: %q", warnings)
	}
}

// On the root page the trail would be the page alone, which says nothing:
// [[breadcrumb]] draws no trail there, and still does one level down.
func TestBreadcrumbIsOmittedOnTheRootPage(t *testing.T) {
	files := map[string]string{"shell/site.md": "---\nsite: true\n---\n[[breadcrumb]]\n\n[[content]]\n"}
	for k, v := range treeFiles {
		if k != "shell/site.md" {
			files[k] = v
		}
	}
	s, res, root := siteFixture(t, files, "shell/site.md")
	home, warnings := sitePage(t, s, res, root, "docs/index.md")
	if strings.Contains(home, `aria-label="Breadcrumb"`) || strings.Contains(home, "[[breadcrumb]]") {
		t.Errorf("the root page has a breadcrumb:\n%s", home)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings: %q", warnings)
	}
	if page, _ := sitePage(t, s, res, root, "docs/start/install.md"); !strings.Contains(page, `aria-label="Breadcrumb"`) {
		t.Errorf("a page below the root lost its breadcrumb:\n%s", page)
	}
}

// A page whose section has nothing below it, the root page or a top-level
// page of its own, would get an empty [[pages from="section"]]; it gets the
// whole tree instead, so a section sidebar is never blank.
func TestPagesFromSectionFallsBackToTheWholeTree(t *testing.T) {
	files := map[string]string{"shell/site.md": "---\nsite: true\n---\n[[pages from=\"section\"]]\n\n[[content]]\n"}
	for k, v := range treeFiles {
		if k != "shell/site.md" {
			files[k] = v
		}
	}
	files["docs/about.md"] = "# About\n"
	s, res, root := siteFixture(t, files, "shell/site.md")
	for _, rel := range []string{"docs/index.md", "docs/about.md"} {
		out, warnings := sitePage(t, s, res, root, rel)
		if len(warnings) != 0 {
			t.Errorf("%s: warnings %q", rel, warnings)
		}
		for _, want := range []string{">Get started<", ">Install<", ">Guides<", ">Bravo<", ">About<"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: the sidebar lacks %s:\n%s", rel, want, out)
			}
		}
	}
	// A page with a section still lists that section alone.
	if out, _ := sitePage(t, s, res, root, "docs/guides/b.md"); strings.Contains(out, ">Install<") {
		t.Errorf("a page in Guides listed another section:\n%s", out)
	}
}
