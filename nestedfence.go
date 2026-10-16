package md2html

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	gtext "github.com/yuin/goldmark/text"
)

// markdownScope is the grammar whose fences can hold fences of their own.
// hostScope names the Markdown grammar md2html derives to tokenize one:
// the same grammar without VS Code's rule per embedded language, each of
// which made the engine compile that language whether or not it was used.
// The derived grammar has a scope of its own so a grammar that includes
// text.html.markdown, such as vue's, still gets the whole one.
const (
	markdownScope = "text.html.markdown"
	hostScope     = "text.html.markdown.md2html-host"
)

// hostGrammar returns raw with each fenced_code_block pattern but the
// last, VS Code's rule for a fence in any language, removed with the
// repository entries they named, under hostScope. It returns nil when raw
// does not have that shape, and the grammar is then used as it is.
func hostGrammar(raw []byte) []byte {
	var g map[string]json.RawMessage
	if json.Unmarshal(raw, &g) != nil {
		return nil
	}
	var repo map[string]json.RawMessage
	if json.Unmarshal(g["repository"], &repo) != nil {
		return nil
	}
	var fcb struct {
		Patterns []struct {
			Include string `json:"include"`
		} `json:"patterns"`
	}
	if json.Unmarshal(repo["fenced_code_block"], &fcb) != nil || len(fcb.Patterns) == 0 {
		return nil
	}
	const keep = "#fenced_code_block_unknown"
	removed := fcb.Patterns[:len(fcb.Patterns)-1]
	if fcb.Patterns[len(fcb.Patterns)-1].Include != keep {
		return nil
	}
	for _, p := range removed {
		name, ok := strings.CutPrefix(p.Include, "#")
		if !ok || !strings.HasPrefix(name, "fenced_code_block_") {
			return nil
		}
		delete(repo, name)
	}
	repo["fenced_code_block"] = json.RawMessage(`{"patterns":[{"include":"` + keep + `"}]}`)
	g["repository"] = mustJSON(repo)
	g["scopeName"] = mustJSON(hostScope)
	out := mustJSON(g)
	// A rule left naming a removed entry, as #name or scope#name, would
	// match nothing in silence.
	for _, p := range removed {
		if bytes.Contains(out, []byte(p.Include+`"`)) {
			return nil
		}
	}
	return out
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // only strings, and maps of JSON already parsed
	}
	return b
}

// innerFence is a fenced code block inside a Markdown fence's lines: its
// info string, the line it opens on, and where each body line's text is.
type innerFence struct {
	info string
	open int
	body []innerLine
}

// innerLine is one body line of an inner fence: lines[line][start:end].
type innerLine struct{ line, start, end int }

// innerFences finds the fenced code blocks in lines as CommonMark does,
// with goldmark's default parser, so a fence in a list item or a block
// quote is found with its markers left out. A body line's text is the
// bytes goldmark gives it, without the spaces it pads a tab with.
func innerFences(lines []string) []innerFence {
	src := []byte(strings.Join(lines, "\n"))
	starts := make([]int, len(lines))
	for i, off := 0, 0; i < len(lines); i++ {
		starts[i] = off
		off += len(lines[i]) + 1
	}
	lineOf := func(off int) int { return sort.SearchInts(starts, off+1) - 1 }
	var out []innerFence
	doc := goldmark.DefaultParser().Parse(gtext.NewReader(src))
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		fc, ok := n.(*ast.FencedCodeBlock)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		f := innerFence{open: -1}
		if fc.Info != nil {
			f.info = string(fc.Info.Segment.Value(src))
			f.open = lineOf(fc.Info.Segment.Start)
		}
		for i := 0; i < fc.Lines().Len(); i++ {
			seg := fc.Lines().At(i)
			l := lineOf(seg.Start)
			end := min(seg.Stop, starts[l]+len(lines[l]))
			f.body = append(f.body, innerLine{l, seg.Start - starts[l], end - starts[l]})
		}
		out = append(out, f)
		return ast.WalkSkipChildren, nil
	})
	return out
}
