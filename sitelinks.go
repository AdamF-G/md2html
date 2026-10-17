package md2html

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// siteRules is what a site build does with the links its pages and shell
// hold: which files are in the site, where copies go, what a link out of
// the site becomes, and the source root paths out of it are measured from.
// A build without a shell, or one written in place, has none, and does what
// it always did.
type siteRules struct {
	shell      string // the shell, absolute and with its symlinks resolved
	base       string // a file under it is in the site
	sourceRoot string // _assets/ and repo-url: paths are measured from it
	outDir     string
	repoURL    string // with a {path} placeholder
	unmounted  string // link, unlink or error
	noAssets   bool   // copy nothing, and leave what would be copied as written
	excluded   exclusions
	mounts     []mountRule       // a mounted site's mounts; nil without mounts
	dirs       map[string]bool   // site directories that hold a page, by site path
	pages      map[string]string // a page's site path without its extension, to its output
}

// indexPages records the site's pages from its docs: each by its site path
// without the extension, and every directory that holds one, the root
// included. Every such directory has a page at its index.html: its own, a
// README written there, or the one BuildSite writes.
func (r *siteRules) indexPages(docs []Doc) {
	r.pages = map[string]string{}
	r.dirs = map[string]bool{}
	for _, d := range docs {
		if d.SitePath == "" {
			continue
		}
		r.pages[strings.TrimSuffix(d.SitePath, path.Ext(d.SitePath))] = d.Out
		for dir := sitePathDir(d.SitePath); ; dir = sitePathDir(dir) {
			r.dirs[dir] = true
			if dir == "" {
				break
			}
		}
	}
}

// inSite reads a link a reader follows as a site path, relative to the site
// directory fromSite ("" is the root), and finds the page it names: the
// absolute output, and how a report names the page. That is the path
// without its extension for a page, the directory with a trailing slash for
// a directory's page, and "the site's root" for the root's. A link that is
// absolute, climbs above the root, names no page, or is an asset a page
// loads finds nothing, as does any link in a site without mounts.
func (r *siteRules) inSite(l Link, fromSite string) (out, at string, ok bool) {
	if r == nil || r.mounts == nil || l.Loaded {
		return "", "", false
	}
	h := l.Href
	if i := strings.IndexAny(h, "#?"); i >= 0 {
		h = h[:i]
	}
	h, err := url.PathUnescape(h)
	if err != nil || h == "" || strings.HasPrefix(h, "/") {
		return "", "", false
	}
	dir := strings.HasSuffix(h, "/")
	p := path.Join(fromSite, h)
	if p == ".." || strings.HasPrefix(p, "../") {
		return "", "", false
	}
	if p == "." {
		p = ""
	}
	if dir || path.Ext(p) == "" {
		if !r.dirs[p] {
			return "", "", false
		}
		if p == "" {
			return filepath.Join(r.outDir, "index.html"), "the site's root", true
		}
		return filepath.Join(r.outDir, filepath.FromSlash(p), "index.html"), p + "/", true
	}
	if !IsMarkdownPath(p) {
		return "", "", false
	}
	stem := strings.TrimSuffix(p, path.Ext(p))
	if o, found := r.pages[stem]; found {
		return o, stem, true
	}
	if isIndexStem(sitePathStem(p)) {
		d := sitePathDir(p)
		if r.dirs[d] {
			if d == "" {
				return filepath.Join(r.outDir, "index.html"), "the site's root", true
			}
			return filepath.Join(r.outDir, filepath.FromSlash(d), "index.html"), d + "/", true
		}
	}
	return "", "", false
}

// mountRule is a mount with its own excludes resolved.
type mountRule struct {
	*Mount
	excluded exclusions
}

// owner is the mount holding target: a file mount of exactly target, else
// the directory mount whose source holds it most deeply, so a deeper mount
// takes its files out of an enclosing one. Nil when no mount holds it.
func (r *siteRules) owner(target string) *mountRule {
	var best *mountRule
	for i := range r.mounts {
		mr := &r.mounts[i]
		if !mr.IsDir {
			if mr.Source == target {
				return mr
			}
			continue
		}
		if isUnder(target, mr.Source) && (best == nil || len(mr.Source) > len(best.Source)) {
			best = mr
		}
	}
	return best
}

