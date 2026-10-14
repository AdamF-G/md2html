package md2html

import (
	"fmt"
	"path/filepath"
	"strconv"

	"golang.org/x/net/html"
)

// navMarkers expands the markers that need the site's tree, for the page
// at src: in the shell, and in a site page's own body.
func (s *Site) navMarkers(src string, warn func(string)) Transform {
	say := func(format string, args ...any) {
		if warn != nil {
			warn(fmt.Sprintf(format, args...))
		}
	}
	return Transform{Name: "navMarkers", Fn: func(root *html.Node) error {
		page := s.bySrc[src]
		from := s.outBySrc[src]
		for _, m := range findMarkers(root) {
			switch m.name {
			case "pages":
				replaceMarker(m, s.pagesList(m, page, from, say))
			case "breadcrumb":
				replaceMarker(m, s.breadcrumb(page, from))
			case "prev-next":
				replaceMarker(m, s.prevNext(page, from))
			}
		}
		return nil
	}}
}

// href is the link from the page written at from to the file at to.
func (s *Site) href(from, to string) string {
	rel, err := filepath.Rel(filepath.Dir(from), to)
	if err != nil {
		return filepath.ToSlash(to)
	}
	return encodePath(filepath.ToSlash(rel))
}

func (s *Site) pagesList(m marker, page *siteNode, from string, say func(string, ...any)) *html.Node {
	depth := -1
	if v, ok := m.attrs["depth"]; ok {
		if d, err := strconv.Atoi(v); err == nil && d >= 1 {
			depth = d
		} else {
			say("[[pages]] depth %q is not a whole number of at least 1; listing every level", v)
		}
	}
	start := s.root
	switch v := m.attrs["from"]; v {
	case "", "root":
	case "section":
		start = nil
		for n := page; n != nil && n.parent != nil; n = n.parent {
			if n.parent == s.root {
				start = n
			}
		}
		if start == nil {
			return nil // the root page has no section
		}
	default:
		say("[[pages]] from %q is not root or section; listing from the root", v)
	}
	return s.pagesLevel(start, page, from, depth)
}

func (s *Site) pagesLevel(n, page *siteNode, from string, depth int) *html.Node {
	var shown []*siteNode
	for _, c := range n.children {
		if !c.hidden {
			shown = append(shown, c)
		}
	}
	if len(shown) == 0 || depth == 0 {
		return nil
	}
	ul := elem("ul", "class", "md2html-pages")
	for _, c := range shown {
		li := elem("li")
		if contains(c, page) {
			li.Attr = append(li.Attr, html.Attribute{Key: "class", Val: "md2html-branch"})
		}
		li.AppendChild(s.nodeLink(c, page, from))
		if sub := s.pagesLevel(c, page, from, depth-1); sub != nil {
			li.AppendChild(sub)
		}
		ul.AppendChild(li)
	}
	return ul
}

// nodeLink is a node's link, marked when it is the page itself, or its
// label when the node has no page.
func (s *Site) nodeLink(n, page *siteNode, from string) *html.Node {
	if n.doc == nil {
		span := elem("span", "class", "md2html-label")
		span.AppendChild(textNode(n.label))
		return span
	}
	a := elem("a", "href", s.href(from, n.doc.Out))
	if n == page {
		a.Attr = append(a.Attr, html.Attribute{Key: "aria-current", Val: "page"})
	}
	a.AppendChild(textNode(n.label))
	return a
}

// contains reports whether page is n or below it.
func contains(n, page *siteNode) bool {
	for p := page; p != nil; p = p.parent {
		if p == n {
			return true
		}
	}
	return false
}

func (s *Site) breadcrumb(page *siteNode, from string) *html.Node {
	if page == nil {
		return nil
	}
	var chain []*siteNode
	for n := page; n != nil; n = n.parent {
		chain = append([]*siteNode{n}, chain...)
	}
	ol := elem("ol", "class", "md2html-breadcrumb")
	for _, n := range chain {
		if n == s.root && n.doc == nil {
			continue
		}
		li := elem("li")
		li.AppendChild(s.nodeLink(n, page, from))
		ol.AppendChild(li)
	}
	// Its own landmark: a breadcrumb appears once and has one obvious name,
	// unlike [[pages]], which a shell may show twice with two labels.
	nav := elem("nav", "aria-label", "Breadcrumb")
	nav.AppendChild(ol)
	return nav
}

func (s *Site) prevNext(page *siteNode, from string) *html.Node {
	at := -1
	for i, n := range s.order {
		if n == page {
			at = i
		}
	}
	if at < 0 {
		return nil
	}
	div := elem("nav", "class", "md2html-prev-next", "aria-label", "Previous and next")
	add := func(n *siteNode, class, rel, dir string) {
		a := elem("a", "href", s.href(from, n.doc.Out), "class", class, "rel", rel)
		d := elem("span", "class", "md2html-dir")
		d.AppendChild(textNode(dir))
		t := elem("span", "class", "md2html-title")
		t.AppendChild(textNode(n.label))
		a.AppendChild(d)
		a.AppendChild(textNode(" "))
		a.AppendChild(t)
		div.AppendChild(a)
	}
	if at > 0 {
		add(s.order[at-1], "md2html-prev", "prev", "Previous")
	}
	if at < len(s.order)-1 {
		add(s.order[at+1], "md2html-next", "next", "Next")
	}
	return div
}

// contentMarkerWarning warns about a [[content]] in a page's own body, where
// it has no meaning: only the shell places the page.
func contentMarkerWarning(warn func(string)) Transform {
	return Transform{Name: "contentMarkerWarning", Fn: func(root *html.Node) error {
		for _, m := range findMarkers(root) {
			if m.name == "content" && warn != nil {
				warn("[[content]] belongs in the site's shell; leaving it as written")
			}
		}
		return nil
	}}
}
