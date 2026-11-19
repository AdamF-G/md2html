package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"

	md2html "github.com/AdamF-G/md2html"
)

// tipPage serves a page with two tipped chips: b shows at once, c after
// the page's 400ms.
func tipPage(t *testing.T) (context.Context, string) {
	t.Helper()
	page, err := md2html.Convert([]byte("---\ntip-delay: 400ms\n---\nFirst [b]{.chip tip=\"note b\" tip-delay=\"0ms\"} then [c]{.chip tip=\"note c\"} end.\n"), md2html.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return newBrowserCtx(t), serveGenerated(t, map[string][]byte{"index.html": page})
}

// tipVisibility reads the tooltip's computed visibility for the host
// whose text is label.
func tipVisibility(label string, out *string) chromedp.Action {
	return chromedp.Evaluate(`(() => {
		const host = [...document.querySelectorAll(".md2html-tip-host")].find(h => h.textContent === `+"`"+label+"`"+`);
		return getComputedStyle(document.getElementById(host.getAttribute("aria-describedby"))).visibility;
	})()`, out)
}

// hoverTip moves the mouse to the centre of the host whose text is label.
func hoverTip(label string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		var box struct{ X, Y float64 }
		if err := chromedp.Evaluate(`(() => {
			const host = [...document.querySelectorAll(".md2html-tip-host")].find(h => h.textContent === `+"`"+label+"`"+`);
			const r = host.getBoundingClientRect();
			return {X: r.x + r.width / 2, Y: r.y + r.height / 2};
		})()`, &box).Do(ctx); err != nil {
			return err
		}
		return input.DispatchMouseEvent(input.MouseMoved, box.X, box.Y).Do(ctx)
	})
}

func TestBrowserTipShowsOnHover(t *testing.T) {
	ctx, baseURL := tipPage(t)
	var before, after string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		tipVisibility("b", &before),
		hoverTip("b"),
		chromedp.Sleep(100*time.Millisecond),
		tipVisibility("b", &after),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if before != "hidden" || after != "visible" {
		t.Errorf("visibility before hover %q, after %q; want hidden, visible", before, after)
	}
}

// A keyboard user reaches the chip with Tab. :focus-visible needs focus
// emulation, since a headless page has no window focus.
func TestBrowserTipShowsOnFocus(t *testing.T) {
	ctx, baseURL := tipPage(t)
	var after, focused string
	if err := chromedp.Run(ctx,
		emulation.SetFocusEmulationEnabled(true),
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.KeyEvent("\t"),
		chromedp.Sleep(100*time.Millisecond),
		chromedp.Evaluate(`document.activeElement.textContent`, &focused),
		tipVisibility("b", &after),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if focused != "b" || after != "visible" {
		t.Errorf("Tab focused %q with its tip %q; want b, visible", focused, after)
	}
}

func TestBrowserTipWaitsForDelay(t *testing.T) {
	ctx, baseURL := tipPage(t)
	var early, late string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		hoverTip("c"),
		chromedp.Sleep(100*time.Millisecond),
		tipVisibility("c", &early),
		chromedp.Sleep(600*time.Millisecond),
		tipVisibility("c", &late),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if early != "hidden" || late != "visible" {
		t.Errorf("visibility 100ms in %q, 700ms in %q; want hidden, visible", early, late)
	}
}

// A reader copying the paragraph gets its text, not the hidden notes.
func TestBrowserTipCopyLeavesNoteOut(t *testing.T) {
	ctx, baseURL := tipPage(t)
	var copied string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Evaluate(`(() => {
			const r = document.createRange();
			r.selectNodeContents(document.querySelector("main p"));
			const s = getSelection(); s.removeAllRanges(); s.addRange(r);
			return s.toString();
		})()`, &copied),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if copied != "First b then c end." {
		t.Errorf("copied %q, want %q", copied, "First b then c end.")
	}
}

// A hidden tip takes no room: near the end of a line on a phone, its box
// would otherwise reach past the viewport and scroll the whole page
// sideways before anyone hovers.
func TestBrowserHiddenTipDoesNotWidenThePage(t *testing.T) {
	page, err := md2html.Convert([]byte("Some words then a chip at the end of the line [edge]{tip=\"A fairly long note that will be about thirty characters wide or more.\"}\n"), md2html.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"index.html": page})
	ctx := newBrowserCtx(t)
	var widths struct{ Scroll, Client float64 }
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(500, 800),
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Evaluate(`({Scroll: document.documentElement.scrollWidth, Client: document.documentElement.clientWidth})`, &widths),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if widths.Scroll > widths.Client {
		t.Errorf("page scrolls sideways: scrollWidth %v > clientWidth %v", widths.Scroll, widths.Client)
	}
}

// A style restyles a tip by naming its classes; the tip's own cue must
// not outrank a style's plain rule for a chip.
func TestBrowserStyleWinsTheTipCursor(t *testing.T) {
	page, err := md2html.Convert([]byte("[b]{.chip tip=\"n\"}\n"), md2html.Options{CSS: ".chip { cursor: pointer; }"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	baseURL := serveGenerated(t, map[string][]byte{"index.html": page})
	ctx := newBrowserCtx(t)
	var cursor string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/index.html"),
		chromedp.Evaluate(`getComputedStyle(document.querySelector(".md2html-tip-host")).cursor`, &cursor),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if cursor != "pointer" {
		t.Errorf("cursor = %q, want the style's pointer", cursor)
	}
}
