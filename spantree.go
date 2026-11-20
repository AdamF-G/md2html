package md2html

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// spanMarker is the attribute a paired span carries from goldmark to Chips:
// its {…} block, as the author typed it. Chips reads it through
// checkAttrBlock and replaces it with the attributes it means, or unwraps
// the span back to its literal text. No attribute block can set it, since
// it has no data- or aria- prefix.
const spanMarker = "md2html-span"

// rawBlock is a {…} block rawBlockParser took straight after a "]" or a
// ")": the author's characters, which goldmark parses nothing inside. One
// after a "]" that opens a bracketed span becomes that span's block
// (spanTransformer); any other is written back as the text it is.
type rawBlock struct {
	ast.BaseInline
	value []byte
	// after is the byte before the block in the source: ']' or ')'.
	after byte
}

var kindRawBlock = ast.NewNodeKind("MD2HTMLRawBlock")

func (n *rawBlock) Kind() ast.NodeKind { return kindRawBlock }

func (n *rawBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Value": string(n.value)}, nil)
}

// bracketSpan is a bracketed span: its children are its label, parsed as
// inline Markdown, and block is its {…}.
type bracketSpan struct {
	ast.BaseInline
	block []byte
}

var kindSpan = ast.NewNodeKind("MD2HTMLSpan")

func (n *bracketSpan) Kind() ast.NodeKind { return kindSpan }

func (n *bracketSpan) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Block": string(n.block)}, nil)
}

// spanBlockEnd returns the index of the "}" that closes the block opening
// at s[0], or -1. It reads a block as chipRe once did: a quote opens a
// value only straight after "=", so the apostrophe in title=don't or in
// [x]{it's .c} is an ordinary character; and a block whose quote never
// closes ends at its first "}", so checkAttrBlock can say what is wrong
// with it rather than the text staying with no word.
func spanBlockEnd(s string) int {
	if s == "" || s[0] != '{' {
		return -1
	}
	for i := 1; i < len(s); i++ {
		switch {
		case s[i] == '\\':
			i++
		case s[i] == '=' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\''):
			q := s[i+1]
			j := i + 2
			for j < len(s) && s[j] != q {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(s) {
				return firstBrace(s)
			}
			i = j
		case s[i] == '}':
			return i
		}
	}
	return -1
}

// firstBrace is the index of the first "}" in s, or -1.
func firstBrace(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '}' {
			return i
		}
	}
	return -1
}

// spanTransformer pairs each rawBlock after a "]" with the "[" that opens
// that "]", as Pandoc parses a bracketed span, and wraps the nodes between
// in a bracketSpan. goldmark leaves an unmatched [...] as text nodes that
// still point at the source, so the brackets are counted by their source
// bytes: a nested pair balances, and a bracket after an odd run of
// backslashes is escaped and counts for nothing. A link, an image, code and
// emphasis are atomic, their insides never counted, and nothing is paired
// inside a link's text, where a badge stays literal.
//
// It also reports a heading that ends in such a block nothing took: in
// ## Call f(x){#call}, after a footnote reference, or after an escaped
// "\]", there is no span or link, and the block would sit in the heading
// as text with no word, though it once set the heading's id. On the tree,
// an escaped brace, ## f(x)\{#a}, never became a block, so it is quiet.
// warn may be nil.
type spanTransformer struct{ warn func(string) }

func (st spanTransformer) Transform(doc *ast.Document, reader gmtext.Reader, _ parser.Context) {
	src := reader.Source()
	var blocks []*rawBlock
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch x := n.(type) {
		case *ast.Link, *ast.Image:
			return ast.WalkSkipChildren, nil
		case *rawBlock:
			if x.after == ']' {
				blocks = append(blocks, x)
			}
		}
		return ast.WalkContinue, nil
	})
	for _, b := range blocks {
		pairSpan(b, src)
	}
	if st.warn != nil {
		warnUnpairedHeadingBlocks(doc, src, st.warn)
	}
}

// warnUnpairedHeadingBlocks reports each heading whose last inline is a
// rawBlock left unpaired, meant as attributes, after anything but a link
// or an image: those are LinkAttrs' to take or refuse, and Chips reports
// the refused one.
func warnUnpairedHeadingBlocks(doc *ast.Document, src []byte, warn func(string)) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		h, ok := n.(*ast.Heading)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		b, ok := h.LastChild().(*rawBlock)
		if !ok || !meantAsAttrs(string(b.value[1:len(b.value)-1])) {
			return ast.WalkSkipChildren, nil
		}
		switch b.PreviousSibling().(type) {
		case *ast.Link, *ast.Image:
			return ast.WalkSkipChildren, nil
		}
		warn(fmt.Sprintf("heading %q: %s touches a ] or ) but no span or link took it; write a space before it to give it to the heading", strings.TrimSpace(inlineText(h, b, src)), b.value))
		return ast.WalkSkipChildren, nil
	})
}

