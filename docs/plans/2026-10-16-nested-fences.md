# Fences Inside Markdown Fences Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A fence inside a Markdown fence is highlighted through the page's own language set. The Markdown fence itself is tokenized with a grammar derived without VS Code's 61 per-language fence rules, so nothing is compiled for languages the page does not use.

**Architecture:** `langSet` gains a loader that also serves a derived **host grammar** under the scope `text.html.markdown.md2html-host`. `highlightContext.spans` splits into a lookup and `grammarSpans`. After a Markdown fence is tokenized, `grammarSpans` finds its inner fences with goldmark's default parser, highlights each with `grammarSpans` again, and splices the result over the outer spans. The record of which untranslatable patterns were said moves from the page to the language set.

**Tech Stack:** Go 1.26, goldmark (already a dependency), textmate-go v0.2.2.

**Spec:** `docs/specs/2026-10-16-nested-fences-design.md`, amending `docs/specs/2026-10-14-syntax-highlighting-design.md`.

## Global Constraints

- Branch `syntax-highlighting`. Conventional prefixes. Bodies wrapped at 72 columns, starting from what was wrong before. No attribution trailer. Nothing is pushed.
- `internal/highlight` stays the only importer of textmate-go.
- md2html never fetches. `go test ./...` needs no Node and no network.
- The built-in grammars plugin is not regenerated and does not change.
- Output for a page without a Markdown fence is unchanged, byte for byte. `TestUnchangedOutput` passes untouched.
- No new warning text: an inner fence's faults use the top-level fence messages.
- Run `go vet ./...` and `go test ./...` before every commit; Task 4 also runs `just e2e`.

## Review Focus

1. An inner fence in a block quote or list item: offsets must land on the right bytes of the outer line, or spans split characters or HTML-escape wrongly. Pinned in Task 2's placement test, with `TestHighlightKeepsTextExactly`'s strip-and-compare.
2. A tab before inner code: goldmark records padding, not bytes. The body must be the real bytes. Pinned in Task 2.
3. An inner fence that never closes swallows the rest of the outer body. It must still splice line by line. Pinned in Task 2.
4. A plugin replacing Markdown with a grammar lacking `fenced_code_block`: highlighting must still work, with the grammar used as is. Pinned in Task 1.
5. Parallel pages sharing one language set: the said-diagnostics record must be locked. Pinned by `go test -race` in Task 3.

---

### Task 1: The host grammar

**Files:**
- Create: `nestedfence.go`, `nestedfence_test.go`
- Modify: `langset.go`

**Interfaces:**
- Produces: `const markdownScope = "text.html.markdown"`, `const hostScope = "text.html.markdown.md2html-host"`, `func hostGrammar(raw []byte) []byte` (nil when raw lacks the shape), `func (ls *langSet) load(scope string) ([]byte, error)`, `func (ls *langSet) markdownHost() string` (hostScope, or markdownScope when no host can be derived).

- [ ] **Step 1: Tests.** In `nestedfence_test.go`:
  - `TestHostGrammarDropsTheEmbeddedRules`: from the built-in Markdown grammar, the host has scope `hostScope`, its `fenced_code_block` patterns are exactly `[{"include": "#fenced_code_block_unknown"}]`, no repository key other than `fenced_code_block` and `fenced_code_block_unknown` starts with `fenced_code_block`, and no `"include"` names a scope other than `text.html.derivative` and `source.yaml`.
  - `TestHostGrammarNeedsTheShape`: `hostGrammar` returns nil for `fabGrammar`, for invalid JSON, for a grammar whose `fenced_code_block` does not end in `#fenced_code_block_unknown`, and for one where another rule still includes a removed entry.
  - `TestHostMatchesMarkdownOutsideInnerBodies`: read `internal/highlight/testdata/conformance/markdown.json.gz` (a gzipped `highlight.Dump`); tokenize its lines with the built-in set's engine under `markdownScope` and under `ls.markdownHost()`; for every line not inside an inner fence body (lines from `innerFences` of Task 2, so in Task 1 compare only lines before the sample's first fence opening, found by `strings.HasPrefix(strings.TrimSpace(l), "```")`, and widen it in Task 2), `highlight.Spans` agree.
  - `TestPluginMarkdownWithoutTheShapeIsUsedAsIs`: a local plugin providing `markdown` with scope `text.html.markdown` and a single `{"match": "^#.*", "name": "markup.heading.x"}` pattern; `ls.markdownHost()` is `markdownScope`, and `# t` in a markdown fence gets `hl-heading`.
