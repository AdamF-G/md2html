# Traversal Controls Implementation Plan

**Goal:** Let a caller declare directory subtrees the crawler must never enter, seed, follow into, or write to, and optionally bound how far link-following may travel from a seed.

**Architecture:** Both controls land on `CrawlOptions` and are enforced inside `Crawl`. Exclusion gets one predicate in `paths.go` (`isExcluded`) sitting alongside the existing `isUnder`, checked in `admit` — the single gate every path already passes through on its way onto the BFS queue — and again in `seed`'s directory walk so an excluded subtree is never even traversed. Link depth turns the BFS queue from `[]string` into a queue of `(path, hops)` pairs, with `admit` taking the hop count of the document a target was reached from.

**Tech Stack:** Go 1.27.1, stdlib only (`path/filepath`, `flag`). No new dependencies.

**Spec:** `docs/specs/2026-09-11-extended-content-model.md` — items 3 (exclusion directories and traversal modes) and 9 (supplemental per-invocation entry set).

## Global Constraints

- No new module dependencies. Everything here is stdlib plus what `go.mod` already lists.
- `gofmt` clean and `go vet ./...` clean — existing repo convention.
- `go test ./...` must stay green throughout. One existing test (`TestCrawlFollowsLinksAcrossDirectoriesUnbounded`) asserts today's unbounded default; it must keep passing unchanged, because unbounded stays the default.
- **The zero value of `CrawlOptions` must not change behavior.** A library caller writing `CrawlOptions{Entries: ...}` today gets unlimited link-following; it must still get unlimited link-following after this plan. This is why `LinkDepth` uses `0` for unlimited rather than mirroring `Depth`'s `-1` convention — see Task 5.
- Exclusion is enforced at `admit`, which is the authoritative gate. Checks added anywhere else (the directory walk in Task 3) are optimizations and must never be the only thing preventing an excluded path from being emitted.
- Excluded-directory checks apply **before** the existing containment (`isUnder`) check, not instead of it. Both are real constraints and either alone must be able to stop a path.
- Comments explain *why*, not *what* — match the density and voice of the surrounding files.

---

### Task 1: The exclusion predicate

A pure path predicate with no crawler wiring, so it can be tested exhaustively before anything depends on it. It lives in `paths.go` next to `isUnder`, whose semantics it reuses: "at or beneath" is exactly `isUnder`, applied against each excluded prefix in turn.

**Files:**
- Modify: `paths.go` (append after `isUnder`)
- Test: `paths_test.go` (append)

**Interfaces:**
- Consumes: `isUnder(path, base string) bool` (existing, `paths.go`).
- Produces: `func isExcluded(path string, excluded []string) bool` — reports whether `path` is at or beneath any prefix in `excluded`. All paths must be absolute and are cleaned by `isUnder`. An empty or nil `excluded` always returns false.

- [ ] **Step 1: Write the failing test**

Append to `paths_test.go`:

```go
func TestIsExcluded(t *testing.T) {
	ex := []string{"/tree/vendor", "/tree/archive/2019"}
	cases := []struct {
		path string
		want bool
	}{
		{"/tree/vendor", true},                  // the prefix itself
		{"/tree/vendor/lib/doc.md", true},       // beneath it
		{"/tree/vendored/doc.md", false},        // prefix of the string, not of the path
		{"/tree/archive/2020/doc.md", false},    // sibling of an excluded dir
		{"/tree/archive/2019/q1/doc.md", true},  // beneath a deeper prefix
		{"/tree/doc.md", false},                 // unrelated
		{"/tree/vendor/../doc.md", false},       // climbs back out before matching
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestIsExcluded -v`
Expected: FAIL — `undefined: isExcluded`.

- [ ] **Step 3: Write minimal implementation**

Append to `paths.go`:

```go
// isExcluded reports whether path is at or beneath any of the excluded
// directory prefixes. Both path and every prefix must be absolute.
//
// This reuses isUnder rather than comparing strings, so "/tree/vendored"
// is not treated as living under "/tree/vendor" — a string-prefix check
// would exclude a sibling directory whose name merely starts the same way.
func isExcluded(path string, excluded []string) bool {
	for _, e := range excluded {
		if isUnder(path, e) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestIsExcluded -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add paths.go paths_test.go
git commit -m "feat: add isExcluded path predicate"
```

---

### Task 2: Resolve `--exclude` values and refuse excluded link targets

