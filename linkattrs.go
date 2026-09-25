package md2html

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// LinkAttrs applies Pandoc's link_attributes: a {#id .class key=value}
// block written directly after a link or an image sets those attributes on
// the <a> or <img>, and is removed from the text.
//
// goldmark has no parser for it, so the block reaches the tree as the
// start of the text node after the element. Working on the tree is also
// what lets an aria-* attribute through: goldmark's own attribute
// allowlist drops them, and this sets attributes directly.
//
// The block must touch the element, as in Pandoc; with a space between
// them it is prose. One holding no attributes, or a name that is not a
// safe attribute name, stays as literal text, as "[x]{}" does for a
// bracketed span.
//
// href and src are refused, with a warning. LinkRewrite looks links up by
// their target exactly as the source wrote it, so a block replacing one
// would route the link around the .md-to-.html rewrite. The block itself
// can never become part of that key: it is a separate text node, never in
// the attribute.
//
// warn may be nil.
func LinkAttrs(warn func(string)) Transform {
	if warn == nil {
		warn = func(string) {}
	}
	return Transform{Name: "linkAttrs", Fn: func(root *html.Node) error {
		// Collect, then mutate: applying a block edits the text node the
		// walk would visit next.
		var els []*html.Node
		walk(root, func(n *html.Node) {
			if n.Type == html.ElementNode && (n.DataAtom == atom.A || n.DataAtom == atom.Img) {
				els = append(els, n)
			}
		})
		for _, el := range els {
			applyLinkAttrs(el, warn)
		}
		return nil
	}}
}

// applyLinkAttrs reads the block after el, if there is one, and applies it.
func applyLinkAttrs(el *html.Node, warn func(string)) {
	text := el.NextSibling
	if text == nil || text.Type != html.TextNode || !strings.HasPrefix(text.Data, "{") {
		return
	}
	end := braceSpan(text.Data, 0)
	if end < 0 {
		return
	}
	a, ok := parseAttrs(text.Data[1:end])
	if !ok {
		return
	}

	// HTML attribute names are case-insensitive and x/net/html lowercases
	// the ones it parses, so a name is lowercased before it is checked or
	// set: HREF is refused like href, and Title replaces the link's title
	// rather than adding a second attribute the browser ignores.
	//
	// A block with any name that is not a safe attribute name is not an
	// attribute block, as in Pandoc, and stays as the text the author
	// wrote. Dropping only the bad names and removing the block would
	// silently delete prose that happens to touch a link, like
	// "[docs](d.md){{version}}".
	kv := make(map[string]string, len(a.kv))
	for k, v := range a.kv {
		if !safeAttrName(k) {
			return
		}
		kv[strings.ToLower(k)] = v
	}
	if v, ok := kv["id"]; ok {
		a.id = v
		delete(kv, "id")
	}
	if v, ok := kv["class"]; ok {
		a.classes = append(a.classes, strings.Fields(v)...)
		delete(kv, "class")
	}

	if a.id != "" {
		setAttr(el, "id", a.id)
	}
	if len(a.classes) > 0 {
		existing, _ := attr(el, "class")
		setAttr(el, "class", strings.Join(mergeTokens(existing, strings.Join(a.classes, " ")), " "))
	}
	for _, k := range sortedKeys(kv) {
		switch {
		case k == "href" || k == "src":
			warn(fmt.Sprintf("%s cannot be set from an attribute block on a %s; "+
				"write the target in the link itself", k, linkKind(el)))
		default:
			setAttr(el, k, kv[k])
		}
	}

	text.Data = text.Data[end+1:]
	if text.Data == "" {
		text.Parent.RemoveChild(text)
	}
}

// linkKind names el the way an author wrote it, for a warning.
func linkKind(el *html.Node) string {
	if el.DataAtom == atom.Img {
		return "image"
	}
	return "link"
}

// mergeTokens appends the whitespace-separated tokens of add to those of
// have, dropping repeats and keeping first-seen order.
func mergeTokens(have, add string) []string {
	seen := map[string]bool{}
	var out []string
	for _, tok := range strings.Fields(have + " " + add) {
		if !seen[tok] {
			seen[tok] = true
			out = append(out, tok)
		}
	}
	return out
}
