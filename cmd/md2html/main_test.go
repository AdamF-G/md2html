package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for rel, c := range files {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(c), 0o644)
	}
	return root
}

func TestRunConvertsTreeToOutputDir(t *testing.T) {
	root := tree(t, map[string]string{
		"docs/index.md":    "# Home\n\n[a](./api/auth.md)",
		"docs/api/auth.md": "# Auth",
	})
	var out, errb bytes.Buffer
	code := run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site")}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	for _, want := range []string{"site/index.html", "site/api/auth.html"} {
		if _, err := os.Stat(filepath.Join(root, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
}

func TestRunSingleFileWritesBesideSource(t *testing.T) {
	root := tree(t, map[string]string{"README.md": "# Title\n\ntext"})
	var out, errb bytes.Buffer
	if code := run([]string{filepath.Join(root, "README.md")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	b, err := os.ReadFile(filepath.Join(root, "README.html"))
	if err != nil {
		t.Fatalf("README.html not written: %v", err)
	}
	if !strings.Contains(string(b), "<title>Title</title>") {
		t.Errorf("title missing: %s", b)
	}
	if out.Len() != 0 {
		t.Errorf("nothing should go to stdout, got: %s", out.String())
	}
}

// Flags must be accepted after the entry point, which is how every
// documented invocation is written.
func TestRunAcceptsFlagsAfterEntry(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# A"})
	var out, errb bytes.Buffer
	code := run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site")}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "site", "a.html")); err != nil {
		t.Errorf("flag after entry not parsed: %v", err)
	}
}

// The derived title must not pick up the "#" that HeadingAnchors appends.
func TestRunTitleExcludesAnchorText(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# Doc Title\n\ntext"})
	var out, errb bytes.Buffer
	run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site")}, &out, &errb)
	b, err := os.ReadFile(filepath.Join(root, "site", "a.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "<title>Doc Title</title>") {
		t.Errorf("title polluted by anchor text: %s", b)
	}
}

func TestRunRefusesToOverwriteHandWrittenHTML(t *testing.T) {
	root := tree(t, map[string]string{
		"docs/a.md":   "# A",
		"site/a.html": "<html>mine</html>",
	})
	var out, errb bytes.Buffer
	code := run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site")}, &out, &errb)
	if code == 0 {
		t.Error("expected non-zero exit when refusing a file")
	}
	b, _ := os.ReadFile(filepath.Join(root, "site/a.html"))
	if string(b) != "<html>mine</html>" {
		t.Errorf("hand-written file was modified: %s", b)
	}
	if !strings.Contains(errb.String(), "refus") {
		t.Errorf("no refusal reported: %s", errb.String())
	}
}

func TestRunIsIdempotent(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# A"})
	args := []string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site")}
	var o1, e1, o2, e2 bytes.Buffer
	if code := run(args, &o1, &e1); code != 0 {
		t.Fatalf("first run exit %d: %s", code, e1.String())
	}
	if code := run(args, &o2, &e2); code != 0 {
		t.Fatalf("second run exit %d (should overwrite own output silently): %s", code, e2.String())
	}
}

func TestRunFragmentMode(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# A"})
	var out, errb bytes.Buffer
	run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site"), "--fragment"}, &out, &errb)
	b, err := os.ReadFile(filepath.Join(root, "site/a.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(b)), "<body") {
		t.Errorf("fragment mode emitted a body tag: %s", b)
	}
}

func TestRunErrorsOnMissingEntryPoint(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"/definitely/not/here.md"}, &out, &errb); code == 0 {
		t.Error("expected non-zero exit for a missing entry point")
	}
}

func TestRunInPlaceWritesBesideSource(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# A"})
	var out, errb bytes.Buffer
	if code := run([]string{filepath.Join(root, "docs")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "docs/a.html")); err != nil {
		t.Errorf("in-place output missing: %v", err)
	}
}
