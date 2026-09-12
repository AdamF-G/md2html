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

// A seed that resolves outside the tree in in-place mode is refused with a
// warning, not an error: the run emits nothing and says why. This was the
// behavior before --exclude existed and must stay that way for callers who
// set no exclusions at all.
func TestCrawlOutOfTreeSeedInPlaceWarnsWithoutError(t *testing.T) {
	root := writeTree(t, map[string]string{
		"outside/real.md": "x",
	})
	docs := filepath.Join(root, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside/real.md"),
		filepath.Join(docs, "link.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	res, err := Crawl(CrawlOptions{Entries: []string{docs}, Depth: -1})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(res.Docs) != 0 {
		t.Errorf("emitted %d docs, want none", len(res.Docs))
	}
	if len(res.Warnings) == 0 {
		t.Error("no warning for the refused out-of-tree seed")
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

// Containment is a whole-system property, not a property of outputPath: a
// document reaches the emit set through two paths — directory seeding and
// link following — and both must apply the same rule. The tree here has a
// symlinked .md pointing outside the entry directory, a ../ link that
// resolves back inside base, and a deep subdirectory.
func TestCrawlContainsOutputInBothModes(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md":     "[deep](./a/b/deep.md) and [back](../docs/side.md)",
		"docs/side.md":      "# Side",
		"docs/a/b/deep.md":  "# Deep",
		"outside/secret.md": "# Secret",
	})
	docs := filepath.Join(root, "docs")
	if err := os.Symlink(filepath.Join(root, "outside", "secret.md"),
		filepath.Join(docs, "alias.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	secret := filepath.Join(root, "outside", "secret.md")

	// In place: nothing may be written outside the source tree, and the
	// out-of-tree symlink must be refused with a warning.
	res := mustCrawl(t, CrawlOptions{Entries: []string{docs}, Depth: -1})
	for _, d := range res.Docs {
		if !isUnder(d.Out, docs) {
			t.Errorf("in place: %s writes outside the source tree: %s", d.Src, d.Out)
		}
		if d.Src == secret {
			t.Errorf("in place: out-of-tree symlink target entered the emit set: %s", d.Out)
		}
	}
	if got, want := srcNames(t, root, res.Docs),
		[]string{"docs/a/b/deep.md", "docs/index.md", "docs/side.md"}; !eq(got, want) {
		t.Errorf("in place: got %v, want %v", got, want)
	}
	var warned bool
	for _, w := range res.Warnings {
		if strings.Contains(w.Message, "refusing to follow") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("in place: no refusal warning for the out-of-tree symlink; got %+v", res.Warnings)
	}
	if len(res.External) != 0 {
		t.Errorf("in place: nothing may be recorded as external, got %v", res.External)
	}

	// With -o: everything lands under the output directory, the out-of-base
	// target is contained beneath _external, and it is visible in the summary.
	site := filepath.Join(root, "site")
	res = mustCrawl(t, CrawlOptions{Entries: []string{docs}, OutDir: site, Depth: -1})
	var external *Doc
	for i := range res.Docs {
		d := &res.Docs[i]
		if !isUnder(d.Out, site) {
			t.Errorf("-o: %s writes outside the output dir: %s", d.Src, d.Out)
		}
		if d.Src == secret {
			external = d
		}
	}
	if external == nil {
		t.Fatalf("-o: out-of-base symlink target not followed; got %v", srcNames(t, root, res.Docs))
	}
	if !isUnder(external.Out, filepath.Join(site, externalDir)) {
		t.Errorf("-o: out-of-base doc not emitted under %s: %s", externalDir, external.Out)
	}
	var listed bool
	for _, e := range res.External {
		if e == secret {
			listed = true
		}
	}
	if !listed {
		t.Errorf("-o: out-of-base seed missing from res.External %v", res.External)
	}
}

// a.md and a.markdown in one directory both map to a.html. The emit pass is
// parallel, so the run must be refused rather than raced.
func TestCrawlRefusesDuplicateOutputPaths(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/a.md":       "# A",
		"docs/a.markdown": "# Also A",
	})
	_, err := Crawl(CrawlOptions{Entries: []string{filepath.Join(root, "docs")}, Depth: -1})
	if err == nil {
		t.Fatal("expected an error for two sources sharing one output path")
	}
	for _, want := range []string{"a.md", "a.markdown", "a.html"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

// A link into an excluded subtree must not pull the target in, and must
// leave the href exactly as written so the other tool's output still
// resolves.
func TestCrawlExcludeRefusesLinkTarget(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md":       "[slides](./slides/deck.md)\n[ok](./ok.md)",
		"docs/slides/deck.md": "owned by another tool",
		"docs/ok.md":          "fine",
	})
	res, err := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs/index.md")},
		Depth:   -1,
		Exclude: []string{"slides"},
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	want := []string{"docs/index.md", "docs/ok.md"}
	if got := srcNames(t, root, res.Docs); !eq(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	var idx *Doc
	for i := range res.Docs {
		if strings.HasSuffix(res.Docs[i].Src, "index.md") {
			idx = &res.Docs[i]
		}
	}
	if idx == nil {
		t.Fatal("index.md not emitted")
	}
	if repl, mapped := idx.LinkMap["./slides/deck.md"]; mapped {
		t.Errorf("excluded link was rewritten to %q, want left as written", repl)
	}
	if idx.LinkMap["./ok.md"] != "ok.html" {
		t.Errorf("non-excluded link map = %q, want %q", idx.LinkMap["./ok.md"], "ok.html")
	}
}

// The refusal is reported, not silent: a link that stops resolving to a
// generated page is something the author needs to know about.
func TestCrawlExcludeWarnsOnLinkTarget(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md":       "[slides](./slides/deck.md)",
		"docs/slides/deck.md": "x",
	})
	res, err := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs/index.md")},
		Depth:   -1,
		Exclude: []string{"slides"},
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	var found bool
	for _, w := range res.Warnings {
		if strings.Contains(w.Message, "./slides/deck.md") && strings.Contains(w.Message, "excluded") {
			found = true
		}
	}
	if !found {
		t.Errorf("no exclusion warning, got %v", res.Warnings)
	}
}

