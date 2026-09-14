package md2html

import (
	"bytes"
	"fmt"
	gohtml "html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	goldhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// figRenderer emits a figure's markup. It owns one inline-Markdown parser
// for the whole figure: per field would rebuild it dozens of times for one
// figure, and a package-level one would raise a question about sharing a
// parser across the CLI's parallel document conversions that a per-figure
// instance simply does not have.
type figRenderer struct{ md goldmark.Markdown }

func newFigRenderer() *figRenderer {
	// The parser is cut down to *one* block parser — paragraphs — plus the
	// full default inline set. That is what makes the pass genuinely inline
	// rather than merely inline-looking.
	//
	// goldmark.New with its defaults is a whole-document parser: it would
	// read "1. Validate" in a box label as an ordered list, "# Step" as a
	// heading, and four leading spaces as an indented code block. A heading
	// is the dangerous one, because it does not stay inside the figure — it
	// competes for anchor ids, joins [[toc]], and enters the namespace §
	// cross-references resolve against, all from text an author wrote as a
	// label. With only the paragraph parser registered, every block
	// construct degrades to the literal characters the author typed, which
	// is exactly what a label promises.
	//
	// No extensions either: a label wants CommonMark inline syntax — code
	// spans, emphasis, links — not tables or footnotes. Unsafe matches the
	// main parser, so raw HTML in a label behaves as it does in prose.
	return &figRenderer{md: goldmark.New(
		goldmark.WithParser(parser.NewParser(
			parser.WithBlockParsers(util.Prioritized(parser.NewParagraphParser(), 1000)),
			parser.WithInlineParsers(parser.DefaultInlineParsers()...),
			parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
		)),
		goldmark.WithRendererOptions(goldhtml.WithUnsafe()),
	)}
}

// figNewlines collapses any run of newlines (and the whitespace around it)
// to a single space.
var figNewlines = regexp.MustCompile(`[ \t]*\r?\n[ \t\r\n]*`)

// inline renders one author-facing text field as inline Markdown and strips
// the single <p> wrapper the paragraph parser leaves around it.
//
// Stripping exactly one <p>/</p> pair is only sound because the parser
// built in newFigRenderer can emit exactly one block, a paragraph. Two
// things are done to the field first to guarantee it really is one:
//
//   - Newline runs collapse to a single space. A YAML block scalar is a
//     legitimate way to write a long label, and a blank line inside one
//     would otherwise close the paragraph and open a second — leaving
//     unbalanced markup ("a</p>\n<p>b") after the strip.
//   - The result is trimmed. Leading whitespace is not merely cosmetic
//     here: goldmark trims it from the paragraph's own output, so a
//     four-space-indented label would leave the "<p>" prefix unstripped
//     without the trim.
//
// If conversion somehow fails, the text is escaped and passed through: a
// visible label beats a dropped one.
func (r *figRenderer) inline(s string) string {
	s = strings.TrimSpace(figNewlines.ReplaceAllString(s, " "))
	var buf bytes.Buffer
	if err := r.md.Convert([]byte(s), &buf); err != nil {
		return gohtml.EscapeString(s)
	}
	out := strings.TrimSpace(buf.String())
	out = strings.TrimPrefix(out, "<p>")
	out = strings.TrimSuffix(out, "</p>")
	return out
}

// leaf renders one bordered leaf kind: its class, the accent flag, the
// label, and an optional secondary note.
//
// A leaf carrying neither modifier emits exactly what it emitted before
// they existed — the class string is untouched and no span is appended —
// which is what keeps this feature additive for every figure already
// written.
func (r *figRenderer) leaf(class string, it figItem, label string) string {
	if it.Accent {
		class += " fig-accent"
	}
	s := `<div class="` + class + `">` + r.inline(label)
	if it.Note != "" {
		s += `<span class="fig-note">` + r.inline(it.Note) + `</span>`
	}
	return s + `</div>`
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
		return r.leaf("fig-box", it, *it.Box)
	case it.Arrow != nil:
		if *it.Arrow == "" {
			// Decorative: the boxes either side carry the meaning.
			return `<div class="fig-arrow" aria-hidden="true"></div>`
		}
		return `<div class="fig-arrow">` + r.inline(*it.Arrow) + `</div>`
	case it.Result != nil:
		return r.leaf("fig-result", it, *it.Result)
	case it.Rail != nil:
		return r.leaf("fig-rail", it, *it.Rail)
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

// figWeight clamps a panel weight to 1-12. A weight is a ratio, not a
// pixel count, and a figure with a panel 99 times wider than its neighbor
// is a typo rather than a layout.
func figWeight(n int) int {
	switch {
	case n < 1:
		return 1
	case n > 12:
		return 12
	}
	return n
}

// panel wraps one top-level item of a cols or split layout. The weight
// rides on a custom property rather than a raw flex-grow so a caller's
// --css replacement can reinterpret it instead of being overridden by an
// inline style it cannot reach.
func (r *figRenderer) panel(it figItem) string {
	return fmt.Sprintf(`<div class="fig-panel" style="--fig-weight:%d">%s</div>`,
		figWeight(it.Weight), r.item(it))
}
