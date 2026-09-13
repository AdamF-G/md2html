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
var chipRe = regexp.MustCompile(
	`\[([^\]\n]+)\]\{([^}\n]*)\}` +
		`|\[(proven|verified|designed|planned|draft|deprecated|c:[^\]\n]{0,60})\]`)

// statusWords is the closed vocabulary the bare form recognizes, and the
// set that gives a bracketed span its chip-<word> modifier class.
var statusWords = map[string]bool{
	"proven": true, "verified": true, "designed": true,
	"planned": true, "draft": true, "deprecated": true,
}

// spanNode builds the <span> for a bracketed span — Pandoc's
// bracketed_spans — from its label and the contents of its attribute
// block. It returns nil for a block that holds no attributes, so
// "[thing]{}" stays literal rather than becoming an empty element.
//
// A chip earns its modifier class here rather than in the vocabulary
// regex: [proven]{.chip} carries chip-proven, while [proven]{.lead} is
// just a span the author classed themselves. The modifier is skipped when
// the author already wrote a chip-* class of their own, so
// [shipped]{.chip .chip-ok} keeps exactly the classes it names.
func spanNode(label, block string) []*html.Node {
	a, ok := parseAttrs(block)
	if !ok {
		return nil
	}
	classes := a.classes
	if statusWords[label] && hasClassToken(classes, "chip") && !hasClassTokenPrefix(classes, "chip-") {
		classes = append(classes, "chip-"+label)
	}
	span := &html.Node{Type: html.ElementNode, DataAtom: atom.Span, Data: "span"}
	if a.id != "" {
		span.Attr = append(span.Attr, html.Attribute{Key: "id", Val: a.id})
	}
	if len(classes) > 0 {
		span.Attr = append(span.Attr, html.Attribute{Key: "class", Val: strings.Join(classes, " ")})
	}
	span.AppendChild(&html.Node{Type: html.TextNode, Data: label})
	return []*html.Node{span}
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
// The match-splitting itself is splitMatches (Task 4): render is offered
// each match and returns nil to decline it (leaving the literal text, as
// the empty-label "[c:]" case does) or the nodes to substitute otherwise.
// Declining must be a real nil, never an empty non-nil slice — splitMatches
// treats any non-nil result, empty included, as "rendered" and deletes the
// matched text.
func chipNodes(s string) []*html.Node {
	return splitMatches(s, chipRe, func(loc []int) []*html.Node {
		if loc[2] >= 0 {
			// Bracketed span: [label]{...}.
			return spanNode(s[loc[2]:loc[3]], s[loc[4]:loc[5]])
		}
		token := s[loc[6]:loc[7]]
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
	locs := chipRe.FindAllStringSubmatchIndex(s, -1)
	if locs == nil {
		return s
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		b.WriteString(s[last:loc[0]])
		last = loc[1]
		if loc[2] >= 0 {
			// A bracketed span. Only a chip is a status marker that has no
			// business in a browser tab; any other span is ordinary prose
			// the author wrapped for styling, so its label survives with
			// only the syntax removed.
			label, block := s[loc[2]:loc[3]], s[loc[4]:loc[5]]
			if a, ok := parseAttrs(block); !ok || !hasClassToken(a.classes, "chip") {
				b.WriteString(label)
				continue
			}
		}
		if strings.HasSuffix(b.String(), " ") && last < len(s) && s[last] == ' ' {
			last++ // merge the flanking single-space pair into one
		}
	}
	b.WriteString(s[last:])
	return strings.TrimSpace(b.String())
}
