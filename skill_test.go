package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The skill is installed as a directory of files, named the way the
// installed copy refers to them.
func TestSkillFilesAreTheSkillAndItsReference(t *testing.T) {
	got := SkillFiles()
	for _, name := range []string{"SKILL.md", "authoring.md"} {
		if len(got[name]) == 0 {
			t.Errorf("SkillFiles() has no %s (keys: %v)", name, keys(got))
		}
	}
	if len(got) != 2 {
		t.Errorf("SkillFiles() returned %d files, want 2: %v", len(got), keys(got))
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Claude Code reads the YAML front matter at the top of SKILL.md, so the
// marker cannot go where it goes in generated HTML — the file has to still
// begin with its "---" fence.
func TestSkillFileStillBeginsWithFrontMatter(t *testing.T) {
	if !strings.HasPrefix(string(SkillFiles()["SKILL.md"]), "---\n") {
		t.Errorf("SKILL.md does not open with a front matter fence:\n%.80s", SkillFiles()["SKILL.md"])
	}
}

// Every file this tool writes carries the same provenance marker, so the
// same SafeWrite refusal protects a hand-edited skill as protects a page.
// IsOurs only searches the first markerWindow bytes: asserting through it,
// rather than a bare Contains, is what pins the marker inside that window
// as SKILL.md's front matter grows.
func TestInstalledSkillFilesAreRecognisedAsOurs(t *testing.T) {
	dir := t.TempDir()
	for name, content := range SkillFiles() {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, content, 0o644); err != nil {
			t.Fatal(err)
		}
		ours, err := IsOurs(p)
		if err != nil {
			t.Fatalf("IsOurs(%s): %v", name, err)
		}
		if !ours {
			t.Errorf("%s is not recognised as ours; the marker is past the first %d bytes",
				name, markerWindow)
		}
	}
}

// The marker names the full repository URL, exactly as a generated page's
// does, so a skill directory written by some other tool of the same name is
// never claimed or overwritten.
func TestSkillMarkerCarriesTheRepositoryURL(t *testing.T) {
	for name, content := range SkillFiles() {
		if !strings.Contains(string(content), "https://github.com/AdamF-G/md2html") {
			t.Errorf("%s marker does not carry the repository URL", name)
		}
		if !strings.Contains(string(content), Version) {
			t.Errorf("%s marker does not carry the version", name)
		}
	}
}

// The bundled reference is the repo's own authoring guide, not a summary of
// it that drifts: the skill defers to it for every detail, and an installed
// copy has no repo to read it from.
func TestSkillReferenceIsTheRepositoryAuthoringGuide(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("docs", "authoring.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(SkillFiles()["authoring.md"]), string(want)) {
		t.Error("bundled authoring.md is not docs/authoring.md verbatim")
	}
}

// The skill's own text must send a reader to the bundled copy. Naming only
// "docs/authoring.md" is the failure this guards: that path is real in this
// repo and absent everywhere the skill is actually installed, so the one
// reader who needs the pointer is the one it fails. Matched on the bare
// spelling in backticks, which the repo path cannot satisfy.
func TestSkillTextPointsAtTheBundledReference(t *testing.T) {
	if !strings.Contains(string(SkillFiles()["SKILL.md"]), "`authoring.md`") {
		t.Error("SKILL.md names no reference an installed copy can actually open")
	}
}
