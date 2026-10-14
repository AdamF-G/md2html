package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

// stylePlugin is a plugin, "acme", with one style that passes, one with a
// warning, one with two errors and one with four.
func stylePlugin(t *testing.T) string {
	t.Helper()
	root := tree(t, map[string]string{
		"acme/md2html-plugin.json": `{"format": 1, "name": "acme", "styles": {
			"plain": {"css": ["plain.css"]}, "quiet": {"css": ["quiet.css"]},
			"bad": {"css": ["bad.css"]}, "worse": {"css": ["worse.css"]}}}`,
		"acme/plain.css": ":root{--accent:#2c5a85}\n:root[data-theme=\"light\"]{--accent:#2c5a85}\n",
		"acme/quiet.css": ":root{--muted:#766f67}\n:root[data-theme=\"light\"]{--muted:#766f67}\n",
		"acme/bad.css":   ":root{--muted:#aaaaaa}\n:root[data-theme=\"light\"]{--muted:#aaaaaa}\n",
		"acme/worse.css": ":root{--muted:#aaaaaa}\n:root[data-theme=\"light\"]{--muted:#aaaaaa}\n" +
			"@media (prefers-color-scheme: dark){:root:not([data-theme=\"light\"]){--muted:#555555}}\n" +
			":root[data-theme=\"dark\"]{--muted:#555555}\n",
	})
	return filepath.Join(root, "acme")
}

func TestCheckStyleBuiltinsPass(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--check-style"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s%s", code, out.String(), errb.String())
	}
	for _, want := range []string{"default/default: no problems\n", "claude-artifact/claude-artifact: no problems\n"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Errorf("stdout lacks %q:\n%s", want, out.String())
		}
	}
}

func TestCheckStyleReportsEachPluginStyle(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"--check-style", "--plugin", stylePlugin(t)}, &out, &errb)
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	want := "acme/bad: 2 errors\n" +
		"  error    light --muted on --bg is 2.22:1, under 3:1\n" +
		"  error    light --muted on --code-bg is 2.01:1, under 3:1\n" +
		"acme/plain: no problems\n" +
		"acme/quiet: 1 warning\n" +
		"  warning  light --muted on --code-bg is 4.28:1, under 4.5:1 (WCAG AA)\n" +
		"acme/worse: 4 errors\n" +
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
