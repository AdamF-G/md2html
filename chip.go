package md2html

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// chipRe matches the recognized bracket tokens.
//
// The status vocabulary is fixed and short on purpose: the alternative is
// treating every bracketed word as a badge, which would swallow reference
// link syntax, footnote-looking text, and any prose that happens to use
// brackets. The c: form is the escape hatch for a label outside the
// vocabulary — explicit, so it can never fire by accident. The label is
// capped and excludes newlines so a stray "[c:" cannot swallow a paragraph.
var chipRe = regexp.MustCompile(`\[(proven|verified|designed|planned|draft|deprecated|c:[^\]\n]{0,60})\]`)

// chipNodes splits s on chip tokens, returning the replacement nodes, or
// nil when s carries none — which is almost every text node in almost every
// document, so it is the cheap path.
//
// The match-splitting itself is splitMatches (Task 4): render is offered
// each match and returns nil to decline it (leaving the literal text, as
// the empty-label "[c:]" case does) or the nodes to substitute otherwise.
// Declining must be a real nil, never an empty non-nil slice — splitMatches
// treats any non-nil result, empty included, as "rendered" and deletes the
// matched text.
func chipNodes(s string) []*html.Node {
	return splitMatches(s, chipRe, func(loc []int) []*html.Node {
		token := s[loc[2]:loc[3]]
		class, label := "chip", token
		if strings.HasPrefix(token, "c:") {
			label = strings.TrimSpace(token[2:])
			if label == "" {
				// "[c:]" names nothing. An empty badge is worse than the
				// literal text, which at least shows the author what they
				// wrote.
				return nil
			}
		} else {
			class = "chip chip-" + token
		}
		span := &html.Node{Type: html.ElementNode, DataAtom: atom.Span, Data: "span",
			Attr: []html.Attribute{{Key: "class", Val: class}}}
		span.AppendChild(&html.Node{Type: html.TextNode, Data: label})
		return []*html.Node{span}
	})
}

// Chips turns a fixed set of bracketed tokens into small styled badges,
// everywhere inline Markdown is rendered — body text and headings alike,
// since a status marker on a heading is the case the convention exists for.
//
// A heading's marker is kept out of its slug by HeadingAnchors, which is
// why this transform must run before it.
func Chips() Transform {
	return Transform{Name: "chips", Fn: func(root *html.Node) error {
		rewriteText(root, chipNodes)
		return nil
	}}
}

// stripChipTokens removes chip tokens from a plain string.
//
// extractTitle runs before any transform — deliberately, so that
// HeadingAnchors' "#" never lands in a title — and therefore sees the raw
// bracketed source text rather than the spans Chips produces. Without this,
// a page titled "Rollback [proven]" would carry the marker in its browser
// tab and in every Artifact listing.
//
// A naive strings.Fields/Join cleanup would collapse every whitespace run
// in the string, not just the one a removed token left behind — turning
// "A  B [proven]" into "A B" instead of "A  B". So only the exact gap where
// a token sat is touched: when a space preceded the token and a space
// followed it, exactly one of the two is dropped so the pair doesn't turn
// into a double space. That drop is capped at a single space either way —
// eating every trailing space would itself violate the same rule it exists
// to enforce, turning "A [proven]  B" (one space before, two after) into
// "A B" and destroying a run of spaces the token never touched. A run of
// whitespace anywhere else in the string, including one the author typed on
// purpose, is left completely alone.
func stripChipTokens(s string) string {
	if !strings.ContainsRune(s, '[') {
		return s
	}
	locs := chipRe.FindAllStringIndex(s, -1)
	if locs == nil {
		return s
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		b.WriteString(s[last:loc[0]])
		last = loc[1]
		if strings.HasSuffix(b.String(), " ") && last < len(s) && s[last] == ' ' {
			last++ // merge the flanking single-space pair into one
		}
	}
	b.WriteString(s[last:])
	return strings.TrimSpace(b.String())
}

// headingText returns a heading's text with status chips and the anchor
// link left out, so a relabeled marker cannot change an anchor other
// documents link to, and the trailing "#" HeadingAnchors appends never
// shows up as part of the heading's own text (Tasks 6 and 7 both build on
// this, and would otherwise have to hand-strip a trailing "#" — silently
// truncating a heading whose visible text legitimately ends in one).
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
