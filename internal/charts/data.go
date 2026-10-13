package charts

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// DataLoader returns the loader a chart's data url is read through. It reads
// url relative to pageDir, and refuses any url with a scheme or host, any
// url with a query or fragment, any path outside root, and any symlink
// that resolves outside root.
func DataLoader(root, pageDir string) func(string) (string, error) {
	return func(u string) (string, error) {
		refuse := func(why string) (string, error) {
			return "", fmt.Errorf("chart data %q: %s", u, why)
		}
		p, err := url.Parse(u)
		if err != nil || p.Scheme != "" || p.Host != "" || strings.HasPrefix(u, "//") {
			return refuse("md2html reads only files beside the page, never the network")
		}
		// A file has no query or fragment. Reading the path alone would
		// drop them without a word.
		if p.RawQuery != "" || p.Fragment != "" {
			return refuse("a file has no query or fragment; remove the part from the ? or #")
		}
		if filepath.IsAbs(p.Path) {
			return refuse("an absolute path is outside the pages")
		}
		target := filepath.Join(pageDir, filepath.FromSlash(p.Path))
		resolved, err := filepath.EvalSymlinks(target)
		if err != nil {
			return refuse(err.Error())
		}
		rootReal, err := filepath.EvalSymlinks(root)
		if err != nil {
			return refuse(err.Error())
		}
		if rel, err := filepath.Rel(rootReal, resolved); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return refuse("it is outside " + root)
		}
		b, err := os.ReadFile(resolved)
		if err != nil {
			return refuse(err.Error())
		}
		return string(b), nil
	}
}
