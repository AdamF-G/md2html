package md2html

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// alertMarkerRe matches a GitHub alert marker at the very start of a
// blockquote's first paragraph, alone on its line.
//
// Upper case only, and alone on the line, because that is what GitHub
// accepts. Being more permissive than GitHub would let a document render
// as a callout here and as a stray "[!note]" there, which is backwards for
// a syntax adopted precisely so the source file reads correctly where it
// lives.
var alertMarkerRe = regexp.MustCompile(`^\[!([A-Z]+)\](\r?\n|$)`)

// alertKinds maps GitHub's five alert types onto the shipped container
// vocabulary. There is no separate alert styling: an alert is an alias for
// the container it matches, so one construct has one rendering and the
// stylesheet keeps one set of names to know about.
//
// The three informational types collapse onto callout because the
// stylesheet draws one informational box. That loses the distinction
// between NOTE, TIP and IMPORTANT in the output; it is kept in the source,
// where GitHub still renders all three distinctly.
var alertKinds = map[string]string{
	"NOTE":      "callout",
	"TIP":       "callout",
	"IMPORTANT": "callout",
	"WARNING":   "warning",
	"CAUTION":   "warning",
}

// Alerts converts GitHub alert blockquotes into the shipped containers.
//
//	> [!WARNING]
//	> Overwrites state.
//
// becomes exactly what "::: warning" produces.
//
// This is the one callout spelling GitHub, Obsidian, Typora and Pandoc's
// gfm and commonmark_x readers all understand, and the only one that
// renders correctly in the place md2html's input actually lives: a
// repository. "::: warning" shows up on GitHub as the literal text
// "::: warning".
//
// It degrades the right way too. A renderer that does not know the syntax
// shows an ordinary blockquote with a visible marker, rather than the
// unstyled div a stray ::: fence leaves behind.
func Alerts() Transform {
	return Transform{Name: "alerts", Fn: func(root *html.Node) error {
		// Collect-then-mutate, the same discipline Containers and
		// TableScroll use: this replaces elements, and a walk that
		// rewrites the tree it is walking is where ordering bugs live.
		var quotes []*html.Node
		walk(root, func(n *html.Node) {
			if n.Type == html.ElementNode && n.DataAtom == atom.Blockquote {
				quotes = append(quotes, n)
			}
		})
		for _, bq := range quotes {
			kind, p, ok := alertMarker(bq)
			if !ok {
				continue
			}
			stripAlertMarker(p)
			toAlertDiv(bq, containerKinds[kind])
		}
		return nil
	}}
}

// alertMarker reports the container kind a blockquote's marker names, and
// the paragraph carrying it.
func alertMarker(bq *html.Node) (kind string, p *html.Node, ok bool) {
	p = firstElementChild(bq)
	if p == nil || p.DataAtom != atom.P ||
		p.FirstChild == nil || p.FirstChild.Type != html.TextNode {
		return "", nil, false
	}
	m := alertMarkerRe.FindStringSubmatch(p.FirstChild.Data)
	if m == nil {
		return "", nil, false
	}
	kind, known := alertKinds[m[1]]
	if !known {
		return "", nil, false
	}
	return kind, p, true
}

// firstElementChild returns the first child element, skipping the
// inter-element whitespace goldmark leaves between blocks.
func firstElementChild(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) == "" {
			continue
		}
		if c.Type == html.ElementNode {
			return c
		}
		return nil
	}
	return nil
}

// stripAlertMarker removes the marker line from the paragraph that carried
// it, and the paragraph itself when the marker was all it held.
func stripAlertMarker(p *html.Node) {
	t := p.FirstChild
	t.Data = alertMarkerRe.ReplaceAllString(t.Data, "")
	if t.Data == "" {
		p.RemoveChild(t)
	}
	if p.FirstChild == nil {
		p.Parent.RemoveChild(p)
	}
}

// toAlertDiv rebuilds a blockquote as the div its kind calls for.
//
// The element is replaced rather than relabeled for the same reason
// toDetails replaces rather than relabels: x/net/html keys rendering off
// both DataAtom and Data, and changing one without the other leaves the
// node inconsistent.
func toAlertDiv(bq *html.Node, k containerKind) {
	div := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div",
		Attr: []html.Attribute{{Key: "class", Val: k.class}}}
	for c := bq.FirstChild; c != nil; {
		next := c.NextSibling
		bq.RemoveChild(c)
		div.AppendChild(c)
		c = next
	}
	bq.Parent.InsertBefore(div, bq)
	bq.Parent.RemoveChild(bq)
}
