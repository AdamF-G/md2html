package md2html

import (
	"regexp"
	"strings"
	"testing"
)

func toc(t *testing.T, in string) string {
	t.Helper()
	root, err := parseFragment([]byte(in))
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	for _, tr := range []Transform{Chips(), HeadingAnchors(), TOC()} {
		if err := tr.Fn(root); err != nil {
			t.Fatalf("%s: %v", tr.Name, err)
		}
	}
	out, err := renderTree(root)
	if err != nil {
		t.Fatalf("renderTree: %v", err)
	}
	return string(out)
}

func TestTOCReplacesMarkerWithNav(t *testing.T) {
	got := toc(t, `<h1>Doc</h1><p>[[toc]]</p><h2>First</h2><h2>Second</h2>`)
	if !strings.Contains(got, `<nav class="toc"`) {
		t.Errorf("no nav\ngot: %s", got)
	}
	if strings.Contains(got, "[[toc]]") {
		t.Errorf("marker left in the body\ngot: %s", got)
	}
	if !strings.Contains(got, `<a href="#first">First</a>`) ||
		!strings.Contains(got, `<a href="#second">Second</a>`) {
		t.Errorf("headings missing from nav\ngot: %s", got)
	}
}

// Flat, not nested: an irregular heading-level jump must not be able to
// produce broken list nesting. Level is carried as a class instead.
func TestTOCIsFlatAndCarriesLevelAsAClass(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><h2>A</h2><h4>B</h4>`)
	if strings.Count(got, "<ol") != 1 {
		t.Errorf("nested lists\ngot: %s", got)
	}
	if !strings.Contains(got, `class="toc-h2 `) || !strings.Contains(got, `class="toc-h4 `) {
		t.Errorf("no level classes\ngot: %s", got)
	}
}

// tocDepths returns each entry's depth class, with " title" appended for
// the entry marked as the page title, in order.
func tocDepths(t *testing.T, in string) []string {
	t.Helper()
	var out []string
	for _, m := range regexp.MustCompile(`<li class="toc-h\d (toc-d\d)( toc-title)?">`).FindAllStringSubmatch(toc(t, in), -1) {
		out = append(out, m[1]+strings.Replace(m[2], " toc-", " ", 1))
	}
	return out
}

// Depth is the entry's place in the outline below the page title, not its
// heading tag. The title is the first entry when it is shallower than every
// other; it is marked, and counts for nothing, so the sections under it are
// depth 0 as they would be on a page with no title. Past that, an entry is
// one deeper than the nearest heading above it with a higher level, so a
// level skipped for its look adds no depth.
func TestTOCDepthFollowsTheOutlineNotTheTag(t *testing.T) {
	cases := []struct {
		name, in string
		want     []string
	}{
		{"title and sections", `<p>[[toc]]</p><h1>T</h1><h2>A</h2><h3>B</h3><h2>C</h2>`,
			[]string{"toc-d0 title", "toc-d0", "toc-d1", "toc-d0"}},
		{"skipped level", `<p>[[toc]]</p><h1>T</h1><h2>A</h2><h4>B</h4><h4>C</h4>`,
			[]string{"toc-d0 title", "toc-d0", "toc-d1", "toc-d1"}},
		{"climbs back out", `<p>[[toc]]</p><h1>T</h1><h2>A</h2><h4>B</h4><h3>C</h3><h4>D</h4><h2>E</h2>`,
			[]string{"toc-d0 title", "toc-d0", "toc-d1", "toc-d1", "toc-d2", "toc-d0"}},
		{"title one level down", `<p>[[toc]]</p><h2>Title</h2><h3>A</h3><h4>B</h4>`,
			[]string{"toc-d0 title", "toc-d0", "toc-d1"}},
		{"no title, sections only", `<p>[[toc]]</p><h2>A</h2><h3>B</h3><h2>C</h2>`,
			[]string{"toc-d0", "toc-d1", "toc-d0"}},
		{"two top headings, no title", `<p>[[toc]]</p><h1>A</h1><h2>B</h2><h1>C</h1>`,
			[]string{"toc-d0", "toc-d1", "toc-d0"}},
		{"shallower later, no title", `<p>[[toc]]</p><h3>A</h3><h2>B</h2><h3>C</h3>`,
			[]string{"toc-d0", "toc-d0", "toc-d1"}},
		{"deepest", `<p>[[toc]]</p><h1>1</h1><h2>2</h2><h3>3</h3><h4>4</h4><h5>5</h5><h6>6</h6>`,
			[]string{"toc-d0 title", "toc-d0", "toc-d1", "toc-d2", "toc-d3", "toc-d4"}},
	}
	for _, c := range cases {
		if got := tocDepths(t, c.in); strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: depths %v, want %v", c.name, got, c.want)
		}
	}
}

// Link text must match what the reader sees, which means without chips.
func TestTOCExcludesChipsFromLinkText(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><h2>Rollback [proven]</h2>`)
	if !strings.Contains(got, `<a href="#rollback">Rollback</a>`) {
		t.Errorf("chip text leaked into the toc\ngot: %s", got)
	}
}

