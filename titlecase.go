package md2html

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// titleCase reads a directory's name as a title: words split on -, _ and
// spaces, each with its first letter upper-cased, and a word fixed lists,
// by its lower-cased form, spelled as listed there.
func titleCase(name string, fixed map[string]string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' || unicode.IsSpace(r) })
	for i, w := range words {
		if f, ok := fixed[strings.ToLower(w)]; ok {
			words[i] = f
			continue
		}
		r, size := utf8.DecodeRuneInString(w)
		words[i] = string(unicode.ToUpper(r)) + w[size:]
	}
	return strings.Join(words, " ")
}

// titleCaseWords reads a title-case: value, a comma-separated list of words
// that keep a fixed spelling, into the map titleCase looks them up in.
func titleCaseWords(v string) map[string]string {
	m := map[string]string{}
	for _, w := range strings.Split(v, ",") {
		if w = strings.TrimSpace(w); w != "" {
			m[strings.ToLower(w)] = w
		}
	}
	return m
}
