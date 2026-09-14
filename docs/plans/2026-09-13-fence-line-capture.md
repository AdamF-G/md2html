# Fence Line Capture Implementation Plan

**Goal:** Make the `:::` container parser own its whole fence line, so a container's kind, attributes and title are read at parse time instead of mined out of the rendered document body.

**Architecture:** The fenced-div extension is vendored into `internal/fences` and given one hook, `SplitInfo`, which md2html supplies. The parser consumes the entire fence-line remainder, hands it to that hook, sets the returned attributes on the node, and — when the hook reports a title — appends a title block whose source segment goldmark parses inlines for natively. `container.go` then reads kind and title from the tree instead of from the first paragraph's leading text, and five workaround functions are deleted.

**Tech Stack:** Go 1.26, goldmark v1.8.6, `go.abhg.dev/goldmark/mermaid` v0.6.0, `golang.org/x/net` v0.58.0, `gopkg.in/yaml.v3` v3.0.1. Standard library `testing` only. `github.com/stefanfritsch/goldmark-fences` v1.0.0 is **removed** by Task 1 and must not appear in `go.mod` afterwards.

**Spec:** `docs/specs/2026-09-13-fence-line-capture.md`

## Global Constraints

- Module path is exactly `github.com/AdamF-G/md2html`.
- Vendored code keeps its MIT licence and both copyright lines verbatim: `Copyright (c) 2019 Yusuke Inuzuka`, `Copyright (c) 2022 Stefan Fritsch`.
- **Behaviour-preserving:** every input that works today produces identical output. The three suites are the proof, and all three must be green at the end of every task.
- `gofmt` clean. `go vet ./...` clean. All exported identifiers get doc comments.
- The two `Open` guards that return `parser.NoChildren` for a fence with nothing after the colons are preserved exactly — that ambiguity is what lets a bare `:::` close a container instead of opening one.
- The `data-fence` id that drives nested fences keeps its current generation and lifetime. It must never reach output.
- Both existing warnings keep their exact current wording: `container has no class and no recognizable kind name`, and `unknown container kind %q: emitting an unclassed div (known kinds: %s)`.
- Test commands, run from the repo root unless stated:
  - `go test ./...`
  - `cd e2e && go test -count=1 ./...` (needs Chrome/Chromium)
  - `cd compat && go test -count=1 ./...` (needs pandoc 3.11)

## Two accepted deviations from "behaviour-preserving" — DECIDED

**1. `isNav` is dropped (Task 5).** The vendored renderer turns a container into `<nav>` when its class matches `elem-nav`. md2html documents no such feature, `grep` finds the string nowhere in the repo outside the vendored code, deriving an element from a class name is surprising, and it is the cause of a real leak: `fenceDivs` only collects `<div>`, so `::: {.elem-nav}` emits `<nav data-fence="1" class="elem-nav">` with the library's internal attribute still on it. Dropping `isNav` makes every container a div and closes the leak.

**2. An unknown bare kind with a title keeps its title element.** `::: notakind Some title` warns today and renders `notakind Some title` as body text. Afterwards it still warns, and the title renders as `<p class="container-title">Some title</p>` inside the unclassed div. This is a warned error path, not a working input.

## File Structure

| File | Responsibility |
|---|---|
| `internal/fences/{ast,extend,parser,renderer,gen_random_string}.go` | Vendored fenced-div extension. Owns fence detection, nesting, the fence line, and the title block node. Knows nothing about md2html's container vocabulary. |
| `internal/fences/doc.go` | Provenance: upstream, version, and what was changed. |
| `internal/fences/LICENSE` | Verbatim upstream MIT licence. |
| `fenceinfo.go` (new) | `parseFenceInfo` — md2html's fence-line grammar, and the only place that knows the five spellings. Supplies the `SplitInfo` hook. |
| `fenceinfo_test.go` (new) | Unit tests for that grammar, string in / struct out. |
| `container.go` | Shrinks. Maps kind to classes, promotes a title into `<summary>`, warns. Loses all first-paragraph mining. |
| `md2html.go:100-108` | Import path change; `fences.Extender` gains the hook. |
| `transform.go:23-45` | The `builtins` ordering comment loses the fence-line half of its rationale. |

---

### Task 1: Vendor the extension unchanged

A pure move. No behaviour changes at all, which is what makes every later task's test signal trustworthy.

**Files:**
- Create: `internal/fences/ast.go`, `internal/fences/extend.go`, `internal/fences/parser.go`, `internal/fences/renderer.go`, `internal/fences/gen_random_string.go`, `internal/fences/LICENSE`, `internal/fences/doc.go`
- Modify: `md2html.go` (import), `go.mod`, `go.sum`

- [ ] **Step 1: Copy the five source files and the licence verbatim**

```bash
SRC="$(go env GOMODCACHE)/github.com/stefanfritsch/goldmark-fences@v1.0.0"
mkdir -p internal/fences
cp "$SRC"/ast.go "$SRC"/extend.go "$SRC"/parser.go "$SRC"/renderer.go \
   "$SRC"/gen_random_string.go "$SRC"/LICENSE internal/fences/
chmod u+w internal/fences/*
```

Do not edit the `.go` files in this step. The package clause stays `package fences`.

- [ ] **Step 2: Write the provenance file**

Create `internal/fences/doc.go`:

```go
// Package fences implements the ::: fenced container syntax.
//
// It is vendored from github.com/stefanfritsch/goldmark-fences v1.0.0, MIT
// licensed — see LICENSE in this directory, whose two copyright lines are
// preserved verbatim. Upstream's own lineage is goldmark itself, hence the
// first of them.
//
// It is vendored rather than imported because md2html's brace-free
// "::: kind" form needs the parser to own the whole fence line, and
// upstream's Open leaves everything that is not an attribute block in the
// content stream. See docs/specs/2026-09-13-fence-line-capture.md.
//
// Changes from upstream are confined to:
//   - Open captures the entire fence-line remainder and delegates its
//     grammar to the SplitInfo hook (parser.go).
//   - A FencedContainerTitle block node carries a container's title so
//     goldmark parses its inlines (ast.go, renderer.go).
//   - isNav and the elem-nav class were removed (renderer.go).
package fences
```

Note: as of this step only the first sentence and the vendoring rationale are true; the change list describes Tasks 2 to 5 and is written now so the file is not revisited three times. This is the one place in this plan where a file's content runs ahead of the code.

- [ ] **Step 3: Point md2html at the vendored copy**

In `md2html.go`, change the import:

```go
	fences "github.com/AdamF-G/md2html/internal/fences"
```

