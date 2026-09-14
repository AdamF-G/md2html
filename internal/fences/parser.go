package fences

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type fencedContainerParser struct {
	splitInfo func(info string) (Info, bool)
}

var defaultFencedContainerParser = &fencedContainerParser{}

// NewFencedContainerParser returns a new BlockParser that
// parses fenced code blocks.
func NewFencedContainerParser() parser.BlockParser {
	return defaultFencedContainerParser
}

type fenceData struct {
	fenceID           string   // The ID of the fence. This enables nested fences with indentation
	char              byte     // Currently, this is always ":"
	indent            int      // The indentation of the opening (and closing) tags (:::{})
	length            int      // The length of the fence, e.g. is it ::: or ::::?
	node              ast.Node // The node of the fence
	contentIndent     int      // The indentation of the content relative to the previous fenced block. The first line of the content is taken as its indentation. If you want a fence with just a code block you need to use backticks
	contentHasStarted bool     // Only used as an indicator if contentIndent has been set already
}

var fencedContainerInfoKey = parser.NewContextKey()

// fenceAttrPrefix namespaces every attribute this parser uses to carry its
// own state to the renderer and to a post-processing transform: data-fence
// (the id that makes nested fences work), data-fence-kind and
// data-fence-title.
const fenceAttrPrefix = "data-fence"

// reservedFenceAttr reports whether an attribute name belongs to the parser
// rather than to the author.
//
// Two things go wrong if author text can set one. A forged data-fence-title
// makes the container claim an ordinary paragraph of the document as its
// title, which is then torn down and rebuilt — the author's own attributes
// on it are lost. A forged data-fence replaces the random id Open generated,
// so Continue's lookup finds no matching fenceData, leaves flevel at
// len(fdataMap) and indexes out of range: a document that panics the
// converter.
//
// So the namespace is reserved here, in the parser, rather than in the
// SplitInfo implementation that happens to supply the attributes today. The
// invariants are this parser's, and it has to hold them against any hook —
// and against the braced attribute block, which never passes through a hook
// at all.
func reservedFenceAttr(name string) bool {
	return strings.HasPrefix(name, fenceAttrPrefix)
}

func (b *fencedContainerParser) Trigger() []byte {
	return []byte{':'}
}

func (b *fencedContainerParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, lineSeg := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || line[pos] != ':' {
		return nil, parser.NoChildren
	}
	findent := pos
	fenceChar := line[pos]
	i := pos
	for ; i < len(line) && line[i] == fenceChar; i++ {
	}
	oFenceLength := i - pos
	if oFenceLength < 3 {
		return nil, parser.NoChildren
	}

	// ========================================================================== //
	// 	Without attributes we return

	if i >= len(line)-1 {
		// If there are no attributes we can't create a div because we won't know
		// if a ":::" ends the last fenced container or opens a new one
		return nil, parser.NoChildren
	}

	rest := line[i:]
	left := i + util.TrimLeftSpaceLength(rest)
	right := len(line) - 1 - util.TrimRightSpaceLength(rest)

	if left >= right {
		// As above:
		// If there are no attributes we can't create a div because we won't know
		// if a ":::" ends the last fenced container or opens a new one
		return nil, parser.NoChildren
	}

	// ========================================================================== //
	// 	With attributes we construct the node

	info := string(line[left : right+1])
	var parsed Info
	owned := false
	if b.splitInfo != nil {
		parsed, owned = b.splitInfo(info)
	}

	node := NewFencedContainer()
	fenceID := genRandomString(24)
	node.SetAttributeString("data-fence", []byte(fenceID))

	if owned {
		// The whole fence line belongs to the fence: consume it so that
		// nothing on it can be reinterpreted as document content by a
		// definition list marker or a setext underline on the next line.
		reader.Advance(right + 1)
		for _, a := range parsed.Attrs {
			if reservedFenceAttr(a.Name) {
				continue
			}
			node.SetAttributeString(a.Name, []byte(a.Value))
		}
		if parsed.Kind != "" {
			node.SetAttributeString("data-fence-kind", []byte(parsed.Kind))
		}
		if parsed.TitleEnd > parsed.TitleStart {
			t := NewFencedContainerTitle()
			// PeekLine returns a padded line: line[k] is
			// source[Start+k-Padding], not source[Start+k]. Without the
			// padding term a fence behind tab-expanded indentation — a
			// container inside a blockquote or a list — shifts its title
			// right by the pad width, and at the end of the buffer reads
			// past the source entirely.
			base := lineSeg.Start + left - lineSeg.Padding
			t.Lines().Append(text.NewSegment(base+parsed.TitleStart, base+parsed.TitleEnd))
			node.AppendChild(node, t)
			// Say on the container that the title element is one of ours.
			// The rendered paragraph is identified downstream by its class
			// alone, and md2html renders with WithUnsafe, so an author's
			// own raw <p class="container-title"> as the first block of a
			// titleless container — ":::card[]" makes one — would
			// otherwise be adopted as that container's title.
			//
			// This marker is trustworthy only because reservedFenceAttr
			// keeps author text out of the data-fence namespace; it is set
			// last, but the namespace is what makes it unforgeable, not
			// the ordering.
			node.SetAttributeString("data-fence-title", []byte("1"))
		}
	} else {
		reader.Advance(left)
		attrs, ok := parser.ParseAttributes(reader)
		if ok {
			for _, attr := range attrs {
				if bytes.HasPrefix(attr.Name, []byte(fenceAttrPrefix)) {
					continue
				}
				node.SetAttribute(attr.Name, attr.Value)
			}
		}
	}

	fdata := &fenceData{
		fenceID:           fenceID,
		char:              fenceChar,
		indent:            findent,
		length:            oFenceLength,
		node:              node,
		contentIndent:     0,
		contentHasStarted: false,
	}

	var fdataMap []*fenceData

	if oldData := pc.Get(fencedContainerInfoKey); oldData != nil {
		fdataMap = oldData.([]*fenceData)
		fdataMap = append(fdataMap, fdata)
	} else {
		fdataMap = []*fenceData{fdata}
	}
	pc.Set(fencedContainerInfoKey, fdataMap)

	// check if it's an empty block
	line, _ = reader.PeekLine()
	w, pos := util.IndentWidth(line, reader.LineOffset())

	if close, _ := hasClosingTag(line, w, pos, fdata); w < fdata.indent || close {
		return node, parser.NoChildren
	}

	return node, parser.HasChildren
}

