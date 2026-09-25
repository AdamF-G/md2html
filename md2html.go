// Package md2html converts Markdown documents to HTML with tree-level
// transforms that reach hand-written raw HTML as well as generated markup.
package md2html

import (
	"bytes"
	"fmt"

	fences "github.com/AdamF-G/md2html/internal/fences"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	goldhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/mermaid"
	"golang.org/x/net/html"
)

// Transform mutates a parsed HTML tree in place. Fn receives the synthetic
// root node whose children are the document's top-level elements.
type Transform struct {
	// Name identifies the transform, e.g. for logging or diagnostics.
	Name string
	// Fn is the function applied to the parsed HTML tree.
	Fn func(*html.Node) error
}

// Options controls a single document conversion.
type Options struct {
	// Fragment emits Artifact shape (marker, title, style, body) instead of
	// a full HTML document.
	Fragment bool
	// Title overrides the derived title. Empty means derive from the first
	// <h1>, falling back to SourcePath's base name.
	Title string
	// SourcePath is the absolute path of the source document. Used for the
	// title fallback and diagnostics.
	SourcePath string
	// CSS replaces the embedded default stylesheet. Empty uses the default.
	CSS string
	// MermaidURL is the ES module a page containing a mermaid diagram
	// imports at view time. Empty uses the pinned CDN build.
	//
	// It is the only thing a generated page ever fetches, so this is the
	// knob for a docs build that must not reach a CDN: point it at a copy
	// you serve yourself. Nothing else changes — a page with no diagram
	// still imports nothing, and Fragment output never imports at all,
	// because Artifacts render mermaid themselves.
	MermaidURL string
	// Lang is the page's language, written as <html lang>. A document's own
	// "lang:" front matter key overrides it; empty means "en". A value not
	// shaped like a BCP 47 language tag is warned about and skipped in
	// favor of the next source. Fragment output has no <html> element, so
	// it ignores both.
	Lang string
	// Transforms to run. Nil means the default list.
	//
	// A list supplied here is used as given, except that any builtin in it
	// that reports diagnostics is rebuilt against Warn — so appending to
	// Builtins() keeps every warning a default conversion would have
	// raised, without the caller naming the sink twice. The caller's own
	// slice is never written to.
	//
	// Transforms and LinkMap are two alternative routes to link rewriting,
	// not complementary ones: either place LinkRewrite in Transforms
	// yourself, or set LinkMap and let Convert do it. Doing both rewrites
	// every href twice — see LinkMap.
	Transforms []Transform
	// LinkMap maps an href exactly as written in the source to its
	// replacement. Populated by the crawler; nil for standalone conversion.
	//
	// Setting this makes Convert append LinkRewrite(LinkMap) to the
	// transform list itself, so callers must not also put LinkRewrite in
	// Transforms. Two passes over one map corrupt output whenever a
	// replacement is itself a key: a document linking both ./a.md (mapped
	// to a.html) and an existing ./a.html asset (mapped to the original
	// file) would have the first rewrite turned into the second.
	LinkMap map[string]string
	// Warn, when non-nil, receives one message per non-fatal problem found
	// while converting this document. Things that report so far: a front
	// matter block that is not flat key: value, a language that is not a
	// language tag, a container naming a kind that does not exist or
	// missing the definition list it is defined to hold, and a fig fence
	// whose body does not parse or does not validate.
	//
	// Convert never writes to stderr itself: it is a library, and the CLI
	// emits every document's output in parallel, so a transform printing
	// directly would interleave with other documents' lines. The callback
	// is invoked synchronously on the calling goroutine, so a caller may
	// append to an unsynchronized per-document slice.
	//
	// Every source reaches it by the same route whatever Transforms holds:
	// Convert raises the front matter warning itself, the fig fence warning
	// comes from the renderer, and a transform that reports is rebuilt
	// against this sink before it runs (see Transforms).
	Warn func(string)
}