// Seeding an excluded subtree is silent: the caller asked for the
// exclusion, and naming every file inside it would bury the warnings that
// matter under one line per excluded document.
func TestCrawlExcludeSkipsSeedsSilently(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md":       "no links",
		"docs/slides/deck.md": "x",
		"docs/slides/more.md": "y",
	})
	res, err := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs")},
		Depth:   -1,
		Exclude: []string{"slides"},
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if got := srcNames(t, root, res.Docs); !eq(got, []string{"docs/index.md"}) {
		t.Errorf("got %v, want [docs/index.md]", got)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("excluded seeds warned: %v", res.Warnings)
	}
}

// An absolute --exclude value is honored as given, rather than being
// joined onto base a second time.
func TestCrawlExcludeAcceptsAbsolutePath(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md":       "x",
		"docs/slides/deck.md": "y",
	})
	res, err := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs")},
		Depth:   -1,
		Exclude: []string{filepath.Join(root, "docs/slides")},
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if got := srcNames(t, root, res.Docs); !eq(got, []string{"docs/index.md"}) {
		t.Errorf("got %v, want [docs/index.md]", got)
	}
}

// An --exclude value that matches nothing on disk excludes nothing, which
// is silently indistinguishable from dropping it — easy to hit, since a
// relative value is resolved against base rather than against whichever
// entry point the caller had in mind. That must be warned about, while a
// value that does resolve to something real stays quiet.
func TestCrawlExcludeWarnsWhenValueMatchesNothing(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md":       "x",
		"docs/slides/deck.md": "y",
	})
	res, err := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs")},
		Depth:   -1,
		Exclude: []string{"slides", "nope-does-not-exist"},
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	var sawMissing, sawReal bool
	for _, w := range res.Warnings {
		if strings.Contains(w.Message, "nope-does-not-exist") {
			sawMissing = true
		}
		if strings.Contains(w.Message, "slides") {
			sawReal = true
		}
	}
	if !sawMissing {
		t.Errorf("no warning naming the nonexistent exclude value, got %v", res.Warnings)
	}
	if sawReal {
		t.Errorf("existing exclude value was warned about unnecessarily, got %v", res.Warnings)
	}
}

// Excluding everything is a mistake worth failing on, not an empty build
// that silently succeeds.
func TestCrawlExcludeEverythingIsAnError(t *testing.T) {
	root := writeTree(t, map[string]string{"docs/slides/deck.md": "x"})
	_, err := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs")},
		Depth:   -1,
		Exclude: []string{"slides"},
	})
	if err == nil {
		t.Fatal("want error when every seed is excluded")
	}
	if !strings.Contains(err.Error(), "excluded") {
		t.Errorf("error %q does not mention exclusion", err)
	}
}

// Exclusion must prune the directory walk, not just filter its results:
// an unreadable directory inside an excluded subtree must never be
// entered, so it cannot contribute anything at all.
func TestSeedSkipsExcludedDirectories(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md":         "x",
		"docs/vendor/a.md":      "y",
		"docs/vendor/deep/b.md": "z",
		"docs/keep/c.md":        "w",
	})
	got, _, err := seed(filepath.Join(root, "docs"), -1,
		[]string{filepath.Join(root, "docs/vendor")})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	var rel []string
	for _, p := range got {
		r, relErr := filepath.Rel(root, p)
		if relErr != nil {
			t.Fatal(relErr)
		}
		rel = append(rel, r)
	}
	sort.Strings(rel)
	want := []string{"docs/index.md", "docs/keep/c.md"}
	if !eq(rel, want) {
		t.Errorf("got %v, want %v", rel, want)
	}
}

