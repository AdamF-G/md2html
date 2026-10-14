// The Pandoc compatibility suite lives in its own module for the same
// reason the browser suite does: it needs a tool that is not a dependency
// of md2html, and "one static binary, no runtime" should stay true of
// anyone who runs `go test ./...` at the root.
//
// The module boundary is the opt-in. Run it with `just compat`, which
// needs pandoc on PATH; the suite skips itself when pandoc is absent.
module github.com/AdamF-G/md2html/compat

go 1.26.0

require (
	github.com/AdamF-G/md2html v0.0.0
	golang.org/x/net v0.58.0
)

require (
	github.com/andybalholm/brotli v1.2.2 // indirect
	github.com/dlclark/regexp2/v2 v2.8.1 // indirect
	github.com/dop251/goja v0.0.0-20261013122152-245c54f48cfc // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/google/pprof v0.0.0-20230207041349-798e818bf904 // indirect
	github.com/tdewolff/font v0.0.0-20261010204837-88a7f6b56918 // indirect
	github.com/tdewolff/parse/v2 v2.8.16 // indirect
	github.com/yuin/goldmark v1.8.6 // indirect
	go.abhg.dev/goldmark/mermaid v0.6.0 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/AdamF-G/md2html => ../