Adds the `CrawlOptions.Exclude` field, turns each value into an absolute prefix, and enforces it in `admit`. After this task exclusion is *correct* — an excluded document never enters the emit set — even though the directory walk still visits it (Task 3 fixes that).

A link pointing into an excluded subtree needs no special handling to keep its href intact: `buildLinkMaps` only maps document links whose target is in `outBySrc`, so a target that was never emitted is already left exactly as written.

**Files:**
- Modify: `crawl.go` (`CrawlOptions`, `Crawl`)
- Test: `crawl_test.go` (append)

**Interfaces:**
- Consumes: `isExcluded` (Task 1); `resolve(p string) (string, error)`, `commonAncestor([]string) string`, `Warning{Src, Message string}` (all existing, `crawl.go` / `paths.go`).
- Produces:
  - `CrawlOptions.Exclude []string` — directory prefixes, each relative to the resolved base or absolute.
  - `func resolveExcludes(vals []string, base string) []string` — absolute, symlink-resolved, cleaned prefixes.
  - `admit` gains a `viaLink bool` parameter: `admit(src, target, ref string, viaLink bool)`.

- [ ] **Step 1: Write the failing test**

Append to `crawl_test.go`:

```go
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
	if got := srcNames(t, root, res.Docs); !eq(got, []string{"index.md"}) {
		t.Errorf("got %v, want [index.md]", got)
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
	if got := srcNames(t, root, res.Docs); !eq(got, []string{"index.md"}) {
		t.Errorf("got %v, want [index.md]", got)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestCrawlExclude -v`
Expected: FAIL — `unknown field Exclude in struct literal of type CrawlOptions`.

- [ ] **Step 3: Write the implementation**

In `crawl.go`, add the field to `CrawlOptions`, after `Depth`:

```go
	// Exclude lists directory prefixes that must never be entered. Each
	// value is either absolute or relative to the resolved base. A path at
	// or beneath one is never seeded, never followed as a link target, and
	// never written to; a link pointing at one keeps its href exactly as
	// written, the same handling a link escaping base gets in in-place mode.
	//
	// This exists for subtrees some other tool already owns — a slide-deck
	// renderer, a vendored dependency's own generated docs, a frozen
	// archive. Without it the only way to keep the crawler out of one is to
	// move it out of the source tree, which is rarely possible.
	Exclude []string
```

Add `resolveExcludes` beside `seed`:

```go
// resolveExcludes turns each Exclude value into an absolute, cleaned,
// symlink-resolved directory prefix.
//
// A relative value is taken against base, which is why this runs after
// commonAncestor rather than at the top of Crawl. Symlinks are resolved so
// a subtree reached through a link is still recognized as the excluded one,
// matching how every other path in the crawler is keyed. A value naming
// nothing on disk is kept as a literal cleaned path rather than dropped:
// excluding a directory that does not exist yet is harmless, whereas
// silently ignoring a misspelled value would hand back a build that quietly
// entered the subtree the caller was trying to protect.
func resolveExcludes(vals []string, base string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		p := v
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		r, err := resolve(p)
		if err != nil {
			r = filepath.Clean(p)
		}
		out = append(out, r)
	}
	return out
}
```

In `Crawl`, immediately after `base := commonAncestor(entryAbs)`:

```go
	excluded := resolveExcludes(opt.Exclude, base)
```

Replace the `admit` closure with the version below. The new `viaLink`
parameter is what decides whether a refusal is reported: a refused *seed*
is the caller getting exactly what they asked for, while a refused *link*
changes how an existing document renders and has to be surfaced.

```go
	admit := func(src, target, ref string, viaLink bool) {
		// Exclusion is checked before containment: both are real
		// constraints, and either one alone has to be able to stop a path.
		if isExcluded(target, excluded) {
			if viaLink {
				res.Warnings = append(res.Warnings, Warning{src,
					fmt.Sprintf("not following %s: excluded directory", ref)})
			}
			return
		}
		if !isUnder(target, base) {
			if opt.OutDir == "" {
				res.Warnings = append(res.Warnings, Warning{src,
					fmt.Sprintf("refusing to follow %s outside %s (no -o given)", ref, base)})
				return
			}
			res.External = append(res.External, target)
		}
		if !visited[target] {
			visited[target] = true
			queue = append(queue, target)
		}
	}
```

Update the two call sites. In the seeding loop:

```go
			admit(entryAbs[i], p, p, false)
```

In the link-following loop:

```go
			admit(cur, target, l.Href, true)
```

