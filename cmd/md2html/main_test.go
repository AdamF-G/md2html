package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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

// --preview-runtime gives a fragment with a diagram the mermaid runtime,
// and without --fragment it is a usage error, since a page has it already.
func TestRunPreviewRuntime(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# A\n\n```mermaid\ngraph LR\n  A-->B\n```\n"})
	var out, errb bytes.Buffer
	if code := run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site"), "--fragment", "--preview-runtime"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	b, err := os.ReadFile(filepath.Join(root, "site/a.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "import mermaid from") {
		t.Errorf("no mermaid runtime in the preview fragment: %s", b)
	}

	errb.Reset()
	if code := run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "page"), "--preview-runtime"}, &out, &errb); code != 2 {
		t.Errorf("exit %d without --fragment, want 2", code)
	}
	if !strings.Contains(errb.String(), "--preview-runtime needs --fragment") {
		t.Errorf("stderr: %s", errb.String())
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
	opts := buildOptions(md2html.Doc{Src: "doc.md"}, false, false, "", "", "", "", false, false, false, false, false, "", nil)
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
	opts := buildOptions(md2html.Doc{Src: "doc.md"}, false, false, "", "", "", "", true, true, true, false, false, "", nil)
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

// --version starts with the constant that every generated file's
// provenance marker carries, so the two can never disagree.
func TestRunVersionPrintsVersionToStdout(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--version"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if got, want := out.String(), "md2html "+versionString()+"\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if !strings.HasPrefix(out.String(), "md2html "+md2html.Version) {
		t.Errorf("stdout = %q, want it to start with the marker's version", out.String())
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

// --guide prints the guide's summary to stdout and nothing else, so it can
// be piped or read straight into an agent's context.
func TestRunGuidePrintsTheSummary(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--guide"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if out.String() != string(md2html.GuideSummary()) {
		t.Error("stdout is not the guide's summary")
	}
	if errb.Len() != 0 {
		t.Errorf("nothing should go to stderr, got: %s", errb.String())
	}
}

// With "all" as its entry, --guide prints the whole guide.
func TestRunGuideAllPrintsTheWholeGuide(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--guide", "all"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if out.String() != string(md2html.AuthoringGuide()) {
		t.Error("stdout is not the authoring guide")
	}
}

// With section names as its entries, --guide prints each of them, in the
// order asked for.
func TestRunGuidePrintsNamedSections(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--guide", "styles", "diagrams"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	got := out.String()
	if !strings.HasPrefix(got, "# Styles\n") || !strings.Contains(got, "\n# Diagrams\n") {
		t.Errorf("did not print Styles then Diagrams:\n%s", got)
	}
}

// Like --version, asking for the guide is the whole invocation: an entry
// beside it is a section name, and a path names none, so the run fails
// having converted nothing rather than printing a guide nobody asked for.
func TestRunGuideAfterEntryConvertsNothing(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# A"})
	var out, errb bytes.Buffer
	if code := run([]string{filepath.Join(root, "docs"), "--guide"}, &out, &errb); code != 2 {
		t.Errorf("exit %d, want 2 for a section that does not exist", code)
	}
	if !strings.Contains(errb.String(), "no guide section") {
		t.Errorf("stderr does not say the section is unknown: %s", errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "a.html")); err == nil {
		t.Error("--guide converted a document")
	}
}

// --help is what an agent without the skill runs first. If it does not
// name the guide, the syntax is only discoverable by probing the output.
func TestRunUsageNamesTheGuide(t *testing.T) {
	var out, errb bytes.Buffer
	run(nil, &out, &errb)
	head, _, _ := strings.Cut(errb.String(), "Flags:")
	if !strings.Contains(head, "md2html --guide") {
		t.Errorf("usage text before the flag list does not point at --guide:\n%s", head)
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

// installed returns the path the skill occupies under a skills parent.
func installed(parent string) string {
	return filepath.Join(parent, "md2html-authoring", "SKILL.md")
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
	skill := installed(filepath.Join(root, ".claude", "skills"))
	if _, err := os.Stat(skill); err != nil {
		t.Errorf("missing %s: %v", skill, err)
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
	skill := installed(filepath.Join(home, ".claude", "skills"))
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
	skill := installed(filepath.Join(root, ".claude", "skills"))
	if got := readFile(t, skill); !strings.Contains(got, "hand written") {
		t.Errorf("hand-edited SKILL.md was destroyed, now:\n%s", got)
	}
	if !strings.Contains(errb.String(), "refusing") {
		t.Errorf("refusal not reported: %s", errb.String())
	}
}

// --install-skill takes any agent's skills directory, since nothing in the
// skill itself is specific to Claude Code.
func TestRunInstallSkillIntoANamedSkillsDirectory(t *testing.T) {
	root := tree(t, map[string]string{".agents/skills/other/SKILL.md": "x"})
	t.Chdir(root)

	var out, errb bytes.Buffer
	if code := run([]string{"--install-skill", filepath.Join(".agents", "skills")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	skill := installed(filepath.Join(root, ".agents", "skills"))
	if _, err := os.Stat(skill); err != nil {
		t.Errorf("missing %s: %v", skill, err)
	}
	// Reported in full, as the .claude flags report their paths.
	if !strings.Contains(out.String(), skill) {
		t.Errorf("install did not report %s: %q", skill, out.String())
	}
}

// The named directory is the one an agent reads, so it must already exist:
// a mistyped path would otherwise take the skill somewhere nothing looks,
// and still report success.
func TestRunInstallSkillRefusesToInventTheNamedDirectory(t *testing.T) {
	root := tree(t, map[string]string{".agents/README": "x"})
	t.Chdir(root)

	var out, errb bytes.Buffer
	code := run([]string{"--install-skill", filepath.Join(".agents", "skils")}, &out, &errb)
	if code == 0 {
		t.Fatal("installing into a missing directory reported success")
	}
	if !strings.Contains(errb.String(), filepath.Join(root, ".agents", "skils")) {
		t.Errorf("error does not name the directory it wanted, in full: %s", errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skils")); err == nil {
		t.Error("the missing directory was created anyway")
	}
}

// One install per invocation: two destinations at once is more likely a
// slip than a wish, and an empty one names nowhere at all.
func TestRunInstallSkillRejectsAmbiguousDestinations(t *testing.T) {
	root := tree(t, map[string]string{
		".claude/settings.json": "{}",
		"skills/README":         "x",
	})
	t.Chdir(root)

	for _, args := range [][]string{
		{"--install-skill", "skills", "--install-skill-project"},
		{"--install-skill", "skills", "--install-skill-user"},
		{"--install-skill-user", "--install-skill-project"},
		{"--install-skill", ""},
	} {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 2 {
			t.Errorf("%q: exit %d, want 2; stderr: %s", args, code, errb.String())
		}
	}
	for _, dir := range []string{"skills", filepath.Join(".claude", "skills")} {
		if _, err := os.Stat(filepath.Join(root, dir, "md2html-authoring")); err == nil {
			t.Errorf("a rejected invocation still installed under %s", dir)
		}
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

func TestRunAutoTOCFlag(t *testing.T) {
	long := "# L\n\n## 1\n\n## 2\n\n## 3\n\n## 4\n\n## 5"
	root := tree(t, map[string]string{
		"docs/short.md": "# S\n\n## One",
		"docs/long.md":  long,
	})
	for mode, want := range map[string]map[string]bool{
		"all":  {"short.html": true, "long.html": true},
		"long": {"short.html": false, "long.html": true},
	} {
		site := filepath.Join(root, "site-"+mode)
		var out, errb bytes.Buffer
		if code := run([]string{filepath.Join(root, "docs"), "-o", site, "--autotoc", mode}, &out, &errb); code != 0 {
			t.Fatalf("%s: exit %d, stderr: %s", mode, code, errb.String())
		}
		for file, float := range want {
			b, err := os.ReadFile(filepath.Join(site, file))
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(string(b), `class="toc toc-float"`); got != float {
				t.Errorf("--autotoc %s: %s: floating list = %v, want %v", mode, file, got, float)
			}
		}
	}
}

func TestRunAutoTOCFlagRejectsUnknownMode(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# A"})
	var out, errb bytes.Buffer
	code := run([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site"), "--autotoc", "some"}, &out, &errb)
	if code != 2 || !strings.Contains(errb.String(), "--autotoc") {
		t.Errorf("exit %d, stderr %q; want 2 naming the flag", code, errb.String())
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

// Pages carry their Markdown unless told not to; fragments never do.
func TestRunNoSourceFlag(t *testing.T) {
	for _, c := range []struct {
		name  string
		flags []string
		want  bool
	}{
		{"default", nil, true},
		{"opted out", []string{"--no-source"}, false},
		{"fragment", []string{"--fragment"}, false},
	} {
		root := tree(t, map[string]string{"docs/a.md": "# A\n\nsecret-ish words\n"})
		args := append([]string{filepath.Join(root, "docs"), "-o", filepath.Join(root, "site")}, c.flags...)
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 0 {
			t.Fatalf("%s: exit %d, stderr: %s", c.name, code, errb.String())
		}
		b, err := os.ReadFile(filepath.Join(root, "site", "a.html"))
		if err != nil {
			t.Fatal(err)
		}
		if has := strings.Contains(string(b), `id="`+md2html.SourceID+`"`); has != c.want {
			t.Errorf("%s: source embedded = %v, want %v", c.name, has, c.want)
		}
	}
}

const cliPlugin = `{"format": 1, "name": "acme", "description": "Acme looks",
	"styles": {"plain": {"description": "Plain and quiet", "css": ["p.css"]}}}`

func TestRunStyleFromPlugin(t *testing.T) {
	root := tree(t, map[string]string{
		"doc.md": "# A\n", "acme/md2html-plugin.json": cliPlugin, "acme/p.css": "/*acme-plain*/",
	})
	var out, errb bytes.Buffer
	code := run([]string{filepath.Join(root, "doc.md"), "--plugin", filepath.Join(root, "acme"), "--style", "plain"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	b, _ := os.ReadFile(filepath.Join(root, "doc.html"))
	if !strings.Contains(string(b), "/*acme-plain*/") {
		t.Error("the page lacks the selected style")
	}
}

func TestRunBadStyleStopsBeforeWriting(t *testing.T) {
	for _, args := range [][]string{
		{"--style", "nope"},
		{"--plugin", "missing-dir"},
		{"--style", "default", "--css", "x.css"},
	} {
		root := tree(t, map[string]string{"doc.md": "# A\n", "x.css": ""})
		var out, errb bytes.Buffer
		code := run(append([]string{filepath.Join(root, "doc.md")}, args...), &out, &errb)
		if code != 2 {
			t.Errorf("%v: exit %d, want 2; stderr %s", args, code, errb.String())
		}
		if _, err := os.Stat(filepath.Join(root, "doc.html")); err == nil {
			t.Errorf("%v: a page was written", args)
		}
	}
}

func TestRunFrontMatterStyleWarnsAndFallsBack(t *testing.T) {
	root := tree(t, map[string]string{"doc.md": "---\nstyle: nope\n---\n# A\n"})
	var out, errb bytes.Buffer
	if code := run([]string{filepath.Join(root, "doc.md")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(errb.String(), `front matter style: no style named "nope"`) {
		t.Errorf("stderr = %s", errb.String())
	}
}

func TestRunListStyles(t *testing.T) {
	root := tree(t, map[string]string{"acme/md2html-plugin.json": cliPlugin, "acme/p.css": ""})
	var out, errb bytes.Buffer
	if code := run([]string{"--list-styles", "--plugin", filepath.Join(root, "acme")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	for _, want := range []string{
		"acme: Acme looks\n  plain  Plain and quiet\n",
		"default: The stylesheet md2html uses when no style is selected\n  default  ",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout lacks %q:\n%s", want, out.String())
		}
	}
}

// A style is listed by the name --style takes: its own, unless another
// plugin has a style of that name too, when only plugin/style picks it.
func TestRunListStylesQualifiesOnlyAClash(t *testing.T) {
	root := tree(t, map[string]string{
		"acme/md2html-plugin.json":  `{"format": 1, "name": "acme", "styles": {"house": {"css": ["p.css"]}, "plain": {"css": ["p.css"]}}}`,
		"acme/p.css":                "",
		"other/md2html-plugin.json": `{"format": 1, "name": "other", "styles": {"house": {"css": ["p.css"]}}}`,
		"other/p.css":               "",
	})
	var out, errb bytes.Buffer
	if code := run([]string{"--list-styles", "--plugin", filepath.Join(root, "acme") + "," + filepath.Join(root, "other")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	for _, want := range []string{"\n  acme/house\n", "\n  other/house\n", "\n  plain\n", "\n  default  "} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout lacks %q:\n%s", want, out.String())
		}
	}
}

func TestRunListStylesBadPluginExits2(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--list-styles", "--plugin", "/no/such/dir"}, &out, &errb); code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
}

// A --plugin is checked whole before anything is converted or listed:
// every style it provides is assembled, so a forbidden url() or a missing
// CSS file in a style the run never selects still stops the run, as the
// guide promises of --list-styles.
func TestRunChecksEveryPluginStyle(t *testing.T) {
	for _, c := range []struct{ name, manifest, want string }{
		{"remote url", `{"format": 1, "name": "bad", "styles": {"ok": {"css": ["ok.css"]}, "remote": {"css": ["remote.css"]}}}`,
			"is not a relative path inside the plugin"},
		{"missing css file", `{"format": 1, "name": "bad", "styles": {"ok": {"css": ["ok.css"]}, "gone": {"css": ["gone.css"]}}}`,
			"gone.css"},
	} {
		for _, list := range []bool{false, true} {
			root := tree(t, map[string]string{
				"doc.md":                  "# A\n",
				"bad/md2html-plugin.json": c.manifest,
				"bad/ok.css":              "a{}",
				"bad/remote.css":          "a{background:url(https://cdn.example/x.png)}",
			})
			args := []string{filepath.Join(root, "doc.md"), "--plugin", filepath.Join(root, "bad")}
			if list {
				args = []string{"--list-styles", "--plugin", filepath.Join(root, "bad")}
			}
			var out, errb bytes.Buffer
			if code := run(args, &out, &errb); code != 2 {
				t.Errorf("%s, list %v: exit %d, want 2; stderr %s", c.name, list, code, errb.String())
			}
			if !strings.Contains(errb.String(), c.want) {
				t.Errorf("%s, list %v: stderr %q lacks %q", c.name, list, errb.String(), c.want)
			}
			if _, err := os.Stat(filepath.Join(root, "doc.html")); err == nil {
				t.Errorf("%s: a page was written", c.name)
			}
			if out.Len() != 0 {
				t.Errorf("%s, list %v: stdout %q, want nothing listed", c.name, list, out.String())
			}
		}
	}
}

// A style with no description lists its name alone, with nothing after it.
func TestRunListStylesNoTrailingSpace(t *testing.T) {
	root := tree(t, map[string]string{
		"acme/md2html-plugin.json": `{"format": 1, "name": "acme", "styles": {"bare": {"css": ["p.css"]}}}`,
		"acme/p.css":               "",
	})
	var out, errb bytes.Buffer
	if code := run([]string{"--list-styles", "--plugin", filepath.Join(root, "acme")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "\n  bare\n") {
		t.Errorf("stdout:\n%q", out.String())
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasSuffix(line, " ") {
			t.Errorf("trailing space: %q", line)
		}
	}
}

func TestRunLicensesPrintsTheNotices(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--licenses"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if out.String() != md2html.ThirdPartyNotices() {
		t.Error("--licenses did not print the notices whole")
	}
}

const chartPage = "```vega-lite\nmark: point\ndata: {url: ../data/r.csv}\nencoding: {x: {field: v, type: quantitative}}\n```\n"

func TestRunNoChartsFlag(t *testing.T) {
	root := tree(t, map[string]string{"a.md": "```vega-lite\nmark: point\n```\n"})
	var out, errb bytes.Buffer
	run([]string{filepath.Join(root, "a.md"), "-o", filepath.Join(root, "site"), "--no-charts"}, &out, &errb)
	b, _ := os.ReadFile(filepath.Join(root, "site", "a.html"))
	if !strings.Contains(string(b), "language-vega-lite") {
		t.Error("--no-charts rendered a chart")
	}
}

// Data a page reads may sit anywhere under the run's base, not only beside
// the page; outside the base it is refused.
func TestRunChartDataInsideTheBase(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": chartPage, "data/r.csv": "v\n1\n", "notes/b.md": "# B"})
	var out, errb bytes.Buffer
	run([]string{filepath.Join(root, "docs"), filepath.Join(root, "notes"), "-o", filepath.Join(root, "site")}, &out, &errb)
	b, _ := os.ReadFile(filepath.Join(root, "site", "docs", "a.html"))
	if !strings.Contains(string(b), "md2html-chart") {
		t.Errorf("data inside the base was refused: %s", errb.String())
	}

	alone := tree(t, map[string]string{"docs/a.md": chartPage, "data/r.csv": "v\n1\n"})
	errb.Reset()
	run([]string{filepath.Join(alone, "docs"), "-o", filepath.Join(alone, "site")}, &out, &errb)
	b, _ = os.ReadFile(filepath.Join(alone, "site", "a.html"))
	if !strings.Contains(string(b), "language-vega-lite") || !strings.Contains(errb.String(), "chart data") {
		t.Errorf("data outside the base was read, or not warned about: %s", errb.String())
	}
}

func TestRunSiteWritesPagesNotTheShell(t *testing.T) {
	root := tree(t, map[string]string{
		"shell.md":      "---\nsite: true\n---\nTOP\n\n[[content]]\n",
		"docs/index.md": "# Home\n",
	})
	var out, errb bytes.Buffer
	code := run([]string{"--site", filepath.Join(root, "shell.md"), filepath.Join(root, "docs"),
		"-o", filepath.Join(root, "site")}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	page, err := os.ReadFile(filepath.Join(root, "site/index.html"))
	if err != nil || !strings.Contains(string(page), "TOP") {
		t.Fatalf("index.html not wrapped (err %v):\n%s", err, page)
	}
	if _, err := os.Stat(filepath.Join(root, "site/shell.html")); err == nil {
		t.Error("the shell was written as a page")
	}
}

func TestRunSiteRefusesTOCFloat(t *testing.T) {
	root := tree(t, map[string]string{"shell.md": "---\nsite: true\n---\n[[content]]\n", "a.md": "# A\n"})
	var out, errb bytes.Buffer
	if code := run([]string{"--site", filepath.Join(root, "shell.md"), "--toc", "float", filepath.Join(root, "a.md")}, &out, &errb); code != 2 ||
		!strings.Contains(errb.String(), "--toc float cannot be used with --site") {
		t.Errorf("exit %d, want 2; stderr: %s", code, errb.String())
	}
}

func TestRunSiteRefusesFragment(t *testing.T) {
	root := tree(t, map[string]string{"shell.md": "---\nsite: true\n---\n[[content]]\n", "a.md": "# A\n"})
	var out, errb bytes.Buffer
	if code := run([]string{"--site", filepath.Join(root, "shell.md"), "--fragment", filepath.Join(root, "a.md")}, &out, &errb); code != 2 ||
		!strings.Contains(errb.String(), "it cannot be used with --fragment") {
		t.Errorf("exit %d, want 2; stderr: %s", code, errb.String())
	}
}

func TestRunSiteRefusesAShellWithoutSiteTrue(t *testing.T) {
	root := tree(t, map[string]string{"shell.md": "[[content]]\n", "docs/a.md": "# A\n"})
	var out, errb bytes.Buffer
	if code := run([]string{"--site", filepath.Join(root, "shell.md"), filepath.Join(root, "docs")}, &out, &errb); code != 1 ||
		!strings.Contains(errb.String(), "site: true") {
		t.Errorf("exit %d, stderr: %s", code, errb.String())
	}
}

func TestRunSiteReportsShellWarningsOnce(t *testing.T) {
	root := tree(t, map[string]string{
		"shell.md":      "---\nsite: true\nstyle: no-such-style\n---\n::: bogus\nx\n:::\n\n[[content]]\n",
		"docs/index.md": "# Home\n",
		"docs/a.md":     "# A\n",
	})
	var out, errb bytes.Buffer
	if code := run([]string{"--site", filepath.Join(root, "shell.md"), filepath.Join(root, "docs"),
		"-o", filepath.Join(root, "site")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	got := errb.String()
	shell := filepath.Join(root, "shell.md")
	if n := strings.Count(got, shell+": unknown container kind"); n != 1 {
		t.Errorf("container warning from the shell %d times, want 1:\n%s", n, got)
	}
	if n := strings.Count(got, shell+": "); n != 2 {
		t.Errorf("want the shell's two warnings (container, style) once each:\n%s", got)
	}
	if strings.Contains(got, "docs/") {
		t.Errorf("a page was blamed for the shell:\n%s", got)
	}
	if !strings.Contains(got, ", 2 warning(s)") {
		t.Errorf("summary should count the shell's warnings:\n%s", got)
	}
}

func TestRunSiteWritesTheSearchIndex(t *testing.T) {
	root := tree(t, map[string]string{
		"shell.md":      "---\nsite: true\n---\n[[search]]\n\n[[content]]\n",
		"docs/index.md": "# Home\n",
	})
	var out, errb bytes.Buffer
	if code := run([]string{"--site", filepath.Join(root, "shell.md"), filepath.Join(root, "docs"),
		"-o", filepath.Join(root, "site")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	b, err := os.ReadFile(filepath.Join(root, "site/md2html-search.js"))
	if err != nil || !strings.Contains(string(b), `"title":"Home"`) {
		t.Errorf("index (err %v):\n%s", err, b)
	}
}

// The site's warnings come out in one order whatever order the pages
// converted in: by file, then by message.
func TestRunSiteWarningsAreSorted(t *testing.T) {
	root := tree(t, map[string]string{
		"shell.md":      "---\nsite: true\n---\n::: zzz\nx\n:::\n\n::: aaa\nx\n:::\n\n[[content]]\n",
		"docs/index.md": "# Home\n",
	})
	var out, errb bytes.Buffer
	if code := run([]string{"--site", filepath.Join(root, "shell.md"), filepath.Join(root, "docs"),
		"-o", filepath.Join(root, "site")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	got := errb.String()
	a, z := strings.Index(got, `"aaa"`), strings.Index(got, `"zzz"`)
	if a < 0 || z < 0 || a > z {
		t.Errorf("want the aaa warning before the zzz one:\n%s", got)
	}
}

// The same site built twice is the same bytes, pages, index and stderr.
func TestRunSiteIsByteIdentical(t *testing.T) {
	root := tree(t, map[string]string{
		"shell.md":            "---\nsite: true\nsearch-facets: kind\n---\n::: zzz\nx\n:::\n\n::: aaa\nx\n:::\n\n[[pages]]\n\n[[breadcrumb]]\n\n[[search]]\n\n[[content]]\n\n[[prev-next]]\n",
		"docs/index.md":       "# Home\n",
		"docs/a/index.md":     "# A\n",
		"docs/a/one.md":       "---\nkind: guide\n---\n# One\n\n::: bogus\nx\n:::\n",
		"docs/a/two.md":       "# Two\n",
		"docs/b/three.md":     "# Three\n",
		"docs/b/c/four.md":    "# Four\n",
		"docs/b/c/five.md":    "# Five\n",
		"docs/b/c/d/six.md":   "# Six\n",
		"docs/b/c/d/seven.md": "# Seven\n",
	})
	build := func(out string) (map[string]string, string) {
		var stdout, errb bytes.Buffer
		if code := run([]string{"--site", filepath.Join(root, "shell.md"), filepath.Join(root, "docs"),
			"-o", filepath.Join(root, out)}, &stdout, &errb); code != 0 {
			t.Fatalf("exit %d, stderr: %s", code, errb.String())
		}
		files := map[string]string{}
		filepath.WalkDir(filepath.Join(root, out), func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				b, _ := os.ReadFile(p)
				rel, _ := filepath.Rel(filepath.Join(root, out), p)
				files[rel] = string(b)
			}
			return nil
		})
		return files, strings.ReplaceAll(errb.String(), filepath.Join(root, out), "OUT")
	}
	first, firstErr := build("site1")
	second, secondErr := build("site2")
	if firstErr != secondErr {
		t.Errorf("stderr differs:\n%s\n---\n%s", firstErr, secondErr)
	}
	if len(first) != len(second) || len(first) < 10 {
		t.Fatalf("%d files then %d", len(first), len(second))
	}
	for name, b := range first {
		if second[name] != b {
			t.Errorf("%s differs between builds", name)
		}
	}
}

// A plugin that provides only grammars has no styles, so --list-styles
// leaves it out rather than printing a heading with nothing beneath it.
func TestListStylesSkipsStylelessPlugins(t *testing.T) {
	root := tree(t, map[string]string{
		"gram/md2html-plugin.json": `{"format": 1, "name": "gram", "grammars": {"fab": {"file": "fab.json"}}}`,
		"gram/fab.json":            `{"scopeName": "source.fab", "patterns": [{"match": "fab", "name": "keyword.fab"}]}`,
	})
	var out, errb bytes.Buffer
	if code := run([]string{"--list-styles", "--plugin", filepath.Join(root, "gram")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if line == "gram" || strings.HasPrefix(line, "gram:") {
			t.Errorf("grammar-only plugin listed:\n%s", out.String())
		}
	}
	if !strings.Contains(out.String(), "default") {
		t.Errorf("styles missing:\n%s", out.String())
	}
}

func TestNoHighlightFlag(t *testing.T) {
	doc := "# A\n\n```go\nfunc main() {}\n```\n"
	for _, tc := range []struct {
		args []string
		want bool
	}{{nil, true}, {[]string{"--no-highlight"}, false}} {
		root := tree(t, map[string]string{"doc.md": doc})
		var out, errb bytes.Buffer
		if code := run(append([]string{filepath.Join(root, "doc.md")}, tc.args...), &out, &errb); code != 0 {
			t.Fatalf("%v: exit %d: %s", tc.args, code, errb.String())
		}
		b, _ := os.ReadFile(filepath.Join(root, "doc.html"))
		if got := strings.Contains(string(b), "md2html-hl"); got != tc.want {
			t.Errorf("%v: md2html-hl present = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestListLanguages(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--list-languages"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	var goLine, shLine bool
	for _, l := range lines {
		if strings.HasPrefix(l, "go ") && strings.Contains(l, "grammars") && strings.Contains(l, "MIT") {
			goLine = true
		}
		if strings.HasPrefix(l, "shellscript (bash, sh, shell, zsh)") {
			shLine = true
		}
	}
	if !goLine {
		t.Errorf("no go line naming grammars and MIT:\n%s", out.String())
	}
	if !shLine {
		t.Errorf("no shellscript line naming its aliases:\n%s", out.String())
	}
	names := make([]string, len(lines))
	for i, l := range lines {
		names[i], _, _ = strings.Cut(l, " ")
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("output is not sorted by language name")
	}
}

const fabPlugin = `{"format": 1, "name": "fabs", "grammars": {"fab": {"file": "fab.json"}}}`
const fabGrammar = `{"scopeName": "source.fab", "patterns": [{"match": "fab", "name": "keyword.fab"}]}`

func TestListLanguagesWithAPlugin(t *testing.T) {
	root := tree(t, map[string]string{
		"gram/md2html-plugin.json": fabPlugin,
		"gram/fab.json":            fabGrammar,
		"bad/md2html-plugin.json":  `{"format": 1, "name": "bad", "grammars": {"x": {"file": "missing.json"}}}`,
	})
	var out, errb bytes.Buffer
	if code := run([]string{"--list-languages", "--plugin", filepath.Join(root, "gram")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "fab  fabs") {
		t.Errorf("plugin language missing:\n%s", out.String())
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"--list-languages", "--plugin", filepath.Join(root, "bad")}, &out, &errb); code != 2 {
		t.Errorf("broken plugin: exit %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "md2html: ") || out.Len() != 0 {
		t.Errorf("stderr %q, stdout %q", errb.String(), out.String())
	}
}

// --list-styles returns before the entry check and ignores the file, so
// --list-languages does too.
func TestListLanguagesTakesNoFiles(t *testing.T) {
	root := tree(t, map[string]string{"page.md": "# A\n"})
	var out, errb bytes.Buffer
	if code := run([]string{"--list-languages", filepath.Join(root, "page.md")}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "grammars") {
		t.Errorf("no listing:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "page.html")); err == nil {
		t.Error("a page was written")
	}
}

// Two plugins providing the same language clash once per run, however the
// run is used, and even when highlighting is off.
func TestLanguageClashWarnsOncePerRun(t *testing.T) {
	root := tree(t, map[string]string{
		"a/md2html-plugin.json": strings.Replace(fabPlugin, "fabs", "fa", 1),
		"a/fab.json":            fabGrammar,
		"b/md2html-plugin.json": strings.Replace(fabPlugin, "fabs", "fb", 1),
		"b/fab.json":            fabGrammar,
		"doc.md":                "# A\n\n```fab\nfab\n```\n",
	})
	plug := filepath.Join(root, "a") + "," + filepath.Join(root, "b")
	for _, args := range [][]string{
		{filepath.Join(root, "doc.md"), "--plugin", plug},
		{filepath.Join(root, "doc.md"), "--plugin", plug, "--no-highlight"},
		{"--list-languages", "--plugin", plug},
	} {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, errb.String())
		}
		if n := strings.Count(errb.String(), "both provide language"); n != 1 {
			t.Errorf("%v: %d clash warnings, want 1:\n%s", args, n, errb.String())
		}
	}
}
