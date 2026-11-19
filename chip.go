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
// A span's block may hold a quoted value, and a "}" inside one belongs to
// the value: [x]{title="a } b"} is one span, not a span with ` b"}` left
// after it. Only a quote straight after "=" opens a value. Anywhere else a
// quote is an ordinary character, so the apostrophe in title=don't, or in
// [x]{it's .c}, can neither cost the span nor run on into a later block.
//
// A block the quoted form cannot match — a stray quote, or a value whose
// quote never closes — is matched by the second form, which ends at the
// first "}" as every block once did. spanBlock then says what is wrong
// with it, rather than the text staying with no word.
var chipRe = regexp.MustCompile(
	`\[([^\]\n]+)\]\{((?:[^}"'\n]|="(?:[^"\\\n]|\\.)*"|='(?:[^'\\\n]|\\.)*')*)\}` +
		`|\[([^\]\n]+)\]\{([^}\n]*)\}` +
		`|\[(proven|verified|designed|planned|draft|deprecated|c:[^\]\n]{0,60})\]`)

// spanMatch reports the label and block of a chipRe match that is a
// bracketed span, from whichever of its two forms matched.
func spanMatch(s string, loc []int) (label, block string, ok bool) {
	switch {
	case loc[2] >= 0:
		return s[loc[2]:loc[3]], s[loc[4]:loc[5]], true
	case loc[6] >= 0:
		return s[loc[6]:loc[7]], s[loc[8]:loc[9]], true
	}
	return "", "", false
}

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
		warn(fmt.Sprintf("bracketed span [%s]: the value of %s has no closing quote; leaving the block as text", label, a.unclosed))
		return spanAttrs{}, false
	}
	keys := sortedKeys(a.kv)
	for _, k := range keys {
		if !safeAttrName(k) {
			hint := ""
			if strings.Contains(k, `"`) {
				// Markdown drops the backslash from \", so an escaped
				// quote ends its value early and the rest reads as a name.
				hint = "; a quote inside a value takes the other kind of quote"
			}
			warn(fmt.Sprintf("bracketed span [%s]: `%s` is not an attribute name; leaving the block as text%s", label, k, hint))
			return spanAttrs{}, false
		}
	}
	if len(a.bare) > 0 {
		k := a.bare[0]
		warn(fmt.Sprintf("bracketed span [%s]: %s has no value; write %s=\"\" for a boolean attribute, or the block stays as text", label, k, k))
		return spanAttrs{}, false
	}

	span = spanAttrs{id: a.id, classes: a.classes}
	// Walking the keys in sorted order makes the winner of two spellings
	// of one name, Title and title, the same on every run.
	kept := map[string]string{}
	for _, k := range keys {
		name := strings.ToLower(k)
		if strings.HasPrefix(name, "data-") || strings.HasPrefix(name, "aria-") ||
			gmhtml.GlobalAttributeFilter.Contains([]byte(name)) {
			kept[name] = a.kv[k]
		}
	}
	for _, name := range sortedKeys(kept) {
		span.extra = append(span.extra, html.Attribute{Key: name, Val: kept[name]})
	}
	return span, true
}

// spanNode builds the <span> for a bracketed span — Pandoc's
// bracketed_spans — from its label and the contents of its attribute
// block. It returns nil for a block spanBlock leaves literal, so
// "[thing]{}" stays as written rather than becoming an empty element.
//
// A chip earns its modifier class here rather than in the vocabulary
// regex: [proven]{.chip} carries chip-proven, while [proven]{.lead} is
// just a span the author classed themselves. The modifier is skipped when
// the author already wrote a chip-* class of their own, so
// [shipped]{.chip .chip-ok} keeps exactly the classes it names.
func spanNode(label, block string) []*html.Node {
	a, ok := spanBlock(label, block, nil)
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
	span.Attr = append(span.Attr, a.extra...)
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
		if label, block, ok := spanMatch(s, loc); ok {
			// Bracketed span: [label]{...}.
			if _, ok := spanBlock(label, block, warn); !ok {
				return nil
			}
			return spanNode(label, block)
		}
		token := s[loc[10]:loc[11]]
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
		if label, block, ok := spanMatch(s, loc); ok {
			// A bracketed span. Only a chip is a status marker that has no
			// business in a browser tab; any other span is ordinary prose
			// the author wrapped for styling, so its label survives with
			// only the syntax removed.
			//
			// A block the page leaves as text stays in the title as text
			// too, so the tab and the page say the same thing.
			a, isSpan := spanBlock(label, block, nil)
			if !isSpan {
				b.WriteString(s[loc[0]:loc[1]])
				continue
			}
			if !hasClassToken(a.classes, "chip") {
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
