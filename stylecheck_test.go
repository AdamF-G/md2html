package md2html

import (
	"testing"

	"github.com/AdamF-G/md2html/internal/stylecheck"
)

// Every built-in style passes the style check with no findings at all,
// so a commit that breaks one fails here, and every finding on a user's
// style is about something that style changed.
func TestBuiltinStylesPassTheStyleCheck(t *testing.T) {
	cache := NewPluginCache()
	n := 0
	for _, p := range BuiltinPlugins() {
		for _, s := range p.StyleNames() {
			name := p.Name + "/" + s
			css, err := cache.Stylesheet([]string{name}, nil, nil)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			n++
			if fs := stylecheck.Check(css); len(fs) != 0 {
				t.Errorf("%s", stylecheck.Report(name, fs))
			}
		}
	}
	if n < 2 {
		t.Errorf("checked %d built-in styles, want default and claude-artifact at least", n)
	}
}
