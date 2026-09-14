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
			// Read the parser's title marker before it is stripped. It is
			// the signal that the first paragraph is a title the fence
			// renderer emitted rather than one the author wrote, and it can
			// be trusted because the parser reserves the whole data-fence
			// attribute namespace against author text (reservedFenceAttr).
			_, titled := attr(div, "data-fence-title")
			removeAttr(div, "data-fence-title")

			// The fence parser owns the label form end to end: its
			// attributes are already on the div, its kind travelled as
			// data-fence-kind, and its title is a first-child paragraph.
			// There is nothing left to mine out of the body.
			if kind, owned := attr(div, "data-fence-kind"); owned {
				removeAttr(div, "data-fence-kind")
				k, known := containerKinds[kind]
				if !known {
					warn(fmt.Sprintf("unknown container kind %q: emitting an unclassed div "+
						"(known kinds: %s)", kind, knownKindList()))
					// The kind was refused, so the container gets no
					// classes — and its title must not keep a styling hook
					// for a container that does not exist. The author's
					// text survives as ordinary prose instead.
					declassParsedTitle(div, titled)
					continue
				}
				applyKindWithTitle(div, k, nil, detachParsedTitle(div, titled))
				continue
			}

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
			if kind, isLabel := labelKind(p); isLabel {
				if k, known := containerKinds[kind]; known {
					if label, block, done := detachLabel(p); done {
						applyLabelAttrs(div, block)
						applyKindWithTitle(div, k, p, label)
						continue
					}
				}
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

// detachTitle removes and returns the inline nodes that made up the rest of
// a brace-free container's opening fence line. The kind word has already
// been stripped from p's leading text node by the caller.
//
// goldmark merges the text after ":::" with the next source line into one
// paragraph, so the first newline among p's direct children is exactly the
// end of the fence line. That boundary is only trustworthy for the
// brace-free form, where the leading word has already proven this paragraph
// began on the fence line; the braced form is ambiguous and never gets here.
//
// Only direct children are scanned. Emphasis opened on the fence line and
// closed on the next one would carry the newline inside an element, and the
// title then runs to the following top-level newline — a title containing a
// newline, which HTML collapses to a space. Harmless, and not worth cloning
// elements across the split to avoid.
//
// Whitespace is trimmed only at the title's two outer edges: the trailing
// space before the newline here, and (via the empty-remnant check below)
// the leading space firstWord left behind after removing the kind word.
// Whitespace between inline nodes — e.g. the space between a `code` span
// and the word after it — is part of the title's own text and must survive
// untouched, or the rendered title would run words together.
func detachTitle(p *html.Node) []*html.Node {
	var title []*html.Node
	for c := p.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.TextNode {
			if i := strings.IndexByte(c.Data, '\n'); i >= 0 {
				head := strings.TrimRight(c.Data[:i], " \t")
				c.Data = c.Data[i+1:]
				if head != "" {
					title = append(title, &html.Node{Type: html.TextNode, Data: head})
				}
				return title
			}
			if len(title) == 0 && strings.TrimSpace(c.Data) == "" {
				// The empty (or whitespace-only) remnant left behind after
				// firstWord stripped the kind word and its separating
				// space — not a real leading space in the title, so it
				// must not become one.
				p.RemoveChild(c)
				c = next
				continue
			}
		}
		p.RemoveChild(c)
		title = append(title, c)
		c = next
	}
	// No newline anywhere: the whole paragraph was the opening fence line.
	return title
}

// applyKind gives a container its element, classes and — for a brace-free
// container with a title on the fence line — its title paragraph or summary.
// p is that container's first paragraph with the kind word already stripped,
// or nil for a braced container, which never gets a title.
//
// A braced container may already carry classes of its own — {#note .callout
// .compact} is documented and merges to class="callout compact" — so this
// merges the kind's classes in rather than overwriting the attribute, with
// the kind's classes first and every token de-duplicated. Overwriting would
// silently discard an author's extra class on a shipping, documented
// syntax. toDetails below carries div's full attribute set (including this
// merged class, and anything else like an id) forward onto the replacement
// <details> element, so this merge is what a collapsible kind ends up
// wearing too.
func applyKind(div *html.Node, k containerKind, p *html.Node) {
	var title []*html.Node
	if p != nil {
		title = detachTitle(p)
	}
	applyKindWithTitle(div, k, p, title)
}

