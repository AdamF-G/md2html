package md2html

import "testing"

func TestParseAttrsClassesAndID(t *testing.T) {
	a, ok := parseAttrs(`#note .callout .compact`)
	if !ok {
		t.Fatal("did not parse")
	}
	if a.id != "note" {
		t.Errorf("id = %q, want note", a.id)
	}
	if len(a.classes) != 2 || a.classes[0] != "callout" || a.classes[1] != "compact" {
		t.Errorf("classes = %v, want [callout compact]", a.classes)
	}
}

func TestParseAttrsQuotedValueWithSpaces(t *testing.T) {
	a, ok := parseAttrs(`caption="cmd/md2html/main.go is here"`)
	if !ok {
		t.Fatal("did not parse")
	}
	if got := a.kv["caption"]; got != "cmd/md2html/main.go is here" {
		t.Errorf("caption = %q", got)
	}
}

// The documented hole in the old hand-rolled fence tokenizer: a quote
// inside a quoted value could not be expressed at all.
func TestParseAttrsEscapedQuoteInsideValue(t *testing.T) {
	a, ok := parseAttrs(`caption="has \"quote\" inside"`)
	if !ok {
		t.Fatal("did not parse")
	}
	if got := a.kv["caption"]; got != `has "quote" inside` {
		t.Errorf("caption = %q, want: has \"quote\" inside", got)
	}
}

func TestParseAttrsClassAttributeExpandsToClasses(t *testing.T) {
	a, ok := parseAttrs(`class="one two"`)
	if !ok {
		t.Fatal("did not parse")
	}
	if len(a.classes) != 2 || a.classes[0] != "one" || a.classes[1] != "two" {
		t.Errorf("classes = %v, want [one two]", a.classes)
	}
}

func TestParseAttrsBareKeyAndSingleQuotes(t *testing.T) {
	a, ok := parseAttrs(`wide caption='x y'`)
	if !ok {
		t.Fatal("did not parse")
	}
	if _, present := a.kv["wide"]; !present {
		t.Errorf("bare key absent: %v", a.kv)
	}
	if a.kv["caption"] != "x y" {
		t.Errorf("caption = %q", a.kv["caption"])
	}
}

func TestParseAttrsLastIDWins(t *testing.T) {
	a, _ := parseAttrs(`#a #b`)
	if a.id != "b" {
		t.Errorf("id = %q, want b", a.id)
	}
}

func TestParseAttrsEmptyIsNotAnAttributeBlock(t *testing.T) {
	if _, ok := parseAttrs(`   `); ok {
		t.Error("empty content parsed as attributes")
	}
}

// splitBraced finds a trailing {...} on a string and returns the head and
// the brace contents. It is what lets a fence info string or a bracketed
// span find its attribute block.
func TestSplitBracedTrailingBlock(t *testing.T) {
	head, in, ok := splitBraced(`.go caption="x"`)
	if ok {
		t.Fatalf("no braces present, but reported one: head=%q in=%q", head, in)
	}
	head, in, ok = splitBraced(`go {caption="x"}`)
	if !ok || head != "go" || in != `caption="x"` {
		t.Errorf("head=%q in=%q ok=%v", head, in, ok)
	}
}

func TestSplitBracedWholeStringIsBlock(t *testing.T) {
	head, in, ok := splitBraced(`{.go caption="x"}`)
	if !ok || head != "" || in != `.go caption="x"` {
		t.Errorf("head=%q in=%q ok=%v", head, in, ok)
	}
}

// braceBlock is splitBraced's scanner, and it reports where the block
// starts. That index is what tells a container fence line where the title in
// front of the block ends; deriving it from len(content) instead was two
// bytes out here, because a trailing no-break space is whitespace to
// strings.TrimSpace (so the block is still accepted) but not to any
// ASCII-only trim.
func TestBraceBlockReportsTheOpeningIndex(t *testing.T) {
	open, in, ok := braceBlock("card Why {#w} ")
	if !ok || in != "#w" {
		t.Fatalf("in=%q ok=%v, want #w / true", in, ok)
	}
	if open != 9 {
		t.Errorf("open = %d, want 9 (the index of the brace itself)", open)
	}
}

// A brace inside the block's own content belongs to the block, so the text
// in front of it is not shortened by the inner one.
func TestBraceBlockKeepsANestedBraceInsideTheBlock(t *testing.T) {
	open, in, ok := braceBlock("card {a{b}}")
	if !ok || open != 5 || in != "a{b}" {
		t.Errorf("open=%d in=%q ok=%v; want 5 / a{b} / true", open, in, ok)
	}
}

// An unclosed block is no block at all: the string stays exactly as written.
func TestBraceBlockRejectsAnUnclosedBrace(t *testing.T) {
	if open, in, ok := braceBlock("card {a"); ok {
		t.Errorf("open=%d in=%q ok=%v; want no block", open, in, ok)
	}
}

// A closing brace inside a quoted value must not end the block early.
func TestSplitBracedIgnoresBraceInsideQuotes(t *testing.T) {
	_, in, ok := splitBraced(`{caption="a } b"}`)
	if !ok || in != `caption="a } b"` {
		t.Errorf("in=%q ok=%v", in, ok)
	}
}
