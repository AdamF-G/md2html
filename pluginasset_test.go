package md2html

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// fakeRead serves files from a map, standing in for a plugin directory.
func fakeRead(files map[string]string) func(string) ([]byte, error) {
	return func(rel string) ([]byte, error) {
		if b, ok := files[rel]; ok {
			return []byte(b), nil
		}
		return nil, errors.New(rel + ": no such file")
	}
}

func TestInlineAssets(t *testing.T) {
	files := map[string]string{"fonts/a.woff2": "FONT", "img/b.svg": "<svg/>", "x.png": "PNG"}
	enc := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	for _, c := range []struct{ name, cssPath, in, want string }{
		{"unquoted", "style.css", `src: url(fonts/a.woff2)`, `src: url("data:font/woff2;base64,` + enc("FONT") + `")`},
		{"double quoted", "style.css", `src: url("fonts/a.woff2")`, `src: url("data:font/woff2;base64,` + enc("FONT") + `")`},
		{"single quoted", "style.css", `src: url('fonts/a.woff2')`, `src: url("data:font/woff2;base64,` + enc("FONT") + `")`},
		{"spaces inside", "style.css", `src: url(  fonts/a.woff2  )`, `src: url("data:font/woff2;base64,` + enc("FONT") + `")`},
		{"uppercase URL(", "style.css", `src: URL(fonts/a.woff2)`, `src: url("data:font/woff2;base64,` + enc("FONT") + `")`},
		{"relative to the css file", "css/style.css", `background: url(../img/b.svg)`, `background: url("data:image/svg+xml;base64,` + enc("<svg/>") + `")`},
		{"query dropped", "style.css", `src: url(fonts/a.woff2?v=3)`, `src: url("data:font/woff2;base64,` + enc("FONT") + `")`},
		{"fragment kept", "style.css", `mask: url(img/b.svg#icon)`, `mask: url("data:image/svg+xml;base64,` + enc("<svg/>") + `#icon")`},
		{"data passes through", "style.css", `background: url(data:image/png;base64,AAAA)`, `background: url(data:image/png;base64,AAAA)`},
		{"fragment reference passes through", "style.css", `filter: url(#shadow)`, `filter: url(#shadow)`},
		{"url in a comment", "style.css", `/* was url(http://x/y.png) */ a{}`, `/* was url(http://x/y.png) */ a{}`},
		{"url in a string", "style.css", `a::after{content:"url(http://x)"}`, `a::after{content:"url(http://x)"}`},
		{"not a url function", "style.css", `--my-url(x)`, `--my-url(x)`},
		{"no urls", "style.css", "body { color: red; }\n", "body { color: red; }\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := inlineAssets(c.in, c.cssPath, fakeRead(files))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

func TestInlineAssetsRefuses(t *testing.T) {
	files := map[string]string{"a.txt": "x", "a.woff2": "x"}
	for _, c := range []struct{ name, in, want string }{
		{"http", `url(http://cdn/x.woff2)`, "is not a relative path inside the plugin"},
		{"https", `url("https://cdn/x.woff2")`, "is not a relative path inside the plugin"},
		{"scheme-relative", `url(//cdn/x.woff2)`, "is not a relative path inside the plugin"},
		{"absolute path", `url(/etc/x.woff2)`, "is not a relative path inside the plugin"},
		{"escapes the plugin", `url(../../x.woff2)`, "is outside the plugin"},
		{"unknown extension", `url(a.txt)`, `".txt" is not a font or image type`},
		{"missing file", `url(b.woff2)`, "no such file"},
		{"import", `@import "other.css";`, "@import is not allowed"},
		{"import uppercase", `@IMPORT url(other.css);`, "@import is not allowed"},
		{"unclosed url", `url(a.woff2`, "unclosed url("},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := inlineAssets(c.in, "style.css", fakeRead(files))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

// The default stylesheet has no url() at all, so inlining must hand it back
// byte for byte; Task 5 relies on this for unchanged default output.
func TestInlineAssetsLeavesDefaultCSSUnchanged(t *testing.T) {
	got, err := inlineAssets(defaultCSS, "default.css", fakeRead(nil))
	if err != nil || got != defaultCSS {
		t.Errorf("default.css changed (err %v)", err)
	}
}

// A ")" inside a quoted url() argument must not end the token, and a
// quoted or escaped character must not open a string the scan never closes:
// either desync would let a later remote url() or @import pass unrefused.
func TestInlineAssetsQuotesAndEscapes(t *testing.T) {
	files := map[string]string{"img/b.svg": "<svg/>"}
	enc := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	for _, c := range []struct{ name, in, want, wantErr string }{
		{"svg data uri with parens in double quotes passes through",
			`a{background:url("data:image/svg+xml,<svg fill='rgb(0,0,0)'/>")}`,
			`a{background:url("data:image/svg+xml,<svg fill='rgb(0,0,0)'/>")}`, ""},
		{"svg data uri with parens in double quotes, then remote url",
			`a{background:url("data:image/svg+xml,<svg fill='rgb(0,0,0)'/>")} b{src:url(https://x/y.woff2)}`,
			"", "is not a relative path inside the plugin"},
		{"svg data uri with parens in single quotes passes through",
			`a{background:url('data:image/svg+xml,<svg fill="rgb(0,0,0)"/>')}`,
			`a{background:url('data:image/svg+xml,<svg fill="rgb(0,0,0)"/>')}`, ""},
		{"svg data uri with parens in single quotes, then remote url",
			`a{background:url('data:image/svg+xml,<svg fill="rgb(0,0,0)"/>')} b{src:url(https://x/y.woff2)}`,
			"", "is not a relative path inside the plugin"},
		{"escaped quote in a selector, then remote url",
			`.a-\'x{} b{src:url(http://x)}`,
			"", "is not a relative path inside the plugin"},
		{"unterminated string ends at newline, then remote url",
			"a{content:\"abc\n}\nb{src:url(http://x)}",
			"", "is not a relative path inside the plugin"},
		{"fragment with a paren inside quotes inlines the file",
			`background: url("img/b.svg#a)b")`,
			`background: url("data:image/svg+xml;base64,` + enc("<svg/>") + `#a%29b")`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := inlineAssets(c.in, "style.css", fakeRead(files))
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Errorf("err = %v, want it to contain %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

// Each payload here is a way a stylesheet can make a browser fetch, or end
// the page's <style> element, that a substring scan missed. CSS decodes
// escapes in names, treats \r and \f as newlines, and reads the strings in
// image-set() and src() as URLs, so the scanner must do all three, in every
// spelling, or a plugin reaches out at view time.
func TestInlineAssetsRefusesEverySpellingOfAFetch(t *testing.T) {
	files := map[string]string{"i.svg": "<svg/>", "a.png": "PNG"}
	for _, c := range []struct{ name, in, want string }{
		{"escaped u in url", `body{background-image:\url(http://h/a.png)}`, "is not a relative path inside the plugin"},
		{"hex escape in url", `body{background-image:\75 rl(http://h/b.png)}`, "is not a relative path inside the plugin"},
		{"hex escape, mixed case", `body{background-image:\55 Rl(http://h/b.png)}`, "is not a relative path inside the plugin"},
		{"every letter escaped", `a{b:\75\72\6c(http://h/b.png)}`, "is not a relative path inside the plugin"},
		{"hex escape ended by crlf", "a{b:\\75\r\nrl(http://h/b.png)}", "is not a relative path inside the plugin"},
		{"escaped scheme colon", `a{b:url(http\3a //h/b.png)}`, "is not a relative path inside the plugin"},
		{"escaped scheme in a string", `a{b:url("\68ttp://h/b.png")}`, "is not a relative path inside the plugin"},
		{"escaped backslashes for slashes", `a{b:url(\5c\5c h/b.png)}`, "is not a relative path inside the plugin"},
		{"image-set string", `body{background-image:image-set("http://h/d.png" 1x)}`, "is not a relative path inside the plugin"},
		{"image-set uppercase", `a{b:IMAGE-SET('//h/d.png' 1x)}`, "is not a relative path inside the plugin"},
		{"webkit image-set", `a{b:-webkit-image-set("http://h/d.png" 1x)}`, "is not a relative path inside the plugin"},
		{"escaped image-set", `a{b:\69mage-set("http://h/d.png" 1x)}`, "is not a relative path inside the plugin"},
		{"image-set second candidate", `a{b:image-set("a.png" type("image/png") 1x, "http://h/d.png" 2x)}`, "is not a relative path inside the plugin"},
		{"image-set string continued over a newline", "a{b:image-set(\"http://h/\\\nd.png\" 1x)}", "is not a relative path inside the plugin"},
		{"image-set string inside if()", `a{b:image-set(if(media(width > 0): "http://h/d.png") 1x)}`, "cannot be checked"},
		{"image-set variable", `a{--u:"http://h/d.png";b:image-set(var(--u) 1x)}`, "cannot be checked"},
		{"image-set custom function", `a{b:image-set(--u() 1x)}`, "cannot be checked"},
		{"src string", `a{b:src("http://h/d.png")}`, "is not a relative path inside the plugin"},
		{"escaped src", `a{b:\73rc("http://h/d.png")}`, "is not a relative path inside the plugin"},
		{"src variable", `a{b:src(var(--u))}`, "cannot be checked"},
		{"image string", `a{b:image("http://h/d.png")}`, "is not a relative path inside the plugin"},
		{"string ended by cr", "body{content:\"x\r;background-image:url(http://h/e.png)}\n", "is not a relative path inside the plugin"},
		{"string ended by form feed", "body{content:\"x\f;background-image:url(http://h/e.png)}\n", "is not a relative path inside the plugin"},
		{"string ended by crlf", "body{content:\"x\r\n;background-image:url(http://h/e.png)}\n", "is not a relative path inside the plugin"},
		{"url after a CDO", `<!--url(http://h/e.png)`, "is not a relative path inside the plugin"},
		{"url after a CDC", `-->url(http://h/e.png)`, "is not a relative path inside the plugin"},
		{"escaped import", `@\69mport "http://h/c.css";`, "@import is not allowed"},
		{"escaped import, mixed case", `@IMP\4fRT "c.css";`, "@import is not allowed"},
		{"style close", `body{} </style><img src="http://h/g.png"><style>`, "</style"},
		{"style close uppercase in a comment", `/* </STYLE> */`, "</style"},
		{"style close in a string", `a::after{content:"</Style"}`, "</style"},
		{"quote in an unquoted url", `a{b:url(i.svg#a"b)}`, "malformed url("},
		{"space in an unquoted url", `a{b:url(a b.png)}`, "malformed url("},
		{"unclosed url at the end", `a{b:url(a.png`, "unclosed url("},
		{"unclosed image-set string", "a{b:image-set(\"a.png\n 1x)}", "unclosed string"},
		{"font-face remote src", `@font-face{src:url(http://h/f.woff2)}`, "is not a relative path inside the plugin"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := inlineAssets(c.in, "style.css", fakeRead(files))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to contain %q; output %q", err, c.want, got)
			}
		})
	}
}

// A fragment is copied from the plugin's CSS onto the data: URI written in
// its place, so it is percent-encoded: a quote or parenthesis in it would
// otherwise close the url("…") token and start new CSS after it.
func TestInlineAssetsEncodesFragments(t *testing.T) {
	files := map[string]string{"i.svg": "<svg/>"}
	in := `body{background-image:url('i.svg#"),url(//h/evil/f.png)/*')} /**/}`
	got, err := inlineAssets(in, "style.css", fakeRead(files))
	if err != nil {
		t.Fatal(err)
	}
	want := `body{background-image:url("data:image/svg+xml;base64,` +
		base64.StdEncoding.EncodeToString([]byte("<svg/>")) +
		`#%22%29%2Curl%28%2F%2Fh%2Fevil%2Ff.png%29%2F%2A")} /**/}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// What a real stylesheet writes must still pass: escaped selectors in the
// style of Tailwind, quotes and parentheses inside a data: URL, fragment
// references, font-face sources with format(), and image-set() naming the
// plugin's own files.
func TestInlineAssetsKeepsLegitimateCSS(t *testing.T) {
	files := map[string]string{"i.svg": "<svg/>", "a.png": "PNG", "f.woff2": "FONT"}
	enc := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	png, svg, font := "data:image/png;base64,"+enc("PNG"), "data:image/svg+xml;base64,"+enc("<svg/>"), "data:font/woff2;base64,"+enc("FONT")
	for _, c := range []struct{ name, in, want string }{
		{"escaped slash in a selector", `.w-1\/2{width:50%}`, `.w-1\/2{width:50%}`},
		{"escaped brackets and quotes in a selector", `.content-\[\'x\'\]{content:'x'}`, `.content-\[\'x\'\]{content:'x'}`},
		{"svg data url with quotes and parens",
			`a{background:url("data:image/svg+xml,<svg fill='rgb(0,0,0)'/>")}`,
			`a{background:url("data:image/svg+xml,<svg fill='rgb(0,0,0)'/>")}`},
		{"fragment reference", `a{filter:url(#shadow)}`, `a{filter:url(#shadow)}`},
		{"quoted fragment reference", `a{filter:url("#shadow")}`, `a{filter:url("#shadow")}`},
		{"font-face with format", `@font-face{src:url(f.woff2) format("woff2")}`, `@font-face{src:url("` + font + `") format("woff2")}`},
		{"image-set of plugin files",
			`a{b:image-set("a.png" 1x, url(i.svg) 2x)}`,
			`a{b:image-set("` + png + `" 1x, url("` + svg + `") 2x)}`},
		{"image-set with a type", `a{b:image-set("a.png" type("image/png") 1x)}`, `a{b:image-set("` + png + `" type("image/png") 1x)}`},
		{"webkit image-set", `a{b:-webkit-image-set('a.png' 1x)}`, `a{b:-webkit-image-set("` + png + `" 1x)}`},
		{"src of a plugin file", `a{b:src("a.png")}`, `a{b:src("` + png + `")}`},
		{"a variable in a gradient in image-set", `a{b:image-set(linear-gradient(var(--a), red) 1x)}`, `a{b:image-set(linear-gradient(var(--a), red) 1x)}`},
		{"a string custom property", `:root{--u:"http://h/d.png"}`, `:root{--u:"http://h/d.png"}`},
		{"a url-named dimension", `a{b:1\75 rl(x)}`, `a{b:1\75 rl(x)}`},
		{"a hash named url", `#url(x){}`, `#url(x){}`},
		{"image-set in a content string", `a::after{content:"image-set(\"http://x\")"}`, `a::after{content:"image-set(\"http://x\")"}`},
		{"import in a comment", `/* @import "http://x"; */ a{}`, `/* @import "http://x"; */ a{}`},
		{"crlf line endings kept", "a{\r\n  color: red;\r\n}\r\n", "a{\r\n  color: red;\r\n}\r\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := inlineAssets(c.in, "style.css", fakeRead(files))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}
