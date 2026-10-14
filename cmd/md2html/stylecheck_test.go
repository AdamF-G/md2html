package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stylePlugin is a plugin, "acme", with one style that passes, one with a
// warning, one with two errors and one with four.
func stylePlugin(t *testing.T) string {
	t.Helper()
	return stylePluginAt(t, tree(t, nil), "acme")
}

// stylePluginAt writes stylePlugin's files to root/sub and returns that dir.
func stylePluginAt(t *testing.T, root, sub string) string {
	t.Helper()
	files := map[string]string{
		"md2html-plugin.json": `{"format": 1, "name": "acme", "styles": {
			"plain": {"css": ["plain.css"]}, "quiet": {"css": ["quiet.css"]},
			"bad": {"css": ["bad.css"]}, "worse": {"css": ["worse.css"]}}}`,
		"plain.css": ":root{--accent:#2c5a85}\n:root[data-theme=\"light\"]{--accent:#2c5a85}\n",
		"quiet.css": ":root{--muted:#766f67}\n:root[data-theme=\"light\"]{--muted:#766f67}\n",
		"bad.css":   ":root{--muted:#aaaaaa}\n:root[data-theme=\"light\"]{--muted:#aaaaaa}\n",
		"worse.css": ":root{--muted:#aaaaaa}\n:root[data-theme=\"light\"]{--muted:#aaaaaa}\n" +
			"@media (prefers-color-scheme: dark){:root:not([data-theme=\"light\"]){--muted:#555555}}\n" +
			":root[data-theme=\"dark\"]{--muted:#555555}\n",
	}
	dir := filepath.Join(root, sub)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, c := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCheckStyleBuiltinsPass(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--check-style"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s%s", code, out.String(), errb.String())
	}
	// Whole lines: "default: no problems" is also the tail of
	// "default/default: no problems".
	lines := map[string]bool{}
	for _, l := range strings.Split(out.String(), "\n") {
		lines[l] = true
	}
	for _, want := range []string{"default: no problems", "claude-artifact: no problems", "proof: no problems"} {
		if !lines[want] {
			t.Errorf("stdout lacks the line %q:\n%s", want, out.String())
		}
	}
}

func TestCheckStyleReportsEachPluginStyle(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"--check-style", "--plugin", stylePlugin(t)}, &out, &errb)
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	want := "bad: 2 errors\n" +
		"  error    light --muted on --bg is 2.22:1, under 3:1\n" +
		"  error    light --muted on --code-bg is 2.01:1, under 3:1\n" +
		"plain: no problems\n" +
		"quiet: 1 warning\n" +
		"  warning  light --muted on --code-bg is 4.28:1, under 4.5:1 (WCAG AA)\n" +
		"worse: 4 errors\n" +
		"  error    light --muted on --bg is 2.22:1, under 3:1\n" +
		"  error    light --muted on --code-bg is 2.01:1, under 3:1\n" +
		"  error    dark --muted on --bg is 2.44:1, under 3:1\n" +
		"  error    dark --muted on --code-bg is 2.19:1, under 3:1\n"
	if out.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", out.String(), want)
	}
}

// Warnings alone exit 0; a --style list is checked as one stack.
func TestCheckStyleStackAndExitCodes(t *testing.T) {
	dir := stylePlugin(t)
	var out, errb bytes.Buffer
	if code := run([]string{"--check-style", "--plugin", dir, "--style", "acme/plain,acme/quiet"}, &out, &errb); code != 0 {
		t.Errorf("exit %d, want 0: %s", code, errb.String())
	}
	if want := "acme/plain,acme/quiet: 1 warning\n  warning  light --muted on --code-bg is 4.28:1, under 4.5:1 (WCAG AA)\n"; out.String() != want {
		t.Errorf("stdout %q, want %q", out.String(), want)
	}
	out.Reset()
	if code := run([]string{"--check-style", "--plugin", "/no/such/dir"}, &out, &errb); code != 2 {
		t.Errorf("bad plugin: exit %d, want 2", code)
	}
	if out.Len() != 0 {
		t.Errorf("bad plugin: stdout %q, want nothing", out.String())
	}
}

// --check-style checks styles, so files or --css are a mistake to say so.
func TestCheckStyleRejectsFilesAndCSS(t *testing.T) {
	for _, args := range [][]string{{"--check-style", "doc.md"}, {"--check-style", "--css", "my.css"}} {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
		if want := "md2html: --check-style checks styles; it takes no files or --css\n"; errb.String() != want || out.Len() != 0 {
			t.Errorf("%v: stderr %q stdout %q, want stderr %q", args, errb.String(), out.String(), want)
		}
	}
}

// buildWith builds doc.md from files with args, and returns stderr and
// the exit code.
func buildWith(t *testing.T, files map[string]string, args ...string) (string, int) {
	t.Helper()
	root := tree(t, files)
	var out, errb bytes.Buffer
	code := run(append([]string{filepath.Join(root, "doc.md")}, args...), &out, &errb)
	return errb.String(), code
}

// One or two findings are printed in full; an error counts as a warning
// in the summary, and the build still succeeds.
func TestRunStyleCheckLineShort(t *testing.T) {
	dir := stylePlugin(t)
	stderr, code := buildWith(t, map[string]string{"doc.md": "# A\n"}, "--plugin", dir, "--style", "acme/bad")
	if code != 0 {
		t.Errorf("exit %d, want 0", code)
	}
	want := "md2html: style acme/bad: light --muted on --bg is 2.22:1, under 3:1; light --muted on --code-bg is 2.01:1, under 3:1\n"
	if !strings.Contains(stderr, want) || !strings.Contains(stderr, ", 1 warning(s)\n") {
		t.Errorf("stderr:\n%s\nwant the line %q and 1 warning", stderr, want)
	}
}

