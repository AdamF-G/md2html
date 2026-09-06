package md2html

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func docByBase(docs []Doc, base string) *Doc {
	for i := range docs {
		if filepath.Base(docs[i].Src) == base {
			return &docs[i]
		}
	}
	return nil
}

// Document links resolve inside the output tree.
func TestLinkMapRewritesDocLinksWithinSite(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md":    "[deep](./api/auth.md)",
		"docs/api/auth.md": "[home](../index.md)",
	})
	res, err := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs")},
		OutDir:  filepath.Join(root, "site"),
		Depth:   -1,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	idx := docByBase(res.Docs, "index.md")
	if idx == nil {
		t.Fatal("index.md missing")
	}
	if got := idx.LinkMap["./api/auth.md"]; got != "api/auth.html" {
		t.Errorf("got %q, want %q", got, "api/auth.html")
	}
	auth := docByBase(res.Docs, "auth.md")
	if got := auth.LinkMap["../index.md"]; got != "../index.html" {
		t.Errorf("got %q, want %q", got, "../index.html")
	}
}

// A fragment on a document link survives rewriting.
func TestLinkMapPreservesFragment(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/a.md": "[b](./b.md#setup)",
		"docs/b.md": "# B",
	})
	res, _ := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs")},
		OutDir:  filepath.Join(root, "site"),
		Depth:   -1,
	})
	a := docByBase(res.Docs, "a.md")
	if got := a.LinkMap["./b.md#setup"]; got != "b.html#setup" {
		t.Errorf("got %q, want %q", got, "b.html#setup")
	}
}

// Assets are never copied: the link climbs out of the site directory back
// to the original file.
func TestLinkMapAssetsClimbOutOfSite(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/api/auth.md":      "![f](./img/flow.png)",
		"docs/api/img/flow.png": "PNG",
	})
	res, err := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs")},
		OutDir:  filepath.Join(root, "site"),
		Depth:   -1,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	auth := docByBase(res.Docs, "auth.md")
	got := auth.LinkMap["./img/flow.png"]
	// site/api/auth.html -> docs/api/img/flow.png
	if got != "../../docs/api/img/flow.png" {
		t.Errorf("got %q, want %q", got, "../../docs/api/img/flow.png")
	}
	if !strings.HasPrefix(got, "../") {
		t.Errorf("asset link must climb out of the site dir, got %q", got)
	}
}

// In place, the HTML sits beside its source, so asset links already resolve.
func TestLinkMapLeavesAssetsAloneInPlace(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/a.md":      "![f](./img/f.png)",
		"docs/img/f.png": "PNG",
	})
	res, _ := Crawl(CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, Depth: -1})
	a := docByBase(res.Docs, "a.md")
	if got, ok := a.LinkMap["./img/f.png"]; ok && got != "./img/f.png" {
		t.Errorf("in-place asset link rewritten to %q, want unchanged", got)
	}
}

func TestNoAssetsFlagSuppressesAssetRewriting(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/a.md":      "![f](./img/f.png)",
		"docs/img/f.png": "PNG",
	})
	res, _ := Crawl(CrawlOptions{
		Entries:  []string{filepath.Join(root, "docs")},
		OutDir:   filepath.Join(root, "site"),
		Depth:    -1,
		NoAssets: true,
	})
	a := docByBase(res.Docs, "a.md")
	if _, ok := a.LinkMap["./img/f.png"]; ok {
		t.Error("asset link rewritten despite NoAssets")
	}
}

func TestLinkRewriteTransformAppliesMap(t *testing.T) {
	m := map[string]string{"./guide.md": "guide.html", "./img/x.png": "../src/img/x.png"}
	got := apply(t, `<a href="./guide.md">g</a><img src="./img/x.png">`, LinkRewrite(m))
	if !strings.Contains(got, `href="guide.html"`) {
		t.Errorf("doc link not rewritten\ngot: %s", got)
	}
	if !strings.Contains(got, `src="../src/img/x.png"`) {
		t.Errorf("asset link not rewritten\ngot: %s", got)
	}
}