// A document with no marker must come out byte-identical.
func TestTOCWithoutMarkerIsANoOp(t *testing.T) {
	in := `<h1>Doc</h1><h2>First</h2>`
	root, err := parseFragment([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if err := TOC().Fn(root); err != nil {
		t.Fatal(err)
	}
	out, err := renderTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Errorf("got %s, want %s", out, in)
	}
}

// The marker only counts alone on its own line. Inside a sentence it is
// prose, and inside a code fence it is documentation of this feature.
func TestTOCIgnoresMarkerInProseAndCode(t *testing.T) {
	got := toc(t, `<p>write [[toc]] to get one</p><pre><code>[[toc]]</code></pre><h2>A</h2>`)
	if strings.Contains(got, "<nav") {
		t.Errorf("expanded a marker that was not alone on its line\ngot: %s", got)
	}
}

// The heading the marker sits under is itself a heading; excluding nothing
// keeps the rule simple, and the page title reads fine as the first entry.
func TestTOCIncludesEveryHeadingInOrder(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><h1>Doc</h1><h2>A</h2><h3>B</h3>`)
	iDoc := strings.Index(got, `href="#doc"`)
	iA := strings.Index(got, `href="#a"`)
	iB := strings.Index(got, `href="#b"`)
	if iDoc < 0 || iA < iDoc || iB < iA {
		t.Errorf("entries out of document order\ngot: %s", got)
	}
}

// a marker in a document with no linkable headings must degrade to
// the literal marker text, not to an empty <nav> and not to silent
// deletion — the spec's stated degradation for this feature is "ugly, but
// not misleading," and an empty element is exactly the misleading case the
// global no-empty-element constraint rules out. The marker staying put is
// the reader's only clue that something needs fixing.
func TestTOCWithNoHeadingsLeavesTheMarker(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><p>body</p>`)
	if strings.Contains(got, "<nav") {
		t.Errorf("emitted an empty nav\ngot: %s", got)
	}
	if !strings.Contains(got, "[[toc]]") {
		t.Errorf("marker was removed even though there was nothing to replace it with\ngot: %s", got)
	}
}

// headingText already excludes both chip spans and the anchor link
// HeadingAnchors appends, so a heading whose visible text genuinely ends in
// "#" must not be truncated by any hand-stripping in buildTOC.
func TestTOCPreservesTrailingHashInLabel(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><h2>Sharp C#</h2>`)
	if !strings.Contains(got, `<a href="#sharp-c">Sharp C#</a>`) {
		t.Errorf("trailing # truncated from label\ngot: %s", got)
	}
}

