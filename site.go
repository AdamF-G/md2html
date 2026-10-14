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
	root      *siteNode
	bySrc     map[string]*siteNode
	order     []*siteNode

	mu          sync.Mutex
	entries     []searchEntry   // the search index, in arrival order
	entryAt     map[string]int  // each entry's place in entries, by URL
	shellWarned map[string]bool // shell warnings already in Warnings
}

// siteNode is one place in the tree: a page, or a directory, which has a
// page when it holds an index.md.
type siteNode struct {
	name     string // file stem or directory name
	label    string
	section  string // slug of the top-level node holding it; "" at the root
	path     string // a directory's path, for warnings
	doc      *Doc
	isDir    bool
	hidden   bool
	hasOrder bool
	order    int
	parent   *siteNode
	children []*siteNode
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
	s.outBySrc = make(map[string]string, len(res.Docs))
	for _, d := range res.Docs {
		s.outBySrc[d.Src] = d.Out
	}
	s.buildTree(res.Docs)
	return s, nil
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
		n := &siteNode{name: path.Base(rel), isDir: true, parent: parent,
			path: filepath.Join(s.base, filepath.FromSlash(rel))}
		parent.children = append(parent.children, n)
		dirs[rel] = n
		return n
	}
	for i := range docs {
		d := &docs[i]
		rel, err := filepath.Rel(s.base, d.Src)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			s.warn(d.Src, fmt.Sprintf("is outside the site's root %s; it is written but has no place in the site's navigation", s.base))
			continue
		}
		rel = filepath.ToSlash(rel)
		dirRel := path.Dir(rel)
		if dirRel == "." {
			dirRel = ""
		}
		dir := dirFor(dirRel)
		stem := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
		n := dir
		if !strings.EqualFold(stem, "index") || dir.doc != nil {
			n = &siteNode{name: stem, parent: dir}
			dir.children = append(dir.children, n)
		}
		n.doc = d
		s.bySrc[d.Src] = n
		s.applyMeta(n, d)
	}
	var finish func(n *siteNode)
	finish = func(n *siteNode) {
		if n.isDir && n.doc == nil {
			n.label = n.name
			if n != s.root {
				s.warn(n.path, "has no index.md; the site lists it by its directory name")
			}
		}
		for _, c := range n.children {
			finish(c)
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
}

// lessNode orders siblings: those with a nav-order first, by it, then the
// rest by label, case-insensitively, with the name breaking ties so the
// order never depends on the crawl's.
func lessNode(a, b *siteNode) bool {
	if a.hasOrder != b.hasOrder {
		return a.hasOrder
	}
	if a.hasOrder && a.order != b.order {
		return a.order < b.order
	}
	if la, lb := strings.ToLower(a.label), strings.ToLower(b.label); la != lb {
		return la < lb
	}
	return a.name < b.name
}

// section returns the slug of the top-level node holding the page at src,
// or "" for the root page or a page the tree could not place.
func (s *Site) section(src string) string {
	if n := s.bySrc[src]; n != nil {
		return n.section
	}
	return ""
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
