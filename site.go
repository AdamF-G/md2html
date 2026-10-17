package md2html

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Site is a run's shell and the tree of pages it wraps. BuildSite makes one
// from a crawl, and Convert uses it through Options.Site. Convert calls in
// parallel share one, so it is safe for concurrent use.
type Site struct {
	// Warnings are the problems found while laying the site out, and those
	// in the shell's own source, each once, found while pages convert. Read
	// them after every Convert call using the site has returned. Pages
	// convert in parallel, so they arrive in no fixed order: sort them for
	// stable output, as the command does, by Src and then Message.
	Warnings []Warning

	shellSrc  string
	shellDir  string
	shellMeta map[string]string
	shellBody []byte
	shellTOC  bool // the shell has a [[toc]], so --autotoc adds none to a page
	facets    []string
	base      string
	outRoot   string
	outBySrc  map[string]string
	crawl     CrawlOptions
	rules     *siteRules // the crawl's link rules, for the shell's links
	refRoot   string     // what a mounted site's pages may read from; "" without mounts
	root      *siteNode
	bySrc     map[string]*siteNode
	order     []*siteNode
	dirs      map[string]*siteNode // directory nodes by site path, while the tree is built

	mu          sync.Mutex
	entries     []searchEntry   // the search index, in arrival order
	entryAt     map[string]int  // each entry's place in entries, by URL
	shellWarned map[string]bool // shell warnings already in Warnings
}

// siteNode is one place in the tree: a page, or a directory, which has a
// page when it holds an index.md.
type siteNode struct {
	name       string // file stem or directory name
	label      string
	section    string // slug of the top-level node holding it; "" at the root
	path       string // a directory's path, for warnings
	doc        *Doc
	isDir      bool
	hidden     bool
	hasOrder   bool
	order      int
	navSort    string // "links": the children are ordered by this page's links
	linkRank   int    // the place of this node's first link in its parent's page
	hasRank    bool
	navSortSet bool   // the page set nav-sort: itself, so a mount's does not reach it
	sitePath   string // a directory's site path, for messages
	parent     *siteNode
	children   []*siteNode
}

// BuildSite lays out the site for a crawl, with the page at shell as the
// shell every page is wrapped in. The shell is a template: BuildSite drops
// it from res.Docs, so it is never written. Its front matter must say
// site: true and it must hold exactly one [[content]] marker.
func BuildSite(res *CrawlResult, shell string, opt CrawlOptions) (*Site, error) {
	shellSrc, err := resolve(shell)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(shellSrc)
	if err != nil {
		return nil, fmt.Errorf("site shell: %w", err)
	}
	meta, body, malformed := splitFrontMatter(raw)
	if malformed || meta["site"] != "true" {
		return nil, fmt.Errorf("%s is not a site shell: its front matter must say site: true", shell)
	}
	s := &Site{shellSrc: shellSrc, shellDir: filepath.Dir(shellSrc), shellBody: body,
		base: res.Base, crawl: opt}
	s.rules = res.rules
	if len(opt.Mounts) > 0 && res.rules != nil {
		s.refRoot = res.rules.sourceRoot
	}
	// Kept as written: the list markers resolve the keys they read, per
	// page, as on any page.
	s.shellMeta = meta
	parsed, err := parseDoc(body)
	if err != nil {
		return nil, fmt.Errorf("site shell %s: %w", shell, err)
	}
	contents := 0
	for _, m := range findMarkers(parsed) {
		if m.name == "content" {
			contents++
		}
	}
	if contents != 1 {
		return nil, fmt.Errorf("%s needs exactly one [[content]] marker; it has %d", shell, contents)
	}
	s.shellTOC = len(findTOCMarkers(parsed)) > 0
	for _, m := range opt.Mounts {
		if m.TOC == "shell" && !s.shellTOC {
			return nil, fmt.Errorf(`%s: toc="shell" needs a [[toc]] in the shell, whose list replaces the pages' own`, m.From)
		}
	}
	for _, f := range strings.Split(s.shellMeta["search-facets"], ",") {
		if f = strings.ToLower(strings.TrimSpace(f)); f != "" {
			s.facets = append(s.facets, f)
		}
	}

	kept := res.Docs[:0]
	for _, d := range res.Docs {
		if d.Src != shellSrc {
			kept = append(kept, d)
		}
	}
	res.Docs = kept

	outDir := opt.OutDir
	if outDir != "" {
		if outDir, err = filepath.Abs(outDir); err != nil {
			return nil, err
		}
	}
	s.outRoot = filepath.Dir(outputPath(filepath.Join(res.Base, "index.md"), res.Base, outDir))
	if err := s.writtenPages(res, opt); err != nil {
		return nil, err
	}
	s.outBySrc = make(map[string]string, len(res.Docs))
	for _, d := range res.Docs {
		s.outBySrc[d.Src] = d.Out
	}
	s.buildTree(res.Docs)
	return s, nil
}

