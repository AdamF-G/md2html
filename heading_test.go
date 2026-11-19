package md2html

import (
	"strings"
	"testing"

	"github.com/yuin/goldmark/parser"
)

// A heading's block is checked by the span's rules, under the heading's
// name, and a heading is no tip's host.
func TestCheckAttrBlockHeadingWho(t *testing.T) {
	var w []string
	collect := func(m string) { w = append(w, m) }
	if _, ok := checkAttrBlock(`heading "Setup"`, "#s data-flag", false, collect); ok {
		t.Error("a bare key beside an id was taken")
	}
	if len(w) != 1 || !strings.HasPrefix(w[0], `heading "Setup": data-flag has no value`) {
		t.Errorf("warnings = %q", w)
	}
	w = nil
	a, ok := checkAttrBlock(`heading "T"`, `#t tip="n" tip-delay="0ms"`, false, collect)
	if !ok || a.id != "t" || a.tip != "" || len(w) != 0 {
		t.Errorf("got %+v ok=%v warnings %q; want id t, no tip, no warning", a, ok, w)
	}
}

// A parser option set on goldmark reaches the heading parsers wrapped.
func TestHeadingParsersTakeParserOptions(t *testing.T) {
	var got parser.OptionName
	for _, p := range headingParsers(nil) {
		if o, ok := p.Value.(parser.SetOptioner); ok {
			if hp, ok := p.Value.(*headingAttrParser); ok {
				hp.BlockParser = optionSpy{BlockParser: hp.BlockParser, saw: &got}
				o.SetOption("probe", true)
			}
		}
	}
	if got != "probe" {
		t.Errorf("option not forwarded: got %q", got)
	}
}

type optionSpy struct {
	parser.BlockParser
	saw *parser.OptionName
}

func (s optionSpy) SetOption(name parser.OptionName, _ interface{}) { *s.saw = name }

// headingOut converts src and returns the first heading's element, from
// its opening tag to the anchor HeadingAnchors adds (or its close), and
// the warnings.
func headingOut(t *testing.T, src string) (string, []string) {
	t.Helper()
	var w []string
	got := convert(t, src, func(m string) { w = append(w, m) })
	i := strings.Index(got, "<h")
	if i < 0 {
		t.Fatalf("no heading in:\n%s", got)
	}
	h := got[i:]
	if j := strings.Index(h, `<a class="anchor"`); j >= 0 {
		h = h[:j]
	} else if j := strings.Index(h, "</h"); j >= 0 {
		h = h[:j]
	}
	return h, w
}

func TestHeadingBlocks(t *testing.T) {
	cases := []struct {
		src, want, not string
		warns          int
		warn           string
	}{
		{"## A {#a .k}\n", `<h2 id="a" class="k">A`, "{", 0, ""},
		{"## Rollback [beta]{.chip}\n", `<span class="chip">beta</span>`, `<h2 class`, 0, ""},
		{"## See [d](https://e.x){.x}\n", `class="x"`, `<h2 class`, 0, ""},
		{"## T {title='q'}\n", `title="q"`, "{", 0, ""},
		{"## T {lang=fr}\n", `lang="fr"`, "{", 0, ""},
		{"## T {data-step=1}\n", `data-step="1"`, "{", 0, ""},
		{"## T {aria-label=\"L\"}\n", `aria-label="L"`, "{", 0, ""},
		{"## T {#t onclick=\"x\"}\n", `id="t"`, "onclick", 0, ""},
		{"## Don't panic {#calm}\n", `id="calm">Don&#39;t panic`, "{", 0, ""},
		{"## It's 5\" tall {#x}\n", `id="x"`, "{", 0, ""},
		{"## GET /v1/jobs/{id}\n", `GET /v1/jobs/{id}`, "", 0, ""},
		{"## T {}\n", `T {}`, "", 0, ""},
		{"## Setup {#s data-flag}\n", `{#s data-flag}`, "", 1, "has no value"},
		{"## T {#t a/b=1}\n", `{#t a/b=1}`, "", 1, "is not an attribute name"},
		{"## Title ## {#id}\n", `id="id">Title`, "#", 0, ""},
		{"## Title ##\n", `>Title`, "#", 0, ""},
		{"## T {title=\"a } *b*\"}\n", `title="a } *b*"`, "<em>", 0, ""},
		{"## T {#t tip=\"n\"}\n", `id="t"`, "tooltip", 0, ""},
		{"Title [z]{.chip}\n---\n", `<span class="chip">z</span>`, `<h2 class`, 0, ""},
		{"Title {#t}\n===\n", `<h1 id="t">Title`, "{", 0, ""},
	}
	for _, c := range cases {
		h, w := headingOut(t, c.src)
		if !strings.Contains(h, c.want) || (c.not != "" && strings.Contains(h, c.not)) {
			t.Errorf("%q:\n got %s\nwant %s, without %q", c.src, h, c.want, c.not)
		}
		if len(w) != c.warns || (c.warn != "" && !strings.Contains(w[0], c.warn)) {
			t.Errorf("%q: warnings %q, want %d with %q", c.src, w, c.warns, c.warn)
		}
	}
}

