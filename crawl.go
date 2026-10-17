package md2html

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path"
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
	// affects link traversal; LinkDepth, below, bounds that instead.
	Depth int
	// NoMdLinks leaves document links unrewritten.
	NoMdLinks bool
	// NoAssets leaves asset links unrewritten.
	NoAssets bool
	// Exclude lists what must never be entered. A value containing *, ? or
	// [ is a filepath.Match pattern tested against each file and directory
	// name below the resolved base, so "AUDIT_*" names documents at any
	// depth; any other value is a directory prefix, either absolute or
	// relative to base. A path at or beneath a prefix, or with a name
	// matching a pattern, is never seeded, never followed as a link target,
	// and never written to; a link pointing at one keeps its href exactly as
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
	// It reads exactly as Depth does: a non-negative value is the bound
	// itself, and -1 is unlimited. 0 therefore follows no links at all,
	// which is what makes a zero-valued CrawlOptions coherent rather than
	// merely conservative — Depth's zero already seeds a directory's own
	// files and none of its subdirectories, so both fields describe the
	// smallest possible run, and the CLI asks for unlimited on both.
	LinkDepth int
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
	// Mounts, from ReadMounts, make the crawl a mounted site's: every
	// Markdown file a mount holds is a page at its site path, nothing else
	// is built, and no link is followed. They need SiteShell and OutDir,
	// and refuse Entries. Base is then the source root.
	Mounts []Mount
}

// Doc is one document in the emit set.
type Doc struct {
	// Src is the resolved absolute source path.
	Src string
	// Out is the absolute destination path for the generated HTML.
	Out string
	// SitePath is the page's place in a site: a slash path from the site's
	// root, with its source's extension, such as "guide/sites.md". It is
	// the page's path under Base, or, in a mounted site, under its mount's
	// path; "" for a page the site has no place for.
	SitePath string
	mount    *Mount // the mount holding the page, in a mounted site
	// LinkMap maps an href exactly as written in this document to its
	// replacement. Populated by buildLinkMaps.
	LinkMap map[string]string
	// LinkDrops names the hrefs, as written in this document, whose links a
	// site build removes: a link out of the site it has nowhere to send.
	// Nil outside a site build.
	LinkDrops map[string]bool
	// AnchorLinkMap and AnchorLinkDrops are, in a site build, the
	// replacement for, or removal of, an <a>'s href, which can differ from
	// what the same href means on an element the page loads. Nil outside a
	// site build.
	AnchorLinkMap   map[string]string
	AnchorLinkDrops map[string]bool
	// Meta is the document's front matter, nil when it has none. Discovery
	// reads it so a site can be laid out before any page converts.
	Meta map[string]string
	// Title is the document's title as Convert derives it with no options:
	// the front matter title, else the first <h1>, else the file's base
	// name. A site labels the page with it.
	Title string
}

// Warning is a non-fatal problem found during a run.
type Warning struct {
	// Src is the document the warning was raised while processing, or, for
	// a warning with no document to point at (an --exclude value that
	// matches nothing on disk), the offending value itself.
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
	// Assets are the files a site build copies into its output, each once,
	// sorted by Out. Empty outside a site build; see CopyAssets.
	Assets []Asset

	rules *siteRules // a site build's link rules, for BuildSite's shell
}

