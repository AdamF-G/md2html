# Site Links and Assets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A site build holds everything it shows. It copies the files its pages load or link into the output, writes nothing under `_external/`, and turns a link out of the site into the file's page on the repository's host, plain text, or a stopped build, as the shell says.

**Architecture:** `CrawlOptions` gains `SiteShell` and `SourceRoot`. With a shell and an output directory, `Crawl` builds a `siteRules` from the shell's front matter (`repo-url:`, `unmounted:`, `source-root:`) and stops following links out of the run's base. Every link a page or the shell holds goes through one decision, `siteRules.action`, which says to rewrite it, copy a file for it, drop it, or fail the build. The crawl records the copies in `CrawlResult.Assets` and each page's drops in `Doc.LinkDrops`. A new `LinkDrop` transform removes a dropped link, keeping an `<a>`'s text and an image's `alt`, and `CopyAssets` copies the files, with a ledger so a later build replaces its own copies and refuses anyone else's file. `ExtractLinks` learns `srcset` and `poster`, and marks each link as loaded by the page or followed by a reader.

**Tech Stack:** Go 1.26; `golang.org/x/net/html`; `crypto/sha256` for the ledger.

**Spec:** `docs/specs/2026-10-16-mounts-design.md`, §2 and §4 (Links and assets), as they apply to every site build. Mounts, `--mount`, the mounted read bound, `toc="shell"`, directory pages and `toc: none` under a shell belong to later plans. Here, a file is **in the site** when it is under the run's base, the case the spec gives for a site built without mounts; the mounts plan widens that to the mounts.

## Global Constraints

- A branch `site-links` from `main`. Conventional prefixes. Commit bodies wrapped at 72 columns, starting from what was wrong before. No attribution trailer. Nothing is pushed.
- **Only a site build changes**: one with a shell (`CrawlOptions.SiteShell`, `--site`) and an output directory. A build without `--site`, or a site built in place, is byte-for-byte what it was, except that `srcset` candidates and a `<video>`'s `poster` are now found and rewritten like any other asset link (spec §4: "Today's link rewriting finds every one of these but `srcset` and `poster`, which it learns").
- An asset is anything a page loads: the `src` of `<img>`, `<script>`, `<source>`, `<video>`, `<audio>`, `<iframe>`, `<embed>`, each `srcset` candidate, a `<video>`'s `poster`, and a `<link>`'s `href`. Only an `<a>`'s `href` is a link a reader follows.
- Shell front matter keys, exactly: `repo-url:` (must contain `{path}`), `unmounted:` (`link`, `unlink` or `error`; default `link` when `repo-url:` is set, else `unlink`), `source-root:` (relative to the shell). Flag: `--source-root DIR` (relative to the working directory, wins over the key, needs `--site`).
- The default source root is the deepest directory holding the shell and the run's base. A default that is the filesystem's root is refused, asking for `source-root:`.
- Copies of files from outside the site go under `_assets/` at their path from the source root.
- A file under an `--exclude`d path keeps its link exactly as written, as `CrawlOptions.Exclude` already promises.
- `--no-assets` copies nothing and leaves every link that would have led to a copy as written, an `<a>` to a file in the site included; a link out of the site still follows `unmounted:`.
- `CopyAssets` never replaces a file it cannot show it wrote: the ledger `md2html-assets.txt` at the output root records each copy's SHA-256.
- Run `gofmt -l .` (empty), `go vet ./...` and `go test ./...` before every commit; Task 8 also runs `just e2e` and `just site`.

## Review Focus

1. **An asset whose output path collides with a page's**, such as `docs/a.html` beside `docs/a.md`. The build must stop naming both, as two pages colliding do, not race in the parallel emit. Pinned in Task 4.
2. **A second build into the same output directory.** Every copy must be replaced or left, never refused, and an edited copy must be refused, not overwritten. Pinned in Task 6 and Task 7.
3. **A link with a fragment or query to a file that leaves the site** (`../site.go#L10`). The repository URL keeps the fragment. Pinned in Task 4.
4. **The same out-of-site link on many pages and in the shell.** The shell's warning is said once, under the shell's path, not once per page. Pinned in Task 5.
5. **A name with a space or `#` in a copied asset's path.** The rewritten href must be percent-encoded as page links are. Pinned in Task 4.

---

### Task 1: `srcset`, `poster`, and loaded links

**Files:**
- Modify: `links.go` (`Link`, `linkAttrFor` → `linkAttrsFor`, `ExtractLinks`, new `srcsetURLs`)
- Modify: `transform.go` (`LinkRewrite`)
- Test: `links_test.go`, `linkmap_test.go`

**Interfaces:**
- Produces: `Link.Loaded bool` (true unless the element is an `<a>`); `func linkAttrsFor(n *html.Node) []string`; `func srcsetURLs(v string) []srcsetCandidate`, `type srcsetCandidate struct{ url, desc string }`, `func joinSrcset(cs []srcsetCandidate) string`. A `srcset` link has `Attr == "srcset"` and `Href` set to one candidate's URL.

- [ ] **Step 1: Write the failing tests.** In `links_test.go`, after `TestExtractLinksFindsRawHTMLLinks`:

```go
// Every candidate of an srcset, and a video's poster, is a link like a src.
// Each link says whether the page loads it or a reader follows it: only an
// <a> is followed.
func TestExtractLinksFindsSrcsetAndPosterAndMarksLoaded(t *testing.T) {
	in := `<img src="a.png" srcset="a.png 1x, ./b@2x.png 2x">` +
		`<video src="v.mp4" poster="p.jpg"></video>` +
		`<link rel="stylesheet" href="s.css">` +
		`<a href="doc.pdf">d</a>`
	root, _ := parseFragment([]byte(in))
	type got struct {
		attr   string
		loaded bool
	}
	seen := map[string]got{}
	var order []string
	for _, l := range ExtractLinks(root, "/docs") {
		seen[l.Attr+" "+l.Href] = got{l.Attr, l.Loaded}
		order = append(order, l.Attr+" "+l.Href)
	}
	want := []string{"src a.png", "srcset a.png", "srcset ./b@2x.png", "src v.mp4", "poster p.jpg", "href s.css", "href doc.pdf"}
	if strings.Join(order, "|") != strings.Join(want, "|") {
		t.Errorf("links:\n%v\nwant:\n%v", order, want)
	}
	for k, g := range seen {
		if wantLoaded := k != "href doc.pdf"; g.loaded != wantLoaded {
			t.Errorf("%s: loaded = %v, want %v", k, g.loaded, wantLoaded)
		}
	}
}

func TestSrcsetURLsKeepsDescriptors(t *testing.T) {
	cs := srcsetURLs(" a.png 1x,b.png  2x , c.png")
	if len(cs) != 3 || cs[0].url != "a.png" || cs[0].desc != "1x" || cs[1].url != "b.png" || cs[1].desc != "2x" || cs[2].url != "c.png" || cs[2].desc != "" {
		t.Fatalf("got %+v", cs)
	}
	if got := joinSrcset(cs); got != "a.png 1x, b.png 2x, c.png" {
		t.Errorf("joined %q", got)
	}
}
```

Add `"strings"` to `links_test.go`'s imports. In `linkmap_test.go`, after `TestLinkRewriteLeavesUnmappedLinksAlone`:

```go
// LinkRewrite rewrites each srcset candidate on its own, keeping its
// descriptor, and a poster as it does a src.
func TestLinkRewriteRewritesSrcsetAndPoster(t *testing.T) {
	m := map[string]string{"a.png": "../docs/a.png", "b.png": "../docs/b.png", "p.jpg": "../docs/p.jpg"}
	got := apply(t, `<img srcset="a.png 1x, b.png 2x, c.png 3x"><video poster="p.jpg"></video>`, LinkRewrite(m))
	for _, want := range []string{`srcset="../docs/a.png 1x, ../docs/b.png 2x, c.png 3x"`, `poster="../docs/p.jpg"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}
```

- [ ] **Step 2: Run them and see them fail.**

Run: `go test -run 'Srcset|SrcsetAndPoster' .`
Expected: FAIL: `l.Loaded` and `srcsetURLs` are undefined.

- [ ] **Step 3: Implement.** In `links.go`, add a field to `Link`, after `Attr`:

```go
	// Attr is the attribute on Node carrying the link: "href", "src",
	// "srcset" (Href is then one candidate's URL) or "poster".
	Attr string
	// Loaded is true for a link the page loads, an image, script,
	// stylesheet or frame, and false for one a reader follows, an <a>.
	Loaded bool
```

(replacing the existing `Attr` field and its comment). Replace `linkAttrFor` with:

```go
// linkAttrsFor returns the attributes carrying links on the given element,
// in the order they are read, or nil when it carries none.
func linkAttrsFor(n *html.Node) []string {
	switch n.DataAtom {
	case atom.A, atom.Link:
		return []string{"href"}
	case atom.Img, atom.Source:
		return []string{"src", "srcset"}
	case atom.Video:
		return []string{"src", "poster"}
	case atom.Script, atom.Audio, atom.Iframe, atom.Embed:
		return []string{"src"}
	}
	return nil
}

// srcsetCandidate is one image candidate of an srcset: its URL and its
// width or density descriptor, if any.
type srcsetCandidate struct{ url, desc string }

