package md2html

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	gmtext "github.com/yuin/goldmark/text"
)

// rawBlockParser takes a bracketed span's or a link's {…} block before
// goldmark parses inline Markdown, so Chips and LinkAttrs read what the
// author typed. Parsed, a block lost its shape: tip="a *b* c" became text
// around an <em>, so the whole span stayed literal; tip="x <b> y" put a
// real <b> in the page; and the backslash in \" was gone, so a value could
// not hold both kinds of quote.
//
// It takes a block only straight after a "]" or a ")", the bytes that end
// a span's label and a link, and only one that closes on its line, as
// chipRe would read it. The block becomes a raw string: goldmark escapes
// it and parses nothing inside. A brace anywhere else is left alone.
type rawBlockParser struct{}

func (rawBlockParser) Trigger() []byte { return []byte{'{'} }

func (rawBlockParser) Parse(_ ast.Node, block gmtext.Reader, _ parser.Context) ast.Node {
	line, seg := block.PeekLine()
	src := block.Source()
	if seg.Start == 0 || (src[seg.Start-1] != ']' && src[seg.Start-1] != ')') {
		return nil
	}
	end := braceSpan(string(line), 0)
	if end < 0 {
		return nil
	}
	s := ast.NewString(append([]byte(nil), line[:end+1]...))
	s.SetRaw(true)
	block.Advance(end + 1)
	return s
}
