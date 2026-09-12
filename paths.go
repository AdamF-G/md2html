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
// path naming a Markdown document contributes its directory; every other
// path is taken to be a directory already, since the entry points it is
// given are either directories or Markdown files.
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

// isUnder reports whether path is base or lives beneath it. Both must be
// absolute paths; results are undefined for relative paths. Paths are
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

// isExcluded reports whether path is at or beneath any of the excluded
// directory prefixes. Both path and every prefix must be absolute.
//
// This reuses isUnder rather than comparing strings, so "/tree/vendored"
// is not treated as living under "/tree/vendor" — a string-prefix check
// would exclude a sibling directory whose name merely starts the same way.
func isExcluded(path string, excluded []string) bool {
	for _, e := range excluded {
		if isUnder(path, e) {
			return true
		}
	}
	return false
}

// outputPath maps a source document to its generated HTML location.
// Both src and base must be absolute paths.
// An empty outDir means in-place: the HTML sits beside its source.
//
// Note a.md and a.markdown in one directory both map to a.html, as does a
// real document under base/_external colliding with one pulled in from
// outside base. Crawl checks the finished emit set for two documents
// sharing an output path and refuses the run, so the parallel emit never
// races on one.
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
	// Sanitize by removing leading .. and . segments to prevent escaping outDir.
	trimmed := strings.TrimPrefix(htmlName, string(filepath.Separator))
	trimmed = strings.TrimPrefix(trimmed, filepath.VolumeName(htmlName))
	trimmed = strings.TrimPrefix(trimmed, string(filepath.Separator))

	// Drop leading .. and . path segments.
	parts := strings.Split(trimmed, string(filepath.Separator))
	i := 0
	for i < len(parts) && (parts[i] == ".." || parts[i] == ".") {
		i++
	}
	trimmed = strings.Join(parts[i:], string(filepath.Separator))

	return filepath.Join(outDir, externalDir, trimmed)
}
