package md2html

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	gtext "github.com/yuin/goldmark/text"
)

// The guide is read whole by a person, on the page md2html renders from
// it, and in pieces by an agent, which needs the quick reference and the
// silent traps every time and any one construct's detail only when it is
// about to write that construct. All of it at once is nine thousand tokens
// of context, and more than some agents keep of a command's output, so
// `md2html --guide` prints the summary and `--guide <name>` one section.
//
// The pieces are cut from the one embedded file at run time, at its `##`
// headings, rather than kept as files of their own, so the rendered guide
// stays one page whose in-page links work. A section's name is its anchor
// on that page.

// summarySections are the sections GuideSummary prints in full: the ones
// every reader needs before writing anything, the traps above all, which
// give no error to send anyone looking for them.
var summarySections = []string{"quick-reference", "silent-traps"}

// guidePart is one `##` section of the guide: its heading, the headings
// below it, and the text it runs over, up to the next `##`.
type guidePart struct {
	name       string   // the heading's anchor
	title      string   // the heading as written
	subtitles  []string // its subsections' headings, for the index
	start, end int      // byte offsets into the guide
}

// guideSections parses the embedded guide once. Headings inside code
// blocks, which the guide has plenty of in its examples, are not headings
// here either, so the cut follows the parser rather than lines that start
// with #.
var guideSections = sync.OnceValue(func() []guidePart {
	return splitGuide(authoringDoc)
})

// splitGuide returns the guide's `##` sections, in document order.
func splitGuide(src []byte) []guidePart {
	doc := goldmark.New().Parser().Parse(gtext.NewReader(src))
	var parts []guidePart
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		h, ok := n.(*ast.Heading)
		if !ok || h.Level < 2 || h.Lines().Len() == 0 {
			continue
		}
		seg := h.Lines().At(0)
		title := strings.TrimSpace(string(seg.Value(src)))
		if h.Level > 2 {
			if len(parts) > 0 && h.Level == 3 {
				last := &parts[len(parts)-1]
				last.subtitles = append(last.subtitles, title)
			}
			continue
		}
		start := bytes.LastIndexByte(src[:seg.Start], '\n') + 1
		if len(parts) > 0 {
			parts[len(parts)-1].end = start
		}
		parts = append(parts, guidePart{
			name:  slugify(strings.ReplaceAll(title, "`", "")),
			title: title,
			start: start,
			end:   len(src),
		})
	}
	return parts
}

// GuideSummary returns the part of the authoring guide to read before
// writing any page: its opening, the quick reference and the silent traps,
// followed by an index naming every other section for GuideSection. It is
// what `md2html --guide` prints.
func GuideSummary() []byte {
	return guideSummary(authoringDoc, guideSections())
}

func guideSummary(src []byte, parts []guidePart) []byte {
	var b bytes.Buffer
	if len(parts) > 0 {
		// The contents marker lists every heading, which the index below
		// does instead, with the names to ask for.
		b.Write(bytes.Replace(src[:parts[0].start], []byte("[[toc]]\n\n"), nil, 1))
	}
	inSummary := map[string]bool{}
	for _, name := range summarySections {
		inSummary[name] = true
		for _, p := range parts {
			if p.name == name {
				b.Write(bytes.TrimRight(src[p.start:p.end], "\n"))
				b.WriteString("\n\n")
			}
		}
	}

	b.WriteString("## The rest of the guide\n\n" +
		"`md2html --guide <name>` prints the section of that name, and\n" +
		"`md2html --guide all` the whole guide. A link above to a `#name`\n" +
		"not listed here is to one of the headings inside a section.\n\n")
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

// GuideSection returns the `##` section of the authoring guide called
// name, its anchor on the rendered page. It is what
// `md2html --guide <name>` prints; a name matching no section is an error.
func GuideSection(name string) ([]byte, error) {
	return guideSection(authoringDoc, guideSections(), name)
}

func guideSection(src []byte, parts []guidePart, name string) ([]byte, error) {
	for _, p := range parts {
		if p.name == name {
			text := bytes.TrimRight(src[p.start:p.end], "\n")
			return append(bytes.Clone(text), '\n'), nil
		}
	}
	return nil, fmt.Errorf("no guide section is called %q; md2html --guide lists them", name)
}