// A real link whose entire visible text happens to be "[[toc]]" must
// survive untouched. Matching on flattened text (textOf) cannot tell this
// apart from the bare marker; matching on the paragraph having a single
// *text-node* child can, since here the paragraph's only child is an <a>
// element, not text.
func TestTOCLeavesRealLinkAlone(t *testing.T) {
	got := toc(t, `<p><a href="http://example.com">[[toc]]</a></p><h2>A</h2>`)
	if strings.Contains(got, "<nav") {
		t.Errorf("ate a real link because its rendered text matched the marker\ngot: %s", got)
	}
	if !strings.Contains(got, `<a href="http://example.com">[[toc]]</a>`) {
		t.Errorf("link was altered or removed\ngot: %s", got)
	}
}

// The single-text-node-child requirement also protects an inline code span
// whose sole content is "[[toc]]": it documents the feature, it does not
// invoke it.
func TestTOCLeavesCodeSpanAlone(t *testing.T) {
	got := toc(t, `<p><code>[[toc]]</code></p><h2>A</h2>`)
	if strings.Contains(got, "<nav") {
		t.Errorf("expanded a marker inside a code span\ngot: %s", got)
	}
	if !strings.Contains(got, `<code>[[toc]]</code>`) {
		t.Errorf("code span was altered or removed\ngot: %s", got)
	}
}

// Two markers in one document each get their own nav — the second is not
// left behind as a stray marker once the first has been expanded.
func TestTOCExpandsEveryMarker(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><h2>A</h2><p>[[toc]]</p><h2>B</h2>`)
	if strings.Count(got, `<nav class="toc"`) != 2 {
		t.Errorf("want two navs, one per marker\ngot: %s", got)
	}
	if strings.Contains(got, "[[toc]]") {
		t.Errorf("a marker was left behind\ngot: %s", got)
	}
}

// Headings can be present without ids — the --no-anchors shape, or simply
// HeadingAnchors not having run — which is a different case from no
// headings at all. TOC must still degrade to the literal marker rather
// than link to headings with nothing to link to.
func TestTOCWithHeadingsButNoIDsLeavesTheMarker(t *testing.T) {
	root, err := parseFragment([]byte(`<p>[[toc]]</p><h2>A</h2>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := TOC().Fn(root); err != nil {
		t.Fatal(err)
	}
	out, err := renderTree(root)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Contains(got, "<nav") {
		t.Errorf("emitted a nav despite no heading having an id\ngot: %s", got)
	}
	if !strings.Contains(got, "[[toc]]") {
		t.Errorf("marker was removed even though no heading had an id\ngot: %s", got)
	}
}

// The other spellings in the wild: [TOC] is Python-Markdown, MkDocs,
// Typora and StackEdit; [[_TOC_]] is GitLab. All name the same thing.
func TestTOCAlternateMarkers(t *testing.T) {
	for _, marker := range []string{"[[toc]]", "[TOC]", "[toc]", "[[TOC]]"} {
		got := convert(t, "# Doc\n\n"+marker+"\n\n## One\n", nil)
		if !strings.Contains(got, `<nav class="toc"`) {
			t.Errorf("%s did not produce a contents list\ngot: %s", marker, got)
		}
		if strings.Contains(got, marker) {
			t.Errorf("%s left in the body\ngot: %s", marker, got)
		}
	}
}

// The marker must still be the paragraph's entire content.
func TestTOCAlternateMarkerMidSentenceStaysLiteral(t *testing.T) {
	got := convert(t, "# Doc\n\nsee [TOC] here\n\n## One\n", nil)
	if strings.Contains(got, `<nav class="toc"`) {
		t.Errorf("mid-sentence marker produced a contents list\ngot: %s", got)
	}
}

// A marker inside a code span documents the feature and must stay literal.
func TestTOCAlternateMarkerInCodeStaysLiteral(t *testing.T) {
	got := convert(t, "# Doc\n\n`[TOC]`\n\n## One\n", nil)
	if strings.Contains(got, `<nav class="toc"`) {
		t.Errorf("marker in a code span produced a contents list\ngot: %s", got)
	}
}

