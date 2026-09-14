package md2html

import (
	"fmt"
	"strings"
)

// figTreeNode is one node of a tree kind: a label, an optional secondary
// note, an optional emphasis state, and its children.
type figTreeNode struct {
	Label  string
	Note   string
	Accent bool
	Muted  bool
	Kids   []figTreeNode
}

// figTreeSep separates a node's label from its note. The spaces on both
// sides are the whole point: without them this would split "md2html
// --fragment" at the flag, and a repo layout that lists flags is exactly
// what a tree is for.
const figTreeSep = " -- "

// figTreeAccent and figTreeMuted mark a line as emphasised or de-emphasised.
// Both require the trailing space for the same reason, so "*_test.go" stays
// a literal glob rather than becoming an accented "_test.go".
const (
	figTreeAccent = "* "
	figTreeMuted  = "- "
)

// parseFigTree parses a tree kind's block scalar into a node forest.
//
// Nesting is carried by indentation, because that is how a directory
// listing is already written by hand. Depth is a stack of indent widths
// rather than a fixed multiple, so two-space and four-space trees both work
// without the grammar needing to know which it is looking at.
//
// Errors are reported as "line N: reason", counting from the first line of
// the block scalar. The caller prefixes the item's path.
func parseFigTree(body string) ([]figTreeNode, error) {
	// A frame is one open level: the indent of the nodes stored in it, and
	// where to put them.
	type frame struct {
		indent int
		kids   *[]figTreeNode
	}
	var roots []figTreeNode
	// The sentinel's indent is -1 so the first real line, at indent 0, is
	// always deeper than it and opens the first level.
	stack := []frame{{indent: -1, kids: &roots}}

	for n, raw := range strings.Split(body, "\n") {
		line := n + 1
		if strings.TrimSpace(raw) == "" {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if strings.ContainsRune(raw[:indent], '\t') {
			return nil, fmt.Errorf(
				"line %d: a tab in a tree's indentation makes its depth ambiguous; use spaces", line)
		}

		// Popping and opening a level are mutually exclusive. Without
		// tracking that, a dedent that lands between two known levels
		// ("a", "    b", "  c") would silently open a third level instead
		// of being the fault it is.
		popped := false
		for len(stack) > 1 && indent < stack[len(stack)-1].indent {
			stack = stack[:len(stack)-1]
			popped = true
		}
		top := stack[len(stack)-1]
		switch {
		case indent == top.indent:
			// A sibling at the current level.
		case indent > top.indent && !popped:
			// A deeper level, parented by the last node written at the
			// current one. Taking a pointer into that slice is safe
			// because the slice can only grow again after this frame is
			// popped, and popping happens before any append.
			kids := top.kids
			if len(*top.kids) > 0 {
				kids = &(*top.kids)[len(*top.kids)-1].Kids
			}
			stack = append(stack, frame{indent: indent, kids: kids})
		default:
			return nil, fmt.Errorf("line %d: indented to no enclosing level", line)
		}

		dst := stack[len(stack)-1].kids
		*dst = append(*dst, parseFigTreeLine(raw[indent:]))
	}
	return roots, nil
}

// parseFigTreeLine parses one line's markers, label and note.
//
// Escaping needs no code here. Both sigils are matched as literal strings,
// and a backslash breaks those strings by sitting inside them, so "\* x"
// and "a \-- b" simply fail to match. The backslash is left in place: every
// field goes through the inline Markdown pass, which removes it, because
// CommonMark escapes any ASCII punctuation.
func parseFigTreeLine(s string) figTreeNode {
	var n figTreeNode
	switch {
	case strings.HasPrefix(s, figTreeAccent):
		n.Accent, s = true, s[len(figTreeAccent):]
	case strings.HasPrefix(s, figTreeMuted):
		n.Muted, s = true, s[len(figTreeMuted):]
	}
	if i := strings.Index(s, figTreeSep); i >= 0 {
		n.Label = strings.TrimSpace(s[:i])
		n.Note = strings.TrimSpace(s[i+len(figTreeSep):])
		return n
	}
	n.Label = strings.TrimSpace(s)
	return n
}
