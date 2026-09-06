package md2html

import (
	"path/filepath"
	"testing"
)

func kindsOf(ls []Link) map[string]LinkKind {
	m := map[string]LinkKind{}
	for _, l := range ls {
		m[l.Href] = l.Kind
	}
	return m
}

func TestExtractLinksClassifies(t *testing.T) {
	in := `<a href="./guide.md">d</a>` +
		`<a href="../other/api.markdown">d2</a>` +
		`<img src="./img/flow.png">` +
		`<a href="https://example.com">r</a>` +
		`<a href="#section">f</a>` +
		`<a href="data:text/plain,hi">r2</a>`
	root, _ := parseFragment([]byte(in))
	got := kindsOf(ExtractLinks(root, "/docs/api"))

	want := map[string]LinkKind{
		"./guide.md":            LinkDoc,
		"../other/api.markdown": LinkDoc,
		"./img/flow.png":        LinkAsset,
		"https://example.com":   LinkRemote,
		"#section":              LinkFragment,
		"data:text/plain,hi":    LinkRemote,
	}
	for href, wantKind := range want {
		if got[href] != wantKind {
			t.Errorf("%s: got kind %d, want %d", href, got[href], wantKind)
		}
	}
}

func TestExtractLinksResolvesAbsolutePaths(t *testing.T) {
	root, _ := parseFragment([]byte(`<a href="../shared/x.md">x</a>`))
	ls := ExtractLinks(root, "/docs/api")
	if len(ls) != 1 {
		t.Fatalf("got %d links, want 1", len(ls))
	}
	want := filepath.Clean("/docs/shared/x.md")
	if ls[0].Abs != want {
		t.Errorf("got %q, want %q", ls[0].Abs, want)
	}
}

// Fragments on a document link belong to the document, not to a separate
// file: ./guide.md#setup resolves to ./guide.md.
func TestExtractLinksStripsFragmentFromDocPath(t *testing.T) {
	root, _ := parseFragment([]byte(`<a href="./guide.md#setup">g</a>`))
	ls := ExtractLinks(root, "/docs")
	if len(ls) != 1 || ls[0].Kind != LinkDoc {
		t.Fatalf("got %+v", ls)
	}
	if ls[0].Abs != filepath.Clean("/docs/guide.md") {
		t.Errorf("got %q, want /docs/guide.md", ls[0].Abs)
	}
}

func TestExtractLinksFindsRawHTMLLinks(t *testing.T) {
	// A link inside hand-written raw HTML must be found, exactly like a
	// generated one. This is what the DOM reparse buys.
	root, _ := parseFragment([]byte(`<figure><img src="./raw.png"></figure>`))
	ls := ExtractLinks(root, "/docs")
	if len(ls) != 1 || ls[0].Kind != LinkAsset {
		t.Fatalf("raw HTML asset not found: %+v", ls)
	}
	if ls[0].Attr != "src" {
		t.Errorf("got attr %q, want src", ls[0].Attr)
	}
}

func TestIsMarkdownPath(t *testing.T) {
	cases := map[string]bool{
		"a.md": true, "a.markdown": true, "a.MD": true,
		"a.png": false, "a.html": false, "a": false,
	}
	for p, want := range cases {
		if got := IsMarkdownPath(p); got != want {
			t.Errorf("%s: got %v, want %v", p, got, want)
		}
	}
}
