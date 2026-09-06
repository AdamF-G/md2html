package md2html

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// writeTree creates files from a path→content map under a temp dir.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	// Resolve symlinks so comparisons match the crawler's own resolution
	// (macOS /var is a symlink to /private/var).
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func srcNames(t *testing.T, root string, docs []Doc) []string {
	t.Helper()
	var out []string
	for _, d := range docs {
		rel, err := filepath.Rel(root, d.Src)
		if err != nil {
			t.Fatalf("Rel: %v", err)
		}
		out = append(out, rel)
	}
	sort.Strings(out)
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Link following is always unlimited, and crosses directories.
func TestCrawlFollowsLinksAcrossDirectoriesUnbounded(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md":        "[a](./a/one.md)",
		"docs/a/one.md":        "[b](../b/two.md)",
		"docs/b/two.md":        "[c](./deep/three.md)",
		"docs/b/deep/three.md": "end",
	})
	res, err := Crawl(CrawlOptions{Entries: []string{filepath.Join(root, "docs/index.md")}, Depth: -1})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	want := []string{"docs/a/one.md", "docs/b/deep/three.md", "docs/b/two.md", "docs/index.md"}
	if got := srcNames(t, root, res.Docs); !eq(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// --depth bounds directory seeding only, never link traversal.
func TestCrawlDepthBoundsSeedingNotLinks(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/top.md":            "no links",
		"docs/sub/mid.md":        "no links",
		"docs/sub/deeper/low.md": "no links",
	})
	res, err := Crawl(CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, Depth: 0})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if got, want := srcNames(t, root, res.Docs), []string{"docs/top.md"}; !eq(got, want) {
		t.Errorf("depth 0: got %v, want %v", got, want)
	}

	res, _ = Crawl(CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, Depth: 1})
	if got, want := srcNames(t, root, res.Docs), []string{"docs/sub/mid.md", "docs/top.md"}; !eq(got, want) {
		t.Errorf("depth 1: got %v, want %v", got, want)
	}

	res, _ = Crawl(CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, Depth: -1})
	if len(res.Docs) != 3 {
		t.Errorf("depth unlimited: got %d docs, want 3", len(res.Docs))
	}
}

// A depth-0 seed must still follow links into deeper directories.
func TestCrawlDepthZeroStillFollowsLinksDeeper(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/top.md":            "[deep](./sub/deeper/low.md)",
		"docs/sub/deeper/low.md": "end",
	})
	res, _ := Crawl(CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, Depth: 0})
	want := []string{"docs/sub/deeper/low.md", "docs/top.md"}
	if got := srcNames(t, root, res.Docs); !eq(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCrawlTerminatesOnCycle(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a.md": "[b](./b.md)",
		"b.md": "[a](./a.md)",
	})
	res, err := Crawl(CrawlOptions{Entries: []string{filepath.Join(root, "a.md")}, Depth: -1})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(res.Docs) != 2 {
		t.Errorf("got %d docs, want 2", len(res.Docs))
	}
}

func TestCrawlWarnsOnMissingLinkTarget(t *testing.T) {
	root := writeTree(t, map[string]string{"a.md": "[gone](./missing.md)"})
	res, _ := Crawl(CrawlOptions{Entries: []string{filepath.Join(root, "a.md")}, Depth: -1})
	if len(res.Warnings) == 0 {
		t.Fatal("expected a warning for the missing link target")
	}
	if len(res.Docs) != 1 {
		t.Errorf("got %d docs, want 1", len(res.Docs))
	}
}

// With -o, a link outside base is followed and mapped under _external.
func TestCrawlFollowsOutsideBaseWithOutDir(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md": "[out](../outside/x.md)",
		"outside/x.md":  "end",
	})
	res, err := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs/index.md")},
		OutDir:  filepath.Join(root, "site"),
		Depth:   -1,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(res.Docs) != 2 {
		t.Fatalf("got %d docs, want 2", len(res.Docs))
	}
	if len(res.External) != 1 {
		t.Errorf("got %d external docs, want 1", len(res.External))
	}
	var found bool
	for _, d := range res.Docs {
		if filepath.Base(d.Src) == "x.md" {
			found = true
			if !strings.Contains(d.Out, externalDir) {
				t.Errorf("external doc not mapped under %s: %s", externalDir, d.Out)
			}
		}
	}
	if !found {
		t.Error("outside document not in emit set")
	}
}

