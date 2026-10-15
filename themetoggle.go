package md2html

import (
	"bytes"

	"golang.org/x/net/html"
)

// themeToggleMarkers expands [[theme-toggle]], which works on any page,
// shell or not, into a button that switches the page between its style's
// light and dark themes. The button is hidden until themeToggleRuntime
// shows it, so a page read without scripts has no dead control. The
// glyph sits in its own span, so a style can hide it, move it, or make
// the span the knob of a switch drawn on the button.
func themeToggleMarkers() Transform {
	return Transform{Name: "themeToggleMarkers", Fn: func(root *html.Node) error {
		for _, m := range findMarkers(root) {
			if m.name == "theme-toggle" {
				b := elem("button", "type", "button", "class", "md2html-theme-toggle",
					"hidden", "", "aria-label", "Dark theme", "aria-pressed", "false")
				b.AppendChild(elem("span", "class", "md2html-theme-icon", "aria-hidden", "true"))
				replaceMarker(m, b)
			}
		}
		return nil
	}}
}

// themeToggleMarkerWarning warns about a [[theme-toggle]] in a fragment,
// which carries no script and whose host sets the theme itself.
func themeToggleMarkerWarning(warn func(string)) Transform {
	return Transform{Name: "themeToggleMarkerWarning", Fn: func(root *html.Node) error {
		for _, m := range findMarkers(root) {
			if m.name == "theme-toggle" && warn != nil {
				warn("[[theme-toggle]] has no effect in a fragment, whose host sets the theme; leaving it as written")
			}
		}
		return nil
	}}
}

// hasThemeToggle reports whether the rendered body carries a toggle.
func hasThemeToggle(body []byte) bool {
	return bytes.Contains(body, []byte(`class="md2html-theme-toggle"`))
}

// themeKey is the localStorage key the reader's choice is kept under. It
// is per origin, so every page of a site served together shares it.
const themeKey = "md2html-theme"

// themeInitRuntime goes in <head>, ahead of anything painting, and applies
// a saved choice so a dark page never flashes light on load.
const themeInitRuntime = `<script>
try {
  const t = localStorage.getItem("` + themeKey + `");
  if (t === "light" || t === "dark") document.documentElement.dataset.theme = t;
} catch (e) {}
</script>
`

// themeToggleRuntime drives the toggle: a click flips the theme the page
// is showing, by data-theme on <html>, which every style's tokens follow.
// Choosing what the system already prefers drops the override instead of
// pinning it, so the page goes back to following the system. Each change
// fires an md2html-theme event on document, which the mermaid runtime
// listens for to redraw its diagrams in the new colours.
const themeToggleRuntime = `<script>
(() => {
  const root = document.documentElement;
  const media = matchMedia("(prefers-color-scheme: dark)");
  const dark = () => root.dataset.theme ? root.dataset.theme === "dark" : media.matches;
  const buttons = document.querySelectorAll(".md2html-theme-toggle");
  const show = () => {
    const d = dark();
    for (const b of buttons) {
      b.setAttribute("aria-pressed", String(d));
      b.querySelector(".md2html-theme-icon").textContent = d ? "☀" : "☾";
    }
  };
  for (const b of buttons) {
    b.hidden = false;
    b.addEventListener("click", () => {
      const next = dark() ? "light" : "dark";
      try {
        if ((next === "dark") === media.matches) localStorage.removeItem("` + themeKey + `");
        else localStorage.setItem("` + themeKey + `", next);
      } catch (e) {}
      if ((next === "dark") === media.matches) delete root.dataset.theme;
      else root.dataset.theme = next;
      show();
      document.dispatchEvent(new Event("md2html-theme"));
    });
  }
  media.addEventListener("change", show);
  show();
})();
</script>
`
