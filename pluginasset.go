package md2html

import (
	"encoding/base64"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

// assetTypes are the files a style may inline, by extension. Fonts and
// images are what a stylesheet references; anything else in a url() is
// more likely a mistake than an asset, so it is refused rather than guessed.
var assetTypes = map[string]string{
	".woff2": "font/woff2",
	".woff":  "font/woff",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
}

// inlineAssets replaces every URL in css that names a file in the plugin
// with a data: URI of that file, so a page stays one file and fetches
// nothing: mermaid stays the only thing a page can load, and fragment
// output stays publishable as an Artifact, which allows no asset host.
//
// cssPath is the stylesheet's own path in the plugin, which relative URLs
// resolve against, as a browser would. read fetches a file by its path in
// the plugin and refuses one outside it.
//
// A URL is anything a browser would fetch: a url() or src() argument, or a
// string in image-set(). Each is found by tokenizing the CSS the way a
// browser does (see cssScanner), not by matching text, so that no spelling
// CSS accepts, escapes included, slips past, and so that "url(" in a
// comment or a content string, which fetches nothing, does not fail the
// load. A fragment reference such as url(#shadow) names an element in the
// page, not a file, so it passes through, as does a data: URL already
// inline. @import, a URL that leaves the plugin, and a URL the scan cannot
// check, such as var() anywhere inside image-set() but in a gradient, a
// colour or a calculation, are errors.
func inlineAssets(css, cssPath string, read func(rel string) ([]byte, error)) (string, error) {
	return scanCSS(css, cssPath, func(ref, label string) (string, error) {
		return inlineURL(ref, label, cssPath, read)
	})
}

// checkInline refuses an assembled stylesheet that still holds a URL to
// fetch. Each CSS file was inlined alone, but the page carries them joined,
// and a file that ends inside an open image-set( or src( makes strings in
// the file after it into URLs that its own scan never saw as such.
// Scanning the joined sheet as the browser will read it closes that gap;
// every URL in it should already be a data: URI or a fragment.
func checkInline(css string) error {
	_, err := scanCSS(css, "stylesheet", func(ref, label string) (string, error) {
		return "", fmt.Errorf("stylesheet: %s is not inline once the style's files are joined; "+
			"a CSS file likely ends inside an unclosed function or block", label)
	})
	return err
}

// scanCSS tokenizes css and hands each URL a browser would fetch to
// resolve, which returns its replacement, a data: URI. A fragment
// reference or a data: URL is left as written without asking. Everything
// else is copied byte for byte, so a stylesheet with nothing to inline
// comes back unchanged.
//
// It also refuses "</style" anywhere, comments and strings included. The
// page writes the stylesheet raw into a <style> element, and the HTML
// parser ends that element at "</style" without knowing CSS syntax, so a
// plugin could otherwise close it and write markup, a script included,
// into every page. A stylesheet the user passes with --css is not
// scanned: it is the user's own file, written into their own pages.
func scanCSS(css, where string, resolve func(ref, label string) (string, error)) (string, error) {
	if strings.Contains(strings.ToLower(css), "</style") {
		return "", fmt.Errorf("%s: contains </style, which would end the page's <style> element", where)
	}
	s := &cssScanner{css: css, where: where, resolve: resolve}
	if err := s.run(); err != nil {
		return "", err
	}
	return s.out.String(), nil
}

// cssScanner follows CSS Syntax Level 3 §4, the tokenizer, for the tokens
// that decide what a browser fetches: comments, strings, escapes, names,
// function and url tokens, and at-keywords. It works on the original bytes
// rather than the preprocessed stream §3.3 describes, so that output can
// copy them unchanged, and instead treats \r, \r\n and \f as the newlines
// preprocessing would have made them.
//
// Names are compared after decoding escapes, as the browser compares them:
// \75 rl( is url(, and @\69mport is @import. A function's arguments end at
// its own closing parenthesis, found by keeping the same stack of open
// blocks the browser's parser keeps, so the scan always knows whether a
// string sits inside image-set( and is a URL.
type cssScanner struct {
	css     string
	i       int
	out     strings.Builder
	where   string
	resolve func(ref, label string) (string, error)

	// stack holds the blocks and functions open at i, innermost last: "(",
	// "[" or "{" for a block, a function's lowercased name for a function.
	stack []string
}

// urlFunctions take URLs as arguments: url() and src() one string,
// image-set() any number of candidates. image() is CSS Images 4's, which no
// browser ships yet; it is here so that one shipping it adds no way out.
var urlFunctions = map[string]bool{"url": true, "src": true, "image": true, "image-set": true, "-webkit-image-set": true}

// substitutions are the functions whose value is not known until the page
// is rendered. Inside a URL function one could supply a URL string the scan
// never sees, as var(--u) does with --u: "http://…", so there they are
// refused. Custom functions, named with a leading "--", are refused too.
var substitutions = map[string]bool{"var": true, "env": true, "attr": true, "if": true, "inherit": true}

// opaqueFunctions are the functions a substitution may sit in inside a URL
// function, because whatever it supplies them, they yield a gradient, a
// number, a colour or a media type, never a URL or an image candidate.
// Every other function is taken to pass its arguments through, as
// light-dark() and random-item() do, handing a substitution in them on to
// the URL function around them. The list names what is known to be safe
// rather than what is known to be dangerous, so that a function a browser
// adds later is refused until it is added here, rather than let through
// until someone notices it fetches.
var opaqueFunctions = map[string]bool{
	"linear-gradient": true, "radial-gradient": true, "conic-gradient": true,
	"repeating-linear-gradient": true, "repeating-radial-gradient": true, "repeating-conic-gradient": true,
	"-webkit-linear-gradient": true, "-webkit-radial-gradient": true, "-webkit-conic-gradient": true,
	"-webkit-repeating-linear-gradient": true, "-webkit-repeating-radial-gradient": true, "-webkit-repeating-conic-gradient": true,

	"calc": true, "min": true, "max": true, "clamp": true, "round": true, "mod": true, "rem": true,
	"abs": true, "sign": true, "sin": true, "cos": true, "tan": true, "asin": true, "acos": true,
	"atan": true, "atan2": true, "pow": true, "sqrt": true, "hypot": true, "log": true, "exp": true,

	"rgb": true, "rgba": true, "hsl": true, "hsla": true, "hwb": true, "lab": true, "lch": true,
	"oklab": true, "oklch": true, "color": true, "color-mix": true,

	"type": true,
}

func (s *cssScanner) run() error {
	css := s.css
	for s.i < len(css) {
		start := s.i
		c := css[s.i]
		switch {
		case c == '/' && s.at(s.i+1) == '*':
			end := strings.Index(css[s.i+2:], "*/")
			if end < 0 {
				s.i = len(css)
			} else {
				s.i += 2 + end + 2
			}
		case isCSSSpace(c):
			s.i++
		case c == '"' || c == '\'':
			val, end, bad := s.consumeString(s.i)
			s.i = end
			if s.inURLFunction() {
				if bad {
					return fmt.Errorf("%s: unclosed string in %s(", s.where, s.urlFunction())
				}
				out, keep, err := s.ref(val, fmt.Sprintf("%s(%q)", s.urlFunction(), val))
				if err != nil {
					return err
				}
				if !keep {
					s.out.WriteString(`"` + out + `"`)
					continue
				}
			}
		case c == '#':
			if s.i+1 < len(css) && isNameByte(css[s.i+1]) || s.validEscape(s.i+1) {
				_, s.i = s.consumeName(s.i + 1)
			} else {
				s.i++
			}
		case c == '(' || c == '[' || c == '{':
			s.stack = append(s.stack, string(c))
			s.i++
		case c == ')' || c == ']' || c == '}':
			s.close(c)
			s.i++
		case c == '+' || c == '.':
			if s.startsNumber(s.i) {
				s.consumeNumeric()
			} else {
				s.i++
			}
		case c == '-':
			switch {
			case s.startsNumber(s.i):
				s.consumeNumeric()
			case strings.HasPrefix(css[s.i:], "-->"):
				s.i += 3
			case s.startsIdent(s.i):
				replaced, err := s.consumeIdentLike()
				if err != nil {
					return err
				}
				if replaced {
					continue
				}
			default:
				s.i++
			}
		case c == '<':
			// <!-- is a token of its own, CDO. Read as a delimiter, its
			// "--" would start an identifier and hide a url( behind it.
			if strings.HasPrefix(css[s.i:], "<!--") {
				s.i += 4
			} else {
				s.i++
			}
		case c == '@':
			if s.startsIdent(s.i + 1) {
				name, end := s.consumeName(s.i + 1)
				if asciiLower(name) == "import" {
					return fmt.Errorf("%s: @import is not allowed; add the file to the style's css list instead", s.where)
				}
				s.i = end
			} else {
				s.i++
			}
		case c >= '0' && c <= '9':
			s.consumeNumeric()
		case c == '\\' && !s.validEscape(s.i):
			s.i++
		case c == '\\' || isNameStartByte(c):
			replaced, err := s.consumeIdentLike()
			if err != nil {
				return err
			}
			if replaced {
				continue
			}
		default:
			s.i++
		}
		s.out.WriteString(css[start:s.i])
	}
	return nil
}

// consumeIdentLike reads an identifier, function or url token at s.i. A
// url token whose URL is replaced is written to the output here, and
// replaced reports it; otherwise the caller copies the token as written.
func (s *cssScanner) consumeIdentLike() (replaced bool, err error) {
	name, end := s.consumeName(s.i)
	s.i = end
	if s.at(s.i) != '(' {
		return false, nil
	}
	s.i++
	fn := asciiLower(name)
	if fn == "url" {
		j := s.i
		for j < len(s.css) && isCSSSpace(s.css[j]) {
			j++
		}
		// url( followed by a quote is a function taking a string, which
		// the string case handles; otherwise the URL is unquoted and the
		// whole url(…) is one token.
		if q := s.at(j); q != '"' && q != '\'' {
			val, end, err := s.consumeURL(s.i)
			if err != nil {
				return false, err
			}
			s.i = end
			out, keep, err := s.ref(val, "url("+val+")")
			if err != nil || keep {
				return false, err
			}
			s.out.WriteString(`url("` + out + `")`)
			return true, nil
		}
	}
	if substitutions[fn] || strings.HasPrefix(fn, "--") {
		if outer := s.exposingURLFunction(); outer != "" {
			return false, fmt.Errorf("%s: %s() inside %s() cannot be checked for a URL to fetch; write the URL itself",
				s.where, fn, outer)
		}
	}
	s.stack = append(s.stack, fn)
	return false, nil
}

// ref decides what one URL becomes. keep means leave it as written.
func (s *cssScanner) ref(val, label string) (out string, keep bool, err error) {
	if strings.HasPrefix(val, "#") || hasPrefixFold(val, "data:") {
		return "", true, nil
	}
	out, err = s.resolve(val, label)
	return out, false, err
}

// close pops the innermost open block if c closes it. A closer that does
// not match is an ordinary token to the browser, so it changes nothing.
func (s *cssScanner) close(c byte) {
	if len(s.stack) == 0 {
		return
	}
	top := s.stack[len(s.stack)-1]
	switch {
	case c == ']' && top == "[", c == '}' && top == "{", c == ')' && top != "[" && top != "{":
		s.stack = s.stack[:len(s.stack)-1]
	}
}

// urlFunction names the URL function a string at s.i is an argument of,
// or "" when it is not one. The innermost function decides, except that a
// string nested in some other function inside image-set() still counts,
// since if() and its kin can hand it to image-set() as a candidate. The
// exception is type(), whose string is a media type.
func (s *cssScanner) urlFunction() string {
	for k := len(s.stack) - 1; k >= 0; k-- {
		switch f := s.stack[k]; {
		case f == "type":
			return ""
		case urlFunctions[f]:
			return f
		}
	}
	return ""
}

func (s *cssScanner) inURLFunction() bool { return s.urlFunction() != "" }

// exposingURLFunction names the URL function a substitution at s.i could
// hand a URL to, or "" when there is none. That is the nearest URL function
// around it, unless an opaque function sits between them: a substitution in
// linear-gradient() inside image-set() can only ever supply that gradient's
// arguments, never an image-set() candidate. Any other function between
// them, and any bare block, is looked through, since light-dark(var(--u),
// x) inside image-set() is var(--u) there in a light page, and nesting one
// pass-through in another changes nothing.
func (s *cssScanner) exposingURLFunction() string {
	for k := len(s.stack) - 1; k >= 0; k-- {
		switch f := s.stack[k]; {
		case opaqueFunctions[f]:
			return ""
		case urlFunctions[f]:
			return f
		}
	}
	return ""
}

// consumeString reads the string token starting at i, its opening quote,
// and returns its value with escapes decoded and where it ends. A newline
// ends it unclosed, which CSS calls a bad string, and the newline is left
// for the next token; the end of the input closes it.
func (s *cssScanner) consumeString(i int) (val string, end int, bad bool) {
	q := s.css[i]
	var b strings.Builder
	for i++; i < len(s.css); {
		c := s.css[i]
		switch {
		case c == q:
			return b.String(), i + 1, false
		case isCSSNewline(c):
			return b.String(), i, true
		case c == '\\':
			switch {
			case i+1 >= len(s.css):
				i++
			case isCSSNewline(s.css[i+1]):
				// An escaped newline continues the string onto the next line.
				i += 1 + newlineLen(s.css, i+1)
			default:
				var r rune
				r, i = s.consumeEscape(i + 1)
				b.WriteRune(r)
			}
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String(), i, false
}

// consumeURL reads an unquoted url token's value from i, just past the
// "(", through the closing ")". CSS turns a quote, "(", a control
// character or space inside the URL into a bad url token, which a browser
// drops; one is refused here rather than copied, so the scan never has to
// agree with the browser on where a bad token ends.
func (s *cssScanner) consumeURL(i int) (val string, end int, err error) {
	var b strings.Builder
	for i < len(s.css) && isCSSSpace(s.css[i]) {
		i++
	}
	for i < len(s.css) {
		c := s.css[i]
		switch {
		case c == ')':
			return b.String(), i + 1, nil
		case isCSSSpace(c):
			for i < len(s.css) && isCSSSpace(s.css[i]) {
				i++
			}
			if i < len(s.css) && s.css[i] == ')' {
				return b.String(), i + 1, nil
			}
			if i < len(s.css) {
				return "", 0, fmt.Errorf("%s: malformed url(%s…); quote a URL that has spaces in it", s.where, b.String())
			}
		case c == '"' || c == '\'' || c == '(' || isNonPrintable(c):
			return "", 0, fmt.Errorf("%s: malformed url(%s…); quote a URL that has quotes or parentheses in it", s.where, b.String())
		case c == '\\':
			if !s.validEscape(i) {
				return "", 0, fmt.Errorf("%s: malformed url(%s…)", s.where, b.String())
			}
			var r rune
			r, i = s.consumeEscape(i + 1)
			b.WriteRune(r)
		default:
			b.WriteByte(c)
			i++
		}
	}
	return "", 0, fmt.Errorf("%s: unclosed url(", s.where)
}

// consumeName reads a name from i, decoding escapes, and returns it and
// where it ends. Bytes of non-ASCII characters are copied as they are:
// the names that matter are all ASCII, so only a match against them is
// ever asked of one.
func (s *cssScanner) consumeName(i int) (string, int) {
	var b strings.Builder
	for i < len(s.css) {
		switch c := s.css[i]; {
		case isNameByte(c):
			b.WriteByte(c)
			i++
		case s.validEscape(i):
			var r rune
			r, i = s.consumeEscape(i + 1)
			b.WriteRune(r)
		default:
			return b.String(), i
		}
	}
	return b.String(), i
}

// consumeEscape decodes the escape whose backslash is just before i: up to
// six hex digits and one optional whitespace, or any one character.
func (s *cssScanner) consumeEscape(i int) (rune, int) {
	if i >= len(s.css) {
		return utf8.RuneError, i
	}
	if isHex(s.css[i]) {
		var v rune
		n := 0
		for ; n < 6 && i < len(s.css) && isHex(s.css[i]); n++ {
			v = v*16 + hexVal(s.css[i])
			i++
		}
		if i < len(s.css) && isCSSSpace(s.css[i]) {
			i += newlineLen(s.css, i)
		}
		if v == 0 || v > utf8.MaxRune || (v >= 0xD800 && v <= 0xDFFF) {
			v = utf8.RuneError
		}
		return v, i
	}
	r, size := utf8.DecodeRuneInString(s.css[i:])
	if r == 0 {
		r = utf8.RuneError
	}
	return r, i + size
}

// consumeNumeric reads a number and its unit or percent sign, so that a
// unit spelled "url", as in 1\75 rl(, is not mistaken for a function.
func (s *cssScanner) consumeNumeric() {
	css := s.css
	i := s.i
	if css[i] == '+' || css[i] == '-' {
		i++
	}
	for i < len(css) && isDigit(css[i]) {
		i++
	}
	if i+1 < len(css) && css[i] == '.' && isDigit(css[i+1]) {
		for i++; i < len(css) && isDigit(css[i]); i++ {
		}
	}
	if i < len(css) && (css[i] == 'e' || css[i] == 'E') {
		j := i + 1
		if j < len(css) && (css[j] == '+' || css[j] == '-') {
			j++
		}
		if j < len(css) && isDigit(css[j]) {
			for i = j; i < len(css) && isDigit(css[i]); i++ {
			}
		}
	}
	switch {
	case s.startsIdent(i):
		_, i = s.consumeName(i)
	case i < len(css) && css[i] == '%':
		i++
	}
	s.i = i
}

// at returns the byte at i, or 0 past the end.
func (s *cssScanner) at(i int) byte {
	if i < len(s.css) {
		return s.css[i]
	}
	return 0
}

// validEscape reports whether a backslash at i starts an escape: one not
// followed by a newline.
func (s *cssScanner) validEscape(i int) bool {
	return i < len(s.css) && s.css[i] == '\\' && (i+1 >= len(s.css) || !isCSSNewline(s.css[i+1]))
}

// startsIdent reports whether an identifier starts at i (§4.3.9).
func (s *cssScanner) startsIdent(i int) bool {
	if i >= len(s.css) {
		return false
	}
	switch c := s.css[i]; {
	case c == '-':
		n := s.at(i + 1)
		return i+1 < len(s.css) && (isNameStartByte(n) || n == '-') || s.validEscape(i+1)
	case isNameStartByte(c):
		return true
	case c == '\\':
		return s.validEscape(i)
	}
	return false
}

// startsNumber reports whether a number starts at i (§4.3.10).
func (s *cssScanner) startsNumber(i int) bool {
	if c := s.at(i); c == '+' || c == '-' {
		i++
	}
	if i >= len(s.css) {
		return false
	}
	if isDigit(s.css[i]) {
		return true
	}
	return s.css[i] == '.' && i+1 < len(s.css) && isDigit(s.css[i+1])
}

// inlineURL returns the data: URI for one URL, which is neither a fragment
// reference nor a data: URL. label is how the CSS wrote it, for messages.
func inlineURL(ref, label, cssPath string, read func(string) ([]byte, error)) (string, error) {
	if hasScheme(ref) {
		return "", fmt.Errorf("%s: %s is not a relative path inside the plugin; a plugin cannot load anything at view time", cssPath, label)
	}
	file, frag, _ := strings.Cut(ref, "#")
	file, _, _ = strings.Cut(file, "?")
	rel := path.Clean(path.Join(path.Dir(cssPath), file))
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("%s: %s is outside the plugin", cssPath, label)
	}
	mime, ok := assetTypes[strings.ToLower(path.Ext(rel))]
	if !ok {
		return "", fmt.Errorf("%s: %s: %q is not a font or image type md2html can inline", cssPath, label, path.Ext(rel))
	}
	data, err := read(rel)
	if err != nil {
		return "", fmt.Errorf("%s: %s: %w", cssPath, label, err)
	}
	uri := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	if frag != "" {
		uri += "#" + escapeFragment(frag)
	}
	return uri, nil
}

// hasScheme reports whether a URL is anything but a relative path: one
// with a scheme, or one starting with a slash or backslash, which a browser
// reads as a path from the server's root or, doubled, as another host.
// Leading spaces and control characters are dropped first, and tabs and
// newlines anywhere, as a browser's URL parser drops them.
func hasScheme(ref string) bool {
	ref = strings.TrimLeft(ref, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\t\n\x0b\x0c\r\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f ")
	ref = strings.NewReplacer("\t", "", "\n", "", "\r", "").Replace(ref)
	if strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, `\`) {
		return true
	}
	first := ref
	if k := strings.IndexAny(ref, `/\?#`); k >= 0 {
		first = ref[:k]
	}
	return strings.Contains(first, ":")
}

// escapeFragment percent-encodes all but the unreserved characters of a
// fragment copied onto a data: URI. The URI is written inside url("…"), and
// a quote, backslash or parenthesis left in the fragment could end that
// token and start CSS of the plugin's choosing after it.
func escapeFragment(frag string) string {
	var b strings.Builder
	for i := 0; i < len(frag); i++ {
		c := frag[i]
		if isAlnum(c) || c == '.' || c == '_' || c == '~' || c == '-' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// newlineLen is how many bytes the whitespace at i takes: two for \r\n,
// which preprocessing makes one newline, otherwise one.
func newlineLen(s string, i int) int {
	if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
		return 2
	}
	return 1
}

func isCSSNewline(c byte) bool { return c == '\n' || c == '\r' || c == '\f' }

func isCSSSpace(c byte) bool { return isCSSNewline(c) || c == ' ' || c == '\t' }

// isNameStartByte reports whether c can start a CSS name. Any byte of a
// non-ASCII character can, and so can NUL, which preprocessing turns into
// U+FFFD.
func isNameStartByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80 || c == 0
}

func isNameByte(c byte) bool { return isNameStartByte(c) || isDigit(c) || c == '-' }

func isNonPrintable(c byte) bool {
	return c <= 0x08 && c != 0 || c == 0x0b || c >= 0x0e && c <= 0x1f || c == 0x7f
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isAlnum(c byte) bool { return isDigit(c) || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func isHex(c byte) bool { return isDigit(c) || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }

func hexVal(c byte) rune {
	switch {
	case isDigit(c):
		return rune(c - '0')
	case c >= 'a':
		return rune(c-'a') + 10
	}
	return rune(c-'A') + 10
}

// asciiLower lowercases ASCII letters only. CSS names compare ASCII
// case-insensitively; full Unicode folding would equate "ſrc" with "src",
// which the browser does not.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}