- [ ] **Step 2:** Run `go test -run 'Host|PluginMarkdown' .`; expect undefined names.
- [ ] **Step 3: Implement.**

```go
// nestedfence.go
package md2html

// markdownScope is the grammar whose fences can hold fences of their own.
// hostScope names the Markdown grammar md2html derives to tokenize one:
// the same grammar without VS Code's rule per embedded language, each of
// which made the engine compile that language whether or not it was used.
// md2html highlights the inner fences itself (see nested). The derived
// grammar has a scope of its own so a grammar that includes
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
	if fcb.Patterns[len(fcb.Patterns)-1].Include != keep {
		return nil
	}
	for _, p := range fcb.Patterns[:len(fcb.Patterns)-1] {
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
	// A rule left naming a removed entry would match nothing in silence.
	for _, p := range fcb.Patterns[:len(fcb.Patterns)-1] {
		if bytes.Contains(out, []byte(`"`+p.Include+`"`)) {
			return nil
		}
	}
	return out
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // only maps, strings and raw JSON already parsed
	}
	return b
}
```

  In `langset.go`, add to `langSet`:

```go
	hostOnce sync.Once
	host     []byte // the derived Markdown host grammar, or nil
```

  and replace the engine's inline loader with `ls.load`:

```go
// load reads the grammar for scope: the derived host grammar, or the file
// of the grammar that serves scope; nil, nil for a scope none does.
func (ls *langSet) load(scope string) ([]byte, error) {
	if scope == hostScope {
		ls.markdownHost()
		return ls.host, nil
	}
	g := ls.byScope[scope]
	if g == nil {
		return nil, nil
	}
	return g.raw()
}

// markdownHost is the scope to tokenize a Markdown fence with: hostScope,
// derived once from whichever grammar serves markdownScope, or
// markdownScope itself when that grammar lacks the shape.
func (ls *langSet) markdownHost() string {
	ls.hostOnce.Do(func() {
		if g := ls.byScope[markdownScope]; g != nil {
			if raw, err := g.raw(); err == nil {
				ls.host = hostGrammar(raw)
			}
		}
	})
	if ls.host == nil {
		return markdownScope
	}
	return hostScope
}
```

  In `highlightContext.spans`, tokenize `scope := g.Scope`, replaced by `h.langs.markdownHost()` when it is `markdownScope`, and ask `Diagnostics` for that same scope. The budget engine in `spans` uses `h.langs.load` too.
- [ ] **Step 4:** `go vet ./... && go test ./...`; all pass.
- [ ] **Step 5: Commit** `feat: Markdown fences are tokenized without the embedded languages`. Body: a Markdown fence compiled every grammar VS Code's 61 per-language rules name; the host grammar has none, and inner fences are left plain until the next commit.

### Task 2: Inner fences

**Files:**
- Modify: `codefence.go`, `nestedfence.go`, `nestedfence_test.go`, `highlight_test.go` (only `budgetScope` in budget tests)

**Interfaces:**
- Consumes: Task 1's `markdownHost`, `load`.
- Produces: `type innerFence struct { info string; open int; body []innerLine }`, `type innerLine struct { line, start, end int }`, `func innerFences(lines []string) []innerFence`, `func splice(outer []highlight.Span, start, end int, inner []highlight.Span) []highlight.Span`, `func (h highlightContext) grammarSpans(g *Grammar, lang string, lines []string, line int, warn func(string)) [][]highlight.Span`, and `highlightContext.budgetScope string` (tests only: the scope the test budget applies to; empty for every scope).

