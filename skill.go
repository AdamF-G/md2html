package md2html

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// skillName is the directory the authoring skill installs into, and the
// name an agent lists it under.
const skillName = "md2html-authoring"

// The skill travels inside the binary, so `go install` delivers it along
// with the tool. It is only SKILL.md: the guide it defers to is printed by
// `md2html --guide`, which always matches the binary doing the conversion,
// where a copy installed beside the skill would describe whichever version
// installed it. Both are embedded from where they already live — the skill
// this repo uses on itself, and the authoring guide — rather than from
// copies kept in step by hand.
var (
	//go:embed .claude/skills/md2html-authoring/SKILL.md
	skillDoc []byte
	//go:embed docs/authoring.md
	authoringDoc []byte
)

// retiredSkillFiles are files earlier versions installed into the skill
// directory and this one no longer does. A stale authoring.md beside the
// skill would be read in preference to `md2html --guide`, so an install
// removes each one that carries this tool's marker, and leaves alone one
// that does not.
var retiredSkillFiles = []string{"authoring.md"}

// InstallSkill writes the authoring skill into a "md2html-authoring"
// directory below parent, creating it as needed, and returns the paths
// written in a stable order.
//
// parent is a skills directory, such as Claude Code's "~/.claude/skills" or
// the one any other agent that reads SKILL.md looks in. Whether it ought to
// exist is the caller's question to answer, not this one's: the CLI refuses
// to invent a missing .claude, or a missing directory it was named, because
// a skill installed under a directory nobody reads is worse than no skill
// at all.
//
// The skill carries this tool's provenance marker, so re-installing over
// an earlier version is silent while a copy someone has edited is refused
// by name. The refusal is whole: every destination is checked before
// anything is written, so a run that refuses leaves the directory exactly
// as it found it rather than holding one file from this version beside one
// the user wrote. That check is the reason this lives here rather than in
// the command — a caller assembling the same install from a bag of file
// contents would have to know to do it, and a caller who forgot would get
// half-written skill directories with no sign anything was wrong.
//
// Files an earlier version installed and this one does not, listed in
// retiredSkillFiles, are removed once the skill is written, if ours.
func InstallSkill(parent string) ([]string, error) {
	dir := filepath.Join(parent, skillName)
	files := skillFiles()

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		p := filepath.Join(dir, name)
		ours, err := IsOurs(p)
		if err != nil {
			return nil, err
		}
		if ours {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return nil, fmt.Errorf("refusing to overwrite %s (not installed by md2html)", p)
		}
	}

	written := make([]string, 0, len(names))
	for _, name := range names {
		p := filepath.Join(dir, name)
		if _, err := SafeWrite(p, files[name]); err != nil {
			return written, err
		}
		written = append(written, p)
	}

	for _, name := range retiredSkillFiles {
		p := filepath.Join(dir, name)
		ours, err := IsOurs(p)
		if err != nil {
			return written, err
		}
		if ours {
			if err := os.Remove(p); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

// AuthoringGuide returns the Markdown authoring guide: the syntax md2html
// understands and the traps that render wrong without a warning. It is what
// `md2html --guide` prints, and what the skill sends its reader to.
func AuthoringGuide() []byte {
	return bytes.Clone(authoringDoc)
}

// skillFiles returns the skill's files, keyed by the name each takes inside
// the installed directory, with the provenance marker already in place.
// There is one today; the map keeps InstallSkill's check-everything-first
// refusal in place should that change.
//
// The marker cannot lead SKILL.md the way it leads generated HTML: an agent
// parses that file's YAML front matter, which has to come first, so
// the marker follows the closing fence. IsOurs searches only the first
// markerWindow bytes, and TestInstalledSkillFilesAreRecognisedAsOurs is
// what will notice if the front matter ever grows past it.
func skillFiles() map[string][]byte {
	return map[string][]byte{
		"SKILL.md": markAfterFrontMatter(skillDoc),
	}
}

// markAfterFrontMatter inserts the marker directly below a leading front
// matter block, or at the top of a document that has none.
func markAfterFrontMatter(doc []byte) []byte {
	const fence = "---\n"
	if bytes.HasPrefix(doc, []byte(fence)) {
		if end := bytes.Index(doc[len(fence):], []byte("\n"+fence)); end >= 0 {
			cut := len(fence) + end + len("\n"+fence)
			return []byte(fmt.Sprintf("%s\n%s\n%s", doc[:cut], Marker(), doc[cut:]))
		}
	}
	return []byte(Marker() + "\n\n" + string(doc))
}
