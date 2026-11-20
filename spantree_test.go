package md2html

import (
	"strings"
	"testing"
)

// A bracketed span is paired on goldmark's tree, as Pandoc parses one: its
// label is inline Markdown, brackets nest, and \] is a bracket.
func TestSpanTree(t *testing.T) {
	cases := []struct {
		src, want, not string
		warns          int
	}{
		{"x [need *a*]{.chip} y\n", `<span class="chip">need <em>a</em></span>`, "", 0},
		{"x [**proven**]{.chip} y\n", `<span class="chip chip-proven"><strong>proven</strong></span>`, "", 0},
		{"x [a [b] c]{.x} y\n", `<span class="x">a [b] c</span>`, "", 0},
		{"x [a \\] b]{.x} y\n", `<span class="x">a ] b</span>`, "", 0},
		{"x [x\\]{.c} y\n", `[x]{.c}`, "<span", 0},
		{"x [see [x]{.chip}](https://e.x) y\n", `<a href="https://e.x"`, "<span", 0},
		{"x [b]{.chip data-flag} y\n", `[b]{.chip data-flag}`, "<span", 1},
		{"x [a]{.x}[b]{.y} y\n", `<span class="x">a</span><span class="y">b</span>`, "", 0},
		{"x [l](https://e.x) [s]{.x} y\n", `<span class="x">s</span>`, "", 0},
		{"x [b]{.c title=don't} y\n", `title="don&#39;t"`, "", 0},
		{"## R [beta *x*]{.chip} z\n", `<span class="chip">beta <em>x</em></span>`, "", 0},
		{"```fig\nitems:\n  - box: \"[a *b*]{.chip}\"\n```\n", `<span class="chip">a <em>b</em></span>`, "", 0},
	}
	for _, c := range cases {
		var w []string
		got := convert(t, c.src, func(m string) { w = append(w, m) })
		if !strings.Contains(got, c.want) || (c.not != "" && strings.Contains(got, c.not)) {
			t.Errorf("%q:\n got %s\nwant %s, without %q", c.src, got, c.want, c.not)
		}
		if len(w) != c.warns {
			t.Errorf("%q: warnings %q, want %d", c.src, w, c.warns)
		}
	}
}

// A chip and its block stay out of the title; any other span's label
// stays in it, Markdown and all reduced to its text.
func TestSpanTreeTitle(t *testing.T) {
	for src, want := range map[string]string{
		"# A [b]{.chip} c\n":                       "<title>A c</title>",
		"# A [b]{.lead} c\n":                       "<title>A b c</title>",
		"# A [*b*]{.lead} c\n":                     "<title>A b c</title>",
		"# A [b]{.c d} c\n":                        "<title>A [b]{.c d} c</title>",
		"# Rollback [beta]{.chip title=\"a b\"}\n": "<title>Rollback</title>",
		"# Rollback [x]{title=\"a } b\"}\n":        "<title>Rollback x</title>",
		"# A [x]{b} B\n":                           "<title>A [x]{b} B</title>",
		"# A [x]{} B\n":                            "<title>A [x]{} B</title>",
	} {
		out, err := Convert([]byte(src), Options{CSS: "/**/"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), want) {
			t.Errorf("%q: want %s", src, want)
		}
	}
}