- [ ] **Step 1: Tests** in `nestedfence_test.go`, each converting a page with `convertBody` unless said:
  - `TestInnerFenceIsHighlighted`: ````` ````markdown ````` holding `# T`, a blank, ```` ```go ````, `func main() {}`, ```` ``` ````: output has `hl-heading` and `<span class="hl-keyword">func</span>`, no warnings.
  - `TestInnerFenceAnyLanguage`: inner `zig` (no VS Code rule) gets `hl-keyword` on `const`; inner `fab` from a `--plugin` grammar gets `hl-keyword`; inner `py` (md2html alias) gets `hl-keyword` on `def`.
  - `TestInnerFencePlacement`: inner Go fences in `> ` block quote lines, in a `- ` list item indented two spaces, with `~~~`, unclosed to the end of the outer body, in `{.go}` form, and `{.go .no-highlight}` (plain). For each, stripping spans and ` md2html-hl` gives the `NoHighlight` output, and the keyword span wraps exactly `func`.
  - `TestInnerFenceTab`: a `- ` list item whose fenced line is `\tfunc x() {}` (tab after the list indent): no span splits the tab and `func` is a keyword span; strip-and-compare holds.
  - `TestInnerFenceRecursion`: Go inside markdown inside markdown (outer fence of five backticks, middle four, inner three): `hl-keyword` on `func`.
  - `TestInnerFenceUnknownIsSilent`: inner `pyhton` gives no warning and no span in its body.
  - `TestInnerFenceOverBudget`: `highlightContext{langs: ..., budget: 1, budgetScope: "source.go"}`; `spans("markdown", nil, lines, 10, warn)` with the inner fence opening on body line 2 (index 2): the one warning is `fence at line 13: highlighting "go" took over 2s; showing it plain`, and the outer heading span stays.
  - `TestInnerFenceLoadsOnlyWhatItUses`: a fresh `buildLangSet(builtinGrammarPlugins(), nil)`, its `engine` replaced by `highlight.NewTextMate` over a loader that records each scope and calls `ls.load`. After highlighting a markdown fence with one Go inner fence, the record holds `hostScope` and `source.go` and none of `source.rust`, `source.python`, `source.cpp`, `source.java`, `source.ruby`, `source.php`.
  - `TestInnerFencesMatchTheFullGrammar`: for every built-in grammar, and every one of its names and aliases, build markdown lines `["```" + name, <first lines of its conformance sample, stopping before 12 lines or the first line whose trimmed form starts with "```" or "~~~">, "```"]`. Tokenize with the built-in engine under `markdownScope` (the full grammar). If no token in the body lines has a scope starting with `meta.embedded.block`, VS Code's grammar has no rule for that spelling: skip it. Otherwise `hl.spans("markdown", ...)` must equal `highlight.Spans` of the full tokens line by line, except for languages listed in a `knownDifferences` map with their reason. Also count compared spellings and fail under 50, so a broken detector cannot pass by skipping everything.
  - Widen `TestHostMatchesMarkdownOutsideInnerBodies` to every line outside `innerFences(lines)` bodies.
- [ ] **Step 2:** run them; expect failures (inner fences plain, undefined names).
- [ ] **Step 3: Implement.** In `nestedfence.go`:

```go
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
	doc := goldmark.DefaultParser().Parse(text.NewReader(src))
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

// nested highlights the fences inside a Markdown fence, whose spans are
// out, and puts each body's spans in place of the outer grammar's. An
// inner fence is text, an example, so its language is looked up but never
// rendered as a figure or chart, and an unknown one is not a typo worth
// naming. line is the outer fence's line in the file.
func (h highlightContext) nested(out [][]highlight.Span, lines []string, line int, warn func(string)) {
	for _, f := range innerFences(lines) {
		var inner [][]highlight.Span
		fi := parseCodeFenceInfo(f.info)
		body := make([]string, len(f.body))
		for i, b := range f.body {
			body[i] = lines[b.line][b.start:b.end]
		}
		if g := h.langs.lookup(fi.lang); g != nil && fi.lang != "" && !slices.Contains(fi.classes, "no-highlight") && !allBlank(body) {
			inner = h.grammarSpans(g, fi.lang, body, line+1+f.open, warn)
		}
		for i, b := range f.body {
			var s []highlight.Span
			if inner != nil {
				s = inner[i]
			}
			out[b.line] = splice(out[b.line], b.start, b.end, s)
		}
	}
}

// splice returns outer with [start, end) given inner's spans, which are
// relative to start. A span that crosses an edge keeps its outside part,
// and touching spans of one kind merge, as highlight.Spans merges them.
func splice(outer []highlight.Span, start, end int, inner []highlight.Span) []highlight.Span {
	var out []highlight.Span
	add := func(s highlight.Span) {
		if s.Start >= s.End {
			return
		}
		if n := len(out); n > 0 && out[n-1].Kind == s.Kind && out[n-1].End == s.Start {
			out[n-1].End = s.End
			return
		}
		out = append(out, s)
	}
	for _, s := range outer {
		if s.Start < start {
			add(highlight.Span{Start: s.Start, End: min(s.End, start), Kind: s.Kind})
		}
	}
	for _, s := range inner {
		add(highlight.Span{Start: s.Start + start, End: s.End + start, Kind: s.Kind})
	}
	for _, s := range outer {
		if s.End > end {
			add(highlight.Span{Start: max(s.Start, end), End: s.End, Kind: s.Kind})
		}
	}
	return out
}
```

  In `codefence.go`, `spans` keeps the guard and the lookup with its near-miss warning, then returns `h.grammarSpans(g, lang, lines, line, warn)`. `grammarSpans` holds the rest of today's body (engine choice, tokenizing, faults, diagnostics, spans), runs `h.nested(out, lines, line, warn)` when `g.Scope == markdownScope`, and only then applies the "nothing coloured is plain" rule. The budget engine is used when `h.budget != 0 && (h.budgetScope == "" || h.budgetScope == g.Scope)`.
