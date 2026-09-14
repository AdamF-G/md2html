package fences

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// Extender allows you to use fenced divs / fenced containers / fences in markdown
//
// Fences are a way to wrap other elements in divs and giving those divs
// attributes such as ids or css classes using the same syntax as with headings.
//
// :::{#big-div .add-border}
// this is some text
//
// ## with a header
//
// :::{.background-green .font-big}
// ```R
// X <- as.data.table(iris)
// X[Species != "virginica", mean(Sepal.Length), Species]
// ```
// :::
// :::
type Extender struct {
	priority int // optional int != 0. the priority value for parser and renderer. Defaults to 100.

	// SplitInfo splits a fence line's remainder — everything after the
	// colons, trimmed — into a kind word, attributes and the source range
	// of a title. It reports false to leave the line in the content
	// stream, which is what upstream always did.
	//
	// The hook exists so this package stays free of md2html's container
	// vocabulary: the grammar lives in md2html, beside the shared
	// attribute parser it needs.
	SplitInfo func(info string) (Info, bool)
}

// This implements the Extend method for goldmark-fences.Extender
func (e *Extender) Extend(md goldmark.Markdown) {
	priority := 100

	if e.priority != 0 {
		priority = e.priority
	}
	md.Parser().AddOptions(
		parser.WithBlockParsers(
			util.Prioritized(&fencedContainerParser{splitInfo: e.SplitInfo}, priority),
		),
	)
	md.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&Renderer{}, priority),
		),
	)
}
