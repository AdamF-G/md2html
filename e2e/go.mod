// The browser suite lives in its own module so chromedp is not a
// requirement of the library: a test-only import would sit in the library's
// own go.mod as a direct dependency and be compiled by anyone running its
// tests. It remains there as an indirect entry, inherited from
// go.abhg.dev/goldmark/mermaid's server-side renderer, which md2html never
// imports — nothing the library builds or tests compiles it.
//
// There is no build tag any more: this module boundary is the opt-in. Run
// it with `just e2e`, which needs a local Chrome or Chromium.
module github.com/AdamF-G/md2html/e2e

go 1.26.0

require (
	github.com/AdamF-G/md2html v0.0.0
	github.com/chromedp/cdproto v0.0.0-20260714215040-dc233986426f
	github.com/chromedp/chromedp v0.16.0
)

require (
	github.com/andybalholm/brotli v1.2.2 // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/dlclark/regexp2/v2 v2.8.1 // indirect
	github.com/dop251/goja v0.0.0-20261013122152-245c54f48cfc // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/google/pprof v0.0.0-20230207041349-798e818bf904 // indirect
	github.com/tdewolff/font v0.0.0-20261010204837-88a7f6b56918 // indirect
	github.com/tdewolff/parse/v2 v2.8.16 // indirect
	github.com/yuin/goldmark v1.8.6 // indirect
	go.abhg.dev/goldmark/mermaid v0.6.0 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/AdamF-G/md2html => ../