// An excluded entry point that is itself a single file contributes
// nothing, rather than being seeded because it is not a directory.
func TestSeedSkipsExcludedFileEntry(t *testing.T) {
	root := writeTree(t, map[string]string{"docs/vendor/a.md": "y"})
	got, _, err := seed(filepath.Join(root, "docs/vendor/a.md"), -1,
		[]string{filepath.Join(root, "docs/vendor")})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

// A tree whose only Markdown lives in an excluded subtree must say so.
// Without the skipped count this reports "no Markdown files found in entry
// points", which points the reader at the wrong cause entirely.
func TestCrawlAllExcludedReportsExclusion(t *testing.T) {
	root := writeTree(t, map[string]string{"docs/vendor/a.md": "x"})
	_, err := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs")},
		Depth:   -1,
		Exclude: []string{"vendor"},
	})
	if err == nil {
		t.Fatal("want an error when every candidate path is excluded")
	}
	if !strings.Contains(err.Error(), "excluded") {
		t.Errorf("error %q does not mention exclusion", err)
	}
}

// An empty tree is not an exclusion problem, and must not be reported as
// one just because an unrelated --exclude was set. Here "vendor" matches
// nothing on disk, so the walk never reaches the SkipDir branch and
// skipped stays 0 — this covers the no-pruning path only. See
// TestCrawlEmptyTreeWithPrunedDirectoryMentionsExclusion for the case
// where something was actually pruned.
func TestCrawlEmptyTreeWithNoPruningReportsNoFiles(t *testing.T) {
	root := writeTree(t, map[string]string{"docs/notes.txt": "x"})
	_, err := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs")},
		Depth:   -1,
		Exclude: []string{"vendor"},
	})
	if err == nil {
		t.Fatal("want an error when the tree has no Markdown")
	}
	if strings.Contains(err.Error(), "excluded") {
		t.Errorf("error %q blames exclusion for an empty tree", err)
	}
}

// A pruned directory bumps the skipped count whether or not it held any
// Markdown, so an empty tree with a real excluded directory is reported as
// possibly-exclusion rather than definitely-empty. SkipDir is what makes
// pruning cheap, so the count can never know what was inside.
func TestCrawlEmptyTreeWithPrunedDirectoryMentionsExclusion(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/notes.txt":         "x",
		"docs/vendor/readme.txt": "y",
	})
	_, err := Crawl(CrawlOptions{
		Entries: []string{filepath.Join(root, "docs")},
		Depth:   -1,
		Exclude: []string{"vendor"},
	})
	if err == nil {
		t.Fatal("want an error when no Markdown is found")
	}
	if !strings.Contains(err.Error(), "excluded") {
		t.Errorf("error %q does not mention exclusion", err)
	}
}

