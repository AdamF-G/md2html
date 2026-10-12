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
			`background: url("data:image/svg+xml;base64,` + enc("<svg/>") + `#a)b")`, ""},
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
