package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AdamF-G/md2html"
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

func TestStringListSplitsAndAccumulates(t *testing.T) {
	var l stringList
	if err := l.Set("a,b"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := l.Set(" c "); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := []string(l); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("got %v, want [a b c]", got)
	}
}

// Empty values would resolve to base itself, excluding the whole tree.
func TestStringListDropsEmptyValues(t *testing.T) {
	var l stringList
	if err := l.Set("a,,b,"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := []string(l); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("got %v, want [a b]", got)
	}
}

// End to end: an excluded subtree gets no .html written into it, and the
// run still succeeds.
func TestRunExcludeWritesNothingIntoExcludedTree(t *testing.T) {
	root := tree(t, map[string]string{
		"index.md":       "[d](./slides/deck.md)\n",
		"slides/deck.md": "# Deck\n",
	})
	var out, errb bytes.Buffer
	code := run([]string{"--exclude", "slides", root}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "slides/deck.html")); !os.IsNotExist(err) {
		t.Errorf("wrote into excluded subtree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
		t.Errorf("did not write index.html: %v", err)
	}
	if !strings.Contains(errb.String(), "excluded") {
		t.Errorf("no exclusion warning on stderr: %s", errb.String())
	}
}

func TestRunLinkDepthBoundsFollowing(t *testing.T) {
	root := tree(t, map[string]string{
		"index.md": "[a](./a.md)\n",
		"a.md":     "[b](./b.md)\n",
		"b.md":     "# B\n",
	})
	var out, errb bytes.Buffer
	if code := run([]string{"--link-depth", "1", filepath.Join(root, "index.md")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "a.html")); err != nil {
		t.Errorf("one hop not followed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "b.html")); !os.IsNotExist(err) {
		t.Errorf("two hops followed despite --link-depth 1: %v", err)
	}
}

// The transform order is a correctness constraint pinned for the library at
// md2html_test.go's TestBuiltinsSignatureUnchanged. buildOptions rebuilds
// this same list from md2html.Builtins() precisely so it cannot drift from
// that pinned order; this test is what makes the constraint enforceable on
// the CLI side too — without it, a reordering here would ship green while
// the library stayed correct.
func TestBuildOptionsMatchesBuiltinsOrder(t *testing.T) {
	opts := buildOptions(md2html.Doc{Src: "doc.md"}, false, "", false, false, false, nil)
	want := md2html.Builtins()
	if len(opts.Transforms) != len(want) {
		t.Fatalf("buildOptions produced %d transforms, want %d", len(opts.Transforms), len(want))
	}
	for i, tr := range want {
		if opts.Transforms[i].Name != tr.Name {
			t.Errorf("Transforms[%d].Name = %q, want %q", i, opts.Transforms[i].Name, tr.Name)
		}
	}
}

// Each --no-* flag must drop exactly its own transform and nothing else,
// leaving every other name in its Builtins() position.
func TestBuildOptionsNoFlagsDropOnlyTheirOwnTransform(t *testing.T) {
	opts := buildOptions(md2html.Doc{Src: "doc.md"}, false, "", true, true, true, nil)
	want := []string{"containers", "chips", "sectionLinks", "toc"}
	var got []string
	for _, tr := range opts.Transforms {
		got = append(got, tr.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Transforms = %v, want %v", got, want)
	}
}

// End to end through the CLI: a chip, a heading, a "[[toc]]" marker and a
// "§" cross-reference must all resolve exactly as the library does on its
// own, so a future reordering in buildOptions shows up here too, not only
// in the unit-level name check above.
func TestRunResolvesChipsSectionLinksAndTOC(t *testing.T) {
	root := tree(t, map[string]string{
		"doc.md": "# Intro\n\n[[toc]]\n\n## 1 Setup [proven]\n\nSee §1 for details.\n",
	})
	var out, errb bytes.Buffer
	if code := run([]string{filepath.Join(root, "doc.md")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	b, err := os.ReadFile(filepath.Join(root, "doc.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, `<span class="chip chip-proven">proven</span>`) {
		t.Errorf("chip not rendered\ngot: %s", got)
	}
	if !strings.Contains(got, `id="1-setup"`) {
		t.Errorf("chip leaked into heading slug\ngot: %s", got)
	}
	if !strings.Contains(got, `<nav class="toc">`) {
		t.Errorf("toc marker not replaced\ngot: %s", got)
	}
	if !strings.Contains(got, `<a class="xref" href="#1-setup">§1</a>`) {
		t.Errorf("section reference not resolved\ngot: %s", got)
	}
}
