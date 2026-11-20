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

// The rest hold the same idea for text that is not an attribute value but
// is still the author's to write: a chip's label, a container's title, a
// code block's caption and a fig label. Each comes out as the author's
// text, and where the construct takes inline Markdown, as exactly what the
// same text renders to in a paragraph: never more structure than that.

var textSeeds = []string{
	"since v2", "a *b* c", "x & y", "&amp;", `say "hi" it's`, "a\\b", "ü 中文 😀",
	"[c:x]", "`code`", "§1", "a  b", "50%", "a: b", "a # b", "@x", "!x", "|x|",
}

// asParagraph renders text inside a paragraph and returns that
// paragraph's rendering of it. A word goes first: alone, "0)" or "- x"
// would start a list, not a paragraph. ok is false if it still is not one.
func asParagraph(t *testing.T, text string) (inner, plain string, markup bool, ok bool) {
	t.Helper()
	p := first(fuzzMain(t, "w "+text+"\n"), "p")
	if p == nil {
		return "", "", false, false
	}
	inner, okInner := strings.CutPrefix(innerHTML(t, p), "w ")
	plain, okPlain := strings.CutPrefix(textOf(p), "w ")
	return inner, plain, len(elements(p)) > 0, okInner && okPlain
}

// innerHTML renders n's children.
func innerHTML(t *testing.T, n *html.Node) string {
	t.Helper()
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&b, c); err != nil {
			t.Fatal(err)
		}
	}
	return b.String()
}

// plainText reports whether s is text with no structure-bearing byte and
// no edge spaces a construct may trim.
func plainLine(s string) bool {
	return fuzzable(s) && s == strings.TrimSpace(s) && s != "" && !strings.ContainsAny(s, "\t\f\v")
}

// A chip's label is inline Markdown, so the chip's contents render as the
// same text does between brackets in a paragraph: no more structure, and no
// less. Between brackets, not bare: goldmark and Pandoc both read [**0!*]
// as written, where bare **0!* is *<em>0!</em>. Raw HTML
// is the author's markup anywhere, so "<" is left out, and brackets and
// braces, whose pairing is a span's own syntax, are left to its tests.
func FuzzChipLabel(f *testing.F) {
	for _, s := range textSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, label string) {
		if !plainLine(label) || strings.ContainsAny(label, "[]{}<") {
			t.Skip()
		}
		// A backslash is written doubled, in the label and in the
		// paragraph alike: left single at the end, it escapes the "]".
		label = strings.ReplaceAll(label, `\`, `\\`)
		bracketed, _, _, ok := asParagraph(t, "["+label+"] z")
		want, inBrackets := strings.CutPrefix(bracketed, "[")
		if want, inBrackets = strings.CutSuffix(want, "] z"); !ok || !inBrackets {
			t.Skip()
		}
		main := fuzzMain(t, "x ["+label+"]{.chip} y\n")
		var chip *html.Node
		walk(main, func(n *html.Node) {
			if chip == nil && hasClass(n, "chip") {
				chip = n
			}
		})
		if chip == nil {
			t.Fatalf("[%s]{.chip}: no chip in %q", label, elements(main))
		}
		if got := innerHTML(t, chip); got != want {
			t.Fatalf("[%s]{.chip}: chip renders %q, a paragraph %q", label, got, want)
		}
	})
}

// A container's title renders as the same text does in a paragraph, and
// the container's body is untouched. Raw HTML is the author's markup,
// passed through anywhere, so a title holding "<" is left out, as is a
// trailing {…}, which is the container's attribute block.
func FuzzContainerTitle(f *testing.F) {
	for _, s := range textSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, title string) {
		if !plainLine(title) || strings.ContainsAny(title, "<{}") {
			t.Skip()
		}
		want, _, _, ok := asParagraph(t, title)
		if !ok {
			t.Skip()
		}
		main := fuzzMain(t, "::: warning "+title+"\nbody\n:::\n")
		var got *html.Node
		walk(main, func(n *html.Node) {
			if got == nil && hasClass(n, "container-title") {
				got = n
			}
		})
		if got == nil {
			t.Fatalf("%q: no container title in %q", title, elements(main))
		}
		if a := innerHTML(t, got); a != want {
			t.Fatalf("%q: title renders %q, a paragraph %q", title, a, want)
		}
		var paras []string
		for c := got.Parent.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode {
				paras = append(paras, c.Data+":"+textOf(c))
			}
		}
		if len(paras) != 2 || paras[1] != "p:body" {
			t.Fatalf("%q: the container holds %q, want its title and the body", title, paras)
		}
	})
}

// A code block's caption is plain text: any value written double-quoted
// in the braced form comes back exactly. A backtick is left out: it ends
// the fence's info string (silent trap 10).
//
// It found that a caption did not decode a character reference, so
// caption="a &amp; b" showed "&amp;", where every other value means "&".
func FuzzCodeCaption(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, v string) {
		if !fuzzable(v) || strings.ContainsRune(v, '`') || v == "" {
			t.Skip()
		}
		// "Plain text apart from status badges and § references", the
		// guide says, so those are left to their own tests.
		if chipRe.MatchString(v) || strings.Contains(v, "§") {
			t.Skip()
		}
		main := fuzzMain(t, "```{.go caption="+quoteValue(v)+"}\nx\n```\n")
		cap := first(main, "figcaption")
		if cap == nil || textOf(cap) != v {
			t.Fatalf("caption %q: got %q (elements %q)", v, textOf(cap), elements(main))
		}
	})
}

// A fig label takes inline Markdown when YAML reads it as a string, so,
// double-quoted for YAML, it renders as the same text does in a paragraph.
func FuzzFigLabel(f *testing.F) {
	for _, s := range textSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, label string) {
		// A fig label takes CommonMark inline syntax, not GFM's: no
		// strikethrough and no bare autolinks (figrender.go), which a
		// paragraph has.
		if !plainLine(label) || strings.ContainsAny(label, "<~@") || strings.Contains(label, "www.") || strings.Contains(label, "://") {
			t.Skip()
		}
		for _, r := range label {
			if r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == 0x2028 || r == 0x2029 || r == 0xfeff {
				t.Skip() // characters YAML will not take in a string
			}
		}
		yaml := `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(label) + `"`
		main := fuzzMain(t, "```fig\nitems:\n  - box: "+yaml+"\n```\n")
		var box *html.Node
		walk(main, func(n *html.Node) {
			if box == nil && hasClass(n, "fig-box") {
				box = n
			}
		})
		if box == nil {
			t.Fatalf("%q: no fig box in %q", label, elements(main))
		}
		want, _, _, ok := asParagraph(t, label)
		if !ok {
			t.Skip()
		}
		if a := innerHTML(t, box); a != want {
			t.Fatalf("%q: box renders %q, a paragraph %q", label, a, want)
		}
	})
}
