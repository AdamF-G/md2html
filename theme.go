package md2html

import "embed"

// defaultCSS is the stylesheet inlined into output when no style is
// selected. It defines every colour token in a bare :root block before any
// media query or [data-theme] block redefines it, so the page renders
// correctly in the un-stamped "system theme" state.
//
// It is the built-in default plugin's one style, embedded by path as well
// as through builtinFS so that the common case, a page with no style, never
// touches the plugin loader.
//
//go:embed plugins/default/default.css
var defaultCSS string

// siteCSS draws what the site and list markers produce. It is not part of
// any style: renderPage puts it ahead of the page's style when the page
// needs it (see needsSiteCSS), so a page without those markers is
// unchanged and a style's own rules for the same classes win.
//
//go:embed site.css
var siteCSS string

// highlightCSS colours highlighted code. Like siteCSS it is not part of any
// style: renderPage puts it ahead of the page's style when the page has a
// highlighted fence (see needsHighlightCSS).
//
//go:embed highlight.css
var highlightCSS string

// tipCSS is what makes a tip work: the tooltip hidden, shown over its
// host on hover or focus after the page's delay, and printed after it.
// Like highlightCSS it is part of no style. renderPage and renderFragment
// put it ahead of the page's style when the page has a tip (see
// needsTipCSS), whatever the style, since a page whose style left these
// rules out would show every note as a second copy in its text. It holds
// the look too, ahead of the style so a style can restyle it: in
// default.css it would change every page, tip or none, which the v0.12.0
// goldens forbid.
//
//go:embed tip.css
var tipCSS string

// builtinFS holds every built-in plugin, one directory each under plugins/.
// A contributor adds a plugin by adding a directory; nothing here changes.
//
//go:embed plugins
var builtinFS embed.FS
