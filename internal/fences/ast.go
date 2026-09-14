package fences

import (
	"github.com/yuin/goldmark/ast"
)

// A FencedContainer struct represents a fenced code block of Markdown text.
type FencedContainer struct {
	ast.BaseBlock
	element string
}

// Dump implements Node.Dump .
func (n *FencedContainer) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// KindFencedContainer is a NodeKind of the FencedContainer node.
var KindFencedContainer = ast.NewNodeKind("FencedContainer")

// Kind implements Node.Kind.
func (n *FencedContainer) Kind() ast.NodeKind {
	return KindFencedContainer
}

// NewFencedContainer return a new FencedContainer node.
func NewFencedContainer() *FencedContainer {
	return &FencedContainer{
		BaseBlock: ast.BaseBlock{},
	}
}

// Attr is one attribute to set on a container element.
type Attr struct {
	Name  string
	Value string
}

// Info is the result of splitting a container's fence-line remainder.
//
// TitleStart and TitleEnd are byte offsets into the info string that was
// split, half-open. TitleEnd <= TitleStart means the fence line carried no
// title. Offsets are used rather than a string so the parser can point a
// source segment at the title and let goldmark parse its inlines.
type Info struct {
	Kind       string
	Attrs      []Attr
	TitleStart int
	TitleEnd   int
}

// FencedContainerTitle is a container's title, as written on its fence
// line. It is a block node with lines rather than a raw string so goldmark
// parses its inline markup the same way it would anywhere else.
type FencedContainerTitle struct {
	ast.BaseBlock
}

// Dump implements Node.Dump.
func (n *FencedContainerTitle) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// KindFencedContainerTitle is the NodeKind of FencedContainerTitle.
var KindFencedContainerTitle = ast.NewNodeKind("FencedContainerTitle")

// Kind implements Node.Kind.
func (n *FencedContainerTitle) Kind() ast.NodeKind { return KindFencedContainerTitle }

// NewFencedContainerTitle returns a new FencedContainerTitle node.
func NewFencedContainerTitle() *FencedContainerTitle {
	return &FencedContainerTitle{}
}
