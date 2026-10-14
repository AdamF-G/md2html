package md2html

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
)

// wrap returns the shell, rendered for the page at src, with the page's
// body moved into its [[content]], which becomes the page's
// <main class="md2html">. content is the page's tree after every transform;
// transforms are the caller's list as given in Options.Transforms (nil for
// the default list), which the shell gets too, without the page's list
// markers, contents list or link rewrite. The shell is parsed afresh per
// page because its links and markers differ from page to page.
//
// What goes wrong in the shell's own source is the shell's to report, not
// the page's: its parse, its list markers and every transform over it warn
// through shellWarn, once per run, under the shell's path. warn is the
// page's sink, for warnings about the page itself.
//
// Order matters. The shell's links are rewritten first, for this page,
// before anything else adds links. Its [[toc]] markers are set aside, so
// the page's toc transform cannot list the shell's own headings. Its
// markers expand next, then the page's transforms run over the shell alone.
// Then any shell id the page also uses is renumbered, and only after that
// do the contents lists go in, since their links point into the page and
// must not be renumbered with the shell's. The page's body goes in last, so
// nothing is transformed twice. Contents lists under a shell are always
// inline: the shell places them.
func (s *Site) wrap(content *html.Node, src string, transforms []Transform, label string, warn func(string)) (*html.Node, error) {
	var buf bytes.Buffer
	if transforms == nil {
		transforms = builtins(s.shellWarn)
	} else {
		transforms = withWarn(transforms, s.shellWarn)
	}
	if err := newParser(s.shellWarn, chartContext{off: true}).Convert(s.shellBody, &buf); err != nil {
		return nil, err
	}
	shell, err := parseFragment(buf.Bytes())
	if err != nil {
		return nil, err
	}
	out := s.outBySrc[src]
	links := ExtractLinks(shell, s.shellDir)
	mapped := linkMapFor(links, out, s.outBySrc, s.crawl)
	if err := LinkRewrite(mapped).Fn(shell); err != nil {
		return nil, err
	}
	s.rebaseShellLinks(links, mapped, out)
	if label == "" {
		label = tocLabel
	}
	// An element no transform knows holds each [[toc]]'s place.
	var tocSlots []*html.Node
	for _, m := range findTOCMarkers(shell) {
		slot := elem("md2html-toc-slot")
		m.Parent.InsertBefore(slot, m)
		m.Parent.RemoveChild(m)
		tocSlots = append(tocSlots, slot)
	}
	ts := append([]Transform{listMarkers(s.shellMeta, s.shellDir, s.shellWarn), s.navMarkers(src, s.shellWarn)}, transforms...)
	for _, t := range ts {
		if err := t.Fn(shell); err != nil {
			return nil, fmt.Errorf("site shell: transform %s: %w", t.Name, err)
		}
	}
	renumberShellIDs(shell, content)
	for _, slot := range tocSlots {
		if nav := buildTOC(headingNodes(content), label, false); nav != nil {
			slot.Parent.InsertBefore(nav, slot)
		}
		slot.Parent.RemoveChild(slot)
	}
	for _, m := range findMarkers(shell) {
		if m.name != "content" {
			continue
		}
		main := elem("main", "class", "md2html")
		for c := content.FirstChild; c != nil; {
			next := c.NextSibling
			content.RemoveChild(c)
			main.AppendChild(c)
			c = next
		}
		replaceMarker(m, main)
	}
	return shell, nil
}

// rebaseShellLinks points each relative link the link map left alone at
// its target from the page written at out. Written in the shell, it is
// relative to the shell's directory, which is no page's: in place an asset
// is not mapped, nor is a document the build does not emit, nor anything
// under --no-md-links or --no-assets. A link to a document the build does
// not emit is said once, since it reaches the Markdown file, not a page.
func (s *Site) rebaseShellLinks(links []Link, mapped map[string]string, out string) {
	for _, l := range links {
		if l.Kind != LinkDoc && l.Kind != LinkAsset {
			continue
		}
		if _, ok := mapped[l.Href]; ok || hasScheme(l.Href) {
			continue
		}
		rel, err := filepath.Rel(filepath.Dir(out), l.Abs)
		if err != nil {
			continue
		}
		suffix := ""
		if i := strings.IndexAny(l.Href, "?#"); i >= 0 {
			suffix = l.Href[i:]
		}
		setAttr(l.Node, l.Attr, encodePath(filepath.ToSlash(rel))+suffix)
		if l.Kind == LinkDoc && !s.crawl.NoMdLinks {
			s.shellWarn(fmt.Sprintf("links to %s, which the build does not write; the link points at the Markdown file", l.Href))
		}
	}
}

// renumberShellIDs keeps the page's ids and renumbers any shell id the page
// also uses, as HeadingAnchors numbers a repeated heading, then points the
// shell's own "#id" links at the new ids. The page wins because other pages
// link to its ids; nothing outside links into the shell.
func renumberShellIDs(shell, content *html.Node) {
	ids := func(root *html.Node) map[string]bool {
		seen := map[string]bool{}
		walk(root, func(n *html.Node) {
			if n.Type == html.ElementNode {
				if id, ok := attr(n, "id"); ok && id != "" {
					seen[id] = true
				}
			}
		})
		return seen
	}
	page, used := ids(content), ids(shell)
	for id := range page {
		used[id] = true
	}
	renamed := map[string]string{}
	walk(shell, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		id, ok := attr(n, "id")
		if !ok || !page[id] {
			return
		}
		if to, done := renamed[id]; done {
			// The shell repeats its own id. HeadingAnchors leaves that as
			// written, so the copies stay one id, but it is the new one.
			setAttr(n, "id", to)
			return
		}
		next := id
		for i := 1; used[next]; i++ {
			next = fmt.Sprintf("%s-%d", id, i)
		}
		used[next] = true
		renamed[id] = next
		setAttr(n, "id", next)
	})
	if len(renamed) == 0 {
		return
	}
	walk(shell, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		for i, a := range n.Attr {
			if a.Key == "href" && strings.HasPrefix(a.Val, "#") {
				if to, ok := renamed[a.Val[1:]]; ok {
					n.Attr[i].Val = "#" + to
				}
			}
		}
	})
}
