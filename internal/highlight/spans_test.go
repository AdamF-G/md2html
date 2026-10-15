package highlight

import (
	"reflect"
	"testing"
)

func TestSpansMergeAndSkipPlain(t *testing.T) {
	str := []string{"source.x", "string.quoted.x"}
	q := []string{"source.x", "string.quoted.x", "punctuation.definition.string.begin.x"}
	kw := []string{"source.x", "keyword.control.x"}
	plain := []string{"source.x"}
	toks := []Token{
		{0, 6, kw}, {6, 7, plain}, {7, 8, q}, {8, 11, str}, {11, 12, q}, {12, 13, plain}, {13, 19, kw},
	}
	got := Spans(toks)
	want := []Span{{0, 6, Keyword}, {7, 12, String}, {13, 19, Keyword}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Spans = %v, want %v", got, want)
	}
}

func TestSpansDoNotMergeAcrossAGap(t *testing.T) {
	kw := []string{"source.x", "keyword.x"}
	got := Spans([]Token{{0, 2, kw}, {3, 5, kw}})
	if len(got) != 2 {
		t.Errorf("Spans = %v, want two spans", got)
	}
}