Finally, add the all-excluded guard. The existing `found == 0` check fires
before exclusion is applied, so it cannot catch this case. Insert directly
after the seeding loop's `if found == 0 { ... }` block:

```go
	// found counts what the entry points contain, before exclusion. A run
	// whose every seed was excluded reaches here with a non-zero found and
	// an empty queue, and would otherwise succeed having written nothing —
	// indistinguishable, from the exit code, from a build that worked.
	if len(queue) == 0 {
		return nil, fmt.Errorf("every Markdown file in the entry points is excluded")
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestCrawl -v`
Expected: PASS — the five new `TestCrawlExclude*` tests and every pre-existing `TestCrawl*` test.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add crawl.go crawl_test.go
git commit -m "feat: add CrawlOptions.Exclude, refusing excluded seeds and link targets"
```

---

### Task 3: Never walk into an excluded directory

Task 2 made exclusion correct; this makes it cheap and makes the spec's "never entered" literally true. `seed`'s `filepath.WalkDir` currently descends into every directory under an entry, so a multi-gigabyte vendored subtree is fully stat-walked only to have every file rejected at `admit`.

**Files:**
- Modify: `crawl.go` (`seed`, and its call in `Crawl`)
- Test: `crawl_test.go` (append)

**Interfaces:**
- Consumes: `isExcluded` (Task 1); `resolveExcludes` (Task 2).
- Produces: `seed` gains a third parameter — `func seed(entry string, depth int, excluded []string) ([]string, error)`.

- [ ] **Step 1: Write the failing test**

Append to `crawl_test.go`:

```go
// Exclusion must prune the directory walk, not just filter its results:
// an unreadable directory inside an excluded subtree must never be
// entered, so it cannot contribute anything at all.
func TestSeedSkipsExcludedDirectories(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/index.md":            "x",
		"docs/vendor/a.md":         "y",
		"docs/vendor/deep/b.md":    "z",
		"docs/keep/c.md":           "w",
	})
	got, err := seed(filepath.Join(root, "docs"), -1,
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
	got, err := seed(filepath.Join(root, "docs/vendor/a.md"), -1,
		[]string{filepath.Join(root, "docs/vendor")})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestSeedSkipsExcluded -v`
Expected: FAIL — `not enough arguments in call to seed`.

- [ ] **Step 3: Write the implementation**

In `crawl.go`, change `seed`'s signature and add the two checks:

```go
// seed returns the Markdown files a single entry point contributes.
// Paths at or beneath an excluded prefix contribute nothing, and an
// excluded directory is never descended into — admit would reject every
// file inside one anyway, but walking a vendored or archived subtree only
// to throw the whole result away is work nobody asked for.
func seed(entry string, depth int, excluded []string) ([]string, error) {
	info, err := os.Stat(entry)
	if err != nil {
		return nil, fmt.Errorf("entry point %s: %w", entry, err)
	}
	if !info.IsDir() {
		if !IsMarkdownPath(entry) {
			return nil, fmt.Errorf("entry point %s is not a Markdown file", entry)
		}
		p, err := resolve(entry)
		if err != nil {
			return nil, err
		}
		if isExcluded(p, excluded) {
			return nil, nil
		}
		return []string{p}, nil
	}

	rootAbs, err := resolve(entry)
	if err != nil {
		return nil, err
	}
	var out []string
	err = filepath.WalkDir(rootAbs, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip, do not abort
		}
		if d.IsDir() {
			if isExcluded(p, excluded) {
				return filepath.SkipDir
			}
			if depth < 0 || p == rootAbs {
				return nil
			}
			rel, relErr := filepath.Rel(rootAbs, p)
			if relErr != nil {
				return filepath.SkipDir
			}
			// A directory at level N contains files at level N; seeding
			// depth D admits directories up to level D.
			if pathDepth(rel) > depth {
				return filepath.SkipDir
			}
			return nil
		}
		if IsMarkdownPath(p) {
			// Resolve here too: the visited set is keyed on resolved paths,
			// so an unresolved seed would emit a symlink alias as a second
			// document alongside its real target.
			rp, rerr := resolve(p)
			if rerr != nil {
				rp = p
			}
			// Resolution can land a file inside an excluded subtree even
			// though the walk reached it outside one, via a symlink.
			if isExcluded(rp, excluded) {
				return nil
			}
			out = append(out, rp)
		}
		return nil
	})
	return out, err
}
```

Update the call in `Crawl`:

```go
		s, err := seed(e, opt.Depth, excluded)