// GitLab's [[_TOC_]] is deliberately not supported: the underscores are
// emphasis delimiters, so goldmark hands this transform a paragraph of
// "[[", <em>TOC</em>, "]]" rather than one text node. Recognizing it would
// mean matching on flattened text, which is exactly what the single-text-
// node guard exists to avoid — it would also match a link whose visible
// text happens to read that way. Pinned so the limitation is discoverable
// rather than surprising.
func TestTOCGitLabMarkerIsNotSupported(t *testing.T) {
	got := convert(t, "# Doc\n\n[[_TOC_]]\n\n## One\n", nil)
	if strings.Contains(got, `<nav class="toc"`) {
		t.Errorf("[[_TOC_]] unexpectedly produced a contents list\ngot: %s", got)
	}
}

// The contents list is a landmark, and an unnamed one is announced as just
// "navigation" — indistinguishable from a hand-written ::: nav on the same
// page. A label names it without adding a visible heading, which would
// change every page's look and itself need an anchor and a contents entry.
func TestTOCNavHasAnAccessibleName(t *testing.T) {
	got := toc(t, `<h1>Doc</h1><p>[[toc]]</p><h2>First</h2>`)
	if !strings.Contains(got, `<nav class="toc" aria-label="Table of Contents">`) {
		t.Errorf("contents list has no accessible name\ngot: %s", got)
	}
}

// toc-title is Pandoc's front matter key for the same label, so a page in
// another language can name its contents list in that language.
func TestTOCTitleFrontMatterSetsTheLabel(t *testing.T) {
	got := convert(t, "---\nlang: de\ntoc-title: Inhalt\n---\n# Doc\n\n[[toc]]\n\n## Eins\n", nil)
	if !strings.Contains(got, `<nav class="toc" aria-label="Inhalt">`) {
		t.Errorf("toc-title not applied\ngot: %s", got)
	}
	if strings.Contains(got, "Table of Contents") {
		t.Errorf("default label left behind\ngot: %s", got)
	}
}

// The label only lands on the generated list: a hand-written nav keeps
// whatever name its author gave it, or none.
func TestTOCTitleLeavesOtherNavsAlone(t *testing.T) {
	got := convert(t, "---\ntoc-title: Inhalt\n---\n# Doc\n\n::: nav {aria-label=\"Site\"}\n- [Home](./home.md)\n:::\n\n[[toc]]\n\n## Eins\n", nil)
	if !strings.Contains(got, `<nav aria-label="Site">`) {
		t.Errorf("hand-written nav's label changed\ngot: %s", got)
	}
}

// toc-title reaches the list TOC builds, not every nav that happens to
// carry the default label: a hand-written ::: nav {.toc} its author named
// "Table of Contents" keeps that name.
func TestTOCTitleDoesNotRenameAHandWrittenTocNav(t *testing.T) {
	got := convert(t, "---\ntoc-title: Inhalt\n---\n# Doc\n\n::: nav {.toc aria-label=\"Table of Contents\"}\n- [Home](./home.md)\n:::\n\n[[toc]]\n\n## Eins\n", nil)
	if strings.Count(got, `aria-label="Inhalt"`) != 1 || !strings.Contains(got, `aria-label="Table of Contents"`) {
		t.Errorf("hand-written .toc nav renamed\ngot: %s", got)
	}
}

// A caller who assembled their own transform list from Builtins() gets the
// page's toc-title too, as they get Options.Warn.
func TestTOCTitleReachesCallerSuppliedTransforms(t *testing.T) {
	out, err := Convert([]byte("---\ntoc-title: Inhalt\n---\n# Doc\n\n[[toc]]\n\n## Eins\n"),
		Options{Fragment: true, CSS: "/**/", Transforms: Builtins()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `aria-label="Inhalt"`) {
		t.Errorf("toc-title lost with a caller-supplied list\ngot: %s", out)
	}
}

// floatingNav is the opening tag of a list marked to float. Tests look for
// the tag, not the class name alone: a full page embeds the stylesheet,
// which names toc-float in its own rules.
const floatingNav = `<nav class="toc toc-float"`

// Float mode marks the list it builds, so the stylesheet can pin it beside
// the text column; the list inside is the same one inline mode builds.
func TestTOCFloatOptionMarksTheNav(t *testing.T) {
	out, err := Convert([]byte("# Doc\n\n[TOC]\n\n## Eins\n"), Options{TOC: "float"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `<nav class="toc toc-float" aria-label="Table of Contents">`) {
		t.Errorf("float mode did not mark the nav\ngot: %s", out)
	}
}

func TestTOCInlineIsTheDefault(t *testing.T) {
	for _, mode := range []string{"", "inline"} {
		out, err := Convert([]byte("# Doc\n\n[TOC]\n\n## Eins\n"), Options{TOC: mode})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), floatingNav) || !strings.Contains(string(out), `<nav class="toc"`) {
			t.Errorf("TOC %q: want a plain inline nav\ngot: %s", mode, out)
		}
	}
}