// writtenPages gives every directory in the site a page: one with neither an
// index nor a README gets a page md2html writes, added to res.Docs at
// <dir>/index.md, so the tree, search, and previous and next treat it as any
// page. Its title is its mount's title, for a directory mount's own
// directory, else its name in title case, with the shell's title-case:
// spellings; its body lists the directory's pages.
func (s *Site) writtenPages(res *CrawlResult, opt CrawlOptions) error {
	fixed := titleCaseWords(s.shellMeta["title-case"])
	hasPage := map[string]bool{}
	dirs := map[string]bool{"": true}
	shown := map[string]bool{}
	for _, d := range res.Docs {
		if d.SitePath == "" {
			continue
		}
		dirRel := sitePathDir(d.SitePath)
		stem := sitePathStem(d.SitePath)
		if isIndexStem(stem) || isReadmeStem(stem) {
			hasPage[dirRel] = true
		}
		for r := dirRel; ; r = sitePathDir(r) {
			dirs[r] = true
			if d.Meta["nav"] != "hidden" {
				shown[r] = true
			}
			if r == "" {
				break
			}
		}
	}
	titles := map[string]string{}
	for _, m := range opt.Mounts {
		if m.IsDir && m.Title != "" {
			titles[m.Path] = m.Title
		}
	}
	assetAt := map[string]string{}
	for _, a := range res.Assets {
		assetAt[a.Out] = a.Src
	}
	var rels []string
	for r := range dirs {
		if hasPage[r] {
			continue
		}
		// A directory whose index or README the crawl did not reach has a
		// page the build lacks; writing another would hide or overwrite it.
		// A mounted site skips this look: a directory mount takes every
		// file in its source, so an unreached index can only be excluded.
		if len(opt.Mounts) == 0 {
			if src := s.unreachedPage(filepath.Join(s.base, filepath.FromSlash(r))); src != "" {
				s.warn(src, "not in the build: no page links it, so its directory has no page")
				continue
			}
		}
		rels = append(rels, r)
	}
	sort.Strings(rels)
	for _, r := range rels {
		title := titles[r]
		if title == "" {
			name := path.Base(r)
			if r == "" {
				name = filepath.Base(s.rootSource(opt))
			}
			if title = titleCase(name, fixed); title == "" {
				title = name // a name of only separators, such as _
			}
		}
		out := filepath.Join(s.outRoot, filepath.FromSlash(r), "index.html")
		if src, taken := assetAt[out]; taken {
			return fmt.Errorf("output path collision: %s and the page md2html writes for %s both produce %s", src, displaySitePath(r), out)
		}
		sp := path.Join(r, "index.md")
		var meta map[string]string
		if !shown[r] {
			// Nothing in it is shown, so its page is not either.
			meta = map[string]string{"nav": "hidden"}
		}
		res.Docs = append(res.Docs, Doc{
			Src:      filepath.Join(s.outRoot, filepath.FromSlash(sp)),
			Out:      out,
			SitePath: sp,
			Meta:     meta,
			Title:    title,
			Body:     []byte("# " + escapeMarkdownText(title) + "\n\n[[pages from=\"page\" depth=\"1\"]]\n"),
		})
	}
	return nil
}

