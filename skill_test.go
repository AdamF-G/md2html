package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installedPaths returns the path InstallSkill writes under a skills
// parent, and the guide copy earlier versions wrote beside it.
func installedPaths(parent string) (skill, retiredRef string) {
	dir := filepath.Join(parent, "md2html-authoring")
	return filepath.Join(dir, "SKILL.md"), filepath.Join(dir, "authoring.md")
}

func install(t *testing.T, parent string) []string {
	t.Helper()
	got, err := InstallSkill(parent)
	if err != nil {
		t.Fatalf("InstallSkill: %v", err)
	}
	return got
}

// The skill is SKILL.md alone. The guide it defers to comes from
// `md2html --guide`, so no copy of it is installed to go stale.
func TestInstallSkillWritesOnlyTheSkill(t *testing.T) {
	parent := t.TempDir()
	got := install(t, parent)

	skill, ref := installedPaths(parent)
	if len(got) != 1 || got[0] != skill {
		t.Errorf("InstallSkill reported %v, want only %s", got, skill)
	}
	if _, err := os.Stat(skill); err != nil {
		t.Errorf("missing %s: %v", skill, err)
	}
	if _, err := os.Stat(ref); err == nil {
		t.Error("a copy of the guide was installed beside the skill")
	}
}

// Claude Code reads the YAML front matter at the top of SKILL.md, so the
// marker cannot lead the file the way it leads generated HTML.
func TestInstalledSkillBeginsWithFrontMatter(t *testing.T) {
	parent := t.TempDir()
	install(t, parent)
	skill, _ := installedPaths(parent)
	if !strings.HasPrefix(read(t, skill), "---\n") {
		t.Errorf("installed SKILL.md does not open with a front matter fence:\n%.80s", read(t, skill))
	}
}

// Every file this tool writes carries the same provenance marker, so the
// same refusal protects a hand-edited skill as protects a page. IsOurs
// searches only the first markerWindow bytes: asserting through it, rather
// than a bare Contains, is what pins the marker inside that window as
// SKILL.md's front matter grows.
func TestInstalledSkillFilesAreRecognisedAsOurs(t *testing.T) {
	parent := t.TempDir()
	for _, p := range install(t, parent) {
		ours, err := IsOurs(p)
		if err != nil {
			t.Fatalf("IsOurs(%s): %v", p, err)
		}
		if !ours {
			t.Errorf("%s is not recognised as ours; the marker is past the first %d bytes",
				filepath.Base(p), markerWindow)
		}
	}
}

// The marker names the full repository URL, exactly as a generated page's
// does, so a skill directory written by some other tool of the same name is
// never claimed.
func TestInstalledSkillMarkerCarriesRepositoryAndVersion(t *testing.T) {
	parent := t.TempDir()
	for _, p := range install(t, parent) {
		body := read(t, p)
		if !strings.Contains(body, "https://github.com/AdamF-G/md2html") {
			t.Errorf("%s marker does not carry the repository URL", filepath.Base(p))
		}
		if !strings.Contains(body, Version) {
			t.Errorf("%s marker does not carry the version", filepath.Base(p))
		}
	}
}

// The skill's own text must send a reader to `md2html --guide`. Naming
// only "docs/authoring.md" is the failure this guards: that path is real in
// this repo and absent everywhere the skill is installed, so the one reader
// who needs the pointer is the one it fails.
func TestInstalledSkillPointsAtTheGuideFlag(t *testing.T) {
	parent := t.TempDir()
	install(t, parent)
	skill, _ := installedPaths(parent)
	if !strings.Contains(read(t, skill), "`md2html --guide`") {
		t.Error("SKILL.md does not point at md2html --guide")
	}
}

// An upgrade over an install that still carries the guide copy removes it:
// left behind, it would be read in place of the guide for this binary.
func TestInstallSkillRemovesItsRetiredGuideCopy(t *testing.T) {
	parent := t.TempDir()
	_, ref := installedPaths(parent)
	if err := os.MkdirAll(filepath.Dir(ref), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ref, []byte(Marker()+"\n\n# old guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	install(t, parent)
	if _, err := os.Stat(ref); err == nil {
		t.Error("the retired authoring.md from an earlier install is still there")
	}
}

// A file of that name the user wrote is theirs, and survives.
func TestInstallSkillKeepsAHandWrittenAuthoringFile(t *testing.T) {
	parent := t.TempDir()
	_, ref := installedPaths(parent)
	if err := os.MkdirAll(filepath.Dir(ref), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ref, []byte("my notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	install(t, parent)
	if body := read(t, ref); body != "my notes\n" {
		t.Errorf("hand-written authoring.md was changed, now:\n%s", body)
	}
}

// Re-installing over this tool's own copy is how an upgrade lands, and must
// succeed rather than need clearing by hand.
func TestInstallSkillReplacesItsOwnEarlierCopy(t *testing.T) {
	parent := t.TempDir()
	install(t, parent)
	install(t, parent)
}

// A copy the user has edited carries no marker and is never destroyed, and
// the refusal comes before anything else is touched — including removing a
// retired file.
func TestInstallSkillRefusesAHandEditedCopyAndWritesNothing(t *testing.T) {
	parent := t.TempDir()
	skill, ref := installedPaths(parent)
	if err := os.MkdirAll(filepath.Dir(skill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("---\nname: mine\n---\n\nhand written\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ref, []byte(Marker()+"\n\n# old guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := InstallSkill(parent)
	if err == nil {
		t.Fatal("InstallSkill overwrote a hand-edited skill and reported success")
	}
	if !strings.Contains(err.Error(), "refusing") || !strings.Contains(err.Error(), skill) {
		t.Errorf("error names neither the refusal nor the file: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("InstallSkill reported writing %v after refusing", got)
	}
	if body := read(t, skill); !strings.Contains(body, "hand written") {
		t.Errorf("hand-edited SKILL.md was destroyed, now:\n%s", body)
	}
	if _, err := os.Stat(ref); err != nil {
		t.Error("the retired authoring.md was removed although the install was refused")
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --guide prints the repo's own guide, not a summary that drifts: it is
// what the skill sends every reader to.
func TestAuthoringGuideIsTheRepositoryGuide(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("docs", "authoring.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(AuthoringGuide()) != string(want) {
		t.Error("AuthoringGuide is not docs/authoring.md verbatim")
	}
}
