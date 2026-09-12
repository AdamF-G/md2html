package md2html

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// containerKind describes one shipped container kind: the element it
// becomes, the classes it carries, and — for the collapsible kinds — how a
// title is presented in its summary line.
type containerKind struct {
	// tag is "div" or "details".
	tag string
	// class is the kind's own class or classes, prepended to whatever the
	// author already wrote (see applyKind).
	class string
	// prefix labels a collapsible kind's summary ahead of any title.
	prefix string
	// fallback is the summary text when the author gave no title.
	fallback string
}

// containerKinds is the shipped vocabulary: a fixed set, not an open one.
// A class outside it stays exactly as inert as it is today, because a team
// that wrote one chose it deliberately and ships its own CSS; a bare name
// outside it is a typo, and saying so is the whole point — the silent
// unclassed div is the failure this replaces.
var containerKinds = map[string]containerKind{
	"callout": {tag: "div", class: "callout"},
	"warning": {tag: "div", class: "callout callout-warning"},
	"card":    {tag: "div", class: "card"},
	"aside":   {tag: "details", class: "container aside", fallback: "Aside"},
	"example": {tag: "details", class: "container example", prefix: "Example"},
}

// firstWord returns the leading whitespace-delimited word of s and the
// remainder with the separating spaces removed. A newline ends the word but
// is kept in the remainder: for a bare container the newline is the end of
// the opening fence line, and Task 3's title lifting needs to see it.
func firstWord(s string) (word, rest string) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	j := i
	for j < len(s) && s[j] != ' ' && s[j] != '\t' && s[j] != '\n' {
		j++
	}
	word = s[i:j]
	for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
		j++
	}
	return word, s[j:]
}

// fenceDivs collects every container the fence extension produced, before
// any of them is modified. Collect-then-mutate, the same discipline
// TableScroll uses: the transform replaces nodes, and a walk that is also
// rewriting the tree it walks is where subtle ordering bugs live.
func fenceDivs(root *html.Node) []*html.Node {
	var out []*html.Node
	walk(root, func(n *html.Node) {
		if n.Type != html.ElementNode || n.DataAtom != atom.Div {
			return
		}
		if _, ok := attr(n, "data-fence"); ok {
			out = append(out, n)
		}
	})
	return out
}

// removeAttr deletes an attribute if present.
func removeAttr(n *html.Node, key string) {
	for i, a := range n.Attr {
		if a.Key == key {
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			return
		}
	}
}

// removeClassToken drops one token from n's class attribute. It is used to
// remove the kind name itself from a braced container like {.warning} or
// {#note .callout .compact} before applyKind merges in the kind's real
// classes — the matched token is an alias for what k.class already spells
// out in full, not an independent class the author also wants, so keeping
// it verbatim would leave a redundant token (e.g. "callout callout-warning
// warning") rather than the normalized result.
func removeClassToken(n *html.Node, tok string) {
	v, ok := attr(n, "class")
	if !ok {
		return
	}
	kept := make([]string, 0, len(strings.Fields(v)))
	for _, f := range strings.Fields(v) {
		if f != tok {
			kept = append(kept, f)
		}
	}
	setAttr(n, "class", strings.Join(kept, " "))
}

// firstParagraph returns the container's first element child if it is a
// paragraph whose own first child is a text node — the shape a bare
// container's opening fence line always produces.
func firstParagraph(div *html.Node) *html.Node {
	for c := div.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) == "" {
			continue // inter-element whitespace
		}
		if c.Type == html.ElementNode && c.DataAtom == atom.P &&
			c.FirstChild != nil && c.FirstChild.Type == html.TextNode {
			return c
		}
		return nil
	}
	return nil
}

// Containers normalizes fenced containers. It accepts the brace-free
// "::: kind" form as an alias for "::: {.kind}", maps the shipped kinds
// onto their stylesheet classes, warns when a brace-free name is not one of
// them, and drops the fence library's internal data-fence attribute.
//
// warn may be nil.
func Containers(warn func(string)) Transform {
	if warn == nil {
		warn = func(string) {}
	}
	return Transform{Name: "containers", Fn: func(root *html.Node) error {
		for _, div := range fenceDivs(root) {
			removeAttr(div, "data-fence")

			if cls, ok := attr(div, "class"); ok {
				// Braced form. Normalize a shipped kind; leave anything
				// else exactly as written. Fields, not a split on space: a
				// class attribute of nothing but whitespace has no first
				// token to index.
				if f := strings.Fields(cls); len(f) > 0 {
					if k, known := containerKinds[f[0]]; known {
						removeClassToken(div, f[0])
						applyKind(div, k, nil)
					}
					continue
				}
			}
			if len(div.Attr) > 0 {
				// Braced, but with something other than a class — an id,
				// say. Not the brace-free form, so do not sniff its text.
				continue
			}

			p := firstParagraph(div)
			if p == nil {
				warn("container has no class and no recognizable kind name")
				continue
			}
			word, rest := firstWord(p.FirstChild.Data)
			k, known := containerKinds[word]
			if !known {
				warn(fmt.Sprintf("unknown container kind %q: emitting an unclassed div "+
					"(known kinds: %s)", word, knownKindList()))
				continue
			}
			p.FirstChild.Data = rest
			applyKind(div, k, p)
		}
		return nil
	}}
}

// applyKind gives a container its element and classes. p is the first
// paragraph for a brace-free container, whose kind word has already been
// stripped, or nil for a braced one; Task 3 uses it to lift a title.
//
// A braced container may already carry classes of its own — {#note .callout
// .compact} is documented and merges to class="callout compact" — so this
// merges the kind's classes in rather than overwriting the attribute, with
// the kind's classes first and every token de-duplicated. Overwriting would
// silently discard an author's extra class on a shipping, documented
// syntax.
func applyKind(div *html.Node, k containerKind, p *html.Node) {
	existing, _ := attr(div, "class")
	seen := map[string]bool{}
	var tokens []string
	for _, tok := range strings.Fields(k.class + " " + existing) {
		if !seen[tok] {
			seen[tok] = true
			tokens = append(tokens, tok)
		}
	}
	setAttr(div, "class", strings.Join(tokens, " "))
}

// knownKindList renders the vocabulary for a warning message, in a stable
// order so the text does not change between runs.
func knownKindList() string {
	names := make([]string, 0, len(containerKinds))
	for n := range containerKinds {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
