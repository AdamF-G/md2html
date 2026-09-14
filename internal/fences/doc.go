// Package fences implements the ::: fenced container syntax.
//
// It is vendored from github.com/stefanfritsch/goldmark-fences v1.0.0, MIT
// licensed — see LICENSE in this directory, whose two copyright lines are
// preserved verbatim. Upstream's own lineage is goldmark itself, hence the
// first of them.
//
// It is vendored rather than imported because md2html's brace-free
// "::: kind" form needs the parser to own the whole fence line, and
// upstream's Open leaves everything that is not an attribute block in the
// content stream. See docs/specs/2026-09-13-fence-line-capture.md.
//
// Changes from upstream are confined to:
//   - Open captures the entire fence-line remainder and delegates its
//     grammar to the SplitInfo hook (parser.go).
//   - A FencedContainerTitle block node carries a container's title so
//     goldmark parses its inlines, and a container that has one is marked
//     with data-fence-title so a transform can tell the parser's title
//     paragraph from one an author wrote in raw HTML (ast.go, parser.go,
//     renderer.go).
//   - The whole data-fence attribute namespace is reserved against author
//     text, case-insensitively, on both the hook and the braced-attribute
//     path: a forged data-fence-title costs an author their own attributes,
//     and a forged data-fence panicked Continue with an out-of-range index
//     (reservedFenceAttr in parser.go).
//   - renderContainerAttributes admits the aria- prefix alongside goldmark's
//     global attribute list and the data- prefix upstream already allowed,
//     because a landmark that cannot be named is worse than no landmark
//     (renderer.go).
//   - isNav and the elem-nav class were removed (renderer.go).
//
// One deliberate exception to "no md2html vocabulary in here": the title
// renderer writes the container-title CSS class, which comes from md2html's
// stylesheet, and md2html's container transform finds the paragraph by it.
// Keeping the name here is what lets the title survive as a real block node
// with parsed inlines; passing it in through the hook would buy nothing and
// cost an option. An upgrader porting these changes to a newer upstream
// should expect that one string to need md2html's stylesheet, and nothing
// else in this package to.
package fences