// unreachedPage is the Markdown file in the source directory dir whose stem
// is index or readme, in any case, or "" when it holds none.
func (s *Site) unreachedPage(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !IsMarkdownPath(e.Name()) {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if isIndexStem(stem) || isReadmeStem(stem) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// escapeMarkdownText backslash-escapes the ASCII punctuation in s, so
// Markdown reads it as the text it is. A comment after each [ also keeps the
// chips transform, which reads the rendered text, from taking [draft] for a
// chip.
func escapeMarkdownText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x80 && strings.ContainsRune("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
		if r == '[' {
			b.WriteString("<!---->")
		}
	}
	return b.String()
}

// rootSource is the directory the site's root stands for: the root mount's
// source in a mounted site, the run's base otherwise.
func (s *Site) rootSource(opt CrawlOptions) string {
	for _, m := range opt.Mounts {
		if m.Path == "" {
			return m.Source
		}
	}
	return s.base
}

func (s *Site) warn(src, msg string) { s.Warnings = append(s.Warnings, Warning{src, msg}) }

// shellWarn records a problem in the shell's own source. Every page
// processes the shell, so each message is kept once, under the shell's
// path. Convert calls run in parallel, so it locks; Warnings is read once
// they are all done.
func (s *Site) shellWarn(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shellWarned[msg] {
		return
	}
	if s.shellWarned == nil {
		s.shellWarned = map[string]bool{}
	}
	s.shellWarned[msg] = true
	s.warn(s.shellSrc, msg)
}

// buildTree places every document below the run's base: a directory is a
// node, its index.md its page, and a document from outside the base is
// written but has no place.
func (s *Site) buildTree(docs []Doc) {
	s.root = &siteNode{isDir: true, path: s.base}
	s.bySrc = map[string]*siteNode{}
	dirs := map[string]*siteNode{"": s.root}
	s.dirs = dirs
	var dirFor func(rel string) *siteNode
	dirFor = func(rel string) *siteNode {
		if n, ok := dirs[rel]; ok {
			return n
		}
		parentRel := path.Dir(rel)
		if parentRel == "." {
			parentRel = ""
		}
		parent := dirFor(parentRel)
		n := &siteNode{name: path.Base(rel), isDir: true, parent: parent, sitePath: rel,
			path: filepath.Join(s.base, filepath.FromSlash(rel))}
		parent.children = append(parent.children, n)
		dirs[rel] = n
		return n
	}
	// A README stands as its directory's page only where the directory has
	// no index, whichever the crawl met first, so the indexes are found
	// before any page is placed.
	hasIndex := map[string]bool{}
	for _, d := range docs {
		if d.SitePath != "" && isIndexStem(sitePathStem(d.SitePath)) {
			hasIndex[sitePathDir(d.SitePath)] = true
		}
	}
	for i := range docs {
		d := &docs[i]
		rel := d.SitePath
		if rel == "" {
			s.warn(d.Src, fmt.Sprintf("is outside the site's root %s; it is written but has no place in the site's navigation", s.base))
			continue
		}
		dirRel := path.Dir(rel)
		if dirRel == "." {
			dirRel = ""
		}
		dir := dirFor(dirRel)
		stem := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
		n := dir
		isPage := isIndexStem(stem) || (isReadmeStem(stem) && !hasIndex[dirRel])
		if !isPage || dir.doc != nil {
			n = &siteNode{name: stem, parent: dir}
			dir.children = append(dir.children, n)
		}
		n.doc = d
		s.bySrc[d.Src] = n
		s.applyMeta(n, d)
	}
	s.applyMounts()
	var finish func(n *siteNode)
	finish = func(n *siteNode) {
		// A directory with no page of its own (a mount that brings in no
		// pages, or a tree built without writtenPages) is labelled by name.
		if n.isDir && n.doc == nil && n.label == "" {
			n.label = n.name
		}
		for _, c := range n.children {
			finish(c)
		}
		if n.navSort == "links" {
			s.rankByLinks(n)
		}
		sort.SliceStable(n.children, func(i, j int) bool { return lessNode(n.children[i], n.children[j]) })
	}
	finish(s.root)
	for _, top := range s.root.children {
		slug := slugify(top.name)
		var mark func(n *siteNode)
		mark = func(n *siteNode) {
			n.section = slug
			for _, c := range n.children {
				mark(c)
			}
		}
		mark(top)
	}
	// A hidden page takes its subtree with it, except at the root: a
	// hidden root hides its own page, not the whole site.
	var visit func(n *siteNode)
	visit = func(n *siteNode) {
		if n.hidden && n != s.root {
			return
		}
		if n.doc != nil && !n.hidden {
			s.order = append(s.order, n)
		}
		for _, c := range n.children {
			visit(c)
		}
	}
	visit(s.root)
}

// applyMeta reads a page's navigation keys. A hidden page takes its
// subtree with it: a directory whose index is hidden is hidden whole, but
// for the root's, which hides only the root's page.
func (s *Site) applyMeta(n *siteNode, d *Doc) {
	n.label = d.Title
	if t := d.Meta["nav-title"]; t != "" {
		n.label = t
	}
	if v := d.Meta["nav-order"]; v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			n.order, n.hasOrder = i, true
		} else {
			s.warn(d.Src, fmt.Sprintf("front matter nav-order %q is not a whole number; ignoring it", v))
		}
	}
	switch v := d.Meta["nav"]; v {
	case "":
	case "hidden":
		n.hidden = true
	default:
		s.warn(d.Src, fmt.Sprintf("front matter nav %q is not hidden; ignoring it", v))
	}
	switch v := d.Meta["nav-sort"]; v {
	case "":
	case "label", "links":
		if !n.isDir {
			if v == "links" {
				s.warn(d.Src, "front matter nav-sort applies to a directory's index.md, which orders the directory; ignoring it")
			}
			break
		}
		n.navSortSet = true
		if v == "links" {
			n.navSort = v
		}
	default:
		s.warn(d.Src, fmt.Sprintf("front matter nav-sort %q is not links or label; ordering by label", v))
	}
}