func TestLinkRewriteLeavesUnmappedLinksAlone(t *testing.T) {
	got := apply(t, `<a href="https://example.com">e</a><a href="#f">f</a>`, LinkRewrite(map[string]string{}))
	if !strings.Contains(got, `href="https://example.com"`) || !strings.Contains(got, `href="#f"`) {
		t.Errorf("unmapped links altered\ngot: %s", got)
	}
}

// Options.LinkMap must take effect without the caller knowing about
// LinkRewrite — the library-user path.
func TestConvertAppliesLinkMapFromOptions(t *testing.T) {
	got, err := Convert([]byte("[g](./guide.md)"), Options{
		LinkMap: map[string]string{"./guide.md": "guide.html"},
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(string(got), `href="guide.html"`) {
		t.Errorf("Options.LinkMap not applied: %s", got)
	}
}

// A rewritten href must be percent-encoded again: ExtractLinks decodes %XX
// before touching the filesystem, so a replacement computed from filesystem
// paths would otherwise emit a raw space (or #, ?, %) into the HTML and
// resolve to nothing.
func TestLinkMapPercentEncodesRewrittenHrefs(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md":       "[d](<./my doc.md>)\n\n![i](<./img/my img.png>)",
		"docs/my doc.md":      "# My Doc",
		"docs/img/my img.png": "PNG",
	})
	site := filepath.Join(root, "site")
	res := mustCrawl(t, CrawlOptions{
		Entries: []string{filepath.Join(root, "docs")},
		OutDir:  site,
		Depth:   -1,
	})
	idx := docByBase(res.Docs, "index.md")
	if idx == nil {
		t.Fatal("index.md missing")
	}
	if got, want := idx.LinkMap["./my%20doc.md"], "my%20doc.html"; got != want {
		t.Errorf("doc link: got %q, want %q", got, want)
	}
	if got, want := idx.LinkMap["./img/my%20img.png"], "../docs/img/my%20img.png"; got != want {
		t.Errorf("asset link: got %q, want %q", got, want)
	}

	// Emit the whole set, then follow the href the way a browser would.
	for _, d := range res.Docs {
		src, err := os.ReadFile(d.Src)
		if err != nil {
			t.Fatal(err)
		}
		out, err := Convert(src, Options{SourcePath: d.Src, LinkMap: d.LinkMap})
		if err != nil {
			t.Fatalf("Convert %s: %v", d.Src, err)
		}
		if _, err := SafeWrite(d.Out, out); err != nil {
			t.Fatalf("SafeWrite %s: %v", d.Out, err)
		}
	}
	page, err := os.ReadFile(idx.Out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `href="my%20doc.html"`) {
		t.Fatalf("href not percent-encoded in the emitted page: %s", page)
	}
	target, err := url.PathUnescape("my%20doc.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(idx.Out), target)); err != nil {
		t.Errorf("emitted href names a file that does not exist: %v", err)
	}
}

func TestNoMdLinksFlagSuppressesDocRewriting(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md":  "[a](./a.md)\n\n![f](./img/f.png)",
		"docs/a.md":      "# A",
		"docs/img/f.png": "PNG",
	})
	res := mustCrawl(t, CrawlOptions{
		Entries:   []string{filepath.Join(root, "docs")},
		OutDir:    filepath.Join(root, "site"),
		Depth:     -1,
		NoMdLinks: true,
	})
	if len(res.Docs) != 2 {
		t.Fatalf("got %d docs, want 2 (NoMdLinks must not stop discovery)", len(res.Docs))
	}
	idx := docByBase(res.Docs, "index.md")
	if got, ok := idx.LinkMap["./a.md"]; ok {
		t.Errorf("doc link rewritten to %q despite NoMdLinks", got)
	}
	if _, ok := idx.LinkMap["./img/f.png"]; !ok {
		t.Error("NoMdLinks suppressed asset rewriting too; the flags are independent")
	}
}