func (b *fencedContainerParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	// ========================================================================== //
	// Get fenceID from node

	rawFenceID, ok := node.AttributeString("data-fence")
	if !ok {
		// huhu: don't panic in production
		panic("fenceID is missing")
	}
	fenceID := string(rawFenceID.([]byte))

	// ========================================================================== //
	// 	Get fenceData for current fenceID

	rawdata := pc.Get(fencedContainerInfoKey)
	fdataMap := rawdata.([]*fenceData)

	var fdata *fenceData
	var flevel int
	for flevel = 0; flevel < len(fdataMap); flevel++ {
		fdata = fdataMap[flevel]
		if fdata.fenceID == fenceID {
			break
		}
	}

	// ========================================================================== //
	// 	Set indentation level if it hasn't been set yet

	line, segment := reader.PeekLine()
	w, pos := util.IndentWidth(line, reader.LineOffset())

	if !fdata.contentHasStarted && !util.IsBlank(line[pos:]) {
		fdata.contentHasStarted = true
		fdata.contentIndent = w

		fdataMap[flevel] = fdata
		pc.Set(fencedContainerInfoKey, fdataMap)
	}

	// ========================================================================== //
	// Are we closing the node?
	// * Either the indentation is below the indentation of the opening tags
	// * or it is at the level of the opening tags but the content was indented
	// * or there is a closing tag and we're in the deepest fenced block
	// indentClose :=
	// 	!util.IsBlank(line) &&
	// 		(w < fdata.indent || (w == fdata.indent && fdata.contentIndent > 0))
	close, newline := hasClosingTag(line, w, pos, fdata)

	if close && flevel == len(fdataMap)-1 {
		reader.Advance(segment.Stop - segment.Start - newline + segment.Padding)
		fdataMap = fdataMap[:flevel]
		node.SetAttributeString("data-fence", []byte(fmt.Sprint(flevel)))

		if len(fdataMap) == 0 {
			return parser.Close
		} else {
			pc.Set(fencedContainerInfoKey, fdataMap)
			return parser.Close
		}
	}

	if fdata.contentIndent > 0 {
		dontJumpLineEnd := segment.Stop - segment.Start - 1
		if fdata.contentIndent < dontJumpLineEnd {
			dontJumpLineEnd = fdata.contentIndent
		}

		reader.Advance(dontJumpLineEnd)
	}

	return parser.Continue | parser.HasChildren
}

func (b *fencedContainerParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
}

func (b *fencedContainerParser) CanInterruptParagraph() bool {
	return true
}

func (b *fencedContainerParser) CanAcceptIndentedLine() bool {
	return false
}

func hasClosingTag(line []byte, w int, pos int, fdata *fenceData) (bool, int) {
	// else, check for the correct number of closing chars and provide the info
	// necessary to advance the reader
	if w == fdata.indent {
		i := pos
		for ; i < len(line) && line[i] == fdata.char; i++ {
		}
		length := i - pos

		if length >= fdata.length && util.IsBlank(line[i:]) {
			newline := 1
			if line[len(line)-1] != '\n' {
				newline = 0
			}

			return true, newline
		}
	}

	return false, 0
}
