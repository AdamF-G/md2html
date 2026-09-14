package md2html

import (
	gohtml "html"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	goldhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// fenceInfo is a parsed fenced CODE block info string — the text after the
// backticks. A container's fence line is a different grammar in a different
// file: see containerInfo and parseContainerInfo in containerinfo.go. The two
// share only the attribute block, through parseAttrs.
type fenceInfo struct {
	lang    string
	caption string
	id      string
	// classes holds every class from a braced block except the one taken
	// as the language, so an author's own styling hook reaches the element
	// instead of being swallowed.
	classes []string
}

// parseCodeFenceInfo reads a fence info string in either supported form:
//
//	```go caption="server.go"        the original, space-separated
//	```{.go caption="server.go"}     Pandoc's fenced_code_attributes
//	```go {caption="server.go"}      a head word plus a braced block
//
// The braced form is the standard one and the reason this exists: it is
// what a document written for Pandoc, kramdown or MyST will use, and
// md2html used to mangle it into class="language-{.go" with a caption of
// `server.go"}`. Both forms are accepted rather than one replacing the
// other, so nothing written for the older spelling changes meaning.
//
// In the braced form the language is the first class, exactly as Pandoc
// reads it. A head word outside the braces wins over that, because it is
// where a reader looks first: `go {.wide}` is Go with a "wide" class, not
// a "go"-classed block in the "wide" language.
func parseCodeFenceInfo(info string) fenceInfo {
	var f fenceInfo
	head, content, braced := splitBraced(info)
	if !braced {
		f.lang, f.caption = splitPlainFenceInfo(info)
		return f
	}
	f.lang, f.caption = splitPlainFenceInfo(head)
	a, ok := parseAttrs(content)
	if !ok {
		return f
	}
	f.id = a.id
	if c, present := a.kv["caption"]; present {
		f.caption = c
	}
	f.classes = a.classes
	if f.lang == "" && len(f.classes) > 0 {
		f.lang, f.classes = f.classes[0], f.classes[1:]
	}
	return f
}

// splitFenceInfo reports just the language and caption, the two things
// most callers and tests care about.
func splitFenceInfo(info string) (lang, caption string) {
	f := parseCodeFenceInfo(info)
	return f.lang, f.caption
}

// splitPlainFenceInfo separates the brace-free form into its language and
// an optional caption:
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
func splitPlainFenceInfo(info string) (lang, caption string) {
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
// (`caption="a \"b\" c"`) in this brace-free form: the backslash and the
// inner quote end up as literal characters in the token, and strings.Trim
// in splitPlainFenceInfo only trims a leading/trailing quote, so the
// embedded one survives into the caption text. That degrades to literal,
// HTML-escaped source text rather than broken markup or a truncated
// caption.
//
// The braced form does support the escape, because it goes through
// parseAttrs. An author who needs a quote inside a caption should write
// ```{.go caption="has \"quote\" inside"}.
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
// A braced {...} block is parsed here rather than through goldmark, whose
// own fenced-code-block renderer never calls n.Attributes()
// (CodeAttributeFilter is wired up for inline code spans, not fenced
// blocks). Handling it in the same place as the caption keeps one parser
// for one grammar — see parseCodeFenceInfo.
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
	f := parseCodeFenceInfo(info)
	lang, caption := f.lang, f.caption

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
	w.WriteString("<pre")
	if f.id != "" {
		// The id names the block, so it belongs on the block element
		// rather than on the <code> inside it.
		w.WriteString(` id="`)
		w.Write(util.EscapeHTML([]byte(f.id)))
		w.WriteString(`"`)
	}
	w.WriteString("><code")
	if classes := codeClasses(lang, f.classes); classes != "" {
		w.WriteString(` class="`)
		w.Write(util.EscapeHTML([]byte(classes)))
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

// codeClasses renders the <code> element's class attribute: the language
// in goldmark's language-<name> spelling, followed by whatever else the
// author's attribute block named. An extra class is a styling hook they
// wrote deliberately, so it reaches the element instead of being dropped.
func codeClasses(lang string, extra []string) string {
	var out []string
	if lang != "" {
		out = append(out, "language-"+lang)
	}
	out = append(out, extra...)
	return strings.Join(out, " ")
}
