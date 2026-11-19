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