```

The `rootAbs` directory itself is checked by the same `isExcluded` branch on the walk's first callback, so an entry point naming an excluded directory yields nothing — no separate guard needed.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS — the whole suite, including the `TestCrawlExclude*` tests from Task 2, which now take the pruned path.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add crawl.go crawl_test.go
git commit -m "feat: prune excluded directories from the seeding walk"
```

---

### Task 4: The `--exclude` CLI flag

Makes exclusion reachable from the command line. `--exclude` is repeatable *and* accepts comma-separated values in one occurrence, because both forms appear in real invocations and supporting only one guarantees somebody writes the other and gets a directory named `a,b`.

**Files:**
- Modify: `cmd/md2html/main.go` (`run`, `fs.Usage`)
- Test: `cmd/md2html/main_test.go` (append)

**Interfaces:**
- Consumes: `md2html.CrawlOptions.Exclude` (Task 2).
- Produces: `type stringList []string` in `package main`, implementing `flag.Value` (`String() string`, `Set(string) error`), splitting on commas.

- [ ] **Step 1: Write the failing test**

Append to `cmd/md2html/main_test.go`:

```go
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
```

`tree` is the existing helper at the top of `cmd/md2html/main_test.go`.
Add `"reflect"` to that file's import block; `bytes`, `os`, `path/filepath`
and `strings` are already there.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/md2html/ -run 'TestStringList|TestRunExclude' -v`
Expected: FAIL — `undefined: stringList`.

- [ ] **Step 3: Write the implementation**

In `cmd/md2html/main.go`, add above `run`:

```go
// stringList is a flag.Value collecting a repeatable, comma-separable
// option. Both spellings are accepted — "--exclude a --exclude b" and
// "--exclude a,b" — because supporting only one reliably produces a
// directory named "a,b" or a second flag that is silently ignored.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			*l = append(*l, p)
		}
	}
	return nil
}
```

Add `"strings"` to the import block.

Inside `run`, declare the flag next to the others (it cannot go in the
existing `var (...)` group, because `fs.Var` returns nothing):

```go
	var exclude stringList
	fs.Var(&exclude, "exclude", "directory prefix never to enter or write to (repeatable, comma-separated)")
```

Pass it through to `Crawl`:

```go
	res, err := md2html.Crawl(md2html.CrawlOptions{
		Entries:   entries,
		OutDir:    *outDir,
		Depth:     *depth,
		Exclude:   exclude,
		NoMdLinks: *noMd,
		NoAssets:  *noAssets,
	})
```

Update the usage text, which currently promises unconditional link
following. Replace the sentence in `fs.Usage`:

```go
		fmt.Fprint(stderr, `md2html - convert Markdown docs to HTML

Usage:
  md2html [flags] <entry> [entry...]

Entries may be files or directories, and several may be given in one run:
the emit set is their union, so an extra directory can be built for a
single invocation without being added to a standing entry list. Links
between documents are followed across directories, without limit unless
--link-depth says otherwise, and never into an --exclude'd directory.
Output never leaves -o.

Flags:
`)
```

(The `--link-depth` mention lands in Task 5; writing the final text now
keeps the usage block from being edited twice. If Task 5 is deferred,
drop that clause.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS — whole suite.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add cmd/md2html/main.go cmd/md2html/main_test.go
git commit -m "feat: add --exclude flag"
```

---

### Task 5: Bound link-following depth

The narrower half of spec item 3: today one stray link reaches arbitrarily far. This adds an opt-in hop limit measured from a seed.

**Zero-value trap.** `CrawlOptions.Depth` uses `-1` for unlimited, which means a zero-valued `CrawlOptions` seeds only one directory level. Copying that convention for link depth would silently turn every existing library caller's build into a seeds-only build. So `LinkDepth` inverts it: `0` (the zero value, and the flag's default) means unlimited, a positive `N` means `N` hops, and a negative value means follow no links at all. The inversion is deliberate and must be documented at the field, or the next reader will "fix" it.

**Files:**
- Modify: `crawl.go` (`CrawlOptions`, `Crawl`), `cmd/md2html/main.go` (`run`)
- Test: `crawl_test.go` (append), `cmd/md2html/main_test.go` (append)

