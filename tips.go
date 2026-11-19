package md2html

import "regexp"

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
