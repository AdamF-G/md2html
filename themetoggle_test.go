package md2html

import (
	"strings"
	"testing"
)

const toggleButton = `<button type="button" class="md2html-theme-toggle" hidden="" aria-label="Dark theme" aria-pressed="false"></button>`

func TestThemeToggleOnAPlainPage(t *testing.T) {
	out, warnings := pageWith(t, t.TempDir(), "p.md", "# Page\n\n[[theme-toggle]]\n")
	if len(warnings) != 0 {
		t.Errorf("warnings: %v", warnings)
	}
	if !strings.Contains(out, toggleButton) {
		t.Errorf("no toggle button:\n%s", out)
	}
	head, body, _ := strings.Cut(out, "</head>")
	if !strings.Contains(head, `localStorage.getItem("md2html-theme")`) {
		t.Error("the saved theme is not applied in <head>, so a dark page flashes light")
	}
	if !strings.Contains(head, ".md2html-theme-toggle {") {
		t.Error("the page has a toggle but not the CSS that draws it")
	}
	if !strings.Contains(body, `dispatchEvent(new Event("md2html-theme"))`) {
		t.Error("the page has a toggle but no runtime")
	}
}

func TestThemeToggleRuntimeOnlyWhereAsked(t *testing.T) {
	out, _ := pageWith(t, t.TempDir(), "p.md", "# Page\n\nSee [[theme-toggle]] in prose.\n")
	if strings.Contains(out, "md2html-theme") {
		t.Error("a page without the marker got the toggle's script or CSS")
	}
}

func TestThemeToggleInAFragmentWarns(t *testing.T) {
	var warnings []string
	out, err := Convert([]byte("[[theme-toggle]]\n"), Options{Fragment: true,
		Warn: func(m string) { warnings = append(warnings, m) }})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<p>[[theme-toggle]]</p>") {
		t.Errorf("the marker was not left as written:\n%s", out)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "[[theme-toggle]] has no effect in a fragment") {
		t.Errorf("warnings = %v", warnings)
	}
}

func TestThemeToggleInTheShell(t *testing.T) {
	files := map[string]string{
		"shell/site.md": "---\nsite: true\n---\n[[theme-toggle]]\n\n[[content]]\n",
		"docs/index.md": "# Home\n",
	}
	s, res, root := siteFixture(t, files, "shell/site.md")
	out, _ := sitePage(t, s, res, root, "docs/index.md")
	if !strings.Contains(out, toggleButton) || !strings.Contains(out, `dispatchEvent(new Event("md2html-theme"))`) {
		t.Errorf("shell toggle missing:\n%s", out)
	}
}

func TestMermaidRedrawsOnThemeChange(t *testing.T) {
	out, _ := pageWith(t, t.TempDir(), "p.md", "[[theme-toggle]]\n\n```mermaid\ngraph TD; A-->B\n```\n")
	if !strings.Contains(out, `addEventListener("md2html-theme"`) {
		t.Error("the mermaid runtime does not redraw when the toggle flips the theme")
	}
}