// Front matter is the document speaking for itself, so it wins over the
// run-wide option in both directions, the order lang already follows.
func TestTOCFrontMatterBeatsOptions(t *testing.T) {
	cases := []struct {
		name, src, opt string
		float          bool
	}{
		{"page opts out", "---\ntoc: inline\n---\n# Doc\n\n[TOC]\n\n## Eins\n", "float", false},
		{"page opts in", "---\ntoc: float\n---\n# Doc\n\n[TOC]\n\n## Eins\n", "inline", true},
	}
	for _, c := range cases {
		out, err := Convert([]byte(c.src), Options{TOC: c.opt})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(out), floatingNav); got != c.float {
			t.Errorf("%s: floating = %v, want %v", c.name, got, c.float)
		}
	}
}

// An unknown mode is a typo, not a layout: it is reported and the next
// source down is used.
func TestTOCInvalidModeFallsBackAndWarns(t *testing.T) {
	cases := []struct {
		name, src, opt string
		float          bool
	}{
		{"front matter falls to options", "---\ntoc: sidebar\n---\n# Doc\n\n[TOC]\n\n## Eins\n", "float", true},
		{"front matter falls to default", "---\ntoc: Float!\n---\n# Doc\n\n[TOC]\n\n## Eins\n", "", false},
		{"options falls to default", "# Doc\n\n[TOC]\n\n## Eins\n", "floating", false},
	}
	for _, c := range cases {
		var warns []string
		out, err := Convert([]byte(c.src), Options{TOC: c.opt, Warn: func(s string) { warns = append(warns, s) }})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(out), floatingNav); got != c.float {
			t.Errorf("%s: floating = %v, want %v", c.name, got, c.float)
		}
		if len(warns) != 1 || !strings.Contains(warns[0], "inline or float") {
			t.Errorf("%s: warnings = %v, want one naming inline or float", c.name, warns)
		}
	}
}

// Only one list can hold the fixed position; a second would sit on top of
// the first. Later markers stay inline.
func TestTOCFloatsOnlyTheFirstMarker(t *testing.T) {
	out, err := Convert([]byte("# Doc\n\n[TOC]\n\n## Eins\n\n[[toc]]\n\n## Zwei\n"), Options{TOC: "float"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Count(s, `<nav class="toc toc-float"`) != 1 || strings.Count(s, `<nav class="toc" `) != 1 {
		t.Errorf("want one floating and one inline nav\ngot: %s", s)
	}
}

// Fragments are published as Artifacts and never carry a script, so the
// side toggle could not exist there: the list stays inline.
func TestTOCFragmentStaysInline(t *testing.T) {
	out, err := Convert([]byte("---\ntoc: float\n---\n# Doc\n\n[TOC]\n\n## Eins\n"), Options{Fragment: true, TOC: "float"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), floatingNav) {
		t.Errorf("fragment floated its contents list\ngot: %s", out)
	}
}

// A caller who assembled their own transform list from Builtins() gets the
// float mode too, as they get toc-title.
func TestTOCFloatReachesCallerSuppliedTransforms(t *testing.T) {
	out, err := Convert([]byte("# Doc\n\n[TOC]\n\n## Eins\n"), Options{TOC: "float", Transforms: Builtins()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), floatingNav) {
		t.Errorf("float mode lost with a caller-supplied list\ngot: %s", out)
	}
}
