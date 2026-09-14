package md2html

import (
	"sort"
	"strings"

	fences "github.com/AdamF-G/md2html/internal/fences"
)

// fenceInfoResult is the vendored package's Info under a local name, so
// tests and helpers in this package need not import it.
type fenceInfoResult = fences.Info

// parseFenceInfo splits a container's fence-line remainder — everything
// after the colons, already trimmed — into its kind word, its attributes
// and the source range of its title.
//
// Every spelling but one is owned: the braced form, the label form and the
// bare form all report true. It reports false only when the line opens with
// "{" but never closes it — a malformed braced form with nothing else to
// fall back to.
//
// Offsets rather than a title string: the parser points a source segment at
// the range so goldmark parses the title's inline markup natively, which is
// what lets a title hold code spans and emphasis without this package
// converting Markdown a second time.
func parseFenceInfo(info string) (fenceInfoResult, bool) {
	out := fenceInfoResult{TitleStart: -1, TitleEnd: -1}

	if strings.HasPrefix(info, "{") {
		content, rest, ok := readBracedPrefix(info)
		if !ok {
			return out, false
		}
		out.Attrs = fenceAttrs(content)
		if t := strings.TrimLeft(rest, " \t"); t != "" {
			out.TitleStart = len(info) - len(t)
			out.TitleEnd = len(info)
		}
		return out, true
	}

	// Label form: kind[Title], with an optional trailing attribute block.
	if open := strings.IndexByte(info, '['); open > 0 && !strings.ContainsAny(info[:open], " \t{") {
		if close := strings.IndexByte(info[open+1:], ']'); close >= 0 {
			close += open + 1
			tail := strings.TrimLeft(info[close+1:], " \t")
			var attrs []fences.Attr
			owned := tail == ""
			if !owned {
				if _, content, ok := splitBraced(tail); ok {
					attrs, owned = fenceAttrs(content), true
				}
			}
			if owned {
				out.Kind = info[:open]
				out.TitleStart, out.TitleEnd = open+1, close
				out.Attrs = attrs
				return out, true
			}
		}
	}

	// Bare form: a kind word, then an optional undelimited title, then an
	// optional trailing attribute block.
	body := info
	if start, content, ok := braceBlock(info); ok && start > 0 {
		out.Attrs = fenceAttrs(content)
		body = strings.TrimRight(info[:start], " \t")
	}
	word := body
	if i := strings.IndexAny(body, " \t"); i >= 0 {
		word = body[:i]
		if t := strings.TrimLeft(body[i:], " \t"); t != "" {
			out.TitleStart = len(body) - len(t)
			out.TitleEnd = len(body)
		}
	}
	out.Kind = word
	return out, true
}

// readBracedPrefix reads a leading {...} attribute block, returning its
// contents and whatever followed it.
//
// The boundary is braceSpan's — the same scan braceBlock uses for a trailing
// block — so a leading and a trailing block cannot disagree about where a
// quoted or a nested brace leaves off.
func readBracedPrefix(s string) (content, rest string, ok bool) {
	end := braceSpan(s, 0)
	if end < 0 {
		return "", s, false
	}
	return s[1:end], s[end+1:], true
}

// fenceAttrs turns an attribute block's contents into the attributes a
// container element carries. It is the shared {#id .class key=value} parser,
// so a container, a bracketed span and a fenced code block all read the
// same grammar.
func fenceAttrs(content string) []fences.Attr {
	a, ok := parseAttrs(content)
	if !ok {
		return nil
	}
	var out []fences.Attr
	if a.id != "" {
		out = append(out, fences.Attr{Name: "id", Value: a.id})
	}
	if len(a.classes) > 0 {
		out = append(out, fences.Attr{Name: "class", Value: strings.Join(a.classes, " ")})
	}
	keys := make([]string, 0, len(a.kv))
	for k := range a.kv {
		if safeAttrName(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, fences.Attr{Name: k, Value: a.kv[k]})
	}
	return out
}

// safeAttrName reports whether name can be written into an HTML attribute
// list verbatim.
//
// The renderer escapes attribute *values* and writes *names* as they are —
// goldmark's does too — so a name is the one piece of author text that
// reaches the output unescaped. attrTokens accepts any byte in a key except
// whitespace and "=", so `data-x"onmouseover="alert(1)` parses as a single
// key, and writing it out would close the attribute and turn the remainder
// into an event handler. Until now goldmark's own ParseAttributes stood in
// the way and rejected such a block; this package's parser is deliberately
// more permissive, so the guard has to move here with it.
//
// A whitelist rather than a blacklist: an HTML attribute name has no
// business containing anything outside this set.
func safeAttrName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			// A letter is legal anywhere, including first.
		case c >= '0' && c <= '9', c == '-', c == '_', c == '.', c == ':':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
