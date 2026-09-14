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
// It is braceBlock with the head text in place of the brace's index. A
// caller that needs the index instead — a container fence line, which has
// to know where the title in front of the block ends — calls braceBlock.
func splitBraced(s string) (head, content string, ok bool) {
	open, content, ok := braceBlock(s)
	if !ok {
		return s, "", false
	}
	return strings.TrimSpace(s[:open]), content, true
}

// braceBlock finds a trailing {...} attribute block, reporting the index of
// its opening brace along with the block's contents. open is -1 when it
// reports false.
//
// This is the one place the boundary of an attribute block is decided, so a
// container's fence line, a fenced code block's info string and a bracketed
// span cannot disagree about it — and the position is reported here rather
// than recomputed by a caller. Deriving it from len(content) instead is what
// silently truncated a container's title when the fence line ended in a
// non-ASCII space: goldmark's own trimmer is ASCII-only, so those bytes
// reach the grammar and shift every length-derived index.
func braceBlock(s string) (open int, content string, ok bool) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"', '\'':
			i = skipQuoted(s, i)
		case '{':
			end := braceSpan(s, i)
			if end < 0 {
				// The block is never closed, and no later "{" can open
				// one that is: this one would have to close first.
				return -1, "", false
			}
			if strings.TrimSpace(s[end+1:]) == "" {
				return i, s[i+1 : end], true
			}
			i = end
		}
	}
	return -1, "", false
}

// braceSpan reports the index of the "}" that closes the "{" at s[open], or
// -1 if s[open] is not a "{" or is never closed.
//
// The scan runs forward rather than searching backward for "}", tracking
// quotes and backslash escapes so a brace inside a value — caption="a } b"
// — does not end the block early, and tracking nesting so a brace inside
// the block's own content — {a{b}} — belongs to the block rather than
// opening a shorter one, which would leave the text in front of it to leak
// out as a title.
func braceSpan(s string, open int) int {
	if open < 0 || open >= len(s) || s[open] != '{' {
		return -1
	}
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"', '\'':
			i = skipQuoted(s, i)
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// skipQuoted returns the index of the quote closing the run that opens at
// s[i], or one past the end of s if it is never closed. A backslash makes
// the next byte literal, so a value can contain a quote of its own.
func skipQuoted(s string, i int) int {
	quote := s[i]
	i++
	for i < len(s) && s[i] != quote {
		if s[i] == '\\' {
			i++
		}
		i++
	}
	return i
}