// srcsetURLs splits an srcset into its candidates. Candidates are separated
// by commas, and a URL from its descriptor by whitespace.
func srcsetURLs(v string) []srcsetCandidate {
	var out []srcsetCandidate
	for _, part := range strings.Split(v, ",") {
		f := strings.Fields(part)
		if len(f) == 0 {
			continue
		}
		out = append(out, srcsetCandidate{url: f[0], desc: strings.Join(f[1:], " ")})
	}
	return out
}

// joinSrcset writes candidates back as an srcset value.
func joinSrcset(cs []srcsetCandidate) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = c.url
		if c.desc != "" {
			parts[i] += " " + c.desc
		}
	}
	return strings.Join(parts, ", ")
}
```

Replace `ExtractLinks`'s body so it reads every attribute, and every srcset candidate, through one classifier:

```go
func ExtractLinks(root *html.Node, srcDir string) []Link {
	var out []Link
	walk(root, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		loaded := n.DataAtom != atom.A
		for _, key := range linkAttrsFor(n) {
			v, ok := attr(n, key)
			if !ok || v == "" {
				continue
			}
			hrefs := []string{v}
			if key == "srcset" {
				hrefs = hrefs[:0]
				for _, c := range srcsetURLs(v) {
					hrefs = append(hrefs, c.url)
				}
			}
			for _, href := range hrefs {
				out = append(out, classifyLink(Link{Href: href, Node: n, Attr: key, Loaded: loaded}, srcDir))
			}
		}
	})
	return out
}

// classifyLink sets l's Kind, and its Abs for a local file.
func classifyLink(l Link, srcDir string) Link {
	href := l.Href
	switch {
	case strings.HasPrefix(href, "#"):
		l.Kind = LinkFragment
	case isRemote(href):
		l.Kind = LinkRemote
	default:
		// Strip any fragment: ./guide.md#setup addresses ./guide.md.
		pathPart := href
		if i := strings.IndexByte(pathPart, '#'); i >= 0 {
			pathPart = pathPart[:i]
		}
		if i := strings.IndexByte(pathPart, '?'); i >= 0 {
			pathPart = pathPart[:i]
		}
		if pathPart == "" {
			l.Kind = LinkFragment
			break
		}
		// goldmark percent-encodes hrefs, so "./my doc.md" renders as
		// "./my%20doc.md". Decode before touching the filesystem; Href
		// keeps the encoded form, which is what LinkMap is keyed by.
		if decoded, decErr := url.PathUnescape(pathPart); decErr == nil {
			pathPart = decoded
		}
		if filepath.IsAbs(pathPart) {
			l.Abs = filepath.Clean(pathPart)
		} else {
			l.Abs = filepath.Clean(filepath.Join(srcDir, pathPart))
		}
		if IsMarkdownPath(pathPart) {
			l.Kind = LinkDoc
		} else {
			l.Kind = LinkAsset
		}
	}
	return l
}
```

The classifying code is moved, not changed. Keep the `ExtractLinks` doc comment.

In `transform.go`, replace `LinkRewrite`'s walk body:

```go
		walk(root, func(n *html.Node) {
			if n.Type != html.ElementNode {
				return
			}
			for _, key := range linkAttrsFor(n) {
				v, ok := attr(n, key)
				if !ok {
					continue
				}
				if key == "srcset" {
					cs := srcsetURLs(v)
					changed := false
					for i, c := range cs {
						if repl, found := m[c.url]; found {
							cs[i].url, changed = repl, true
						}
					}
					if changed {
						setAttr(n, key, joinSrcset(cs))
					}
					continue
				}
				if repl, found := m[v]; found {
					setAttr(n, key, repl)
				}
			}
		})
```

- [ ] **Step 4: Run the tests and see them pass.**

Run: `go test ./...`
Expected: PASS, the existing link and link-map tests included.

- [ ] **Step 5: Commit.**

```bash
git add links.go transform.go links_test.go linkmap_test.go
git commit -F - <<'EOF'
feat: srcset candidates and a video's poster are links too

ExtractLinks read one attribute per element, so an image's srcset and
a video's poster were never found: a build into another directory
rewrote an image's src to reach its file and left the srcset and the
poster pointing at paths that no longer led anywhere.

