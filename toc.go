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
// Each list item carries its heading's tag (toc-h2) and its depth in the
// page's outline below the title (toc-d0 for a top-level section), and the
// title's item is also marked toc-title, so the stylesheet can mark depth
// without the markup being a hierarchy.
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
	type entry struct {
		id, label, tag string
		level          int
	}
	var entries []entry
	for _, h := range heads {
		id, ok := attr(h, "id")
		if !ok || id == "" {
			continue // --no-anchors: nothing to link to
		}
		// headingText already excludes both the chip spans and the anchor
		// link HeadingAnchors appends (see chip.go), so there is no
		// trailing "#" to strip here — and no risk of truncating a heading
		// whose visible text genuinely ends in one, like "Sharp C#".
		text := strings.TrimSpace(headingText(h))
		if text == "" {
			continue
		}
		entries = append(entries, entry{id, text, h.Data, int(h.Data[1] - '0')})
	}
	if len(entries) == 0 {
		return nil
	}

	// The page title is the first entry when it is shallower than every
	// other: an h1 over h2 sections, or an h2 over h3 ones on a page that
	// titles itself with ##. It is marked, and left out of the outline, so
	// the sections under it are depth 0 just as on a page with no title.
	// Two headings at the top level, or a shallower one later, means the
	// page has no title in this sense and its top headings are sections.
	title := len(entries) > 1
	for _, e := range entries[1:] {
		if e.level <= entries[0].level {
			title = false
			break
		}
	}

	list := &html.Node{Type: html.ElementNode, DataAtom: atom.Ol, Data: "ol"}
	// open holds the levels of the listed headings still enclosing the
	// current one, shallowest first; its length is the current depth.
	var open []int
	for i, e := range entries {
		class := fmt.Sprintf("toc-%s toc-d0 toc-title", e.tag)
		if !title || i > 0 {
			// Depth is the entry's place in the outline, not its tag: one
			// more than the nearest listed heading above it with a higher
			// level. A level skipped for its look (h2 straight to h4) adds
			// no depth.
			for len(open) > 0 && open[len(open)-1] >= e.level {
				open = open[:len(open)-1]
			}
			class = fmt.Sprintf("toc-%s toc-d%d", e.tag, len(open))
			open = append(open, e.level)
		}
		li := &html.Node{Type: html.ElementNode, DataAtom: atom.Li, Data: "li",
			Attr: []html.Attribute{{Key: "class", Val: class}}}
		a := &html.Node{Type: html.ElementNode, DataAtom: atom.A, Data: "a",
			Attr: []html.Attribute{{Key: "href", Val: "#" + e.id}}}
		a.AppendChild(&html.Node{Type: html.TextNode, Data: e.label})
		li.AppendChild(a)
		list.AppendChild(li)
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
