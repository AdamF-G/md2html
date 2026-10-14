// Package md2html converts Markdown documents to HTML with tree-level
// transforms that reach hand-written raw HTML as well as generated markup.
package md2html

import (
	"bytes"
	"fmt"
	"path/filepath"

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
	// CSS replaces the stylesheet outright for every page. A page's own
	// "style:" front matter still overrides it. Setting both CSS and Styles
	// is an error.
	CSS string
	// Plugins are the directories of local plugins every page can draw
	// styles from, beside the built-in ones, resolved against the working
	// directory. A page's own "plugins:" front matter adds to them for that
	// page's "style:" only.
	//
	// Convert loads these without reporting their warnings: a run loads
	// them once up front, as the CLI does, and hears them there, instead of
	// once per page.
	Plugins []string
	// Styles selects the run's styles by name, stacked in order: a bare
	// style name, or plugin/style where two plugins share one. A page's own
	// "style:" front matter replaces the whole list. Empty means the
	// default style.
	Styles []string
	// PluginCache keeps loaded plugins and assembled stylesheets across
	// conversions. A caller converting many documents shares one, so a
	// plugin's fonts are read and inlined once rather than once per page.
	// Nil loads afresh on every call.
	PluginCache *PluginCache
	// StyleChosen, if set, is called once per Convert after the page's
	// style is chosen from Styles or its own "style:" front matter, with
	// the styles' qualified names (plugin/style), the directory of each
	// style's plugin ("" for a built-in), and the stylesheet they
	// assemble. Two plugins can share a name, so the dirs tell them apart.
	// It is not called when the page gets the default stylesheet by
	// default, or CSS. The CLI checks each list's colours here. A caller
	// converting pages in parallel shares it, so it must be safe for
	// concurrent use.
	StyleChosen func(names, dirs []string, css string)
	// MermaidURL is the ES module a page containing a mermaid diagram
	// imports at view time. Empty uses the pinned CDN build.
	//
	// It is the only thing a generated page ever fetches, so this is the
	// knob for a docs build that must not reach a CDN: point it at a copy
	// you serve yourself. Nothing else changes — a page with no diagram
	// still imports nothing, and Fragment output imports nothing unless
	// PreviewRuntime asks, because Artifacts render mermaid themselves.
	MermaidURL string
	// PreviewRuntime adds to Fragment output, when it has a mermaid diagram,
	// the runtime a full page loads from MermaidURL, so the fragment can be
	// previewed in a browser outside the host that would draw it. Output
	// meant for that host should leave it off: the host renders mermaid
	// itself, in its own theme. A full page ignores it.
	PreviewRuntime bool
	// Lang is the page's language, written as <html lang>. A document's own
	// "lang:" front matter key overrides it; empty means "en". A value not
	// shaped like a BCP 47 language tag is warned about and skipped in
	// favor of the next source. Fragment output has no <html> element, so
	// it ignores both.
	Lang string
	// TOC is how a contents list is laid out: "inline" (the default) where
	// its marker stands, or "float", pinned beside the text column on a wide
	// screen with a control to move it to the other side. A document's own
	// "toc:" front matter key overrides it; any other value is warned about
	// and skipped in favor of the next source. Only the first list on a page
	// floats, and Fragment output is always inline: an Artifact carries no
	// script, so it could not offer the side control.
	TOC string
	// AutoTOC gives pages a contents list without a [[toc]] marker: "all"
	// for every page, "long" for a page of more than 1000 source lines or
	// more than 5 headings. The list goes after the page's title block, and
	// a page that already has a marker keeps its own, and a page whose front
	// matter says "toc: none" gets none. Setting it makes
	// "float" the TOC default. Empty adds none; any other value is warned
	// about and ignored.
	AutoTOC string
	// NoSource leaves the original Markdown out of the page. By default a
	// full page carries it, byte for byte and front matter included, in a
	// hidden element with the id SourceID, so whoever receives the HTML can
	// hand the source to their own agent or convert it again. Turn it off
	// when the source holds something the rendered page leaves out and that
	// should not travel with it, such as a front matter key the page does
	// not use. Fragment output never carries the source.
	NoSource bool
	// NoCharts leaves a vega-lite fence as a code block. By default each
	// one renders as a chart: static SVG drawn when the page is built, in
	// the page style's colours and face, with a hover layer in CSS and a
	// table of its data. Turn it off when a caller does not want the chart
	// engine started.
	NoCharts bool
	// DataRoot is the directory a chart's data files must stay inside. A
	// chart's data url resolves against the source document's directory,
	// and one that leads outside DataRoot is refused. Empty means the
	// source document's directory. Without a SourcePath there is no
	// directory to read from, and a chart's data url is refused.
	DataRoot string
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
	// missing the definition list it is defined to hold, a fig fence
	// whose body does not parse or does not validate, and a vega-lite fence
	// that does not render or that Vega-Lite warns about.
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

	// Site, when set, wraps the page in the site's shell and gives its
	// markers the site's tree (see BuildSite). SourcePath must be the page's
	// source as the crawl resolved it. Fragment output ignores it.
	Site *Site
}

