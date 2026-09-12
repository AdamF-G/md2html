package md2html

import (
	"regexp"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// inlineSkip reports whether a node's whole subtree is off-limits to inline
// rewriting.
//
// Code is skipped because a token inside a code span or a code block is
// being quoted, not used — `[proven]` in a code span is documentation of
// the syntax, and rewriting it would make the syntax undocumentable. Links
// are skipped because every inline rewriter here produces either a link or
// a badge, and neither may nest inside an <a>. Script and style are skipped
// because their text is not prose at all.
func inlineSkip(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	switch n.DataAtom {
	case atom.Code, atom.Pre, atom.A, atom.Script, atom.Style, atom.Kbd, atom.Samp:
		return true
	}
	return hasClass(n, "chip")
}

// rewriteText offers every prose text node under root to fn, replacing the
// node with fn's result when it returns one and leaving it alone when fn
// returns nil.
//
// Nodes fn produces are never offered back to fn: the next sibling is
// captured before the replacement happens, so the cursor is already past
// everything inserted. Without that, a rewriter emitting text that contains
// its own trigger would not terminate.
func rewriteText(root *html.Node, fn func(string) []*html.Node) {
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			switch {
			case c.Type == html.TextNode:
				if repl := fn(c.Data); repl != nil {
					for _, r := range repl {
						n.InsertBefore(r, c)
					}
					n.RemoveChild(c)
				}
			case inlineSkip(c):
				// Leave the whole subtree alone.
			default:
				visit(c)
			}
			c = next
		}
	}
	visit(root)
}

// splitMatches finds every match of re in s and offers each one to render,
// which sees the match's full submatch index slice (as
// regexp.FindAllStringSubmatchIndex returns it) so it can inspect captured
// groups. A match render declines — by returning nil — is left as literal
// text; this is how the §-autolinker (Task 6) leaves a section number alone
// when it names no heading, without needing its own copy of this loop.
//
// The text between rendered matches, including any run render declined,
// becomes plain text nodes; no empty text node is ever emitted, so adjacent
// matches with nothing between them produce no spurious gap. splitMatches
// returns nil when every match was declined (or there were none at all),
// so its result can be passed straight to rewriteText's fn and get
// "nothing changed, node left alone" for free.
func splitMatches(s string, re *regexp.Regexp, render func(loc []int) []*html.Node) []*html.Node {
	locs := re.FindAllStringSubmatchIndex(s, -1)
	var out []*html.Node
	last := 0
	changed := false
	for _, loc := range locs {
		rendered := render(loc)
		if rendered == nil {
			continue // declined: leave this occurrence as literal text
		}
		changed = true
		if loc[0] > last {
			out = append(out, &html.Node{Type: html.TextNode, Data: s[last:loc[0]]})
		}
		out = append(out, rendered...)
		last = loc[1]
	}
	if !changed {
		// Nothing rendered — including the no-match case — so the whole
		// string, declined runs included, is left as the original text.
		return nil
	}
	if last < len(s) {
		out = append(out, &html.Node{Type: html.TextNode, Data: s[last:]})
	}
	return out
}