// A block touching a "]" or a ")" is left for a span or a link; when
// none takes it, it stays as text and says how to give it to the heading.
func TestHeadingUnclaimedBlockWarns(t *testing.T) {
	// A code span before the block is not among them: the byte before
	// the "{" is a backtick, so the block is the heading's.
	for _, src := range []string{"## Call f(x){#call}\n", "## Note[^1]{#n}\n\n[^1]: x\n", "## Lit\\]{#x}\n"} {
		h, w := headingOut(t, src)
		if len(w) != 1 || !strings.Contains(w[0], "write a space before") {
			t.Errorf("%q: warnings %q, want one", src, w)
		}
		if !strings.Contains(h, "{#") {
			t.Errorf("%q: block left the text: %s", src, h)
		}
	}
}

func TestHeadingClaimedBlockQuiet(t *testing.T) {
	if _, w := headingOut(t, "## R [b]{.chip}\n"); len(w) != 0 {
		t.Errorf("warnings %q, want none", w)
	}
	if _, w := headingOut(t, "## See [d](https://e.x){.x}\n"); len(w) != 0 {
		t.Errorf("warnings %q, want none", w)
	}
}

// The title is read before any transform, so a link's block, which a
// heading now leaves for the link, has to be dropped there too.
func TestTitleDropsLinkBlock(t *testing.T) {
	for src, want := range map[string]string{
		"# See [docs](d.md){.x}\n":                    "<title>See docs</title>",
		"# See [docs](d.md) and [x](y.md){.k} more\n": "<title>See docs and x more</title>",
	} {
		out, err := Convert([]byte(src), Options{CSS: "/**/"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), want) {
			t.Errorf("%q: want %s in\n%s", src, want, out)
		}
	}
}

// A span or a link that refused its own block leaves it in the heading's
// text, but it was never the heading's: telling the author to give it to
// the heading would move a chip's class onto the <h2>.
func TestHeadingRefusedSpanBlockIsTheSpans(t *testing.T) {
	_, w := headingOut(t, "## Bad [beta]{.chip data-flag}\n")
	if len(w) != 1 || !strings.Contains(w[0], "has no value") {
		t.Errorf("warnings %q, want only the span's", w)
	}
}

func TestHeadingRefusedLinkBlockSaysFixIt(t *testing.T) {
	_, w := headingOut(t, "## Bad link [d](https://e.x){#i bad/name=1}\n")
	if len(w) != 1 || !strings.Contains(w[0], "after a link") || !strings.Contains(w[0], "fix the block") {
		t.Errorf("warnings %q, want one saying to fix the link's block", w)
	}
}