**Interfaces:**
- Consumes: `admit(src, target, ref string, viaLink bool)` (Task 2) — gains a fifth parameter, `hops int`.
- Produces:
  - `CrawlOptions.LinkDepth int`.
  - `type queued struct { path string; hops int }` in `package md2html`; `queue` becomes `[]queued`.
  - CLI flag `--link-depth`.

- [ ] **Step 1: Write the failing test**

Append to `crawl_test.go`:

```go
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

// Every seed is hop zero, so depth is measured per seed rather than from
// whichever document happened to be dequeued first.
func TestCrawlLinkDepthMeasuredFromEachSeed(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/one.md":   "[deep](./deep.md)",
		"docs/two.md":   "[other](./other.md)",
		"docs/deep.md":  "end",
		"docs/other.md": "end",
	})
	res, err := Crawl(CrawlOptions{
		Entries:   []string{filepath.Join(root, "docs")},
		Depth:     -1,
		LinkDepth: 1,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	want := []string{"docs/deep.md", "docs/one.md", "docs/other.md", "docs/two.md"}
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
```

Append to `cmd/md2html/main_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestCrawlLinkDepth|TestRunLinkDepth' -v`
Expected: FAIL — `unknown field LinkDepth in struct literal of type CrawlOptions`.

- [ ] **Step 3: Write the implementation**

In `crawl.go`, add the field after `Exclude`:

```go
	// LinkDepth bounds how far link-following may travel from a seed,
	// counted in hops: a document a seed links to is one hop, one it links
	// to in turn is two.
	//
	// 0 — the zero value, and the CLI default — means unlimited, which is
	// the opposite of Depth's convention above and is deliberate. Depth was
	// introduced with the tool; LinkDepth is being added to callers who
	// already exist, and a zero-valued CrawlOptions has always followed
	// links without limit. Mirroring Depth's -1 would have turned every
	// such caller's build into a seeds-only build without a line of their
	// code changing. A negative value follows no links at all.
	LinkDepth int
```

Replace the queue declaration:

```go
	// queued pairs a document with its distance, in links, from the nearest
	// seed. BFS dequeues in nondecreasing hop order, so the first time a
	// document is admitted is always by its shortest path, and re-reaching
	// it later by a longer one cannot matter.
	type queued struct {
		path string
		hops int
	}

	res := &CrawlResult{Base: base}
	visited := map[string]bool{}
	var queue []queued
```

Note `queued` is declared inside `Crawl`, next to the queue it describes — nothing outside the function needs it.

Give `admit` the hop count of the document being admitted:

```go
	admit := func(src, target, ref string, viaLink bool, hops int) {
		if isExcluded(target, excluded) {
			if viaLink {
				res.Warnings = append(res.Warnings, Warning{src,
					fmt.Sprintf("not following %s: excluded directory", ref)})
			}
			return
		}
		if !isUnder(target, base) {
			if opt.OutDir == "" {
				res.Warnings = append(res.Warnings, Warning{src,
					fmt.Sprintf("refusing to follow %s outside %s (no -o given)", ref, base)})
				return
			}
			res.External = append(res.External, target)
		}
		if !visited[target] {
			visited[target] = true
			queue = append(queue, queued{target, hops})
		}
	}
```

Seeds are hop zero:

```go
			admit(entryAbs[i], p, p, false, 0)
```

In the BFS loop, dequeue the pair, and gate link-following on the limit:

```go
	for len(queue) > 0 {
		cur := queue[0].path
		hops := queue[0].hops
		queue = queue[1:]
```

and, in the link loop, replace the `admit` call with:

```go
			admit(cur, target, l.Href, true, hops+1)
```

Finally, gate link-following on the limit. The guard goes inside the
`for _, l := range links` loop, on the last two lines — directly replacing
the existing `admit(cur, target, l.Href)` call — so that the asset checks
earlier in the same loop body still run:

```go
			// LinkDepth 0 is unlimited; a negative value follows nothing.
			// The asset-existence warnings above stay unconditional —
			// a missing image is worth reporting whether or not this
			// document's links are being followed.
			if opt.LinkDepth < 0 || (opt.LinkDepth > 0 && hops >= opt.LinkDepth) {
				continue
			}
			admit(cur, target, l.Href, true, hops+1)
```

In `cmd/md2html/main.go`, add to the `var (...)` flag group:

```go
		linkDepth = fs.Int("link-depth", 0, "hops from a seed that link-following may travel (0 unlimited, -1 none)")
```

and pass it through:

