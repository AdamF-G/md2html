package md2html

import (
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// These fuzz targets hold one property for every attribute value an author
// writes: whatever string goes in comes back out, exactly, and it never
// becomes markup. They were written after example tests with plain values
// missed that a span's tip holding *b* or <b>, or a \" in a value, broke the
// span: nothing in the examples tried them, and a fuzzer tries everything.
//
// go test runs the seeds; go test -fuzz=FuzzSpanTitle (one target at a
// time) explores further.

var fuzzSeeds = []string{
	"May change before 1.0.", "a *b* c", "x <b> y", `say "hi" it's`, `a\b`,
	"a } b", "a { b", "&amp;", "&", "[x]{.chip}", "`code`", "a\tb", "",
	" lead and trail ", "ü 中文 😀", "](u){#x}", `\"`, "<!-- c -->", "<script>",
}

// quoteValue writes v as a double-quoted attribute value md2html reads
// back as v: a backslash and a quote escaped, and "&" as a character
// reference, since a value's references are decoded.
func quoteValue(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "&", "&amp;")
	return `"` + r.Replace(v) + `"`
}

// fuzzable reports whether v can sit in a one-line block at all: a block
// ends at its line, and goldmark and the HTML parser replace NUL and
// normalise carriage returns, so neither can round-trip by any means.
func fuzzable(v string) bool {
	return utf8.ValidString(v) && !strings.ContainsAny(v, "\n\r\x00")
}

// fuzzMain converts src as a fragment and returns its <main>, failing the
// test on any error.
func fuzzMain(t *testing.T, src string) *html.Node {
	t.Helper()
	out, err := Convert([]byte(src), Options{Fragment: true, CSS: "/**/"})
	if err != nil {
		t.Fatalf("Convert(%q): %v", src, err)
	}
	doc, err := html.Parse(strings.NewReader(string(out)))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var main *html.Node
	walk(doc, func(n *html.Node) {
		if main == nil && n.Type == html.ElementNode && n.DataAtom == atom.Main {
			main = n
		}
	})
	if main == nil {
		t.Fatalf("no <main> in %s", out)
	}
	return main
}

// elements lists the tags of every element under root, in order.
func elements(root *html.Node) []string {
	var tags []string
	walk(root, func(n *html.Node) {
		if n != root && n.Type == html.ElementNode {
			tags = append(tags, n.Data)
		}
	})
	return tags
}

// first returns the first element under root with the given tag.
func first(root *html.Node, tag string) *html.Node {
	var found *html.Node
	walk(root, func(n *html.Node) {
		if found == nil && n.Type == html.ElementNode && n.Data == tag {
			found = n
		}
	})
	return found
}

func checkTitle(t *testing.T, src, v, tag string, want []string) {
	t.Helper()
	main := fuzzMain(t, src)
	if got := strings.Join(elements(main), " "); got != strings.Join(want, " ") {
		t.Fatalf("%q: elements %q, want %q", src, got, strings.Join(want, " "))
	}
	if got, _ := attr(first(main, tag), "title"); got != v {
		t.Fatalf("%q: title %q, want %q", src, got, v)
	}
}

func FuzzSpanTitle(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, v string) {
		if !fuzzable(v) {
			t.Skip()
		}
		checkTitle(t, "x [s]{title="+quoteValue(v)+"} y\n", v, "span", []string{"p", "span"})
	})
}

func FuzzLinkTitle(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, v string) {
		if !fuzzable(v) {
			t.Skip()
		}
		checkTitle(t, "x [s](https://e.x){title="+quoteValue(v)+"} y\n", v, "a", []string{"p", "a"})
	})
}

func FuzzHeadingTitle(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, v string) {
		if !fuzzable(v) {
			t.Skip()
		}
		// HeadingAnchors adds the "#" anchor link.
		checkTitle(t, "## T {title="+quoteValue(v)+"}\n", v, "h2", []string{"h2", "a"})
	})
}

// A tip's note is text in the page, so it must come out as that text in
// its tooltip, and never as markup.
func FuzzTipNote(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, v string) {
		if !fuzzable(v) || v == "" {
			t.Skip() // an empty tip makes no tip, by design
		}
		src := "x [s]{tip=" + quoteValue(v) + "} y\n"
		main := fuzzMain(t, src)
		if got := strings.Join(elements(main), " "); got != "p span span span" {
			t.Fatalf("%q: elements %q, want a wrap, a host and a tooltip", src, got)
		}
		var tip *html.Node
		walk(main, func(n *html.Node) {
			if tip == nil && hasClass(n, "md2html-tooltip") {
				tip = n
			}
		})
		if tip == nil || textOf(tip) != v {
			t.Fatalf("%q: tooltip text %q, want %q", src, textOf(tip), v)
		}
	})
}
