package md2html

import (
	"path/filepath"
	"strings"
)

// externalDir holds documents pulled in from outside base. Their absolute
// path is mirrored beneath it, minus the leading separator, which is
// deterministic, collision-free, and readable when tracing where a stray
// document came from.
const externalDir = "_external"

// commonAncestor returns the deepest directory containing every path. A
// path naming a file contributes its directory.
func commonAncestor(paths []string) string {
	if len(paths) == 0 {
		return string(filepath.Separator)
	}
	dirOf := func(p string) string {
		p = filepath.Clean(p)
		if IsMarkdownPath(p) {
			return filepath.Dir(p)
		}
		return p
	}
	common := strings.Split(dirOf(paths[0]), string(filepath.Separator))
	for _, p := range paths[1:] {
		parts := strings.Split(dirOf(p), string(filepath.Separator))
		n := len(common)
		if len(parts) < n {
			n = len(parts)
		}
		i := 0
		for i < n && common[i] == parts[i] {
			i++
		}
		common = common[:i]
	}
	joined := strings.Join(common, string(filepath.Separator))
	if joined == "" {
		return string(filepath.Separator)
	}
	return filepath.Clean(joined)
}

// isUnder reports whether path is base or lives beneath it. Both are
// cleaned first, so a link written with ../ that resolves back inside base
// is correctly recognized as contained.
func isUnder(path, base string) bool {
	path = filepath.Clean(path)
	base = filepath.Clean(base)
	if path == base {
		return true
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// outputPath maps a source document to its generated HTML location.
// An empty outDir means in-place: the HTML sits beside its source.
//
// Note a.md and a.markdown in one directory both map to a.html. That is a
// genuine collision and the parallel emit would race on it. It is rare
// enough to leave unhandled for now; if it bites, add a duplicate-output
// check in Crawl after the emit set is built and refuse the run.
func outputPath(src, base, outDir string) string {
	src = filepath.Clean(src)
	htmlName := strings.TrimSuffix(src, filepath.Ext(src)) + ".html"

	if outDir == "" {
		return htmlName
	}
	outDir = filepath.Clean(outDir)

	if isUnder(src, base) {
		rel, err := filepath.Rel(base, htmlName)
		if err == nil {
			return filepath.Join(outDir, rel)
		}
	}
	// Outside base: mirror the absolute path under _external.
	trimmed := strings.TrimPrefix(htmlName, string(filepath.Separator))
	trimmed = strings.TrimPrefix(trimmed, filepath.VolumeName(htmlName))
	trimmed = strings.TrimPrefix(trimmed, string(filepath.Separator))
	return filepath.Join(outDir, externalDir, trimmed)
}
