package md2html

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// headingNumRe matches a leading section number on a heading: "4", "4.2",
// "4.2.1". The number must be followed by whitespace or end the heading, so
// "4.2rc" is not a section number and "2026 in review" is — the latter
// harmlessly, since nothing will write "§2026" unless it means that heading.
var headingNumRe = regexp.MustCompile(`^(\d+(?:\.\d+)*)(?:\s|$)`)

// sectionRe matches a bare cross-reference: "§4.2", or "§ 4.2".
var sectionRe = regexp.MustCompile(`§\s?(\d+(?:\.\d+)*)`)

// sectionNumbers maps each numbered heading's number to its anchor id.
//
// It reads the id off the heading rather than re-deriving a slug, so an
// explicit {#custom-id} is what references resolve to, and so a heading
// HeadingAnchors disambiguated with a "-2" suffix resolves to the id it was
// actually given. Headings with no id — HeadingAnchors disabled via
// --no-anchors — contribute nothing, which is why this degrades to plain
// text rather than emitting hrefs that point nowhere.
//
// The first heading claiming a number keeps it. Two headings numbered the
// same is an authoring error, and picking the first is at least stable.
func sectionNumbers(root *html.Node) map[string]string {
	out := map[string]string{}
	for _, h := range headingNodes(root) {
		id, ok := attr(h, "id")
		if !ok || id == "" {
			continue
		}
		m := headingNumRe.FindStringSubmatch(strings.TrimSpace(headingText(h)))
		if m == nil {
			continue
		}
		if _, dup := out[m[1]]; !dup {
			out[m[1]] = id
		}
	}
	return out
}

// possessiveBefore reports whether the text immediately preceding a § ends
// with a possessive — "the design doc's §7" — which scopes the reference to
// another document rather than to a heading here.
func possessiveBefore(before string) bool {
	t := strings.TrimRight(before, " \t")
	return strings.HasSuffix(t, "'s") || strings.HasSuffix(t, "’s")
}

// SectionLinks autolinks bare "§N.M" references to the heading in the same
// document numbered N.M.
//
// It is a transform rather than a goldmark extension because it needs the
// finished heading set, ids included, to resolve against — which only
// exists after HeadingAnchors has run.
//
// Left alone: a § inside a code span, code block or existing link (handled
// by rewriteText, via splitMatches only ever touching prose text nodes); a
// § whose number matches no heading here; and a possessive reference
// scoping the section to another document, as in "the design doc's §7".
// That last rule is narrow — it catches the possessive phrasing and
// nothing else — so a cross-document reference written any other way
// still needs a code span to opt out.
func SectionLinks() Transform {
	return Transform{Name: "sectionLinks", Fn: func(root *html.Node) error {
		nums := sectionNumbers(root)
		if len(nums) == 0 {
			// No numbered heading has an id — either the document has none,
			// or --no-anchors suppressed every id. Either way there is
			// nothing to resolve against, so skip the walk entirely rather
			// than run it only to decline every match.
			return nil
		}
		rewriteText(root, func(s string) []*html.Node {
			return splitMatches(s, sectionRe, func(loc []int) []*html.Node {
				id, known := nums[s[loc[2]:loc[3]]]
				if !known || possessiveBefore(s[:loc[0]]) {
					return nil // decline: leave this occurrence as literal text
				}
				a := &html.Node{Type: html.ElementNode, DataAtom: atom.A, Data: "a",
					Attr: []html.Attribute{
						{Key: "class", Val: "xref"},
						{Key: "href", Val: "#" + id},
					}}
				a.AppendChild(&html.Node{Type: html.TextNode, Data: s[loc[0]:loc[1]]})
				return []*html.Node{a}
			})
		})
		return nil
	}}
}
