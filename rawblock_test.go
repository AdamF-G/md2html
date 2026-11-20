package md2html

import (
	"strings"
	"testing"
)

// A span's or a link's block reaches Chips and LinkAttrs as the author
// typed it: no Markdown or HTML parsed inside it, and \" escaping a quote.
func TestRawBlocks(t *testing.T) {
	cases := []struct{ src, want, not string }{
		{"x [b]{.chip tip=\"a *b* c\"} z\n", `role="tooltip" aria-hidden="true">a *b* c</span>`, "<em>"},
		{"x [c]{.chip tip=\"x <b> y\"} z\n", `aria-hidden="true">x &lt;b&gt; y</span>`, "<b>"},
		{"x [d]{.chip title=\"say \\\"hi\\\" it's\"} z\n", `title="say &#34;hi&#34; it&#39;s"`, "{"},
		{"x [l](https://e.x){title=\"a *b*\"} z\n", `title="a *b*"`, "<em>"},
		{"x [m]{title=\"a &amp; b\"} z\n", `title="a &amp; b"`, "&amp;amp;"},
		{"## T {title=\"a &amp; b\"}\n", `title="a &amp; b"`, "&amp;amp;"},
		{"`[x]{.chip}`\n", `<code>[x]{.chip}</code>`, "<span"},
		{"x [y]{title=\"a\n", `[y]{title=`, "<span"},
		{"f(x){*a*}\n", `f(x){*a*}`, "<em>"},
		{"a {*b*} c\n", `{<em>b</em>}`, ""},
	}
	for _, c := range cases {
		var w []string
		got := convert(t, c.src, func(m string) { w = append(w, m) })
		if !strings.Contains(got, c.want) || (c.not != "" && strings.Contains(got, c.not)) {
			t.Errorf("%q:\n got %s\nwant %s, without %q", c.src, got, c.want, c.not)
		}
		if strings.Contains(c.src, `\"hi`) && len(w) != 0 {
			t.Errorf("%q: warnings %q, want none", c.src, w)
		}
	}
}
