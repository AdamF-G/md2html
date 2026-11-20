package md2html

import (
	"fmt"
	"regexp"
	"strings"

	gmhtml "github.com/yuin/goldmark/renderer/html"
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
//
// A bracketed span is not matched here: it is paired on goldmark's tree
// (spanTransformer), where its label can be Markdown and its brackets can
// nest or be escaped, and reaches Chips as an element carrying spanMarker.
// Matched here too, the pattern would make a span of text the tree had
// ruled out: [x\]{.c} renders as [x]{.c}.
var chipRe = regexp.MustCompile(`\[(proven|verified|designed|planned|draft|deprecated|c:[^\]\n]{0,60})\]`)

// statusWords is the closed vocabulary the bare form recognizes, and the
// set that gives a bracketed span its chip-<word> modifier class.
var statusWords = map[string]bool{
	"proven": true, "verified": true, "designed": true,
	"planned": true, "draft": true, "deprecated": true,
}

// spanAttrs is what a bracketed span's block sets on its <span>.
type spanAttrs struct {
	id      string
	classes []string
	// extra holds every other kept attribute, by lowercased name in
	// sorted order.
	extra []html.Attribute
	// tip is the block's tip="…", empty for none, and tipDelay its
	// tip-delay when that is a valid time.
	tip, tipDelay string
}

// spanBlock decides what a bracketed span's attribute block means. It is
// the one place that decides, so the page and its title — stripChipTokens
// reads the same text — cannot disagree about whether a block is a span.
//
// ok is false when the block stays on the page as the author wrote it.
// warn, which may be nil, then hears why when the block was clearly meant
// as attributes and has something the author can fix. It hears nothing
// when the block stays quietly: an empty block, or one of bare words
// alone, which is far more often prose, a regex or terminal markup than a
// span. Only the page's rewrite passes a warn; the title reads the same
// text again and stays silent, so one block warns once.
//
// A name a link block may set is kept here too, by the same rule: any of
// goldmark's global attributes, and any data- or aria- name, lowercased.
// Any other safe name, an event handler most obviously, is dropped while
// the rest of the block applies. A name that is not safe stays literal
// rather than dropped, as it does on a link: such a block is not an
// attribute block.
//
// A bare key, {.chip hidden}, keeps the block literal too. Pandoc has no
// bare-key form, and before spans kept attributes one silently ate the
// braces. A boolean attribute takes a value, hidden="".
func spanBlock(label, block string, warn func(string)) (span spanAttrs, ok bool) {
	return checkAttrBlock("bracketed span ["+label+"]", block, true, warn)
}

// checkAttrBlock is the one reading of an attribute block a span and a
// heading share: what spanBlock says above, for either. who names the
// element in a warning, `bracketed span [x]` or `heading "x"`. tips is
// false for a heading, which hosts no tip, so tip and tip-delay are
// dropped there as any name it does not take, without a word.
func checkAttrBlock(who, block string, tips bool, warn func(string)) (span spanAttrs, ok bool) {
	if warn == nil {
		warn = func(string) {}
	}
	a, found := parseAttrs(block)
	if !found {
		return spanAttrs{}, false
	}
	if a.id == "" && len(a.classes) == 0 && len(a.bare) == len(a.kv) {
		return spanAttrs{}, false
	}
	if a.unclosed != "" {
		warn(fmt.Sprintf("%s: the value of %s has no closing quote; leaving the block as text", who, a.unclosed))
		return spanAttrs{}, false
	}
	keys := sortedKeys(a.kv)
	for _, k := range keys {
		if !safeAttrName(k) {
			hint := ""
			if strings.Contains(k, `"`) {
				// A quote that ended its value early, title="a"b", leaves
				// the rest to read as a name.
				hint = "; a quote inside a double-quoted value is written \\\""
			}
			warn(fmt.Sprintf("%s: `%s` is not an attribute name; leaving the block as text%s", who, k, hint))
			return spanAttrs{}, false
		}
	}
	if len(a.bare) > 0 {
		k := a.bare[0]
		warn(fmt.Sprintf("%s: %s has no value; write %s=\"\" for a boolean attribute, or the block stays as text", who, k, k))
		return spanAttrs{}, false
	}

	span = spanAttrs{id: a.id, classes: a.classes}
	// Walking the keys in sorted order makes the winner of two spellings
	// of one name, Title and title, the same on every run.
	kept := map[string]string{}
	delay := ""
	for _, k := range keys {
		name := strings.ToLower(k)
		switch name {
		case "tip":
			if tips {
				span.tip = a.kv[k]
			}
			continue
		case "tip-delay":
			if tips {
				delay = a.kv[k]
			}
			continue
		}
		if strings.HasPrefix(name, "data-") || strings.HasPrefix(name, "aria-") ||
			gmhtml.GlobalAttributeFilter.Contains([]byte(name)) {
			kept[name] = a.kv[k]
		}
	}
	for _, name := range sortedKeys(kept) {
		span.extra = append(span.extra, html.Attribute{Key: name, Val: kept[name]})
	}
	switch {
	case delay == "":
	case span.tip == "":
		warn(fmt.Sprintf("%s: tip-delay without a tip does nothing", who))
	case !validTipDelay(delay):
		warn(fmt.Sprintf("%s: tip-delay %q is not a time such as 150ms; using the page's delay", who, delay))
	default:
		span.tipDelay = delay
	}
	return span, true
}

// applySpan gives el, a bracketed span — Pandoc's bracketed_spans — the
// attributes its block means, in place of its spanMarker.
//
// A chip earns its modifier class here rather than in the vocabulary
// regex: [proven]{.chip} carries chip-proven, while [proven]{.lead} is
// just a span the author classed themselves. The modifier is skipped when
// the author already wrote a chip-* class of their own, so
// [shipped]{.chip .chip-ok} keeps exactly the classes it names.
func applySpan(el *html.Node, label string, a spanAttrs) {
	classes := a.classes
	if statusWords[label] && hasClassToken(classes, "chip") && !hasClassTokenPrefix(classes, "chip-") {
		classes = append(classes, "chip-"+label)
	}
	removeAttr(el, spanMarker)
	if a.id != "" {
		el.Attr = append(el.Attr, html.Attribute{Key: "id", Val: a.id})
	}
	if len(classes) > 0 {
		el.Attr = append(el.Attr, html.Attribute{Key: "class", Val: strings.Join(classes, " ")})
	}
	el.Attr = append(el.Attr, a.extra...)
	if a.tip != "" {
		el.Attr = append(el.Attr, html.Attribute{Key: tipAttr, Val: a.tip})
		if a.tipDelay != "" {
			el.Attr = append(el.Attr, html.Attribute{Key: tipDelayAttr, Val: a.tipDelay})
		}
	}
}

// resolveSpans turns each element spanTransformer marked into the span its
// block means, or, for a block spanBlock refuses, back into the literal
// [label]{…} the author wrote, warning as spanBlock does. The innermost go
// first, so a span in a label is settled before the label is read.
func resolveSpans(root *html.Node, warn func(string)) {
	var marked []*html.Node
	walk(root, func(n *html.Node) {
		if n.Type == html.ElementNode {
			if _, ok := attr(n, spanMarker); ok {
				marked = append(marked, n)
			}
		}
	})
	for i := len(marked) - 1; i >= 0; i-- {
		el := marked[i]
		block, _ := attr(el, spanMarker)
		label := textOf(el)
		if a, ok := spanBlock(label, block[1:len(block)-1], warn); ok {
			applySpan(el, label, a)
			continue
		}
		p := el.Parent
		p.InsertBefore(textNode("["), el)
		for c := el.FirstChild; c != nil; {
			next := c.NextSibling
			el.RemoveChild(c)
			p.InsertBefore(c, el)
			c = next
		}
		p.InsertBefore(textNode("]"+block), el)
		p.RemoveChild(el)
	}
}

func hasClassToken(classes []string, want string) bool {
	for _, c := range classes {
		if c == want {
			return true
		}
	}
	return false
}

func hasClassTokenPrefix(classes []string, prefix string) bool {
	for _, c := range classes {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// chipNodes splits s on chip tokens, returning the replacement nodes, or
// nil when s carries none — which is almost every text node in almost every
// document, so it is the cheap path.
//
// The match-splitting itself is splitMatches: render is offered
// each match and returns nil to decline it (leaving the literal text, as
// the empty-label "[c:]" case does) or the nodes to substitute otherwise.
// Declining must be a real nil, never an empty non-nil slice — splitMatches
// treats any non-nil result, empty included, as "rendered" and deletes the
// matched text.
//
// warn, which may be nil, hears why a bracketed span's block was left as
// text, when spanBlock has a reason the author can act on.
func chipNodes(s string, warn func(string)) []*html.Node {
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
func Chips() Transform { return chips(nil) }

// chips is Chips with a warning for a bracketed span's block that is left
// on the page as text for a reason the author can fix. Only this rewrite
// warns: stripChipTokens reads a heading's text again for the page title,
// and staying silent there is what keeps one block to one warning.
func chips(warn func(string)) Transform {
	return Transform{Name: "chips", Fn: func(root *html.Node) error {
		rewriteText(root, func(s string) []*html.Node { return chipNodes(s, warn) })
		// After the bare badges, so a refused span's literal text is not
		// read again: [proven]{.chip hidden} stays as written, whole.
		resolveSpans(root, warn)
		if warn != nil {
			warnUnclaimedHeadingBlocks(root, warn)
		}
		return nil
	}}
}

// warnUnclaimedHeadingBlocks reports a heading that ends in a link's or an
// image's block LinkAttrs refused: it was never the heading's, so the fix
// is in the block. A block touching a "]" or a ")" that no span or link
// took at all is reported on goldmark's tree (spanTransformer), where an
// escaped brace is still escaped; this case needs LinkAttrs' verdict, so
// it waits until LinkAttrs has run, which is before Chips.
func warnUnclaimedHeadingBlocks(root *html.Node, warn func(string)) {
	walk(root, func(n *html.Node) {
		if n.Type != html.ElementNode || !isHeading(n) {
			return
		}
		last := n.LastChild
		if last == nil || last.Type != html.TextNode || !strings.HasPrefix(last.Data, "{") {
			return
		}
		prev := last.PrevSibling
		if prev == nil || prev.Type != html.ElementNode || (prev.DataAtom != atom.A && prev.DataAtom != atom.Img) {
			return
		}
		s := strings.TrimRight(last.Data, " \t")
		if braceSpan(s, 0) != len(s)-1 || !meantAsAttrs(s[1:len(s)-1]) {
			return
		}
		title := strings.TrimSpace(strings.TrimSuffix(textOf(n), s))
		warn(fmt.Sprintf("heading %q: %s after a link is not a block the link takes; fix the block, or write a space before it to give it to the heading", title, s))
	})
}

// meantAsAttrs reports whether block, the inside of a {…}, was clearly
// written as attributes: an id, a class or a key=value, not bare words
// alone, which is far more often a path or prose.
func meantAsAttrs(block string) bool {
	a, ok := parseAttrs(block)
	return ok && (a.id != "" || len(a.classes) > 0 || len(a.bare) < len(a.kv))
}

func isHeading(n *html.Node) bool {
	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		return true
	}
	return false
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
	locs := chipRe.FindAllStringSubmatchIndex(s, -1)
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