```go
		LinkDepth: *linkDepth,
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS — whole suite, including `TestCrawlFollowsLinksAcrossDirectoriesUnbounded`, which never sets `LinkDepth` and so proves the zero value still means unlimited.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add crawl.go crawl_test.go cmd/md2html/main.go cmd/md2html/main_test.go
git commit -m "feat: add --link-depth to bound link-following"
```

---

### Task 6: Document both flags and the multi-entry idiom

Spec item 9 asks for nothing but documentation: multiple entry-point arguments already serve the "build the maintained set plus this one extra directory, just for today" case, and the only gap is that no example says so. That belongs in the same pass as the two new flags.

**Files:**
- Modify: `docs/authoring.md` (the "Traps" list), `README.md`
- Test: none — this task is prose. Its verification is Step 3.

**Interfaces:**
- Consumes: `--exclude` (Task 4), `--link-depth` (Task 5), and the usage text already updated in Task 4.
- Produces: no code.

- [ ] **Step 1: Update README**

Find the flag table or list in `README.md` and add two rows in the same style the file already uses:

| Flag | Meaning |
|---|---|
| `--exclude DIR` | Never enter, seed, follow into, or write to `DIR` (relative to the base, or absolute). Repeatable, or comma-separated. |
| `--link-depth N` | Follow links at most `N` hops from a seed. `0` (default) is unlimited; `-1` follows none. |

Then add a short usage example beneath the existing ones:

```bash
# The maintained set, plus one scratch directory for this run only,
# with a vendored subtree another tool owns left strictly alone.
md2html -o ./site ./docs ./scratch/notes --exclude vendor
```

- [ ] **Step 2: Update the authoring guide**

In `docs/authoring.md`, the "Traps" list currently says link following is
unconditional. Add one bullet to that list:

```markdown
- **A link into an `--exclude`d directory keeps its href as written.** The
  target is never converted, so the link resolves to whatever that subtree's
  own tool produced — which is the point — but nothing checks that it did.
  The run warns once per such link.
```

And extend the "No navigation is generated" bullet's neighbourhood with a
note on multi-entry builds, since that is the documented answer to spec
item 9:

```markdown
- **Several entry points build as one set.** `md2html -o ./site ./docs
  ./notes` emits both trees into one output root, with links between them
  rewritten. Nothing has to be added to a standing entry list to include a
  directory for a single run.
```

- [ ] **Step 3: Verify every documented claim against the binary**

`docs/authoring.md` states that every claim in it is verified against the
binary. Hold the new text to that:

```bash
go build -o /tmp/md2html ./cmd/md2html
/tmp/md2html 2>&1 | head -20          # usage text mentions both flags
mkdir -p /tmp/t/docs /tmp/t/vendor
printf '[v](../vendor/x.md)\n' > /tmp/t/docs/i.md
printf '# V\n' > /tmp/t/vendor/x.md
/tmp/md2html -o /tmp/out /tmp/t --exclude vendor
test ! -e /tmp/out/vendor/x.html && echo "exclusion verified"
grep -o 'href="[^"]*"' /tmp/out/docs/i.html   # expect ../vendor/x.md, unrewritten
```

Expected: the usage block lists `--exclude` and `--link-depth`; `exclusion verified` prints; the href is `../vendor/x.md`, unchanged.

- [ ] **Step 4: Commit**

```bash
git add README.md docs/authoring.md
git commit -m "docs: document --exclude, --link-depth, and multi-entry builds"
```

---

## Spec coverage

| Spec item | Where |
|---|---|
| 3 — `--exclude`, repeatable/comma-separated, prefix relative to base | Tasks 2, 4 |
| 3 — never seeded, never followed, never written | Tasks 2, 3 |
| 3 — excluded link renders with href intact | Task 2 (falls out of `buildLinkMaps`; asserted in `TestCrawlExcludeRefusesLinkTarget`) |
| 3 — exclusion checked before containment, both able to stop a path | Task 2 (`admit`) |
| 3 — traversal depth for link-following, distinct from `--depth` | Task 5 |
| 3 — unbounded stays the default | Task 5 (`LinkDepth` zero value; `TestCrawlLinkDepthZeroIsUnlimited`) |
| 9 — supplemental per-invocation entry set | Task 6 (documentation only, as the spec prescribes) |

## Not in this plan

Spec items 1, 2, 4, 5, 6, 7 and 8 are content-model features with no
traversal surface; they are planned separately in
`docs/plans/2026-09-11-extended-content-model.md`, except item 1
(structured diagram fence), which the spec itself defers to its own design.
