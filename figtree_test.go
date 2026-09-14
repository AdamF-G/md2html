package md2html

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseFigTreeNesting(t *testing.T) {
	got, err := parseFigTree("a\n  b\n    c\n  d\ne\n")
	if err != nil {
		t.Fatalf("parseFigTree: %v", err)
	}
	want := []figTreeNode{
		{Label: "a", Kids: []figTreeNode{
			{Label: "b", Kids: []figTreeNode{{Label: "c"}}},
			{Label: "d"},
		}},
		{Label: "e"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseFigTree =\n%+v\nwant\n%+v", got, want)
	}
}

// Any consistent indent width works, because depth is a stack of widths
// rather than a fixed multiple.
func TestParseFigTreeIndentWidthIsFree(t *testing.T) {
	two, err := parseFigTree("a\n  b\n")
	if err != nil {
		t.Fatalf("two-space: %v", err)
	}
	four, err := parseFigTree("a\n    b\n")
	if err != nil {
		t.Fatalf("four-space: %v", err)
	}
	if !reflect.DeepEqual(two, four) {
		t.Errorf("indent width should not change the tree: %+v vs %+v", two, four)
	}
}

func TestParseFigTreeMarkersAndNotes(t *testing.T) {
	got, err := parseFigTree("* fig.go -- the fence branch\n- testdata/ -- not shipped\nplain\n")
	if err != nil {
		t.Fatalf("parseFigTree: %v", err)
	}
	want := []figTreeNode{
		{Label: "fig.go", Note: "the fence branch", Accent: true},
		{Label: "testdata/", Note: "not shipped", Muted: true},
		{Label: "plain"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseFigTree =\n%+v\nwant\n%+v", got, want)
	}
}

// The two collisions this grammar exists to survive. A glob is not a
// marker, and a flag is not a separator.
func TestParseFigTreeSpaceRuleAvoidsCollisions(t *testing.T) {
	cases := []struct {
		in   string
		want figTreeNode
	}{
		{"*_test.go", figTreeNode{Label: "*_test.go"}},
		{"-fno-strict", figTreeNode{Label: "-fno-strict"}},
		{
			"md2html --fragment -- writes a bare fragment",
			figTreeNode{Label: "md2html --fragment", Note: "writes a bare fragment"},
		},
		{"no separator here", figTreeNode{Label: "no separator here"}},
	}
	for _, c := range cases {
		got, err := parseFigTree(c.in)
		if err != nil {
			t.Fatalf("parseFigTree(%q): %v", c.in, err)
		}
		if len(got) != 1 || !reflect.DeepEqual(got[0], c.want) {
			t.Errorf("parseFigTree(%q) = %+v, want [%+v]", c.in, got, c.want)
		}
	}
}

// A backslash suppresses a sigil for free: it sits inside the literal
// string being matched, so the match simply fails. The backslash is left in
// place for the inline Markdown pass to remove.
func TestParseFigTreeEscapesSuppressSigils(t *testing.T) {
	cases := []struct {
		in   string
		want figTreeNode
	}{
		{`\* not accented`, figTreeNode{Label: `\* not accented`}},
		{`go run \-- args`, figTreeNode{Label: `go run \-- args`}},
	}
	for _, c := range cases {
		got, err := parseFigTree(c.in)
		if err != nil {
			t.Fatalf("parseFigTree(%q): %v", c.in, err)
		}
		if len(got) != 1 || !reflect.DeepEqual(got[0], c.want) {
			t.Errorf("parseFigTree(%q) = %+v, want [%+v]", c.in, got, c.want)
		}
	}
}

func TestParseFigTreeBlankLinesAndEmpty(t *testing.T) {
	got, err := parseFigTree("a\n\n  b\n\n")
	if err != nil {
		t.Fatalf("parseFigTree: %v", err)
	}
	want := []figTreeNode{{Label: "a", Kids: []figTreeNode{{Label: "b"}}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("blank lines should be skipped: %+v", got)
	}
	empty, err := parseFigTree("")
	if err != nil {
		t.Fatalf("an empty tree is not a fault: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("an empty tree should have no nodes, got %+v", empty)
	}
}

func TestParseFigTreeFaults(t *testing.T) {
	cases := []struct{ name, in, wantMsg string }{
		{
			name:    "dedent to a level that does not exist",
			in:      "a\n    b\n  c\n",
			wantMsg: "line 3: indented to no enclosing level",
		},
		{
			name:    "a tab in the indentation",
			in:      "a\n\tb\n",
			wantMsg: "line 2: a tab in a tree's indentation",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseFigTree(c.in)
			if err == nil {
				t.Fatalf("want a fault for %q", c.in)
			}
			if !strings.Contains(err.Error(), c.wantMsg) {
				t.Errorf("error %q should contain %q", err, c.wantMsg)
			}
		})
	}
}