The existing use at `md2html.go:108` — `&fences.Extender{}` — is unchanged.

- [ ] **Step 4: Drop the dependency**

```bash
go mod edit -droprequire github.com/stefanfritsch/goldmark-fences
go mod tidy
grep -c goldmark-fences go.mod   # expect 0
```

- [ ] **Step 5: Verify nothing changed**

```bash
gofmt -l . && go vet ./... && go test ./...
cd e2e && go test -count=1 ./... && cd ..
cd compat && go test -count=1 ./... && cd ..
```

Expected: `gofmt -l` prints nothing, vet silent, all three suites PASS. No test should need editing in this task. If one does, stop — the copy was not faithful.

- [ ] **Step 6: Commit**

```bash
git add internal/fences md2html.go go.mod go.sum
git commit -m "$(cat <<'EOF'
refactor: vendor the fenced container extension

A pure move, no behaviour change: the five files and the MIT licence are
copied verbatim from goldmark-fences v1.0.0 and the dependency is dropped.

The next commits need the parser to own the whole ::: fence line, which
upstream's Open cannot do — it parses an attribute block and leaves
anything else in the content stream. Vendoring first keeps that change
readable as a diff against upstream.
EOF
)"
```

---

### Task 2: The `SplitInfo` hook, and the label form owned end to end

The parser starts consuming the fence line, but only for the label form `:::kind[Title]{attrs}` — the most broken spelling (case D in the spec) and the only self-delimiting one. Every other spelling still goes down today's paragraph-mining path, so the suites stay green throughout.

**Files:**
- Modify: `internal/fences/parser.go`, `internal/fences/ast.go`, `internal/fences/renderer.go`, `internal/fences/extend.go`
- Create: `fenceinfo.go`, `fenceinfo_test.go`
- Modify: `md2html.go`, `container.go`
- Test: `container_test.go`, `fenceinfo_test.go`

**Interfaces:**
- Produces: `fences.Info{Kind string, Attrs []fences.Attr, TitleStart, TitleEnd int}`, `fences.Attr{Name, Value string}`, `fences.Extender{SplitInfo func(string) (Info, bool)}`, and md2html's `parseFenceInfo(info string) (fences.Info, bool)`. `TitleStart`/`TitleEnd` are byte offsets into the info string, half-open; `TitleEnd <= TitleStart` means no title.
- Consumes: `parseAttrs(content string) (attrs, bool)` and `splitBraced(s string) (head, content string, ok bool)` from `attrs.go`.

- [ ] **Step 1: Write the failing grammar test**

Create `fenceinfo_test.go`:

```go
package md2html

import "testing"

// title extracts what parseFenceInfo marked as the title, so a test can
// assert on the string rather than on two offsets.
func title(info string, i fenceInfoResult) string {
	if i.TitleEnd <= i.TitleStart {
		return ""
	}
	return info[i.TitleStart:i.TitleEnd]
}

func TestParseFenceInfoLabelForm(t *testing.T) {
	const info = "aside[Why this matters]{#w .compact}"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "aside" {
		t.Errorf("kind = %q, want %q", got.Kind, "aside")
	}
	if s := title(info, got); s != "Why this matters" {
		t.Errorf("title = %q, want %q", s, "Why this matters")
	}
	want := map[string]string{"id": "w", "class": "compact"}
	for _, a := range got.Attrs {
		if want[a.Name] != a.Value {
			t.Errorf("attr %s = %q, want %q", a.Name, a.Value, want[a.Name])
		}
		delete(want, a.Name)
	}
	if len(want) > 0 {
		t.Errorf("missing attributes: %v", want)
	}
}

func TestParseFenceInfoLabelFormNoAttrs(t *testing.T) {
	const info = "aside[Why]"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "aside" {
		t.Errorf("kind = %q, want %q", got.Kind, "aside")
	}
	if s := title(info, got); s != "Why" {
		t.Errorf("title = %q, want %q", s, "Why")
	}
	if len(got.Attrs) != 0 {
		t.Errorf("attrs = %v, want none", got.Attrs)
	}
}

// An unclosed bracket is not a label form. It must fall through as a bare
// kind word so the existing unknown-kind warning still fires on it.
func TestParseFenceInfoUnclosedBracketIsNotALabel(t *testing.T) {
	const info = "aside[Why"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "aside[Why" {
		t.Errorf("kind = %q, want the whole word %q", got.Kind, "aside[Why")
	}
	if s := title(info, got); s != "" {
		t.Errorf("title = %q, want none", s)
	}
}
```

`fenceInfoResult` is a local alias so this test does not depend on the `internal/fences` import path; define it in `fenceinfo.go` in Step 3 as `type fenceInfoResult = fences.Info`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestParseFenceInfo -v`
Expected: FAIL — `undefined: parseFenceInfo`, `undefined: fenceInfoResult`.

- [ ] **Step 3: Add the `Info` type and the hook to the vendored package**

In `internal/fences/ast.go`, append:

```go
// Attr is one attribute to set on a container element.
type Attr struct {
	Name  string
	Value string
}

// Info is the result of splitting a container's fence-line remainder.
//
// TitleStart and TitleEnd are byte offsets into the info string that was
// split, half-open. TitleEnd <= TitleStart means the fence line carried no
// title. Offsets are used rather than a string so the parser can point a
// source segment at the title and let goldmark parse its inlines.
type Info struct {
	Kind       string
	Attrs      []Attr
	TitleStart int
	TitleEnd   int
}

// FencedContainerTitle is a container's title, as written on its fence
// line. It is a block node with lines rather than a raw string so goldmark
// parses its inline markup the same way it would anywhere else.
type FencedContainerTitle struct {
	ast.BaseBlock
}

// Dump implements Node.Dump.
func (n *FencedContainerTitle) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// KindFencedContainerTitle is the NodeKind of FencedContainerTitle.
var KindFencedContainerTitle = ast.NewNodeKind("FencedContainerTitle")

// Kind implements Node.Kind.
func (n *FencedContainerTitle) Kind() ast.NodeKind { return KindFencedContainerTitle }