func TestRunStyleCheckLineCounted(t *testing.T) {
	dir := stylePlugin(t)
	stderr, _ := buildWith(t, map[string]string{"doc.md": "# A\n"}, "--plugin", dir, "--style", "acme/worse")
	want := "md2html: style acme/worse: 4 errors (md2html --check-style --plugin " + dir + " --style acme/worse for details)\n"
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr:\n%s\nwant %q", stderr, want)
	}
}

// Warnings alone print the line but leave the summary's count alone.
func TestRunStyleCheckLineWarningsDoNotCount(t *testing.T) {
	dir := stylePlugin(t)
	stderr, _ := buildWith(t, map[string]string{"doc.md": "# A\n"}, "--plugin", dir, "--style", "acme/quiet")
	if !strings.Contains(stderr, "md2html: style acme/quiet: light --muted on --code-bg is 4.28:1, under 4.5:1 (WCAG AA)\n") ||
		!strings.Contains(stderr, ", 0 warning(s)\n") {
		t.Errorf("stderr:\n%s", stderr)
	}
}

// A list chosen by front matter on two pages converted in parallel is
// reported once. A style set in :root alone is reported too, since a
// reader who picks light gets the default's colour.
func TestRunStyleCheckLineForFrontMatterStyle(t *testing.T) {
	dir := stylePlugin(t)
	if err := os.WriteFile(filepath.Join(dir, "rootonly.css"), []byte(":root{--accent:#2c5a85}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"format": 1, "name": "acme", "styles": {"bad": {"css": ["bad.css"]}, "rootonly": {"css": ["rootonly.css"]}}}`
	if err := os.WriteFile(filepath.Join(dir, "md2html-plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	page := "---\nstyle: acme/bad\n---\n# A\n\n[B](b.md)\n"
	stderr, _ := buildWith(t, map[string]string{"doc.md": page, "b.md": "---\nstyle: acme/bad\n---\n# B\n"}, "--plugin", dir)
	if n := strings.Count(stderr, "md2html: style acme/bad: "); n != 1 {
		t.Errorf("acme/bad reported %d times, want once:\n%s", n, stderr)
	}
	stderr, _ = buildWith(t, map[string]string{"doc.md": "---\nstyle: acme/rootonly\n---\n# A\n"}, "--plugin", dir)
	if want := "md2html: style acme/rootonly: light --accent is #9a4f2c (light picked) but #2c5a85 (light system, nothing picked)\n"; !strings.Contains(stderr, want) {
		t.Errorf("stderr:\n%s\nwant %q", stderr, want)
	}
}

// A style loaded only through a page's plugins: gets a hint naming that
// dir, quoted when it holds a space.
func TestRunStyleCheckLineHintNamesPageDir(t *testing.T) {
	root := tree(t, map[string]string{"doc.md": "---\nstyle: acme/worse\nplugins: my styles/acme\n---\n# A\n"})
	dir := stylePluginAt(t, root, "my styles/acme")
	var out, errb bytes.Buffer
	run([]string{filepath.Join(root, "doc.md")}, &out, &errb)
	want := "md2html: style acme/worse: 4 errors (md2html --check-style --plugin '" + dir + "' --style acme/worse for details)\n"
	if !strings.Contains(errb.String(), want) {
		t.Errorf("stderr:\n%s\nwant %q", errb.String(), want)
	}
}

// Two plugins that give the same qualified name are checked apart, each
// line naming its plugin's dir, and each hint loading it.
func TestRunStyleCheckLineKeysByDir(t *testing.T) {
	root := tree(t, map[string]string{
		"doc.md": "---\nstyle: acme/worse\nplugins: one\n---\n# A\n\n[B](b.md)\n[C](c.md)\n[D](d.md)\n",
		"b.md":   "---\nstyle: acme/worse\nplugins: two\n---\n# B\n",
		"c.md":   "---\nstyle: acme/bad\nplugins: one\n---\n# C\n",
		"d.md":   "---\nstyle: acme/bad\nplugins: two\n---\n# D\n",
	})
	one, two := stylePluginAt(t, root, "one"), stylePluginAt(t, root, "two")
	var out, errb bytes.Buffer
	run([]string{filepath.Join(root, "doc.md")}, &out, &errb)
	for _, dir := range []string{one, two} {
		for _, want := range []string{
			"md2html: style acme/worse (from " + dir + "): 4 errors (md2html --check-style --plugin " + dir + " --style acme/worse for details)\n",
			"md2html: style acme/bad (from " + dir + "): light --muted on --bg is 2.22:1",
		} {
			if !strings.Contains(errb.String(), want) {
				t.Errorf("stderr:\n%s\nwant %q", errb.String(), want)
			}
		}
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/a/b-c_d.e": "/a/b-c_d.e",
		"/a b":       "'/a b'",
		"/it's":      `'/it'\''s'`,
		"":           "''",
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRunNoStyleNoLine(t *testing.T) {
	stderr, _ := buildWith(t, map[string]string{"doc.md": "# A\n"})
	if strings.Contains(stderr, "md2html: style ") {
		t.Errorf("stderr:\n%s", stderr)
	}
}
