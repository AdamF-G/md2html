package md2html

import (
	"path/filepath"
	"testing"
)

func TestCommonAncestor(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"/docs/a.md", "/docs/b.md"}, "/docs"},
		{[]string{"/docs/api/a.md", "/docs/guides/b.md"}, "/docs"},
		{[]string{"/docs/a.md"}, "/docs"},
		{[]string{"/a/x.md", "/b/y.md"}, "/"},
		{[]string{"/docs", "/docs/api"}, "/docs"},
	}
	for _, c := range cases {
		if got := commonAncestor(c.in); got != filepath.Clean(c.want) {
			t.Errorf("commonAncestor(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsUnder(t *testing.T) {
	cases := []struct {
		path, base string
		want       bool
	}{
		{"/docs/a.md", "/docs", true},
		{"/docs/api/a.md", "/docs", true},
		{"/docs", "/docs", true},
		{"/other/a.md", "/docs", false},
		// A sibling directory sharing a name prefix is NOT under base.
		{"/docs-other/a.md", "/docs", false},
		// Resolves before comparing, so ../ that stays inside is fine.
		{"/docs/api/../guide.md", "/docs", true},
	}
	for _, c := range cases {
		if got := isUnder(c.path, c.base); got != c.want {
			t.Errorf("isUnder(%q, %q) = %v, want %v", c.path, c.base, got, c.want)
		}
	}
}

func TestOutputPathMirrorsUnderBase(t *testing.T) {
	got := outputPath("/docs/api/auth.md", "/docs", "/site")
	if want := filepath.Clean("/site/api/auth.html"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOutputPathMapsExternalDocsIntoExternalDir(t *testing.T) {
	got := outputPath("/other-repo/docs/api.md", "/docs", "/site")
	if want := filepath.Clean("/site/_external/other-repo/docs/api.html"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// With no -o, output sits beside its source.
func TestOutputPathInPlace(t *testing.T) {
	got := outputPath("/docs/api/auth.md", "/docs", "")
	if want := filepath.Clean("/docs/api/auth.html"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOutputPathHandlesMarkdownExtension(t *testing.T) {
	got := outputPath("/docs/a.markdown", "/docs", "/site")
	if want := filepath.Clean("/site/a.html"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