// newParser builds the goldmark instance. Every extension here is required
// by the conformance fixture; see docs/specs.
//
// warn is handed to the fenced-code renderer, which needs it for the `fig`
// language: a malformed figure degrades to a code block and says why. It
// may be nil.
func newParser(warn func(string)) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.DefinitionList,
			extension.Footnote,
			// NoScript: artifacts render mermaid natively, so the extension
			// must not inject its own MermaidJS <script> tag.
			&mermaid.Extender{RenderMode: mermaid.RenderModeClient, NoScript: true},
			&fences.Extender{SplitInfo: parseContainerInfo},
		),
		goldmark.WithParserOptions(parser.WithAttribute()),
		goldmark.WithRendererOptions(
			goldhtml.WithUnsafe(),
			// Priority below goldmark's own (1000) so this wins for
			// fenced code blocks; every other node keeps the default
			// renderer.
			renderer.WithNodeRenderers(util.Prioritized(newCodeFenceRenderer(warn), 100)),
		),
	)
}

// Convert renders Markdown to HTML, running transforms over the parsed tree.
func Convert(src []byte, opt Options) ([]byte, error) {
	// Front matter comes off the bytes: a "---" block is already valid
	// Markdown, so by the time a tree exists it has become an <hr> and a
	// setext heading, with no way back to the key/value lines.
	//
	// Only title/subtitle/date/lang are read below. A block carrying some other
	// flat key (an "author:" line, say) is still accepted and still
	// stripped from the body — it parsed as valid front matter — but the
	// key itself is silently dropped: there is nowhere in the page for it
	// to go yet, and warning about a key the format doesn't forbid would
	// be noise, not a diagnostic.
	meta, src, malformed := splitFrontMatter(src)
	if malformed && opt.Warn != nil {
		opt.Warn("front matter block is not flat key: value; rendering it as body text")
	}

	var buf bytes.Buffer
	if err := newParser(opt.Warn).Convert(src, &buf); err != nil {
		return nil, err
	}
	root, err := parseFragment(buf.Bytes())
	if err != nil {
		return nil, err
	}

	// Before transforms: HeadingAnchors would otherwise contribute its "#"
	// anchor text to the derived title.
	explicitTitle := opt.Title != "" || meta["title"] != ""
	title := opt.Title
	if title == "" {
		title = meta["title"]
	}
	if title == "" {
		title = extractTitle(root, opt.SourcePath)
	}

	// Also before transforms, so the subtitle paragraph is an ordinary part
	// of the tree by the time anything walks it. The italic-line lift is a
	// fallback for documents that declare neither a subtitle nor a title —
	// gating it on subtitle alone would let it eat an italic line of body
	// text out from under a document that only omitted a subtitle.
	subtitle := meta["subtitle"]
	if subtitle == "" && !explicitTitle {
		liftSubtitle(root)
	}
	applyDocMeta(root, subtitle, meta["date"])

	transforms := opt.Transforms
	if transforms == nil {
		transforms = builtins(opt.Warn)
	} else if opt.Warn != nil {
		transforms = withWarn(transforms, opt.Warn)
	}
	if opt.LinkMap != nil {
		// Full-slice expression: never append into the caller's array.
		transforms = append(transforms[:len(transforms):len(transforms)],
			LinkRewrite(opt.LinkMap))
	}
	for _, t := range transforms {
		if err := t.Fn(root); err != nil {
			return nil, fmt.Errorf("transform %s: %w", t.Name, err)
		}
	}

	body, err := renderTree(root)
	if err != nil {
		return nil, err
	}
	css := opt.CSS
	if css == "" {
		css = defaultCSS
	}
	if opt.Fragment {
		return renderFragment(body, title, css), nil
	}
	mermaidURL := opt.MermaidURL
	if mermaidURL == "" {
		mermaidURL = mermaidCDN
	}
	return renderPage(body, title, pageLang(meta["lang"], opt.Lang, opt.Warn), css, mermaidURL), nil
}
