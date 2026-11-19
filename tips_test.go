package md2html

import (
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
	got := apply(t, "<p>x "+src+" y</p>", chips(func(m string) { w = append(w, m) }))
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
