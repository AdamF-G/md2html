package md2html

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Top-level nodes come back from ParseFragment as a slice, not under a
// parent. A walk that only recurses into children silently skips every
// top-level element — the bug this test exists to prevent.
func TestParseFragmentReachesTopLevelNodes(t *testing.T) {
	root, err := parseFragment([]byte(`<table><tr><td>a</td></tr></table><p>x</p><table></table>`))
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	tables := 0
	walk(root, func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Table {
			tables++
		}
	})
	if tables != 2 {
		t.Errorf("walk found %d tables, want 2 (top-level nodes were skipped)", tables)
	}
}

func TestRoundTripPreservesRawHTML(t *testing.T) {
	in := `<div class="grid"><svg viewBox="0 0 10 10"><circle cx="5" cy="5" r="1"></circle></svg></div>`
	root, err := parseFragment([]byte(in))
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	out, err := renderTree(root)
	if err != nil {
		t.Fatalf("renderTree: %v", err)
	}
	for _, want := range []string{`class="grid"`, `viewBox="0 0 10 10"`, `<circle`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("round trip lost %q\ngot: %s", want, out)
		}
	}
}

// A transform registered in Options must actually run.
func TestConvertRunsTransforms(t *testing.T) {
	ran := false
	_, err := Convert([]byte("# hi"), Options{
		Transforms: []Transform{{Name: "probe", Fn: func(*html.Node) error { ran = true; return nil }}},
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !ran {
		t.Error("transform did not run")
	}
}
