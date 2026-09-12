package md2html

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// upper replaces every prose text node with an uppercased copy, so the
// tests can see exactly which nodes were offered to fn.
func upper(s string) []*html.Node {
	u := strings.ToUpper(s)
	if u == s {
		return nil
	}
	return []*html.Node{{Type: html.TextNode, Data: u}}
}

func rewritten(t *testing.T, in string) string {
	t.Helper()
	root, err := parseFragment([]byte(in))
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	rewriteText(root, upper)
	out, err := renderTree(root)
	if err != nil {
		t.Fatalf("renderTree: %v", err)
	}
	return string(out)
}

func TestRewriteTextReachesNestedProse(t *testing.T) {
	got := rewritten(t, `<p>one <em>two</em> three</p>`)
	if got != `<p>ONE <em>TWO</em> THREE</p>` {
		t.Errorf("got %s", got)
	}
}

// A token inside a code span is being quoted, not used.
func TestRewriteTextSkipsCodeAndPre(t *testing.T) {
	got := rewritten(t, `<p>a <code>b</code></p><pre><code>c</code></pre>`)
	if !strings.Contains(got, "<code>b</code>") || !strings.Contains(got, "<code>c</code>") {
		t.Errorf("descended into code: %s", got)
	}
	if !strings.Contains(got, ">A ") {
		t.Errorf("skipped the prose too: %s", got)
	}
}

// Both callers produce links or badges, neither of which may nest inside
// an existing link.
func TestRewriteTextSkipsLinks(t *testing.T) {
	got := rewritten(t, `<p>a <a href="#x">b</a></p>`)
	if !strings.Contains(got, `<a href="#x">b</a>`) {
		t.Errorf("descended into a link: %s", got)
	}
}

func TestRewriteTextSkipsExistingChips(t *testing.T) {
	got := rewritten(t, `<p><span class="chip">b</span></p>`)
	if !strings.Contains(got, `<span class="chip">b</span>`) {
		t.Errorf("descended into a chip: %s", got)
	}
}

// Replacements must not be offered back to fn, or a rewrite that emits
// text containing its own trigger would loop.
func TestRewriteTextDoesNotRevisitReplacements(t *testing.T) {
	root, err := parseFragment([]byte(`<p>x</p>`))
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	rewriteText(root, func(s string) []*html.Node {
		calls++
		return []*html.Node{
			{Type: html.ElementNode, DataAtom: atom.Span, Data: "span"},
			{Type: html.TextNode, Data: "y"},
		}
	})
	if calls != 1 {
		t.Errorf("fn called %d times, want 1", calls)
	}
}

// Returning nil must leave the node untouched, identity included: the
// common case is a document with no tokens at all.
func TestRewriteTextNilLeavesTreeAlone(t *testing.T) {
	root, err := parseFragment([]byte(`<p>hello <em>there</em></p>`))
	if err != nil {
		t.Fatal(err)
	}
	rewriteText(root, func(string) []*html.Node { return nil })
	out, err := renderTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `<p>hello <em>there</em></p>` {
		t.Errorf("got %s", out)
	}
}

// --- splitMatches ---

// text is a small helper for building expected/rendered text nodes without
// repeating the html.Node literal everywhere.
func text(s string) *html.Node {
	return &html.Node{Type: html.TextNode, Data: s}
}

// texts extracts the Data of a []*html.Node that is expected to hold only
// text nodes, so test assertions can compare plain strings instead of node
// pointers.
func texts(t *testing.T, nodes []*html.Node) []string {
	t.Helper()
	out := make([]string, len(nodes))
	for i, n := range nodes {
		if n.Type != html.TextNode {
			t.Fatalf("node %d is not a text node: %+v", i, n)
		}
		out[i] = n.Data
	}
	return out
}

var wordRE = regexp.MustCompile(`\[(\w+)\]`)

func TestSplitMatchesNoMatches(t *testing.T) {
	got := splitMatches("plain prose", wordRE, func(loc []int) []*html.Node {
		t.Fatal("render should not be called")
		return nil
	})
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestSplitMatchesAllDeclined(t *testing.T) {
	got := splitMatches("a [one] b [two] c", wordRE, func(loc []int) []*html.Node {
		return nil
	})
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestSplitMatchesSomeDeclinedSomeRendered(t *testing.T) {
	got := splitMatches("a [one] b [two] c", wordRE, func(loc []int) []*html.Node {
		word := "a [one] b [two] c"[loc[2]:loc[3]]
		if word == "one" {
			return nil // declined
		}
		return []*html.Node{text("<" + word + ">")}
	})
	want := []string{"a [one] b ", "<two>", " c"}
	if got := texts(t, got); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitMatchesAtStart(t *testing.T) {
	got := splitMatches("[one] rest", wordRE, func(loc []int) []*html.Node {
		return []*html.Node{text("X")}
	})
	want := []string{"X", " rest"}
	if got := texts(t, got); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitMatchesAtEnd(t *testing.T) {
	got := splitMatches("rest [one]", wordRE, func(loc []int) []*html.Node {
		return []*html.Node{text("X")}
	})
	want := []string{"rest ", "X"}
	if got := texts(t, got); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitMatchesAdjacentNoGapText(t *testing.T) {
	got := splitMatches("[one][two]", wordRE, func(loc []int) []*html.Node {
		return []*html.Node{text("X")}
	})
	want := []string{"X", "X"}
	if got := texts(t, got); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A rendered node's type is passed through untouched: splitMatches must not
// assume every emitted node is text, since chips emit <span> elements.
func TestSplitMatchesPassesThroughElementNodes(t *testing.T) {
	got := splitMatches("a [one] b", wordRE, func(loc []int) []*html.Node {
		return []*html.Node{{Type: html.ElementNode, DataAtom: atom.Span, Data: "span"}}
	})
	if len(got) != 3 {
		t.Fatalf("got %d nodes, want 3", len(got))
	}
	if got[0].Type != html.TextNode || got[0].Data != "a " {
		t.Errorf("node 0 = %+v", got[0])
	}
	if got[1].Type != html.ElementNode || got[1].DataAtom != atom.Span {
		t.Errorf("node 1 = %+v", got[1])
	}
	if got[2].Type != html.TextNode || got[2].Data != " b" {
		t.Errorf("node 2 = %+v", got[2])
	}
}
