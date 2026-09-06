package md2html

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

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

// seed returns the Markdown files a single entry point contributes.
func seed(entry string, depth int) ([]string, error) {
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
			out = append(out, rp)
		}
		return nil
	})
	return out, err
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

// Crawl discovers every document reachable from the entry points.
func Crawl(opt CrawlOptions) (*CrawlResult, error) {
	if len(opt.Entries) == 0 {
		return nil, fmt.Errorf("no entry points given")
	}

	var seeds []string
	for _, e := range opt.Entries {
		s, err := seed(e, opt.Depth)
		if err != nil {
			return nil, err
		}
		seeds = append(seeds, s...)
	}
	if len(seeds) == 0 {
		return nil, fmt.Errorf("no Markdown files found in entry points")
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

	res := &CrawlResult{Base: base}
	visited := map[string]bool{}
	var queue []string
	for _, s := range seeds {
		if !visited[s] {
			visited[s] = true
			queue = append(queue, s)
		}
	}

	var order []string
	for len(queue) > 0 {
		cur := queue[0]
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

		for _, l := range ExtractLinks(root, filepath.Dir(cur)) {
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
			if !isUnder(target, base) {
				if opt.OutDir == "" {
					res.Warnings = append(res.Warnings, Warning{cur,
						fmt.Sprintf("refusing to follow %s outside %s (no -o given)", l.Href, base)})
					continue
				}
				res.External = append(res.External, target)
			}
			if !visited[target] {
				visited[target] = true
				queue = append(queue, target)
			}
		}
	}

	sort.Strings(order)
	for _, s := range order {
		res.Docs = append(res.Docs, Doc{Src: s, Out: outputPath(s, base, opt.OutDir)})
	}
	sort.Strings(res.External)
	res.External = dedupe(res.External)
	return res, nil
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