// parseDoc converts Markdown straight to a parsed tree, skipping transforms
// and the page shell. Discovery only needs the tree, and going through
// Convert would inline the whole stylesheet on every discovery pass.
func parseDoc(src []byte) (*html.Node, error) {
	var buf bytes.Buffer
	// nil: discovery only needs the tree to extract links, and has no
	// per-document warning sink of its own to hand a `fig` fence. Charts
	// and highlighting are off, so discovery never starts either engine.
	if err := newParser(nil, chartContext{off: true}, highlightContext{}).Convert(src, &buf); err != nil {
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
// how many candidate paths exclusion caused it to drop. Excluded paths
// contribute nothing, and an excluded directory
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
func seed(entry string, depth int, excluded exclusions) (files []string, skipped int, err error) {
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
		if excluded.match(p) {
			return nil, 1, nil
		}
		return []string{p}, 0, nil
	}

	rootAbs, err := resolve(entry)
	if err != nil {
		return nil, 0, err
	}
	err = filepath.WalkDir(rootAbs, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip, do not abort
		}
		if d.IsDir() {
			if excluded.match(p) {
				skipped++
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
			if excluded.match(rp) {
				skipped++
				return nil
			}
			files = append(files, rp)
		}
		return nil
	})
	return files, skipped, err
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

// resolveExcludes sorts each Exclude value into a name pattern or an
// absolute, cleaned, symlink-resolved directory prefix, plus a warning for
// any value that can exclude nothing.
//
// A name pattern is matched against a single file or directory name, which
// can never contain a separator, so a pattern holding one — most likely a
// path glob like "docs/AUDIT_*" — could never match. That, and a pattern
// filepath.Match rejects as malformed, is warned about and dropped. A
// pattern is not checked against the disk: finding out whether it matches
// anything would mean walking the whole tree up front.
//
// A relative prefix is taken against base, which is why this runs after
// commonAncestor rather than at the top of Crawl. Symlinks are resolved so
// a subtree reached through a link is still recognized as the excluded one,
// matching how every other path in the crawler is keyed. A value naming
// nothing on disk is kept as a literal cleaned path rather than dropped —
// excluding a directory that does not exist yet is harmless — but it is
// warned about: a literal path that exists nowhere excludes nothing, which
// is observationally identical to silently dropping it, and because a
// relative value is resolved against base rather than against the entry
// point that named it, a value like "docs/vendor" against entry "docs"
// resolves to ".../docs/docs/vendor" and excludes nothing without the
// warning saying why.
func resolveExcludes(vals []string, base string) (exclusions, []Warning) {
	ex := exclusions{base: base}
	var warnings []Warning
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if isNamePattern(v) {
			if strings.ContainsRune(v, '/') || strings.ContainsRune(v, filepath.Separator) {
				warnings = append(warnings, Warning{v,
					fmt.Sprintf("exclude %q matches nothing: a pattern is matched against single file and directory names, so it cannot contain a path separator", v)})
				continue
			}
			if _, err := filepath.Match(v, ""); err != nil {
				warnings = append(warnings, Warning{v,
					fmt.Sprintf("exclude %q is not a valid pattern: %v", v, err)})
				continue
			}
			ex.names = append(ex.names, v)
			continue
		}
		p := v
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		if _, statErr := os.Stat(p); statErr != nil {
			warnings = append(warnings, Warning{v,
				fmt.Sprintf("exclude %q matches nothing on disk", v)})
		}
		r, err := resolve(p)
		if err != nil {
			r = filepath.Clean(p)
		}
		ex.prefixes = append(ex.prefixes, r)
	}
	return ex, warnings
}

// discovered is what the crawl learns from reading one document.
type discovered struct {
	parsed bool // false for a document that read but did not parse
	meta   map[string]string
	title  string
	links  []Link
}

// discover reads the document at cur and warns about the links in it that
// lead to nothing. For each link to a document that exists it then calls
// follow, when not nil, so what follow says lands among those warnings in
// the order of the links. ok is false when the document cannot be read,
// which it has warned about; one that reads but does not parse is still a
// page, with nothing discovered in it.
func discover(cur string, res *CrawlResult, follow func(l Link, target string)) (d discovered, ok bool) {
	src, err := os.ReadFile(cur)
	if err != nil {
		// Warn and skip: an unreadable document must not enter the emit
		// set, or the CLI reads it again, fails again, and reports twice.
		res.Warnings = append(res.Warnings, Warning{cur, "unreadable: " + err.Error()})
		return d, false
	}
	// Strip front matter before parsing, matching what Convert sees: a
	// Markdown link or image sitting inside a "title:" value is source
	// text, not a document reference, and must not be followed or
	// checked for existence just because it was still in the tree the
	// unstripped bytes would have produced.
	meta, body, _ := splitFrontMatter(src)
	root, err := parseDoc(body)
	if err != nil {
		res.Warnings = append(res.Warnings, Warning{cur, "parse failed: " + err.Error()})
		return d, true
	}
	d.parsed, d.meta = true, meta
	d.title = meta["title"]
	if d.title == "" {
		d.title = extractTitle(root, cur)
	}
	d.links = ExtractLinks(root, filepath.Dir(cur))
	for _, l := range d.links {
		switch l.Kind {
		case LinkAsset:
			if _, statErr := os.Stat(l.Abs); statErr != nil {
				res.Warnings = append(res.Warnings, Warning{cur,
					fmt.Sprintf("referenced asset does not exist: %s", l.Href)})
			}
		case LinkDoc:
			target, err := resolve(l.Abs)
			if err != nil {
				continue
			}
			if _, statErr := os.Stat(target); statErr != nil {
				res.Warnings = append(res.Warnings, Warning{cur,
					fmt.Sprintf("link target does not exist: %s", l.Href)})
				continue
			}
			if follow != nil {
				follow(l, target)
			}
		}
	}
	return d, true
}

