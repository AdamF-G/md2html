package md2html

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

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
			case "search":
				replaceMarker(m, s.searchBox(from))
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
		// The root page has no section, and a top-level page of its own has
		// nothing below it: either would list nothing, so the whole tree
		// stands in, and a section sidebar is never blank.
		if start == nil || !hasShownChild(start) {
			start = s.root
		}
	case "page":
		// The current page's own node: a directory's page lists what is
		// in it, a leaf lists nothing.
		start = page
		if start == nil {
			start = s.root
		}
	default:
		say("[[pages]] from %q is not root, section or page; listing from the root", v)
	}
	collapse := false
	switch v := m.attrs["collapse"]; v {
	case "", "false":
	case "true":
		collapse = true
	default:
		say("[[pages]] collapse %q is not true or false; leaving every branch open", v)
	}
	right := false
	switch v := m.attrs["chevron"]; v {
	case "", "left":
	case "right":
		right = true
	default:
		say("[[pages]] chevron %q is not left or right; putting chevrons on the left", v)
	}
	ul := s.pagesLevel(start, page, from, depth, collapse)
	// The outer list says it folds, and which side its chevrons go, so a
	// style can lay out every level from there.
	if ul != nil && collapse {
		class := "md2html-pages md2html-collapse"
		if right {
			class += " md2html-chevron-right"
		}
		setAttr(ul, "class", class)
	}
	return ul
}

// hasShownChild reports whether n has a child [[pages]] would list.
func hasShownChild(n *siteNode) bool {
	for _, c := range n.children {
		if !c.hidden {
			return true
		}
	}
	return false
}

// pagesLevel lists n's shown children, and below each its own, to depth
// levels. With collapse, a child's list folds into a <details> beside its
// link, open when it holds page. The link stays outside the summary, so a
// reader can follow it or open the branch, and the summary, which holds
// only the toggle, is named for the page it opens.
func (s *Site) pagesLevel(n, page *siteNode, from string, depth int, collapse bool) *html.Node {
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
		var class []string
		if contains(c, page) {
			class = append(class, "md2html-branch")
		}
		li.AppendChild(s.nodeLink(c, page, from))
		if sub := s.pagesLevel(c, page, from, depth-1, collapse); sub != nil && collapse {
			class = append(class, "md2html-fold")
			details := elem("details")
			if contains(c, page) {
				details.Attr = append(details.Attr, html.Attribute{Key: "open"})
			}
			details.AppendChild(elem("summary", "aria-label", c.label))
			details.AppendChild(sub)
			li.AppendChild(details)
		} else if sub != nil {
			li.AppendChild(sub)
		}
		if class != nil {
			li.Attr = append(li.Attr, html.Attribute{Key: "class", Val: strings.Join(class, " ")})
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
	// A trail of one entry is the page alone, as on the root page: it says
	// nothing, so nothing is drawn.
	if ol.FirstChild == ol.LastChild {
		return nil
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

// contentMarkerWarning warns about a [[content]] or [[mount]] in a page's own
// body, where it has no meaning: only the shell places the page, and only
// the shell's mounts make the site.
func contentMarkerWarning(warn func(string)) Transform {
	return Transform{Name: "contentMarkerWarning", Fn: func(root *html.Node) error {
		for _, m := range findMarkers(root) {
			if (m.name == "content" || m.name == "mount") && warn != nil {
				warn(fmt.Sprintf("[[%s]] belongs in the site's shell; leaving it as written", m.name))
			}
		}
		return nil
	}}
}
