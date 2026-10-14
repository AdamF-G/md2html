package md2html

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// A marker is a paragraph standing alone as [[name key="value" ...]], the
// shape [[toc]] already has. Site mode adds several; each expands to plain
// HTML and the container around it decides how it looks. Names are
// lowercase and values double-quoted, so a sentence that happens to hold
// brackets is never read as one.
type marker struct {
	node  *html.Node
	name  string
	attrs map[string]string
}

var (
	markerRe     = regexp.MustCompile(`^\[\[([a-z][a-z-]*)((?:\s+[a-z][a-z-]*="[^"]*")*)\s*\]\]$`)
	markerAttrRe = regexp.MustCompile(`([a-z][a-z-]*)="([^"]*)"`)
)

// navMarkerNames are the markers that need a site: outside one they are
// warned about and left as written.
var navMarkerNames = map[string]bool{
	"content": true, "pages": true, "breadcrumb": true, "prev-next": true, "search": true,
}

// markerText returns a paragraph's text when it is made only of text and
// links. GFM turns a bare URL into an <a>, so a marker whose attribute holds
// one is no longer a single text node; reading the link's text back keeps it
// a marker. A paragraph holding anything else — a code span, emphasis — is
// prose.
func markerText(p *html.Node) (string, bool) {
	var b strings.Builder
	for c := p.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == html.TextNode:
			b.WriteString(c.Data)
		case c.Type == html.ElementNode && c.DataAtom == atom.A:
			b.WriteString(textOf(c))
		default:
			return "", false
		}
	}
	return strings.TrimSpace(b.String()), true
}

// findMarkers returns every marker paragraph in root in document order.
// [[toc]] is left to the contents list transform, which owns it.
func findMarkers(root *html.Node) []marker {
	var out []marker
	walk(root, func(n *html.Node) {
		if n.Type != html.ElementNode || n.DataAtom != atom.P {
			return
		}
		s, ok := markerText(n)
		if !ok {
			return
		}
		m := markerRe.FindStringSubmatch(s)
		if m == nil || m[1] == "toc" {
			return
		}
		attrs := map[string]string{}
		for _, a := range markerAttrRe.FindAllStringSubmatch(m[2], -1) {
			attrs[a[1]] = a[2]
		}
		out = append(out, marker{node: n, name: m[1], attrs: attrs})
	})
	return out
}

// replaceMarker puts nodes where the marker's paragraph was, and removes it.
func replaceMarker(m marker, nodes ...*html.Node) {
	for _, n := range nodes {
		if n != nil {
			m.node.Parent.InsertBefore(n, m.node)
		}
	}
	m.node.Parent.RemoveChild(m.node)
}

// elem builds an element; attrs are key, value pairs, rendered in order.
func elem(tag string, attrs ...string) *html.Node {
	n := &html.Node{Type: html.ElementNode, Data: tag, DataAtom: atom.Lookup([]byte(tag))}
	for i := 0; i+1 < len(attrs); i += 2 {
		n.Attr = append(n.Attr, html.Attribute{Key: attrs[i], Val: attrs[i+1]})
	}
	return n
}

// textNode builds a text node.
func textNode(s string) *html.Node { return &html.Node{Type: html.TextNode, Data: s} }
