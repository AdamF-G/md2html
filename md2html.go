// Package md2html converts Markdown documents to HTML with tree-level
// transforms that reach hand-written raw HTML as well as generated markup.
package md2html

import (
	"bytes"
	"fmt"

	fences "github.com/stefanfritsch/goldmark-fences"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	goldhtml "github.com/yuin/goldmark/renderer/html"
	"go.abhg.dev/goldmark/mermaid"
	"golang.org/x/net/html"
)

// Transform mutates a parsed HTML tree in place. Fn receives the synthetic
// root node whose children are the document's top-level elements.
type Transform struct {
	// Name identifies the transform, e.g. for logging or diagnostics.
	Name string
	// Fn is the function applied to the parsed HTML tree.
	Fn func(*html.Node) error
}

// Options controls a single document conversion.
type Options struct {
	// Fragment emits Artifact shape (marker, title, style, body) instead of
	// a full HTML document.
	Fragment bool
	// Title overrides the derived title. Empty means derive from the first
	// <h1>, falling back to SourcePath's base name.
	Title string
	// SourcePath is the absolute path of the source document. Used for the
	// title fallback and diagnostics.
	SourcePath string
	// CSS replaces the embedded default stylesheet. Empty uses the default.
	CSS string
	// Transforms to run. Nil means Builtins().
	Transforms []Transform
	// LinkMap maps an href exactly as written in the source to its
	// replacement. Populated by the crawler; nil for standalone conversion.
	LinkMap map[string]string
}

// newParser builds the goldmark instance. Every extension here is required
// by the conformance fixture; see docs/specs.
func newParser() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.DefinitionList,
			extension.Footnote,
			// NoScript: artifacts render mermaid natively, so the extension
			// must not inject its own MermaidJS <script> tag.
			&mermaid.Extender{RenderMode: mermaid.RenderModeClient, NoScript: true},
			&fences.Extender{},
		),
		goldmark.WithParserOptions(parser.WithAttribute()),
		goldmark.WithRendererOptions(goldhtml.WithUnsafe()),
	)
}

// Convert renders Markdown to HTML, running transforms over the parsed tree.
func Convert(src []byte, opt Options) ([]byte, error) {
	var buf bytes.Buffer
	if err := newParser().Convert(src, &buf); err != nil {
		return nil, err
	}
	root, err := parseFragment(buf.Bytes())
	if err != nil {
		return nil, err
	}

	// Before transforms: HeadingAnchors would otherwise contribute its "#"
	// anchor text to the derived title.
	title := opt.Title
	if title == "" {
		title = extractTitle(root, opt.SourcePath)
	}

	transforms := opt.Transforms
	if transforms == nil {
		transforms = Builtins()
	}
	if opt.LinkMap != nil {
		// Full-slice expression: never append into the caller's array.
		transforms = append(transforms[:len(transforms):len(transforms)],
			LinkRewrite(opt.LinkMap))
	}
	for _, t := range transforms {
		if err := t.Fn(root); err != nil {
			return nil, fmt.Errorf("transform %s: %w", t.Name, err)
		}
	}

	body, err := renderTree(root)
	if err != nil {
		return nil, err
	}
	css := opt.CSS
	if css == "" {
		css = defaultCSS
	}
	if opt.Fragment {
		return renderFragment(body, title, css), nil
	}
	return renderPage(body, title, css), nil
}
