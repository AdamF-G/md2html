package md2html

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// CrawlOptions configures a discovery pass.
type CrawlOptions struct {
	// Entries are the starting points: files, directories, or both.
	Entries []string
	// OutDir is the output root. Empty means in-place.
	OutDir string
	// Depth bounds how many directory levels a directory entry seeds.
	// 0 seeds only files directly inside it; -1 is unlimited. It never
	// affects link traversal, which is always unlimited.
	Depth int
	// NoMdLinks leaves document links unrewritten.
	NoMdLinks bool
	// NoAssets leaves asset links unrewritten.
	NoAssets bool
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
}

// Doc is one document in the emit set.
type Doc struct {
	// Src is the resolved absolute source path.
	Src string
	// Out is the absolute destination path for the generated HTML.
	Out string
	// LinkMap maps an href exactly as written in this document to its
	// replacement. Populated by buildLinkMaps.
	LinkMap map[string]string
}

// Warning is a non-fatal problem found during a run.
type Warning struct {
	// Src is the document the warning was raised while processing.
	Src string
	// Message describes the problem.
	Message string
}

// CrawlResult is the outcome of a discovery pass.
type CrawlResult struct {
	// Docs is the emit set: every document reachable from the entry points.
	Docs []Doc
	// Base is the deepest directory containing every entry point.
	Base string
	// External lists the resolved absolute paths of documents pulled in
	// from outside Base.
	External []string
	// Warnings are the non-fatal problems found during the crawl.
	Warnings []Warning
}

// parseDoc converts Markdown straight to a parsed tree, skipping transforms
// and the page shell. Discovery only needs the tree, and going through
// Convert would inline the whole stylesheet on every discovery pass.
func parseDoc(src []byte) (*html.Node, error) {
	var buf bytes.Buffer
	if err := newParser().Convert(src, &buf); err != nil {
		return nil, err
	}
	return parseFragment(buf.Bytes())
}

// resolve returns the symlink-resolved absolute path, so cycles through
// symlinks terminate.
func resolve(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs, nil // not yet existing; caller reports
	}
	return real, nil
}

// seed returns the Markdown files a single entry point contributes, plus
// how many candidate paths exclusion caused it to drop. Paths at or
// beneath an excluded prefix contribute nothing, and an excluded directory
// is never descended into — admit would reject every file inside one
// anyway, but walking a vendored or archived subtree only to throw the
// whole result away is work nobody asked for.
//
// skipped counts each excluded Markdown file and each excluded directory
// declined at the walk (once per directory, not per file inside it, since
// SkipDir is precisely what avoids looking inside). Crawl needs this to
// tell "nothing here was ever Markdown" apart from "everything here was
// excluded" once pruning means the latter no longer shows up as files
// admit had to refuse.
func seed(entry string, depth int, excluded []string) (files []string, skipped int, err error) {
	info, err := os.Stat(entry)
	if err != nil {
		return nil, 0, fmt.Errorf("entry point %s: %w", entry, err)
	}
	if !info.IsDir() {
		if !IsMarkdownPath(entry) {
			return nil, 0, fmt.Errorf("entry point %s is not a Markdown file", entry)
		}
		p, err := resolve(entry)
		if err != nil {
			return nil, 0, err
		}
		if isExcluded(p, excluded) {
			return nil, 1, nil
		}
		return []string{p}, 0, nil
	}

	rootAbs, err := resolve(entry)
	if err != nil {
		return nil, 0, err
	}
	var out []string
	skippedCount := 0
	err = filepath.WalkDir(rootAbs, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip, do not abort
		}
		if d.IsDir() {
			if isExcluded(p, excluded) {
				skippedCount++
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
				skippedCount++
				return nil
			}
			out = append(out, rp)
		}
		return nil
	})
	return out, skippedCount, err
}

// pathDepth counts separators in a cleaned relative path: "a" is 1,
// "a/b" is 2.
func pathDepth(rel string) int {
	rel = filepath.Clean(rel)
	if rel == "." {
		return 0
	}
	n := 1
	for _, r := range rel {
		if r == filepath.Separator {
			n++
		}
	}
	return n
}

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

