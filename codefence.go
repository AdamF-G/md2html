package md2html

import (
	gohtml "html"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	goldhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// splitFenceInfo separates a fence info string into its language and an
// optional caption:
//
//	```go caption="cmd/md2html/main.go"
//
// The caption is one more space-separated attribute alongside the language,
// not a new fence syntax — info strings already carry attributes, and
// anything this tool does not recognize keeps being ignored exactly as
// goldmark ignores it today. A quoted value may contain spaces.
//
// The caption is stripped out first, and the language is taken from the
// first token that remains — not from token index 0 — so that
// `caption="x.go" go` still yields the language: a caption may precede the
// language in the info string, and the language must not be lost just
// because it wasn't first.
func splitFenceInfo(info string) (lang, caption string) {
	for _, tok := range fenceTokens(info) {
		if v, ok := strings.CutPrefix(tok, "caption="); ok {
			caption = strings.Trim(v, `"'`)
			continue
		}
		if lang == "" {
			lang = tok
		}
	}
	return lang, caption
}

// fenceTokens splits an info string on whitespace, keeping a quoted run
// together so a caption may contain spaces.
//
// There is no backslash-escape for a quote embedded inside a quoted run
// (`caption="a \"b\" c"`): the backslash and the inner quote just end up
// as literal characters in the token, and strings.Trim in splitFenceInfo
// only trims a leading/trailing quote, so the embedded one survives into
// the caption text. That degrades to literal, HTML-escaped source text
// rather than broken markup or a truncated caption, which is an acceptable
// fallback for a syntax this narrow — not worth a real escaping grammar.
func fenceTokens(s string) []string {
	var out []string
	var cur strings.Builder
	var quote byte
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			cur.WriteByte(c)
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
			cur.WriteByte(c)
		case c == ' ' || c == '\t':
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}

// codeFenceRenderer replaces goldmark's fenced-code-block rendering so a
// caption in the info string becomes a visible bar above the block.
//
// This has to happen at the renderer rather than as a tree transform:
// goldmark takes the first word of the info string as the language and
// discards the rest without a word, so by the time an HTML tree exists the
// caption is gone. That also means today's failure is silent — a fence
// written with a caption renders byte-for-byte like one without.
//
// Mermaid is unaffected: its extension rewrites its fences into its own AST
// node during parsing and registers a renderer for that node, so a mermaid
// fence never reaches this function.
//
// A trailing {...} attribute (e.g. `go {.wide}`) is left exactly as
// unhandled as it is in goldmark's own fenced-code-block renderer: that
// renderer never calls n.Attributes() either (CodeAttributeFilter is wired
// up for inline code spans, not fenced blocks), so there is no existing
// behavior here to preserve beyond "still does nothing with it".
type codeFenceRenderer struct{ warn func(string) }

func newCodeFenceRenderer(warn func(string)) renderer.NodeRenderer {
	if warn == nil {
		warn = func(string) {}
	}
	return &codeFenceRenderer{warn: warn}
}

func (r *codeFenceRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.render)
}

func (r *codeFenceRenderer) render(w util.BufWriter, source []byte, node ast.Node,
	entering bool) (ast.WalkStatus, error) {

	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.FencedCodeBlock)

	var info string
	if n.Info != nil {
		info = string(n.Info.Segment.Value(source))
	}
	lang, caption := splitFenceInfo(info)

	// A `fig` fence is a figure, not code. renderFig either returns markup
	// or declines, in which case the body falls through to the ordinary
	// code-block path below and the reader sees their own source.
	if lang == "fig" {
		// An info-string caption is code-fence vocabulary; a figure's
		// caption is a `caption:` key in its body. It is not quietly
		// adopted as a fallback, because one thing having two spellings is
		// what the single-vocabulary rule exists to prevent — but author
		// text must never be dropped in silence either, so say so and name
		// the spelling that works.
		//
		// This warning is deliberately unconditional rather than raised
		// only when the figure renders. The sentence is true either way —
		// a caption in the info string is not part of the fence language —
		// and an author whose body also fails to decode is otherwise told
		// about the caption only later, once they have fixed the body and
		// watched it vanish. Two mistakes here mean two warnings; the
		// one-warning-per-fault invariant is per fault, not per fence.
		if caption != "" {
			r.warn(`fig fence: a caption in the info string is not part of a figure; ` +
				`use a caption: key in the fence body instead`)
		}
		var body strings.Builder
		lines := n.Lines()
		for i := 0; i < lines.Len(); i++ {
			line := lines.At(i)
			body.Write(line.Value(source))
		}
		if markup, ok := renderFig([]byte(body.String()), r.warn); ok {
			w.WriteString(markup)
			w.WriteByte('\n')
			return ast.WalkSkipChildren, nil
		}
	}

	if caption != "" {
		w.WriteString(`<figure class="code-figure"><figcaption>`)
		// The caption is author text, HTML-escaped here because it is
		// written into markup this function builds by hand rather than
		// through goldmark's escaping writer. Escaping only guards against
		// literal markup injection: parseFragment re-parses this
		// <figcaption> like everything else in the tree, so its text nodes
		// are ordinary prose by the time Chips and SectionLinks walk the
		// document — a caption's "[proven]" or "§2.1" resolves the same
		// way it would in body text, not as literal escaped brackets.
		w.WriteString(gohtml.EscapeString(caption))
		w.WriteString("</figcaption>")
	}
	w.WriteString("<pre><code")
	if lang != "" {
		w.WriteString(` class="language-`)
		w.Write(util.EscapeHTML([]byte(lang)))
		w.WriteString(`"`)
	}
	w.WriteByte('>')
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		goldhtml.DefaultWriter.RawWrite(w, line.Value(source))
	}
	w.WriteString("</code></pre>")
	if caption != "" {
		w.WriteString("</figure>")
	}
	w.WriteByte('\n')

	// A fenced code block's content is raw lines, not child nodes; skipping
	// children matches what goldmark's own renderer does with it.
	return ast.WalkSkipChildren, nil
}
