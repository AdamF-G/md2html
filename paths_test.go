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

// Regression: relative paths with .. must not escape outDir.
func TestOutputPathSanitizesRelativeWithDotDot(t *testing.T) {
	got := outputPath("../../etc/passwd.md", "/docs", "/site")
	outDir := "/site"
	if !isUnder(got, outDir) {
		t.Errorf("outputPath(\"../../etc/passwd.md\", \"/docs\", \"/site\") = %q, not under %q", got, outDir)
	}
}

// Regression: deeper relative paths must not escape outDir.
func TestOutputPathSanitizesDeepRelativePath(t *testing.T) {
	got := outputPath("../../../var/log/app.md", "/docs", "/site")
	outDir := "/site"
	if !isUnder(got, outDir) {
		t.Errorf("outputPath(\"../../../var/log/app.md\", \"/docs\", \"/site\") = %q, not under %q", got, outDir)
	}
}

// Regression: relative paths without .. are also sanitized and contained.
func TestOutputPathSanitizesRelativeWithoutDotDot(t *testing.T) {
	got := outputPath("docs/readme.md", "/docs", "/site")
	outDir := "/site"
	if !isUnder(got, outDir) {
		t.Errorf("outputPath(\"docs/readme.md\", \"/docs\", \"/site\") = %q, not under %q", got, outDir)
	}
}

func TestIsExcluded(t *testing.T) {
	ex := []string{"/tree/vendor", "/tree/archive/2019"}
	cases := []struct {
		path string
		want bool
	}{
		{"/tree/vendor", true},                 // the prefix itself
		{"/tree/vendor/lib/doc.md", true},      // beneath it
		{"/tree/vendored/doc.md", false},       // prefix of the string, not of the path
		{"/tree/archive/2020/doc.md", false},   // sibling of an excluded dir
		{"/tree/archive/2019/q1/doc.md", true}, // beneath a deeper prefix
		{"/tree/doc.md", false},                // unrelated
		{"/tree/vendor/../doc.md", false},      // climbs back out before matching
	}
	for _, c := range cases {
		if got := isExcluded(c.path, ex); got != c.want {
			t.Errorf("isExcluded(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// No exclusions must never exclude anything — the default path through
// every call site.
func TestIsExcludedEmptyExcludesNothing(t *testing.T) {
	if isExcluded("/tree/doc.md", nil) {
		t.Error("nil exclusions excluded a path")
	}
	if isExcluded("/tree/doc.md", []string{}) {
		t.Error("empty exclusions excluded a path")
	}
}
