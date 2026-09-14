package fences

import (
	"bytes"

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

// renderContainerAttributes writes a container's attributes, allowing the
// aria- prefix alongside goldmark's global attribute list and the data-
// prefix its own renderer already permits.
//
// An accessible name is what a landmark most needs, and aria-label is not
// in that list — so without this a <nav> or a labelled region could be
// emitted but never named, which is worse than not emitting it at all.
//
// Values are escaped and names are not, exactly as goldmark does it. Names
// therefore have to arrive already validated; md2html's safeAttrName is
// what does that, at the point where author text becomes attributes.
func renderContainerAttributes(w util.BufWriter, n ast.Node) {
	for _, attr := range n.Attributes() {
		if !FencedContainerAttributeFilter.Contains(attr.Name) &&
			!bytes.HasPrefix(attr.Name, []byte("data-")) &&
			!bytes.HasPrefix(attr.Name, []byte("aria-")) {
			continue
		}
		_, _ = w.WriteString(" ")
		_, _ = w.Write(attr.Name)
		_, _ = w.WriteString(`="`)
		var value []byte
		switch typed := attr.Value.(type) {
		case []byte:
			value = typed
		case string:
			value = util.StringToReadOnlyBytes(typed)
		}
		_, _ = w.Write(util.EscapeHTML(value))
		_ = w.WriteByte('"')
	}
}

func (r *Renderer) renderFencedContainer(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*FencedContainer)
	if entering {
		n.element = "div"
		if n.Attributes() != nil {
			_, _ = w.WriteString("<" + n.element)
			renderContainerAttributes(w, n)
			_, _ = w.WriteString(">\n")
		} else {
			_, _ = w.WriteString("<" + n.element + ">\n")
		}
	} else {
		_, _ = w.WriteString("</" + n.element + ">\n")
	}
	return ast.WalkContinue, nil
}
