package md2html

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// splitFrontMatter peels a leading "---"-delimited block of flat
// "key: value" lines off the source, returning the keys and the remaining
// body. It returns (nil, src) unchanged when there is no such block.
//
// It is deliberately not YAML. The three keys the page can use are all flat
// strings, and taking on a YAML parser to read them would be the tool's
// first dependency for a feature this small. A block that is not flat
// key/value — a list, a nested map — is refused whole and rendered as it is
// today: an <hr> followed by a setext heading holding the lines, which is
// noisy above a title and therefore self-reporting.
func splitFrontMatter(src []byte) (map[string]string, []byte) {
	s := string(src)
	// A leading UTF-8 BOM makes this prefix check fail, so a document
	// saved with one simply never has its front matter detected — the
	// same as any other document with no leading "---". That degrades to
	// prior behavior rather than corrupting anything, so it is left alone
	// rather than stripped here.
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return nil, src
	}
	rest := s[strings.IndexByte(s, '\n')+1:]
	end := -1
	lines := strings.Split(rest, "\n")
	meta := map[string]string{}
	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if trimmed == "---" || trimmed == "..." {
			end = i
			break
		}
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		// Flat only: an indented line is a nested structure, and a line
		// with no colon is not a key at all.
		if trimmed != strings.TrimLeft(trimmed, " \t") {
			return nil, src
		}
		k, v, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, src
		}
		key := strings.TrimSpace(k)
		if key == "" {
			return nil, src
		}
		// A repeated key overwrites rather than erroring: last-wins is the
		// same rule a Go map assignment gives for free, and treating a
		// duplicate as malformed would make this stricter than the tool
		// needs to be for three flat keys.
		meta[strings.ToLower(key)] = strings.TrimSpace(v)
	}
	if end < 0 {
		// No closing delimiter: this was an <hr>, not front matter.
		return nil, src
	}
	body := strings.Join(lines[end+1:], "\n")
	return meta, []byte(strings.TrimLeft(body, "\n"))
}

// firstHeading returns the document's first <h1>, or nil.
func firstHeading(root *html.Node) *html.Node {
	var found *html.Node
	walk(root, func(n *html.Node) {
		if found == nil && n.Type == html.ElementNode && n.DataAtom == atom.H1 {
			found = n
		}
	})
	return found
}

// skipInert advances past HTML comments and whitespace-only text nodes to
// find the next node that could plausibly be meaningful structure. Goldmark
// emits a newline between every pair of top-level block elements, and
// parseFragment turns each into its own TextNode sibling; neither that nor
// a hand-written HTML comment carries any content of its own, so both
// liftSubtitle (looking for the paragraph right after the h1) and
// applyDocMeta (checking whether that paragraph already exists) need to
// see past them the same way.
func skipInert(n *html.Node) *html.Node {
	for n != nil && (n.Type == html.CommentNode ||
		(n.Type == html.TextNode && strings.TrimSpace(n.Data) == "")) {
		n = n.NextSibling
	}
	return n
}

// liftSubtitle promotes a line consisting of nothing but italic text,
// immediately following the leading h1, into a subtitle paragraph. It
// reports whether it fired.
//
// "Immediately following" is the next element sibling: blank lines leave no
// node, and an HTML comment — whether before the h1 (where the heading
// search simply skips over it while looking for the first h1) or between
// the h1 and the candidate paragraph (where the loop below must skip it
// explicitly) — does not block the lift. The paragraph must be entirely
// one <em> — emphasis at the *start* of a paragraph is prose, and lifting
// it would silently eat a line of the document.
func liftSubtitle(root *html.Node) bool {
	h := firstHeading(root)
	if h == nil {
		return false
	}
	p := skipInert(h.NextSibling)
	if p == nil || p.Type != html.ElementNode || p.DataAtom != atom.P {
		return false
	}
	em := p.FirstChild
	if em == nil || em.NextSibling != nil ||
		em.Type != html.ElementNode || em.DataAtom != atom.Em {
		return false
	}
	// Unwrap the <em>: the subtitle's styling is the stylesheet's job, and
	// leaving the italics would double up on it.
	p.RemoveChild(em)
	for c := em.FirstChild; c != nil; {
		next := c.NextSibling
		em.RemoveChild(c)
		p.AppendChild(c)
		c = next
	}
	setAttr(p, "class", "subtitle")
	return true
}

// applyDocMeta inserts a subtitle and a date under the document's leading
// h1 — or at the top of the document when it has no h1, which is the only
// other place they could go and still read as document metadata.
//
// Both insertions anchor off the same, updated position: after the
// subtitle paragraph is inserted, the anchor advances to it so the date
// lands after the subtitle rather than between the heading and it. That
// covers the front-matter-subtitle case, where the subtitle text arrives
// here and insert creates the paragraph itself. It does not cover
// liftSubtitle's case: when the lift already fired, the subtitle text
// passed in here is "" (front matter had none to give), so insert's own
// anchor advance never runs for it — the paragraph already exists, in
// place, right after the heading. Without the check below, a front-matter
// date would then be spliced in at the heading's old NextSibling, landing
// between the heading and the already-lifted subtitle. So the anchor is
// advanced past that paragraph up front, whether it is there or not.
func applyDocMeta(root *html.Node, subtitle, date string) {
	if subtitle == "" && date == "" {
		return
	}
	anchor := firstHeading(root)
	if anchor != nil {
		if n := skipInert(anchor.NextSibling); n != nil &&
			n.Type == html.ElementNode && n.DataAtom == atom.P && hasClass(n, "subtitle") {
			anchor = n
		}
	}
	insert := func(class, text string) {
		if text == "" {
			return
		}
		p := &html.Node{Type: html.ElementNode, DataAtom: atom.P, Data: "p",
			Attr: []html.Attribute{{Key: "class", Val: class}}}
		p.AppendChild(&html.Node{Type: html.TextNode, Data: text})
		if anchor == nil {
			root.InsertBefore(p, root.FirstChild)
		} else {
			anchor.Parent.InsertBefore(p, anchor.NextSibling)
		}
		anchor = p // keep date after subtitle, on both branches
	}
	insert("subtitle", subtitle)
	insert("docdate", date)
}