// inlineText is n's text as a reader sees it, up to stop, leaving out a
// footnote reference's number.
func inlineText(n, stop ast.Node, src []byte) string {
	var sb strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if c == stop {
			return ast.WalkStop, nil
		}
		if !entering {
			return ast.WalkContinue, nil
		}
		switch x := c.(type) {
		case *east.FootnoteLink:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			v := x.Segment.Value(src)
			// A backslash escape is a reader's one character.
			for i := 0; i < len(v); i++ {
				if v[i] == '\\' && i+1 < len(v) && util.IsPunct(v[i+1]) {
					i++
				}
				sb.WriteByte(v[i])
			}
		case *ast.String:
			sb.Write(x.Value)
		}
		return ast.WalkContinue, nil
	})
	return sb.String()
}

// pairSpan wraps b's label in a bracketSpan, if the "]" before b closes a
// bracket opened among its earlier siblings.
func pairSpan(b *rawBlock, src []byte) {
	closer, ok := b.PreviousSibling().(*ast.Text)
	if !ok {
		return
	}
	cs := closer.Segment
	if cs.Len() == 0 || src[cs.Stop-1] != ']' || escaped(src, cs.Stop-1) {
		return
	}
	depth := 0
	for n := ast.Node(closer); n != nil; n = n.PreviousSibling() {
		t, ok := n.(*ast.Text)
		if !ok {
			continue // atomic: its insides are not this label's brackets
		}
		stop := t.Segment.Stop
		if t == closer {
			stop-- // past the closing "]" itself
		}
		for i := stop - 1; i >= t.Segment.Start; i-- {
			if (src[i] != '[' && src[i] != ']') || escaped(src, i) {
				continue
			}
			if src[i] == ']' {
				depth++
				continue
			}
			if depth > 0 {
				depth--
				continue
			}
			wrapSpan(t, i, closer, b)
			return
		}
	}
}

// escaped reports whether src[i] follows an odd run of backslashes.
func escaped(src []byte, i int) bool {
	n := 0
	for j := i - 1; j >= 0 && src[j] == '\\'; j-- {
		n++
	}
	return n%2 == 1
}

// wrapSpan makes a bracketSpan of everything between the "[" at src[open], in
// opener, and the "]" ending closer, and puts it in place of b.
func wrapSpan(opener *ast.Text, open int, closer *ast.Text, b *rawBlock) {
	parent := b.Parent()
	span := &bracketSpan{block: b.value}
	parent.InsertBefore(parent, b, span)
	parent.RemoveChild(parent, b)

	// Collect the label's nodes first: moving one changes its siblings.
	var label []ast.Node
	if opener == closer {
		inner := gmtext.NewSegment(open+1, closer.Segment.Stop-1)
		if inner.Len() > 0 {
			label = append(label, ast.NewTextSegment(inner))
		}
		opener.Segment = opener.Segment.WithStop(open)
	} else {
		if rest := gmtext.NewSegment(open+1, opener.Segment.Stop); rest.Len() > 0 {
			t := ast.NewTextSegment(rest)
			t.SetSoftLineBreak(opener.SoftLineBreak())
			t.SetHardLineBreak(opener.HardLineBreak())
			label = append(label, t)
		}
		opener.Segment = opener.Segment.WithStop(open)
		opener.SetSoftLineBreak(false)
		opener.SetHardLineBreak(false)
		for n := opener.NextSibling(); n != nil && n != closer; n = n.NextSibling() {
			label = append(label, n)
		}
		closer.Segment = closer.Segment.WithStop(closer.Segment.Stop - 1)
		if closer.Segment.Len() > 0 {
			label = append(label, closer)
		}
	}
	for _, n := range label {
		if n.Parent() != nil {
			n.Parent().RemoveChild(n.Parent(), n)
		}
		span.AppendChild(span, n)
	}
	if opener.Segment.Len() == 0 {
		parent.RemoveChild(parent, opener)
	}
	if closer != opener && closer.Segment.Len() == 0 && closer.Parent() != nil {
		closer.Parent().RemoveChild(closer.Parent(), closer)
	}
}

// spanRenderer writes a bracketSpan as <span md2html-span="{…}"> around its
// label, and a rawBlock left unpaired as the escaped text it is.
type spanRenderer struct{}

func (spanRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindSpan, func(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, _ = w.WriteString(`<span ` + spanMarker + `="`)
			_, _ = w.Write(util.EscapeHTML(node.(*bracketSpan).block))
			_, _ = w.WriteString(`">`)
		} else {
			_, _ = w.WriteString("</span>")
		}
		return ast.WalkContinue, nil
	})
	reg.Register(kindRawBlock, func(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, _ = w.Write(util.EscapeHTML(node.(*rawBlock).value))
		}
		return ast.WalkContinue, nil
	})
}