// NewFencedContainerTitle returns a new FencedContainerTitle node.
func NewFencedContainerTitle() *FencedContainerTitle {
	return &FencedContainerTitle{}
}
```

In `internal/fences/extend.go`, give `Extender` the hook and pass it to the parser:

```go
type Extender struct {
	priority int // optional int != 0. the priority value for parser and renderer. Defaults to 100.

	// SplitInfo splits a fence line's remainder — everything after the
	// colons, trimmed — into a kind word, attributes and the source range
	// of a title. It reports false to leave the line in the content
	// stream, which is what upstream always did.
	//
	// The hook exists so this package stays free of md2html's container
	// vocabulary: the grammar lives in md2html, beside the shared
	// attribute parser it needs.
	SplitInfo func(info string) (Info, bool)
}
```

and in `Extend`, replace the parser registration:

```go
	md.Parser().AddOptions(
		parser.WithBlockParsers(
			util.Prioritized(&fencedContainerParser{splitInfo: e.SplitInfo}, priority),
		),
	)
```

In `internal/fences/parser.go`, give the parser the field:

```go
type fencedContainerParser struct {
	splitInfo func(info string) (Info, bool)
}
```

`NewFencedContainerParser` and `defaultFencedContainerParser` keep working: a nil `splitInfo` means "behave exactly as upstream", which Step 5 relies on.

- [ ] **Step 4: Capture the fence line in `Open`**

In `internal/fences/parser.go`, replace the block that runs from `rest := line[i:]` down to the end of the `attrs, ok := parser.ParseAttributes(reader)` statement with:

```go
	rest := line[i:]
	left := i + util.TrimLeftSpaceLength(rest)
	right := len(line) - 1 - util.TrimRightSpaceLength(rest)

	if left >= right {
		// As above:
		// If there are no attributes we can't create a div because we won't know
		// if a ":::" ends the last fenced container or opens a new one
		return nil, parser.NoChildren
	}

	// ========================================================================== //
	// 	With attributes we construct the node

	info := string(line[left : right+1])
	var parsed Info
	owned := false
	if b.splitInfo != nil {
		parsed, owned = b.splitInfo(info)
	}

	node := NewFencedContainer()
	fenceID := genRandomString(24)
	node.SetAttributeString("data-fence", []byte(fenceID))

	if owned {
		// The whole fence line belongs to the fence: consume it so that
		// nothing on it can be reinterpreted as document content by a
		// definition list marker or a setext underline on the next line.
		reader.Advance(right + 1)
		for _, a := range parsed.Attrs {
			node.SetAttributeString(a.Name, []byte(a.Value))
		}
		if parsed.Kind != "" {
			node.SetAttributeString("data-fence-kind", []byte(parsed.Kind))
		}
		if parsed.TitleEnd > parsed.TitleStart {
			t := NewFencedContainerTitle()
			base := lineSeg.Start + left
			t.Lines().Append(text.NewSegment(base+parsed.TitleStart, base+parsed.TitleEnd))
			node.AppendChild(t)
		}
	} else {
		reader.Advance(left)
		attrs, ok := parser.ParseAttributes(reader)
		if ok {
			for _, attr := range attrs {
				node.SetAttribute(attr.Name, attr.Value)
			}
		}
	}
```

Two details. `reader.Advance` is relative to the reader's current position, which is the start of the line, so `right + 1` lands just past the last non-space character — the same place upstream's `ParseAttributes` left it. And the first statement of `Open` must now keep the line's segment:

```go
	line, lineSeg := reader.PeekLine()
```

- [ ] **Step 5: Render the title node**

In `internal/fences/renderer.go`, register and implement it:

```go
func (r *Renderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindFencedContainer, r.renderFencedContainer)
	reg.Register(KindFencedContainerTitle, r.renderFencedContainerTitle)
}

