package md2html

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveRefs returns meta with the value of each of keys that has the form
// @path#key replaced by key's value in the front matter of the file at path,
// relative to dir. Only keys are looked at: a reference resolves where
// md2html reads the key as data (a [[list]] key, a {name} template, a search
// facet), and every other value stays as written, unwarned, so a page with
// no markers is unchanged by references existing. A value found is used as
// written: one that is itself a reference is not followed, so references
// cannot loop. A reference that cannot be resolved becomes empty, so its @
// text never reaches a page, and is warned about.
func resolveRefs(meta map[string]string, dir string, keys []string, warn func(string)) map[string]string {
	if meta == nil {
		return nil
	}
	say := func(format string, args ...any) {
		if warn != nil {
			warn(fmt.Sprintf(format, args...))
		}
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	for _, k := range keys {
		k = strings.ToLower(k)
		v, ok := meta[k]
		if !ok || !strings.HasPrefix(v, "@") {
			continue
		}
		file, key, ok := strings.Cut(v[1:], "#")
		if !ok || file == "" || key == "" {
			say("front matter %s %q is not @file#key; using it as written", k, v)
			continue
		}
		p := filepath.FromSlash(file)
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		src, err := os.ReadFile(p)
		if err != nil {
			say("front matter %s refers to %s, which cannot be read; leaving it empty", k, file)
			out[k] = ""
			continue
		}
		ref, _, _ := splitFrontMatter(src)
		val, found := ref[strings.ToLower(key)]
		switch {
		case !found:
			say("front matter %s refers to %s in %s, which has no such key; leaving it empty", k, key, file)
			out[k] = ""
		case strings.HasPrefix(val, "@"):
			say("front matter %s refers to %s#%s, which is itself a reference; not following it", k, file, key)
			out[k] = ""
		default:
			out[k] = val
		}
	}
	return out
}