// place is where target sits in the site, as a slash path from the site's
// root, and whether the site holds it at all: under the base without
// mounts, or held by a mount, at the mount's path.
func (r *siteRules) place(target string) (string, bool) {
	if r.mounts == nil {
		return relUnder(r.base, target)
	}
	mr := r.owner(target)
	if mr == nil {
		return "", false
	}
	if !mr.IsDir {
		if mr.Path == "" {
			return "index" + path.Ext(target), true
		}
		return mr.Path + path.Ext(target), true
	}
	rel, ok := relUnder(mr.Source, target)
	if !ok {
		return "", false
	}
	return path.Join(mr.Path, rel), true
}

// excludedAt reports whether target is excluded: by --exclude, or by the
// excludes of the mount holding it.
func (r *siteRules) excludedAt(target string) bool {
	if r.excluded.match(target) {
		return true
	}
	mr := r.owner(target)
	return mr != nil && mr.excluded.match(target)
}

// newSiteRules reads a site's link settings from its shell's front matter.
// It returns nil, and no error, for a build that is not a site's or that
// writes in place, and an error for a setting the build cannot honour.
func newSiteRules(opt CrawlOptions, base string, excluded exclusions) (*siteRules, error) {
	if opt.SiteShell == "" || opt.OutDir == "" {
		return nil, nil
	}
	shell, err := resolve(opt.SiteShell)
	if err != nil {
		return nil, fmt.Errorf("site shell: %w", err)
	}
	raw, err := os.ReadFile(shell)
	if err != nil {
		return nil, fmt.Errorf("site shell: %w", err)
	}
	meta, _, _ := splitFrontMatter(raw)
	r := &siteRules{shell: shell, base: base, outDir: opt.OutDir, repoURL: meta["repo-url"],
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
		paths := []string{shell, base}
		if len(opt.Mounts) > 0 {
			paths = []string{shell}
			for _, m := range opt.Mounts {
				paths = append(paths, m.Source)
			}
		}
		r.sourceRoot, err = defaultSourceRoot(paths...)
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

// defaultSourceRoot is the deepest directory holding every path given: the
// shell and the run's base, or the shell and every mount's source. For a
// repository's documentation that is the repository's root. When it is
// the filesystem's root, every path measured from it would carry the
// building machine's own, so it is refused.
func defaultSourceRoot(paths ...string) (string, error) {
	root := commonAncestor(paths)
	if filepath.Dir(root) == root {
		return "", fmt.Errorf("the site's source root would be the filesystem's root %s, since the shell and the pages share no other directory; set source-root: in the shell or --source-root", root)
	}
	return root, nil
}

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
// shell written to fromOut, from the site directory fromSite. outBySrc maps
// each page built to its output.
//
// A page in the site is linked where it is written. Any other file in the
// site, under the base, is copied to the same place under the output
// directory, whether the page loads it or a reader follows it. A file the
// page loads from outside the site is copied to _assets/, at its path from
// the source root, or dropped when it has none. A link a reader follows
// out of the site, to a page or any other file, goes where unmounted: says.
// A link a reader follows to a missing file is read as a site path, and
// goes to the page it names when the site has one. Otherwise a missing
// file, and one under an excluded path, keep the link as written; in a
// mounted site an excluded file is outside the site instead, unless the
// site has a page at its path.
func (r *siteRules) action(l Link, fromOut, fromSite string, outBySrc map[string]string) linkAction {
	if l.Kind != LinkDoc && l.Kind != LinkAsset {
		return linkAction{}
	}
	target, err := resolve(l.Abs)
	if err != nil {
		return linkAction{}
	}
	// Without mounts an excluded file keeps its link as written, since
	// the crawl said it did not follow it. A mounted crawl follows no
	// links, so there it is outside the site, as any file no mount holds.
	excluded := r.excludedAt(target)
	if excluded && r.mounts == nil {
		return linkAction{}
	}
	fi, err := os.Stat(target)
	if excluded || err != nil {
		// An excluded file or directory is outside the site, and nothing
		// on disk is no file: a link a reader follows may still name a
		// page by its site path, which it then reaches.
		if out, _, ok := r.inSite(l, fromSite); ok {
			return linkAction{rewrite: relHref(fromOut, out) + fragmentOf(l.Href)}
		}
	}
	if err != nil {
		return linkAction{} // the crawl warned
	}
	if fi.IsDir() {
		// A link a reader follows to a directory the site holds reaches
		// the directory's page. In a mounted site one it does not hold is
		// outside the site; any other directory is left as written.
		if l.Loaded {
			return linkAction{}
		}
		if excluded {
			return r.leaving(l, target, fromSite)
		}
		if out, ok := r.dirPage(target); ok {
			return linkAction{rewrite: relHref(fromOut, out) + fragmentOf(l.Href)}
		}
		if _, held := r.place(target); !held && r.mounts != nil {
			return r.leaving(l, target, fromSite)
		}
		return linkAction{}
	}
	if excluded {
		if l.Kind == LinkDoc || !l.Loaded {
			return r.leaving(l, target, fromSite)
		}
		return r.outsideLoaded(l, target, fromOut)
	}
	if l.Kind == LinkDoc {
		if out, ok := outBySrc[target]; ok {
			return linkAction{rewrite: relHref(fromOut, out) + fragmentOf(l.Href)}
		}
		if _, ok := r.place(target); ok {
			return linkAction{} // in the site, but not built: left as written
		}
		return r.leaving(l, target, fromSite)
	}
	if sp, ok := r.place(target); ok {
		if r.noAssets {
			return linkAction{}
		}
		out := filepath.Join(r.outDir, filepath.FromSlash(sp))
		return linkAction{rewrite: relHref(fromOut, out) + fragmentOf(l.Href), copy: &Asset{Src: target, Out: out}}
	}
	if !l.Loaded {
		return r.leaving(l, target, fromSite)
	}
	return r.outsideLoaded(l, target, fromOut)
}

// outsideLoaded is what a file a page loads from outside the site becomes:
// a copy under _assets/, at its path from the source root, or dropped when
// it has none.
func (r *siteRules) outsideLoaded(l Link, target, fromOut string) linkAction {
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
	return linkAction{rewrite: relHref(fromOut, out) + fragmentOf(l.Href), copy: &Asset{Src: target, Out: out}}
}

// leaving is what a link a reader follows out of the site becomes. The
// root of the source is a directory too, and fills {path} with nothing.
// fromSite, the site directory of the page holding the link, is for the
// clause that names a site page the file shadows.
func (r *siteRules) leaving(l Link, target, fromSite string) linkAction {
	switch r.unmounted {
	case "error":
		return linkAction{fail: fmt.Sprintf("links to %s, which is outside the site", l.Href)}
	case "link":
		if rel, ok := relUnder(r.sourceRoot, target); ok {
			if rel == "." {
				rel = ""
			}
			return linkAction{rewrite: strings.ReplaceAll(r.repoURL, "{path}", encodePath(rel)) + fragmentOf(l.Href)}
		}
		return linkAction{drop: true, warn: fmt.Sprintf("unlinked %s: it is outside the source root %s, so repo-url: has no path for it; its text is kept", l.Href, r.sourceRoot)}
	}
	return linkAction{drop: true, warn: fmt.Sprintf("unlinked %s, which is outside the site; its text is kept", l.Href)}
}

// dirPage is the output of the page of the directory target, when the site
// holds the directory and it holds a page: every such directory's page is
// at its index.html.
func (r *siteRules) dirPage(target string) (string, bool) {
	sp, ok := r.place(target)
	if !ok {
		return "", false
	}
	if sp == "." {
		sp = ""
	}
	if !r.dirs[sp] {
		return "", false
	}
	return filepath.Join(r.outDir, filepath.FromSlash(sp), "index.html"), true
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