// renderFencedContainerTitle emits a container's title. The class is the one
// md2html's container transform already uses for a title paragraph, so a
// non-collapsible container needs no further work and a collapsible one is
// recognised by it.
func (r *Renderer) renderFencedContainerTitle(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<p class="container-title">`)
	} else {
		_, _ = w.WriteString("</p>\n")
	}
	return ast.WalkContinue, nil
}
```

- [ ] **Step 6: Write the grammar, label form only**

Create `fenceinfo.go`:

```go
package md2html

import (
	"strings"

	fences "github.com/AdamF-G/md2html/internal/fences"
)

// fenceInfoResult is the vendored package's Info under a local name, so
// tests and helpers in this package need not import it.
type fenceInfoResult = fences.Info

// parseFenceInfo splits a container's fence-line remainder — everything
// after the colons, already trimmed — into its kind word, its attributes
// and the source range of its title.
//
// It reports false for a spelling this package does not yet own, which
// leaves the line in the content stream for the container transform to mine
// out of the first paragraph, exactly as it always did.
//
// Offsets rather than a title string: the parser points a source segment at
// the range so goldmark parses the title's inline markup natively, which is
// what lets a title hold code spans and emphasis without this package
// converting Markdown a second time.
func parseFenceInfo(info string) (fenceInfoResult, bool) {
	out := fenceInfoResult{TitleStart: -1, TitleEnd: -1}

	// Label form: kind[Title] with an optional trailing attribute block.
	open := strings.IndexByte(info, '[')
	if open <= 0 || strings.ContainsAny(info[:open], " \t{") {
		return out, false
	}
	close := strings.IndexByte(info[open+1:], ']')
	if close < 0 {
		return out, false
	}
	close += open + 1

	out.Kind = info[:open]
	out.TitleStart, out.TitleEnd = open+1, close

	if tail := strings.TrimLeft(info[close+1:], " \t"); tail != "" {
		if _, content, ok := splitBraced(tail); ok {
			out.Attrs = fenceAttrs(content)
		} else {
			// Trailing text that is not an attribute block. Not a spelling
			// we own; let the old path see the whole line rather than
			// silently dropping the tail.
			return fenceInfoResult{TitleStart: -1, TitleEnd: -1}, false
		}
	}
	return out, true
}

// fenceAttrs turns an attribute block's contents into the attributes a
// container element carries. It is the shared {#id .class key=value} parser,
// so a container, a bracketed span and a fenced code block all read the
// same grammar.
func fenceAttrs(content string) []fences.Attr {
	a, ok := parseAttrs(content)
	if !ok {
		return nil
	}
	var out []fences.Attr
	if a.id != "" {
		out = append(out, fences.Attr{Name: "id", Value: a.id})
	}
	if len(a.classes) > 0 {
		out = append(out, fences.Attr{Name: "class", Value: strings.Join(a.classes, " ")})
	}
	for k, v := range a.kv {
		out = append(out, fences.Attr{Name: k, Value: v})
	}
	return out
}
```

- [ ] **Step 7: Wire the hook in**

In `md2html.go`, at line 108, replace `&fences.Extender{}` with:

```go
			&fences.Extender{SplitInfo: parseFenceInfo},
```

- [ ] **Step 8: Run the grammar tests**

Run: `go test ./... -run TestParseFenceInfo -v`
Expected: PASS, all three.

- [ ] **Step 9: Teach the transform to read the parsed title**

The label form now arrives with its attributes already on the div, a `data-fence-kind` attribute, and a `<p class="container-title">` first child. `container.go`'s label path must stop mining and start reading. In `Containers`, replace the body of the loop over `fenceDivs(root)` with:

```go
		for _, div := range fenceDivs(root) {
			removeAttr(div, "data-fence")

			if kind, owned := attr(div, "data-fence-kind"); owned {
				removeAttr(div, "data-fence-kind")
				k, known := containerKinds[kind]
				if !known {
					warn(fmt.Sprintf("unknown container kind %q: emitting an unclassed div "+
						"(known kinds: %s)", kind, knownKindList()))
					continue
				}
				applyKindWithTitle(div, k, nil, detachParsedTitle(div))
				continue
			}

			if cls, ok := attr(div, "class"); ok {
				if f := strings.Fields(cls); len(f) > 0 {
					if k, known := containerKinds[f[0]]; known {
						removeClassToken(div, f[0])
						applyKind(div, k, nil)
					}
					continue
				}
			}
			if len(div.Attr) > 0 {
				continue
			}

			p := firstParagraph(div)
			if p == nil {
				warn("container has no class and no recognizable kind name")
				continue
			}
			if kind, isLabel := labelKind(p); isLabel {
				if k, known := containerKinds[kind]; known {
					if label, block, done := detachLabel(p); done {
						applyLabelAttrs(div, block)
						applyKindWithTitle(div, k, p, label)
						continue
					}
				}
			}

			word, rest := firstWord(p.FirstChild.Data)
			k, known := containerKinds[word]
			if !known {
				warn(fmt.Sprintf("unknown container kind %q: emitting an unclassed div "+
					"(known kinds: %s)", word, knownKindList()))
				continue
			}
			p.FirstChild.Data = rest
			applyKind(div, k, p)
		}
```

and add:

```go
// detachParsedTitle removes the title element the fence parser emitted and
// returns its inline children, for applyKindWithTitle to place as a title
// paragraph or a summary.
//
// It returns nil when the fence line carried no title, which is the same
// thing applyKindWithTitle already expects from a titleless container.
func detachParsedTitle(div *html.Node) []*html.Node {
	var tp *html.Node
	for c := div.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) == "" {
			continue
		}
		if c.Type == html.ElementNode && c.DataAtom == atom.P && hasClass(c, "container-title") {
			tp = c
		}
		break
	}
	if tp == nil {
		return nil
	}
	var out []*html.Node
	for c := tp.FirstChild; c != nil; {
		next := c.NextSibling
		tp.RemoveChild(c)
		out = append(out, c)
		c = next
	}
	div.RemoveChild(tp)
	return out
}
```

- [ ] **Step 10: Write the behaviour tests for the label form**

Append to `container_test.go`:

```go
// Case D from the spec: a definition list as the first block used to make
// the label form's whole fence line a <dt>.
func TestContainerLabelFormBeforeDefinitionList(t *testing.T) {
	var warns []string
	got := convert(t, ":::card[Numbers]\nterm\n: def\n:::\n",
		func(s string) { warns = append(warns, s) })
	if len(warns) != 0 {
		t.Errorf("warned: %v\ngot: %s", warns, got)
	}
	if !strings.Contains(got, `<div class="card">`) {
		t.Errorf("no card class\ngot: %s", got)
	}
	if !strings.Contains(got, `<p class="container-title">Numbers</p>`) {
		t.Errorf("title missing\ngot: %s", got)
	}
	if strings.Contains(got, "<dt>card") {
		t.Errorf("fence line was captured by the definition list\ngot: %s", got)
	}
	if !strings.Contains(got, "<dt>term</dt>") {
		t.Errorf("definition list lost\ngot: %s", got)
	}
}

// The title is Markdown, parsed by goldmark rather than re-converted.
func TestContainerLabelFormTitleKeepsInlineMarkup(t *testing.T) {
	got := convert(t, ":::card[Why `code` matters]\nbody\n:::\n", nil)
	if !strings.Contains(got, "<code>code</code>") {
		t.Errorf("title inline markup lost\ngot: %s", got)
	}
}

// A collapsible kind puts the parsed title in its summary.
func TestContainerLabelFormSummaryUsesTitle(t *testing.T) {
	got := convert(t, ":::aside[Why this matters]{#w .compact}\nbody\n:::\n", nil)
	if !strings.Contains(got, "<summary>Why this matters</summary>") {
		t.Errorf("summary does not carry the title\ngot: %s", got)
	}
	if !strings.Contains(got, `id="w"`) || !strings.Contains(got, "compact") {
		t.Errorf("label attributes lost\ngot: %s", got)
	}
}
```

- [ ] **Step 11: Run the full suite**

```bash
gofmt -l . && go vet ./... && go test ./...
```

Expected: PASS, including every pre-existing container test. The three new tests prove case D is fixed.

If `TestContainerLabelFormTitleKeepsInlineMarkup` fails with the literal backticks in the output, goldmark did not parse inlines for the appended block. The fix is to append an `*ast.Paragraph` instead of `FencedContainerTitle` and have `detachParsedTitle` match the div's first `<p>` when a `data-fence-title` marker attribute is set — but try the node first; paragraphs and headings both get inline parsing from lines, and this node is built the same way.

- [ ] **Step 12: Run the other two suites**

```bash
cd e2e && go test -count=1 ./... && cd ..
cd compat && go test -count=1 ./... && cd ..
```

Expected: both PASS, unedited.

- [ ] **Step 13: Commit**

```bash
git add internal/fences fenceinfo.go fenceinfo_test.go md2html.go container.go container_test.go
git commit -m "$(cat <<'EOF'
feat: let the parser own a label-form fence line

The fenced container parser gains a SplitInfo hook and, for the label form
:::kind[Title]{attrs}, consumes the whole fence line: attributes go on the
node, the kind travels as data-fence-kind, and the title becomes a block
node whose inlines goldmark parses.

That fixes the case where a definition list on the next line captured the
fence line and turned ":::card[Numbers]" into a <dt>. The grammar lives in
md2html beside the shared attribute parser, so the vendored package stays
free of the container vocabulary.

The four other spellings still take the old first-paragraph path.
EOF
)"
```

---

### Task 3: Own the braced form, with a title

Fixes the spec's silent failure: a title after `::: {.card}` is absorbed into the body, and a collapsible kind shows its fallback label instead.

**Files:**
- Modify: `fenceinfo.go`, `container.go`
- Test: `fenceinfo_test.go`, `container_test.go`

**Interfaces:**
- Consumes: `parseFenceInfo`, `fenceAttrs`, `detachParsedTitle` from Task 2.
- Produces: no new identifiers.

- [ ] **Step 1: Write the failing tests**

Append to `fenceinfo_test.go`:

```go
func TestParseFenceInfoBracedWithTitle(t *testing.T) {
	const info = "{.card} Why this matters"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "" {
		t.Errorf("kind = %q, want empty — the kind comes from the class", got.Kind)
	}
	if s := title(info, got); s != "Why this matters" {
		t.Errorf("title = %q, want %q", s, "Why this matters")
	}
	if len(got.Attrs) != 1 || got.Attrs[0].Name != "class" || got.Attrs[0].Value != "card" {
		t.Errorf("attrs = %v, want class=card", got.Attrs)
	}
}

func TestParseFenceInfoBracedWithoutTitle(t *testing.T) {
	const info = "{#note .callout .compact}"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if s := title(info, got); s != "" {
		t.Errorf("title = %q, want none", s)
	}
	var id, class string
	for _, a := range got.Attrs {
		switch a.Name {
		case "id":
			id = a.Value
		case "class":
			class = a.Value
		}
	}
	if id != "note" || class != "callout compact" {
		t.Errorf("id = %q, class = %q; want note / \"callout compact\"", id, class)
	}
}

// A brace inside a quoted value must not end the block early.
func TestParseFenceInfoBracedQuotedBrace(t *testing.T) {
	const info = `{.card data-x="a } b"} Title`
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if s := title(info, got); s != "Title" {
		t.Errorf("title = %q, want %q", s, "Title")
	}
}
```

Append to `container_test.go`:

```go
// The spec's silent failure: a title on a braced fence was absorbed into
// the body, and a collapsible kind showed its fallback instead.
func TestContainerBracedFormTakesATitle(t *testing.T) {
	got := convert(t, "::: {.card} Why this matters\nbody\n:::\n", nil)
	if !strings.Contains(got, `<p class="container-title">Why this matters</p>`) {
		t.Errorf("title not recognized\ngot: %s", got)
	}
	if strings.Contains(got, "Why this matters\nbody") {
		t.Errorf("title still absorbed into the body\ngot: %s", got)
	}
}

func TestContainerBracedCollapsibleTakesATitle(t *testing.T) {
	got := convert(t, "::: {.aside} Why this matters\nbody\n:::\n", nil)
	if !strings.Contains(got, "<summary>Why this matters</summary>") {
		t.Errorf("summary shows the fallback, not the title\ngot: %s", got)
	}
	if strings.Contains(got, "<summary>Aside</summary>") {
		t.Errorf("fallback label still used\ngot: %s", got)
	}
}

// Case G stays correct, and an unknown braced class stays inert and silent.
func TestContainerBracedUnknownClassStillInert(t *testing.T) {
	var warns []string
	got := convert(t, "::: {.house-style}\nterm\n: def\n:::\n",
		func(s string) { warns = append(warns, s) })
	if len(warns) != 0 {
		t.Errorf("warned on a deliberate custom class: %v", warns)
	}
	if !strings.Contains(got, `<div class="house-style">`) {
		t.Errorf("custom class lost\ngot: %s", got)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./... -run 'TestParseFenceInfoBraced|TestContainerBraced' -v`
Expected: the three `parseFenceInfo` tests FAIL (`reported false`), `TestContainerBracedFormTakesATitle` and `TestContainerBracedCollapsibleTakesATitle` FAIL, `TestContainerBracedUnknownClassStillInert` PASSES already.

- [ ] **Step 3: Extend the grammar to the braced form**

In `fenceinfo.go`, insert this branch at the top of `parseFenceInfo`, before the label-form branch:

```go
	if strings.HasPrefix(info, "{") {
		content, rest, ok := readBracedPrefix(info)
		if !ok {
			return out, false
		}
		out.Attrs = fenceAttrs(content)
		if t := strings.TrimLeft(rest, " \t"); t != "" {
			out.TitleStart = len(info) - len(t)
			out.TitleEnd = len(info)
		}
		return out, true
	}
```

and add the helper:

```go
// readBracedPrefix reads a leading {...} attribute block, returning its
// contents and whatever followed it.
//
// The scan tracks quotes and backslash escapes rather than searching for
// the first "}", so a brace inside a quoted value — data-x="a } b" — does
// not end the block early. It is splitBraced's discipline applied to a
// leading block rather than a trailing one.
func readBracedPrefix(s string) (content, rest string, ok bool) {
	if len(s) == 0 || s[0] != '{' {
		return "", s, false
	}
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"', '\'':
			quote := s[i]
			i++
			for i < len(s) && s[i] != quote {
				if s[i] == '\\' {
					i++
				}
				i++
			}
		case '}':
			return s[1:i], s[i+1:], true
		}
	}
	return "", s, false
}
```

- [ ] **Step 4: Run the grammar tests**

Run: `go test ./... -run TestParseFenceInfo -v`
Expected: PASS, all six.

- [ ] **Step 5: Let the transform apply a kind named by a class, with a title**

The braced form arrives with no `data-fence-kind`, so it falls to the class branch, which passes `nil` for the title. In `container.go`, change that branch to read the parsed title:

```go
			if cls, ok := attr(div, "class"); ok {
				if f := strings.Fields(cls); len(f) > 0 {
					if k, known := containerKinds[f[0]]; known {
						removeClassToken(div, f[0])
						applyKindWithTitle(div, k, nil, detachParsedTitle(div))
					}
					continue
				}
			}
```

- [ ] **Step 6: Run the full suite**

```bash
gofmt -l . && go vet ./... && go test ./...
```

Expected: PASS. `TestContainerBracedCalloutUnchanged` and the compat suite's braced cases are the regression guard here.

- [ ] **Step 7: Run the other two suites**

```bash
cd e2e && go test -count=1 ./... && cd ..
cd compat && go test -count=1 ./... && cd ..
```

Expected: both PASS.

- [ ] **Step 8: Commit**

```bash
git add fenceinfo.go fenceinfo_test.go container.go container_test.go
git commit -m "$(cat <<'EOF'
feat: accept a title on a braced container fence

"::: {.card} Why this matters" silently absorbed the title into the body,
and "::: {.aside} Why this matters" additionally showed the generic Aside
fallback in its summary while discarding what the author wrote. Neither
warned — the only silent failure in this family.

The parser now owns the braced fence line too: attributes come from the
shared attribute parser, and anything after the block is the title. A
leading brace block is read with the same quote-aware scan splitBraced
uses, so data-x="a } b" does not end the block early.
EOF
)"
```

---

### Task 4: Own the bare forms, and delete the mining

The last two spellings — `::: kind` and `::: kind Title` — move to the parser, which fixes cases A, B and C. Five functions and their tests go with them.

**Files:**
- Modify: `fenceinfo.go`, `container.go`, `transform.go`
- Test: `fenceinfo_test.go`, `container_test.go`

**Interfaces:**
- Consumes: everything from Tasks 2 and 3.
- Produces: `applyKind` is deleted; `applyKindWithTitle(div *html.Node, k containerKind, p *html.Node, title []*html.Node)` becomes `applyKind(div *html.Node, k containerKind, title []*html.Node)`.

- [ ] **Step 1: Write the failing tests**

Append to `fenceinfo_test.go`:

```go
func TestParseFenceInfoBareKind(t *testing.T) {
	const info = "callout"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "callout" {
		t.Errorf("kind = %q, want %q", got.Kind, "callout")
	}
	if s := title(info, got); s != "" {
		t.Errorf("title = %q, want none", s)
	}
}

func TestParseFenceInfoBareKindWithTitle(t *testing.T) {
	const info = "aside Why this matters"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "aside" {
		t.Errorf("kind = %q, want %q", got.Kind, "aside")
	}
	if s := title(info, got); s != "Why this matters" {
		t.Errorf("title = %q, want %q", s, "Why this matters")
	}
}
```

Append to `container_test.go`:

```go
// Cases A, B and C from the spec: a definition list marker or a setext
// underline on the next line used to capture the kind word.
func TestContainerBareKindBeforeDefinitionList(t *testing.T) {
	var warns []string
	got := convert(t, "::: card\nterm\n: def\n:::\n",
		func(s string) { warns = append(warns, s) })
	if len(warns) != 0 {
		t.Errorf("warned: %v\ngot: %s", warns, got)
	}
	if !strings.Contains(got, `<div class="card">`) {
		t.Errorf("kind word was stolen\ngot: %s", got)
	}
	if strings.Contains(got, "<dt>card</dt>") {
		t.Errorf("kind word became a term\ngot: %s", got)
	}
	if !strings.Contains(got, "<dt>term</dt>") {
		t.Errorf("definition list lost\ngot: %s", got)
	}
}

func TestContainerBareKindBeforeSetextHeading(t *testing.T) {
	for _, underline := range []string{"===", "---"} {
		var warns []string
		got := convert(t, "::: card\nHeading text\n"+underline+"\n:::\n",
			func(s string) { warns = append(warns, s) })
		if len(warns) != 0 {
			t.Errorf("%s: warned: %v\ngot: %s", underline, warns, got)
		}
		if !strings.Contains(got, `<div class="card">`) {
			t.Errorf("%s: kind word was stolen\ngot: %s", underline, got)
		}
		if strings.Contains(got, "card\nHeading text") || strings.Contains(got, `id="card-heading-text"`) {
			t.Errorf("%s: kind word reached the heading text or its id\ngot: %s", underline, got)
		}
	}
}

// The undelimited title form keeps working, and a bare ":::" still closes.
func TestContainerBareKindWithTitleUnchanged(t *testing.T) {
	got := convert(t, "::: aside Why this matters\nBecause.\n:::\n", nil)
	if !strings.Contains(got, "<summary>Why this matters</summary>") {
		t.Errorf("title lost\ngot: %s", got)
	}
}

func TestContainerNestingStillWorks(t *testing.T) {
	got := convert(t, ":::: card\n::: callout\ninner\n:::\n::::\n", nil)
	if !strings.Contains(got, `<div class="card">`) || !strings.Contains(got, `<div class="callout">`) {
		t.Errorf("nesting broken\ngot: %s", got)
	}
}

// A ::: line inside a code fence is content, not a container.
func TestContainerInsideCodeFenceStaysLiteral(t *testing.T) {
	got := convert(t, "```markdown\n::: card\nbody\n:::\n```\n", nil)
	if strings.Contains(got, `<div class="card">`) {
		t.Errorf("a fenced code example became a container\ngot: %s", got)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./... -run 'TestParseFenceInfoBare|TestContainerBareKindBefore' -v`
Expected: the two `parseFenceInfo` tests FAIL (`reported false`); `TestContainerBareKindBeforeDefinitionList` and `TestContainerBareKindBeforeSetextHeading` FAIL on the warning and the stolen kind word.

- [ ] **Step 3: Extend the grammar to the bare forms**

In `fenceinfo.go`, replace the label-form branch's two early `return out, false` exits so that a line which is not a label form falls through to a bare-kind branch. The whole function becomes:

```go
func parseFenceInfo(info string) (fenceInfoResult, bool) {
	out := fenceInfoResult{TitleStart: -1, TitleEnd: -1}

	if strings.HasPrefix(info, "{") {
		content, rest, ok := readBracedPrefix(info)
		if !ok {
			return out, false
		}
		out.Attrs = fenceAttrs(content)
		if t := strings.TrimLeft(rest, " \t"); t != "" {
			out.TitleStart = len(info) - len(t)
			out.TitleEnd = len(info)
		}
		return out, true
	}

	// Label form: kind[Title], with an optional trailing attribute block.
	if open := strings.IndexByte(info, '['); open > 0 && !strings.ContainsAny(info[:open], " \t{") {
		if close := strings.IndexByte(info[open+1:], ']'); close >= 0 {
			close += open + 1
			tail := strings.TrimLeft(info[close+1:], " \t")
			var attrs []fences.Attr
			owned := tail == ""
			if !owned {
				if _, content, ok := splitBraced(tail); ok {
					attrs, owned = fenceAttrs(content), true
				}
			}
			if owned {
				out.Kind = info[:open]
				out.TitleStart, out.TitleEnd = open+1, close
				out.Attrs = attrs
				return out, true
			}
		}
	}

	// Bare form: a kind word, optionally followed by an undelimited title
	// running to the end of the line.
	word := info
	if i := strings.IndexAny(info, " \t"); i >= 0 {
		word = info[:i]
		if t := strings.TrimLeft(info[i:], " \t"); t != "" {
			out.TitleStart = len(info) - len(t)
			out.TitleEnd = len(info)
		}
	}
	out.Kind = word
	return out, true
}
```

Note what this does to the unclosed-bracket case from Task 2: `aside[Why` now reaches the bare branch and yields `Kind: "aside[Why"`, which is what `TestParseFenceInfoUnclosedBracketIsNotALabel` already asserts, except that it now reports true rather than false. Update that test's name and body to assert the kind and `ok == true`:

```go
// An unclosed bracket is not a label form. It falls through to the bare
// path as one unusable kind word, so the unknown-kind warning fires on it.
func TestParseFenceInfoUnclosedBracketIsNotALabel(t *testing.T) {
	const info = "aside[Why"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "aside[Why" {
		t.Errorf("kind = %q, want the whole word %q", got.Kind, "aside[Why")
	}
	if s := title(info, got); s != "" {
		t.Errorf("title = %q, want none", s)
	}
}
```

- [ ] **Step 4: Run the grammar tests**

Run: `go test ./... -run TestParseFenceInfo -v`
Expected: PASS, all eight.

- [ ] **Step 5: Delete the mining from the transform**

Every spelling is now owned, so `container.go`'s `Containers` loop reduces to:

```go
	return Transform{Name: "containers", Fn: func(root *html.Node) error {
		for _, div := range fenceDivs(root) {
			removeAttr(div, "data-fence")

			if kind, owned := attr(div, "data-fence-kind"); owned {
				removeAttr(div, "data-fence-kind")
				k, known := containerKinds[kind]
				if !known {
					warn(fmt.Sprintf("unknown container kind %q: emitting an unclassed div "+
						"(known kinds: %s)", kind, knownKindList()))
					continue
				}
				applyKind(div, k, detachParsedTitle(div))
				continue
			}

			if cls, ok := attr(div, "class"); ok {
				if f := strings.Fields(cls); len(f) > 0 {
					if k, known := containerKinds[f[0]]; known {
						removeClassToken(div, f[0])
						applyKind(div, k, detachParsedTitle(div))
					}
					continue
				}
			}
			if len(div.Attr) > 0 {
				continue
			}
			warn("container has no class and no recognizable kind name")
		}
		return nil
	}}
```

Delete these functions entirely: `firstWord`, `firstParagraph`, `labelKind`, `detachLabel`, `applyLabelAttrs`, `detachTitle`. Rename `applyKindWithTitle` to `applyKind` and drop its now-unused `p *html.Node` parameter along with the `if p != nil && p.FirstChild == nil` block that removed an emptied fence-line paragraph — no such paragraph exists any more.

- [ ] **Step 6: Delete the tests that asserted the deleted internals**

```bash
grep -n "firstWord\|detachTitle\|detachLabel\|labelKind\|firstParagraph\|applyLabelAttrs" *_test.go
```

Remove or rewrite each hit. A test that asserted a deleted helper's behaviour goes; a test that asserted an author-visible outcome stays and must still pass through the new path.

- [ ] **Step 7: Correct the ordering rationale**

In `transform.go`, the `builtins` comment claims Containers and Alerts must both precede Chips "so that a fence line's or a marker's brackets are consumed before bracketed-span rewriting could read them". Half of that is no longer true. Replace that clause with:

```go
// Containers must run first because it restructures fenced containers
// before anything else inspects the tree. Alerts follows it as the other
// transform that produces containers, and must precede Chips so a marker's
// brackets are consumed before bracketed-span rewriting could read them.
// A container's own fence line no longer needs that protection: the fence
// parser consumes it, so ":::aside[Why]{.compact}" never reaches the tree
// as text a bracketed span could claim.
```

Then find the guard test that pins this ordering and update its stated reason the same way:

```bash
grep -rn "Containers\|ordering\|before Chips" transform_test.go
```

Keep the ordering assertion — Containers-first is still a correctness constraint for tree restructuring — and correct only the comment explaining why.

- [ ] **Step 8: Run everything**

```bash
gofmt -l . && go vet ./... && go test ./...
cd e2e && go test -count=1 ./... && cd ..
cd compat && go test -count=1 ./... && cd ..
```

Expected: all three PASS.

- [ ] **Step 9: Commit**

```bash
git add fenceinfo.go fenceinfo_test.go container.go container_test.go transform.go transform_test.go
git commit -m "$(cat <<'EOF'
feat: let the parser own every container fence line

The bare "::: kind" and "::: kind Title" spellings move to the parser, so
a definition list marker or a setext underline on the following line can no
longer capture the kind word. The setext cases were the worst of the four:
the kind word reached the heading text and so the generated anchor id,
where a permalink could be minted from it.

With nothing left to recover from the document body, firstWord,
firstParagraph, labelKind, detachLabel, applyLabelAttrs and detachTitle are
deleted, and applyKindWithTitle becomes applyKind.

Containers no longer needs to precede Chips to protect a fence line's
brackets from bracketed-span rewriting; the comment and the ordering guard
say so. Alerts still does, for a marker's brackets.
EOF
)"
```

---

### Task 5: Drop `isNav`, and with it a leaked attribute

**Files:**
- Modify: `internal/fences/renderer.go`
- Test: `container_test.go`

- [ ] **Step 1: Write the failing test**

Append to `container_test.go`:

```go
// The vendored renderer used to turn a container into <nav> when its class
// matched elem-nav — undocumented here, used nowhere, and the cause of a
// real leak: fenceDivs only collects <div>, so the library's internal
// data-fence attribute survived into the output.
func TestContainerElemNavIsAnOrdinaryDiv(t *testing.T) {
	got := convert(t, "::: {.elem-nav}\nbody\n:::\n", nil)
	if strings.Contains(got, "<nav") {
		t.Errorf("still emitting a nav element\ngot: %s", got)
	}
	if strings.Contains(got, "data-fence") {
		t.Errorf("internal data-fence attribute leaked\ngot: %s", got)
	}
	if !strings.Contains(got, `<div class="elem-nav">`) {
		t.Errorf("class lost\ngot: %s", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./... -run TestContainerElemNav -v`
Expected: FAIL — the output contains `<nav data-fence="1" class="elem-nav">`.

- [ ] **Step 3: Remove `isNav`**

In `internal/fences/renderer.go`, delete `isNav`, the `navChk` regexp and the `regexp` import, and reduce the element choice to a constant:

```go
func (r *Renderer) renderFencedContainer(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*FencedContainer)
	if entering {
		n.element = "div"
		if n.Attributes() != nil {
			_, _ = w.WriteString("<" + n.element)
			html.RenderAttributes(w, n, FencedContainerAttributeFilter)
			_, _ = w.WriteString(">\n")
		} else {
			_, _ = w.WriteString("<" + n.element + ">\n")
		}
	} else {
		_, _ = w.WriteString("</" + n.element + ">\n")
	}
	return ast.WalkContinue, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./... -run TestContainerElemNav -v`
Expected: PASS.

- [ ] **Step 5: Run everything**

```bash
gofmt -l . && go vet ./... && go test ./...
cd e2e && go test -count=1 ./... && cd ..
cd compat && go test -count=1 ./... && cd ..
```

Expected: all three PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/fences/renderer.go container_test.go
git commit -m "$(cat <<'EOF'
fix: drop the elem-nav element switch

The vendored renderer turned a container into <nav> when its class matched
elem-nav. md2html documents no such feature and nothing in the repo uses
it; deriving an element from a class name is surprising on its own, and it
leaked: fenceDivs collects only <div>, so such a container kept the
library's internal data-fence attribute all the way into the output.

Every container is a div again, and the leak has nowhere to hide.
EOF
)"
```

---

### Task 6: Documentation

**Files:**
- Modify: `docs/authoring.md`, `docs/specs/2026-09-13-standards-alignment.md`, `CHANGELOG.md`, `.claude/skills/md2html-authoring/SKILL.md`, `README.md`

- [ ] **Step 1: Correct the authoring reference**

In `docs/authoring.md`, find the container section and the claim that the label form is the only spelling that can carry both a title and attributes. Both halves of that are now wrong: every form takes a title, and the braced form takes attributes and a title together. State the four spellings and what each is for, and say that a title is inline Markdown.

Check the Traps section too. Nothing about the definition-list or setext interaction belongs there — it is fixed, not documented — but if the branch added anything about needing a blank line after a fence, remove it.

- [ ] **Step 2: Update the standards alignment spec**

In `docs/specs/2026-09-13-standards-alignment.md`, §5.1's third bullet says the label form collided with bracketed spans and that Containers' ordering is "load-bearing for a second reason". Replace that bullet's last sentence: the collision is gone, because the fence line no longer reaches the tree as text. Point at `docs/specs/2026-09-13-fence-line-capture.md`.

While there, give the "Splitting the `fig` vocabulary" section a status marker consistent with the others, and note that it is now unblocked.

- [ ] **Step 3: Update the CHANGELOG**

Under `## Unreleased`, add to `### Fixed`, creating the subsection if the branch has not already:

