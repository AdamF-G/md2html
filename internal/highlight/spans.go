package highlight

// Span is a coloured run of a line, in byte offsets. Text between spans is
// plain.
type Span struct {
	Start, End int
	Kind       Kind
}

// Spans sorts a line's tokens into kinds and merges touching tokens of one
// kind, so a string and its quotes are one span. Plain tokens give none.
func Spans(toks []Token) []Span {
	var out []Span
	for _, t := range toks {
		k := KindOf(t.Scopes)
		if k == Plain || t.Start == t.End {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Kind == k && out[n-1].End == t.Start {
			out[n-1].End = t.End
			continue
		}
		out = append(out, Span{t.Start, t.End, k})
	}
	return out
}
