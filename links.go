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
	// Kind classifies the link target.
	Kind LinkKind
	// Abs is the resolved absolute path, for LinkDoc and LinkAsset only.
	// Any #fragment has been stripped.
	Abs string
	// Node is the element carrying the link, so a transform can rewrite it.
	Node *html.Node
	// Attr is the attribute on Node carrying the link: "href", "src",
	// "srcset" (Href is then one candidate's URL) or "poster".
	Attr string
	// Loaded is true for a link the page loads, an image, script,
	// stylesheet or frame, and false for one a reader follows, an <a>.
	Loaded bool
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

// linkAttrsFor returns the attributes carrying links on the given element,
// in the order they are read, or nil when it carries none.
func linkAttrsFor(n *html.Node) []string {
	switch n.DataAtom {
	case atom.A, atom.Link:
		return []string{"href"}
	case atom.Img, atom.Source:
		return []string{"src", "srcset"}
	case atom.Video:
		return []string{"src", "poster"}
	case atom.Script, atom.Audio, atom.Iframe, atom.Embed:
		return []string{"src"}
	}
	return nil
}

// srcsetCandidate is one image candidate of an srcset: its URL and its
// width or density descriptor, if any.
type srcsetCandidate struct{ url, desc string }

// srcsetURLs splits an srcset into its candidates, following the HTML
// standard's parse of a srcset attribute: whitespace and commas between
// candidates are skipped, a candidate's URL is the run of non-whitespace
// after them, and its descriptor runs to the next comma outside parentheses.
// A comma inside a URL, as in a data: URL, does not end the candidate.
func srcsetURLs(v string) []srcsetCandidate {
	var out []srcsetCandidate
	isSpace := func(c byte) bool {
		return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
	}
	pos := 0
	for pos < len(v) {
		for pos < len(v) && (isSpace(v[pos]) || v[pos] == ',') {
			pos++
		}
		if pos >= len(v) {
			break
		}
		start := pos
		for pos < len(v) && !isSpace(v[pos]) {
			pos++
		}
		url := v[start:pos]
		if strings.HasSuffix(url, ",") {
			url = strings.TrimRight(url, ",")
			if url != "" {
				out = append(out, srcsetCandidate{url: url})
			}
			continue
		}
		// Descriptor: up to the next comma outside parentheses.
		depth := 0
		descStart := pos
		for pos < len(v) {
			c := v[pos]
			if c == '(' {
				depth++
			} else if c == ')' && depth > 0 {
				depth--
			} else if c == ',' && depth == 0 {
				break
			}
			pos++
		}
		desc := strings.Join(strings.Fields(v[descStart:pos]), " ")
		if pos < len(v) {
			pos++ // consume the comma ending this candidate
		}
		if url != "" {
			out = append(out, srcsetCandidate{url: url, desc: desc})
		}
	}
	return out
}

// joinSrcset writes candidates back as an srcset value.
func joinSrcset(cs []srcsetCandidate) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = c.url
		if c.desc != "" {
			parts[i] += " " + c.desc
		}
	}
	return strings.Join(parts, ", ")
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
		loaded := n.DataAtom != atom.A
		for _, key := range linkAttrsFor(n) {
			v, ok := attr(n, key)
			if !ok || v == "" {
				continue
			}
			hrefs := []string{v}
			if key == "srcset" {
				hrefs = hrefs[:0]
				for _, c := range srcsetURLs(v) {
					hrefs = append(hrefs, c.url)
				}
			}
			for _, href := range hrefs {
				out = append(out, classifyLink(Link{Href: href, Node: n, Attr: key, Loaded: loaded}, srcDir))
			}
		}
	})
	return out
}

// classifyLink sets l's Kind, and its Abs for a local file.
func classifyLink(l Link, srcDir string) Link {
	href := l.Href
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
	return l
}
