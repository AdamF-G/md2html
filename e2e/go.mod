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

go 1.26

require (
	github.com/AdamF-G/md2html v0.0.0
	github.com/chromedp/chromedp v0.16.0
)

require (
	github.com/chromedp/cdproto v0.0.0-20260714215040-dc233986426f // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/stefanfritsch/goldmark-fences v1.0.0 // indirect
	github.com/yuin/goldmark v1.8.6 // indirect
	go.abhg.dev/goldmark/mermaid v0.6.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/AdamF-G/md2html => ../
