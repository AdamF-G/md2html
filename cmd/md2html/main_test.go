package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AdamF-G/md2html"
)

func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for rel, c := range files {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(c), 0o644)
	}
	return root
}

func TestRunConvertsTreeToOutputDir(t *testing.T) {
	root := tree(t, map[string]string{
		"docs/index.md":    "# Home\n\n[a](./api/auth.md)",
		"docs/api/auth.md": "# Auth",
	})
	var out, errb bytes.Buffer
	code := run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site")}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	for _, want := range []string{"site/index.html", "site/api/auth.html"} {
		if _, err := os.Stat(filepath.Join(root, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
}

func TestRunSingleFileWritesBesideSource(t *testing.T) {
	root := tree(t, map[string]string{"README.md": "# Title\n\ntext"})
	var out, errb bytes.Buffer
	if code := run([]string{filepath.Join(root, "README.md")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	b, err := os.ReadFile(filepath.Join(root, "README.html"))
	if err != nil {
		t.Fatalf("README.html not written: %v", err)
	}
	if !strings.Contains(string(b), "<title>Title</title>") {
		t.Errorf("title missing: %s", b)
	}
	if out.Len() != 0 {
		t.Errorf("nothing should go to stdout, got: %s", out.String())
	}
}

// Flags must be accepted after the entry point, which is how every
// documented invocation is written.
func TestRunAcceptsFlagsAfterEntry(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# A"})
	var out, errb bytes.Buffer
	code := run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site")}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "site", "a.html")); err != nil {
		t.Errorf("flag after entry not parsed: %v", err)
	}
}

// The derived title must not pick up the "#" that HeadingAnchors appends.
func TestRunTitleExcludesAnchorText(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# Doc Title\n\ntext"})
	var out, errb bytes.Buffer
	run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site")}, &out, &errb)
	b, err := os.ReadFile(filepath.Join(root, "site", "a.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "<title>Doc Title</title>") {
		t.Errorf("title polluted by anchor text: %s", b)
	}
}

func TestRunRefusesToOverwriteHandWrittenHTML(t *testing.T) {
	root := tree(t, map[string]string{
		"docs/a.md":   "# A",
		"site/a.html": "<html>mine</html>",
	})
	var out, errb bytes.Buffer
	code := run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site")}, &out, &errb)
	if code == 0 {
		t.Error("expected non-zero exit when refusing a file")
	}
	b, _ := os.ReadFile(filepath.Join(root, "site/a.html"))
	if string(b) != "<html>mine</html>" {
		t.Errorf("hand-written file was modified: %s", b)
	}
	if !strings.Contains(errb.String(), "refus") {
		t.Errorf("no refusal reported: %s", errb.String())
	}
}

func TestRunIsIdempotent(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# A"})
	args := []string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site")}
	var o1, e1, o2, e2 bytes.Buffer
	if code := run(args, &o1, &e1); code != 0 {
		t.Fatalf("first run exit %d: %s", code, e1.String())
	}
	if code := run(args, &o2, &e2); code != 0 {
		t.Fatalf("second run exit %d (should overwrite own output silently): %s", code, e2.String())
	}
}

func TestRunFragmentMode(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# A"})
	var out, errb bytes.Buffer
	run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site"), "--fragment"}, &out, &errb)
	b, err := os.ReadFile(filepath.Join(root, "site/a.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(b)), "<body") {
		t.Errorf("fragment mode emitted a body tag: %s", b)
	}
}

func TestRunErrorsOnMissingEntryPoint(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"/definitely/not/here.md"}, &out, &errb); code == 0 {
		t.Error("expected non-zero exit for a missing entry point")
	}
}

func TestRunInPlaceWritesBesideSource(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# A"})
	var out, errb bytes.Buffer
	if code := run([]string{filepath.Join(root, "docs")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "docs/a.html")); err != nil {
		t.Errorf("in-place output missing: %v", err)
	}
}

func TestStringListSplitsAndAccumulates(t *testing.T) {
	var l stringList
	if err := l.Set("a,b"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := l.Set(" c "); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := []string(l); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("got %v, want [a b c]", got)
	}
}

// Empty values would resolve to base itself, excluding the whole tree.
func TestStringListDropsEmptyValues(t *testing.T) {
	var l stringList
	if err := l.Set("a,,b,"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := []string(l); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("got %v, want [a b]", got)
	}
}

// End to end: an excluded subtree gets no .html written into it, and the
// run still succeeds.
func TestRunExcludeWritesNothingIntoExcludedTree(t *testing.T) {
	root := tree(t, map[string]string{
		"index.md":       "[d](./slides/deck.md)\n",
		"slides/deck.md": "# Deck\n",
	})
	var out, errb bytes.Buffer
	code := run([]string{"--exclude", "slides", root}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "slides/deck.html")); !os.IsNotExist(err) {
		t.Errorf("wrote into excluded subtree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
		t.Errorf("did not write index.html: %v", err)
	}
	if !strings.Contains(errb.String(), "excluded") {
		t.Errorf("no exclusion warning on stderr: %s", errb.String())
	}
}

// End to end: a name glob drops matching documents wherever they sit.
func TestRunExcludeNamePattern(t *testing.T) {
	root := tree(t, map[string]string{
		"index.md":           "# Index\n",
		"AUDIT_2026.md":      "# Audit\n",
		"notes/AUDIT_old.md": "# Old\n",
	})
	var out, errb bytes.Buffer
	if code := run([]string{"--exclude", "AUDIT_*", root}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
		t.Errorf("did not write index.html: %v", err)
	}
	for _, p := range []string{"AUDIT_2026.html", "notes/AUDIT_old.html"} {
		if _, err := os.Stat(filepath.Join(root, p)); !os.IsNotExist(err) {
			t.Errorf("wrote excluded %s: %v", p, err)
		}
	}
}

func TestRunLinkDepthBoundsFollowing(t *testing.T) {
	root := tree(t, map[string]string{
		"index.md": "[a](./a.md)\n",
		"a.md":     "[b](./b.md)\n",
		"b.md":     "# B\n",
	})
	var out, errb bytes.Buffer
	if code := run([]string{"--link-depth", "1", filepath.Join(root, "index.md")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "a.html")); err != nil {
		t.Errorf("one hop not followed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "b.html")); !os.IsNotExist(err) {
		t.Errorf("two hops followed despite --link-depth 1: %v", err)
	}
}

// The transform order is a correctness constraint pinned for the library at
// md2html_test.go's TestBuiltinsSignatureUnchanged. buildOptions rebuilds
// this same list from md2html.Builtins() precisely so it cannot drift from
// that pinned order; this test is what makes the constraint enforceable on
// the CLI side too — without it, a reordering here would ship green while
// the library stayed correct.
func TestBuildOptionsMatchesBuiltinsOrder(t *testing.T) {
	opts := buildOptions(md2html.Doc{Src: "doc.md"}, false, "", "", "", false, false, false, nil)
	want := md2html.Builtins()
	if len(opts.Transforms) != len(want) {
		t.Fatalf("buildOptions produced %d transforms, want %d", len(opts.Transforms), len(want))
	}
	for i, tr := range want {
		if opts.Transforms[i].Name != tr.Name {
			t.Errorf("Transforms[%d].Name = %q, want %q", i, opts.Transforms[i].Name, tr.Name)
		}
	}
}

// Each --no-* flag must drop exactly its own transform and nothing else,
// leaving every other name in its Builtins() position.
func TestBuildOptionsNoFlagsDropOnlyTheirOwnTransform(t *testing.T) {
	opts := buildOptions(md2html.Doc{Src: "doc.md"}, false, "", "", "", true, true, true, nil)
	want := []string{"containers", "alerts", "linkAttrs", "chips", "sectionLinks", "toc"}
	var got []string
	for _, tr := range opts.Transforms {
		got = append(got, tr.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Transforms = %v, want %v", got, want)
	}
}

// End to end through the CLI: a chip, a heading, a "[[toc]]" marker and a
// "§" cross-reference must all resolve exactly as the library does on its
// own, so a future reordering in buildOptions shows up here too, not only
// in the unit-level name check above.
func TestRunResolvesChipsSectionLinksAndTOC(t *testing.T) {
	root := tree(t, map[string]string{
		"doc.md": "# Intro\n\n[[toc]]\n\n## 1 Setup [proven]\n\nSee §1 for details.\n",
	})
	var out, errb bytes.Buffer
	if code := run([]string{filepath.Join(root, "doc.md")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	b, err := os.ReadFile(filepath.Join(root, "doc.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, `<span class="chip chip-proven">proven</span>`) {
		t.Errorf("chip not rendered\ngot: %s", got)
	}
	if !strings.Contains(got, `id="1-setup"`) {
		t.Errorf("chip leaked into heading slug\ngot: %s", got)
	}
	if !strings.Contains(got, `<nav class="toc"`) {
		t.Errorf("toc marker not replaced\ngot: %s", got)
	}
	if !strings.Contains(got, `<a class="xref" href="#1-setup">§1</a>`) {
		t.Errorf("section reference not resolved\ngot: %s", got)
	}
}

// --version reports the same constant that every generated file's
// provenance marker carries, so the two can never disagree.
func TestRunVersionPrintsVersionToStdout(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--version"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if got, want := out.String(), "md2html "+md2html.Version+"\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if errb.Len() != 0 {
		t.Errorf("nothing should go to stderr, got: %s", errb.String())
	}
}

// The flag wins over the work: asking for the version never converts a
// document, wherever in the arguments it appears.
func TestRunVersionAfterEntryConvertsNothing(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# A"})
	var out, errb bytes.Buffer
	code := run([]string{filepath.Join(root, "docs"), "--version"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), md2html.Version) {
		t.Errorf("version not printed: %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "a.html")); err == nil {
		t.Error("--version converted a document")
	}
}

// The container warning is the one diagnostic that reaches the sink through
// a transform rather than from Convert itself, so it is the one that goes
// quiet if the CLI's transform list is ever assembled without the sink
// wired in. Nothing else in this suite would notice: the page still
// converts, and the run still exits 0.
func TestRunReportsContainerWarningToStderr(t *testing.T) {
	root := tree(t, map[string]string{"docs/index.md": "::: kaution\noops\n:::\n"})
	var out, errb bytes.Buffer
	if code := run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(errb.String(), "kaution") {
		t.Errorf("container warning did not reach stderr:\n%s", errb.String())
	}
	if !strings.Contains(errb.String(), "1 warning(s)") {
		t.Errorf("container warning not counted in the summary:\n%s", errb.String())
	}
}

// installed returns the two paths the skill occupies under a skills parent.
func installed(parent string) (skill, ref string) {
	dir := filepath.Join(parent, "md2html-authoring")
	return filepath.Join(dir, "SKILL.md"), filepath.Join(dir, "authoring.md")
}

// --install-skill-project writes into the .claude the caller is standing
// in, so a repo can carry the skill for everyone who clones it.
func TestRunInstallSkillProjectWritesBesideTheCaller(t *testing.T) {
	root := tree(t, map[string]string{".claude/settings.json": "{}"})
	t.Chdir(root)

	var out, errb bytes.Buffer
	if code := run([]string{"--install-skill-project"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	skill, ref := installed(filepath.Join(root, ".claude", "skills"))
	for _, p := range []string{skill, ref} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	if !strings.HasPrefix(readFile(t, skill), "---\n") {
		t.Error("installed SKILL.md does not open with its front matter")
	}
	if !strings.Contains(out.String(), "md2html-authoring") {
		t.Errorf("install reported nothing useful on stdout: %q", out.String())
	}
}

// --install-skill-user installs for the user, under their home.
func TestRunInstallSkillWritesUnderHome(t *testing.T) {
	home := tree(t, map[string]string{".claude/settings.json": "{}"})
	t.Setenv("HOME", home)

	var out, errb bytes.Buffer
	if code := run([]string{"--install-skill-user"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	skill, _ := installed(filepath.Join(home, ".claude", "skills"))
	if _, err := os.Stat(skill); err != nil {
		t.Errorf("missing %s: %v", skill, err)
	}
}

// A missing .claude means this is not a Claude Code workspace — or the
// caller is in the wrong directory. Inventing one puts the skill somewhere
// nothing will ever read it and still reports success, which is the same
// failure `just install` refuses for the binary.
func TestRunInstallSkillRefusesToInventDotClaude(t *testing.T) {
	root := tree(t, map[string]string{"docs/index.md": "# x"})
	t.Chdir(root)

	var out, errb bytes.Buffer
	code := run([]string{"--install-skill-project"}, &out, &errb)
	if code == 0 {
		t.Fatal("installing into a tree with no .claude reported success")
	}
	// Absolute, not a bare ".claude": this error exists to catch a caller
	// who is not standing where they think they are, and a relative path is
	// the least useful thing to tell exactly that caller.
	if !strings.Contains(errb.String(), filepath.Join(root, ".claude")) {
		t.Errorf("error does not name the directory it wanted, in full: %s", errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); err == nil {
		t.Error(".claude was created anyway")
	}
}

// Re-installing over this tool's own copy is how an upgrade lands, and must
// be silent rather than a refusal the user has to clear by hand.
func TestRunInstallSkillReplacesItsOwnEarlierCopy(t *testing.T) {
	root := tree(t, map[string]string{".claude/settings.json": "{}"})
	t.Chdir(root)

	var out, errb bytes.Buffer
	if code := run([]string{"--install-skill-project"}, &out, &errb); code != 0 {
		t.Fatalf("first install: exit %d, stderr: %s", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"--install-skill-project"}, &out, &errb); code != 0 {
		t.Fatalf("re-install: exit %d, stderr: %s", code, errb.String())
	}
}

// A copy the user has edited carries no marker, and is never destroyed —
// the same rule, and the same message, a generated page gets.
func TestRunInstallSkillRefusesAHandEditedCopy(t *testing.T) {
	root := tree(t, map[string]string{
		".claude/settings.json":                     "{}",
		".claude/skills/md2html-authoring/SKILL.md": "---\nname: mine\n---\n\nhand written\n",
	})
	t.Chdir(root)

	var out, errb bytes.Buffer
	code := run([]string{"--install-skill-project"}, &out, &errb)
	if code == 0 {
		t.Fatal("overwrote a hand-edited skill and reported success")
	}
	skill, ref := installed(filepath.Join(root, ".claude", "skills"))
	if got := readFile(t, skill); !strings.Contains(got, "hand written") {
		t.Errorf("hand-edited SKILL.md was destroyed, now:\n%s", got)
	}
	if !strings.Contains(errb.String(), "refusing") {
		t.Errorf("refusal not reported: %s", errb.String())
	}
	// Nothing half-installed: the reference is not written either, so the
	// directory is not left holding one file from this version beside one
	// the user wrote.
	if _, err := os.Stat(ref); err == nil {
		t.Error("authoring.md was installed beside the refused SKILL.md")
	}
}

// Installing is a whole invocation, like --version: it needs no entry point
// and must convert nothing.
func TestRunInstallSkillNeedsNoEntryAndConvertsNothing(t *testing.T) {
	root := tree(t, map[string]string{
		".claude/settings.json": "{}",
		"docs/index.md":         "# Home",
	})
	t.Chdir(root)

	var out, errb bytes.Buffer
	if code := run([]string{"--install-skill-project"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "index.html")); err == nil {
		t.Error("installing the skill also converted a document")
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --lang sets every page's language, and a document's own front matter
// still overrides it.
func TestRunLangFlag(t *testing.T) {
	root := tree(t, map[string]string{
		"docs/a.md": "# A",
		"docs/b.md": "---\nlang: de\n---\n# B",
	})
	var out, errb bytes.Buffer
	code := run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site"), "--lang", "fr"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	for file, want := range map[string]string{"a.html": `<html lang="fr">`, "b.html": `<html lang="de">`} {
		b, err := os.ReadFile(filepath.Join(root, "site", file))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), want) {
			t.Errorf("%s: missing %s", file, want)
		}
	}
}

func TestRunTOCFlag(t *testing.T) {
	root := tree(t, map[string]string{
		"docs/a.md": "# A\n\n[TOC]\n\n## One",
		"docs/b.md": "---\ntoc: inline\n---\n# B\n\n[TOC]\n\n## One",
	})
	var out, errb bytes.Buffer
	code := run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site"), "--toc", "float"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	for file, float := range map[string]bool{"a.html": true, "b.html": false} {
		b, err := os.ReadFile(filepath.Join(root, "site", file))
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(b), `class="toc toc-float"`); got != float {
			t.Errorf("%s: floating = %v, want %v", file, got, float)
		}
	}
}

// A bad --toc is caught before anything is written, like a bad --lang.
func TestRunTOCFlagRejectsUnknownLayout(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# A"})
	var out, errb bytes.Buffer
	code := run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site"), "--toc", "sidebar"}, &out, &errb)
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "--toc") {
		t.Errorf("stderr does not name the flag: %s", errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "site", "a.html")); err == nil {
		t.Error("a page was written despite the bad flag")
	}
}

// A bad --lang is a mistake on the command line, caught before anything is
// written, not a warning repeated once per document.
func TestRunLangFlagRejectsNonTag(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# A"})
	var out, errb bytes.Buffer
	code := run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site"), "--lang", "en_US"}, &out, &errb)
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "--lang") {
		t.Errorf("stderr does not name the flag: %s", errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "site", "a.html")); err == nil {
		t.Error("a page was written despite the bad flag")
	}
}
