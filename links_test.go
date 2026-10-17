package md2html

import (
	"path/filepath"
	"strings"
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

// Every candidate of an srcset, and a video's poster, is a link like a src.
// Each link says whether the page loads it or a reader follows it: only an
// <a> is followed.
func TestExtractLinksFindsSrcsetAndPosterAndMarksLoaded(t *testing.T) {
	in := `<img src="a.png" srcset="a.png 1x, ./b@2x.png 2x">` +
		`<video src="v.mp4" poster="p.jpg"></video>` +
		`<link rel="stylesheet" href="s.css">` +
		`<a href="doc.pdf">d</a>`
	root, _ := parseFragment([]byte(in))
	type got struct {
		attr   string
		loaded bool
	}
	seen := map[string]got{}
	var order []string
	for _, l := range ExtractLinks(root, "/docs") {
		seen[l.Attr+" "+l.Href] = got{l.Attr, l.Loaded}
		order = append(order, l.Attr+" "+l.Href)
	}
	want := []string{"src a.png", "srcset a.png", "srcset ./b@2x.png", "src v.mp4", "poster p.jpg", "href s.css", "href doc.pdf"}
	if strings.Join(order, "|") != strings.Join(want, "|") {
		t.Errorf("links:\n%v\nwant:\n%v", order, want)
	}
	for k, g := range seen {
		if wantLoaded := k != "href doc.pdf"; g.loaded != wantLoaded {
			t.Errorf("%s: loaded = %v, want %v", k, g.loaded, wantLoaded)
		}
	}
}

// A candidate's URL is the run of non-whitespace after the whitespace and
// commas that separate candidates, so a comma inside a URL or a data: URL
// does not split it.
func TestSrcsetURLsKeepsCommasInsideURLs(t *testing.T) {
	cases := []struct {
		in   string
		want []srcsetCandidate
	}{
		{"data:image/png;base64,AAAA 1x, b.png 2x", []srcsetCandidate{{"data:image/png;base64,AAAA", "1x"}, {"b.png", "2x"}}},
		{"https://cdn/x/w_100,h_100/a.png 1x,c.png 2x", []srcsetCandidate{{"https://cdn/x/w_100,h_100/a.png", "1x"}, {"c.png", "2x"}}},
		{"a.png, b.png", []srcsetCandidate{{"a.png", ""}, {"b.png", ""}}},
		{"a.png 1x, , b.png 2x", []srcsetCandidate{{"a.png", "1x"}, {"b.png", "2x"}}},
	}
	for _, c := range cases {
		got := srcsetURLs(c.in)
		if len(got) != len(c.want) {
			t.Errorf("srcsetURLs(%q) = %+v, want %+v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("srcsetURLs(%q)[%d] = %+v, want %+v", c.in, i, got[i], c.want[i])
			}
		}
	}
}

// A data: URL in an srcset is one candidate and therefore one remote link.
func TestExtractLinksDataURLInSrcsetIsOneRemoteLink(t *testing.T) {
	root, _ := parseFragment([]byte(`<img srcset="data:image/png;base64,AAAA 1x">`))
	ls := ExtractLinks(root, "/docs")
	if len(ls) != 1 || ls[0].Attr != "srcset" || ls[0].Kind != LinkRemote || ls[0].Href != "data:image/png;base64,AAAA" {
		t.Fatalf("got %+v", ls)
	}
}

func TestSrcsetURLsKeepsDescriptors(t *testing.T) {
	cs := srcsetURLs(" a.png 1x,b.png  2x , c.png")
	if len(cs) != 3 || cs[0].url != "a.png" || cs[0].desc != "1x" || cs[1].url != "b.png" || cs[1].desc != "2x" || cs[2].url != "c.png" || cs[2].desc != "" {
		t.Fatalf("got %+v", cs)
	}
	if got := joinSrcset(cs); got != "a.png 1x, b.png 2x, c.png" {
		t.Errorf("joined %q", got)
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
