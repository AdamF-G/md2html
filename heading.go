package md2html

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	gmtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// headingParsers is goldmark's default block parsers with its two heading
// parsers wrapped, so a heading's trailing {…} is read by md2html's
// attribute parser under a span's rules (checkAttrBlock) rather than by
// goldmark's own, which had a grammar of its own: single quotes and bare
// keys left the whole block in the heading's text, an unquoted number
// became empty, and a span's or a link's block at the end of a heading
// was taken as the heading's.
//
// The wrappers sit at the priorities goldmark gives the parsers they wrap,
// and do their work where goldmark read the block, after the heading's
// line is parsed as a block and before its text is parsed inline, so the
// block never reaches the heading's text, whatever Markdown is inside it.
func headingParsers(warn func(string)) []util.PrioritizedValue {
	// Told apart by what triggers them: goldmark's types are unexported,
	// and a priority is only a number.
	ps := parser.DefaultBlockParsers()
	for i, p := range ps {
		bp, ok := p.Value.(parser.BlockParser)
		if !ok {
			continue
		}
		switch string(bp.Trigger()) {
		case "-=": // setext
			ps[i] = util.Prioritized(&headingAttrParser{BlockParser: bp, setext: true, warn: warn}, p.Priority)
		case "#": // ATX
			ps[i] = util.Prioritized(&headingAttrParser{BlockParser: bp, warn: warn}, p.Priority)
		}
	}
	return ps
}

// headingAttrParser wraps one of goldmark's heading parsers, built with no
// options, and reads the heading's block itself.
type headingAttrParser struct {
	parser.BlockParser
	setext bool
	warn   func(string)
}

// SetOption passes a parser option on to the parser wrapped, as goldmark
// would have given it unwrapped: parser.WithAutoHeadingID(), say.
func (p *headingAttrParser) SetOption(name parser.OptionName, value interface{}) {
	if o, ok := p.BlockParser.(parser.SetOptioner); ok {
		o.SetOption(name, value)
	}
}

// Open reads an ATX heading's block once goldmark has opened it. goldmark,
// built without its attribute option, has trimmed a closing # sequence only
// when it ended the line; with a block after it, ## Title ## {#id}, the
// hashes are still there, so they are trimmed again once the block is off.
func (p *headingAttrParser) Open(parent ast.Node, reader gmtext.Reader, pc parser.Context) (ast.Node, parser.State) {
	node, state := p.BlockParser.Open(parent, reader, pc)
	if node != nil && !p.setext {
		if p.takeBlock(node, reader.Source()) {
			trimClosingHashes(node, reader.Source())
		}
	}
	return node, state
}

// Close reads a setext heading's block once goldmark has turned the
// paragraph above the underline into the heading.
func (p *headingAttrParser) Close(node ast.Node, reader gmtext.Reader, pc parser.Context) {
	p.BlockParser.Close(node, reader, pc)
	if p.setext && node.Parent() != nil {
		p.takeBlock(node, reader.Source())
	}
}

// takeBlock reads the trailing block of node's last line, when it is the
// heading's, sets its attributes and trims it off the line. It reports
// whether it did.
func (p *headingAttrParser) takeBlock(node ast.Node, source []byte) bool {
	n := node.Lines().Len()
	if n == 0 {
		return false
	}
	seg := node.Lines().At(n - 1)
	line := seg.Value(source)
	start, block, ok := takeHeadingBlock(line)
	if !ok {
		return false
	}
	who := `heading "` + strings.TrimSpace(string(line[:start])) + `"`
	a, ok := checkAttrBlock(who, block, false, p.warn)
	if !ok {
		return false
	}
	if a.id != "" {
		node.SetAttributeString("id", []byte(a.id))
	}
	if len(a.classes) > 0 {
		node.SetAttributeString("class", []byte(strings.Join(a.classes, " ")))
	}
	for _, x := range a.extra {
		node.SetAttributeString(x.Key, []byte(x.Val))
	}
	seg.Stop = seg.Start + start
	node.Lines().Set(n-1, seg.TrimRightSpace(source))
	return true
}

// takeHeadingBlock finds the {…} that ends a heading's line: the first
// unescaped "{" whose block, closed by braceSpan, runs to the end of the
// line. A quote counts only inside the block, as goldmark counted it:
// braceBlock, which counts one anywhere on the line, would have
// ## Don't panic {#calm} lose its id.
//
// A block straight after a "]" or a ")" is the span's or the link's before
// it, so ok is false for it, and it stays in the heading's text for Chips
// or LinkAttrs to read. start is where the block's "{" is.
func takeHeadingBlock(line []byte) (start int, block string, ok bool) {
	s := string(bytes.TrimRight(line, " \t"))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '{':
			if braceSpan(s, i) != len(s)-1 {
				continue
			}
			if i > 0 && (s[i-1] == ']' || s[i-1] == ')') {
				return 0, "", false
			}
			return i, s[i+1 : len(s)-1], true
		}
	}
	return 0, "", false
}

// trimClosingHashes removes an ATX heading's optional closing sequence of
// "#"s, and the spaces before it, from node's line: the same rule
// goldmark's own parser applies, which needs a space before the hashes.
func trimClosingHashes(node ast.Node, source []byte) {
	seg := node.Lines().At(0)
	line := seg.Value(source)
	i := len(line) - 1
	for i >= 0 && line[i] == '#' {
		i--
	}
	if i == len(line)-1 {
		return
	}
	if i >= 0 && line[i] != ' ' && line[i] != '\t' {
		return
	}
	seg.Stop = seg.Start + i + 1
	node.Lines().Set(0, seg.TrimRightSpace(source))
}

// headingRenderer writes <h1>…<h6> with every attribute a heading's block
// may keep, aria- names included: goldmark's own filters a heading through
// its global list, which admits data- but not aria-, and changing that
// package-level filter would change it for any other goldmark user in the
// same process.
type headingRenderer struct{}

func (headingRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, renderHeading)
}

func renderHeading(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Heading)
	if !entering {
		_, _ = w.WriteString("</h")
		_ = w.WriteByte("0123456"[n.Level])
		_, _ = w.WriteString(">\n")
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString("<h")
	_ = w.WriteByte("0123456"[n.Level])
	for _, a := range n.Attributes() {
		name := string(a.Name)
		if !safeAttrName(name) || !(strings.HasPrefix(name, "data-") || strings.HasPrefix(name, "aria-") ||
			gmhtml.HeadingAttributeFilter.Contains(a.Name)) {
			continue
		}
		v, ok := a.Value.([]byte)
		if !ok {
			continue
		}
		_, _ = w.WriteString(" " + name + `="`)
		_, _ = w.Write(util.EscapeHTML(v))
		_ = w.WriteByte('"')
	}
	_ = w.WriteByte('>')
	return ast.WalkContinue, nil
}
