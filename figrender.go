package md2html

import (
	"bytes"
	gohtml "html"
	"strings"

	"github.com/yuin/goldmark"
	goldhtml "github.com/yuin/goldmark/renderer/html"
)

// figRenderer emits a figure's markup. It owns one inline-Markdown parser
// for the whole figure: per field would rebuild it dozens of times for one
// figure, and a package-level one would raise a question about sharing a
// parser across the CLI's parallel document conversions that a per-figure
// instance simply does not have.
type figRenderer struct{ md goldmark.Markdown }

func newFigRenderer() *figRenderer {
	// No extensions: a figure label wants CommonMark inline syntax — code
	// spans, emphasis, links — not tables or footnotes. Unsafe matches the
	// main parser, so raw HTML in a label behaves as it does in prose.
	return &figRenderer{md: goldmark.New(
		goldmark.WithRendererOptions(goldhtml.WithUnsafe()),
	)}
}

// inline renders one author-facing text field as inline Markdown and strips
// the block wrapper goldmark puts around it.
//
// A field is one line by construction, so exactly one <p> comes back. If
// conversion somehow fails, the text is escaped and passed through: a
// visible label beats a dropped one.
func (r *figRenderer) inline(s string) string {
	var buf bytes.Buffer
	if err := r.md.Convert([]byte(s), &buf); err != nil {
		return gohtml.EscapeString(s)
	}
	out := strings.TrimSpace(buf.String())
	out = strings.TrimPrefix(out, "<p>")
	out = strings.TrimSuffix(out, "</p>")
	return out
}

// item renders one item. Kinds arrive here already validated, so an item
// that matches nothing renders as nothing rather than as a diagnostic.
func (r *figRenderer) item(it figItem) string {
	switch {
	case it.Box != nil:
		return `<div class="fig-box">` + r.inline(*it.Box) + `</div>`
	}
	return ""
}
