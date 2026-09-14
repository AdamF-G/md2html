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
//     goldmark parses its inlines (ast.go, renderer.go).
//   - isNav and the elem-nav class were removed (renderer.go).
package fences