// newParser builds the goldmark instance. Every extension here is required
// by the conformance fixture; see docs/specs.
//
// warn is handed to the fenced-code renderer, which needs it for the `fig`
// and `vega-lite` languages: a malformed figure or chart degrades to a code
// block and says why. It may be nil. charts is how that renderer draws
// charts; its zero value, with off unset and no style, is not usable, so a
// caller that wants none sets off.
func newParser(warn func(string), charts chartContext) goldmark.Markdown {
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
			renderer.WithNodeRenderers(util.Prioritized(newCodeFenceRenderer(warn, charts), 100)),
		),
	)
}

// Convert renders Markdown to HTML, running transforms over the parsed tree.
func Convert(src []byte, opt Options) ([]byte, error) {
	// Front matter comes off the bytes: a "---" block is already valid
	// Markdown, so by the time a tree exists it has become an <hr> and a
	// setext heading, with no way back to the key/value lines.
	//
	// Only title/subtitle/date/title-block/lang/toc/toc-title/plugins/style are read below. A block carrying some other
	// flat key (an "author:" line, say) is still accepted and still
	// stripped from the body — it parsed as valid front matter — but the
	// key itself is silently dropped: there is nowhere in the page for it
	// to go yet, and warning about a key the format doesn't forbid would
	// be noise, not a diagnostic.
	// The page embeds the file as written, so keep it before front matter
	// comes off. Copied into a non-nil slice: nil tells renderPage to embed
	// nothing, and an empty document still has a source, if an empty one.
	var source []byte
	if !opt.Fragment && !opt.NoSource {
		source = append([]byte{}, src...)
	}
	meta, src, malformed := splitFrontMatter(src)
	if malformed && opt.Warn != nil {
		opt.Warn("front matter block is not flat key: value; rendering it as body text")
	}
	// Whether the site's shell will wrap this page. Decided this early
	// because the contents list depends on it.
	wrapping := opt.Site != nil && !opt.Fragment && meta["shell"] != "none"

	// The style comes before rendering: a chart is drawn in the page's
	// face and colours.
	// The shell's style is the site's: a page without a style of its own
	// takes it, with the shell's plugins found from the shell. A page's own
	// style still wins, and --style applies where neither sets one.
	styleOpt, styleMeta := opt, meta
	if opt.Site != nil && !opt.Fragment && meta["style"] == "" && opt.Site.shellMeta["style"] != "" {
		styleOpt.SourcePath = opt.Site.shellSrc
		styleOpt.Warn = opt.Site.shellWarn // a problem with it is the shell's
		styleMeta = map[string]string{"style": opt.Site.shellMeta["style"], "plugins": opt.Site.shellMeta["plugins"]}
		// The page's plugins widen only its own style:, as without a site.
		if meta["plugins"] != "" && opt.Warn != nil {
			opt.Warn("front matter plugins has no effect without style; ignoring it")
		}
	}
	style, err := choosePageStyle(styleOpt, styleMeta)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := newParser(opt.Warn, newChartContext(opt, style)).Convert(src, &buf); err != nil {
		return nil, err
	}
	root, err := parseFragment(buf.Bytes())
	if err != nil {
		return nil, err
	}
	// The navigation markers need a site's tree, which a fragment ignores.
	// Said once per marker, and left as written, the degradation [[toc]]
	// has with nothing to list.
	if (opt.Site == nil || opt.Fragment) && opt.Warn != nil {
		for _, m := range findMarkers(root) {
			if navMarkerNames[m.name] {
				opt.Warn(fmt.Sprintf("[[%s]] needs a site shell (--site); leaving it as written", m.name))
			}
		}
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
	switch tb := meta["title-block"]; tb {
	case "above":
		raiseDocMeta(root)
	case "", "below":
	default:
		if opt.Warn != nil {
			opt.Warn(fmt.Sprintf("front matter title-block %q is not above or below; the subtitle and date stay below the title", tb))
		}
	}

	transforms := opt.Transforms
	if transforms == nil {
		transforms = builtins(opt.Warn)
	} else if opt.Warn != nil {
		transforms = withWarn(transforms, opt.Warn)
	}
	// Ahead of every other transform: a marker's URL must still be the plain
	// autolink GFM made it, and the links it expands to then get the same
	// treatment as any others (external-link marking, for one).
	transforms = append([]Transform{listMarkers(meta, filepath.Dir(opt.SourcePath), opt.Warn)}, transforms...)
	// Resolved even for a fragment, so a bad value is still reported; a
	// fragment then stays inline, since it can carry no side control.
	tocMode := opt.TOC
	if opt.AutoTOC != "" && !IsAutoTOCMode(opt.AutoTOC) {
		if opt.Warn != nil {
			opt.Warn(fmt.Sprintf("AutoTOC option %q is not all or long; ignoring it", opt.AutoTOC))
		}
	} else if opt.AutoTOC != "" && tocMode == "" {
		tocMode = "float"
	}
	// "toc: none" opts one page out of AutoTOC. It names no layout, so it
	// goes no further: a marker the author wrote still gets the run's.
	frontTOC := meta["toc"]
	optOut := frontTOC == "none"
	if optOut {
		frontTOC = ""
	}
	// Placed before the front matter check below, which it satisfies.
	var autoMarker *html.Node
	if !optOut && !(wrapping && opt.Site.shellTOC) && wantsAutoTOC(opt.AutoTOC, src, root) {
		autoMarker = insertTOCMarker(root)
	}
	float := tocFloats(frontTOC, tocMode, opt.Warn) && !opt.Fragment
	// A floating list is placed against a column centred in the viewport,
	// which a site's layout does not promise, so under a shell every
	// contents list is inline where its marker is. Said only when the page
	// asked: --toc float is refused with --site, and AutoTOC's float is
	// only a default.
	if wrapping && float {
		if meta["toc"] == "float" && opt.Warn != nil {
			opt.Warn(`front matter toc "float" has no effect under a site shell; the contents list is inline`)
		}
		float = false
	}
	// A front matter toc value only sets layout for a marker that already
	// exists; on its own it renders nothing, and silently — the mismatch
	// (front matter says the page has a contents list, the page doesn't)
	// is exactly what warnings are for. Checked here, before the "toc"
	// transform below has a chance to remove any marker present.
	if opt.Warn != nil && IsTOCMode(meta["toc"]) && len(findTOCMarkers(root)) == 0 && !(wrapping && opt.Site.shellTOC) {
		opt.Warn(fmt.Sprintf("front matter toc %q has no [[toc]] or [TOC] marker in the document; it has no effect", meta["toc"]))
	}
	transforms = withTOC(transforms, meta["toc-title"], float)
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

	// Still in the tree only when there was nothing to list, or a caller's
	// own transforms had no TOC to replace it.
	if autoMarker != nil && autoMarker.Parent != nil {
		autoMarker.Parent.RemoveChild(autoMarker)
	}

	section, wrapped := "", false
	if opt.Site != nil && !opt.Fragment {
		section = opt.Site.section(opt.SourcePath)
		switch v := meta["shell"]; v {
		case "none":
		default:
			if v != "" && opt.Warn != nil {
				opt.Warn(fmt.Sprintf("front matter shell %q is not none; wrapping the page in the site's shell", v))
			}
			if root, err = opt.Site.wrap(root, opt.SourcePath, opt.Transforms, meta["toc-title"], opt.Warn); err != nil {
				return nil, err
			}
			wrapped = true
		}
	}

	body, err := renderTree(root)
	if err != nil {
		return nil, err
	}
	css := style.css
	mermaidURL := opt.MermaidURL
	if mermaidURL == "" {
		mermaidURL = mermaidCDN
	}
	if opt.Fragment {
		runtime := ""
		if opt.PreviewRuntime && hasMermaid(body) {
			runtime = mermaidRuntime(mermaidURL)
		}
		return renderFragment(body, title, css, runtime), nil
	}
	return renderPage(body, title, pageLang(meta["lang"], opt.Lang, opt.Warn), section, css, mermaidURL, source, wrapped), nil
}
