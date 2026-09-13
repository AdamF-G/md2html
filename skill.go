package md2html

import (
	"bytes"
	_ "embed"
	"fmt"
)

// SkillName is the directory the authoring skill installs into, and the
// name Claude Code lists it under.
const SkillName = "md2html-authoring"

// The skill travels inside the binary, so `go install` delivers it along
// with the tool and the guidance can never describe a version the reader
// does not have. Both files are embedded from where they already live —
// the skill this repo uses on itself, and the authoring guide it defers to
// — rather than from a copy kept in step by hand.
var (
	//go:embed .claude/skills/md2html-authoring/SKILL.md
	skillDoc []byte
	//go:embed docs/authoring.md
	authoringDoc []byte
)

// SkillFiles returns the authoring skill's files, keyed by the name each
// takes inside the installed skill directory, with this tool's provenance
// marker already in place.
//
// The marker is what lets an installed skill be re-installed silently by a
// later version while a copy someone has edited is refused instead — the
// same rule, and the same SafeWrite, that protects a generated page.
//
// It cannot lead the file the way it leads generated HTML: Claude Code
// parses SKILL.md's YAML front matter, which has to come first, so the
// marker follows the closing fence. IsOurs searches only the first
// markerWindow bytes, and TestInstalledSkillFilesAreRecognisedAsOurs is
// what will notice if the front matter ever grows past it.
func SkillFiles() map[string][]byte {
	return map[string][]byte{
		"SKILL.md":     markAfterFrontMatter(skillDoc),
		"authoring.md": []byte(Marker() + "\n\n" + string(authoringDoc)),
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