Each srcset candidate is now a link of its own, rewritten with its
descriptor kept, and a poster is read as a src is. Each link also
says whether the page loads it, an image, script, stylesheet or
frame, or a reader follows it, an <a>, which a site build needs to
tell an image from a link to a file.
EOF
```

---

### Task 2: `LinkDrop`

**Files:**
- Modify: `transform.go` (new `LinkDrop`)
- Modify: `md2html.go` (`Options.LinkDrops`, `Convert`)
- Test: `linkmap_test.go`

**Interfaces:**
- Consumes: `linkAttrsFor`, `srcsetURLs`, `joinSrcset` (Task 1).
- Produces: `func LinkDrop(m map[string]bool) Transform` (name `"linkDrop"`), `Options.LinkDrops map[string]bool`. Convert runs `LinkDrop(opt.LinkDrops)` just before `LinkRewrite(opt.LinkMap)`.

- [ ] **Step 1: Write the failing tests.** In `linkmap_test.go`:

```go
// A dropped link goes, and what it said stays: an <a> keeps its children,
// an image its alt text, an srcset loses only that candidate, and any
// other element is removed.
func TestLinkDropKeepsTextAndAlt(t *testing.T) {
	m := map[string]bool{"../LICENSE": true, "../../pic.png": true, "x.mp4": true, "big.png": true}
	got := apply(t, `<p>See <a href="../LICENSE">the <b>licence</b></a>.</p>`+
		`<p><img src="../../pic.png" alt="A picture"></p>`+
		`<p><img src="ok.png" srcset="ok.png 1x, big.png 2x"></p>`+
		`<p><video src="x.mp4"></video>after</p>`, LinkDrop(m))
	for _, want := range []string{
		`<p>See the <b>licence</b>.</p>`,
		`<p>A picture</p>`,
		`<img src="ok.png" srcset="ok.png 1x"/>`,
		`<p>after</p>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
}

func TestConvertAppliesLinkDropsBeforeTheMap(t *testing.T) {
	out, err := Convert([]byte("[licence](../LICENSE) and [guide](./g.md)\n"), Options{Fragment: true,
		LinkDrops: map[string]bool{"../LICENSE": true}, LinkMap: map[string]string{"./g.md": "g.html"}})
	if err != nil {
		t.Fatal(err)
	}
	if s := string(out); strings.Contains(s, "LICENSE") || !strings.Contains(s, "licence and") || !strings.Contains(s, `href="g.html"`) {
		t.Errorf("got:\n%s", s)
	}
}
```

- [ ] **Step 2: Run them and see them fail.**

Run: `go test -run 'LinkDrop' .`
Expected: FAIL: `LinkDrop` and `Options.LinkDrops` are undefined.

- [ ] **Step 3: Implement.** In `transform.go`, after `LinkRewrite`:

```go
// LinkDrop removes each link in m, keyed by the href as written, and keeps
// what it said: an <a> is replaced by its children, an <img> by its alt
// text, a dropped srcset candidate leaves the others, and any other element
// carrying a dropped link is removed. A site build drops a link it cannot
// keep, one leading outside the site that it has nowhere to send.
func LinkDrop(m map[string]bool) Transform {
	return Transform{Name: "linkDrop", Fn: func(root *html.Node) error {
		if len(m) == 0 {
			return nil
		}
		var gone []*html.Node
		walk(root, func(n *html.Node) {
			if n.Type != html.ElementNode {
				return
			}
			for _, key := range linkAttrsFor(n) {
				v, ok := attr(n, key)
				if !ok {
					continue
				}
				if key == "srcset" {
					var kept []srcsetCandidate
					for _, c := range srcsetURLs(v) {
						if !m[c.url] {
							kept = append(kept, c)
						}
					}
					if len(kept) == 0 {
						removeAttr(n, key)
					} else {
						setAttr(n, key, joinSrcset(kept))
					}
					continue
				}
				if m[v] {
					gone = append(gone, n)
					return
				}
			}
		})
		// Removed after the walk, so it never steps from a detached node.
		for _, n := range gone {
			p := n.Parent
			if p == nil {
				continue
			}
			switch n.DataAtom {
			case atom.A:
				for c := n.FirstChild; c != nil; {
					next := c.NextSibling
					n.RemoveChild(c)
					p.InsertBefore(c, n)
					c = next
				}
			case atom.Img:
				if alt, _ := attr(n, "alt"); alt != "" {
					p.InsertBefore(textNode(alt), n)
				}
			}
			p.RemoveChild(n)
		}
		return nil
	}}
}
```

`removeAttr` (container.go) and `textNode` (marker.go) already exist; `transform.go` already imports `atom`.

In `md2html.go`, add after `LinkMap` in `Options`:

```go
	// LinkDrops names hrefs, as written in the source, whose links are
	// removed, keeping an <a>'s text and an image's alt. Populated by the
	// crawler in a site build for links it cannot keep; nil otherwise.
	// Convert runs LinkDrop(LinkDrops) just before LinkRewrite(LinkMap).
	LinkDrops map[string]bool
```

and in `Convert`, replace the `if opt.LinkMap != nil { … }` block with:

```go
	if opt.LinkDrops != nil {
		transforms = append(transforms[:len(transforms):len(transforms)],
			LinkDrop(opt.LinkDrops))
	}
	if opt.LinkMap != nil {
		// Full-slice expression: never append into the caller's array.
		transforms = append(transforms[:len(transforms):len(transforms)],
			LinkRewrite(opt.LinkMap))
	}
```

- [ ] **Step 4: Run the tests and see them pass.**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add transform.go md2html.go linkmap_test.go
git commit -F - <<'EOF'
feat: LinkDrop removes a link and keeps what it said

A link could only be rewritten or left as written, so a build that had
nowhere to send a link could only leave it pointing at a path that
does not exist where the page is published.

LinkDrop, and Options.LinkDrops, remove such a link and keep its
words: an <a> is replaced by its children, an image by its alt text,
an srcset loses only the candidate, and any other element carrying
the link goes. Convert drops before it rewrites.
EOF
```

---

### Task 3: A site's link rules

**Files:**
- Create: `sitelinks.go`, `sitelinks_test.go`
- Modify: `crawl.go` (`CrawlOptions.SiteShell`, `CrawlOptions.SourceRoot`)

**Interfaces:**
- Produces:

```go
type siteRules struct {
	base, sourceRoot, outDir, repoURL, unmounted string
	noAssets                                    bool
	excluded                                    exclusions
}
func newSiteRules(opt CrawlOptions, base string, excluded exclusions) (*siteRules, error) // nil, nil when not a site build into a directory
func defaultSourceRoot(shell, base string) (string, error)
```

- [ ] **Step 1: Write the failing tests** in `sitelinks_test.go`:

```go
package md2html

import (
	"path/filepath"
	"strings"
	"testing"
)

// rulesFor reads the rules of a shell at root/site/shell.md, whose front
// matter adds shellFront, for pages under root/docs.
func rulesFor(t *testing.T, shellFront, sourceRootFlag string) (*siteRules, string, error) {
	t.Helper()
	root := writeTree(t, map[string]string{
		"site/shell.md": "---\nsite: true\n" + shellFront + "---\n[[content]]\n",
		"docs/index.md": "# Home\n",
	})
	opt := CrawlOptions{SiteShell: filepath.Join(root, "site/shell.md"), SourceRoot: sourceRootFlag,
		OutDir: filepath.Join(root, "out")}
	r, err := newSiteRules(opt, filepath.Join(root, "docs"), exclusions{})
	return r, root, err
}

func TestSiteRulesDefaults(t *testing.T) {
	r, root, err := rulesFor(t, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.unmounted != "unlink" || r.sourceRoot != root {
		t.Errorf("unmounted %q, source root %s; want unlink and %s", r.unmounted, r.sourceRoot, root)
	}
	r, _, err = rulesFor(t, "repo-url: https://example.org/r/blob/main/{path}\n", "")
	if err != nil || r.unmounted != "link" {
		t.Errorf("with repo-url: %+v, %v; want unmounted link", r, err)
	}
}

func TestSiteRulesSourceRootKeyAndFlag(t *testing.T) {
	r, root, err := rulesFor(t, "source-root: .\n", "")
	if err != nil || r.sourceRoot != filepath.Join(root, "site") {
		t.Errorf("source-root: . gave %+v, %v; want the shell's directory", r, err)
	}
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	r, _, err = rulesFor(t, "source-root: .\n", dir)
	if err != nil || r.sourceRoot != dir {
		t.Errorf("--source-root gave %+v, %v; want %s, winning over the key", r, err, dir)
	}
}

func TestSiteRulesRefuses(t *testing.T) {
	for front, want := range map[string]string{
		"unmounted: maybe\n":               `unmounted "maybe" is not link, unlink or error`,
		"unmounted: link\n":                "unmounted: link needs repo-url:",
		"repo-url: https://example.org/\n": "repo-url: needs a {path}",
		"source-root: no/such/dir\n":       "source-root:",
	} {
		if _, _, err := rulesFor(t, front, ""); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err %v, want one containing %q", front, err, want)
		}
	}
}

// The default source root is the deepest directory holding the shell and
// the base. When that is the filesystem's root, every path measured from
// it would carry the machine's own, so it is refused.
func TestDefaultSourceRootRefusesTheFilesystemRoot(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skip("unix paths")
	}
	if got, err := defaultSourceRoot("/repo/site/shell.md", "/repo/docs"); err != nil || got != "/repo" {
		t.Errorf("got %q, %v; want /repo", got, err)
	}
	if _, err := defaultSourceRoot("/repo/site/shell.md", "/tmp/gen"); err == nil || !strings.Contains(err.Error(), "source-root:") {
		t.Errorf("a default at / was not refused: %v", err)
	}
}

// Without a shell, or without an output directory, there are no rules and
// the build is what it was.
func TestSiteRulesOnlyForASiteBuildIntoADirectory(t *testing.T) {
	root := writeTree(t, map[string]string{"site/shell.md": "---\nsite: true\n---\n[[content]]\n"})
	for _, opt := range []CrawlOptions{
		{OutDir: filepath.Join(root, "out")},
		{SiteShell: filepath.Join(root, "site/shell.md")},
	} {
		if r, err := newSiteRules(opt, root, exclusions{}); r != nil || err != nil {
			t.Errorf("%+v: got %+v, %v; want no rules", opt, r, err)
		}
	}
}
```

- [ ] **Step 2: Run them and see them fail.**

Run: `go test -run 'SiteRules|DefaultSourceRoot' .`
Expected: FAIL: `SiteShell`, `SourceRoot`, `newSiteRules` and `defaultSourceRoot` are undefined.

- [ ] **Step 3: Implement.** In `crawl.go`, add to `CrawlOptions`, after `LinkDepth`:

```go
	// SiteShell is the site shell a build is wrapped in, or empty. With an
	// OutDir, it makes the crawl a site's: links out of Base are not
	// followed, the files pages load or link are copied into the output
	// (CrawlResult.Assets), and a link out of the site becomes what the
	// shell's front matter says, with repo-url:, unmounted: and
	// source-root:. See siteRules.
	SiteShell string
	// SourceRoot overrides the shell's source-root: and the default, the
	// deepest directory holding the shell and Base. Relative to the working
	// directory. Only a site build reads it.
	SourceRoot string
```

Create `sitelinks.go`:

```go
package md2html

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// siteRules is what a site build does with the links its pages and shell
// hold: which files are in the site, where copies go, what a link out of
// the site becomes, and the source root paths out of it are measured from.
// A build without a shell, or one written in place, has none, and does what
// it always did.
type siteRules struct {
	base       string // a file under it is in the site
	sourceRoot string // _assets/ and repo-url: paths are measured from it
	outDir     string
	repoURL    string // with a {path} placeholder
	unmounted  string // link, unlink or error
	noAssets   bool   // copy nothing, and leave what would be copied as written
	excluded   exclusions
}

// newSiteRules reads a site's link settings from its shell's front matter.
// It returns nil, and no error, for a build that is not a site's or that
// writes in place, and an error for a setting the build cannot honour.
func newSiteRules(opt CrawlOptions, base string, excluded exclusions) (*siteRules, error) {
	if opt.SiteShell == "" || opt.OutDir == "" {
		return nil, nil
	}
	shell, err := filepath.Abs(opt.SiteShell)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(shell)
	if err != nil {
		return nil, fmt.Errorf("site shell: %w", err)
	}
	meta, _, _ := splitFrontMatter(raw)
	r := &siteRules{base: base, outDir: opt.OutDir, repoURL: meta["repo-url"],
		noAssets: opt.NoAssets, excluded: excluded}
	if r.repoURL != "" && !strings.Contains(r.repoURL, "{path}") {
		return nil, fmt.Errorf("%s: repo-url: needs a {path}, where a file's path from the source root goes", opt.SiteShell)
	}
	switch v := meta["unmounted"]; v {
	case "":
		r.unmounted = "unlink"
		if r.repoURL != "" {
			r.unmounted = "link"
		}
	case "unlink", "error":
		r.unmounted = v
	case "link":
		if r.repoURL == "" {
			return nil, fmt.Errorf("%s: unmounted: link needs repo-url:, the address a link out of the site goes to", opt.SiteShell)
		}
		r.unmounted = v
	default:
		return nil, fmt.Errorf("%s: unmounted %q is not link, unlink or error", opt.SiteShell, v)
	}
	switch {
	case opt.SourceRoot != "":
		r.sourceRoot, err = resolveDir(opt.SourceRoot, "--source-root")
	case meta["source-root"] != "":
		r.sourceRoot, err = resolveDir(filepath.Join(filepath.Dir(shell), meta["source-root"]), opt.SiteShell+": source-root:")
	default:
		r.sourceRoot, err = defaultSourceRoot(shell, base)
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

// resolveDir returns p, absolute and with its symlinks resolved, or an
// error naming where it was given when it is not a directory.
func resolveDir(p, from string) (string, error) {
	abs, err := resolve(p)
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", from, p, err)
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%s %s is not a directory", from, p)
	}
	return abs, nil
}

// defaultSourceRoot is the deepest directory holding the shell and the
// run's base, which for a repository's documentation is the repository's
// root. When that is the filesystem's root, every path measured from it
// would carry the building machine's own, so it is refused.
func defaultSourceRoot(shell, base string) (string, error) {
	root := commonAncestor([]string{shell, base})
	if filepath.Dir(root) == root {
		return "", fmt.Errorf("the site's source root would be the filesystem's root %s, since the shell and the pages share no other directory; set source-root: in the shell or --source-root", root)
	}
	return root, nil
}
```

`resolve` (crawl.go) returns the symlink-resolved absolute path and errors for a missing one.

- [ ] **Step 4: Run the tests and see them pass.**

Run: `go test -run 'SiteRules|DefaultSourceRoot' . && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add crawl.go sitelinks.go sitelinks_test.go
git commit -F - <<'EOF'
feat: a site shell says where links out of the site go

A site build had no say over a link out of the site: it pointed back
at the file on the building machine, which does not exist where the
site is published.

CrawlOptions.SiteShell and SourceRoot give a crawl a site's rules,
read from the shell's front matter: repo-url:, a URL with a {path}
for a file's path from the source root; unmounted:, which says
whether such a link goes there, is unlinked, or stops the build; and
source-root:. The source root defaults to the deepest directory
holding the shell and the pages, and a default at the filesystem's
root is refused. The rules are read here; the next change uses them.
EOF
```

---

### Task 4: The crawl keeps a site to itself

**Files:**
- Modify: `crawl.go` (`Doc.LinkDrops`, `CrawlResult.Assets`, `CrawlResult.rules`, `Crawl`, `buildLinkMaps`, `linkMapFor` → `planLinks`)
- Modify: `sitelinks.go` (`Asset`, `linkAction`, `siteRules.action`, `siteRules.leaving`)
- Test: `sitecrawl_test.go`

**Interfaces:**
- Consumes: `siteRules`, `newSiteRules` (Task 3); `Link.Loaded` (Task 1).
- Produces:

```go
type Asset struct{ Src, Out string } // exported, in CrawlResult.Assets, sorted by Out
// Doc.LinkDrops map[string]bool
// CrawlResult.rules *siteRules (unexported; BuildSite reads it)
type linkPlan struct {
	rewrite map[string]string
	drop    map[string]bool
	copies  []Asset
	warns   []string
	fails   []string
}
func planLinks(links []Link, fromOut string, outBySrc map[string]string, opt CrawlOptions, rules *siteRules) linkPlan
```

- [ ] **Step 1: Write the failing tests** in `sitecrawl_test.go`:

```go
package md2html

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// siteCrawl crawls root/repo/docs as a site wrapped in
// root/repo/site/shell.md, into root/out. files are relative to repo/, so
// "../far.txt" sits beside the repository, outside the default source
// root, which is repo/.
func siteCrawl(t *testing.T, shellFront string, files map[string]string, mod func(*CrawlOptions)) (*CrawlResult, string, error) {
	t.Helper()
	all := map[string]string{"repo/site/shell.md": "---\nsite: true\n" + shellFront + "---\n[[content]]\n"}
	for k, v := range files {
		all["repo/"+k] = v
	}
	root := writeTree(t, all)
	opt := CrawlOptions{Entries: []string{filepath.Join(root, "repo/docs")}, OutDir: filepath.Join(root, "out"),
		Depth: -1, LinkDepth: -1, SiteShell: filepath.Join(root, "repo/site/shell.md")}
	if mod != nil {
		mod(&opt)
	}
	res, err := Crawl(opt)
	return res, root, err
}

var siteLinkFiles = map[string]string{
	"docs/index.md": "# Home\n\n" +
		"![flow](img/flow.png) [guide](guide.md) [pdf](files/my%20doc.pdf)\n\n" +
		"[licence](../LICENSE) [readme](../README.md#install) [code](../site.go#L10)\n\n" +
		"![logo](../logo.png)\n\n" +
		"[far](../../far.txt) ![farpic](../../farpic.png)\n",
	"docs/guide.md":         "# Guide\n",
	"docs/img/flow.png":     "png",
	"docs/files/my doc.pdf": "pdf",
	"LICENSE":               "MIT",
	"README.md":             "# Readme\n",
	"site.go":               "package x",
	"logo.png":              "png",
	"../far.txt":            "far",
	"../farpic.png":         "png",
}

func TestSiteCrawlKeepsTheSiteToItself(t *testing.T) {
	res, root, err := siteCrawl(t, "repo-url: https://example.org/r/blob/main/{path}\n", siteLinkFiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	// ../../far.txt and ../../farpic.png are beside repo/, above the source
	// root, so they have no path from it.
	for _, d := range res.Docs {
		if strings.HasSuffix(d.Src, "README.md") {
			t.Error("a page outside the site was built")
		}
	}
	if len(res.External) != 0 {
		t.Errorf("External = %v", res.External)
	}
	idx := docByBase(res.Docs, "index.md")
	for href, want := range map[string]string{
		"img/flow.png":         "img/flow.png",
		"guide.md":             "guide.html",
		"files/my%20doc.pdf":   "files/my%20doc.pdf",
		"../LICENSE":           "https://example.org/r/blob/main/LICENSE",
		"../README.md#install": "https://example.org/r/blob/main/README.md#install",
		"../site.go#L10":       "https://example.org/r/blob/main/site.go#L10",
		"../logo.png":          "_assets/logo.png",
	} {
		if got := idx.LinkMap[href]; got != want {
			t.Errorf("%s -> %q, want %q", href, got, want)
		}
	}
	for _, href := range []string{"../../far.txt", "../../farpic.png"} {
		if !idx.LinkDrops[href] {
			t.Errorf("%s not dropped", href)
		}
	}
	var outs []string
	for _, a := range res.Assets {
		rel, _ := filepath.Rel(filepath.Join(root, "out"), a.Out)
		outs = append(outs, filepath.ToSlash(rel))
	}
	sort.Strings(outs)
	if got := strings.Join(outs, ","); got != "_assets/logo.png,files/my doc.pdf,img/flow.png" {
		t.Errorf("assets %s", got)
	}
	var warns []string
	for _, w := range res.Warnings {
		warns = append(warns, w.Message)
	}
	all := strings.Join(warns, "\n")
	for _, want := range []string{"unlinked ../../far.txt", "dropped ../../farpic.png"} {
		if !strings.Contains(all, want) {
			t.Errorf("no warning %q in:\n%s", want, all)
		}
	}
}

func TestSiteCrawlUnlinksWithoutARepoURL(t *testing.T) {
	res, _, err := siteCrawl(t, "", siteLinkFiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	idx := docByBase(res.Docs, "index.md")
	for _, href := range []string{"../LICENSE", "../README.md#install", "../site.go#L10"} {
		if !idx.LinkDrops[href] {
			t.Errorf("%s not unlinked", href)
		}
	}
	if idx.LinkMap["../logo.png"] != "_assets/logo.png" {
		t.Error("an image from outside the site was not copied")
	}
}

func TestSiteCrawlStopsUnderUnmountedError(t *testing.T) {
	_, _, err := siteCrawl(t, "unmounted: error\n", siteLinkFiles, nil)
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"index.md", "../LICENSE", "../README.md#install", "../../farpic.png"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
}

// --no-assets copies nothing and leaves what would have been copied as
// written, while a link out of the site still follows unmounted:.
func TestSiteCrawlNoAssets(t *testing.T) {
	res, _, err := siteCrawl(t, "repo-url: https://example.org/r/blob/main/{path}\n", siteLinkFiles,
		func(o *CrawlOptions) { o.NoAssets = true })
	if err != nil {
		t.Fatal(err)
	}
	idx := docByBase(res.Docs, "index.md")
	if len(res.Assets) != 0 {
		t.Errorf("assets %v", res.Assets)
	}
	for _, href := range []string{"img/flow.png", "files/my%20doc.pdf", "../logo.png"} {
		if _, ok := idx.LinkMap[href]; ok {
			t.Errorf("%s was rewritten", href)
		}
	}
	if idx.LinkMap["../LICENSE"] != "https://example.org/r/blob/main/LICENSE" {
		t.Error("a link out of the site did not follow unmounted:")
	}
}

func TestSiteCrawlRefusesAnAssetOverAPage(t *testing.T) {
	_, _, err := siteCrawl(t, "", map[string]string{
		"docs/a.md":   "# A\n\n[old](a.html)\n",
		"docs/a.html": "<p>old</p>",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "output path collision") {
		t.Errorf("err %v", err)
	}
}

// An excluded file keeps its link as written, as --exclude promises.
func TestSiteCrawlLeavesAnExcludedFileAlone(t *testing.T) {
	res, _, err := siteCrawl(t, "", map[string]string{
		"docs/a.md":         "# A\n\n![x](vendor/x.png)\n",
		"docs/vendor/x.png": "png",
	}, func(o *CrawlOptions) { o.Exclude = []string{"vendor"} })
	if err != nil {
		t.Fatal(err)
	}
	a := docByBase(res.Docs, "a.md")
	if _, ok := a.LinkMap["vendor/x.png"]; ok || a.LinkDrops["vendor/x.png"] || len(res.Assets) != 0 {
		t.Errorf("map %v drops %v assets %v", a.LinkMap, a.LinkDrops, res.Assets)
	}
}
```

- [ ] **Step 2: Run them and see them fail.**

Run: `go test -run 'SiteCrawl' .`
Expected: FAIL: `Doc.LinkDrops` and `CrawlResult.Assets` are undefined.

- [ ] **Step 3: Implement the decision.** Append to `sitelinks.go`:

```go
// Asset is a file a site build copies into its output: Src, absolute, to
// Out, absolute, under the output directory.
type Asset struct {
	Src string
	Out string
}

// linkAction is what a site build does with one link. A zero action leaves
// the link as written.
type linkAction struct {
	rewrite string // the href to write instead
	drop    bool   // remove the link, keeping its text or alt
	copy    *Asset // a file to copy for it
	warn    string // said once, about the page holding the link
	fail    string // under unmounted: error, why the build stops
}

// action decides what a site build does with l, a link in the page or
// shell written to fromOut. outBySrc maps each page built to its output.
//
// A page in the site is linked where it is written. Any other file in the
// site, under the base, is copied to the same place under the output
// directory, whether the page loads it or a reader follows it. A file the
// page loads from outside the site is copied to _assets/, at its path from
// the source root, or dropped when it has none. A link a reader follows
// out of the site, to a page or any other file, goes where unmounted: says.
// A missing file, and one under an excluded path, keep the link as written.
func (r *siteRules) action(l Link, fromOut string, outBySrc map[string]string) linkAction {
	if l.Kind != LinkDoc && l.Kind != LinkAsset {
		return linkAction{}
	}
	target, err := resolve(l.Abs)
	if err != nil {
		return linkAction{} // missing: the crawl warned
	}
	if r.excluded.match(target) {
		return linkAction{}
	}
	if l.Kind == LinkDoc {
		if out, ok := outBySrc[target]; ok {
			return linkAction{rewrite: relHref(fromOut, out) + fragmentOf(l.Href)}
		}
		return r.leaving(l, target)
	}
	if isUnder(target, r.base) {
		if r.noAssets {
			return linkAction{}
		}
		rel, _ := filepath.Rel(r.base, target)
		out := filepath.Join(r.outDir, rel)
		return linkAction{rewrite: relHref(fromOut, out) + fragmentOf(l.Href), copy: &Asset{Src: target, Out: out}}
	}
	if !l.Loaded {
		return r.leaving(l, target)
	}
	if r.noAssets {
		return linkAction{}
	}
	rel, ok := relUnder(r.sourceRoot, target)
	if !ok {
		why := fmt.Sprintf("dropped %s, which is outside the source root %s; an image keeps its alt text", l.Href, r.sourceRoot)
		if r.unmounted == "error" {
			return linkAction{fail: why}
		}
		return linkAction{drop: true, warn: why}
	}
	out := filepath.Join(r.outDir, "_assets", rel)
	return linkAction{rewrite: relHref(fromOut, out), copy: &Asset{Src: target, Out: out}}
}

// leaving is what a link a reader follows out of the site becomes.
func (r *siteRules) leaving(l Link, target string) linkAction {
	switch r.unmounted {
	case "error":
		return linkAction{fail: fmt.Sprintf("links to %s, which is outside the site", l.Href)}
	case "link":
		if rel, ok := relUnder(r.sourceRoot, target); ok {
			return linkAction{rewrite: strings.ReplaceAll(r.repoURL, "{path}", encodePath(rel)) + fragmentOf(l.Href)}
		}
		return linkAction{drop: true, warn: fmt.Sprintf("unlinked %s: it is outside the source root %s, so repo-url: has no path for it; its text is kept", l.Href, r.sourceRoot)}
	}
	return linkAction{drop: true, warn: fmt.Sprintf("unlinked %s, which is outside the site; its text is kept", l.Href)}
}

// relUnder returns target's slash-separated path from root, and whether it
// is under root at all.
func relUnder(root, target string) (string, bool) {
	if !isUnder(target, root) {
		return "", false
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// relHref is the encoded relative href from the page written at fromOut to
// the file written at out.
func relHref(fromOut, out string) string {
	rel, err := filepath.Rel(filepath.Dir(fromOut), out)
	if err != nil {
		return filepath.ToSlash(out)
	}
	return encodePath(filepath.ToSlash(rel))
}
```

- [ ] **Step 4: Wire it into the crawl.** In `crawl.go`:

Add to `Doc`, after `LinkMap`:

```go
	// LinkDrops names the hrefs, as written in this document, whose links a
	// site build removes: a link out of the site it has nowhere to send.
	// Nil outside a site build.
	LinkDrops map[string]bool
```

Add to `CrawlResult`, after `Warnings`:

```go
	// Assets are the files a site build copies into its output, each once,
	// sorted by Out. Empty outside a site build; see CopyAssets.
	Assets []Asset

	rules *siteRules // a site build's link rules, for BuildSite's shell
```

In `Crawl`, after `excluded, excludeWarnings := resolveExcludes(opt.Exclude, base)`:

```go
	rules, err := newSiteRules(opt, base, excluded)
	if err != nil {
		return nil, err
	}
```

In `admit`, replace the `if !isUnder(target, base) { … }` block with:

```go
		if !isUnder(target, base) {
			if rules != nil {
				// A site holds what is under its base: a link out of it
				// becomes what the shell says, at link-mapping time, and a
				// seed outside it is refused.
				if !viaLink {
					res.Warnings = append(res.Warnings, Warning{src,
						fmt.Sprintf("not building %s: it is outside the site's root %s", ref, base)})
				}
				return
			}
			if opt.OutDir == "" {
				res.Warnings = append(res.Warnings, Warning{src,
					fmt.Sprintf("refusing to follow %s outside %s (no -o given)", ref, base)})
				return
			}
			res.External = append(res.External, target)
		}
```

At the end of `Crawl`, replace `buildLinkMaps(res, opt, linksBySrc)` and `return res, nil` with:

```go
	res.rules = rules
	if err := buildLinkMaps(res, opt, linksBySrc); err != nil {
		return nil, err
	}
	return res, nil
```

Replace `buildLinkMaps` and `linkMapFor` with:

```go
// buildLinkMaps computes, for every document, the replacement for each
// href it contains, and in a site build what it drops and copies. A site's
// shell is planned too, once, so its copies are made and its warnings said
// once. linksBySrc supplies the links already extracted during discovery.
func buildLinkMaps(res *CrawlResult, opt CrawlOptions, linksBySrc map[string][]Link) error {
	outBySrc := make(map[string]string, len(res.Docs))
	for _, d := range res.Docs {
		outBySrc[d.Src] = d.Out
	}
	var fails []string
	copies := map[string]Asset{}
	take := func(src string, p linkPlan) {
		for _, c := range p.copies {
			if prev, ok := copies[c.Out]; ok && prev.Src != c.Src {
				fails = append(fails, fmt.Sprintf("output path collision: %s and %s both produce %s", prev.Src, c.Src, c.Out))
				continue
			}
			copies[c.Out] = c
		}
		seen := map[string]bool{}
		for _, w := range p.warns {
			if !seen[w] {
				seen[w] = true
				res.Warnings = append(res.Warnings, Warning{src, w})
			}
		}
		for _, f := range p.fails {
			fails = append(fails, src+": "+f+" (unmounted: error)")
		}
	}
	for i := range res.Docs {
		d := &res.Docs[i]
		p := planLinks(linksBySrc[d.Src], d.Out, outBySrc, opt, res.rules)
		d.LinkMap = p.rewrite
		if res.rules != nil {
			d.LinkDrops = p.drop
		}
		take(d.Src, p)
	}
	if res.rules != nil {
		shell, err := filepath.Abs(opt.SiteShell)
		if err != nil {
			return err
		}
		if raw, err := os.ReadFile(shell); err == nil {
			_, body, _ := splitFrontMatter(raw)
			if root, err := parseDoc(body); err == nil {
				// The shell's links are planned from a page at the output
				// root; only the copies, warnings and failures are kept, and
				// each page plans the shell's hrefs again from where it is.
				take(shell, planLinks(ExtractLinks(root, filepath.Dir(shell)),
					filepath.Join(opt.OutDir, "index.html"), outBySrc, opt, res.rules))
			}
		}
	}
	if len(fails) > 0 {
		sort.Strings(fails)
		return fmt.Errorf("the site cannot be built:\n  %s", strings.Join(fails, "\n  "))
	}
	for _, a := range copies {
		res.Assets = append(res.Assets, a)
	}
	sort.Slice(res.Assets, func(i, j int) bool { return res.Assets[i].Out < res.Assets[j].Out })
	for _, a := range res.Assets {
		for _, d := range res.Docs {
			if d.Out == a.Out {
				return fmt.Errorf("output path collision: %s and %s both produce %s", d.Src, a.Src, a.Out)
			}
		}
	}
	return nil
}

// linkPlan is what a document's links become: the hrefs rewritten, and in a
// site build the hrefs dropped, the files copied, and what is warned or
// stops the build.
type linkPlan struct {
	rewrite map[string]string
	drop    map[string]bool
	copies  []Asset
	warns   []string
	fails   []string
}

// planLinks plans links, as written in a document whose output is fromOut.
// Without site rules, document links resolve to other emitted documents
// and asset links point back out at the original file on disk; with them,
// siteRules.action decides. A site uses it to rewrite its shell's links
// once per page.
func planLinks(links []Link, fromOut string, outBySrc map[string]string, opt CrawlOptions, rules *siteRules) linkPlan {
	p := linkPlan{rewrite: map[string]string{}, drop: map[string]bool{}}
	if rules != nil {
		for _, l := range links {
			a := rules.action(l, fromOut, outBySrc)
			switch {
			case a.fail != "":
				p.fails = append(p.fails, a.fail)
			case a.drop:
				p.drop[l.Href] = true
			case a.rewrite != "":
				if opt.NoMdLinks && l.Kind == LinkDoc {
					continue
				}
				p.rewrite[l.Href] = a.rewrite
			}
			if a.copy != nil {
				p.copies = append(p.copies, *a.copy)
			}
			if a.warn != "" {
				p.warns = append(p.warns, a.warn)
			}
		}
		return p
	}
	outDir := filepath.Dir(fromOut)
	for _, l := range links {
		switch l.Kind {
		case LinkDoc:
			if opt.NoMdLinks {
				continue
			}
			target, err := resolve(l.Abs)
			if err != nil {
				continue
			}
			targetOut, ok := outBySrc[target]
			if !ok {
				continue // not emitted: leave the link as written
			}
			rel, err := filepath.Rel(outDir, targetOut)
			if err != nil {
				continue
			}
			p.rewrite[l.Href] = encodePath(filepath.ToSlash(rel)) + fragmentOf(l.Href)

		case LinkAsset:
			if opt.NoAssets || opt.OutDir == "" {
				// In place, the HTML sits beside its source and existing
				// relative links already resolve.
				continue
			}
			if _, err := os.Stat(l.Abs); err != nil {
				continue // missing asset: Crawl already warned; leave link as written
			}
			rel, err := filepath.Rel(outDir, l.Abs)
			if err != nil {
				continue
			}
			p.rewrite[l.Href] = encodePath(filepath.ToSlash(rel))
		}
	}
	return p
}
```

The non-site branch is the old `linkMapFor`, moved. The asset collision check must run after the doc-vs-doc check already in `Crawl`, which it does, since `buildLinkMaps` runs last.

`siteshell.go` calls `linkMapFor`; until Task 5, change that one call to `planLinks(links, out, s.outBySrc, s.crawl, nil).rewrite` so the package builds.

- [ ] **Step 5: Run the tests and see them pass.**

Run: `go test -run 'SiteCrawl' . && go test ./...`
Expected: PASS. Nothing else called `linkMapFor`.

- [ ] **Step 6: Commit.**

```bash
git add crawl.go sitelinks.go sitecrawl_test.go siteshell.go
git commit -F - <<'EOF'
feat: a site build copies what it shows and keeps links inside it

A site build followed links out of its base and wrote those pages
under _external/, at the building machine's own path, with no place
in the site. Its images and other files were linked back to their
sources, which a host serving the output directory alone cannot
reach, and a link to a file outside the site led nowhere once
published.

With a shell and an output directory, the crawl now builds only what
is under the base. A file in the site that a page loads or links is
copied to the same place in the output, an image from outside it to
_assets/ at its path from the source root, and a link out of the
site goes to the repository's host, is unlinked with a warning, or
stops the build, as the shell's unmounted: says. A file outside the
source root has no path, so its link or image is dropped with a
warning. A copy that would land on a page's output stops the build.
EOF
```

---

### Task 5: The shell's links follow the site's rules

**Files:**
- Modify: `site.go` (`Site.rules`, set in `BuildSite`)
- Modify: `siteshell.go` (`wrap`, `rebaseShellLinks`)
- Test: `siteshell_test.go`

**Interfaces:**
- Consumes: `CrawlResult.rules`, `planLinks`, `LinkDrop` (Tasks 2 and 4).
- Produces: `sitePage` (test helper) passes `LinkDrops`.

- [ ] **Step 1: Write the failing test** in `siteshell_test.go`. First make `sitePage` pass the drops: in its `Convert` call add `LinkDrops: doc.LinkDrops,`. Then:

```go
// The shell's links follow the site's rules, from each page it wraps: its
// image is copied once and linked from where the page is, its link out of
// the site goes to the repository's host, and one it has nowhere to send
// is dropped, warned about once under the shell's path.
func TestShellLinksFollowTheSiteRules(t *testing.T) {
	root := writeTree(t, map[string]string{
		"repo/site/shell.md": "---\nsite: true\nrepo-url: https://example.org/r/blob/main/{path}\n---\n" +
			"![logo](logo.svg) [licence](../LICENSE) [far](../../far.txt)\n\n[[content]]\n",
		"repo/site/logo.svg":       "<svg/>",
		"repo/LICENSE":             "MIT",
		"repo/docs/index.md":       "# Home\n",
		"repo/docs/guide/intro.md": "# Intro\n",
		"far.txt":                  "far",
	})
	opt := CrawlOptions{Entries: []string{filepath.Join(root, "repo/docs")}, OutDir: filepath.Join(root, "out"),
		Depth: -1, LinkDepth: -1, SiteShell: filepath.Join(root, "repo/site/shell.md")}
	res := mustCrawl(t, opt)
	s, err := BuildSite(res, opt.SiteShell, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Assets) != 1 || res.Assets[0].Out != filepath.Join(root, "out/_assets/site/logo.svg") {
		t.Fatalf("assets %v", res.Assets)
	}
	page, _ := sitePage(t, s, res, root, "repo/docs/guide/intro.md")
	for _, want := range []string{`src="../_assets/site/logo.svg"`, `href="https://example.org/r/blob/main/LICENSE"`, "far"} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %s in:\n%s", want, page)
		}
	}
	if strings.Contains(page, "far.txt") {
		t.Errorf("the dropped link is still there:\n%s", page)
	}
	n := 0
	for _, w := range append(res.Warnings, s.Warnings...) {
		if strings.Contains(w.Message, "far.txt") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the dropped link was warned about %d times", n)
	}
}
```

The shell sits in `repo/site/`, outside the base `repo/docs/`, so its logo is outside the site and goes to `_assets/site/logo.svg`, its path from the source root, `repo/`. `far.txt` sits beside `repo/`, above the source root.

- [ ] **Step 2: Run it and see it fail.**

Run: `go test -run 'TestShellLinksFollowTheSiteRules' .`
Expected: FAIL: the logo's `src` points back at `site/logo.svg` and the licence link at the file.

- [ ] **Step 3: Implement.** In `site.go`, add to `Site`, after `crawl CrawlOptions`:

```go
	rules     *siteRules // the crawl's link rules, for the shell's links
```

and in `BuildSite`, after `base: res.Base, crawl: opt}`'s statement, add `s.rules = res.rules`.

In `siteshell.go`, in `wrap`, replace:

```go
	links := ExtractLinks(shell, s.shellDir)
	mapped := linkMapFor(links, out, s.outBySrc, s.crawl)
	if err := LinkRewrite(mapped).Fn(shell); err != nil {
		return nil, err
	}
	s.rebaseShellLinks(links, mapped, out)
```

(the `planLinks(..., nil).rewrite` stand-in from Task 4) with:

```go
	links := ExtractLinks(shell, s.shellDir)
	// The crawl planned the shell once, for its copies and warnings; each
	// page plans it again from where the page is written.
	plan := planLinks(links, out, s.outBySrc, s.crawl, s.rules)
	if err := LinkDrop(plan.drop).Fn(shell); err != nil {
		return nil, err
	}
	if err := LinkRewrite(plan.rewrite).Fn(shell); err != nil {
		return nil, err
	}
	s.rebaseShellLinks(links, plan.rewrite, plan.drop, out)
```

and change `rebaseShellLinks`'s signature and first check:

```go
func (s *Site) rebaseShellLinks(links []Link, mapped map[string]string, dropped map[string]bool, out string) {
	for _, l := range links {
		if l.Kind != LinkDoc && l.Kind != LinkAsset {
			continue
		}
		if _, ok := mapped[l.Href]; ok || dropped[l.Href] || hasScheme(l.Href) {
			continue
		}
```

(the rest of the loop is unchanged). A shell link the rules left as written, a missing or excluded file, is still rebased, as before.

- [ ] **Step 4: Run the tests and see them pass.**

Run: `go test ./...`
Expected: PASS, every existing site and shell test included.

- [ ] **Step 5: Commit.**

```bash
git add site.go siteshell.go siteshell_test.go
git commit -F - <<'EOF'
feat: a site shell's links follow the site's rules on every page

The shell's links were rewritten for each page by the old rule, so its
logo pointed back at the file beside the shell and a link from it out
of the site led nowhere once published, though the pages' own links
now follow the site's rules.

Each page now plans the shell's links with the same rules from where
the page is written: the shell's files are copied once, by the crawl,
and linked from every page, a link out of the site goes where
unmounted: says, and one with nowhere to go is dropped, warned about
once under the shell's path.
EOF
```

---

### Task 6: `CopyAssets` and its ledger

**Files:**
- Create: `assetcopy.go`, `assetcopy_test.go`
- (`Site.AssetLedgerPath` goes in `assetcopy.go`, beside the ledger it names.)

**Interfaces:**
- Consumes: `Asset` (Task 4), `Marker`, `SafeWrite`, `WriteResult`.
- Produces:

```go
const AssetLedger = "md2html-assets.txt"
type AssetResult struct {
	Asset
	Result WriteResult
	Err    error
}
func CopyAssets(assets []Asset, ledger string) ([]AssetResult, error)
func (s *Site) AssetLedgerPath() string
```

- [ ] **Step 1: Write the failing tests** in `assetcopy_test.go`:

```go
package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A copy is made, made again unchanged, replaced when its source changes,
// and refused when someone else's file, or an edited copy, is in its place.
func TestCopyAssetsKeepsToItsOwnCopies(t *testing.T) {
	root := writeTree(t, map[string]string{"src/a.png": "one", "src/b.png": "two", "out/c.png": "theirs"})
	ledger := filepath.Join(root, "out", AssetLedger)
	assets := []Asset{
		{Src: filepath.Join(root, "src/a.png"), Out: filepath.Join(root, "out/img/a.png")},
		{Src: filepath.Join(root, "src/b.png"), Out: filepath.Join(root, "out/c.png")},
	}
	results := func() map[string]WriteResult {
		rs, err := CopyAssets(assets, ledger)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]WriteResult{}
		for _, r := range rs {
			if r.Err != nil {
				t.Fatal(r.Err)
			}
			m[filepath.Base(r.Out)] = r.Result
		}
		return m
	}
	if got := results(); got["a.png"] != WriteCreated || got["c.png"] != WriteRefused {
		t.Errorf("first build: %v", got)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "out/c.png")); string(b) != "theirs" {
		t.Error("someone else's file was overwritten")
	}
	os.WriteFile(filepath.Join(root, "src/a.png"), []byte("one, changed"), 0o644)
	if got := results(); got["a.png"] != WriteOverwritten {
		t.Errorf("changed source: %v", got)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "out/img/a.png")); string(b) != "one, changed" {
		t.Errorf("copy holds %q", b)
	}
	os.WriteFile(filepath.Join(root, "out/img/a.png"), []byte("edited by hand"), 0o644)
	if got := results(); got["a.png"] != WriteRefused {
		t.Errorf("edited copy: %v", got)
	}
	b, err := os.ReadFile(ledger)
	if err != nil || !strings.HasPrefix(string(b), MarkerPrefix) || !strings.Contains(string(b), "img/a.png") {
		t.Errorf("ledger %v:\n%s", err, b)
	}
}

// A copy that already holds its source's bytes is left as it is, even with
// no ledger: a fresh clone of a published site rebuilds cleanly.
func TestCopyAssetsLeavesAnIdenticalFile(t *testing.T) {
	root := writeTree(t, map[string]string{"src/a.png": "same", "out/a.png": "same"})
	rs, err := CopyAssets([]Asset{{Src: filepath.Join(root, "src/a.png"), Out: filepath.Join(root, "out/a.png")}},
		filepath.Join(root, "out", AssetLedger))
	if err != nil || rs[0].Result == WriteRefused || rs[0].Err != nil {
		t.Errorf("%+v, %v", rs, err)
	}
}
```

- [ ] **Step 2: Run them and see them fail.**

Run: `go test -run 'CopyAssets' .`
Expected: FAIL: `CopyAssets` and `AssetLedger` are undefined.

- [ ] **Step 3: Implement** `assetcopy.go`:

```go
package md2html

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AssetLedger is the file, at a site's output root, that lists each asset
// md2html copied there with the SHA-256 of what it wrote. An image cannot
// carry the provenance marker a page does, so the ledger is how a later
// build tells its own copy from a file someone else put in its place.
const AssetLedger = "md2html-assets.txt"

// AssetResult is what CopyAssets did with one asset.
type AssetResult struct {
	Asset
	Result WriteResult
	Err    error
}

// CopyAssets copies each asset to its output path and records the copies in
// the ledger at ledger. A file already at an output path is replaced when
// the ledger says md2html wrote what it holds, and left alone when it holds
// its source's bytes already; any other file is refused, as SafeWrite
// refuses a page md2html did not write. The ledger is rewritten to list
// what this build copied, and is itself written with SafeWrite.
func CopyAssets(assets []Asset, ledger string) ([]AssetResult, error) {
	root := filepath.Dir(ledger)
	before := readLedger(ledger)
	after := map[string]string{}
	results := make([]AssetResult, len(assets))
	for i, a := range assets {
		results[i] = AssetResult{Asset: a}
		data, err := os.ReadFile(a.Src)
		if err != nil {
			results[i].Result, results[i].Err = WriteRefused, err
			continue
		}
		sum := hashOf(data)
		rel, err := filepath.Rel(root, a.Out)
		if err != nil {
			results[i].Result, results[i].Err = WriteRefused, err
			continue
		}
		key := filepath.ToSlash(rel)
		old, err := os.ReadFile(a.Out)
		switch {
		case os.IsNotExist(err):
			results[i].Result = WriteCreated
		case err != nil:
			results[i].Result, results[i].Err = WriteRefused, err
			continue
		case bytes.Equal(old, data):
			results[i].Result = WriteOverwritten
			after[key] = sum
			continue
		case before[key] == hashOf(old):
			results[i].Result = WriteOverwritten
		default:
			// Someone else's file, or an edited copy: left alone, and an
			// edited copy stays in the ledger as what md2html last wrote.
			results[i].Result = WriteRefused
			if s, ok := before[key]; ok {
				after[key] = s
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(a.Out), 0o755); err != nil {
			results[i].Result, results[i].Err = WriteRefused, err
			continue
		}
		if err := os.WriteFile(a.Out, data, 0o644); err != nil {
			results[i].Result, results[i].Err = WriteRefused, err
			continue
		}
		after[key] = sum
	}
	if len(after) == 0 && len(before) == 0 {
		return results, nil
	}
	if wr, err := SafeWrite(ledger, ledgerText(after)); err != nil || wr == WriteRefused {
		if err == nil {
			err = fmt.Errorf("refusing to overwrite %s (not generated by md2html)", ledger)
		}
		return results, err
	}
	return results, nil
}

func hashOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// readLedger returns the ledger's paths and hashes, or none when it is
// missing or not md2html's.
func readLedger(path string) map[string]string {
	m := map[string]string{}
	if ours, err := IsOurs(path); err != nil || !ours {
		return m
	}
	f, err := os.Open(path)
	if err != nil {
		return m
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		sum, rel, ok := strings.Cut(sc.Text(), "  ")
		if ok && len(sum) == 64 {
			m[rel] = sum
		}
	}
	return m
}

// ledgerText is the ledger: the marker, then a line per copy, its hash and
// its path from the output root, as sha256sum writes them, sorted.
func ledgerText(m map[string]string) []byte {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(Marker() + "\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s  %s\n", m[k], k)
	}
	return []byte(b.String())
}
```

Append to `assetcopy.go`:

```go
// AssetLedgerPath is where a site build records the assets it copies; see
// CopyAssets.
func (s *Site) AssetLedgerPath() string { return filepath.Join(s.outRoot, AssetLedger) }
```

- [ ] **Step 4: Run the tests and see them pass.**

Run: `go test -run 'CopyAssets' . && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add assetcopy.go assetcopy_test.go
git commit -F - <<'EOF'
feat: CopyAssets copies a site's files and keeps a ledger of them

md2html never overwrites a file it did not write, and tells its own
pages by the provenance marker at their top. A copied image cannot
carry that marker, so a second build would have refused to replace
every copy the first one made.

CopyAssets copies a site's files and records each with its SHA-256 in
md2html-assets.txt at the output root. A later build replaces a copy
whose bytes the ledger vouches for, leaves one that already holds its
source's bytes, and refuses anything else, an edited copy included.
EOF
```

---

### Task 7: The command line

**Files:**
- Modify: `cmd/md2html/main.go` (`--source-root`, `crawlOpt`, the copy step, `--no-assets` help, `--help` text)
- Test: `cmd/md2html/main_test.go`

**Interfaces:**
- Consumes: `CrawlOptions.SiteShell`, `CrawlOptions.SourceRoot`, `CrawlResult.Assets`, `CopyAssets`, `Site.AssetLedgerPath`, `Doc.LinkDrops`, `Options.LinkDrops`.

- [ ] **Step 1: Write the failing tests** in `cmd/md2html/main_test.go`, after `TestRunSiteWritesPagesNotTheShell`:

```go
// A site build copies its files into the output and sends a link out of the
// site to the repository's host. Built again, it replaces its own copies.
func TestRunSiteCopiesAssetsAndLinksOut(t *testing.T) {
	root := tree(t, map[string]string{
		"site/shell.md":     "---\nsite: true\nrepo-url: https://example.org/r/blob/main/{path}\n---\n[[content]]\n",
		"docs/index.md":     "# Home\n\n![flow](img/flow.png) [licence](../LICENSE)\n",
		"docs/img/flow.png": "png",
		"LICENSE":           "MIT",
	})
	args := []string{"--site", filepath.Join(root, "site/shell.md"), filepath.Join(root, "docs"), "-o", filepath.Join(root, "out")}
	for i := 0; i < 2; i++ {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 0 {
			t.Fatalf("build %d: exit %d, stderr: %s", i+1, code, errb.String())
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, "out/img/flow.png")); err != nil || string(b) != "png" {
		t.Errorf("image not copied: %v", err)
	}
	page, _ := os.ReadFile(filepath.Join(root, "out/index.html"))
	for _, want := range []string{`src="img/flow.png"`, `href="https://example.org/r/blob/main/LICENSE"`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("missing %s", want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "out", md2html.AssetLedger)); err != nil {
		t.Errorf("no ledger: %v", err)
	}
}

func TestRunSiteRefusesSomeoneElsesFile(t *testing.T) {
	root := tree(t, map[string]string{
		"site/shell.md":     "---\nsite: true\n---\n[[content]]\n",
		"docs/index.md":     "# Home\n\n![flow](img/flow.png)\n",
		"docs/img/flow.png": "png",
		"out/img/flow.png":  "theirs",
	})
	var out, errb bytes.Buffer
	code := run([]string{"--site", filepath.Join(root, "site/shell.md"), filepath.Join(root, "docs"), "-o", filepath.Join(root, "out")}, &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "refusing to overwrite "+filepath.Join(root, "out/img/flow.png")) {
		t.Errorf("exit %d, stderr: %s", code, errb.String())
	}
}

func TestRunSourceRootNeedsSite(t *testing.T) {
	root := tree(t, map[string]string{"docs/index.md": "# Home\n"})
	var out, errb bytes.Buffer
	if code := run([]string{"--source-root", root, filepath.Join(root, "docs"), "-o", filepath.Join(root, "out")}, &out, &errb); code != 2 ||
		!strings.Contains(errb.String(), "--source-root needs --site") {
		t.Errorf("exit %d, stderr: %s", code, errb.String())
	}
}
```

- [ ] **Step 2: Run them and see them fail.**

Run: `go test -run 'TestRunSiteCopies|TestRunSiteRefuses|TestRunSourceRoot' ./cmd/md2html`
Expected: FAIL: the image is not copied, and `--source-root` is not a flag.

- [ ] **Step 3: Implement.** In `cmd/md2html/main.go`:

Add the flag after `sitePath`:

```go
		sourceRoot  = fs.String("source-root", "", "with --site, the directory a link out of the site and a copied file's path are measured from (default: the deepest one holding the shell and the pages)")
```

Change the `noAssets` flag's help to `"do not rewrite asset links, nor copy files into a site"`.

After the `--site` / `--toc float` check, add:

```go
	if *sourceRoot != "" && *sitePath == "" {
		fmt.Fprint(stderr, "md2html: --source-root needs --site; only a site measures paths from a source root\n")
		return 2
	}
```

Add to `crawlOpt`:

```go
		SiteShell:  *sitePath,
		SourceRoot: *sourceRoot,
```

In `buildOptions`, add `LinkDrops: d.LinkDrops,` after `LinkMap: d.LinkMap,`. Run `gofmt -w cmd/md2html/main.go` afterwards: it realigns the `crawlOpt` literal around the two longer field names.

After the search-index write (the `if site != nil { wr, err := md2html.SafeWrite(site.IndexPath(), …` block), add:

```go
	if site != nil && len(res.Assets) > 0 {
		copied, err := md2html.CopyAssets(res.Assets, site.AssetLedgerPath())
		for _, c := range copied {
			switch {
			case c.Err != nil:
				fmt.Fprintf(stderr, "md2html: %s: %v\n", c.Out, c.Err)
				failed++
			case c.Result == md2html.WriteRefused:
				fmt.Fprintf(stderr, "md2html: refusing to overwrite %s (not copied by md2html)\n", c.Out)
				refused++
			}
		}
		if err != nil {
			fmt.Fprintf(stderr, "md2html: %v\n", err)
			failed++
		}
	}
```

In the `--help` text, extend the paragraph that begins `Building a site:` to:

```text
Building a site: --site shell.md wraps every page in a shell page whose
markers draw navigation and search; md2html --guide sites explains it.
A site copies the files its pages show into -o, and the shell's
repo-url: and unmounted: say where a link out of the site goes.
```

- [ ] **Step 4: Run the tests and see them pass.**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add cmd/md2html/main.go cmd/md2html/main_test.go
git commit -F - <<'EOF'
feat: md2html --site copies a site's files and takes --source-root

The command built a site's pages but had no way to copy the files they
show or to say where the source root is, so a site's images still
pointed back at the source tree.

--site now passes the shell to the crawl, copies the files it lists
into -o after the pages, keeping the ledger, and reports a copy it
refuses as it does a page. --source-root sets the source root for the
build, over the shell's source-root:, and needs --site.
EOF
```

---

### Task 8: Docs, the changelog, and md2html's own site

**Files:**
- Modify: `README.md` (`### Assets are linked, never copied`, the flags table)
- Modify: `docs/guide/sites.md`, `docs/guide/build-behaviour.md`, `docs/guide/quick-reference.md`
- Modify: `CHANGELOG.md`
- Modify: `site/shell.md`

- [ ] **Step 1: README.** Rename `### Assets are linked, never copied` to `### Assets are linked, except in a site`, and add after its last paragraph:

```markdown
A site build is the exception. A site is published as its output
directory alone, so `--site` copies the files its pages show into `-o`,
beside the pages that use them, and records them in
`md2html-assets.txt` so a later build replaces its own copies and
refuses anything else. A link from the site to a file outside it goes
where the shell's `repo-url:` and `unmounted:` say;
`md2html --guide sites` explains them.
```

In the flags table, change the `--no-assets` row to `| \`--no-assets\` | do not rewrite asset links, nor copy files into a site |` and add a row after `--site`'s: `| \`--source-root DIR\` | with \`--site\`, where a link out of the site and a copied file's path are measured from |`.

- [ ] **Step 2: The guide.** In `docs/guide/sites.md`, add a section after `## The tree`:

````markdown
## Links and files {#links-and-files}

A site is published as its output directory alone, so it holds
everything it shows. A file is in the site when it is under the run's
base.

- A page in the site is linked where it is written. A `.md` outside the
  site is never built: nothing is written under `_external/`.
- Any other file in the site that a page shows or links, an image, a
  stylesheet, a PDF, is copied to the same place in the output.
  `md2html-assets.txt` at the output root records the copies, so a later
  build replaces its own and refuses a file someone else put there.
- An image or other file a page loads from outside the site is copied to
  `_assets/`, at its path from the source root.
- A link a reader follows out of the site, to a page or any file, goes
  where the shell's `unmounted:` says:

  | `unmounted:` | The link |
  |---|---|
  | `link` | Goes to the file's page on the repository's host, from `repo-url:`. The default when `repo-url:` is set. |
  | `unlink` | Is removed, with a warning; its text stays. The default without `repo-url:`. |
  | `error` | Stops the build, naming the page and the link. |

```markdown
---
site: true
repo-url: https://github.com/acme/repo/blob/main/{path}
---
```

`{path}` is the file's path from the **source root**: the deepest
directory holding the shell and the pages, which for a repository's
docs is the repository's root. `source-root:` in the shell, relative to
it, or `--source-root`, sets it instead; a default that would be the
filesystem's root is refused. A file outside the source root has no
path, so a link to it is unlinked and an image from it dropped, with a
warning.

`--no-assets` copies nothing and leaves those links as written; a link
out of the site still follows `unmounted:`. A file under an
`--exclude`d path keeps its link as written. A site built in place,
without `-o`, keeps its files where they are.
````

In `docs/guide/build-behaviour.md`, change the bullet beginning `` - `.md` links rewritten to `.html`, and asset links pointed back at the `` so that it ends with: `…pointed back at the original file, except in a site, which copies them (see [Links and files](sites.md#links-and-files)).` Read the bullet first and keep its wording up to that point.

In `docs/guide/quick-reference.md`, add a row after `| A site shell around every page | … |`:

```markdown
| Where a site's links out of it go | `repo-url: https://…/blob/main/{path}` in the shell |
```

- [ ] **Step 3: The changelog.** Under `## Unreleased` (creating it above the newest release if absent), add to `### Added`:

```markdown
- A site's shell says where a link out of the site goes: `repo-url:`, a
  URL with `{path}`, sends it to the file's page on the repository's
  host, and `unmounted:` (`link`, `unlink` or `error`) chooses. Paths
  are measured from the source root, the deepest directory holding the
  shell and the pages, or `source-root:` and `--source-root`.
- `Options.LinkDrops` and the `LinkDrop` transform remove a link and keep
  its text or an image's alt.
```

and under `### Changed`:

```markdown
- **breaking:** a site build (`--site` with `-o`) copies the files its
  pages show into the output, recorded in `md2html-assets.txt`, instead
  of linking back to the sources, and builds nothing outside its base:
  a page it used to write under `_external/` is no longer written, and a
  link to it goes where `unmounted:` says. Without `repo-url:`, a link
  out of the site is now unlinked, with a warning.
- An image's `srcset` candidates and a video's `poster` are found and
  rewritten like any other link, in every build.
```

- [ ] **Step 4: md2html's own site.** In `site/shell.md`'s front matter, add:

```markdown
repo-url: https://github.com/AdamF-G/md2html/blob/main/{path}
```

- [ ] **Step 5: Check everything.**

Run: `gofmt -l . && go vet ./... && go test ./... && just e2e && just site`
Expected: all pass; `just site` reports `0 warning(s)`. Then:

The site's base is the repository's root, so `LICENSE` and `THIRD_PARTY_NOTICES`, which the README links, are in the site and are copied, not sent to GitHub.

Run: `grep -o 'href="[^"]*\(LICENSE\|THIRD_PARTY_NOTICES\)"' _site/README.html; ls _site/LICENSE _site/THIRD_PARTY_NOTICES; ls _site/_external 2>/dev/null; cat _site/md2html-assets.txt`
Expected: `href="LICENSE"` and `href="THIRD_PARTY_NOTICES"`; both files present; no `_external`; the ledger lists both with their hashes.

- [ ] **Step 6: Commit.**

```bash
git add README.md docs/guide/sites.md docs/guide/build-behaviour.md docs/guide/quick-reference.md CHANGELOG.md site/shell.md
git commit -F - <<'EOF'
docs: a site's links and files, and md2html's site links out to GitHub

The README said assets are never copied, and the sites guide said
nothing of where a link out of a site goes, though a site build now
copies its files and sends such a link where its shell says.
md2html's own site linked its LICENSE and notices back at files the
published site did not hold; it now copies them.

The README, the sites and build-behaviour guides and the quick
reference describe the copying, repo-url:, unmounted:, the source
root and --no-assets in a site, the changelog marks the change as
breaking for site builds, and md2html's shell sends links out of the
site to GitHub.
EOF
```
