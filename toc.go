package md2html

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// tocMarkers are the tokens a document may use to ask for a contents list,
// compared case-insensitively. Each has to be inert to every other Markdown
// renderer, so that a source file carrying one still reads correctly
// unprocessed; none is link syntax in CommonMark, so all render as literal
// text elsewhere.
//
// There is no single convention here, so md2html accepts the two that
// matter: "[[toc]]" is markdown-it and VitePress, and "[TOC]" is
// Python-Markdown and therefore MkDocs, as well as Typora and StackEdit.
// Documents are written for one or the other, and rejecting either would
// be an arbitrary tax on whichever the author already knows.
//
// GitLab's "[[_TOC_]]" is absent on purpose: its underscores are emphasis
// delimiters, so it never reaches this transform as a single text node.
// See TestTOCGitLabMarkerIsNotSupported.
var tocMarkers = map[string]bool{
	"[[toc]]": true,
	"[toc]":   true,
}

// tocLabel is the contents list's accessible name. It is a label rather
// than a visible heading: a heading would change the look of every page
// that has a contents list, and would itself take a slug, an anchor and an
// entry in the list it introduces. A page in another language renames it
// with the toc-title front matter key, which Convert passes to tocWith.
const tocLabel = "Table of Contents"

// isTOCMarker reports whether s, already trimmed, is one of them.
func isTOCMarker(s string) bool {
	return tocMarkers[strings.ToLower(s)]
}

// TOC replaces a marker paragraph with a flat list of the current
// document's own headings.
//
// Flat, not nested by heading level: a document that jumps h2 to h4 would
// otherwise produce either invalid list nesting or a silently wrong tree.
// Level travels as a class on the list item, so the stylesheet can indent
// without the markup having to be a hierarchy.
//
// This is a table of contents for one page and nothing more. Cross-document
// navigation, a site sidebar and a site index stay out of scope — see
// docs/specs/2026-09-11-extended-content-model.md, item 6. Float mode
// (tocWith) changes only where this same list sits on the screen.
func TOC() Transform { return tocWith(tocLabel, false) }

// tocWith is TOC with its list named label and, when float is set, its
// first list marked toc-float for the stylesheet to pin beside the text
// column. Convert rebuilds the TOC in a transform list with a page's
// toc-title and layout (withTOC), the way it rebuilds a warning builtin
// with Options.Warn, so both reach the list TOC builds and nowhere else.
//
// Only the first list floats: a fixed position holds one box, and a second
// floating list would sit on top of the first.
func tocWith(label string, float bool) Transform {
	return Transform{Name: "toc", Fn: func(root *html.Node) error {
		var markers []*html.Node
		walk(root, func(n *html.Node) {
			if n.Type != html.ElementNode || n.DataAtom != atom.P {
				return
			}
			// Alone on its own line: the marker must be the paragraph's
			// entire content, structurally as well as textually. Matching
			// on flattened text (textOf) would also match a real link or
			// code span whose visible text happens to be "[[toc]]" —
			// e.g. "[[[toc]]](http://example.com)" — and eat it along
			// with its href. Requiring the paragraph's one and only child
			// to be a text node rules that out: an <a> or <code> wrapping
			// the marker is an element child, not a text child, so it is
			// left as prose. A marker inside a sentence is likewise
			// excluded (extra text-node siblings), and one inside a fence
			// is this feature's own documentation — the fence is a <pre>,
			// never a <p>, so it is excluded by construction.
			if n.FirstChild != nil && n.FirstChild == n.LastChild &&
				n.FirstChild.Type == html.TextNode &&
				isTOCMarker(strings.TrimSpace(n.FirstChild.Data)) {
				markers = append(markers, n)
			}
		})
		if len(markers) == 0 {
			return nil
		}
		heads := headingNodes(root)
		for i, m := range markers {
			nav := buildTOC(heads, label, float && i == 0)
			if nav == nil {
				// Nothing to list. Leave the marker as literal text rather
				// than remove it: the spec's own degradation for this
				// feature is "ugly, but not misleading" — deleting the
				// marker would erase the reader's only clue that the
				// document has no linkable headings.
				continue
			}
			m.Parent.InsertBefore(nav, m)
			m.Parent.RemoveChild(m)
		}
		return nil
	}}
}

// buildTOC renders the nav, or nil when there is nothing to list — a
// document whose marker has no headings with an id (none present, or
// --no-anchors suppressed every one) to point at.
func buildTOC(heads []*html.Node, label string, float bool) *html.Node {
	list := &html.Node{Type: html.ElementNode, DataAtom: atom.Ol, Data: "ol"}
	n := 0
	for _, h := range heads {
		id, ok := attr(h, "id")
		if !ok || id == "" {
			continue // --no-anchors: nothing to link to
		}
		// headingText already excludes both the chip spans and the anchor
		// link HeadingAnchors appends (see chip.go), so there is no
		// trailing "#" to strip here — and no risk of truncating a heading
		// whose visible text genuinely ends in one, like "Sharp C#".
		label := strings.TrimSpace(headingText(h))
		if label == "" {
			continue
		}
		li := &html.Node{Type: html.ElementNode, DataAtom: atom.Li, Data: "li",
			Attr: []html.Attribute{{Key: "class", Val: "toc-" + h.Data}}}
		a := &html.Node{Type: html.ElementNode, DataAtom: atom.A, Data: "a",
			Attr: []html.Attribute{{Key: "href", Val: "#" + id}}}
		a.AppendChild(&html.Node{Type: html.TextNode, Data: label})
		li.AppendChild(a)
		list.AppendChild(li)
		n++
	}
	if n == 0 {
		return nil
	}
	class := "toc"
	if float {
		class = "toc toc-float"
	}
	nav := &html.Node{Type: html.ElementNode, DataAtom: atom.Nav, Data: "nav",
		Attr: []html.Attribute{{Key: "class", Val: class}, {Key: "aria-label", Val: label}}}
	nav.AppendChild(list)
	return nav
}

// withTOC returns ts with its TOC rebuilt to name its list label (the
// default when empty) and float its first list, or ts itself when neither
// changes anything. Like withWarn, it never writes to ts.
func withTOC(ts []Transform, label string, float bool) []Transform {
	if label == "" && !float {
		return ts
	}
	if label == "" {
		label = tocLabel
	}
	out, copied := ts, false
	for i, t := range ts {
		if t.Name != "toc" {
			continue
		}
		if !copied {
			out = append([]Transform(nil), ts...)
			copied = true
		}
		out[i] = tocWith(label, float)
	}
	return out
}

// IsTOCMode reports whether s names a contents list layout: "inline" or
// "float".
func IsTOCMode(s string) bool { return s == "inline" || s == "float" }

// tocFloats resolves a page's contents list layout from its front matter,
// then the run-wide option, then the inline default — the order pageLang
// follows. A value naming no layout is warned about and skipped.
func tocFloats(front, opt string, warn func(string)) bool {
	for _, c := range []struct{ val, from string }{
		{front, "front matter toc"},
		{opt, "TOC option"},
	} {
		if c.val == "" {
			continue
		}
		if IsTOCMode(c.val) {
			return c.val == "float"
		}
		if warn != nil {
			warn(fmt.Sprintf("%s %q is not a contents list layout (inline or float); ignoring it", c.from, c.val))
		}
	}
	return false
}
