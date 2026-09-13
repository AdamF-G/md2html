package md2html

import "strings"

// attrs is a parsed {#id .class key=value} attribute block — the syntax
// PHP Markdown Extra introduced and kramdown, Pandoc and goldmark's own
// heading attributes all share.
//
// Three constructs in this package previously did three different ad-hoc
// jobs on this one grammar: headings through goldmark's parser.WithAttribute,
// containers through the fence library, and code fences through a bespoke
// token strip. Only the first was a real parser, which is why the code
// fence form could not express a quote inside a quoted value. This is the
// one parser they now share.
type attrs struct {
	id      string
	classes []string
	// kv holds every key=value pair, and every bare key with an empty
	// value. id and class are lifted out into their own fields rather
	// than left here, so a caller never has to know both spellings.
	kv map[string]string
}

// parseAttrs reads the inside of an attribute block — the text between the
// braces, with the braces already removed. It reports false for content
// that holds no attributes at all, so a caller can leave the source text
// alone rather than emit an empty element.
//
// Both spellings of a class reach the same place: ".one" and class="one two"
// accumulate into classes in the order written. An id may be written either
// way too; the last one wins, matching the directive syntax the CommonMark
// proposal settled on and Pandoc's own behavior.
func parseAttrs(content string) (attrs, bool) {
	a := attrs{kv: map[string]string{}}
	found := false
	for _, tok := range attrTokens(content) {
		switch {
		case strings.HasPrefix(tok.key, "#") && tok.val == "":
			if name := tok.key[1:]; name != "" {
				a.id = name
				found = true
			}
		case strings.HasPrefix(tok.key, ".") && tok.val == "":
			if name := tok.key[1:]; name != "" {
				a.classes = append(a.classes, name)
				found = true
			}
		case tok.key == "id":
			a.id = tok.val
			found = true
		case tok.key == "class":
			a.classes = append(a.classes, strings.Fields(tok.val)...)
			found = true
		case tok.key != "":
			a.kv[tok.key] = tok.val
			found = true
		}
	}
	return a, found
}

// attrToken is one key, with the value that followed its "=" if any.
type attrToken struct{ key, val string }

// attrTokens splits an attribute block on whitespace, keeping a quoted
// value together and honouring a backslash escape inside one.
//
// The escape is what the old fence tokenizer lacked: caption="has \"quote\""
// used to leave the backslashes and the inner quotes as literal characters
// and terminate the value early. Here a backslash makes the next byte
// literal wherever it appears, so a value can contain a quote, a closing
// brace, or a backslash of its own.
func attrTokens(s string) []attrToken {
	var out []attrToken
	i := 0
	for i < len(s) {
		for i < len(s) && isAttrSpace(s[i]) {
			i++
		}
		if i >= len(s) {
			break
		}
		var key strings.Builder
		for i < len(s) && !isAttrSpace(s[i]) && s[i] != '=' {
			key.WriteByte(s[i])
			i++
		}
		if i < len(s) && s[i] == '=' {
			i++
			val, next := readAttrValue(s, i)
			out = append(out, attrToken{key: key.String(), val: val})
			i = next
			continue
		}
		out = append(out, attrToken{key: key.String()})
	}
	return out
}

// readAttrValue reads the value at s[i:], quoted or bare, and returns it
// along with the index just past it.
func readAttrValue(s string, i int) (string, int) {
	var v strings.Builder
	if i < len(s) && (s[i] == '"' || s[i] == '\'') {
		quote := s[i]
		i++
		for i < len(s) && s[i] != quote {
			if s[i] == '\\' && i+1 < len(s) {
				i++
			}
			v.WriteByte(s[i])
			i++
		}
		if i < len(s) {
			i++ // closing quote
		}
		return v.String(), i
	}
	for i < len(s) && !isAttrSpace(s[i]) {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		v.WriteByte(s[i])
		i++
	}
	return v.String(), i
}

func isAttrSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' }

// splitBraced finds a trailing {...} attribute block, returning the text
// before it and the block's contents. It reports false when there is no
// well-formed trailing block, in which case the caller leaves the string
// exactly as written.
//
// The scan runs forward tracking quotes rather than searching backward for
// "}", so a closing brace inside a quoted value — caption="a } b" — does
// not end the block early.
func splitBraced(s string) (head, content string, ok bool) {
	open := -1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"', '\'':
			quote := s[i]
			i++
			for i < len(s) && s[i] != quote {
				if s[i] == '\\' {
					i++
				}
				i++
			}
		case '{':
			open = i
		case '}':
			if open >= 0 && strings.TrimSpace(s[i+1:]) == "" {
				return strings.TrimSpace(s[:open]), s[open+1 : i], true
			}
		}
	}
	return s, "", false
}
