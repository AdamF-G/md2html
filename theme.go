package md2html

import _ "embed"

// defaultCSS is the stylesheet inlined into output when Options.CSS is
// empty. It defines every colour token in a bare :root block before any
// media query or [data-theme] block redefines it, so the page renders
// correctly in the un-stamped "system theme" state.
//
//go:embed default.css
var defaultCSS string
