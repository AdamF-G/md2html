package stylecheck

import (
	"regexp"
	"strings"
)

// block kinds: the four theme blocks the guide has a style write, and a
// bare :root in the dark media query, which people often write instead.
const (
	rootBlock     = iota // :root
	darkRootBlock        // :root in the dark media query
	mediaBlock           // :root:not([data-theme="light"]) in the dark media query
	lightBlock           // :root[data-theme="light"]
	darkBlock            // :root[data-theme="dark"]
)

// view is one way a reader can see the page: their system's theme and
// what, if anything, they picked.
type view struct {
	name   string
	blocks []int
}

var (
	viewL1 = view{"light system, nothing picked", []int{rootBlock}}
	viewL2 = view{"light picked", []int{rootBlock, lightBlock}}
	viewL3 = view{"light picked on a dark system", []int{rootBlock, darkRootBlock, lightBlock}}
	viewD1 = view{"dark system, nothing picked", []int{rootBlock, darkRootBlock, mediaBlock}}
	viewD2 = view{"dark picked on a light system", []int{rootBlock, darkBlock}}
	viewD3 = view{"dark picked on a dark system", []int{rootBlock, darkRootBlock, mediaBlock, darkBlock}}
)

var (
	declRe      = regexp.MustCompile(`(?s)^\s*--([A-Za-z0-9_-]+)\s*:(.*)$`)
	varRe       = regexp.MustCompile(`^var\(\s*--([A-Za-z0-9_-]+)\s*\)$`)
	importantRe = regexp.MustCompile(`(?i)\s*!\s*important\s*$`)
	// The dark query is the dark scheme alone, or after "screen and" or
	// "all and", either with "only" before it: any other condition limits
	// when it applies, so its blocks are not a theme every dark reader sees.
	darkMediaRe = regexp.MustCompile(`(?i)^@media(\s*|\s+(only\s+)?(screen|all)\s+and\s*)\(\s*prefers-color-scheme\s*:\s*dark\s*\)$`)
	wrapperRe   = regexp.MustCompile(`(?i)^@(supports|layer)\b`)
	// :root, :not and the attribute name match in any case, as in CSS;
	// the theme names are attribute values, which are case-sensitive.
	selectorRe = regexp.MustCompile(`^(?i::root)((?i::not)\(\[(?i:data-theme)=["']?light["']?\]\)|\[(?i:data-theme)=["']?(light|dark)["']?\])?$`)
)

type decl struct {
	kind  int
	name  string
	value string
}

// blockKind says which theme block a selector is, or false for any other
// selector. Inside the dark media query a bare :root is its own block: it
// has no :not guard, and it is no more specific than a bare :root.
func blockKind(selector string, inDark bool) (int, bool) {
	m := selectorRe.FindStringSubmatch(strings.TrimSpace(selector))
	switch {
	case m == nil:
		return 0, false
	case m[2] == "light":
		return lightBlock, true
	case m[2] == "dark":
		return darkBlock, true
	case m[1] != "":
		return mediaBlock, true
	case inDark:
		return darkRootBlock, true
	}
	return rootBlock, true
}

// at-rule kinds, for the at-rules a theme block can sit in.
const (
	wrapperRule = iota // @supports or @layer: read as if it were not there
	darkRule           // the dark colour-scheme query
	otherRule          // anything else, such as @media print: not a theme
)

// readBlocks lists every custom property the theme blocks declare, in
// source order. A selector list gives its declarations to each block it
// names. A theme selector inside any other at-rule, such as @media print,
// is not a theme block.
func readBlocks(css string) []decl {
	sc := &scanner{css: stripComments(css)}
	var out []decl
	sc.rules(&out)
	return out
}

// scanner reads a stylesheet's structure. A brace or semicolon inside a
// quoted string, or escaped, is text, not structure.
type scanner struct {
	css string
	pos int
}

// until reads up to the next byte in stops, and returns the text before
// it and the stop, or 0 at the end of the sheet.
func (s *scanner) until(stops string) (string, byte) {
	start := s.pos
	for s.pos < len(s.css) {
		c := s.css[s.pos]
		switch {
		case c == '\\':
			s.pos += 2
			continue
		case c == '"' || c == '\'':
			s.pos = skipString(s.css, s.pos)
			continue
		case strings.IndexByte(stops, c) >= 0:
			s.pos++
			return s.css[start : s.pos-1], c
		}
		s.pos++
	}
	s.pos = len(s.css)
	return s.css[start:], 0
}

// skipBlock skips past the } that closes a block whose { was just read.
func (s *scanner) skipBlock() {
	for depth := 1; depth > 0; {
		switch _, c := s.until("{}"); c {
		case 0:
			return
		case '{':
			depth++
		case '}':
			depth--
		}
	}
}

