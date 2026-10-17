package md2html

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	gtext "github.com/yuin/goldmark/text"
)

// The guide is read whole by a person, on md2html's documentation site,
// and in pieces by an agent, which needs the quick reference and the
// silent traps every time and any one construct's detail only when it is
// about to write that construct. All of it at once is nine thousand tokens
// of context, and more than some agents keep of a command's output, so
// `md2html --guide` prints the summary and `--guide <name>` one section.
//
// The pieces are files: docs/guide holds the guide's opening in index.md
// and one file per section, so the site shows each section as a page of
// its own, in the order their nav-order gives, and `--guide` reads the
// same files. A section's name is its file's name, which is also the
// anchor it had when the guide was one page, so the names agents learnt
// then still work.
//
//go:embed docs/guide/*.md
var guideFiles embed.FS

// guideOpening is the file holding the guide's title and opening text,
// which comes first and is not a section of its own.
const guideOpening = "index"

// summarySections are the sections GuideSummary prints in full: the ones
// every reader needs before writing anything, the traps above all, which
// give no error to send anyone looking for them.
var summarySections = []string{"quick-reference", "silent-traps"}

// guidePart is one file of the guide: its opening, or one section.
type guidePart struct {
	name      string   // the file's name, less .md
	title     string   // its # heading as written
	subtitles []string // its ## headings, for the index
	order     int      // its nav-order
	text      []byte   // what --guide prints of it
}

// guideParts loads the embedded guide once. The files are the binary's
// own, so one that cannot be read or placed is a fault in the build, which
// TestGuideKeepsItsSectionNames and the other guide tests catch first.
var guideParts = sync.OnceValue(func() []guidePart {
	sub, err := fs.Sub(guideFiles, "docs/guide")
	if err == nil {
		var parts []guidePart
		if parts, err = loadGuide(sub); err == nil {
			return parts
		}
	}
	panic("md2html: the embedded guide: " + err.Error())
})

// loadGuide reads the guide from the .md files at the root of fsys: the
// opening first, then the sections by nav-order. Two sections may not
// share a nav-order: the site breaks such a tie by title and this would by
// file name, so the two orders could differ with nothing to say so. The
// opening is exempt, being first here whatever its nav-order says, though
// it must still have one, as every file here must. Each file loses its
// front matter, which places the page in the site, and any contents
// marker, which lists the page's headings there; a reader of --guide gets
// the index instead.
func loadGuide(fsys fs.FS) ([]guidePart, error) {
	names, err := fs.Glob(fsys, "*.md")
	if err != nil {
		return nil, err
	}
	var parts []guidePart
	taken := map[int]string{} // nav-order -> the section that has it
	for _, file := range names {
		src, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, err
		}
		meta, body, _ := splitFrontMatter(src)
		order, err := strconv.Atoi(meta["nav-order"])
		if err != nil {
			return nil, fmt.Errorf("%s: no whole-number nav-order to place it in the guide", file)
		}
		p := parseGuidePart(body)
		p.name = strings.TrimSuffix(path.Base(file), ".md")
		if p.name != guideOpening {
			if other, ok := taken[order]; ok {
				return nil, fmt.Errorf("%s and %s share nav-order %d; give each section its own", other, file, order)
			}
			taken[order] = file
		}
		p.order = order
		parts = append(parts, p)
	}
	sort.SliceStable(parts, func(i, j int) bool {
		a, b := parts[i], parts[j]
		if (a.name == guideOpening) != (b.name == guideOpening) {
			return a.name == guideOpening
		}
		return a.order < b.order
	})
	return parts, nil
}

// parseGuidePart reads one file's title and headings and cuts out its
// contents markers. Headings and markers inside code blocks, which the
// guide has plenty of in its examples, are example text here too, so this
// follows the parser rather than lines that start with # or read [[toc]].
func parseGuidePart(src []byte) guidePart {
	var p guidePart
	doc := goldmark.New().Parser().Parse(gtext.NewReader(src))
	var cut [][2]int // byte ranges to leave out, in order
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if n.Lines().Len() == 0 {
			continue
		}
		first, last := n.Lines().At(0), n.Lines().At(n.Lines().Len()-1)
		switch n := n.(type) {
		case *ast.Heading:
			title := strings.TrimSpace(string(first.Value(src)))
			switch {
			case n.Level == 1 && p.title == "":
				p.title = title
			case n.Level == 2:
				p.subtitles = append(p.subtitles, title)
			}
		case *ast.Paragraph:
			if n.Lines().Len() == 1 && isTOCMarker(strings.TrimSpace(string(first.Value(src)))) {
				// The marker's line, and the blank line after it.
				end := last.Stop
				for end < len(src) && src[end] == '\n' {
					end++
				}
				cut = append(cut, [2]int{bytes.LastIndexByte(src[:first.Start], '\n') + 1, end})
			}
		}
	}
	var b bytes.Buffer
	at := 0
	for _, c := range cut {
		b.Write(src[at:c[0]])
		at = c[1]
	}
	b.Write(src[at:])
	p.text = append(bytes.TrimRight(b.Bytes(), "\n"), '\n')
	return p
}

// GuideSummary returns the part of the authoring guide to read before
// writing any page: its opening, the quick reference and the silent traps,
// followed by an index naming every other section for GuideSection. It is
// what `md2html --guide` prints.
func GuideSummary() []byte {
	return guideSummary(guideParts())
}

func guideSummary(parts []guidePart) []byte {
	var b bytes.Buffer
	inSummary := map[string]bool{guideOpening: true}
	for _, p := range parts {
		if p.name == guideOpening {
			b.Write(p.text)
			b.WriteString("\n")
		}
	}
	for _, name := range summarySections {
		inSummary[name] = true
		for _, p := range parts {
			if p.name == name {
				b.Write(p.text)
				b.WriteString("\n")
			}
		}
	}

	// The sections' titles are # headings, each its own page on the site,
	// so the index is one too rather than a subsection of the traps. The
	// opening has already said what --guide <name> and --guide all print;
	// what it cannot say is how the sections' links read, each to the file
	// a section is, which is also the name to ask for. The summary's byte
	// budget is why this is no longer than it is.
	b.WriteString("# The rest of the guide\n\n" +
		"A link above to `name.md` is to the section `--guide name` prints,\n" +
		"and `name.md#anchor` to a heading in it.\n\n")
	for _, p := range parts {
		if inSummary[p.name] {
			continue
		}
		fmt.Fprintf(&b, "- `%s`: %s", p.name, p.title)
		if len(p.subtitles) > 0 {
			fmt.Fprintf(&b, " (%s)", strings.Join(p.subtitles, "; "))
		}
		b.WriteString("\n")
	}
	return b.Bytes()
}

// GuideSection returns the section of the authoring guide called name,
// its file's name in docs/guide. It is what `md2html --guide <name>`
// prints; a name matching no section is an error.
func GuideSection(name string) ([]byte, error) {
	return guideSection(guideParts(), name)
}

func guideSection(parts []guidePart, name string) ([]byte, error) {
	for _, p := range parts {
		if p.name == name && name != guideOpening {
			return bytes.Clone(p.text), nil
		}
	}
	return nil, fmt.Errorf("no guide section is called %q; md2html --guide lists them", name)
}
