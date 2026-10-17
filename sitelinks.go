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
	shell, err := resolve(opt.SiteShell)
	if err != nil {
		return nil, fmt.Errorf("site shell: %w", err)
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
	if fi, err := os.Stat(target); err == nil && fi.IsDir() {
		return linkAction{} // a directory is not a file to copy or link
	}
	if l.Kind == LinkDoc {
		if out, ok := outBySrc[target]; ok {
			return linkAction{rewrite: relHref(fromOut, out) + fragmentOf(l.Href)}
		}
		if isUnder(target, r.base) {
			return linkAction{} // in the site, but not built: left as written
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
	return linkAction{rewrite: relHref(fromOut, out) + fragmentOf(l.Href), copy: &Asset{Src: target, Out: out}}
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
