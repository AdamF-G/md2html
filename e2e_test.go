package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A realistic tree exercising every feature together.
func TestEndToEndRealisticTree(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md": "# Handbook\n\n" +
			"[API](./api/auth.md) and [missing](./nope.md)\n\n" +
			"![diagram](./img/arch.png)\n\n" +
			"| A | B |\n|---|---|\n| 1 | 2 |\n",
		"docs/api/auth.md": "# Auth\n\n[home](../index.md)\n\n" +
			"```mermaid\ngraph LR\n  A --> B\n```\n",
		"docs/img/arch.png": "PNG",
	})
	site := filepath.Join(root, "site")

	res, err := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs")},
		OutDir:  site,
		Depth:   -1,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(res.Docs) != 2 {
		t.Fatalf("got %d docs, want 2", len(res.Docs))
	}
	if len(res.Warnings) == 0 {
		t.Error("expected a warning for ./nope.md")
	}

	for _, d := range res.Docs {
		src, err := os.ReadFile(d.Src)
		if err != nil {
			t.Fatal(err)
		}
		ts := append(Builtins(), LinkRewrite(d.LinkMap))
		out, err := Convert(src, Options{SourcePath: d.Src, Transforms: ts})
		if err != nil {
			t.Fatalf("Convert %s: %v", d.Src, err)
		}
		if _, err := SafeWrite(d.Out, out); err != nil {
			t.Fatalf("SafeWrite %s: %v", d.Out, err)
		}
	}

	idx, err := os.ReadFile(filepath.Join(site, "index.html"))
	if err != nil {
		t.Fatalf("index.html: %v", err)
	}
	s := string(idx)

	if !strings.HasPrefix(s, MarkerPrefix) {
		t.Errorf("no provenance marker: %.80s", s)
	}
	if !strings.Contains(s, `href="api/auth.html"`) {
		t.Errorf("doc link not rewritten into the site: %s", s)
	}
	if !strings.Contains(s, `href="./nope.md"`) {
		t.Errorf("unresolvable link should be left as written: %s", s)
	}
	if !strings.Contains(s, `src="../docs/img/arch.png"`) {
		t.Errorf("asset link should climb out of the site dir: %s", s)
	}
	if !strings.Contains(s, `<div class="table-scroll">`) {
		t.Errorf("table not wrapped: %s", s)
	}
	if !strings.Contains(s, `<title>Handbook</title>`) {
		t.Errorf("title not derived: %s", s)
	}

	auth, _ := os.ReadFile(filepath.Join(site, "api", "auth.html"))
	a := string(auth)
	if !strings.Contains(a, `<pre class="mermaid">`) {
		t.Errorf("mermaid not preserved: %s", a)
	}
	if !strings.Contains(a, `href="../index.html"`) {
		t.Errorf("upward doc link not rewritten: %s", a)
	}
}
