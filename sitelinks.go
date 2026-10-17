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
