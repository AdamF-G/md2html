package md2html

import (
	"encoding/base64"
	"fmt"
	"path"
	"strings"
)

// assetTypes are the files a style may inline, by extension. Fonts and
// images are what a stylesheet references; anything else in a url() is
// more likely a mistake than an asset, so it is refused rather than guessed.
var assetTypes = map[string]string{
	".woff2": "font/woff2",
	".woff":  "font/woff",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
}

// inlineAssets replaces every url() in css that names a file in the plugin
// with a data: URI of that file, so a page stays one file and fetches
// nothing: mermaid stays the only thing a page can load, and fragment
// output stays publishable as an Artifact, which allows no asset host.
//
// cssPath is the stylesheet's own path in the plugin, which relative URLs
// resolve against, as a browser would. read fetches a file by its path in
// the plugin and refuses one outside it.
//
// It scans rather than matching a pattern because the text "url(" can
// appear where it is not a URL, in a comment or a content string, and a
// comment noting an old CDN address must not fail the load. A fragment
// reference such as url(#shadow) names an element in the page, not a file,
// so it passes through, as does a data: URL already inline.
func inlineAssets(css, cssPath string, read func(rel string) ([]byte, error)) (string, error) {
	var b strings.Builder
	for i := 0; i < len(css); {
		rest := css[i:]
		switch {
		case strings.HasPrefix(rest, "/*"):
			end := strings.Index(rest[2:], "*/")
			n := len(rest)
			if end >= 0 {
				n = 2 + end + 2
			}
			b.WriteString(rest[:n])
			i += n
		case rest[0] == '"' || rest[0] == '\'':
			n := stringEnd(rest)
			b.WriteString(rest[:n])
			i += n
		case hasPrefixFold(rest, "@import"):
			return "", fmt.Errorf("%s: @import is not allowed; add the file to the style's css list instead", cssPath)
		case hasPrefixFold(rest, "url(") && (i == 0 || !isIdentByte(css[i-1])):
			end := strings.IndexByte(rest, ')')
			if end < 0 {
				return "", fmt.Errorf("%s: unclosed url(", cssPath)
			}
			raw := strings.TrimSpace(rest[4:end])
			ref := strings.Trim(raw, `"'`)
			out, err := inlineURL(ref, cssPath, read)
			if err != nil {
				return "", err
			}
			if out == "" {
				b.WriteString(rest[:end+1])
			} else {
				b.WriteString(`url("` + out + `")`)
			}
			i += end + 1
		default:
			b.WriteByte(css[i])
			i++
		}
	}
	return b.String(), nil
}

// inlineURL returns the data: URI for one url() reference, or "" for a
// reference left as written.
func inlineURL(ref, cssPath string, read func(string) ([]byte, error)) (string, error) {
	if strings.HasPrefix(ref, "#") || hasPrefixFold(ref, "data:") {
		return "", nil
	}
	if strings.HasPrefix(ref, "/") || strings.Contains(strings.SplitN(ref, "/", 2)[0], ":") {
		return "", fmt.Errorf("%s: url(%s) is not a relative path inside the plugin; a plugin cannot load anything at view time", cssPath, ref)
	}
	file, frag, _ := strings.Cut(ref, "#")
	file, _, _ = strings.Cut(file, "?")
	rel := path.Clean(path.Join(path.Dir(cssPath), file))
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("%s: url(%s) is outside the plugin", cssPath, ref)
	}
	mime, ok := assetTypes[strings.ToLower(path.Ext(rel))]
	if !ok {
		return "", fmt.Errorf("%s: url(%s): %q is not a font or image type md2html can inline", cssPath, ref, path.Ext(rel))
	}
	data, err := read(rel)
	if err != nil {
		return "", fmt.Errorf("%s: url(%s): %w", cssPath, ref, err)
	}
	uri := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	if frag != "" {
		uri += "#" + frag
	}
	return uri, nil
}

// stringEnd returns the length of the CSS string literal s starts with,
// including both quotes, or len(s) if it never closes.
func stringEnd(s string) int {
	q := s[0]
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case q:
			return i + 1
		}
	}
	return len(s)
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// isIdentByte reports whether c can be part of a CSS identifier, so that
// "my-url(" is not taken for "url(".
func isIdentByte(c byte) bool {
	return c == '-' || c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
