package md2html

import (
	"regexp"
	"strings"
	"testing"
)

// A delay is written into CSS, so only a plain time gets through: the
// check is what keeps "0ms; background:red" out of a style.
func TestValidTipDelay(t *testing.T) {
	for _, v := range []string{"0ms", "150ms", "0.2s", "1s"} {
		if !validTipDelay(v) {
			t.Errorf("validTipDelay(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"", "ms", "150", ".5s", "0ms; background:red", "calc(1s)", "150 ms", "-1ms"} {
		if validTipDelay(v) {
			t.Errorf("validTipDelay(%q) = true, want false", v)
		}
	}
}

// chipsWarn renders src between two words through chips, collecting its
// warnings. It is transform-level, so writeTips never runs on it and the
// recorded attributes can be read.
func chipsWarn(t *testing.T, src string) (string, []string) {
	t.Helper()
	var w []string
	got := chipsMarkdown(t, "x "+src+" y\n", func(m string) { w = append(w, m) })
	return got, w
}

func linkWarn(t *testing.T, src string) (string, []string) {
	t.Helper()
	var w []string
	got := apply(t, "<p>"+src+"</p>", LinkAttrs(func(m string) { w = append(w, m) }))
	return got, w
}

func TestChipsSpanRecordsTip(t *testing.T) {
	got, _ := chipsWarn(t, `[beta]{.chip tip="May change."}`)
	if !strings.Contains(got, `md2html-tip="May change."`) || strings.Contains(got, ` tip=`) {
		t.Errorf("tip not recorded as md2html-tip\ngot: %s", got)
	}
}

func TestChipsSpanRecordsTipDelay(t *testing.T) {
	got, _ := chipsWarn(t, `[beta]{.chip tip="n" tip-delay="0ms"}`)
	if !strings.Contains(got, `md2html-tip-delay="0ms"`) {
		t.Errorf("delay not recorded\ngot: %s", got)
	}
}

func TestChipsEmptyTipIsNoTip(t *testing.T) {
	got, w := chipsWarn(t, `[beta]{.chip tip=""}`)
	if strings.Contains(got, "md2html-tip") || !strings.Contains(got, `<span class="chip">beta</span>`) || len(w) != 0 {
		t.Errorf("empty tip made a tip, or warned %q\ngot: %s", w, got)
	}
}

func TestChipsWarnOnBadTipDelay(t *testing.T) {
	got, w := chipsWarn(t, `[b]{.chip tip="n" tip-delay="0ms; background:red"}`)
	if len(w) != 1 || !strings.Contains(w[0], "tip-delay") || !strings.Contains(w[0], "background:red") {
		t.Errorf("warnings = %q, want one naming the value", w)
	}
	if strings.Contains(got, "md2html-tip-delay") || !strings.Contains(got, `md2html-tip="n"`) {
		t.Errorf("bad delay recorded, or tip lost\ngot: %s", got)
	}
}

func TestChipsWarnOnTipDelayWithoutTip(t *testing.T) {
	got, w := chipsWarn(t, `[b]{.chip tip-delay="0ms"}`)
	if len(w) != 1 || !strings.Contains(w[0], "without a tip") {
		t.Errorf("warnings = %q, want one", w)
	}
	if strings.Contains(got, "md2html-tip") {
		t.Errorf("recorded a delay without a tip\ngot: %s", got)
	}
}

// A title beside a tip stays: the element then has both tooltips, the
// author's choice.
func TestChipsTipKeepsTitle(t *testing.T) {
	got, _ := chipsWarn(t, `[b]{.chip title="t" tip="n"}`)
	if !strings.Contains(got, `title="t"`) || !strings.Contains(got, `md2html-tip="n"`) {
		t.Errorf("title or tip lost\ngot: %s", got)
	}
}

func TestLinkAttrsRecordTip(t *testing.T) {
	got, _ := linkWarn(t, `<a href="m.md">guide</a>{tip="Covers 0.x."}`)
	if !strings.Contains(got, `md2html-tip="Covers 0.x."`) || strings.Contains(got, "{") {
		t.Errorf("tip not recorded on the link\ngot: %s", got)
	}
}

// An image has alt, and a tip on an image inside a link would compete
// with the link's, so an image drops it like any unlisted name.
func TestLinkAttrsImageDropsTip(t *testing.T) {
	got, w := linkWarn(t, `<img src="i.png" alt="a">{tip="n"}`)
	if strings.Contains(got, "md2html-tip") || len(w) != 0 {
		t.Errorf("image kept a tip or warned %q\ngot: %s", w, got)
	}
}

func TestLinkAttrsWarnOnBadTipDelay(t *testing.T) {
	_, w := linkWarn(t, `<a href="m.md">g</a>{tip="n" tip-delay="soon"}`)
	if len(w) != 1 || !strings.Contains(w[0], "soon") {
		t.Errorf("warnings = %q, want one naming soon", w)
	}
}

// tipsOut converts src as a fragment, collecting its warnings.
func tipsOut(t *testing.T, src string) (string, []string) {
	t.Helper()
	var w []string
	return convert(t, src, func(m string) { w = append(w, m) }), w
}

func TestTipsSpanMarkup(t *testing.T) {
	got, _ := tipsOut(t, "[beta]{.chip tip=\"May change.\"}\n")
	want := `<span class="md2html-tip-wrap"><span class="chip md2html-tip-host" tabindex="0" aria-describedby="md2html-tooltip-1">beta</span><span class="md2html-tooltip" id="md2html-tooltip-1" role="tooltip" aria-hidden="true">May change.</span></span>`
	if !strings.Contains(got, want) {
		t.Errorf("want:\n%s\ngot:\n%s", want, got)
	}
}

// A link already takes focus, so it gets no tabindex; and the tooltip
// follows it rather than sitting inside it, where its text would become
// part of the link's name.
func TestTipsLinkMarkup(t *testing.T) {
	got, _ := tipsOut(t, "[guide](https://e.x){tip=\"n\"}\n")
	i := strings.Index(got, `<span class="md2html-tip-wrap">`)
	if i < 0 {
		t.Fatalf("no tip wrap\ngot: %s", got)
	}
	wrap := got[i:]
	a := wrap[:strings.Index(wrap, "</a>")+len("</a>")]
	if !strings.Contains(a, `class="md2html-tip-host"`) || !strings.Contains(a, `aria-describedby="md2html-tooltip-1"`) || strings.Contains(a, "tabindex") {
		t.Errorf("link host wrong: %s", a)
	}
	if !strings.HasPrefix(wrap[len(a):], `<span class="md2html-tooltip" id="md2html-tooltip-1"`) {
		t.Errorf("tooltip does not follow the link inside the wrap:\n%s", wrap)
	}
}

func TestTipsNumberInOrder(t *testing.T) {
	got, _ := tipsOut(t, "[a]{tip=\"1\"} [b]{tip=\"2\"} [c]{tip=\"3\"}\n")
	i1, i2, i3 := strings.Index(got, `id="md2html-tooltip-1"`), strings.Index(got, `id="md2html-tooltip-2"`), strings.Index(got, `id="md2html-tooltip-3"`)
	if i1 < 0 || !(i1 < i2 && i2 < i3) {
		t.Errorf("ids not 1, 2, 3 in order\ngot: %s", got)
	}
}

// An element's delay goes on the wrap: a custom property inherits only
// from an ancestor, and the tooltip is the host's sibling, so a delay on
// the host never reached it.
func TestTipsElementDelay(t *testing.T) {
	got, _ := tipsOut(t, "[b]{tip=\"n\" tip-delay=\"0ms\"}\n")
	if !strings.Contains(got, `<span class="md2html-tip-wrap" style="--md2html-tip-delay: 0ms">`) {
		t.Errorf("element delay not on the wrap\ngot: %s", got)
	}
}

func TestTipsElementDelayLeavesHostStyle(t *testing.T) {
	got, _ := tipsOut(t, "[b]{style=\"color: red\" tip=\"n\" tip-delay=\"0ms\"}\n")
	if !strings.Contains(got, `style="color: red"`) || strings.Contains(got, `color: red; --md2html`) {
		t.Errorf("author's style changed\ngot: %s", got)
	}
}

func TestTipsNoInternalAttributes(t *testing.T) {
	for _, src := range []string{
		"[beta]{.chip tip=\"May change.\"}\n",
		"[guide](https://e.x){tip=\"n\" tip-delay=\"0ms\"}\n",
		"[b]{tip=\"n\" tip-delay=\"0ms\"}\n",
	} {
		if got, _ := tipsOut(t, src); strings.Contains(got, "md2html-tip=") || strings.Contains(got, "md2html-tip-delay=") {
			t.Errorf("%q left an internal attribute\ngot: %s", src, got)
		}
	}
}

func TestTipsSpanInLinkDropsTip(t *testing.T) {
	// Chips never rewrites a link's text, so Markdown cannot put a tip
	// host inside a link; writeTips still refuses one it is handed.
	root, err := parseFragment([]byte(`<p><a href="u"><span md2html-tip="n">beta</span></a></p>`))
	if err != nil {
		t.Fatal(err)
	}
	writeTips(root)
	b, err := renderTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); strings.Contains(got, "md2html-tooltip") || strings.Contains(got, "md2html-tip=") || !strings.Contains(got, "beta") {
		t.Errorf("tip kept inside a link, or label lost\ngot: %s", got)
	}
}

// The tip is written after HeadingAnchors and TOC, so neither sees it.
func TestTipsInHeadingKeepSlugAndTOC(t *testing.T) {
	plain, _ := tipsOut(t, "# Doc\n\n[[toc]]\n\n## Rollback [x]{.chip} plan\n")
	got, _ := tipsOut(t, "# Doc\n\n[[toc]]\n\n## Rollback [x]{.chip tip=\"SECRET\"} plan\n")
	id := func(s string) string {
		i := strings.Index(s, "<h2 id=\"") + len("<h2 id=\"")
		return s[i : i+strings.Index(s[i:], "\"")]
	}
	if id(got) != id(plain) {
		t.Errorf("slug changed: %q, want %q", id(got), id(plain))
	}
	start, end := strings.Index(got, "<nav"), strings.Index(got, "</nav>")
	if start < 0 || end < start {
		t.Fatalf("no contents list\ngot: %s", got)
	}
	toc := got[start:end]
	if strings.Contains(toc, "SECRET") {
		t.Errorf("note in the contents list: %s", toc)
	}
}

// A site build unwraps a link it drops; the tip stays, on the text.
func TestTipsDroppedLinkKeepsTip(t *testing.T) {
	out, err := Convert([]byte("[g](x.md){tip=\"n\"}\n"), Options{Fragment: true, CSS: "/**/", LinkDrops: map[string]bool{"x.md": true}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Contains(got, "<a") || !strings.Contains(got, `<span class="md2html-tip-host" tabindex="0" aria-describedby="md2html-tooltip-1">g</span>`) {
		t.Errorf("dropped link lost its tip\ngot: %s", got)
	}
}

// styleOf returns the <style> element's contents of a converted page.
func styleOf(t *testing.T, page string) string {
	t.Helper()
	i, j := strings.Index(page, "<style>"), strings.Index(page, "</style>")
	if i < 0 || j < i {
		t.Fatalf("no <style> in:\n%s", page)
	}
	return page[i:j]
}

func convertOpt(t *testing.T, src string, opt Options) (string, []string) {
	t.Helper()
	var w []string
	opt.Warn = func(m string) { w = append(w, m) }
	out, err := Convert([]byte(src), opt)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), w
}

func TestTipCSSOnlyWithTips(t *testing.T) {
	for _, frag := range []bool{false, true} {
		with, _ := convertOpt(t, "[b]{tip=\"n\"}\n", Options{Fragment: frag})
		without, _ := convertOpt(t, "[b]{.chip}\n", Options{Fragment: frag})
		if !strings.Contains(styleOf(t, with), tipCSS) {
			t.Errorf("fragment=%v: a page with a tip lacks tip.css", frag)
		}
		if strings.Contains(styleOf(t, without), tipCSS) {
			t.Errorf("fragment=%v: a page without a tip has tip.css", frag)
		}
	}
}

// The rules that hide a tip ship with the page, not the style: without
// them the note would sit in the text as a second copy.
func TestTipCSSWithoutDefaultStyle(t *testing.T) {
	got, _ := convertOpt(t, "[b]{tip=\"n\"}\n", Options{CSS: "/**/"})
	if !strings.Contains(styleOf(t, got), tipCSS) {
		t.Error("tip.css missing when the caller replaces the stylesheet")
	}
}

func TestTipCSSHidesUntilHover(t *testing.T) {
	norm := strings.Join(strings.Fields(tipCSS), " ")
	for _, want := range []string{
		"visibility: hidden",
		".md2html-tip-host:hover + .md2html-tooltip",
		".md2html-tip-host:focus-visible + .md2html-tooltip",
		"visibility: visible",
		"var(--md2html-tip-delay, 100ms)",
		"prefers-reduced-motion",
		"@media print",
		`content: " ("`,
	} {
		if !strings.Contains(norm, want) {
			t.Errorf("tip.css lacks %q", want)
		}
	}
}

// The charts' tooltips are md2html-tip on SVG; nothing here may match them.
func TestTipCSSLeavesChartTipsAlone(t *testing.T) {
	if m := regexp.MustCompile(`\.md2html-tip[^-a-z]`).FindString(tipCSS); m != "" {
		t.Errorf("tip.css selects the charts' .md2html-tip: %q", m)
	}
}

func TestTipDelayFrontMatter(t *testing.T) {
	got, _ := convertOpt(t, "---\ntip-delay: 150ms\n---\n[b]{tip=\"n\"}\n", Options{})
	if !strings.Contains(styleOf(t, got), "main.md2html { --md2html-tip-delay: 150ms; }") {
		t.Errorf("front matter delay not set\n%s", styleOf(t, got))
	}
}

func TestTipDelayFrontMatterRefused(t *testing.T) {
	got, w := convertOpt(t, "---\ntip-delay: 0ms; background:red\n---\n[b]{tip=\"n\"}\n", Options{})
	if strings.Contains(styleOf(t, got), "--md2html-tip-delay:") {
		t.Errorf("refused delay reached the style\n%s", styleOf(t, got))
	}
	if len(w) != 1 || !strings.Contains(w[0], "background:red") {
		t.Errorf("warnings = %q, want one naming the value", w)
	}
}

// The look lives in tip.css, not default.css, so a page without a tip is
// unchanged (TestUnchangedOutput), and goes ahead of the style so a style
// can restyle it.
func TestTipCSSLook(t *testing.T) {
	norm := strings.Join(strings.Fields(tipCSS), " ")
	// :where() keeps the cue at no specificity, so any style's rule for a
	// chip wins; start aligns a right-to-left page's note to the right.
	for _, want := range []string{"var(--chart-surface", ":where(.md2html-tip-host:not(a)) { cursor: help; }", "text-align: start"} {
		if !strings.Contains(norm, want) {
			t.Errorf("tip.css lacks %q", want)
		}
	}
	if strings.Contains(norm, "text-align: left") {
		t.Error("tip.css aligns notes left, wrong for a right-to-left page")
	}
	if regexp.MustCompile(`\.md2html-tip-host[^{]*\{[^}]*text-decoration`).MatchString(tipCSS) {
		t.Error("tip.css underlines a tip host")
	}
	if strings.Contains(defaultCSS, "md2html-tooltip") {
		t.Error("default.css styles tips; every page would change")
	}
	got, _ := convertOpt(t, "[b]{tip=\"n\"}\n", Options{})
	style := styleOf(t, got)
	if strings.Index(style, tipCSS) > strings.Index(style, defaultCSS) {
		t.Error("tip.css comes after the style, which then cannot restyle it")
	}
}

// A front matter tip-delay that is not a time warns even on a page with
// no tip: the design note says such a value is ignored with a warning,
// and a page that gains a tip later should not be the first to hear.
func TestTipDelayFrontMatterWarnsWithoutTips(t *testing.T) {
	got, w := convertOpt(t, "---\ntip-delay: soon\n---\nNo tips here.\n", Options{})
	if len(w) != 1 || !strings.Contains(w[0], "soon") {
		t.Errorf("warnings = %q, want one naming soon", w)
	}
	if strings.Contains(got, "--md2html-tip-delay") {
		t.Error("a page with no tip got a delay rule")
	}
}

// A page's own id can take a tooltip's number: a heading "md2html
// tooltip 1" slugs to md2html-tooltip-1. The tooltip takes the next free
// one, so the host never describes itself with the heading.
func TestTipsSkipIDsThePageUses(t *testing.T) {
	got, _ := tipsOut(t, "## md2html tooltip 1\n\n[x]{tip=\"n\"}\n")
	if strings.Count(got, `id="md2html-tooltip-1"`) != 1 {
		t.Errorf("id md2html-tooltip-1 used twice\ngot: %s", got)
	}
	if !strings.Contains(got, `aria-describedby="md2html-tooltip-2"`) || !strings.Contains(got, `class="md2html-tooltip" id="md2html-tooltip-2"`) {
		t.Errorf("tooltip did not take the next free id\ngot: %s", got)
	}
}
