package md2html

import (
	"strings"
	"testing"
)

// Pandoc's link_attributes: a {…} block written straight after a link lands
// on the <a>, and the block's text is gone from the page.
func TestLinkAttrsOnALink(t *testing.T) {
	got := convert(t, "See [Home](./home.md){#h .big .nav aria-current=page} now.\n", nil)
	if !strings.Contains(got, `<a href="./home.md" id="h" class="big nav" aria-current="page">Home</a> now.`) {
		t.Errorf("attributes not applied to the link\ngot: %s", got)
	}
	if strings.Contains(got, "{") {
		t.Errorf("attribute block left in the text\ngot: %s", got)
	}
}

// The same extension covers images, where a width is the usual reason.
func TestLinkAttrsOnAnImage(t *testing.T) {
	got := convert(t, "![A chart](chart.png){width=50% .wide}\n", nil)
	if !strings.Contains(got, `<img src="chart.png" alt="A chart" class="wide" width="50%"/>`) {
		t.Errorf("attributes not applied to the image\ngot: %s", got)
	}
}

// The case that motivated the feature: marking the current page inside a
// hand-written navigation block.
func TestLinkAttrsAriaCurrentInsideNav(t *testing.T) {
	got := convert(t, "::: nav {aria-label=\"Guide\"}\n- [Intro](./intro.md){aria-current=page}\n- [Setup](./setup.md)\n:::\n", nil)
	if !strings.Contains(got, `<a href="./intro.md" aria-current="page">Intro</a>`) {
		t.Errorf("aria-current not on the link\ngot: %s", got)
	}
}

// Text after the block stays, and so does a link's inline markup.
func TestLinkAttrsKeepsSurroundingText(t *testing.T) {
	got := convert(t, "[in **bold**](a.md){.c}text after\n", nil)
	if !strings.Contains(got, `<a href="a.md" class="c">in <strong>bold</strong></a>text after`) {
		t.Errorf("surrounding text or markup lost\ngot: %s", got)
	}
}

// Pandoc requires the block to touch the link. With a space between them
// it is prose, and so is a block holding no attributes, which stays as
// literal text the way [x]{} does for a span.
func TestLinkAttrsLeavesNonBlocksAlone(t *testing.T) {
	for _, src := range []string{
		"[spaced](a.md) {.no}\n",
		"[empty](a.md){}\n",
		"[open](a.md){.never closed\n",
		"Not a link {.c} here.\n",
	} {
		got := convert(t, src, nil)
		if strings.Contains(got, `class="no"`) || strings.Contains(got, `class="never`) || strings.Contains(got, `class="c"`) {
			t.Errorf("%q: applied a block it should not have\ngot: %s", src, got)
		}
		if !strings.Contains(got, "{") {
			t.Errorf("%q: literal text removed\ngot: %s", src, got)
		}
	}
}

// An attribute name reaches the output unescaped, so it gets the same
// whitelist a container's does. A block with any name that fails it is not
// an attribute block at all, as in Pandoc: it stays on the page as the
// text the author wrote, and nothing from it is applied. Dropping only the
// bad names would silently delete prose that happens to touch a link.
func TestLinkAttrsLeavesBlocksWithUnsafeNamesLiteral(t *testing.T) {
	for _, c := range []struct{ src, text string }{
		{"[x](a.md){x<y=1 .ok}\n", "{x&lt;y=1 .ok}"},
		{"[w](d.md){{version}}\n", "{{version}}"},
		{"[S](s.md){a, b, c}\n", "{a, b, c}"},
	} {
		got := convert(t, c.src, nil)
		if !strings.Contains(got, "</a>"+c.text) {
			t.Errorf("%q: block not left as literal text\ngot: %s", c.src, got)
		}
		if strings.Contains(got, `class="ok"`) || strings.Contains(got, `c=""`) {
			t.Errorf("%q: part of a refused block applied\ngot: %s", c.src, got)
		}
	}
}

// href and src are what link rewriting keys on, as written in the source.
// Letting a block replace them would route a link around that rewrite, so
// the block cannot, and says why.
func TestLinkAttrsRefusesHrefAndSrc(t *testing.T) {
	for _, c := range []struct{ src, attr string }{
		{"[x](a.md){href=b.md}\n", `href="a.md"`},
		{"![x](a.png){src=b.png}\n", `src="a.png"`},
	} {
		var warns []string
		got := convert(t, c.src, func(m string) { warns = append(warns, m) })
		if !strings.Contains(got, c.attr) {
			t.Errorf("%q: target overridden\ngot: %s", c.src, got)
		}
		if len(warns) != 1 || !strings.Contains(warns[0], "cannot be set") {
			t.Errorf("%q: warnings = %v", c.src, warns)
		}
	}
}

// A link carrying attributes is still rewritten from .md to .html: the
// block is never part of the href the link map is keyed on.
func TestLinkAttrsLinkStillRewritten(t *testing.T) {
	out, err := Convert([]byte("[Home](./home.md){.c}\n"), Options{Fragment: true, CSS: "/**/",
		LinkMap: map[string]string{"./home.md": "./home.html"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `<a href="./home.html" class="c">`) {
		t.Errorf("link not rewritten\ngot: %s", out)
	}
}

// ExternalLinks adds its rel tokens to an author's rather than replacing
// them, and leaves an author's target alone.
func TestLinkAttrsExternalMergesRelAndKeepsTarget(t *testing.T) {
	got := convert(t, "[me](https://e.com){rel=me target=_self}\n", nil)
	if !strings.Contains(got, `target="_self"`) || strings.Contains(got, `target="_blank"`) {
		t.Errorf("author's target not kept\ngot: %s", got)
	}
	if !strings.Contains(got, `rel="me noopener noreferrer"`) {
		t.Errorf("rel not merged\ngot: %s", got)
	}
}

// The merge is ExternalLinks' own, so it holds for raw HTML too: rel="me"
// used to be overwritten.
func TestExternalLinksMergesRawRel(t *testing.T) {
	got := apply(t, `<a href="https://e.com" rel="me noopener">x</a>`, ExternalLinks())
	if !strings.Contains(got, `rel="me noopener noreferrer"`) || !strings.Contains(got, `target="_blank"`) {
		t.Errorf("raw rel not merged\ngot: %s", got)
	}
}

// HTML attribute names are case-insensitive, so a block's names are
// lowercased: HREF is refused like href, and Title replaces the title the
// link already has instead of adding a second one the browser ignores.
func TestLinkAttrsNamesAreCaseInsensitive(t *testing.T) {
	var warns []string
	got := convert(t, "[y](b.md){HREF=evil.md} [x](u \"old\"){Title=new}\n", func(m string) { warns = append(warns, m) })
	if strings.Contains(got, "evil.md") {
		t.Errorf("HREF got past the refusal\ngot: %s", got)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "cannot be set") {
		t.Errorf("warnings = %v, want one refusal", warns)
	}
	if !strings.Contains(got, `title="new"`) || strings.Contains(got, "old") || strings.Contains(got, "Title=") {
		t.Errorf("Title did not replace title\ngot: %s", got)
	}
}