// rules reads every rule in the sheet. It keeps the open at-rules on a
// stack, not the Go stack, since a plugin's sheet can nest them as deep
// as it likes, and counts each kind, so asking what a rule sits in costs
// the same at any depth.
func (s *scanner) rules(out *[]decl) {
	var open []int // the kind of each open at-rule
	var count [3]int
	for {
		prelude, stop := s.until("{};")
		switch stop {
		case 0:
			return
		case '}': // the end of an at-rule, or a stray } to read past
			if len(open) > 0 {
				count[open[len(open)-1]]--
				open = open[:len(open)-1]
			}
			continue
		case ';': // a statement, such as @import
			continue
		}
		prelude = strings.TrimSpace(prelude)
		if strings.HasPrefix(prelude, "@") {
			kind := otherRule
			switch {
			case darkMediaRe.MatchString(prelude):
				kind = darkRule
			case wrapperRe.MatchString(prelude):
				kind = wrapperRule
			}
			open = append(open, kind)
			count[kind]++
			continue
		}
		body := s.declarations()
		if count[otherRule] > 0 {
			continue
		}
		seen := map[int]bool{}
		for _, sel := range strings.Split(prelude, ",") {
			kind, ok := blockKind(sel, count[darkRule] > 0)
			if !ok || seen[kind] {
				continue
			}
			seen[kind] = true
			for _, d := range body {
				*out = append(*out, decl{kind, d[0], d[1]})
			}
		}
	}
}

// declarations reads a style rule's custom properties, name and value, up
// to the } that closes it. A nested rule is skipped whole: its
// declarations apply to other elements, or only under its condition.
func (s *scanner) declarations() [][2]string {
	var out [][2]string
	for {
		text, stop := s.until("{};")
		if stop == '{' {
			s.skipBlock()
			continue
		}
		if m := declRe.FindStringSubmatch(text); m != nil {
			out = append(out, [2]string{m[1], importantRe.ReplaceAllString(strings.TrimSpace(m[2]), "")})
		}
		if stop != ';' {
			return out
		}
	}
}

// skipString returns the index just past the string that starts at i: its
// closing quote, or the end of its line if it has none.
func skipString(css string, i int) int {
	q := css[i]
	for i++; i < len(css); i++ {
		switch css[i] {
		case '\\':
			i++
		case q:
			return i + 1
		case '\n':
			return i
		}
	}
	return len(css)
}

// stripComments removes comments, leaving any /* inside a string or an
// unquoted url() alone: CSS reads each as one token.
func stripComments(css string) string {
	var b strings.Builder
	for i := 0; i < len(css); {
		if n := unquotedURL(css, i); n > 0 {
			b.WriteString(css[i : i+n])
			i += n
			continue
		}
		switch {
		case css[i] == '"' || css[i] == '\'':
			j := skipString(css, i)
			b.WriteString(css[i:j])
			i = j
		case css[i] == '\\' && i+1 < len(css):
			b.WriteString(css[i : i+2])
			i += 2
		case strings.HasPrefix(css[i:], "/*"):
			end := strings.Index(css[i+2:], "*/")
			if end < 0 {
				return b.String()
			}
			i += end + 4
		default:
			b.WriteByte(css[i])
			i++
		}
	}
	return b.String()
}

// winners is the index in decls of the declaration each custom property
// takes in v. A bare :root, in the dark query or not, loses to the other
// three selectors, which tie, and among ties the later declaration wins,
// as the cascade decides.
func (v view) winners(decls []decl) map[string]int {
	applies := map[int]bool{}
	for _, b := range v.blocks {
		applies[b] = true
	}
	win := map[string]int{}
	spec := map[string]int{}
	for i, d := range decls {
		if !applies[d.kind] {
			continue
		}
		s := 2
		if d.kind == rootBlock || d.kind == darkRootBlock {
			s = 1
		}
		if s >= spec[d.name] {
			win[d.name], spec[d.name] = i, s
		}
	}
	return win
}

// values is what each custom property resolves to in v. var() references
// resolve in the same view.
func (v view) values(decls []decl) map[string]string {
	val := map[string]string{}
	for name, i := range v.winners(decls) {
		val[name] = decls[i].value
	}
	out := make(map[string]string, len(val))
	for k, x := range val {
		for range 8 { // bounded, so a reference cycle cannot loop
			m := varRe.FindStringSubmatch(x)
			if m == nil {
				break
			}
			next, ok := val[m[1]]
			if !ok {
				break
			}
			x = next
		}
		out[k] = x
	}
	return out
}

// unquotedURL is the length of the unquoted url() token at i, up to and
// including its ), or 0 if none starts there. A backslash escapes the
// character after it, so \) does not end the token. A url( whose
// argument is quoted is a function holding a string, read as any other.
func unquotedURL(css string, i int) int {
	if len(css)-i < 4 || !strings.EqualFold(css[i:i+4], "url(") {
		return 0
	}
	if i > 0 && isNameByte(css[i-1]) { // e.g. myurl(
		return 0
	}
	j := i + 4
	for j < len(css) && strings.IndexByte(" \t\n\r\f", css[j]) >= 0 {
		j++
	}
	if j < len(css) && (css[j] == '"' || css[j] == '\'') {
		return 0
	}
	for ; j < len(css); j++ {
		switch css[j] {
		case '\\': // an escape: the next character is part of the url
			j++
		case ')':
			return j + 1 - i
		}
	}
	return len(css) - i
}

func isNameByte(c byte) bool {
	return c == '-' || c == '_' || c == '\\' || c >= 0x80 ||
		'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}
