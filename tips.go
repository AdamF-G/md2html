package md2html

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// A tip is recorded on its host by Chips or LinkAttrs as these two
// attributes, and writeTips turns them into the tip's markup. Neither name
// has a data- or aria- prefix, so no attribute block can set one, and
// writeTips removes both, so neither reaches the page.
const (
	tipAttr      = "md2html-tip"
	tipDelayAttr = "md2html-tip-delay"
)

// tipDelayRe is a CSS time with no sign, unit space or expression.
var tipDelayRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?m?s$`)

// validTipDelay reports whether v may be written into CSS as a tip's
// delay. It is the only thing between an author's value and a style
// attribute or the page's stylesheet, so it admits nothing but a plain
// time: "0ms; background: red" or calc(1s) would carry CSS of its own.
func validTipDelay(v string) bool { return tipDelayRe.MatchString(v) }

// writeTips turns every element carrying tipAttr into a tip: the element
// becomes the host, a focusable one unless it is a link, and its note a
// tooltip beside it, the two held in a wrap the CSS can place the note
// over. Each tooltip is md2html-tooltip-N, numbered from 1 in document
// order and skipping any id the page already has, and the host names it
// with aria-describedby. An element's own delay goes on the wrap. The
// tooltip is aria-hidden, so a screen reader hears the note once, as the
// host's description, and not again in the flow.
//
// It runs after every transform, a site's linkDrop and linkRewrite
// included, so HeadingAnchors and TOC never see a note, and a link
// linkDrop unwrapped has already become the span that hosts its tip.
//
// A span host inside a link would put a focusable element in the link and
// the note into the link's name, so its tip is dropped. Markdown cannot
// make one, since Chips leaves a link's text alone, so it says nothing.
func writeTips(root *html.Node) {
	var hosts []*html.Node
	// The page's own ids, so a tooltip never takes one: a heading
	// "md2html tooltip 1" slugs to md2html-tooltip-1.
	used := map[string]bool{}
	walk(root, func(n *html.Node) {
		if n.Type == html.ElementNode {
			if _, ok := attr(n, tipAttr); ok {
				hosts = append(hosts, n)
			}
			if v, ok := attr(n, "id"); ok {
				used[v] = true
			}
		}
	})
	id := 0
	for _, host := range hosts {
		note, _ := attr(host, tipAttr)
		delay, _ := attr(host, tipDelayAttr)
		removeAttr(host, tipAttr)
		removeAttr(host, tipDelayAttr)
		if host.DataAtom != atom.A && insideLink(host) {
			continue
		}
		tipID := ""
		for tipID == "" || used[tipID] {
			id++
			tipID = fmt.Sprintf("md2html-tooltip-%d", id)
		}
		existing, _ := attr(host, "class")
		setAttr(host, "class", strings.Join(mergeTokens(existing, "md2html-tip-host"), " "))
		if host.DataAtom != atom.A {
			setAttr(host, "tabindex", "0")
		}
		setAttr(host, "aria-describedby", tipID)
		wrap := elem("span", "class", "md2html-tip-wrap")
		if delay != "" {
			// On the wrap, not the host: a custom property inherits only
			// from an ancestor, and the tooltip is the host's sibling.
			setAttr(wrap, "style", "--md2html-tip-delay: "+delay)
		}
		tip := elem("span", "class", "md2html-tooltip", "id", tipID, "role", "tooltip", "aria-hidden", "true")
		tip.AppendChild(textNode(note))
		host.Parent.InsertBefore(wrap, host)
		host.Parent.RemoveChild(host)
		wrap.AppendChild(host)
		wrap.AppendChild(tip)
	}
}

// insideLink reports whether n has an <a> above it.
func insideLink(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.DataAtom == atom.A {
			return true
		}
	}
	return false
}

// tipDelayRule is the page's tip delay as a rule for its stylesheet: its
// own front matter's tip-delay, else the site shell's, else none, which
// leaves tip.css's default. It goes on main.md2html, the element a
// fragment's stylesheet scopes itself to, so a fragment never sets
// anything on its host's :root. A value that is not a plain time is
// passed over with a warning, to the page's warn or the shell's.
func tipDelayRule(page, shell string, warn, shellWarn func(string)) string {
	rule := func(v string) string { return "\nmain.md2html { --md2html-tip-delay: " + v + "; }\n" }
	// The shell's value is checked even when the page's wins, so a bad
	// one is reported whichever page is built first; shellWarn keeps it
	// once.
	shellOK := shell == "" || validTipDelay(shell)
	if !shellOK && shellWarn != nil {
		shellWarn(fmt.Sprintf("front matter tip-delay %q is not a time such as 150ms; using the default delay", shell))
	}
	if page != "" {
		if validTipDelay(page) {
			return rule(page)
		}
		if warn != nil {
			warn(fmt.Sprintf("front matter tip-delay %q is not a time such as 150ms; using the site's or the default delay", page))
		}
	}
	if shell != "" && shellOK {
		return rule(shell)
	}
	return ""
}