// checkOutputs refuses two sources that map to one output path: a.md and
// a.markdown in one directory, a real document under base/_external
// colliding with a document pulled in from outside base, or two mounts that
// place a page at one site path. The emit pass is parallel, so that is a
// file-level race between two goroutines, not a benign last-writer-wins.
func checkOutputs(docs []Doc) error {
	srcByOut := make(map[string]string, len(docs))
	for _, d := range docs {
		if prev, dup := srcByOut[d.Out]; dup {
			return fmt.Errorf("output path collision: %s and %s both produce %s", prev, d.Src, d.Out)
		}
		srcByOut[d.Out] = d.Src
	}
	return nil
}

// readmesAtIndex writes each README that stands as its directory's page, in
// a directory with no index, at the directory's index.html, so a reader
// reaches it at the directory's address and the root's is the home page.
// Called for a site build into an output directory, before the link maps,
// so links to it follow.
func readmesAtIndex(docs []Doc) {
	hasIndex := map[string]bool{}
	for _, d := range docs {
		if d.SitePath != "" && isIndexStem(sitePathStem(d.SitePath)) {
			hasIndex[sitePathDir(d.SitePath)] = true
		}
	}
	for i := range docs {
		d := &docs[i]
		if d.SitePath != "" && isReadmeStem(sitePathStem(d.SitePath)) && !hasIndex[sitePathDir(d.SitePath)] {
			d.Out = filepath.Join(filepath.Dir(d.Out), "index.html")
		}
	}
}