// applyKindWithTitle is applyKind with the title already detached, for the
// label form, whose title is delimited by brackets rather than by the end
// of the fence line and so is found a different way.
func applyKindWithTitle(div *html.Node, k containerKind, p *html.Node, title []*html.Node) {
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

	if p != nil && p.FirstChild == nil {
		// The fence line was the paragraph's entire content.
		p.Parent.RemoveChild(p)
	}

	if k.tag == "details" {
		toDetails(div, k, title)
		return
	}
	if len(title) > 0 {
		tp := &html.Node{Type: html.ElementNode, DataAtom: atom.P, Data: "p",
			Attr: []html.Attribute{{Key: "class", Val: "container-title"}}}
		for _, n := range title {
			tp.AppendChild(n)
		}
		div.InsertBefore(tp, div.FirstChild)
	}
}

// toDetails rebuilds a container as a <details> with a <summary>.
//
// The element has to be replaced rather than relabeled: x/net/html keys
// rendering off DataAtom and Data, and a <div> cannot simply become a
// <details> in place without leaving one of the two stale. div's Attr slice
// — already carrying the merged class from applyKind, plus anything else
// the author wrote, like an id — is reused as-is rather than rebuilt from
// k.class alone, so a braced collapsible container's extra attributes
// survive the rebuild.
func toDetails(div *html.Node, k containerKind, title []*html.Node) {
	d := &html.Node{Type: html.ElementNode, DataAtom: atom.Details, Data: "details", Attr: div.Attr}

	sum := &html.Node{Type: html.ElementNode, DataAtom: atom.Summary, Data: "summary"}
	switch {
	case k.prefix != "" && len(title) > 0:
		sum.AppendChild(&html.Node{Type: html.TextNode, Data: k.prefix + " — "})
	case k.prefix != "":
		sum.AppendChild(&html.Node{Type: html.TextNode, Data: k.prefix})
	case len(title) == 0:
		sum.AppendChild(&html.Node{Type: html.TextNode, Data: k.fallback})
	}
	for _, n := range title {
		sum.AppendChild(n)
	}
	d.AppendChild(sum)

	for c := div.FirstChild; c != nil; {
		next := c.NextSibling
		div.RemoveChild(c)
		d.AppendChild(c)
		c = next
	}
	div.Parent.InsertBefore(d, div)
	div.Parent.RemoveChild(div)
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

// labelKind reads the kind word of a label-form container — the "aside" of
// ":::aside[Why this matters]" — without modifying anything, so an unknown
// kind can fall through to the brace-free path and warn there exactly as
// it does today.
func labelKind(p *html.Node) (string, bool) {
	first := p.FirstChild
	if first == nil || first.Type != html.TextNode {
		return "", false
	}
	open := strings.IndexByte(first.Data, '[')
	if open <= 0 {
		return "", false
	}
	kind := first.Data[:open]
	if strings.ContainsAny(kind, " \t\n") {
		return "", false
	}
	return kind, true
}

// detachLabel removes a label-form container's opening fence line from p,
// returning the label's inline nodes and the contents of a trailing
// attribute block if one followed.
//
// This is the directive label syntax — :::kind[Title] — from the
// CommonMark generic directives proposal, as implemented by
// remark-directive and used by Docusaurus. It exists alongside the
// undelimited "::: kind Title" form rather than replacing it, and it is
// the only one of the two that can also carry an attribute block: an
// undelimited title runs to the end of the line, so a following {...}
// would be part of the title text rather than attributes.
//
// The scan crosses sibling nodes because a label may contain inline
// markup: "[Why `code` matters]" reaches this function as a text node, a
// <code> element and another text node, and the closing bracket is in the
// third of them.
//
// It reports false without mutating anything when there is no closing
// bracket, leaving the caller to fall through to the brace-free path.
func detachLabel(p *html.Node) (label []*html.Node, block string, ok bool) {
	first := p.FirstChild
	open := strings.IndexByte(first.Data, '[')

	// Pass one: locate the closing bracket. Nothing is modified until it
	// is known to exist, so a malformed fence line is left exactly as the
	// author wrote it.
	var closing *html.Node
	closeIdx := -1
	for c := first; c != nil; c = c.NextSibling {
		if c.Type != html.TextNode {
			continue
		}
		start := 0
		if c == first {
			start = open + 1
		}
		if j := strings.IndexByte(c.Data[start:], ']'); j >= 0 {
			closing, closeIdx = c, start+j
			break
		}
	}
	if closing == nil {
		return nil, "", false
	}

	// Pass two: move everything up to the bracket into the label.
	for c := first; c != closing; {
		next := c.NextSibling
		if c == first {
			if head := c.Data[open+1:]; head != "" {
				label = append(label, &html.Node{Type: html.TextNode, Data: head})
			}
			p.RemoveChild(c)
		} else {
			p.RemoveChild(c)
			label = append(label, c)
		}
		c = next
	}
	start := 0
	if closing == first {
		start = open + 1
	}
	if head := closing.Data[start:closeIdx]; head != "" {
		label = append(label, &html.Node{Type: html.TextNode, Data: head})
	}
	rest := closing.Data[closeIdx+1:]

	// An attribute block may follow the label on the same line.
	if trimmed := strings.TrimLeft(rest, " \t"); strings.HasPrefix(trimmed, "{") {
		if line, tail, found := strings.Cut(trimmed, "\n"); found {
			if _, content, braced := splitBraced(line); braced {
				block, rest = content, "\n"+tail
			}
		} else if _, content, braced := splitBraced(trimmed); braced {
			block, rest = content, ""
		}
	}
	// The newline ending the fence line is a separator, not body text —
	// the same boundary detachTitle consumes for the undelimited form.
	rest = strings.TrimPrefix(rest, "\n")

	if rest == "" {
		p.RemoveChild(closing)
	} else {
		closing.Data = rest
	}
	return label, block, true
}

// applyLabelAttrs puts a label-form container's attribute block onto the
// div, before applyKindWithTitle merges the kind's own classes in on top.
func applyLabelAttrs(div *html.Node, block string) {
	if block == "" {
		return
	}
	a, ok := parseAttrs(block)
	if !ok {
		return
	}
	if a.id != "" {
		setAttr(div, "id", a.id)
	}
	if len(a.classes) > 0 {
		setAttr(div, "class", strings.Join(a.classes, " "))
	}
}

// parsedTitleNode returns the paragraph the fence renderer emitted for a
// container's title, or nil if there is none.
//
// titled is the container's data-fence-title marker, and it gates the whole
// search. The paragraph is identified by its class, md2html renders with
// WithUnsafe, and a titleless owned container already exists — ":::card[]"
// is one — so without the gate an author's own raw
// <p class="container-title"> as a container's first block would be adopted
// as that container's title and torn apart, losing whatever attributes the
// author put on it.
//
// The marker is worth trusting because the parser refuses to set any
// attribute in the data-fence namespace from author text
// (reservedFenceAttr); a document that writes data-fence-title itself
// therefore cannot forge one.
func parsedTitleNode(div *html.Node, titled bool) *html.Node {
	if !titled {
		return nil
	}
	for c := div.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) == "" {
			continue // inter-element whitespace
		}
		if c.Type == html.ElementNode && c.DataAtom == atom.P && hasClass(c, "container-title") {
			return c
		}
		return nil
	}
	return nil
}

// detachParsedTitle removes the title element the fence parser emitted and
// returns its inline children, for applyKindWithTitle to place as a title
// paragraph or a summary.
//
// It returns nil when the fence line carried no title, which is the same
// thing applyKindWithTitle already expects from a titleless container.
func detachParsedTitle(div *html.Node, titled bool) []*html.Node {
	tp := parsedTitleNode(div, titled)
	if tp == nil {
		return nil
	}
	var out []*html.Node
	for c := tp.FirstChild; c != nil; {
		next := c.NextSibling
		tp.RemoveChild(c)
		out = append(out, c)
		c = next
	}
	div.RemoveChild(tp)
	return out
}

// declassParsedTitle drops the class from a parsed title paragraph, leaving
// the author's words in place as an ordinary paragraph.
//
// It is the unknown-kind path's counterpart to detachParsedTitle: the
// container was refused and carries no kind classes, so a title paragraph
// still wearing container-title would be a styling hook for a container that
// was not built. Removing the attribute rather than the element keeps the
// text the author wrote.
func declassParsedTitle(div *html.Node, titled bool) {
	if tp := parsedTitleNode(div, titled); tp != nil {
		removeAttr(tp, "class")
	}
}
