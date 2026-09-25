package md2html

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Builtins returns the transforms enabled by default.
//
// It takes no arguments and reports nothing: it is the documented public
// door for callers assembling their own transform list (see README), and
// changing its signature would break them. A list built from it still
// reports — Convert rebuilds the entries in warnAware against Options.Warn
// before running them — so the nil sink here costs a caller nothing.
func Builtins() []Transform {
	return builtins(nil)
}

// builtins is the ordered default list. Containers must run first because
// it restructures fenced containers before anything else inspects the tree.
// Alerts follows it as the other transform that produces containers, and
// must precede Chips so a marker's brackets are consumed before
// bracketed-span rewriting could read them. A container's own fence line no
// longer needs that protection: the fence parser consumes it, so
// ":::aside[Why]{.compact}" never reaches the tree as text a bracketed span
// could claim.
// LinkAttrs runs before ExternalLinks, which adds to the rel an attribute
// block may have set rather than replacing it, and before LinkRewrite,
// which Convert appends last and which must see each href as the source
// wrote it; a block cannot change an href, so the order only matters for
// rel.
// Chips must run before HeadingAnchors so a status marker is already a
// <span class="chip"> — and therefore excluded by headingText — by the time
// slugs are computed; SectionLinks and TOC must both run after
// HeadingAnchors because they resolve against the ids it assigns ("§4.2"
// for SectionLinks, every heading's anchor for TOC), and TOC must also run
// after Chips so its link text is the same label the reader sees in the
// heading, chip-free. Nothing else among the rest depends on running
// before or after another today, but the list's order is a correctness
// constraint, not a style choice: later tasks insert their transforms at
// specific positions in it (id-resolving transforms after anchors are
// assigned), so new entries belong at their documented position, not
// appended to the end. warn is threaded through so a transform can report
// a non-fatal problem; Containers and LinkAttrs use it so far.
func builtins(warn func(string)) []Transform {
	return []Transform{
		Containers(warn),
		Alerts(),
		LinkAttrs(warn),
		TableScroll(),
		Chips(),
		HeadingAnchors(),
		SectionLinks(),
		TOC(),
		ExternalLinks(),
	}
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
//
// The rules are the ones GitHub, GitLab and Pandoc's commonmark_x share, so
// an in-page link written against any of them resolves here too: lowercase,
// drop everything but letters, numbers, marks, hyphens and underscores, and
// turn each whitespace rune into a hyphen. Nothing is merged or trimmed
// after that, so "a × b" is "a--b" and "— Intro" is "-intro".
func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r), unicode.IsMark(r), r == '-', r == '_':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteByte('-')
		}
	}
	return b.String()
}

// HeadingAnchors gives every heading a stable id and a linkable anchor.
// An id already present — from a {#custom-id} attribute — is left alone.
//
// Status chips are excluded from the slug: a marker is metadata about the
// section, not part of its name, and relabeling one later must not rot an
// anchor other documents already link to.
func HeadingAnchors() Transform {
	return Transform{Name: "headingAnchors", Fn: func(root *html.Node) error {
		seen := map[string]bool{}
		heads := headingNodes(root)
		// Reserve every id already in the document before assigning any
		// generated one, or a slug could claim a string an explicit id is
		// using, and the two would collide. That means every element's, not
		// only a heading's: a link, a span or a container can carry an id
		// too. An explicit id always wins and is never rewritten; two
		// explicit ids that collide with each other are left exactly as
		// written.
		walk(root, func(n *html.Node) {
			if n.Type != html.ElementNode {
				return
			}
			if id, ok := attr(n, "id"); ok && id != "" {
				seen[id] = true
			}
		})
		for i, h := range heads {
			id, ok := attr(h, "id")
			if !ok || id == "" {
				base := slugify(headingText(h))
				if base == "" {
					// Nothing to derive a slug from — a heading of only
					// punctuation, or one holding just an image. Fall back to
					// position so it is still linkable. Positional ids shift
					// when headings are inserted above, so this stays a last
					// resort rather than the general rule.
					base = fmt.Sprintf("section-%d", i+1)
				}
				// Repeats are numbered from 1, as GitHub, GitLab and Pandoc
				// number them: the second "Setup" is "setup-1".
				id = base
				for n := 1; seen[id]; n++ {
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
// leaking the referring window. rel="noopener noreferrer" is added to any
// rel tokens the link already has, and a target the author chose is left
// alone.
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
			// An author's own target and rel tokens are kept: a link
			// attribute block or raw HTML may carry rel="me", and
			// overwriting it would drop a relation the author declared.
			if _, ok := attr(n, "target"); !ok {
				setAttr(n, "target", "_blank")
			}
			rel, _ := attr(n, "rel")
			setAttr(n, "rel", strings.Join(mergeTokens(rel, "noopener noreferrer"), " "))
		})
		return nil
	}}
}

// warnAware lists the builtins that report non-fatal problems, keyed by the
// Name their constructor stamps on them. It exists so Options.Warn is the
// single place a caller names a diagnostic sink: Convert rebuilds these
// entries against that sink in whatever transform list it is handed, so a
// caller who assembled their own list from Builtins() — the documented way
// to add a transform — does not have to know which entries take one.
//
// A transform that starts reporting belongs here as well as in builtins,
// or its warnings reach only callers who passed no list of their own.
var warnAware = map[string]func(func(string)) Transform{
	"containers": Containers,
	"linkAttrs":  LinkAttrs,
}

// withWarn returns ts with every warn-aware builtin rebuilt against warn.
//
// ts is never written to. A transform list is a value a caller may hold and
// reuse across documents, and the CLI converts documents in parallel from
// one list per goroutine; mutating an entry in place would repoint a
// sink at whichever document happened to be converted last.
func withWarn(ts []Transform, warn func(string)) []Transform {
	out, copied := ts, false
	for i, t := range ts {
		ctor, ok := warnAware[t.Name]
		if !ok {
			continue
		}
		if !copied {
			out = append([]Transform(nil), ts...)
			copied = true
		}
		out[i] = ctor(warn)
	}
	return out
}