// applyMounts gives each mount's node the mount's settings, over the page's
// own: its label and place among its siblings, and how a directory's
// children are ordered, which reaches every directory below it up to one
// that sets its own, whose setting holds for its subtree.
func (s *Site) applyMounts() {
	for i := range s.crawl.Mounts {
		m := &s.crawl.Mounts[i]
		var n *siteNode
		if m.IsDir {
			n = s.dirs[m.Path]
		} else if m.Path == "" {
			n = s.root
		} else {
			n = s.bySrc[m.Source]
		}
		if n == nil {
			// A directory mount with no node holds no page the site
			// builds. A file mount yields its page, or the crawl said why
			// it could not.
			if m.IsDir {
				s.warn(s.shellSrc, fmt.Sprintf("%s: no pages: the mount's source holds no Markdown file the site builds", m.From))
			}
			continue
		}
		if m.NavTitle != "" {
			n.label = m.NavTitle
		}
		if m.HasNavOrder {
			n.order, n.hasOrder = m.NavOrder, true
		}
		if m.NavSort == "" {
			continue
		}
		sortAs := ""
		if m.NavSort == "links" {
			sortAs = "links"
		}
		n.navSort, n.navSortSet = sortAs, true
		var below func(d *siteNode)
		below = func(d *siteNode) {
			for _, c := range d.children {
				if !c.isDir {
					continue
				}
				if c.navSortSet {
					continue // the nearest setting wins, for its whole subtree
				}
				c.navSort = sortAs
				below(c)
			}
		}
		below(n)
	}
}

