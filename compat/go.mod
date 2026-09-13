// The Pandoc compatibility suite lives in its own module for the same
// reason the browser suite does: it needs a tool that is not a dependency
// of md2html, and "one static binary, no runtime" should stay true of
// anyone who runs `go test ./...` at the root.
//
// The module boundary is the opt-in. Run it with `just compat`, which
// needs pandoc on PATH; the suite skips itself when pandoc is absent.
module github.com/AdamF-G/md2html/compat

go 1.26

require (
	github.com/AdamF-G/md2html v0.0.0
	golang.org/x/net v0.58.0
)

require (
	github.com/stefanfritsch/goldmark-fences v1.0.0 // indirect
	github.com/yuin/goldmark v1.8.6 // indirect
	go.abhg.dev/goldmark/mermaid v0.6.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/AdamF-G/md2html => ../
