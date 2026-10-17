package md2html

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Mount places a source in a site: a directory, whose Markdown files become
// pages below Path, or one Markdown file, which becomes the page at Path. A
// site's shell declares its mounts with [[mount]] markers, and a build adds
// more with --mount; see ReadMounts.
type Mount struct {
	// Path is where the source appears in the site, a slash path with no
	// leading or trailing slash; "" is the site's root.
	Path string
	// Source is the directory or .md file, absolute and symlink-resolved.
	Source string
	// IsDir reports whether Source is a directory.
	IsDir bool
	// Exclude lists directory prefixes and name globs, as
	// CrawlOptions.Exclude, relative to Source. A directory mount's only.
	Exclude []string
	// NavTitle, when set, is the mount's label in the site's tree.
	NavTitle string
	// NavOrder, when HasNavOrder, is the mount's place among its siblings.
	NavOrder    int
	HasNavOrder bool
	// NavSort, "links" or "label" when set, orders a directory mount's
	// children, and those of every directory below it that sets none.
	NavSort string
	// TOC is "shell" when the shell's [[toc]] is the mounted pages' only
	// contents list; "" or "page" leaves their own alone.
	TOC string
	// Title is the title of the mount's directory page when md2html writes
	// one.
	Title string
	// From says where the mount was declared, for messages.
	From string
}

// mountAttrsRe matches a --mount value: a marker's attributes and nothing
// else.
var mountAttrsRe = regexp.MustCompile(`^(?:\s*[a-z][a-z-]*="[^"]*")*\s*$`)

// ReadMounts returns the mounts a site declares: the [[mount]] markers in
// the shell at shell, whose sources are relative to the shell, then each
// --mount value in flags, written as a marker's attributes, whose sources
// are relative to the working directory. A shell and flags that declare
// none give none, and the site is built from its entries as before. Every
// mount is checked here, so a build with a bad one stops before it starts.
func ReadMounts(shell string, flags []string) ([]Mount, error) {
	shellAbs, err := resolve(shell)
	if err != nil {
		return nil, fmt.Errorf("site shell: %w", err)
	}
	raw, err := os.ReadFile(shellAbs)
	if err != nil {
		return nil, fmt.Errorf("site shell: %w", err)
	}
	_, body, _ := splitFrontMatter(raw)
	root, err := parseDoc(body)
	if err != nil {
		return nil, fmt.Errorf("site shell %s: %w", shell, err)
	}
	var mounts []Mount
	n := 0
	for _, m := range findMarkers(root) {
		if m.name != "mount" {
			continue
		}
		n++
		mt, err := parseMount(m.attrs, filepath.Dir(shellAbs), fmt.Sprintf("%s: [[mount]] %d", shell, n))
		if err != nil {
			return nil, err
		}
		mounts = append(mounts, mt)
	}
	if len(flags) > 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		for i, f := range flags {
			from := fmt.Sprintf("--mount %d", i+1)
			if !mountAttrsRe.MatchString(f) {
				return nil, fmt.Errorf("%s: %q is not key=\"value\" attributes, as a [[mount]] marker's are", from, f)
			}
			attrs := map[string]string{}
			for _, a := range markerAttrRe.FindAllStringSubmatch(f, -1) {
				attrs[a[1]] = a[2]
			}
			mt, err := parseMount(attrs, cwd, from)
			if err != nil {
				return nil, err
			}
			mounts = append(mounts, mt)
		}
	}
	if len(mounts) == 0 {
		return nil, nil
	}
	if err := checkMounts(mounts); err != nil {
		return nil, err
	}
	return mounts, nil
}