// rankByLinks gives each child of dir that dir's page links to a rank: the
// place of its first link in the page's body. A link counts when it reaches
// the child's own page, a page or a subdirectory's index; one deeper into a
// subdirectory, or to anything that is not a child, ranks nothing. The page
// was read once by the crawl; it is read again here, and only for a
// directory that asks, so a site without nav-sort pays nothing.
func (s *Site) rankByLinks(dir *siteNode) {
	if dir.doc == nil {
		return
	}
	if dir.doc.Body != nil {
		return // a written page links nothing
	}
	raw, err := os.ReadFile(dir.doc.Src)
	if err != nil {
		return
	}
	_, body, _ := splitFrontMatter(raw)
	root, err := parseDoc(body)
	if err != nil {
		return
	}
	bySrc := map[string]*siteNode{}
	for _, c := range dir.children {
		if c.doc != nil {
			bySrc[c.doc.Src] = c
		}
	}
	byDir := map[string]*siteNode{}
	for _, c := range dir.children {
		if c.isDir {
			byDir[c.sitePath] = c
		}
	}
	rank := 0
	for _, l := range ExtractLinks(root, filepath.Dir(dir.doc.Src)) {
		if l.Kind == LinkDoc || l.Kind == LinkAsset {
			if target, err := resolve(l.Abs); err == nil {
				if fi, err := os.Stat(target); err == nil && fi.IsDir() {
					var sp string
					var ok bool
					if s.rules != nil {
						sp, ok = s.rules.place(target)
					} else {
						sp, ok = relUnder(s.base, target)
					}
					if c := byDir[sp]; ok && c != nil && !c.hasRank {
						c.linkRank, c.hasRank = rank, true
						rank++
					}
					continue
				}
			}
		}
		if l.Kind != LinkDoc {
			continue
		}
		target, err := resolve(l.Abs)
		if err != nil {
			continue
		}
		if c := bySrc[target]; c != nil && !c.hasRank {
			c.linkRank, c.hasRank = rank, true
			rank++
		}
	}
}

// lessNode orders siblings: those with a nav-order first, by it, then those
// their directory's page ranks by its links (nav-sort: links), by rank, then
// the rest by label, case-insensitively, with the name breaking ties so the
// order never depends on the crawl's.
func lessNode(a, b *siteNode) bool {
	if a.hasOrder != b.hasOrder {
		return a.hasOrder
	}
	if a.hasOrder && a.order != b.order {
		return a.order < b.order
	}
	if a.hasRank != b.hasRank {
		return a.hasRank
	}
	if a.hasRank && a.linkRank != b.linkRank {
		return a.linkRank < b.linkRank
	}
	if la, lb := strings.ToLower(a.label), strings.ToLower(b.label); la != lb {
		return la < lb
	}
	return a.name < b.name
}

// isIndexStem and isReadmeStem name a directory's page by the stem of its
// file, in any case: index.md, Index.markdown, README.md, readme.md.
func isIndexStem(stem string) bool  { return strings.EqualFold(stem, "index") }
func isReadmeStem(stem string) bool { return strings.EqualFold(stem, "readme") }

// sitePathDir is the directory of a site path, "" at the root, and
// sitePathStem its file's name without the extension.
func sitePathDir(sp string) string {
	if d := path.Dir(sp); d != "." {
		return d
	}
	return ""
}

func sitePathStem(sp string) string { return strings.TrimSuffix(path.Base(sp), path.Ext(sp)) }

// section returns the slug of the top-level node holding the page at src,
// or "" for the root page or a page the tree could not place.
func (s *Site) section(src string) string {
	if n := s.bySrc[src]; n != nil {
		return n.section
	}
	return ""
}

// tocFromShell reports whether the page at src takes its contents list from
// the shell alone: its mount says toc="shell".
func (s *Site) tocFromShell(src string) bool {
	n := s.bySrc[src]
	return n != nil && n.doc != nil && n.doc.mount != nil && n.doc.mount.TOC == "shell"
}

// outline renders the tree as indented labels, for tests.
func (s *Site) outline() string {
	var b strings.Builder
	var rec func(n *siteNode, depth int)
	rec = func(n *siteNode, depth int) {
		for _, c := range n.children {
			mark := ""
			if c.hidden {
				mark = " (hidden)"
			}
			if c.doc == nil {
				mark += " (label)"
			}
			fmt.Fprintf(&b, "%s%s%s\n", strings.Repeat("  ", depth), c.label, mark)
			rec(c, depth+1)
		}
	}
	rec(s.root, 0)
	return b.String()
}
