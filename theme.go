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

// builtinFS holds every built-in plugin, one directory each under plugins/.
// A contributor adds a plugin by adding a directory; nothing here changes.
//
//go:embed plugins
var builtinFS embed.FS