// parseMount reads one mount's attributes. dir is what a relative source is
// read from: the shell's directory for a marker, the working directory for
// a flag. from names the mount in messages.
func parseMount(attrs map[string]string, dir, from string) (Mount, error) {
	m := Mount{From: from}
	for k := range attrs {
		switch k {
		case "source", "path", "exclude", "nav-title", "nav-order", "nav-sort", "toc", "title":
		default:
			return m, fmt.Errorf("%s: unknown attribute %q", from, k)
		}
	}
	src := attrs["source"]
	if src == "" {
		return m, fmt.Errorf("%s: needs a source", from)
	}
	p := filepath.FromSlash(src)
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	abs, err := resolve(p)
	if err != nil {
		return m, fmt.Errorf("%s: source %s: %w", from, src, err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return m, fmt.Errorf("%s: source %s does not exist", from, src)
	}
	m.Source, m.IsDir = abs, fi.IsDir()
	if !m.IsDir && !IsMarkdownPath(abs) {
		return m, fmt.Errorf("%s: source %s is not a directory or a Markdown file", from, src)
	}
	sp := attrs["path"]
	if !m.IsDir {
		sp = strings.TrimSuffix(sp, ".md")
	}
	if sp != "" && (strings.HasPrefix(sp, "/") || strings.HasSuffix(sp, "/") || strings.Contains(sp, `\`) ||
		path.Clean(sp) != sp || sp == "." || sp == ".." || strings.HasPrefix(sp, "../")) {
		return m, fmt.Errorf("%s: path %q is not a site path: names joined by /, with no leading or trailing /, and no . or ..", from, attrs["path"])
	}
	m.Path = sp
	if v := attrs["exclude"]; v != "" {
		for _, e := range strings.Split(v, ",") {
			if e = strings.TrimSpace(e); e != "" {
				m.Exclude = append(m.Exclude, e)
			}
		}
	}
	m.NavTitle, m.Title = attrs["nav-title"], attrs["title"]
	if v, ok := attrs["nav-order"]; ok {
		i, err := strconv.Atoi(v)
		if err != nil {
			return m, fmt.Errorf("%s: nav-order %q is not a whole number", from, v)
		}
		m.NavOrder, m.HasNavOrder = i, true
	}
	switch v := attrs["nav-sort"]; v {
	case "", "links", "label":
		m.NavSort = v
	default:
		return m, fmt.Errorf("%s: nav-sort %q is not links or label", from, v)
	}
	switch v := attrs["toc"]; v {
	case "", "page", "shell":
		m.TOC = v
	default:
		return m, fmt.Errorf("%s: toc %q is not page or shell", from, v)
	}
	if !m.IsDir && (m.NavSort != "" || len(m.Exclude) > 0) {
		return m, fmt.Errorf("%s: nav-sort and exclude apply to a directory mount, and %s is a file", from, src)
	}
	return m, nil
}

// checkMounts checks what no one mount can: one root, no source mounted
// twice, and one value per setting for a directory two mounts share.
func checkMounts(mounts []Mount) error {
	var root *Mount
	bySource := map[string]*Mount{}
	byDir := map[string]map[string]recorded{}
	for i := range mounts {
		m := &mounts[i]
		if m.Path == "" {
			if root != nil {
				return fmt.Errorf("%s and %s both mount the site's root; give one of them a path", root.From, m.From)
			}
			root = m
		}
		if prev := bySource[m.Source]; prev != nil {
			return fmt.Errorf("%s and %s both mount %s", prev.From, m.From, m.Source)
		}
		bySource[m.Source] = m
		if !m.IsDir {
			continue
		}
		rec := byDir[m.Path]
		if rec == nil {
			rec = map[string]recorded{}
			byDir[m.Path] = rec
		}
		settings := []struct{ name, val string }{
			{"nav-title", m.NavTitle},
			{"nav-sort", m.NavSort},
			{"toc", m.TOC},
			{"title", m.Title},
			{"nav-order", ""},
		}
		if m.HasNavOrder {
			settings[4].val = strconv.Itoa(m.NavOrder)
		}
		for _, s := range settings {
			if s.val == "" {
				continue
			}
			prev, ok := rec[s.name]
			if !ok {
				rec[s.name] = recorded{val: s.val, from: m.From}
				continue
			}
			if prev.val != s.val {
				return fmt.Errorf("%s and %s both mount directory %s with different %s: %s and %s",
					prev.from, m.From, displaySitePath(m.Path), s.name, shownSetting(s.name, prev.val), shownSetting(s.name, s.val))
			}
		}
	}
	if root == nil {
		return fmt.Errorf("no mount is the site's root: give one mount no path")
	}
	return nil
}

// recorded is the first value a directory's mounts set for one setting, and
// the mount that set it.
type recorded struct{ val, from string }

// shownSetting formats a setting's value for a message: a whole number as
// is, a string quoted.
func shownSetting(name, val string) string {
	if name == "nav-order" {
		return val
	}
	return strconv.Quote(val)
}

// displaySitePath names a site path in a message.
func displaySitePath(p string) string {
	if p == "" {
		return "the site's root"
	}
	return p
}