// Crawl discovers every document reachable from the entry points.
func Crawl(opt CrawlOptions) (*CrawlResult, error) {
	if len(opt.Entries) == 0 {
		return nil, fmt.Errorf("no entry points given")
	}

	var entryAbs []string
	for _, e := range opt.Entries {
		p, err := resolve(e)
		if err != nil {
			return nil, err
		}
		entryAbs = append(entryAbs, p)
	}
	base := commonAncestor(entryAbs)
	excluded := resolveExcludes(opt.Exclude, base)

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

	// admit is the single gate onto the queue. Both the seeding pass and
	// the link-following pass go through it, so the containment policy has
	// exactly one implementation and the two paths cannot drift apart: a
	// symlinked seed pointing outside the tree is refused for the same
	// reason, and recorded in External for the same reason, as a link is.
	// src is the document or entry point the target was reached from and
	// ref is how it was written there, both for diagnostics only. viaLink
	// is what decides whether a refusal is reported: a refused seed is the
	// caller getting exactly what they asked for, while a refused link
	// changes how an existing document renders and has to be surfaced.
	admit := func(src, target, ref string, viaLink bool, hops int) {
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
			queue = append(queue, queued{target, hops})
		}
	}

	found := 0
	skipped := 0
	for i, e := range opt.Entries {
		s, n, err := seed(e, opt.Depth, excluded)
		if err != nil {
			return nil, err
		}
		found += len(s)
		skipped += n
		for _, p := range s {
			// A seed can point outside the tree even though it was found
			// inside it: a symlinked .md resolves wherever it points.
			admit(entryAbs[i], p, p, false, 0)
		}
	}
	if found == 0 {
		// Pruning excluded directories means an all-excluded tree no longer
		// reaches the empty-queue guard below with a non-zero found: seed
		// never walked far enough to count its files. skipped is the only
		// way left to tell that apart from a tree with no Markdown at all.
		if skipped > 0 {
			return nil, fmt.Errorf("no Markdown files found in entry points: every candidate path was excluded")
		}
		return nil, fmt.Errorf("no Markdown files found in entry points")
	}
	// found counts what the entry points contain, before exclusion. A run
	// whose every seed was excluded reaches here with a non-zero found and
	// an empty queue, and would otherwise succeed having written nothing —
	// indistinguishable, from the exit code, from a build that worked.
	if len(queue) == 0 {
		return nil, fmt.Errorf("every Markdown file in the entry points is excluded")
	}

	// linksBySrc caches the links extracted from each document during
	// discovery, keyed by resolved source path, so buildLinkMaps can reuse
	// them below instead of re-reading and re-parsing every document.
	linksBySrc := map[string][]Link{}

	var order []string
	for len(queue) > 0 {
		cur := queue[0].path
		hops := queue[0].hops
		queue = queue[1:]

		src, err := os.ReadFile(cur)
		if err != nil {
			// Warn and skip: an unreadable document must not enter the emit
			// set, or the CLI reads it again, fails again, and reports twice.
			res.Warnings = append(res.Warnings, Warning{cur, "unreadable: " + err.Error()})
			continue
		}
		order = append(order, cur)
		root, err := parseDoc(src)
		if err != nil {
			res.Warnings = append(res.Warnings, Warning{cur, "parse failed: " + err.Error()})
			continue
		}

		links := ExtractLinks(root, filepath.Dir(cur))
		linksBySrc[cur] = links
		for _, l := range links {
			if l.Kind == LinkAsset {
				if _, statErr := os.Stat(l.Abs); statErr != nil {
					res.Warnings = append(res.Warnings, Warning{cur,
						fmt.Sprintf("referenced asset does not exist: %s", l.Href)})
				}
				continue
			}
			if l.Kind != LinkDoc {
				continue
			}
			target, err := resolve(l.Abs)
			if err != nil {
				continue
			}
			if _, statErr := os.Stat(target); statErr != nil {
				res.Warnings = append(res.Warnings, Warning{cur,
					fmt.Sprintf("link target does not exist: %s", l.Href)})
				continue
			}
			// LinkDepth 0 is unlimited; a negative value follows nothing.
			// The asset-existence warnings above stay unconditional —
			// a missing image is worth reporting whether or not this
			// document's links are being followed.
			if opt.LinkDepth < 0 || (opt.LinkDepth > 0 && hops >= opt.LinkDepth) {
				continue
			}
			admit(cur, target, l.Href, true, hops+1)
		}
	}

	sort.Strings(order)
	for _, s := range order {
		res.Docs = append(res.Docs, Doc{Src: s, Out: outputPath(s, base, opt.OutDir)})
	}
	// Two distinct sources can map to one output path: a.md and a.markdown
	// in one directory, or a real document under base/_external colliding
	// with a document pulled in from outside base. The emit pass is
	// parallel, so that is a file-level race between two goroutines, not a
	// benign last-writer-wins. Refuse the whole run instead.
	srcByOut := make(map[string]string, len(res.Docs))
	for _, d := range res.Docs {
		if prev, dup := srcByOut[d.Out]; dup {
			return nil, fmt.Errorf("output path collision: %s and %s both produce %s",
				prev, d.Src, d.Out)
		}
		srcByOut[d.Out] = d.Src
	}
	sort.Strings(res.External)
	res.External = dedupe(res.External)

	buildLinkMaps(res, opt, linksBySrc)
	return res, nil
}

// buildLinkMaps computes, for every document, the replacement for each
// href it contains. Document links resolve to other emitted documents
// inside the output tree; asset links point back out at the original file
// on disk, since assets are never copied. linksBySrc supplies the links
// already extracted for each document during discovery, so this does not
// re-read or re-parse any file.
func buildLinkMaps(res *CrawlResult, opt CrawlOptions, linksBySrc map[string][]Link) {
	// Index the emit set by source path for O(1) lookup.
	outBySrc := make(map[string]string, len(res.Docs))
	for _, d := range res.Docs {
		outBySrc[d.Src] = d.Out
	}

	for i := range res.Docs {
		d := &res.Docs[i]
		d.LinkMap = map[string]string{}

		outDir := filepath.Dir(d.Out)
		for _, l := range linksBySrc[d.Src] {
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
				d.LinkMap[l.Href] = encodePath(filepath.ToSlash(rel)) + fragmentOf(l.Href)

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
				d.LinkMap[l.Href] = encodePath(filepath.ToSlash(rel))
			}
		}
	}
}

// encodePath percent-encodes each segment of a slash-separated relative
// path. ExtractLinks decodes %XX before touching the filesystem, so the
// replacement — computed from filesystem paths — has to be re-encoded or a
// name containing a space, '#', '?' or '%' would emit an href that resolves
// to the wrong target or to nothing. Separators are left alone, as are the
// "." and ".." segments, which url.PathEscape treats as unreserved.
func encodePath(rel string) string {
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// fragmentOf returns the #fragment portion of an href, or "".
func fragmentOf(href string) string {
	if i := strings.IndexByte(href, '#'); i >= 0 {
		return href[i:]
	}
	return ""
}

// dedupe removes adjacent duplicates from a sorted slice.
func dedupe(s []string) []string {
	if len(s) < 2 {
		return s
	}
	out := s[:1]
	for _, v := range s[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
