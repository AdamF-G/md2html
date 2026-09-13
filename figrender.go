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

// items renders a run of items with no wrapper of its own. Every nesting
// kind shares it, which is what keeps recursion depth a non-issue: nesting
// is the same code all the way down.
func (r *figRenderer) items(list []figItem) string {
	var b strings.Builder
	for _, it := range list {
		b.WriteString(r.item(it))
	}
	return b.String()
}

// item renders one item. Kinds arrive here already validated, so an item
// that matches nothing renders as nothing rather than as a diagnostic.
func (r *figRenderer) item(it figItem) string {
	switch {
	case it.Box != nil:
		return `<div class="fig-box">` + r.inline(*it.Box) + `</div>`
	case it.Arrow != nil:
		if *it.Arrow == "" {
			// Decorative: the boxes either side carry the meaning.
			return `<div class="fig-arrow" aria-hidden="true"></div>`
		}
		return `<div class="fig-arrow">` + r.inline(*it.Arrow) + `</div>`
	case it.Result != nil:
		return `<div class="fig-result">` + r.inline(*it.Result) + `</div>`
	case it.Rail != nil:
		return `<div class="fig-rail">` + r.inline(*it.Rail) + `</div>`
	case it.Stats != nil:
		var b strings.Builder
		b.WriteString(`<div class="fig-stats">`)
		for _, s := range it.Stats {
			b.WriteString(`<div class="fig-stat"><span class="fig-stat-value">`)
			b.WriteString(r.inline(s.Value))
			b.WriteString(`</span><span class="fig-stat-label">`)
			b.WriteString(r.inline(s.Label))
			b.WriteString(`</span></div>`)
		}
		b.WriteString(`</div>`)
		return b.String()
	case it.Defs != nil:
		var b strings.Builder
		b.WriteString(`<dl class="fig-defs">`)
		for _, d := range it.Defs {
			b.WriteString(`<dt>`)
			b.WriteString(r.inline(d.Term))
			b.WriteString(`</dt><dd>`)
			b.WriteString(r.inline(d.Def))
			b.WriteString(`</dd>`)
		}
		b.WriteString(`</dl>`)
		return b.String()
	case it.Group != nil:
		return `<div class="fig-group"><div class="fig-group-title">` +
			r.inline(*it.Group) + `</div>` + r.items(it.Items) + `</div>`
	case it.Chain != nil:
		return `<div class="fig-chain">` + r.items(it.Chain) + `</div>`
	case it.Lanes != nil:
		var b strings.Builder
		b.WriteString(`<div class="fig-lanes">`)
		for _, lane := range it.Lanes {
			b.WriteString(`<div class="fig-lane">`)
			b.WriteString(r.items(lane))
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
		return b.String()
	}
	return ""
}