// One hop pulls in what a seed links to, and stops there.
func TestCrawlLinkDepthOneStopsAfterOneHop(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md": "[a](./a.md)",
		"docs/a.md":     "[b](./b.md)",
		"docs/b.md":     "[c](./c.md)",
		"docs/c.md":     "end",
	})
	res, err := Crawl(CrawlOptions{
		Entries:   []string{filepath.Join(root, "docs/index.md")},
		Depth:     -1,
		LinkDepth: 1,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	want := []string{"docs/a.md", "docs/index.md"}
	if got := srcNames(t, root, res.Docs); !eq(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Depth is measured per seed, not from whichever document happened to be
// dequeued first. Two seeds, each heading its own two-hop chain: depth 1
// must admit hop 1 from both and hop 2 from neither.
func TestCrawlLinkDepthMeasuredFromEachSeed(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/one.md": "[a](./a.md)",
		"docs/a.md":   "[aa](./aa.md)",
		"docs/aa.md":  "end",
		"docs/two.md": "[b](./b.md)",
		"docs/b.md":   "[bb](./bb.md)",
		"docs/bb.md":  "end",
	})
	res, err := Crawl(CrawlOptions{
		Entries: []string{
			filepath.Join(root, "docs/one.md"),
			filepath.Join(root, "docs/two.md"),
		},
		Depth:     -1,
		LinkDepth: 1,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	want := []string{"docs/a.md", "docs/b.md", "docs/one.md", "docs/two.md"}
	if got := srcNames(t, root, res.Docs); !eq(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The zero value must not change behavior: this is the same tree and the
// same expectation as TestCrawlFollowsLinksAcrossDirectoriesUnbounded,
// asserted through an explicit LinkDepth: 0.
func TestCrawlLinkDepthZeroIsUnlimited(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md":        "[a](./a/one.md)",
		"docs/a/one.md":        "[b](../b/two.md)",
		"docs/b/two.md":        "[c](./deep/three.md)",
		"docs/b/deep/three.md": "end",
	})
	res, err := Crawl(CrawlOptions{
		Entries:   []string{filepath.Join(root, "docs/index.md")},
		Depth:     -1,
		LinkDepth: 0,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	want := []string{"docs/a/one.md", "docs/b/deep/three.md", "docs/b/two.md", "docs/index.md"}
	if got := srcNames(t, root, res.Docs); !eq(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A negative value follows nothing at all — seeds only.
func TestCrawlLinkDepthNegativeFollowsNothing(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md": "[a](./a.md)",
		"docs/a.md":     "end",
	})
	res, err := Crawl(CrawlOptions{
		Entries:   []string{filepath.Join(root, "docs/index.md")},
		Depth:     -1,
		LinkDepth: -1,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if got := srcNames(t, root, res.Docs); !eq(got, []string{"docs/index.md"}) {
		t.Errorf("got %v, want [docs/index.md]", got)
	}
}

// Exclusion and the hop limit compose: a link inside the budget is still
// refused for being excluded, and one beyond the budget is not followed at
// all. The hop gate short-circuits before admit, so the out-of-budget link
// produces no exclusion warning — nothing was followed either way.
func TestCrawlExcludeAndLinkDepthCompose(t *testing.T) {
	// index(0) -> a(1) -> {vendor/x(2), b(2)}, and b(2) -> vendor/y(3).
	// With LinkDepth 2, a's links are still inside the budget and reach
	// admit; b's are not, and the gate returns before admit ever sees them.
	// Both vendor targets exist on disk, because the missing-target check
	// runs ahead of the gate and would otherwise raise a different warning.
	root := writeTree(t, map[string]string{
		"docs/index.md":    "[a](./a.md)",
		"docs/a.md":        "[v](./vendor/x.md)\n[b](./b.md)",
		"docs/b.md":        "[deep](./vendor/y.md)",
		"docs/vendor/x.md": "owned elsewhere",
		"docs/vendor/y.md": "also owned elsewhere",
	})
	res, err := Crawl(CrawlOptions{
		Entries:   []string{filepath.Join(root, "docs/index.md")},
		Depth:     -1,
		LinkDepth: 2,
		Exclude:   []string{"vendor"},
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	want := []string{"docs/a.md", "docs/b.md", "docs/index.md"}
	if got := srcNames(t, root, res.Docs); !eq(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	var inBudget, outOfBudget bool
	for _, w := range res.Warnings {
		if !strings.Contains(w.Message, "excluded") {
			continue
		}
		if strings.Contains(w.Message, "./vendor/x.md") {
			inBudget = true
		}
		if strings.Contains(w.Message, "./vendor/y.md") {
			outOfBudget = true
		}
	}
	if !inBudget {
		t.Errorf("no exclusion warning for the in-budget link, got %v", res.Warnings)
	}
	if outOfBudget {
		t.Errorf("exclusion warning for a link past the hop budget, got %v", res.Warnings)
	}
}

// Reviewer-reported bug: Convert strips front matter before parsing
// (md2html.go), but the crawler parsed the raw, unstripped bytes, so a
// Markdown link sitting inside a "title:" value was followed and its
// target pulled into the emit set even though it never appears in any
// rendered output.
func TestCrawlIgnoresLinkInsideFrontMatter(t *testing.T) {
	root := writeTree(t, map[string]string{
		"index.md": "---\ntitle: See [other](./other.md)\n---\n\n# Index\n",
		"other.md": "# Other\n",
	})
	res := mustCrawl(t, CrawlOptions{Entries: []string{filepath.Join(root, "index.md")}, Depth: -1})
	if got := srcNames(t, root, res.Docs); !eq(got, []string{"index.md"}) {
		t.Errorf("front-matter link followed: got %v, want [index.md]", got)
	}
}

// Same bug, the image/asset-warning half: an image referenced only inside a
// front-matter value must not be checked for existence or reported missing,
// since it appears in no rendered document.
func TestCrawlIgnoresImageInsideFrontMatter(t *testing.T) {
	root := writeTree(t, map[string]string{
		"index.md": "---\ntitle: See ![i](./missing.png)\n---\n\n# Index\n",
	})
	res := mustCrawl(t, CrawlOptions{Entries: []string{filepath.Join(root, "index.md")}, Depth: -1})
	for _, w := range res.Warnings {
		if strings.Contains(w.Message, "missing.png") {
			t.Errorf("warned about an asset referenced only in front matter: %v", w)
		}
	}
}
