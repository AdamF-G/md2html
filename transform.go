package md2html

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Builtins returns the transforms enabled by default.
func Builtins() []Transform {
	return []Transform{TableScroll(), HeadingAnchors(), ExternalLinks()}
}

// attr returns the value of the named attribute and whether it was present.
func attr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

// setAttr sets or replaces an attribute.
func setAttr(n *html.Node, key, val string) {
	for i, a := range n.Attr {
		if a.Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

// hasClass reports whether n carries the given class token.
func hasClass(n *html.Node, want string) bool {
	v, _ := attr(n, "class")
	for _, f := range strings.Fields(v) {
		if f == want {
			return true
		}
	}
	return false
}

// textOf returns the concatenated text content of n's subtree.
func textOf(n *html.Node) string {
	var b strings.Builder
	walk(n, func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
	})
	return b.String()
}

// TableScroll wraps every table in a horizontally scrollable container, so
// wide tables never force the page body to scroll sideways. It reaches
// hand-written tables in raw HTML as well as generated ones.
func TableScroll() Transform {
	return Transform{Name: "tableScroll", Fn: func(root *html.Node) error {
		var targets []*html.Node
		walk(root, func(n *html.Node) {
			if n.Type == html.ElementNode && n.DataAtom == atom.Table {
				if n.Parent != nil && hasClass(n.Parent, "table-scroll") {
					return // already wrapped
				}
				targets = append(targets, n)
			}
		})
		for _, t := range targets {
			p := t.Parent
			if p == nil {
				continue
			}
			div := &html.Node{
				Type: html.ElementNode, DataAtom: atom.Div, Data: "div",
				Attr: []html.Attribute{{Key: "class", Val: "table-scroll"}},
			}
			p.InsertBefore(div, t)
			p.RemoveChild(t)
			div.AppendChild(t)
		}
		return nil
	}}
}

// slugify converts heading text to a URL fragment. Letters and digits from
// any script are kept, so headings in Japanese, Cyrillic or Greek — and Latin
// words carrying diacritics — get meaningful ids rather than being stripped to
// nothing. HTML5 allows any id without whitespace, and a slug derived from the
// text stays stable when headings move, which a positional scheme would not.
func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// HeadingAnchors gives every heading a stable id and a linkable anchor.
// An id already present — from a {#custom-id} attribute — is left alone.
func HeadingAnchors() Transform {
	return Transform{Name: "headingAnchors", Fn: func(root *html.Node) error {
		seen := map[string]bool{}
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
		// Reserve every author-supplied id before assigning any generated
		// one, or a slug could claim a string an explicit id further down
		// the document is going to use, and the two would collide. An
		// explicit id always wins and is never rewritten; two explicit ids
		// that collide with each other are left exactly as written.
		for _, h := range heads {
			if id, ok := attr(h, "id"); ok && id != "" {
				seen[id] = true
			}
		}
		for i, h := range heads {
			id, ok := attr(h, "id")
			if !ok || id == "" {
				base := slugify(textOf(h))
				if base == "" {
					// Nothing to derive a slug from — a heading of only
					// punctuation, or one holding just an image. Fall back to
					// position so it is still linkable. Positional ids shift
					// when headings are inserted above, so this stays a last
					// resort rather than the general rule.
					base = fmt.Sprintf("section-%d", i+1)
				}
				id = base
				for n := 2; seen[id]; n++ {
					id = fmt.Sprintf("%s-%d", base, n)
				}
				setAttr(h, "id", id)
			}
			seen[id] = true
			a := &html.Node{
				Type: html.ElementNode, DataAtom: atom.A, Data: "a",
				Attr: []html.Attribute{
					{Key: "class", Val: "anchor"},
					{Key: "href", Val: "#" + id},
					{Key: "aria-hidden", Val: "true"},
				},
			}
			a.AppendChild(&html.Node{Type: html.TextNode, Data: "#"})
			h.AppendChild(a)
		}
		return nil
	}}
}

// LinkRewrite replaces hrefs and srcs using a map keyed by the link exactly
// as written in the source document. Links absent from the map are left
// untouched, which is how remote URLs, fragments, and deliberately
// unrewritten links survive.
func LinkRewrite(m map[string]string) Transform {
	return Transform{Name: "linkRewrite", Fn: func(root *html.Node) error {
		if len(m) == 0 {
			return nil
		}
		walk(root, func(n *html.Node) {
			if n.Type != html.ElementNode {
				return
			}
			key := linkAttrFor(n)
			if key == "" {
				return
			}
			href, ok := attr(n, key)
			if !ok {
				return
			}
			if repl, found := m[href]; found {
				setAttr(n, key, repl)
			}
		})
		return nil
	}}
}

// isExternal reports whether an href points off-site.
func isExternal(href string) bool {
	return strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") ||
		strings.HasPrefix(href, "//")
}

// ExternalLinks marks off-site links so they open in a new tab without
// leaking the referring window.
func ExternalLinks() Transform {
	return Transform{Name: "externalLinks", Fn: func(root *html.Node) error {
		walk(root, func(n *html.Node) {
			if n.Type != html.ElementNode || n.DataAtom != atom.A {
				return
			}
			href, ok := attr(n, "href")
			if !ok || !isExternal(href) {
				return
			}
			setAttr(n, "target", "_blank")
			setAttr(n, "rel", "noopener noreferrer")
		})
		return nil
	}}
}