// Without -o, a link outside base is refused and warned about.
func TestCrawlRefusesOutsideBaseInPlace(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md": "[out](../outside/x.md)",
		"outside/x.md":  "end",
	})
	res, err := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs/index.md")},
		Depth:   -1,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(res.Docs) != 1 {
		t.Errorf("got %d docs, want 1 (must not escape base in place)", len(res.Docs))
	}
	if len(res.Warnings) == 0 {
		t.Error("expected a warning about refusing to escape base")
	}
}

// mustCrawl fails the test on error instead of leaving a nil result to be
// dereferenced into a panic.
func mustCrawl(t *testing.T, opt CrawlOptions) *CrawlResult {
	t.Helper()
	res, err := Crawl(opt)
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	return res
}

// A directory symlink pointing at an ancestor must not loop forever.
func TestCrawlTerminatesOnSymlinkLoop(t *testing.T) {
	root := writeTree(t, map[string]string{"docs/a.md": "# A"})
	if err := os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "docs", "loop")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	done := make(chan *CrawlResult, 1)
	go func() {
		res, err := Crawl(CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, Depth: -1})
		if err == nil {
			done <- res
		} else {
			done <- nil
		}
	}()
	select {
	case res := <-done:
		if res == nil {
			t.Fatal("Crawl errored on a symlink loop")
		}
		if len(res.Docs) != 1 {
			t.Errorf("got %d docs, want 1 (symlink alias emitted twice?)", len(res.Docs))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Crawl did not terminate on a symlink loop")
	}
}

// A symlinked alias of a document must not be emitted as a second document.
func TestCrawlDeduplicatesSymlinkedAlias(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/real.md":  "# Real",
		"docs/index.md": "[r](./real.md)",
	})
	if err := os.Symlink(filepath.Join(root, "docs", "real.md"), filepath.Join(root, "docs", "alias.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	res := mustCrawl(t, CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, Depth: -1})
	if len(res.Docs) != 2 {
		t.Errorf("got %d docs, want 2 (alias.md should resolve to real.md)", len(res.Docs))
	}
}

// The spec requires a warning when a referenced asset does not exist.
func TestCrawlWarnsOnMissingAsset(t *testing.T) {
	root := writeTree(t, map[string]string{"docs/a.md": "![f](./img/nope.png)"})
	res := mustCrawl(t, CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, Depth: -1})
	var found bool
	for _, w := range res.Warnings {
		if strings.Contains(w.Message, "asset") {
			found = true
		}
	}
	if !found {
		t.Errorf("no missing-asset warning; got %+v", res.Warnings)
	}
}

// An unreadable document must be warned about and NOT enter the emit set.
func TestCrawlSkipsUnreadableSource(t *testing.T) {
	root := writeTree(t, map[string]string{"docs/ok.md": "# OK", "docs/bad.md": "# Bad"})
	bad := filepath.Join(root, "docs", "bad.md")
	if err := os.Chmod(bad, 0o000); err != nil {
		t.Skipf("chmod unavailable: %v", err)
	}
	t.Cleanup(func() { os.Chmod(bad, 0o644) })
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits are not enforced")
	}
	res := mustCrawl(t, CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, Depth: -1})
	for _, d := range res.Docs {
		if filepath.Base(d.Src) == "bad.md" {
			t.Error("unreadable document entered the emit set")
		}
	}
	if len(res.Warnings) == 0 {
		t.Error("expected a warning for the unreadable document")
	}
}

func TestCrawlErrorsOnMissingEntryPoint(t *testing.T) {
	root := t.TempDir()
	_, err := Crawl(CrawlOptions{Entries: []string{filepath.Join(root, "nope.md")}, Depth: -1})
	if err == nil {
		t.Error("expected an error for a missing entry point")
	}
}