```markdown
- A container's `:::` fence line is now consumed by the parser rather than
  recovered from the rendered body. Four spellings previously lost their kind
  word to a definition list marker or a setext underline on the following
  line — the setext cases leaked it into the heading text and the generated
  anchor id — and a title written after a braced fence was silently absorbed
  into the body, with collapsible kinds showing their fallback label
  instead. A title is now accepted on every container form. See
  [docs/specs/2026-09-13-fence-line-capture.md](./docs/specs/2026-09-13-fence-line-capture.md).
- `::: {.elem-nav}` no longer emits a `<nav>` carrying the fence library's
  internal `data-fence` attribute.
```

Under `### Changed`, note that `github.com/stefanfritsch/goldmark-fences` is vendored into `internal/fences` and is no longer a dependency.

- [ ] **Step 4: Update the shipped skill**

In `.claude/skills/md2html-authoring/SKILL.md`, the container section says two forms carry a title and names the label form as the only one that can also carry an id or classes. Replace with: every form carries a title, and both the label form and the braced form carry attributes too. The silent-failures section needs no new entry — this change removes one and adds none.

- [ ] **Step 5: Check the README**

```bash
grep -n "goldmark-fences\|fenced div\|container" README.md
```

Fix any dependency list that still names the external package, and any container description that repeats the title restriction.

