package highlight

import (
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Dump is one sample tokenized by one tokenizer. Columns are UTF-16 code
// unit offsets, since the reference tokenizer counts in JavaScript.
type Dump struct {
	Language string      `json:"language"`
	Scope    string      `json:"scope"`
	Lines    []string    `json:"lines"`
	Tokens   [][]DumpTok `json:"tokens"` // per line
}

// DumpTok is one token of a Dump.
type DumpTok struct {
	Start  int    `json:"s"`
	End    int    `json:"e"`
	Scopes string `json:"x"` // space-separated, root first
}

// DumpOf tokenizes lines with e and returns the result in Dump form. It
// keeps zero-width tokens, since the reference emits one for an empty
// line, and clamps ends to the line, as the reference script does.
func DumpOf(e Engine, language, scope string, lines []string) (Dump, error) {
	toks, err := e.Tokenize(scope, lines)
	if err != nil {
		return Dump{}, err
	}
	d := Dump{Language: language, Scope: scope, Lines: lines, Tokens: make([][]DumpTok, len(lines))}
	for i, line := range lines {
		at := utf16Offsets(line)
		row := []DumpTok{}
		for _, t := range toks[i] {
			row = append(row, DumpTok{Start: at[min(t.Start, len(line))], End: at[min(t.End, len(line))], Scopes: strings.Join(t.Scopes, " ")})
		}
		d.Tokens[i] = row
	}
	return d, nil
}

// utf16Offsets maps each byte offset of line, and its end, to a UTF-16
// code unit offset. An offset inside a rune maps to the rune's start.
func utf16Offsets(line string) []int {
	at := make([]int, len(line)+1)
	n := 0
	for i, r := range line {
		_, size := utf8.DecodeRuneInString(line[i:])
		for k := i; k < i+size; k++ {
			at[k] = n
		}
		n += utf16.RuneLen(r)
	}
	at[len(line)] = n
	return at
}

// Compare returns "" when a and b agree token for token, or a sentence
// naming the first line, column and scopes where they differ.
func Compare(want, got Dump) string {
	if len(want.Tokens) != len(got.Tokens) {
		return fmt.Sprintf("the reference has %d lines and md2html %d", len(want.Tokens), len(got.Tokens))
	}
	for i := range want.Tokens {
		w, g := want.Tokens[i], got.Tokens[i]
		for j := 0; j < max(len(w), len(g)); j++ {
			switch {
			case j >= len(g):
				return fmt.Sprintf("line %d, column %d: the reference has %q there and md2html has no token", i+1, w[j].Start+1, w[j].Scopes)
			case j >= len(w):
				return fmt.Sprintf("line %d, column %d: md2html has %q there and the reference has no token", i+1, g[j].Start+1, g[j].Scopes)
			case w[j] != g[j]:
				return fmt.Sprintf("line %d, column %d: the reference has %q for columns %d-%d and md2html %q for columns %d-%d",
					i+1, min(w[j].Start, g[j].Start)+1, w[j].Scopes, w[j].Start+1, w[j].End, g[j].Scopes, g[j].Start+1, g[j].End)
			}
		}
	}
	return ""
}
