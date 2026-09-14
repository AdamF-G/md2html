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
				applyKind(div, k, detachParsedTitle(div, titled))
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
						applyKind(div, k, detachParsedTitle(div, titled))
					}
					continue
				}
			}
			if len(div.Attr) > 0 {
				// Braced, but with something other than a class — an id,
				// say. Not the brace-free form, so do not sniff its text.
				continue
			}
			warn("container has no class and no recognizable kind name")
		}
		return nil
	}}
}

// applyKind gives a container its element, classes and — when the fence
// line carried one — its title paragraph or summary.
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
func applyKind(div *html.Node, k containerKind, title []*html.Node) {
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
// returns its inline children, for applyKind to place as a title paragraph
// or a summary.
//
// It returns nil when the fence line carried no title, which is the same
// thing applyKind already expects from a titleless container.
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