- [ ] **Step 6: Verify the docs render**

```bash
go run ./cmd/md2html docs/ -o /tmp/fence-docs
```

Expected: `0 warning(s)`. A warning here means a doc now contains a container spelling that does not parse.

- [ ] **Step 7: Run everything one last time**

```bash
gofmt -l . && go vet ./... && go test ./...
cd e2e && go test -count=1 ./... && cd ..
cd compat && go test -count=1 ./... && cd ..
```

- [ ] **Step 8: Commit**

```bash
git add docs CHANGELOG.md README.md .claude/skills/md2html-authoring/SKILL.md
git commit -m "$(cat <<'EOF'
docs: record that every container form takes a title

The authoring reference, the shipped skill and the README all said the
label form was the only spelling that could carry both a title and
attributes. The parser owning the fence line retires that restriction, and
the standards alignment spec's note about Containers-before-Chips being
load-bearing for a second reason retires with it.
EOF
)"
```

---

## Self-Review

**Spec coverage.** §2's mechanism is what Tasks 2 to 4 replace. §3.1's cases A and B/C are covered by `TestContainerBareKindBeforeDefinitionList` and `TestContainerBareKindBeforeSetextHeading` in Task 4, case D by `TestContainerLabelFormBeforeDefinitionList` in Task 2, and the working controls E through K by the existing suites plus `TestContainerNestingStillWorks` and `TestContainerInsideCodeFenceStaysLiteral`. §4's two silent cases are covered in Task 3. §5.1 is Tasks 1 and 2, §5.2 is the grammar built across Tasks 2 to 4, §5.3 is Task 2's Steps 3 to 5, §5.4's deletions are Task 4's Step 5 and its ordering note is Step 7. §6's test plan is distributed across tasks as listed. §7's "what we keep" items are the Global Constraints.

One item in the spec has no task by design: it says the fix "dissolves the whole family" including the title forms, which Task 4 does, but the spec does not mention `isNav` or the `data-fence` leak — those were found while writing this plan and are Task 5, declared as a deviation above.

**Placeholders.** None. Every code step carries the actual code. Task 6's steps describe edits to prose rather than quoting whole rewritten sections, which is the one place a step says what to say rather than exactly how to word it — acceptable for documentation, and each step names the specific false claim to fix.

**Type consistency.** `fences.Info` and `fences.Attr` are defined in Task 2 Step 3 and used in Tasks 2 to 4. `fenceInfoResult = fences.Info` is introduced in Task 2 Step 6 and used by every `fenceinfo_test.go` test. `parseFenceInfo(string) (fenceInfoResult, bool)` keeps that signature across Tasks 2, 3 and 4. `detachParsedTitle(*html.Node) []*html.Node` is defined in Task 2 Step 9 and reused in Tasks 3 and 4. `applyKindWithTitle(div, k, p, title)` survives Tasks 2 and 3 and becomes `applyKind(div, k, title)` in Task 4 Step 5, which is also where the old three-argument `applyKind` is removed — the rename is called out in that task's Interfaces block because a reader of Task 4 alone would otherwise see two functions of the same name.