- [ ] **Step 4:** `go vet ./... && go test ./...`. For each `TestInnerFencesMatchTheFullGrammar` failure, read the diff: a fault in the splice is fixed; a real difference between VS Code's embedding and a top-level fence goes into `knownDifferences` with its reason, and into the spec's Output changes.
- [ ] **Step 5: Commit** `feat: fences inside a Markdown fence are highlighted in any language`.

### Task 3: The untranslatable-pattern warning, once and short

**Files:**
- Modify: `codefence.go`, `langset.go`, `highlight_test.go`

- [ ] **Step 1: Tests.** Rename `TestLocalGrammarDiagnosticsWarnOncePerPage` to `TestLocalGrammarDiagnosticsWarnOncePerSet`: two `Convert` calls sharing one `PluginCache` give one warning between them; a third page with a fence exercising a second bad pattern (a grammar whose second bad pattern is in a `begin`/`end` rule reached only by `(`) gives one more warning, `... 1 pattern cannot be translated and is skipped: ` naming only the new pattern. `TestLocalGrammarDiagnosticIsShortened`: a bad pattern of 300 characters appears as its first 60, ` … `, and its last 90.
- [ ] **Step 2:** run; expect failures.
- [ ] **Step 3: Implement.** Remove `highlightContext.warned` and its initialisation. Add to `langSet`:

```go
	saidMu sync.Mutex
	said   map[string]int // scope -> diagnostics already said
```

  initialised in `buildLangSet`, and:

```go
// unsaid returns the diagnostics in d not yet said for scope, and records
// them as said. The engine's list only grows, so a count is enough.
func (ls *langSet) unsaid(scope string, d []string) []string {
	ls.saidMu.Lock()
	defer ls.saidMu.Unlock()
	n := ls.said[scope]
	if len(d) <= n {
		return nil
	}
	ls.said[scope] = len(d)
	return d[n:]
}
```

  (a nil `said` map is made on first use, for test literals). In `grammarSpans`, the local-grammar branch warns about `h.langs.unsaid(scope, engine.Diagnostics(scope))`, each entry passed through `shortDiagnostic`, which is `abbreviate` from the generator: over 160 runes, the first 60, ` … `, the last 90.
- [ ] **Step 4:** `go vet ./... && go test -race ./...`.
- [ ] **Step 5: Commit** `fix: an untranslatable pattern is said once per language set`. Body: the spec said one warning from the first page, but the record lived with the page, so every page said it, at full length.

### Task 4: Docs and costs

**Files:**
- Modify: `docs/guide/code-blocks.md`, `CHANGELOG.md`, `docs/specs/2026-10-14-syntax-highlighting-design.md`, `docs/specs/2026-10-16-nested-fences-design.md` (status, measured costs)

- [ ] **Step 1:** Measure with a built binary, warm, best of three: the docs site with and without `--no-highlight`; a page with one one-line Markdown fence; one with a Go fence.
- [ ] **Step 2:** Guide, Highlighting section: one paragraph saying a fence inside a Markdown fence is coloured in any language the page knows, under the same names, with `.no-highlight` working there too, and that an unknown language there is silent.
- [ ] **Step 3:** CHANGELOG highlighting entries: the Added bullet gains the inner-fence sentence; the build-time bullet gets the new figures and loses "loading the Markdown grammar" as the main cost.
- [ ] **Step 4:** Highlighting spec: the Faults row for untranslatable patterns reads "Loads; one warning listing them per language set, and one more for any found later"; Costs as built gets the new figures; a line under the status points to the nested-fences spec.
- [ ] **Step 5:** `go test ./...` (the guide summary budget test included) and `just e2e`.
- [ ] **Step 6: Commit** `docs: fences inside Markdown fences in the guide and changelog`.
