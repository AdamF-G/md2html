package fences

import (
	"regexp"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// A Config struct has configurations for the HTML based renderers.
type Config struct {
	Writer    html.Writer
	HardWraps bool
	XHTML     bool
	Unsafe    bool
}

// HeadingAttributeFilter defines attribute names which heading elements can have
var FencedContainerAttributeFilter = html.GlobalAttributeFilter

// A Renderer struct is an implementation of renderer.NodeRenderer that renders
// nodes as (X)HTML.
type Renderer struct {
	Config
}

// RegisterFuncs implements NodeRenderer.RegisterFuncs .
func (r *Renderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindFencedContainer, r.renderFencedContainer)
	reg.Register(KindFencedContainerTitle, r.renderFencedContainerTitle)
}

// renderFencedContainerTitle emits a container's title. The class is a
// marker: md2html's container transform finds the paragraph by it (together
// with the container's data-fence-title attribute) and then rebuilds it —
// as a title paragraph, as a <summary>, or as unclassed prose when the kind
// turns out to be unknown. It is the same class that transform writes for a
// title paragraph, so the marker and the final markup coincide for the
// commonest case.
func (r *Renderer) renderFencedContainerTitle(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<p class="container-title">`)
	} else {
		_, _ = w.WriteString("</p>\n")
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderFencedContainer(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*FencedContainer)
	if entering {
		n.element = "div"
		if isNav(node) {
			n.element = "nav"
		}

		if n.Attributes() != nil {
			_, _ = w.WriteString("<" + n.element)
			html.RenderAttributes(w, n, FencedContainerAttributeFilter)
			_, _ = w.WriteString(">\n")
		} else {
			_, _ = w.WriteString("<" + n.element + ">\n")
		}
	} else {
		_, _ = w.WriteString("</" + n.element + ">\n")
	}
	return ast.WalkContinue, nil
}

func isNav(node ast.Node) bool {
	class, ok := node.AttributeString("class")
	if !ok {
		return false
	}
	if navChk.Match(class.([]byte)) {
		return true
	}

	return false
}

// check for the .nav class
var navChk = regexp.MustCompile(`(^| |\.)elem-nav($| )`)
