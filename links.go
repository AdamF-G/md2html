package md2html

import (
	"net/url"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// LinkKind classifies a link target.
type LinkKind int

const (
	// LinkDoc is a local Markdown document.
	LinkDoc LinkKind = iota
	// LinkAsset is any other local file: images, diagrams, downloads.
	LinkAsset
	// LinkRemote is an off-machine or inline URL, left untouched.
	LinkRemote
	// LinkFragment is a same-document anchor, left untouched.
	LinkFragment
)

// Link is one href or src found in a document.
type Link struct {
	// Href is the value exactly as written in the source.
	Href string
	Kind LinkKind
	// Abs is the resolved absolute path, for LinkDoc and LinkAsset only.
	// Any #fragment has been stripped.
	Abs string
	// Node and Attr locate the link so a transform can rewrite it.
	Node *html.Node
	Attr string
}

// markdownExts are the extensions treated as documents rather than assets.
var markdownExts = map[string]bool{".md": true, ".markdown": true}

// IsMarkdownPath reports whether p names a Markdown document.
func IsMarkdownPath(p string) bool {
	return markdownExts[strings.ToLower(filepath.Ext(p))]
}

// isRemote reports whether an href addresses something not on local disk.
func isRemote(href string) bool {
	if strings.HasPrefix(href, "//") {
		return true
	}
	for _, scheme := range []string{"http:", "https:", "data:", "mailto:", "tel:", "ftp:"} {
		if strings.HasPrefix(strings.ToLower(href), scheme) {
			return true
		}
	}
	return false
}

// linkAttrFor returns the attribute carrying a link for the given element,
// or "" if the element does not carry one.
func linkAttrFor(n *html.Node) string {
	switch n.DataAtom {
	case atom.A, atom.Link:
		return "href"
	case atom.Img, atom.Script, atom.Source, atom.Video, atom.Audio, atom.Iframe, atom.Embed:
		return "src"
	}
	return ""
}

// ExtractLinks returns every local or remote link in the tree. srcDir is the
// absolute directory of the document being examined, used to resolve
// relative hrefs.
func ExtractLinks(root *html.Node, srcDir string) []Link {
	var out []Link
	walk(root, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		key := linkAttrFor(n)
		if key == "" {
			return
		}
		href, ok := attr(n, key)
		if !ok || href == "" {
			return
		}

		l := Link{Href: href, Node: n, Attr: key}
		switch {
		case strings.HasPrefix(href, "#"):
			l.Kind = LinkFragment
		case isRemote(href):
			l.Kind = LinkRemote
		default:
			// Strip any fragment: ./guide.md#setup addresses ./guide.md.
			pathPart := href
			if i := strings.IndexByte(pathPart, '#'); i >= 0 {
				pathPart = pathPart[:i]
			}
			if i := strings.IndexByte(pathPart, '?'); i >= 0 {
				pathPart = pathPart[:i]
			}
			if pathPart == "" {
				l.Kind = LinkFragment
				break
			}
			// goldmark percent-encodes hrefs, so "./my doc.md" renders as
			// "./my%20doc.md". Decode before touching the filesystem; Href
			// keeps the encoded form, which is what LinkMap is keyed by.
			if decoded, decErr := url.PathUnescape(pathPart); decErr == nil {
				pathPart = decoded
			}
			if filepath.IsAbs(pathPart) {
				l.Abs = filepath.Clean(pathPart)
			} else {
				l.Abs = filepath.Clean(filepath.Join(srcDir, pathPart))
			}
			if IsMarkdownPath(pathPart) {
				l.Kind = LinkDoc
			} else {
				l.Kind = LinkAsset
			}
		}
		out = append(out, l)
	})
	return out
}
