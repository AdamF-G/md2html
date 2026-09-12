package md2html

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// parseFragment parses an HTML fragment and re-parents the resulting
// top-level nodes under a single synthetic root.
//
// html.ParseFragment returns top-level nodes as a slice with no common
// parent. Callers that walk only a node's children would never examine
// them, so every top-level element — most importantly tables and divs —
// would be invisible to transforms. Re-parenting makes one uniform tree.
func parseFragment(b []byte) (*html.Node, error) {
	ctx := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
	nodes, err := html.ParseFragment(bytes.NewReader(b), ctx)
	if err != nil {
		return nil, err
	}
	root := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	return root, nil
}

// renderTree serializes a synthetic root's children back to HTML. The root
// itself is not emitted.
func renderTree(root *html.Node) ([]byte, error) {
	var buf bytes.Buffer
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&buf, c); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// walk visits n and every descendant in document order. It is safe against
// fn removing or replacing the node it is given, because the next sibling
// is captured before fn runs.
//
// A node fn inserts into the parent's child list during a pass is not
// itself visited by that pass: each sibling link is read before fn runs, so
// an insertion before the current node is already behind the cursor and one
// after it is never picked up. TableScroll relies on this — it collects its
// targets first, then wraps them, and the wrapper divs it inserts are never
// re-examined. A node inserted *below* the current one is still visited,
// since the descent into its children happens after fn returns.
func walk(n *html.Node, fn func(*html.Node)) {
	if n == nil {
		return
	}
	fn(n)
	var next *html.Node
	for c := n.FirstChild; c != nil; c = next {
		next = c.NextSibling
		walk(c, fn)
	}
}

// headingNodes returns every h1–h6 element under root, in document order.
//
// Three transforms need this same set — HeadingAnchors to assign ids,
// SectionLinks to index numbered headings, and TOC to list them — so it
// lives here rather than being re-typed as a switch on n.DataAtom in each
// one. HeadingAnchors calls this directly rather than keeping its own copy:
// its former walk was character-for-character the same predicate, so the
// duplication bought no margin on the byte-identical-slug guarantee, only a
// second place for the two to quietly drift apart.
func headingNodes(root *html.Node) []*html.Node {
	var heads []*html.Node
	walk(root, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		switch n.DataAtom {
		case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
			heads = append(heads, n)
		}
	})
	return heads
}

// headingText returns a heading's text with status chips and the anchor
// link left out, so a relabeled marker cannot change an anchor other
// documents link to, and the trailing "#" HeadingAnchors appends never
// shows up as part of the heading's own text (SectionLinks and TOC both
// build on this, and would otherwise have to hand-strip a trailing "#" —
// silently truncating a heading whose visible text legitimately ends in
// one).
func headingText(n *html.Node) string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
			return
		}
		if x.Type == html.ElementNode && (hasClass(x, "chip") || (x.DataAtom == atom.A && hasClass(x, "anchor"))) {
			return
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return b.String()
}