// Crawl discovers every document reachable from the entry points.
func Crawl(opt CrawlOptions) (*CrawlResult, error) {
	if len(opt.Mounts) > 0 {
		return crawlMounts(opt)
	}
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
	// Doc.Out is promised absolute, and every relation computed from it —
	// most of all the asset rewrite — needs it so: filepath.Rel refuses to
	// relate a relative path to an absolute one.
	if opt.OutDir != "" {
		out, err := filepath.Abs(opt.OutDir)
		if err != nil {
			return nil, err
		}
		opt.OutDir = out
	}
	base := commonAncestor(entryAbs)
	excluded, excludeWarnings := resolveExcludes(opt.Exclude, base)
	rules, err := newSiteRules(opt, base, excluded)
	if err != nil {
		return nil, err
	}

	// queued pairs a document with its distance, in links, from the nearest
	// seed. BFS dequeues in nondecreasing hop order, so the first time a
	// document is admitted is always by its shortest path, and re-reaching
	// it later by a longer one cannot matter.
	type queued struct {
		path string
		hops int
	}

	res := &CrawlResult{Base: base, Warnings: excludeWarnings}
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
		if excluded.match(target) {
			if viaLink {
				res.Warnings = append(res.Warnings, Warning{src,
					fmt.Sprintf("not following %s: excluded", ref)})
			}
			return
		}
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
		// skipped can't tell "everything here was excluded" apart from "an
		// excluded directory happened to be empty too": SkipDir is what
		// makes pruning cheap, and it fires on the directory itself without
		// looking inside, so skipped counts pruned paths, not files that
		// would otherwise have been Markdown. The message can only claim
		// exclusion was involved, not that it was the sole cause.
		if skipped > 0 {
			return nil, fmt.Errorf("no Markdown files found in entry points (some paths were excluded)")
		}
		return nil, fmt.Errorf("no Markdown files found in entry points")
	}

	// linksBySrc caches the links extracted from each document during
	// discovery, keyed by resolved source path, so buildLinkMaps can reuse
	// them below instead of re-reading and re-parsing every document.
	linksBySrc := map[string][]Link{}
	metaBySrc := map[string]map[string]string{}
	titleBySrc := map[string]string{}

	var order []string
	for len(queue) > 0 {
		cur := queue[0].path
		hops := queue[0].hops
		queue = queue[1:]

		d, ok := discover(cur, res, func(l Link, target string) {
			// A negative LinkDepth is unlimited; otherwise this document
			// sits at hops and may only be followed out of while that is
			// still short of the bound.
			if opt.LinkDepth >= 0 && hops >= opt.LinkDepth {
				return
			}
			admit(cur, target, l.Href, true, hops+1)
		})
		if !ok {
			continue
		}
		order = append(order, cur)
		if !d.parsed {
			continue // read but not parsed: a page with nothing discovered
		}
		metaBySrc[cur] = d.meta
		titleBySrc[cur] = d.title
		linksBySrc[cur] = d.links
	}

	sort.Strings(order)
	for _, s := range order {
		sp, _ := relUnder(base, s)
		res.Docs = append(res.Docs, Doc{Src: s, Out: outputPath(s, base, opt.OutDir), SitePath: sp,
			Meta: metaBySrc[s], Title: titleBySrc[s]})
	}
	if rules != nil {
		readmesAtIndex(res.Docs)
	}
	if err := checkOutputs(res.Docs); err != nil {
		return nil, err
	}
	sort.Strings(res.External)
	res.External = dedupe(res.External)

	res.rules = rules
	if err := buildLinkMaps(res, opt, linksBySrc); err != nil {
		return nil, err
	}
	return res, nil
}

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
			d.AnchorLinkMap, d.AnchorLinkDrops = p.anchorRewrite, p.anchorDrop
		}
		take(d.Src, p)
	}
	if res.rules != nil {
		// Read where it really is, as each page reads it, so a shell reached
		// through a symlink has its links planned from one directory.
		shell := res.rules.shell
		raw, err := os.ReadFile(shell)
		if err != nil {
			return fmt.Errorf("site shell: %w", err)
		}
		_, body, _ := splitFrontMatter(raw)
		root, err := parseDoc(body)
		if err != nil {
			return fmt.Errorf("site shell: %w", err)
		}
		// The shell's links are planned from a page at the output root;
		// only the copies, warnings and failures are kept, and each page
		// plans the shell's hrefs again from where it is.
		take(shell, planLinks(ExtractLinks(root, filepath.Dir(shell)),
			filepath.Join(opt.OutDir, "index.html"), outBySrc, opt, res.rules))
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
// stops the build. In a site build an <a>'s hrefs are planned apart, in
// anchorRewrite and anchorDrop, since a link a reader follows out of the
// site can go elsewhere than the same href on an image the page shows.
type linkPlan struct {
	rewrite       map[string]string
	drop          map[string]bool
	anchorRewrite map[string]string
	anchorDrop    map[string]bool
	copies        []Asset
	warns         []string
	fails         []string
}

// planLinks plans links, as written in a document whose output is fromOut.
// Without site rules, document links resolve to other emitted documents
// and asset links point back out at the original file on disk; with them,
// siteRules.action decides. A site uses it to rewrite its shell's links
// once per page.
func planLinks(links []Link, fromOut string, outBySrc map[string]string, opt CrawlOptions, rules *siteRules) linkPlan {
	p := linkPlan{rewrite: map[string]string{}, drop: map[string]bool{}}
	if rules != nil {
		p.anchorRewrite, p.anchorDrop = map[string]string{}, map[string]bool{}
		for _, l := range links {
			a := rules.action(l, fromOut, outBySrc)
			rewrite, drop := p.rewrite, p.drop
			if !l.Loaded {
				rewrite, drop = p.anchorRewrite, p.anchorDrop
			}
			switch {
			case a.fail != "":
				p.fails = append(p.fails, a.fail)
			case a.drop:
				drop[l.Href] = true
			case a.rewrite != "":
				if opt.NoMdLinks && l.Kind == LinkDoc {
					continue
				}
				rewrite[l.Href] = a.rewrite
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

// crawlMounts is Crawl for a site its mounts describe. Each Markdown file a
// mount holds, and no deeper mount holds instead, is a page at its site
// path; a file excluded by --exclude, relative to the source root, or by its
// mount's own excludes, is not. No link is followed: what the mounts hold is
// the whole site.
func crawlMounts(opt CrawlOptions) (*CrawlResult, error) {
	if len(opt.Entries) > 0 {
		return nil, fmt.Errorf("the site's mounts say what it holds; give no entry points beside them")
	}
	if opt.SiteShell == "" || opt.OutDir == "" {
		return nil, fmt.Errorf("mounts need a site shell and an output directory")
	}
	out, err := filepath.Abs(opt.OutDir)
	if err != nil {
		return nil, err
	}
	opt.OutDir = out
	rules, err := newSiteRules(opt, "", exclusions{})
	if err != nil {
		return nil, err
	}
	base := rules.sourceRoot
	excluded, excludeWarnings := resolveExcludes(opt.Exclude, base)
	rules.excluded = excluded
	res := &CrawlResult{Base: base, Warnings: excludeWarnings}
	rules.mounts = make([]mountRule, len(opt.Mounts))
	for i := range opt.Mounts {
		m := &opt.Mounts[i]
		if excluded.match(m.Source) {
			return nil, fmt.Errorf("%s: source %s is excluded by --exclude", m.From, m.Source)
		}
		mr := mountRule{Mount: m, excluded: exclusions{base: m.Source}}
		if m.IsDir {
			var w []Warning
			mr.excluded, w = resolveExcludes(m.Exclude, m.Source)
			res.Warnings = append(res.Warnings, w...)
		}
		rules.mounts[i] = mr
	}

	mountOf := map[string]*Mount{}
	for i := range rules.mounts {
		mr := &rules.mounts[i]
		cands := []string{mr.Source}
		if mr.IsDir {
			cands = nil
			_ = filepath.WalkDir(mr.Source, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return nil // unreadable subtree: skip, do not abort
				}
				if p != mr.Source && (excluded.match(p) || mr.excluded.match(p)) {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if !d.IsDir() && IsMarkdownPath(p) {
					cands = append(cands, p)
				}
				return nil
			})
		}
		for _, p := range cands {
			if rules.owner(p) != mr {
				continue // a deeper mount holds its path, and considers it
			}
			real, err := resolve(p)
			if err != nil {
				continue
			}
			if real == rules.shell {
				continue // the shell is the site's template, never a page
			}
			owner := rules.owner(real)
			if owner == nil {
				res.Warnings = append(res.Warnings, Warning{p,
					fmt.Sprintf("not building %s: it leads to %s, which no mount holds", p, real)})
				continue
			}
			if owner != mr || rules.excludedAt(real) {
				continue // a deeper mount holds it, or it is excluded there
			}
			mountOf[real] = owner.Mount
		}
	}

	order := make([]string, 0, len(mountOf))
	for src := range mountOf {
		order = append(order, src)
	}
	sort.Strings(order)
	linksBySrc := map[string][]Link{}
	for _, src := range order {
		d, ok := discover(src, res, nil)
		if !ok {
			continue
		}
		sp, _ := rules.place(src)
		outPath := filepath.Join(out, filepath.FromSlash(strings.TrimSuffix(sp, path.Ext(sp))+".html"))
		res.Docs = append(res.Docs, Doc{Src: src, Out: outPath, SitePath: sp,
			Meta: d.meta, Title: d.title, mount: mountOf[src]})
		linksBySrc[src] = d.links
	}
	readmesAtIndex(res.Docs)
	if err := checkOutputs(res.Docs); err != nil {
		return nil, err
	}
	res.rules = rules
	if err := buildLinkMaps(res, opt, linksBySrc); err != nil {
		return nil, err
	}
	return res, nil
}
