# Syntax Highlighting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A fenced code block whose language a loaded TextMate grammar names is highlighted when md2html builds the page, as classed spans coloured by the page style's `--syntax-*` tokens. There is no script, nothing is fetched, and md2html carries no code for any one language.

**Architecture:** A new package, `internal/highlight`, is the only code that imports the engine (`github.com/eugenioenko/textmate-go`). It wraps the engine in a small `Engine` interface, maps scope stacks to a fixed set of thirteen token kinds, and merges tokens into per-line spans. Languages arrive through a new `grammars` key in the plugin manifest. A generated built-in plugin, `plugins/grammars/`, carries about 200 permissively licensed grammars from `tm-grammars`, gzipped. `PluginCache` builds one language set per distinct plugin list and parses each grammar lazily. The fenced-code renderer asks that set for the fence's language and writes spans. A new `highlight.css` goes ahead of the style, only on pages that need it, and each built-in style defines the thirteen colours. `--check-style` checks those colours.

**Tech Stack:** Go 1.26; `github.com/eugenioenko/textmate-go` v0.2.2, which brings `github.com/eugenioenko/regexp2/v2` v2.8.2; `tm-grammars` 1.32.22 from npm; Node (only in `just grammars`) with `vscode-textmate` 9.3.2 and `vscode-oniguruma` 2.0.1 as the reference tokenizer; chromedp in the `e2e` module.

**Spec:** `docs/specs/2026-10-14-syntax-highlighting-design.md`. Read it before starting. Every task argues from it.

## Global Constraints

- Branch: `syntax-highlighting`. Commit with conventional prefixes (`feat:`, `fix:`, `test:`, `docs:`, `chore:`), matching the log. Bodies are wrapped at 72 columns and start from what was wrong or missing before. There is no `Co-Authored-By` or other attribution trailer, and nothing is pushed.
- A fix to an earlier task's own work is `git commit --fixup=<that commit>`, not a new `fix:` commit. One `--autosquash` pass happens in Task 12.
- `internal/highlight` is the only package that imports `github.com/eugenioenko/textmate-go`. A test in Task 1 holds this.
- md2html never fetches, at build time or at run time. Only `just grammars` touches the network, and it is never run by `go test` or by the binary.
- Node is used only by `just grammars`. `go test ./...` must pass on a machine with no Node.
- The engine is pinned to `github.com/eugenioenko/textmate-go v0.2.2` and the collection to `tm-grammars` 1.32.22. The collection's samples come from `shikijs/textmate-grammars-themes` at commit `37edd1b26f18838050661d912334aba0ca7f4931`, the commit that released 1.32.22. The reference tokenizer is `vscode-textmate` 9.3.2 with `vscode-oniguruma` 2.0.1.
- The licence allowlist is exactly: MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, 0BSD, Unlicense, CC0-1.0. Anything else ships only through the reviewed override list in `internal/highlight/gen/overrides.json`.
- Names: `Options.NoHighlight` / `--no-highlight`, `--list-languages`, front matter `highlight: false`, fence class `no-highlight`, the `<code>` class `md2html-hl`, span classes `hl-<kind>`, CSS tokens `--syntax-<kind>`.
- Output for any page without a highlighted fence is byte-identical to v0.12.0, stylesheet included. Every existing test passes unchanged except those this plan names.
- Highlighting adds only `<span>` elements and the `md2html-hl` class. The element's text content is unchanged.
- Output is deterministic: the same input gives the same bytes, including under parallel conversion.
- Limits: a time budget of 2 s per fence (`highlight.DefaultBudget`), and 20,000 runes tokenized per line (`highlight.DefaultMaxLineRunes`).
- Every new warning is a sentence that names the thing and what md2html did instead. Each is quoted in `docs/guide/warnings.md` (Task 11), which `TestGuideWarningsExistInTheCode` checks.
- Messages, docs and tests name no product or project from other repositories, except the upstream components credited in notices and the guide.
- Run `go vet ./...` and `go test ./...` at the repo root before every commit. Long runs redirect to a file, for example `go test ./... > /tmp/md2html-test.txt 2>&1`. Tasks 7, 8 and 10 also run `just e2e`.

## Review Focus

1. **Many pages at once, one grammar.** The CLI converts pages in parallel, and a textmate-go `Grammar` serializes tokenizing behind a lock whose wait counts against `TimeLimit`. Thirty-two goroutines each highlighting a 400-line Go fence must all come back highlighted, with no budget warning and no race. *Pinned by `TestTokenizeConcurrently` (Task 1, under `-race`) and `TestConvertHighlightsInParallel` (Task 6).*
2. **Front matter shifts the line a warning names.** The renderer sees the body after front matter is cut off. A near miss at line 9 of a file with 4 lines of front matter must say `line 9`, not `line 5`. *Pinned by `TestNearMissNamesTheFileLine` (Task 6).*
3. **Non-ASCII and CRLF.** textmate-go returns rune offsets, md2html slices bytes, and goldmark keeps a CRLF line's `\r`. A line holding `"héllo — 世界"` followed by a keyword, in a file with CRLF endings, must keep every character and colour the keyword. *Pinned by `TestTokenizeByteOffsets` (Task 1) and `TestHighlightKeepsTextExactly` (Task 6).*
4. **A grammar-only plugin named in front matter.** `plugins: ./langs` with no `style:` must make the page's languages available and must not warn "has no effect without style". A broken one gives one warning and leaves the page's other fences highlighted with the run's grammars. *Pinned by `TestPagePluginsProvideGrammars` and `TestBrokenPagePluginKeepsRunGrammars` (Task 6).*
5. **Fences that look like code but hold none.** A known-language fence with an empty body, a fence nested in a list item, and one inside a `::: details` container must render byte-identical to today when empty, and keep exact text when not. *Pinned by `TestEmptyFenceIsUnchanged` and `TestHighlightInsideContainers` (Task 6).*

## Decisions this plan makes that the spec left open

Each of these is called out in its task's commit body, so the owner can strike it in review.

- **Untranslatable patterns are reported at first use, not at load.** textmate-go compiles rules lazily, and `Grammar.Diagnostics()` reports only the rules exercised so far. A local grammar's warning therefore comes from the first page that highlights a fence with it, once per grammar per page. (Spec: "Local grammar with untranslatable patterns | Loads; one warning listing them".)
- **A long line keeps its first 20,000 runes highlighted.** textmate-go's own `MaxLineRunes` skips the whole line. The wrapper instead tokenizes the first 20,000 runes and leaves the rest plain, as the spec says.
- **The manifest gains an optional `scope` key.** It is required in built-in plugins, so the 200 built-in grammars are not decompressed at start-up just to learn their scope names. In a local plugin it is optional, and it is an error when it does not match the file's `scopeName`.
- **Injection grammars are not shipped** (the collection's 18 `injections`). Neither md2html nor the reference tokenizer loads them, so conformance compares like with like. Supporting them is a follow-up.
- **`keyword.operator` is plain.** Otherwise `=`, `+` and `:=` take the keyword colour in every language, which no mainstream theme does. This is one row in the kind table, with its own test.
- **The near-miss rule is tighter than the spec's guess** (open question 3). Measured against the built-in names, "edit distance 2, four letters or more" warns on `text` (did you mean `tex`?), `tree`, `data`, `none` and `math`. The plan's rule:
  - edit distance counts a transposition as one edit (optimal string alignment);
  - a tag of 4 to 7 runes may be 1 edit away, and one of 8 or more may be 2;
  - a suggestion is never shorter than the tag;
  - a short fixed list of tags that mean "no language" never warns: `text`, `txt`, `plain`, `plaintext`, `none`, `output`, `tree`, `ascii`, `logs`.

  Checked on 2026-10-15 against 50 common non-language tags, it gave no false alarm. It caught 27 of 30 common typos, missing `golnag`, `rubyy` and `sqll`.
- **`highlight:` in front matter** accepts `true` and `false`. Any other value is warned about and highlighting stays on, as other front matter keys do.

---

## File Structure

| Path | Responsibility |
|---|---|
| `internal/highlight/engine.go` | `Token`, `Engine`, the textmate-go implementation, limits, `CheckGrammar` |
| `internal/highlight/kinds.go` | `Kind`, the scope-to-kind table, `KindOf` |
| `internal/highlight/spans.go` | `Span`, `Spans`: merging a line's tokens into coloured runs |
| `internal/highlight/conformance.go` | The golden dump format and `Compare`, shared by the generator and the test |
| `internal/highlight/conformance_test.go`, `testdata/conformance/*.json.gz` | Every shipped grammar against VS Code's tokenizer |
| `internal/highlight/gen/main.go`, `gen/overrides.json` | The generator: selection, conformance, `plugins/grammars/`, report, notices |
| `internal/highlight/gen/node/package.json`, `package-lock.json`, `reference.mjs` | The reference dump, run only by `just grammars` |
| `plugins/grammars/` | Generated: `md2html-plugin.json`, `*.json.gz`, `REPORT.md` |
| `grammar.go` (md2html) | The `grammars` manifest key, `Grammar`, reading and validating grammar files |
| `langset.go` (md2html) | A page's language set: precedence, the engine per set, lookups, near misses, `Languages` |
| `codefence.go`, `md2html.go`, `stylesheet.go` (md2html) | Highlighting in the fence renderer, `Options.NoHighlight`, front matter, page plugins |
| `highlight.css`, `theme.go`, `page.go` (md2html) | The span rules, embedded and added only when a page needs them |
| `plugins/default/default.css`, `plugins/proof/proof.css`, `plugins/claude-artifact/artifact.css` | `--syntax-*` palettes |
| `internal/stylecheck/check.go` | The thirteen tokens and the separation rule |
| `cmd/md2html/main.go` | `--no-highlight`, `--list-languages`, `--list-styles` skipping style-less plugins |
| `THIRD_PARTY_NOTICES` | The two modules, by hand; the grammars, by the generator |
| `e2e/browser_test.go` | Colours resolve in every theme state |
| `docs/guide/code-blocks.md`, `styles.md`, `silent-traps.md`, `warnings.md`, `README.md`, `CHANGELOG.md` | Documentation |
| `justfile` | `grammars` recipe |

---

### Task 1: The engine wrapper

**Files:**
- Modify: `go.mod`, `go.sum`, `THIRD_PARTY_NOTICES`, `licenses_test.go`
- Create: `internal/highlight/engine.go`, `internal/highlight/engine_test.go`, `internal/highlight/imports_test.go`

**Interfaces:**
- Produces:
  - `highlight.Token{Start, End int; Scopes []string}`, where Start and End are **byte** offsets in the line and Scopes are root first.
  - `highlight.Engine`, with `Tokenize(scopeName string, lines []string) ([][]Token, error)` and `Diagnostics(scopeName string) []string`.
  - `highlight.NewTextMate(load func(scopeName string) ([]byte, error), opt Options) *TextMate`, which implements `Engine`. `load` returns `nil, nil` for a scope it does not have.
  - `highlight.Options{Budget time.Duration; MaxLineRunes int}`. A zero field means `DefaultBudget` (2 s) or `DefaultMaxLineRunes` (20,000).
  - `highlight.ErrBudget`.
  - `highlight.CheckGrammar(data []byte) (scopeName string, err error)`.

- [ ] **Step 1: Add the dependency.**

```bash
go get github.com/eugenioenko/textmate-go@v0.2.2
go mod tidy
```

Expected: `go.mod` requires `github.com/eugenioenko/textmate-go v0.2.2`, and `github.com/eugenioenko/regexp2/v2 v2.8.2` appears as indirect. `github.com/dlclark/regexp2/v2` stays, because goja needs it. Both forks are linked, as the spec's Costs section says.

- [ ] **Step 2: Write the failing tests** in `internal/highlight/engine_test.go`. They use a tiny inline grammar, so they do not depend on the collection.

```go
package highlight

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// A grammar small enough to read: keywords, double-quoted strings that may
// span lines, and line comments.
const toyGrammar = `{
  "scopeName": "source.toy",
  "patterns": [
    {"match": "\\b(func|return)\\b", "name": "keyword.control.toy"},
    {"begin": "\"", "end": "\"", "name": "string.quoted.double.toy",
     "beginCaptures": {"0": {"name": "punctuation.definition.string.begin.toy"}},
     "endCaptures": {"0": {"name": "punctuation.definition.string.end.toy"}}},
    {"match": "//.*$", "name": "comment.line.double-slash.toy"}
  ]
}`

func toyLoader(scope string) ([]byte, error) {
	if scope == "source.toy" {
		return []byte(toyGrammar), nil
	}
	return nil, nil
}

// text returns the slice of line a token covers.
func text(line string, t Token) string { return line[t.Start:t.End] }

func innermost(t Token) string { return t.Scopes[len(t.Scopes)-1] }

func TestTokenizeCarriesStateAcrossLines(t *testing.T) {
	e := NewTextMate(toyLoader, Options{})
	lines := []string{`func f() { return "a`, `b" } // done`}
	got, err := e.Tokenize("source.toy", lines)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d lines, want 2", len(got))
	}
	// The second line opens inside the string the first left open.
	first := got[1][0]
	if text(lines[1], first) != "b" || !strings.HasPrefix(innermost(first), "string.quoted") {
		t.Errorf("line 2 starts with %q %v, want b inside the string", text(lines[1], first), first.Scopes)
	}
	if got[0][0].Scopes[0] != "source.toy" {
		t.Errorf("root scope %q, want source.toy first", got[0][0].Scopes[0])
	}
}

// Offsets are bytes, so a caller can slice the line; textmate-go counts
// runes. A keyword after multi-byte text must still be found exactly.
func TestTokenizeByteOffsets(t *testing.T) {
	e := NewTextMate(toyLoader, Options{})
	line := `"héllo — 世界" return`
	got, err := e.Tokenize("source.toy", []string{line})
	if err != nil {
		t.Fatal(err)
	}
	var joined strings.Builder
	found := false
	for _, tok := range got[0] {
		joined.WriteString(text(line, tok))
		if text(line, tok) == "return" && innermost(tok) == "keyword.control.toy" {
			found = true
		}
	}
	if joined.String() != line {
		t.Errorf("tokens rebuild %q, want %q", joined.String(), line)
	}
	if !found {
		t.Errorf("no keyword token for return in %+v", got[0])
	}
}

func TestTokenizeLongLineKeepsItsHead(t *testing.T) {
	e := NewTextMate(toyLoader, Options{MaxLineRunes: 10})
	line := "return xx" + strings.Repeat("é", 20) + " return"
	got, err := e.Tokenize("source.toy", []string{line})
	if err != nil {
		t.Fatal(err)
	}
	toks := got[0]
	if text(line, toks[0]) != "return" {
		t.Errorf("first token %q, want the head still tokenized", text(line, toks[0]))
	}
	last := toks[len(toks)-1]
	if last.End > len(string([]rune(line)[:10])) {
		t.Errorf("a token ends at byte %d, past the first 10 runes", last.End)
	}
}

func TestTokenizeOverBudget(t *testing.T) {
	e := NewTextMate(toyLoader, Options{Budget: time.Nanosecond})
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = `func f() { return "x" } // y`
	}
	if _, err := e.Tokenize("source.toy", lines); !errors.Is(err, ErrBudget) {
		t.Errorf("err %v, want ErrBudget", err)
	}
}

func TestTokenizeUnknownScope(t *testing.T) {
	e := NewTextMate(toyLoader, Options{})
	if _, err := e.Tokenize("source.nothing", []string{"x"}); err == nil {
		t.Error("no error for a scope the loader does not have")
	}
}

// One engine serves every page of a run at once.
func TestTokenizeConcurrently(t *testing.T) {
	e := NewTextMate(toyLoader, Options{})
	lines := make([]string, 400)
	for i := range lines {
		lines[i] = `func f() { return "x" } // y`
	}
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := e.Tokenize("source.toy", lines)
			if err == nil && len(got) != len(lines) {
				err = errors.New("short result")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestCheckGrammar(t *testing.T) {
	if s, err := CheckGrammar([]byte(toyGrammar)); err != nil || s != "source.toy" {
		t.Errorf("CheckGrammar = %q, %v", s, err)
	}
	for name, bad := range map[string]string{
		"not JSON":    `{`,
		"no scope":    `{"patterns": [{"match": "x"}]}`,
		"no patterns": `{"scopeName": "source.x"}`,
	} {
		if _, err := CheckGrammar([]byte(bad)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// An Oniguruma pattern the engine cannot translate is reported, not fatal.
func TestDiagnosticsAfterUse(t *testing.T) {
	const g = `{"scopeName": "source.bad", "patterns": [
	  {"match": "\\p{Xwd}+", "name": "keyword.bad"},
	  {"match": "ok", "name": "keyword.ok"}]}`
	e := NewTextMate(func(string) ([]byte, error) { return []byte(g), nil }, Options{})
	if _, err := e.Tokenize("source.bad", []string{"ok"}); err != nil {
		t.Fatal(err)
	}
	if len(e.Diagnostics("source.bad")) == 0 {
		t.Error("no diagnostics for an untranslatable pattern")
	}
	if d := e.Diagnostics("source.never-loaded"); d != nil {
		t.Errorf("diagnostics %v for a grammar never loaded", d)
	}
}
```

If `\p{Xwd}` turns out to translate cleanly, replace it with any construct that `oniguruma.Diagnostic` reports in `textmate-go`'s own `regexpsource_test.go`, and say which in a comment.

`internal/highlight/imports_test.go`:

```go
package highlight

import (
	"os/exec"
	"strings"
	"testing"
)

// The engine is swappable because nothing outside this package names it.
func TestOnlyThisPackageImportsTheEngine(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", `{{.ImportPath}} {{join .Imports " "}}`, "github.com/AdamF-G/md2html/...").Output()
	if err != nil {
		t.Skipf("go list: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pkg, imports, _ := strings.Cut(line, " ")
		if strings.Contains(imports, "github.com/eugenioenko/textmate-go") && pkg != "github.com/AdamF-G/md2html/internal/highlight" {
			t.Errorf("%s imports textmate-go; only internal/highlight may", pkg)
		}
	}
}
```

- [ ] **Step 3: Run them.** `go test ./internal/highlight/` should fail because the package does not exist.

- [ ] **Step 4: Implement** `internal/highlight/engine.go`.

```go
// Package highlight tokenizes code with TextMate grammars and sorts the
// tokens into a small, fixed set of kinds. It is the only code in md2html
// that names the engine, so swapping engines means a second Engine and
// nothing else.
package highlight

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
	"unicode/utf8"

	textmate "github.com/eugenioenko/textmate-go"
)

// DefaultBudget is how long one fence may take; DefaultMaxLineRunes is how
// much of one line is tokenized, VS Code's default.
const (
	DefaultBudget       = 2 * time.Second
	DefaultMaxLineRunes = 20_000
)

// ErrBudget is returned when a call runs past its time budget.
var ErrBudget = errors.New("over its time budget")

// Token is one run of a line. Start and End are byte offsets in the line;
// Scopes are root first and must not be modified.
type Token struct {
	Start, End int
	Scopes     []string
}

// Engine tokenizes lines with a grammar named by its scope.
type Engine interface {
	// Tokenize returns one token list per line, carrying the rule
	// stack across lines.
	Tokenize(scopeName string, lines []string) ([][]Token, error)
	// Diagnostics lists the patterns of a loaded grammar that could not
	// be used, each as "pattern: reason". It is nil for a grammar not yet
	// loaded, and grows as more of the grammar is exercised.
	Diagnostics(scopeName string) []string
}

// Options bounds the work of one Tokenize call. Zero fields take the
// defaults.
type Options struct {
	Budget       time.Duration
	MaxLineRunes int
}

// TextMate is the textmate-go Engine. It is safe for concurrent use: the
// registry locks, and each grammar serializes its own tokenizing.
type TextMate struct {
	reg *textmate.Registry
	opt Options

	mu     sync.Mutex
	loaded map[string]*textmate.Grammar
}

// NewTextMate returns an engine that reads grammars through load, which
// returns nil, nil for a scope it does not have. Grammars are read and
// compiled on first use and kept.
func NewTextMate(load func(scopeName string) ([]byte, error), opt Options) *TextMate {
	if opt.Budget <= 0 {
		opt.Budget = DefaultBudget
	}
	if opt.MaxLineRunes <= 0 {
		opt.MaxLineRunes = DefaultMaxLineRunes
	}
	reg := textmate.NewRegistry(textmate.RegistryOptions{
		LoadGrammar: func(scope string) (*textmate.RawGrammar, error) {
			data, err := load(scope)
			if err != nil || data == nil {
				return nil, err
			}
			return textmate.ParseRawGrammar(data)
		},
	})
	return &TextMate{reg: reg, opt: opt, loaded: map[string]*textmate.Grammar{}}
}

func (e *TextMate) grammar(scope string) (*textmate.Grammar, error) {
	g, err := e.reg.LoadGrammar(scope)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, fmt.Errorf("no grammar for scope %s", scope)
	}
	e.mu.Lock()
	e.loaded[scope] = g
	e.mu.Unlock()
	return g, nil
}

func (e *TextMate) Tokenize(scope string, lines []string) ([][]Token, error) {
	g, err := e.grammar(scope)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(e.opt.Budget)
	out := make([][]Token, len(lines))
	var state *textmate.StateStack
	for i, line := range lines {
		left := time.Until(deadline)
		if left <= 0 {
			return nil, ErrBudget
		}
		head := runePrefix(line, e.opt.MaxLineRunes)
		r := g.TokenizeLineWithOptions(head, state, textmate.TokenizeOptions{TimeLimit: left})
		if r.Stopped && r.StoppedReason == textmate.StopReasonTimeLimit {
			return nil, ErrBudget
		}
		out[i] = byteTokens(head, r.Tokens)
		state = r.RuleStack
	}
	return out, nil
}

func (e *TextMate) Diagnostics(scope string) []string {
	e.mu.Lock()
	g := e.loaded[scope]
	e.mu.Unlock()
	if g == nil {
		return nil
	}
	var out []string
	for _, d := range g.Diagnostics() {
		out = append(out, fmt.Sprintf("%s: %s", d.Pattern, d.Message))
	}
	return out
}

// runePrefix is s cut to its first n runes.
func runePrefix(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for range n {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i]
}

// byteTokens converts the engine's rune offsets to byte offsets in line.
func byteTokens(line string, toks []textmate.Token) []Token {
	at := make([]int, 0, len(line)+1) // rune index -> byte offset
	for i := range line {
		at = append(at, i)
	}
	at = append(at, len(line))
	out := make([]Token, len(toks))
	for i, t := range toks {
		out[i] = Token{Start: at[min(t.Start, len(at)-1)], End: at[min(t.End, len(at)-1)], Scopes: t.Scopes}
	}
	return out
}

// CheckGrammar reports a grammar file's scope name, or why it cannot be
// used: not JSON, no scopeName, or no patterns.
func CheckGrammar(data []byte) (string, error) {
	var g struct {
		ScopeName string            `json:"scopeName"`
		Patterns  []json.RawMessage `json:"patterns"`
	}
	if err := json.Unmarshal(data, &g); err != nil {
		return "", fmt.Errorf("not valid JSON: %v", err)
	}
	if g.ScopeName == "" {
		return "", errors.New("has no scopeName")
	}
	if len(g.Patterns) == 0 {
		return "", errors.New("has no patterns")
	}
	return g.ScopeName, nil
}
```

The engine counts the newline it appends, so a token can end one rune past the line. `min(..., len(at)-1)` clamps that to the line's end.

- [ ] **Step 5: Run the tests**, with `go test -race ./internal/highlight/`. Expected: PASS.

- [ ] **Step 6: Notices.** Add to `THIRD_PARTY_NOTICES`, in the existing `== name version (SPDX) ==` form and in module order, each with its full licence text from the module cache:
  - `== github.com/eugenioenko/regexp2/v2 v2.8.2 (MIT) ==`
  - `== github.com/eugenioenko/textmate-go v0.2.2 (MIT) ==`

  Add `"github.com/eugenioenko/regexp2/v2 ", "github.com/eugenioenko/textmate-go "` to the list in `TestThirdPartyNoticesNameEveryBundledComponent`.

- [ ] **Step 7: Run `go vet ./... && go test ./...`**, then commit:

```bash
git add go.mod go.sum internal/highlight THIRD_PARTY_NOTICES licenses_test.go
git commit -m "feat: a TextMate tokenizer behind an internal interface"
```

The body states the first-use diagnostics and long-line decisions from this plan's "Decisions" section.

---

### Task 2: Token kinds and spans

**Files:**
- Create: `internal/highlight/kinds.go`, `internal/highlight/kinds_test.go`, `internal/highlight/spans.go`, `internal/highlight/spans_test.go`

**Interfaces:**
- Consumes: `highlight.Token` (Task 1).
- Produces:
  - `highlight.Kind` (a `uint8`), with `Plain` and the thirteen kinds `Comment, Keyword, String, Number, Constant, Function, Type, Property, Tag, Attribute, Inserted, Deleted, Heading`, in that order.
  - `Kind.Name() string`, for example `"keyword"`, and `""` for `Plain`.
  - `Kind.Class() string`, for example `"hl-keyword"`, and `""` for `Plain`.
  - `highlight.Kinds []Kind`: the thirteen, in order.
  - `highlight.KindOf(scopes []string) Kind`.
  - `highlight.Span{Start, End int; Kind Kind}` and `highlight.Spans(toks []Token) []Span`, byte offsets, coloured runs only.

- [ ] **Step 1: Write the failing tests.** `kinds_test.go` has one case per table row plus the walk rules:

```go
package highlight

import "testing"

func TestKindOfEachRow(t *testing.T) {
	for _, c := range []struct {
		scope string
		want  Kind
	}{
		{"comment.line.number-sign.python", Comment},
		{"keyword.control.go", Keyword},
		{"storage.type.function.go", Keyword},
		{"keyword.operator.assignment.go", Plain}, // plan decision: operators are plain
		{"string.quoted.double.go", String},
		{"constant.numeric.integer.go", Number},
		{"constant.language.boolean.json", Constant},
		{"support.constant.property-value.css", Constant},
		{"variable.language.this.js", Constant},
		{"constant.character.escape.go", Constant},
		{"entity.name.function.go", Function},
		{"support.function.builtin.go", Function},
		{"entity.name.type.go", Type},
		{"entity.name.class.python", Type},
		{"support.type.primitive.ts", Type},
		{"support.class.builtin.python", Type},
		{"entity.other.inherited-class.python", Type},
		{"support.type.property-name.json", Property},
		{"variable.other.property.js", Property},
		{"entity.name.tag.html", Tag},
		{"entity.name.tag.yaml", Tag}, // no per-language exceptions
		{"entity.other.attribute-name.html", Attribute},
		{"markup.inserted.diff", Inserted},
		{"markup.deleted.diff", Deleted},
		{"markup.heading.markdown", Heading},
		{"meta.diff.header.from-file", Heading},
		{"meta.diff.range.unified", Heading},
		{"variable.other.readwrite.go", Plain},
		{"str", Plain}, // prefixes are on dot boundaries
	} {
		if got := KindOf([]string{"source.x", c.scope}); got != c.want {
			t.Errorf("%s: %v, want %v", c.scope, got.Name(), c.want.Name())
		}
	}
}

func TestKindOfWalk(t *testing.T) {
	for _, c := range []struct {
		name   string
		scopes []string
		want   Kind
	}{
		{"a string's quote takes string", []string{"source.go", "string.quoted.double.go", "punctuation.definition.string.begin.go"}, String},
		{"a comment's marker takes comment", []string{"source.python", "comment.line.number-sign.python", "punctuation.definition.comment.python"}, Comment},
		{"meta never matches on its own", []string{"source.go", "meta.function.go"}, Plain},
		{"innermost match wins", []string{"source.go", "string.quoted.go", "constant.character.escape.go"}, Constant},
		{"meta.embedded stops the walk", []string{"source.python", "string.interpolated.python", "meta.embedded.expression.python"}, Plain},
		{"meta.embedded still lets inner scopes match", []string{"source.python", "string.interpolated.python", "meta.embedded.expression.python", "constant.numeric.python"}, Number},
		{"an embedded source stops the walk", []string{"source.shell", "string.unquoted.heredoc.shell", "source.python"}, Plain},
		{"the root never stops it", []string{"source.go"}, Plain},
		{"empty", nil, Plain},
	} {
		if got := KindOf(c.scopes); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got.Name(), c.want.Name())
		}
	}
}

func TestKindNamesAndClasses(t *testing.T) {
	if len(Kinds) != 13 {
		t.Fatalf("%d kinds, want 13", len(Kinds))
	}
	want := []string{"comment", "keyword", "string", "number", "constant", "function", "type",
		"property", "tag", "attribute", "inserted", "deleted", "heading"}
	for i, k := range Kinds {
		if k.Name() != want[i] || k.Class() != "hl-"+want[i] {
			t.Errorf("kind %d: %q %q", i, k.Name(), k.Class())
		}
	}
	if Plain.Name() != "" || Plain.Class() != "" {
		t.Error("Plain has a name")
	}
}
```

`spans_test.go`:

```go
package highlight

import (
	"reflect"
	"testing"
)

func TestSpansMergeAndSkipPlain(t *testing.T) {
	str := []string{"source.x", "string.quoted.x"}
	q := []string{"source.x", "string.quoted.x", "punctuation.definition.string.begin.x"}
	kw := []string{"source.x", "keyword.control.x"}
	plain := []string{"source.x"}
	toks := []Token{
		{0, 6, kw}, {6, 7, plain}, {7, 8, q}, {8, 11, str}, {11, 12, q}, {12, 13, plain}, {13, 19, kw},
	}
	got := Spans(toks)
	want := []Span{{0, 6, Keyword}, {7, 12, String}, {13, 19, Keyword}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Spans = %v, want %v", got, want)
	}
}

func TestSpansDoNotMergeAcrossAGap(t *testing.T) {
	kw := []string{"source.x", "keyword.x"}
	got := Spans([]Token{{0, 2, kw}, {3, 5, kw}})
	if len(got) != 2 {
		t.Errorf("Spans = %v, want two spans", got)
	}
}
```

- [ ] **Step 2: Run them.** They should fail to compile, because `KindOf` and `Spans` do not exist.

- [ ] **Step 3: Implement** `kinds.go`:

```go
package highlight

import "strings"

// Kind is what a token is, for colouring: one of a small fixed set, or
// Plain.
type Kind uint8

const (
	Plain Kind = iota
	Comment
	Keyword
	String
	Number
	Constant
	Function
	Type
	Property
	Tag
	Attribute
	Inserted
	Deleted
	Heading
)

// Kinds are every kind but Plain, in order.
var Kinds = []Kind{Comment, Keyword, String, Number, Constant, Function, Type,
	Property, Tag, Attribute, Inserted, Deleted, Heading}

var kindNames = [...]string{"", "comment", "keyword", "string", "number", "constant", "function", "type",
	"property", "tag", "attribute", "inserted", "deleted", "heading"}

// Name is the kind's name, "" for Plain.
func (k Kind) Name() string { return kindNames[k] }

// Class is the kind's span class, "" for Plain.
func (k Kind) Class() string {
	if k == Plain {
		return ""
	}
	return "hl-" + kindNames[k]
}

// rules map scope selectors to kinds. A selector matches a scope that is
// it or starts with it and a dot, and the longest match wins. No rule
// names a language suffix: TextMate grammars share these names, so one
// table serves every grammar. A rule for Plain is a decided "no colour",
// which still ends the walk.
//
// Changing this table changes output; record it in the changelog.
var rules = []struct {
	selector string
	kind     Kind
}{
	{"comment", Comment},
	{"keyword", Keyword},
	{"keyword.operator", Plain},
	{"storage", Keyword},
	{"string", String},
	{"constant", Constant},
	{"constant.numeric", Number},
	{"support.constant", Constant},
	{"variable.language", Constant},
	{"entity.name.function", Function},
	{"support.function", Function},
	{"entity.name.type", Type},
	{"entity.name.class", Type},
	{"support.type", Type},
	{"support.class", Type},
	{"entity.other.inherited-class", Type},
	{"support.type.property-name", Property},
	{"variable.other.property", Property},
	{"entity.name.tag", Tag},
	{"entity.other.attribute-name", Attribute},
	{"markup.inserted", Inserted},
	{"markup.deleted", Deleted},
	{"markup.heading", Heading},
	{"meta.diff.header", Heading},
	{"meta.diff.range", Heading},
}

// KindOf walks a scope stack from the innermost scope out. The first scope
// any rule matches decides the kind. A meta.embedded scope, or a source
// scope below the root, ends the walk with Plain, so code embedded in a
// string is not coloured as the string.
func KindOf(scopes []string) Kind {
	for i := len(scopes) - 1; i >= 0; i-- {
		s := scopes[i]
		if i > 0 && (under(s, "meta.embedded") || under(s, "source")) {
			return Plain
		}
		if k, ok := match(s); ok {
			return k
		}
	}
	return Plain
}

func match(scope string) (Kind, bool) {
	best, kind := -1, Plain
	for _, r := range rules {
		if under(scope, r.selector) && len(r.selector) > best {
			best, kind = len(r.selector), r.kind
		}
	}
	return kind, best >= 0
}

// under reports whether scope is sel or lies beneath it on a dot boundary.
func under(scope, sel string) bool {
	return scope == sel || strings.HasPrefix(scope, sel+".")
}
```

And `spans.go`:

```go
package highlight

// Span is a coloured run of a line, in byte offsets. Text between spans is
// plain.
type Span struct {
	Start, End int
	Kind       Kind
}

// Spans sorts a line's tokens into kinds and merges touching tokens of one
// kind, so a string and its quotes are one span. Plain tokens give none.
func Spans(toks []Token) []Span {
	var out []Span
	for _, t := range toks {
		k := KindOf(t.Scopes)
		if k == Plain || t.Start == t.End {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Kind == k && out[n-1].End == t.Start {
			out[n-1].End = t.End
			continue
		}
		out = append(out, Span{t.Start, t.End, k})
	}
	return out
}
```

- [ ] **Step 4: Run** `go test ./internal/highlight/`. Expected: PASS.

- [ ] **Step 5: Commit.** `git add internal/highlight && git commit -m "feat: token kinds from scope names, and spans per line"`. The body notes the `keyword.operator` decision.

---

### Task 3: The `grammars` manifest key

**Files:**
- Create: `grammar.go`, `grammar_test.go`
- Modify: `plugin.go` (the `Plugin` struct, `parseManifest`), `pluginload.go` (`loadDir`), `plugin_test.go` (the two "provides nothing" rows), `cmd/md2html/main.go` (`listAvailableStyles`), `cmd/md2html/main_test.go`

**Interfaces:**
- Consumes: `highlight.CheckGrammar` (Task 1).
- Produces:
  - `md2html.Grammar` with exported fields `Name string`, `Aliases []string`, `License string`, `Source string` and `Scope string`, and unexported `plugin *Plugin` and `file string`.
  - `(*Grammar).PluginName() string`.
  - `(*Grammar).raw() ([]byte, error)`, which reads the file and gunzips a `.gz`.
  - `Plugin.Grammars map[string]*Grammar` and `(*Plugin).GrammarNames() []string`, sorted.
  - `langName`, the name regexp.

- [ ] **Step 1: Write the failing tests** in `grammar_test.go`. They reuse `pluginDir` from `pluginload_test.go`.

```go
package md2html

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

const fabGrammar = `{"scopeName": "source.fab", "patterns": [{"match": "\\bfab\\b", "name": "keyword.fab"}]}`

const fabManifest = `{"format": 1, "name": "house", "grammars": {
  "fabcfg": {"file": "grammars/fab.json", "aliases": ["fab", "c++", "f#", "objective-c", "x.y"]}}}`

func TestGrammarOnlyPluginLoads(t *testing.T) {
	root := pluginDir(t, map[string]string{
		"p/md2html-plugin.json":   fabManifest,
		"p/grammars/fab.json":     fabGrammar,
	})
	p, err := NewPluginCache().Load("p", root, nil)
	if err != nil {
		t.Fatal(err)
	}
	g := p.Grammars["fabcfg"]
	if g == nil || g.Scope != "source.fab" || len(g.Aliases) != 5 || g.PluginName() != "house" {
		t.Fatalf("grammar %+v", g)
	}
	if len(p.Styles) != 0 {
		t.Errorf("styles %v", p.Styles)
	}
}

func TestGzippedGrammarLoads(t *testing.T) {
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	zw.Write([]byte(fabGrammar))
	zw.Close()
	root := pluginDir(t, map[string]string{
		"p/md2html-plugin.json": `{"format": 1, "name": "house", "grammars": {"fabcfg": {"file": "fab.json.gz"}}}`,
		"p/fab.json.gz":         b.String(),
	})
	p, err := NewPluginCache().Load("p", root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Grammars["fabcfg"].Scope != "source.fab" {
		t.Errorf("scope %q", p.Grammars["fabcfg"].Scope)
	}
}

func TestGrammarManifestErrors(t *testing.T) {
	for _, c := range []struct{ name, manifest, file, want string }{
		{"no file", `{"format": 1, "name": "h", "grammars": {"x": {}}}`, "", `grammar "x" has no file`},
		{"bad name", `{"format": 1, "name": "h", "grammars": {"X Y": {"file": "g.json"}}}`, fabGrammar, `language name "X Y"`},
		{"bad alias", `{"format": 1, "name": "h", "grammars": {"x": {"file": "g.json", "aliases": ["9x"]}}}`, fabGrammar, `alias "9x"`},
		{"missing file", `{"format": 1, "name": "h", "grammars": {"x": {"file": "nope.json"}}}`, "", "nope.json"},
		{"not json", `{"format": 1, "name": "h", "grammars": {"x": {"file": "g.json"}}}`, `{`, "not valid JSON"},
		{"no scope", `{"format": 1, "name": "h", "grammars": {"x": {"file": "g.json"}}}`, `{"patterns": [{}]}`, "no scopeName"},
		{"no patterns", `{"format": 1, "name": "h", "grammars": {"x": {"file": "g.json"}}}`, `{"scopeName": "source.x"}`, "no patterns"},
		{"scope mismatch", `{"format": 1, "name": "h", "grammars": {"x": {"file": "g.json", "scope": "source.y"}}}`, fabGrammar, `scope "source.y"`},
		{"outside", `{"format": 1, "name": "h", "grammars": {"x": {"file": "../g.json"}}}`, "", "outside the plugin"},
		{"empty grammars", `{"format": 1, "name": "h", "grammars": {}}`, "", "provides nothing"},
	} {
		files := map[string]string{"p/md2html-plugin.json": c.manifest}
		if c.file != "" {
			files["p/g.json"] = c.file
		}
		_, err := NewPluginCache().Load("p", pluginDir(t, files), nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err %v, want it to mention %q", c.name, err, c.want)
		}
	}
}

func TestUnknownGrammarKeyWarns(t *testing.T) {
	root := pluginDir(t, map[string]string{
		"p/md2html-plugin.json": `{"format": 1, "name": "h", "grammars": {"x": {"file": "g.json", "colour": "red"}}}`,
		"p/g.json":              fabGrammar,
	})
	var w []string
	if _, err := NewPluginCache().Load("p", root, func(s string) { w = append(w, s) }); err != nil {
		t.Fatal(err)
	}
	if len(w) != 1 || !strings.Contains(w[0], `unknown key "colour" in grammar "x"`) {
		t.Errorf("warnings %q", w)
	}
}
```

In `plugin_test.go`, change the expected text of the two "provides nothing" rows to `"provides nothing"` (unchanged substring), and add one row: `{"styles and grammars empty", `{"format": 1, "name": "a", "styles": {}, "grammars": {}}`, "provides nothing"}`.

In `cmd/md2html/main_test.go`, add `TestListStylesSkipsStylelessPlugins`. It runs `--list-styles` and requires that no output line is exactly `grammars` or starts with `grammars:`. Use the test file's existing `run(...)` helper, the way the other `--list-styles` tests call it.

- [ ] **Step 2: Run them.** Expected: FAIL. The `grammars` key is unknown, and a manifest with no styles is refused.

- [ ] **Step 3: Implement.** In `plugin.go`:
  - Add `Grammars map[string]*Grammar` to `Plugin`. Update its comment: a plugin provides styles, grammars, or both.
  - In `parseManifest`, initialise `Grammars: map[string]*Grammar{}`. Take `"grammars"` into `map[string]map[string]json.RawMessage` before the unknown-key loop. Replace the provides-nothing check with:

```go
	if len(styles) == 0 && len(grammars) == 0 {
		return fail("provides nothing: %s needs a non-empty styles or grammars", ManifestName)
	}
```

  - After the styles loop, add `if err := parseGrammars(p, grammars, where, builtin, warn); err != nil { return nil, err }`.

`grammar.go`:

```go
package md2html

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/AdamF-G/md2html/internal/highlight"
)

// langName is the shape of a language name or alias: real fence tags
// include c++, c# and objective-c. Fences match it ignoring case.
var langName = regexp.MustCompile(`^[a-z][a-z0-9+#.-]*$`)

// Grammar is one language a plugin provides: a TextMate grammar file, the
// names a fence can use for it, and where it came from.
type Grammar struct {
	Name    string
	Aliases []string
	// License is an SPDX identifier and Source is where the grammar came
	// from, both shown by --list-languages. Built-in grammars always have
	// both.
	License, Source string
	// Scope is the grammar's scopeName, such as source.go.
	Scope string

	plugin *Plugin
	file   string
}

// PluginName is the name of the plugin the grammar comes from.
func (g *Grammar) PluginName() string { return g.plugin.Name }

// GrammarNames returns the plugin's language names in sorted order.
func (p *Plugin) GrammarNames() []string { return sortedJSONKeys(p.Grammars) }

// raw reads the grammar file, decompressing a .gz.
func (g *Grammar) raw() ([]byte, error) {
	b, err := g.plugin.readFile(g.file)
	if err != nil || !strings.HasSuffix(g.file, ".gz") {
		return b, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %s: %w", g.plugin.Name, g.file, err)
	}
	return io.ReadAll(zr)
}

// parseGrammars reads a manifest's grammars block into p. A built-in must
// give each grammar's scope, licence and source, so nothing is read until a
// page needs it; a local plugin's files are read and checked at load, in
// loadDir.
func parseGrammars(p *Plugin, grammars map[string]map[string]json.RawMessage, where string, builtin bool, warn func(string)) error {
	fail := func(format string, a ...any) error {
		return fmt.Errorf("plugin %s: "+format, append([]any{where}, a...)...)
	}
	for _, name := range sortedJSONKeys(grammars) {
		raw := grammars[name]
		if !langName.MatchString(name) {
			return fail("language name %q is not lowercase letters, digits, -, +, # and . starting with a letter", name)
		}
		g := &Grammar{Name: name, plugin: p}
		for _, f := range []struct {
			key string
			dst any
		}{{"file", &g.file}, {"aliases", &g.Aliases}, {"license", &g.License}, {"source", &g.Source}, {"scope", &g.Scope}} {
			if _, err := take(raw, f.key, f.dst); err != nil {
				return fail("grammar %q: %s: %v", name, f.key, err)
			}
		}
		if g.file == "" {
			return fail("grammar %q has no file", name)
		}
		for _, a := range g.Aliases {
			if !langName.MatchString(a) {
				return fail("grammar %q: alias %q is not lowercase letters, digits, -, +, # and . starting with a letter", name, a)
			}
		}
		if builtin && (g.Scope == "" || g.License == "" || g.Source == "") {
			return fail("built-in grammar %q needs scope, license and source", name)
		}
		for _, k := range sortedJSONKeys(raw) {
			if warn != nil {
				warn(fmt.Sprintf("plugin %s: unknown key %q in grammar %q; ignoring it", where, k, name))
			}
		}
		p.Grammars[name] = g
	}
	return nil
}

// checkGrammars reads each of a local plugin's grammar files, so a broken
// one is an error when the plugin loads, as a missing CSS file is.
func (p *Plugin) checkGrammars() error {
	for _, name := range p.GrammarNames() {
		g := p.Grammars[name]
		data, err := g.raw()
		if err != nil {
			return fmt.Errorf("plugin %s: grammar %q: %w", p.Name, name, err)
		}
		scope, err := highlight.CheckGrammar(data)
		if err != nil {
			return fmt.Errorf("plugin %s: grammar %q: %s %v", p.Name, name, g.file, err)
		}
		if g.Scope != "" && g.Scope != scope {
			return fmt.Errorf("plugin %s: grammar %q: scope %q, but %s says %q", p.Name, name, g.Scope, g.file, scope)
		}
		g.Scope = scope
	}
	return nil
}

// sortedGrammars returns gs sorted by name; listings use it.
func sortedGrammars(gs []*Grammar) []*Grammar {
	sort.Slice(gs, func(i, j int) bool { return gs[i].Name < gs[j].Name })
	return gs
}
```

`readFile` already wraps errors in `plugin <name>:`. If that doubles the prefix in `checkGrammars`, use the unwrapped `readContained` there. Pick whichever reads cleanly in the "missing file" case.

In `pluginload.go`'s `loadDir`, after `p.loadChartFonts()`, add:

```go
	if err := p.checkGrammars(); err != nil {
		return nil, err
	}
```

`builtinPlugins` does not call it. Task 5's `TestBuiltinGrammarsAreValid` checks every built-in instead.

In `cmd/md2html/main.go`'s `listAvailableStyles`, skip a plugin with no styles: `if len(p.Styles) == 0 { continue }`. A grammar-only plugin is listed by `--list-languages` (Task 9).

- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.

- [ ] **Step 5: Commit.** `git add grammar.go grammar_test.go plugin.go pluginload.go plugin_test.go cmd/md2html && git commit -m "feat: a plugin can provide languages with a grammars key"`. The body notes the `scope` key decision.

---

### Task 4: Language sets

**Files:**
- Create: `langset.go`, `langset_test.go`
- Modify: `pluginload.go` (`PluginCache` gains `langs`)

**Interfaces:**
- Consumes: `Grammar`, `Plugin.Grammars`, `(*Grammar).raw` (Task 3); `highlight.Engine`, `highlight.NewTextMate`, `highlight.Options` (Task 1).
- Produces:
  - `(*PluginCache).languages(plugins []*Plugin, warn func(string)) *langSet`. The built-in `grammars` plugin is always first, and the result is cached by the list of plugin keys.
  - `(*langSet).lookup(lang string) *Grammar`, which ignores case and returns nil when nothing matches.
  - `(*langSet).nearMiss(lang string) string`, which returns `""` when there is none.
  - `langSet.engine highlight.Engine`.
  - `(*langSet).local(scope string) *Grammar`: the grammar that serves `scope`, when it comes from a local plugin, else nil.
  - `md2html.Languages(cache *PluginCache, plugins []*Plugin, warn func(string)) []*Grammar`: every language a fence can name, sorted, overridden ones left out.

- [ ] **Step 1: Write the failing tests** in `langset_test.go`. Until Task 5 there is no built-in `grammars` plugin, so the tests use local plugins only, and `languages` must cope with the built-in being absent. Also add one test that a built-in grammar loses to a local one. It builds a fake built-in `*Plugin` with `key: "builtin:grammars"`, `dir: ""` and `fsys: fstest.MapFS{...}`, and passes it first in the list.

```go
package md2html

import (
	"strings"
	"testing"
	"testing/fstest"
)

func housePlugin(t *testing.T, name, manifestGrammars string, files map[string]string) *Plugin {
	t.Helper()
	all := map[string]string{name + "/md2html-plugin.json": `{"format": 1, "name": "` + name + `", "grammars": ` + manifestGrammars + `}`}
	for k, v := range files {
		all[name+"/"+k] = v
	}
	p, err := NewPluginCache().Load(name, pluginDir(t, all), nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func fakeBuiltin() *Plugin {
	p := &Plugin{Name: "grammars", key: "builtin:grammars", Styles: map[string]*Style{},
		fsys: fstest.MapFS{"go.json": {Data: []byte(`{"scopeName": "source.go", "patterns": [{"match": "func", "name": "keyword.go"}]}`)}}}
	p.Grammars = map[string]*Grammar{"go": {Name: "go", Aliases: []string{"golang"}, Scope: "source.go",
		License: "MIT", Source: "https://example.org/go", plugin: p, file: "go.json"}}
	return p
}

func TestLookupByNameAndAliasIgnoringCase(t *testing.T) {
	p := housePlugin(t, "house", `{"fabcfg": {"file": "g.json", "aliases": ["fab"]}}`, map[string]string{"g.json": fabGrammar})
	ls := NewPluginCache().languages([]*Plugin{p}, nil)
	for _, n := range []string{"fabcfg", "FAB", "Fab"} {
		if g := ls.lookup(n); g == nil || g.Name != "fabcfg" {
			t.Errorf("lookup(%q) = %v", n, g)
		}
	}
	if ls.lookup("text") != nil {
		t.Error("lookup(text) found something")
	}
}

func TestLocalOverridesBuiltinSilently(t *testing.T) {
	g := `{"scopeName": "source.go", "patterns": [{"match": "x", "name": "keyword.x"}]}`
	for name, manifest := range map[string]string{
		"by name":  `{"go": {"file": "g.json"}}`,
		"by alias": `{"mygo": {"file": "g.json", "aliases": ["golang"]}}`,
		"by scope": `{"mygo": {"file": "g.json"}}`,
	} {
		local := housePlugin(t, "house", manifest, map[string]string{"g.json": g})
		var w []string
		ls := NewPluginCache().languages([]*Plugin{fakeBuiltin(), local}, func(s string) { w = append(w, s) })
		if len(w) != 0 {
			t.Errorf("%s: warnings %q", name, w)
		}
		got := ls.byScope["source.go"]
		if got == nil || got.plugin != local {
			t.Errorf("%s: source.go served by %v, want the local plugin", name, got)
		}
	}
}

func TestTwoLocalPluginsClashWarnsAndLaterWins(t *testing.T) {
	a := housePlugin(t, "a", `{"fab": {"file": "g.json"}}`, map[string]string{"g.json": fabGrammar})
	b := housePlugin(t, "b", `{"fab": {"file": "g.json"}}`, map[string]string{"g.json": fabGrammar})
	var w []string
	ls := NewPluginCache().languages([]*Plugin{a, b}, func(s string) { w = append(w, s) })
	if ls.lookup("fab").plugin != b {
		t.Error("the earlier plugin won")
	}
	if len(w) != 1 || !strings.Contains(w[0], `both provide language "fab"; using`) {
		t.Errorf("warnings %q", w)
	}
}

// A grammar may include another by scope name, from any plugin.
func TestCrossPluginInclude(t *testing.T) {
	inner := housePlugin(t, "inner", `{"inner": {"file": "g.json"}}`, map[string]string{
		"g.json": `{"scopeName": "source.inner", "patterns": [{"match": "zap", "name": "keyword.inner"}]}`})
	outer := housePlugin(t, "outer", `{"outer": {"file": "g.json"}}`, map[string]string{
		"g.json": `{"scopeName": "source.outer", "patterns": [{"include": "source.inner"}]}`})
	ls := NewPluginCache().languages([]*Plugin{inner, outer}, nil)
	toks, err := ls.engine.Tokenize("source.outer", []string{"zap"})
	if err != nil {
		t.Fatal(err)
	}
	if s := toks[0][0].Scopes; s[len(s)-1] != "keyword.inner" {
		t.Errorf("scopes %v, want the included grammar's keyword", s)
	}
}

func TestNearMiss(t *testing.T) {
	p := housePlugin(t, "house", `{"python": {"file": "g.json", "aliases": ["py"]}, "shellscript": {"file": "g.json", "aliases": ["bash"]}}`,
		map[string]string{"g.json": fabGrammar})
	ls := NewPluginCache().languages([]*Plugin{p}, nil)
	for in, want := range map[string]string{
		"pyhton":  "python", // a transposition is one edit
		"Pyton":   "python", // case ignored
		"bahs":    "bash",
		"shellscirpt": "shellscript", // 8 or more runes may be 2 away
		"pyt":     "",       // under four letters
		"text":    "",       // means "no language"
		"tree":    "",       // means "no language"
		"pythn":   "python",
		"pyth":    "",       // 2 away at 4 runes
		"bashh":   "",       // the suggestion would be shorter
		"console": "",
		"python":  "",       // an exact match is not a miss
	} {
		if got := ls.nearMiss(in); got != want {
			t.Errorf("nearMiss(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLanguageSetsAreCachedByPluginList(t *testing.T) {
	p := housePlugin(t, "house", `{"fab": {"file": "g.json"}}`, map[string]string{"g.json": fabGrammar})
	c := NewPluginCache()
	if c.languages([]*Plugin{p}, nil) != c.languages([]*Plugin{p, p}, nil) {
		t.Error("one plugin listed twice built a second set")
	}
}

func TestLanguagesListsWhatAFenceCanName(t *testing.T) {
	local := housePlugin(t, "house", `{"go": {"file": "g.json"}}`, map[string]string{"g.json": fabGrammar})
	// From Task 5 on, the real built-in grammars are in the set too, so
	// look only at go.
	var gos []*Grammar
	for _, g := range languagesOf(NewPluginCache().languages([]*Plugin{fakeBuiltin(), local}, nil)) {
		if g.Name == "go" {
			gos = append(gos, g)
		}
	}
	if len(gos) != 1 || gos[0].plugin != local {
		t.Errorf("go listed as %v, want only the local one", gos)
	}
}
```

- [ ] **Step 2: Run them.** Expected: FAIL, because `languages` is undefined.

- [ ] **Step 3: Implement** `langset.go`:

```go
package md2html

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/AdamF-G/md2html/internal/highlight"
)

// langSet is the languages one page can highlight: the built-in grammars,
// then each local plugin's, in load order. Names are matched ignoring case;
// scope names decide which file serves a grammar, and every include.
type langSet struct {
	byName  map[string]*Grammar // names and aliases, lower case
	byScope map[string]*Grammar
	engine  highlight.Engine
}

type langEntry struct {
	once     sync.Once
	set      *langSet
	warnings []string
}

// languages returns the language set for plugins, with the built-in
// grammars plugin ahead of them, building it once per distinct list. Its
// warnings are replayed to every caller, as a plugin's are.
func (c *PluginCache) languages(plugins []*Plugin, warn func(string)) *langSet {
	list := uniquePlugins(append(builtinGrammarPlugins(), plugins...))
	keys := make([]string, len(list))
	for i, p := range list {
		keys[i] = p.key
	}
	id := strings.Join(keys, "\x00")

	c.mu.Lock()
	e, ok := c.langs[id]
	if !ok {
		e = &langEntry{}
		c.langs[id] = e
	}
	c.mu.Unlock()

	e.once.Do(func() { e.set = buildLangSet(list, func(w string) { e.warnings = append(e.warnings, w) }) })
	if warn != nil {
		for _, w := range e.warnings {
			warn(w)
		}
	}
	return e.set
}

// builtinGrammarPlugins is the built-in grammars plugin, or nothing in a
// build without one.
func builtinGrammarPlugins() []*Plugin {
	if p := findPlugin(BuiltinPlugins(), "grammars"); p != nil {
		return []*Plugin{p}
	}
	return nil
}

// uniquePlugins drops a plugin listed a second time, keeping the first.
func uniquePlugins(ps []*Plugin) []*Plugin {
	seen := map[string]bool{}
	var out []*Plugin
	for _, p := range ps {
		if !seen[p.key] {
			seen[p.key] = true
			out = append(out, p)
		}
	}
	return out
}

func buildLangSet(list []*Plugin, warn func(string)) *langSet {
	ls := &langSet{byName: map[string]*Grammar{}, byScope: map[string]*Grammar{}}
	claim := func(m map[string]*Grammar, key string, g *Grammar, what string) {
		if prev, ok := m[key]; ok && prev.plugin != g.plugin && prev.plugin.dir != "" {
			warn(fmt.Sprintf("plugins %s and %s both provide %s %q; using %s's", where(prev.plugin), where(g.plugin), what, key, g.plugin.Name))
		}
		m[key] = g
	}
	for _, p := range list {
		for _, name := range p.GrammarNames() {
			g := p.Grammars[name]
			for _, n := range append([]string{g.Name}, g.Aliases...) {
				claim(ls.byName, strings.ToLower(n), g, "language")
			}
			claim(ls.byScope, g.Scope, g, "scope")
		}
	}
	ls.engine = highlight.NewTextMate(func(scope string) ([]byte, error) {
		g := ls.byScope[scope]
		if g == nil {
			return nil, nil
		}
		return g.raw()
	}, highlight.Options{})
	return ls
}

func (ls *langSet) lookup(lang string) *Grammar { return ls.byName[strings.ToLower(lang)] }

// local returns the local grammar that serves scope, or nil for a built-in.
func (ls *langSet) local(scope string) *Grammar {
	if g := ls.byScope[scope]; g != nil && g.plugin.dir != "" {
		return g
	}
	return nil
}

// noLanguage are fence tags that say "this is not code in any language".
// They are common and deliberate, and several sit one edit from a real
// name (text and tex), so they never draw a near-miss warning.
var noLanguage = map[string]bool{"text": true, "txt": true, "plain": true, "plaintext": true,
	"none": true, "output": true, "tree": true, "ascii": true, "logs": true}

// nearMiss returns the known name closest to lang, for a likely typo: lang
// is at least four runes, not known, not a no-language tag, and within one
// edit of a name at least as long (two edits from eight runes up). Ties go
// to the name that sorts first.
func (ls *langSet) nearMiss(lang string) string {
	l := strings.ToLower(lang)
	n := len([]rune(l))
	if n < 4 || ls.byName[l] != nil || noLanguage[l] {
		return ""
	}
	limit := 1
	if n >= 8 {
		limit = 2
	}
	names := make([]string, 0, len(ls.byName))
	for name := range ls.byName {
		names = append(names, name)
	}
	sort.Strings(names)
	best, bestD := "", limit+1
	for _, name := range names {
		if len([]rune(name)) < n {
			continue
		}
		if d := editDistance(l, name); d < bestD {
			best, bestD = name, d
		}
	}
	return best
}

// editDistance is the optimal string alignment distance between a and b,
// in runes: Levenshtein, with swapping two adjacent runes as one edit,
// since that is the commonest typo.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}

// languagesOf is every grammar a fence can reach by its own name, sorted.
func languagesOf(ls *langSet) []*Grammar {
	var out []*Grammar
	for n, g := range ls.byName {
		if n == g.Name {
			out = append(out, g)
		}
	}
	return sortedGrammars(out)
}

// Languages returns every language a page could highlight with the
// built-in grammars and plugins, sorted by name. A language another plugin
// overrides is left out. --list-languages prints it.
func Languages(cache *PluginCache, plugins []*Plugin, warn func(string)) []*Grammar {
	return languagesOf(cache.languages(plugins, warn))
}
```

In `pluginload.go`, add `langs map[string]*langEntry // by plugin list; see langset.go` to `PluginCache` and initialise it in `NewPluginCache`.

- [ ] **Step 4: Run** `go test -race ./...`. Expected: PASS.

- [ ] **Step 5: Commit.** `git add langset.go langset_test.go pluginload.go && git commit -m "feat: language sets, with local grammars overriding built-in ones"`

---

### Task 5: The generator, the built-in `grammars` plugin and conformance

**Files:**
- Create: `internal/highlight/conformance.go`, `internal/highlight/conformance_test.go`, `internal/highlight/testdata/conformance/` (generated)
- Create: `internal/highlight/gen/main.go`, `internal/highlight/gen/overrides.json`
- Create: `internal/highlight/gen/node/package.json`, `package-lock.json`, `reference.mjs`
- Create: `plugins/grammars/` (generated), `builtin_grammars_test.go`
- Modify: `justfile`, `.gitignore`, `THIRD_PARTY_NOTICES`, `licenses_test.go`

**Interfaces:**
- Consumes: `highlight.NewTextMate`, `highlight.Token` (Task 1); `Grammar` (Task 3); `languages` (Task 4).
- Produces:
  - The generated golden dump format, as Go types in `conformance.go`:

```go
// Dump is one sample tokenized by one tokenizer. Columns are UTF-16 code
// unit offsets, since the reference tokenizer counts in JavaScript.
type Dump struct {
	Language string      `json:"language"`
	Scope    string      `json:"scope"`
	Lines    []string    `json:"lines"`
	Tokens   [][]DumpTok `json:"tokens"` // per line
}

type DumpTok struct {
	Start  int    `json:"s"`
	End    int    `json:"e"`
	Scopes string `json:"x"` // space-separated, root first
}

// DumpOf tokenizes d.Lines with e and returns the result in Dump form.
func DumpOf(e Engine, language, scope string, lines []string) (Dump, error)

// Compare returns "" when a and b agree token for token, or a sentence
// naming the first line, column and scopes where they differ.
func Compare(want, got Dump) string
```

  - The `plugins/grammars/` directory. Its manifest is named `grammars`, and each entry has `file`, `scope`, `license`, `source` and, when the collection gives them, `aliases`.
  - `just grammars`.

- [ ] **Step 1: Pin the Node tools.** In `internal/highlight/gen/node/package.json`:

```json
{
  "private": true,
  "type": "module",
  "dependencies": {
    "tm-grammars": "1.32.22",
    "vscode-oniguruma": "2.0.1",
    "vscode-textmate": "9.3.2"
  }
}
```

Run `cd internal/highlight/gen/node && npm install --ignore-scripts --no-audit --no-fund` once to write `package-lock.json`, and commit the lock file. Add `internal/highlight/gen/node/node_modules/` to `.gitignore`.

- [ ] **Step 2: The reference script**, `internal/highlight/gen/node/reference.mjs`. It reads `<work>/selected.json`, written by the generator's `select` step:

```json
[{"name": "go", "scope": "source.go", "sample": "<work>/samples/go.sample"}, ...]
```

For each entry it tokenizes the sample with VS Code's tokenizer and writes `<work>/ref/<name>.json` in the `Dump` form. Only the selected grammars can be loaded, so an include of a grammar md2html does not ship is missing in both tokenizers alike.

```js
import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'
import vsctm from 'vscode-textmate'
import oniguruma from 'vscode-oniguruma'

const require = createRequire(import.meta.url)
const work = process.argv[2]
const selected = JSON.parse(fs.readFileSync(path.join(work, 'selected.json'), 'utf8'))
const grammarDir = path.join(path.dirname(require.resolve('tm-grammars/package.json')), 'grammars')
const byScope = new Map(selected.map(s => [s.scope, path.join(grammarDir, s.name + '.json')]))

const wasm = fs.readFileSync(require.resolve('vscode-oniguruma/release/onig.wasm')).buffer
await oniguruma.loadWASM(wasm)
const registry = new vsctm.Registry({
  onigLib: Promise.resolve({
    createOnigScanner: p => new oniguruma.OnigScanner(p),
    createOnigString: s => new oniguruma.OnigString(s),
  }),
  loadGrammar: async scope => {
    const file = byScope.get(scope)
    return file ? vsctm.parseRawGrammar(fs.readFileSync(file, 'utf8'), file) : null
  },
})

fs.mkdirSync(path.join(work, 'ref'), { recursive: true })
for (const s of selected) {
  if (!s.sample) continue
  const grammar = await registry.loadGrammar(s.scope)
  const lines = fs.readFileSync(s.sample, 'utf8').replace(/\r\n/g, '\n').replace(/\n$/, '').split('\n')
  let state = vsctm.INITIAL
  const tokens = []
  for (const line of lines) {
    const r = grammar.tokenizeLine(line, state, 0)
    tokens.push(r.tokens.map(t => ({ s: t.startIndex, e: Math.min(t.endIndex, line.length), x: t.scopes.join(' ') })))
    state = r.ruleStack
  }
  fs.writeFileSync(path.join(work, 'ref', s.name + '.json'),
    JSON.stringify({ language: s.name, scope: s.scope, lines, tokens }))
}
```

`tokenizeLine`'s third argument is a time limit, and `0` means none. `endIndex` can be one past the line, because the tokenizer appends `\n`. Clamping it here, and in `DumpOf`, keeps the comparison on real text.

- [ ] **Step 3: `conformance.go`**, implementing `DumpOf` and `Compare` as declared above. `DumpOf` converts byte offsets to UTF-16 offsets with a per-line table built from `utf16.RuneLen` for each rune. It joins scopes with a space. It drops zero-width tokens, as the reference does not emit them. Check this against one sample before relying on it, and if the reference does emit them, keep them in both.

Write `TestCompareFindsADifference` first. It builds a `Dump` and a copy with one scope changed, then asserts that `Compare` names line 1 and the changed scope, and that `Compare(d, d) == ""`. Run it and see it fail, then implement.

- [ ] **Step 4: The overrides file**, `internal/highlight/gen/overrides.json`:

```json
{
  "license": {
    "yaml": {
      "spdx": "LicenseRef-TextMate-Bundle",
      "text": "Permission to copy, use, modify, sell and distribute this software is granted. This software is provided \"as is\" without express or implied warranty, and with no claim as to its suitability for any purpose.",
      "from": "https://github.com/textmate/yaml.tmbundle/blob/e54ceae3b719506dba7e481a77cea4a8b576ae46/README.mdown"
    },
    "toml": {
      "spdx": "LicenseRef-TextMate-Bundle",
      "text": "<copy the licence section of textmate/toml.tmbundle's README at e82b64c1e86396220786846201e9aa3f0a2d9ca2, verbatim>",
      "from": "https://github.com/textmate/toml.tmbundle/blob/e82b64c1e86396220786846201e9aa3f0a2d9ca2/README.mdown"
    }
  },
  "exclude": {}
}
```

Fetch the TOML bundle's README at that commit with `gh api` and paste its licence text verbatim. If that text is not permissive, drop the `toml` entry and say so in the commit body. `exclude` maps a grammar name to the reason it is left out. The generator also writes its own exclusions into the report, but `exclude` is for ones a person decided.

- [ ] **Step 5: The generator**, `internal/highlight/gen/main.go`, a `package main` with two subcommands. The constants at its top are the pins:

```go
const (
	collection     = "1.32.22"
	samplesCommit  = "37edd1b26f18838050661d912334aba0ca7f4931"
)

var allow = map[string]bool{"MIT": true, "Apache-2.0": true, "BSD-2-Clause": true, "BSD-3-Clause": true,
	"ISC": true, "0BSD": true, "Unlicense": true, "CC0-1.0": true}
```

**`select -work DIR -pkg DIR`** runs these steps in order:
1. Read the collection's metadata. Run `node --input-type=module -e "import('<pkg>/index.js').then(m => console.log(JSON.stringify(m.grammars)))"` with `exec.Command` and decode it into `[]struct{Name, ScopeName, Source, License string; Aliases []string}`. This is the one place the generator runs Node. It reads metadata only.
2. Check that `<pkg>/package.json`'s version is `collection`. If not, exit non-zero.
3. Keep a grammar when `allow[License]` holds, or when it has a `license` override, and when it is not in `exclude`.
4. Write `<work>/selected.json`, with `sample` set to `<work>/samples/<name>.sample` when that file exists and `""` otherwise.
5. Print the counts kept and left out.

**`write -work DIR -pkg DIR -out plugins/grammars -golden internal/highlight/testdata/conformance -notices THIRD_PARTY_NOTICES`** runs these steps in order:
1. Build a `highlight.NewTextMate` over the selected grammars' raw JSON, loaded by scope.
2. For each selected grammar with a sample, read `<work>/ref/<name>.json`, run `highlight.DumpOf` on the same lines, and `highlight.Compare` them.
   - A mismatch leaves the grammar out, with the sentence from `Compare` as the reason.
   - If a grammar left out this way is included by another kept grammar (search the kept grammars' raw JSON for `"include": "<its scope>` with or without a `#rule` suffix), exit non-zero. Name both, and say to add the failing one to `exclude` and rerun. This keeps the reference and md2html on the same set.
3. Record `Diagnostics` for every kept grammar after its sample is tokenized.
4. Remove and rewrite `-out`:
   - Write `<name>.json.gz` for each kept grammar. Use `gzip.BestCompression`, a zero `ModTime` and an empty `Name`, so the bytes are deterministic.
   - Write `md2html-plugin.json` with `format` 1, name `grammars`, description `Languages for highlighting code, from tm-grammars <collection>`, and a `grammars` entry per kept grammar. Entries are in sorted order and use `json.MarshalIndent` with two spaces.
   - Write `REPORT.md`, which lists:
     - the collection version and samples commit;
     - the grammars kept, with count;
     - the grammars left out, each with its reason: licence (and which), override exclusion, or conformance (with the `Compare` sentence);
     - each kept grammar's untranslatable patterns.
5. Remove and rewrite `-golden`, with one `<name>.json.gz` per kept grammar that has a sample. It holds the reference `Dump`, gzipped the same deterministic way.
6. In `-notices`, replace everything between the lines `== BEGIN GENERATED GRAMMARS ==` and `== END GENERATED GRAMMARS ==` with:
   - one `== <displayName or name> grammar (<SPDX>) ==` heading per kept grammar, with its source URL;
   - the full licence text, from the collection's `NOTICE` file for that grammar, or from the override's `text`;
   - the collection's own MIT notice, once.

   If the markers are missing, add them at the end of the file first.

Write `TestGzipIsDeterministic` in `internal/highlight/gen/main_test.go` first: compressing the same bytes twice gives identical output. Then implement.

- [ ] **Step 6: The recipe.** Add this to `justfile` after `compat`:

```just
# The built-in grammars are generated, never hand-edited: this reads the
# pinned tm-grammars release and its samples, keeps the permissively
# licensed grammars that match VS Code's own tokenizer on their sample,
# and rewrites plugins/grammars/, the conformance goldens and the
# grammars' notices. It is the only thing that needs Node or the network.

# Regenerate the built-in grammars (needs node, npm and network).
grammars:
    #!/usr/bin/env bash
    set -euo pipefail
    work=$(mktemp -d)
    trap 'rm -rf "$work"' EXIT
    (cd internal/highlight/gen/node && npm ci --ignore-scripts --no-audit --no-fund)
    pkg=internal/highlight/gen/node/node_modules/tm-grammars
    curl -sSfL "https://codeload.github.com/shikijs/textmate-grammars-themes/tar.gz/37edd1b26f18838050661d912334aba0ca7f4931" \
        | tar -xz -C "$work" --strip-components=1 --wildcards '*/samples/*'
    go run ./internal/highlight/gen select -work "$work" -pkg "$pkg"
    node internal/highlight/gen/node/reference.mjs "$work"
    go run ./internal/highlight/gen write -work "$work" -pkg "$pkg" \
        -out plugins/grammars -golden internal/highlight/testdata/conformance -notices THIRD_PARTY_NOTICES
```

The commit hash appears twice, in `gen/main.go` and here. Add a test, `TestRecipePinsTheSamplesCommit` in `internal/highlight/gen/main_test.go`, that reads `../../../justfile` and requires it to contain `samplesCommit`.

- [ ] **Step 7: Generate.** Run `just grammars`. Expected:
  - about 200 kept, and 42 or fewer left out for licence;
  - YAML and TOML kept through overrides;
  - `plugins/grammars/` about 1.2 MB.

  Read `REPORT.md`. Every conformance exclusion is a finding to bring to the owner before going further, because the spike found none in seven languages. Run `just grammars` a second time and confirm that `git status` shows no change (the output is deterministic).

- [ ] **Step 8: The Go-side tests.** `internal/highlight/conformance_test.go`:

```go
package highlight

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every shipped grammar tokenizes its sample exactly as VS Code's own
// tokenizer did when the grammars were generated: same tokens, same full
// scope stacks. This is the gate for an engine upgrade or swap.
func TestConformance(t *testing.T) {
	files, _ := filepath.Glob("testdata/conformance/*.json.gz")
	if len(files) < 150 {
		t.Fatalf("%d golden files; run just grammars", len(files))
	}
	byScope := map[string][]byte{}
	matches, _ := filepath.Glob("../../plugins/grammars/*.json.gz")
	for _, f := range matches {
		raw := gunzipFile(t, f)
		var g struct {
			ScopeName string `json:"scopeName"`
		}
		json.Unmarshal(raw, &g)
		byScope[g.ScopeName] = raw
	}
	e := NewTextMate(func(s string) ([]byte, error) { return byScope[s], nil }, Options{})
	for _, f := range files {
		var want Dump
		if err := json.Unmarshal(gunzipFile(t, f), &want); err != nil {
			t.Fatal(err)
		}
		t.Run(want.Language, func(t *testing.T) {
			got, err := DumpOf(e, want.Language, want.Scope, want.Lines)
			if err != nil {
				t.Fatal(err)
			}
			if d := Compare(want, got); d != "" {
				t.Error(d)
			}
		})
	}
}

func gunzipFile(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if _, err := io.Copy(&b, zr); err != nil {
		t.Fatal(err)
	}
	return []byte(b.String())
}
```

Add `"io"` to the imports. Then add a corrupted-golden check, as the spike did, so the test is known to fail when it should. `TestConformanceCatchesADifference` takes the first golden, changes one token's scopes, and asserts that `Compare` reports it.

`builtin_grammars_test.go` (package md2html):

```go
package md2html

import (
	"testing"

	"github.com/AdamF-G/md2html/internal/highlight"
)

// Every built-in grammar decompresses, parses, and declares the scope its
// manifest entry says, so a broken one cannot ship.
func TestBuiltinGrammarsAreValid(t *testing.T) {
	p := findPlugin(BuiltinPlugins(), "grammars")
	if p == nil {
		t.Fatal("no built-in grammars plugin")
	}
	if n := len(p.Grammars); n < 190 {
		t.Errorf("%d built-in grammars, want about 200", n)
	}
	for _, name := range p.GrammarNames() {
		g := p.Grammars[name]
		data, err := g.raw()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		scope, err := highlight.CheckGrammar(data)
		if err != nil || scope != g.Scope {
			t.Errorf("%s: scope %q, manifest says %q (%v)", name, scope, g.Scope, err)
		}
	}
}

// The aliases the guide promises reach their grammars.
func TestBuiltinAliases(t *testing.T) {
	ls := NewPluginCache().languages(nil, nil)
	for alias, want := range map[string]string{"sh": "shellscript", "bash": "shellscript", "zsh": "shellscript",
		"yml": "yaml", "py": "python", "go": "go", "json": "json", "diff": "diff", "toml": "toml"} {
		if g := ls.lookup(alias); g == nil || g.Name != want {
			t.Errorf("%s reaches %v, want %s", alias, g, want)
		}
	}
}

// No copyleft or unlicensed grammar ships.
func TestBuiltinGrammarLicences(t *testing.T) {
	allowed := map[string]bool{"MIT": true, "Apache-2.0": true, "BSD-2-Clause": true, "BSD-3-Clause": true,
		"ISC": true, "0BSD": true, "Unlicense": true, "CC0-1.0": true, "LicenseRef-TextMate-Bundle": true}
	for _, g := range findPlugin(BuiltinPlugins(), "grammars").Grammars {
		if !allowed[g.License] {
			t.Errorf("%s ships under %q", g.Name, g.License)
		}
	}
}
```

In `licenses_test.go`, add `"go grammar", "yaml grammar", "tm-grammars"` to the names list, matching the headings the generator writes. Adjust the spelling to what it actually writes.

- [ ] **Step 9: Run** `go test ./...`. Expected: PASS. Check that `TestEveryBuiltinStyleIsWellFormed`, `TestBuiltinStylesPassTheStyleCheck`, `--check-style` and `--list-styles` all still pass with a style-less built-in plugin present. If any iterates every built-in plugin's styles in a way that expects at least one, skip style-less plugins there, the way `listAvailableStyles` does.

- [ ] **Step 10: Commit** in two commits, so the generated bulk is reviewable apart from the code:

```bash
git add internal/highlight/conformance.go internal/highlight/conformance_test.go internal/highlight/gen justfile .gitignore
git commit -m "feat: generate the built-in grammars from tm-grammars, checked against VS Code"
git add plugins/grammars internal/highlight/testdata THIRD_PARTY_NOTICES licenses_test.go builtin_grammars_test.go
git commit -m "feat: the built-in grammars plugin, generated from tm-grammars 1.32.22"
```

The first commit's tests reference generated goldens that do not exist yet. To keep every commit green, make `TestConformance` skip with "run just grammars" when there are no golden files at all. It still fails when there are fewer than 150. The second body records the counts kept and left out, the size of `plugins/grammars/`, and the injection-grammars decision.

---

### Task 6: Highlighting fences

**Files:**
- Modify: `codefence.go`, `md2html.go` (`Options`, `newParser`, `Convert`), `stylesheet.go` (`choosePageStyle`), `siteshell_test.go`, `stylesheet_test.go`
- Create: `highlight_test.go`

**Interfaces:**
- Consumes: `(*PluginCache).languages`, `lookup`, `nearMiss`, `local`, `engine` (Task 4); `highlight.Spans`, `highlight.ErrBudget`, `Kind.Class` (Tasks 1 and 2); the built-in grammars (Task 5).
- Produces:
  - `Options.NoHighlight bool`.
  - The `highlightContext` type and `newHighlightContext(opt Options, meta map[string]string, lineOffset int) highlightContext`.
  - `newParser(warn func(string), charts chartContext, hl highlightContext)`.
  - The markup: `<code class="language-go md2html-hl …">` with `<span class="hl-<kind>">` per run.

- [ ] **Step 1: Write the failing tests** in `highlight_test.go`. Each states one rule from the spec.

```go
package md2html

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func convertBody(t *testing.T, src string, opt Options) (string, []string) {
	t.Helper()
	var w []string
	opt.Fragment = true
	opt.Warn = func(s string) { w = append(w, s) }
	out, err := Convert([]byte(src), opt)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), w
}

var spanTags = regexp.MustCompile(`</?span[^>]*>`)

func TestGoFenceIsHighlighted(t *testing.T) {
	out, w := convertBody(t, "```go\nfunc main() { fmt.Println(\"hi\", 42) } // ok\n```\n", Options{})
	if len(w) != 0 {
		t.Errorf("warnings %q", w)
	}
	for _, want := range []string{`<code class="language-go md2html-hl">`, `<span class="hl-keyword">func</span>`,
		`<span class="hl-string">&#34;hi&#34;</span>`, `<span class="hl-number">42</span>`, `<span class="hl-comment">// ok</span>`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
}
```

Check the escaping `RawWrite` produces for `"` against today's output for the same fence, and use that spelling in the test. The rule is "escaped exactly as today", not any particular entity.

```go
// Only spans are added: strip them and the code is today's, byte for byte.
func TestHighlightKeepsTextExactly(t *testing.T) {
	for _, src := range []string{
		"```go\nx := \"héllo — 世界\" // <b>&amp;</b>\nreturn\n```\n",
		"```go\r\nfunc a() {}\r\n// c\r\n```\r\n",
		"```python\ns = '''multi\nline'''\n# done\n```\n",
	} {
		hl, _ := convertBody(t, src, Options{})
		plain, _ := convertBody(t, src, Options{NoHighlight: true})
		stripped := strings.Replace(spanTags.ReplaceAllString(hl, ""), " md2html-hl", "", 1)
		if stripped != plain {
			t.Errorf("text changed for %q:\n got %s\nwant %s", src, stripped, plain)
		}
	}
}

// A multi-line string is closed at each line end and reopened.
func TestSpansNeverCrossALine(t *testing.T) {
	out, _ := convertBody(t, "```python\ns = '''one\ntwo'''\n```\n", Options{})
	code := out[strings.Index(out, "<code"):strings.Index(out, "</code>")]
	for i, line := range strings.Split(code, "\n") {
		if strings.Count(line, "<span") != strings.Count(line, "</span>") {
			t.Errorf("line %d is unbalanced: %s", i, line)
		}
	}
}

func TestUnchangedOutput(t *testing.T) {
	for name, c := range map[string]struct {
		src string
		opt Options
	}{
		"no language":       {"```\nfunc x\n```\n", Options{}},
		"unknown language":  {"```text\nfunc x\n```\n", Options{}},
		"no-highlight":      {"```{.go .no-highlight}\nfunc x\n```\n", Options{}},
		"front matter off":  {"---\nhighlight: false\n---\n```go\nfunc x\n```\n", Options{}},
		"option off":        {"```go\nfunc x\n```\n", Options{NoHighlight: true}},
		"no code":           {"# Title\n\nText.\n", Options{}},
		"fig falls back":    {"```fig\nnot: [a figure\n```\n", Options{}},
		"vega-lite off":     {"```vega-lite\n{\"mark\": \"bar\"}\n```\n", Options{NoCharts: true}},
	} {
		out, _ := convertBody(t, c.src, c.opt)
		if strings.Contains(out, "md2html-hl") || strings.Contains(out, `class="hl-`) {
			t.Errorf("%s: highlighted:\n%s", name, out)
		}
	}
}
```

The "byte-identical to the release before" half of the rule is covered by every existing golden and conversion test passing unchanged. Do not regenerate any of them in this task.

```go
func TestNoHighlightClassStays(t *testing.T) {
	out, _ := convertBody(t, "```{.go .no-highlight}\nfunc x\n```\n", Options{})
	if !strings.Contains(out, `class="language-go no-highlight"`) {
		t.Errorf("class lost:\n%s", out)
	}
}

func TestCaptionsIdsAndClassesSurvive(t *testing.T) {
	out, _ := convertBody(t, "```go {#srv .wide caption=\"server.go\"}\nfunc x\n```\n", Options{})
	for _, want := range []string{`<figure class="code-figure"><figcaption>server.go</figcaption>`,
		`<pre id="srv"><code class="language-go wide md2html-hl">`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
}

func TestEveryInfoStringFormHighlights(t *testing.T) {
	for _, info := range []string{"go", "{.go caption=\"x\"}", "go {.wide}", "Go", "golang"} {
		out, _ := convertBody(t, "```"+info+"\nfunc x\n```\n", Options{})
		if !strings.Contains(out, `<span class="hl-keyword">func</span>`) {
			t.Errorf("%s: not highlighted", info)
		}
	}
}

func TestEmptyFenceIsUnchanged(t *testing.T) {
	for _, src := range []string{"```go\n```\n", "```go\n\n```\n"} {
		hl, _ := convertBody(t, src, Options{})
		plain, _ := convertBody(t, src, Options{NoHighlight: true})
		if hl != plain {
			t.Errorf("%q changed:\n%s", src, hl)
		}
	}
}

func TestHighlightInsideContainers(t *testing.T) {
	src := "- item\n\n  ```go\n  func x() {}\n  ```\n\n::: details Code\n```go\nreturn 1\n```\n:::\n"
	out, w := convertBody(t, src, Options{})
	if len(w) != 0 {
		t.Errorf("warnings %q", w)
	}
	if strings.Count(out, "md2html-hl") != 2 {
		t.Errorf("want two highlighted fences:\n%s", out)
	}
	plain, _ := convertBody(t, src, Options{NoHighlight: true})
	if strings.ReplaceAll(spanTags.ReplaceAllString(out, ""), " md2html-hl", "") != plain {
		t.Error("text changed inside containers")
	}
}

func TestNearMissWarnsOnce(t *testing.T) {
	_, w := convertBody(t, "Text.\n\n```pyhton\nx\n```\n\n```text\nx\n```\n\n```console\n$ x\n```\n", Options{})
	want := `fence at line 3: no grammar for "pyhton"; did you mean "python"?`
	if len(w) != 1 || w[0] != want {
		t.Errorf("warnings %q, want [%q]", w, want)
	}
}

func TestNearMissNamesTheFileLine(t *testing.T) {
	src := "---\ntitle: T\nlang: en\n---\n\nText.\n\nMore.\n```pyhton\nx\n```\n"
	_, w := convertBody(t, src, Options{})
	if len(w) != 1 || !strings.HasPrefix(w[0], "fence at line 9:") {
		t.Errorf("warnings %q, want line 9", w)
	}
}

func TestHighlightFrontMatterValue(t *testing.T) {
	_, w := convertBody(t, "---\nhighlight: no\n---\n```go\nfunc x\n```\n", Options{})
	if len(w) != 1 || w[0] != `front matter highlight "no" is not true or false; highlighting the page's fences` {
		t.Errorf("warnings %q", w)
	}
}

func TestOverBudgetFenceIsPlain(t *testing.T) {
	hl := highlightContext{langs: NewPluginCache().languages(nil, nil), budget: 1}
	var w []string
	spans := hl.spans("go", nil, []string{"func x() {}"}, 3, func(s string) { w = append(w, s) })
	if spans != nil {
		t.Error("spans returned over budget")
	}
	if len(w) != 1 || w[0] != `fence at line 3: highlighting "go" took over 2s; showing it plain` {
		t.Errorf("warnings %q", w)
	}
}
```

`budget` is a test-only field on `highlightContext`. When it is non-zero, the context uses its own engine built with that budget. That is the cheapest way to exercise the fault without a pathological grammar. The message keeps naming 2 s, since that is what the run's budget is.

```go
func TestEngineErrorIsPlain(t *testing.T) {
	hl := highlightContext{langs: &langSet{byName: map[string]*Grammar{"go": {Name: "go", Scope: "source.missing", plugin: &Plugin{}}},
		engine: NewPluginCache().languages(nil, nil).engine}}
	var w []string
	if hl.spans("go", nil, []string{"x"}, 1, func(s string) { w = append(w, s) }) != nil {
		t.Error("spans returned on an engine error")
	}
	if len(w) != 1 || !strings.HasPrefix(w[0], `fence at line 1: highlighting "go" failed: `) || !strings.HasSuffix(w[0], "; showing it plain") {
		t.Errorf("warnings %q", w)
	}
}

func TestPagePluginsProvideGrammars(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"langs/md2html-plugin.json": `{"format": 1, "name": "langs", "grammars": {"fab": {"file": "g.json"}}}`,
		"langs/g.json":              fabGrammar,
		"page.md":                   "---\nplugins: ./langs\n---\n```fab\nfab x\n```\n",
	})
	src, _ := os.ReadFile(filepath.Join(dir, "page.md"))
	out, w := convertBody(t, string(src), Options{SourcePath: filepath.Join(dir, "page.md"), PluginCache: NewPluginCache()})
	if len(w) != 0 {
		t.Errorf("warnings %q", w)
	}
	if !strings.Contains(out, `<span class="hl-keyword">fab</span>`) {
		t.Errorf("not highlighted:\n%s", out)
	}
}

func TestBrokenPagePluginKeepsRunGrammars(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"page.md": "---\nplugins: ./nope\n---\n```go\nfunc x\n```\n"})
	src, _ := os.ReadFile(filepath.Join(dir, "page.md"))
	out, w := convertBody(t, string(src), Options{SourcePath: filepath.Join(dir, "page.md")})
	if len(w) != 1 || !strings.HasPrefix(w[0], "front matter plugins: ") {
		t.Errorf("warnings %q", w)
	}
	if !strings.Contains(out, "md2html-hl") {
		t.Error("the run's grammars were lost")
	}
}

func TestLocalGrammarDiagnosticsWarnOncePerPage(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"langs/md2html-plugin.json": `{"format": 1, "name": "langs", "grammars": {"bad": {"file": "g.json"}}}`,
		"langs/g.json":              `{"scopeName": "source.bad", "patterns": [{"match": "\\p{Xwd}+", "name": "keyword.bad"}, {"match": "ok", "name": "keyword.ok"}]}`,
	})
	_, w := convertBody(t, "```bad\nok\n```\n\n```bad\nok\n```\n", Options{Plugins: []string{filepath.Join(dir, "langs")}})
	if len(w) != 1 || !strings.HasPrefix(w[0], `grammar "bad" from plugin langs: 1 pattern cannot be translated and is skipped: `) {
		t.Errorf("warnings %q", w)
	}
}

func TestConvertHighlightsInParallel(t *testing.T) {
	var b strings.Builder
	b.WriteString("```go\n")
	for i := range 400 {
		fmt.Fprintf(&b, "func f%d() string { return \"x\" } // %d\n", i, i)
	}
	b.WriteString("```\n")
	cache := NewPluginCache()
	var wg sync.WaitGroup
	outs := make([]string, 32)
	for i := range outs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, w := convertBody(t, b.String(), Options{PluginCache: cache})
			if len(w) != 0 {
				t.Errorf("warnings %q", w)
			}
			outs[i] = out
		}()
	}
	wg.Wait()
	for i := range outs {
		if outs[i] != outs[0] {
			t.Fatalf("output %d differs", i)
		}
	}
}
```

`convertBody` calls `t.Fatal` from a goroutine in the last test. Use a goroutine-safe variant there that returns the error, and report it after `wg.Wait()`. `writeTree` is whichever multi-file temp-dir helper the package tests already use (`pluginDir` writes a map of files into a temp dir and returns the root). Use that and adjust the paths.

Update the existing tests that pin the old message:
- `siteshell_test.go:256` and `stylesheet_test.go:260` expect `front matter plugins has no effect without style; ignoring it` for a page plugin with no style. Those fixtures provide styles and no grammars, so the warning still fires. Confirm they pass unchanged. If either fixture is a grammar-providing plugin, change the fixture, not the message.

- [ ] **Step 2: Run them.** `go test -run 'Highlight|Fence|NearMiss|PagePlugin|BrokenPage|Diagnostics|Budget|EngineError|Unchanged|Parallel' .` should fail to compile.

- [ ] **Step 3: Implement.** In `md2html.go`, add `Options.NoHighlight` after `NoCharts`:

```go
	// NoHighlight leaves every fenced code block plain. By default a fence
	// whose language a loaded grammar names is highlighted when the page is
	// built: its tokens are wrapped in spans coloured by the style's
	// --syntax-* tokens. A page's "highlight: false" turns it off for that
	// page alone.
	NoHighlight bool
```

Extend the `Warn` doc comment's list with "a fence whose language is a near miss of a known one, or that could not be highlighted".

In `Convert`, after `meta, src, malformed := splitFrontMatter(src)`, compute the front matter's line count, `lineOffset := bytes.Count(orig[:len(orig)-len(src)], []byte("\n"))`. `orig` is the `src` from before the split, so keep it in a variable. Then pass `newHighlightContext(opt, meta, lineOffset)` to `newParser`.

`newParser` takes the context and passes it to `newCodeFenceRenderer(warn, charts, hl)`.

The context, in `codefence.go` beside `chartContext`'s use:

```go
// highlightContext is how the fence renderer highlights: the page's
// language set, or nil when highlighting is off for the page.
type highlightContext struct {
	langs      *langSet
	lineOffset int             // lines of front matter above the body
	warned     map[string]bool // local grammars already reported on this page
	budget     time.Duration   // tests only: a budget other than the default
}

func newHighlightContext(opt Options, meta map[string]string, lineOffset int) highlightContext {
	switch v := meta["highlight"]; v {
	case "false":
		return highlightContext{}
	case "", "true":
	default:
		if opt.Warn != nil {
			opt.Warn(fmt.Sprintf("front matter highlight %q is not true or false; highlighting the page's fences", v))
		}
	}
	if opt.NoHighlight {
		return highlightContext{}
	}
	cache := opt.PluginCache
	if cache == nil {
		cache = NewPluginCache()
	}
	return highlightContext{langs: cache.languages(pageGrammarPlugins(cache, opt, meta), nil),
		lineOffset: lineOffset, warned: map[string]bool{}}
}

// pageGrammarPlugins is the run's plugins and the page's own, the page's
// resolved against its directory. Their load warnings and errors were said
// when the style was chosen (see choosePageStyle), so this stays quiet: a
// page plugin that does not load leaves the run's.
func pageGrammarPlugins(cache *PluginCache, opt Options, meta map[string]string) []*Plugin {
	ps, err := cache.LoadAll(opt.Plugins, "", nil)
	if err != nil {
		return nil
	}
	baseDir := ""
	if opt.SourcePath != "" {
		baseDir = filepath.Dir(opt.SourcePath)
	}
	own, err := cache.LoadAll(splitList(meta["plugins"]), baseDir, nil)
	if err != nil {
		return ps
	}
	return append(ps, own...)
}

// spans highlights one fence's lines, or returns nil for a plain fence:
// highlighting off, a language no grammar names, or a fault, which is
// said. line is the fence's line in the file, for messages.
func (h highlightContext) spans(lang string, classes []string, lines []string, line int, warn func(string)) [][]highlight.Span {
	if h.langs == nil || lang == "" || slices.Contains(classes, "no-highlight") || allBlank(lines) {
		return nil
	}
	g := h.langs.lookup(lang)
	if g == nil {
		if m := h.langs.nearMiss(lang); m != "" {
			warn(fmt.Sprintf("fence at line %d: no grammar for %q; did you mean %q?", line, lang, m))
		}
		return nil
	}
	engine := h.langs.engine
	if h.budget != 0 {
		engine = highlight.NewTextMate(func(s string) ([]byte, error) {
			if g := h.langs.byScope[s]; g != nil {
				return g.raw()
			}
			return nil, nil
		}, highlight.Options{Budget: h.budget})
	}
	toks, err := engine.Tokenize(g.Scope, lines)
	switch {
	case errors.Is(err, highlight.ErrBudget):
		warn(fmt.Sprintf("fence at line %d: highlighting %q took over %s; showing it plain", line, lang, highlight.DefaultBudget))
		return nil
	case err != nil:
		warn(fmt.Sprintf("fence at line %d: highlighting %q failed: %v; showing it plain", line, lang, err))
		return nil
	}
	if lg := h.langs.local(g.Scope); lg != nil && !h.warned[g.Scope] {
		if d := engine.Diagnostics(g.Scope); len(d) > 0 {
			h.warned[g.Scope] = true
			noun := "patterns cannot be translated and are skipped"
			if len(d) == 1 {
				noun = "pattern cannot be translated and is skipped"
			}
			warn(fmt.Sprintf("grammar %q from plugin %s: %d %s: %s", lg.Name, lg.PluginName(), len(d), noun, strings.Join(d, "; ")))
		}
	}
	out := make([][]highlight.Span, len(toks))
	for i, t := range toks {
		out[i] = highlight.Spans(t)
	}
	return out
}

func allBlank(lines []string) bool {
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			return false
		}
	}
	return true
}
```

`%s` on `time.Duration` 2 s prints `2s`, which matches the test. In the renderer, `codeFenceRenderer` gains `hl highlightContext`. In `render`, after the fig and vega-lite blocks and before writing `<pre`, do the following:

```go
	special := lang == "fig" || lang == "vega-lite"
	var spans [][]highlight.Span
	var text [][]byte // each line without its line ending
	var ends [][]byte // the line ending
	if !special {
		lines := n.Lines()
		strs := make([]string, lines.Len())
		for i := 0; i < lines.Len(); i++ {
			v := lines.At(i).Value(source)
			body := bytes.TrimSuffix(bytes.TrimSuffix(v, []byte("\n")), []byte("\r"))
			text = append(text, body)
			ends = append(ends, v[len(body):])
			strs[i] = string(body)
		}
		spans = r.hl.spans(lang, f.classes, strs, r.fenceLine(n, source), r.warn)
	}
```

`fenceLine` is `1 + r.hl.lineOffset + bytes.Count(source[:n.Info.Segment.Start], []byte("\n"))`. The fence has a language, so it has an `Info`. Then:
- `codeClasses(lang, f.classes)` gains `md2html-hl` at the end when `spans != nil`. Add a third parameter, `hl bool`. Its only other caller is the code block here, so check with grep.
- The body loop writes the spans when `spans != nil`:

```go
	if spans != nil {
		for i := range text {
			pos := 0
			for _, s := range spans[i] {
				goldhtml.DefaultWriter.RawWrite(w, text[i][pos:s.Start])
				w.WriteString(`<span class="` + s.Kind.Class() + `">`)
				goldhtml.DefaultWriter.RawWrite(w, text[i][s.Start:s.End])
				w.WriteString("</span>")
				pos = s.End
			}
			goldhtml.DefaultWriter.RawWrite(w, text[i][pos:])
			goldhtml.DefaultWriter.RawWrite(w, ends[i])
		}
	} else {
		// the existing loop, unchanged
	}
```

In `stylesheet.go`'s `choosePageStyle`, replace the `len(names) == 0` branch's warning:

```go
		if len(pageDirs) > 0 {
			baseDir := ""
			if opt.SourcePath != "" {
				baseDir = filepath.Dir(opt.SourcePath)
			}
			own, err := cache.LoadAll(pageDirs, baseDir, warn)
			switch {
			case err != nil:
				warn(fmt.Sprintf("front matter plugins: %v; ignoring it", err))
			case !anyGrammars(own):
				warn("front matter plugins has no effect without style; ignoring it")
			}
		}
```

and add:

```go
// anyGrammars reports whether any of ps provides a language, which a
// page's plugins: can bring without a style.
func anyGrammars(ps []*Plugin) bool {
	for _, p := range ps {
		if len(p.Grammars) > 0 {
			return true
		}
	}
	return false
}
```

Apply the same change to the site-shell branch in `md2html.go`, which raises the same message when a shell supplies the style. There, a page plugin that provides grammars is not "no effect" either. Load the page's plugins with `warn` and apply the same switch.

- [ ] **Step 4: Run** `go test -race ./...`. Expected: PASS, with every pre-existing golden unchanged.

- [ ] **Step 5: Commit.** `git add -A . && git commit -m "feat: fences are highlighted at build time"`. Check `git status` first so nothing unrelated is staged. The body notes the front matter values decision.

---

### Task 7: `highlight.css`

**Files:**
- Create: `highlight.css`
- Modify: `theme.go`, `page.go` (`renderPage`, `renderFragment`), `page_test.go`

**Interfaces:**
- Consumes: the `md2html-hl` class (Task 6).
- Produces: `highlightCSS string` (embedded) and `needsHighlightCSS(body []byte) bool`.

- [ ] **Step 1: Write the failing tests** in `page_test.go`:

```go
func TestHighlightCSSOnlyWhenNeeded(t *testing.T) {
	hl, err := Convert([]byte("```go\nfunc x\n```\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Convert([]byte("```text\nfunc x\n```\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hl), ".hl-keyword") {
		t.Error("a highlighted page lacks the span rules")
	}
	if strings.Contains(string(plain), ".hl-keyword") {
		t.Error("a page with no highlighted fence carries the span rules")
	}
	// Ahead of the style, so a style's own rules for the same classes win.
	if i, j := strings.Index(string(hl), ".hl-keyword"), strings.Index(string(hl), "--bg:"); i > j {
		t.Error("the span rules come after the style")
	}
}

func TestHighlightCSSInFragments(t *testing.T) {
	out, err := Convert([]byte("```go\nfunc x\n```\n"), Options{Fragment: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), ".hl-keyword") {
		t.Error("fragment lacks the span rules")
	}
}

// Every rule falls back to the surrounding colour, so a stylesheet with no
// syntax tokens shows readable, plain code.
func TestHighlightCSSFallsBack(t *testing.T) {
	for _, k := range highlight.Kinds {
		want := fmt.Sprintf(".%s { color: var(--syntax-%s, inherit)", k.Class(), k.Name())
		if !strings.Contains(highlightCSS, want) {
			t.Errorf("highlight.css lacks %q", want)
		}
	}
	if !strings.Contains(highlightCSS, "font-style: italic") || strings.Count(highlightCSS, "font-style") != 1 {
		t.Error("only comments are italic")
	}
	if strings.Contains(highlightCSS, "font-weight") {
		t.Error("no kind changes weight")
	}
}
```

- [ ] **Step 2: Run them.** Expected: FAIL.

- [ ] **Step 3: Implement.** `highlight.css`:

```css
/* Highlighted code: md2html wraps each token of a fence whose language a
   grammar names in a span classed by its kind. Each colour is the page
   style's --syntax-* token, falling back to the surrounding text, so a
   stylesheet that defines none still shows readable code. Only comments
   change style, so metrics in a code block never change. */
.hl-comment { color: var(--syntax-comment, inherit); font-style: italic; }
.hl-keyword { color: var(--syntax-keyword, inherit); }
.hl-string { color: var(--syntax-string, inherit); }
.hl-number { color: var(--syntax-number, inherit); }
.hl-constant { color: var(--syntax-constant, inherit); }
.hl-function { color: var(--syntax-function, inherit); }
.hl-type { color: var(--syntax-type, inherit); }
.hl-property { color: var(--syntax-property, inherit); }
.hl-tag { color: var(--syntax-tag, inherit); }
.hl-attribute { color: var(--syntax-attribute, inherit); }
.hl-inserted { color: var(--syntax-inserted, inherit); }
.hl-deleted { color: var(--syntax-deleted, inherit); }
.hl-heading { color: var(--syntax-heading, inherit); }
```

In `theme.go`, embed it with a comment in the same voice as `siteCSS`:

```go
// highlightCSS colours highlighted code. Like siteCSS it is not part of any
// style: renderPage puts it ahead of the page's style when the page has a
// highlighted fence (see needsHighlightCSS).
//
//go:embed highlight.css
var highlightCSS string
```

In `page.go`:

```go
// needsHighlightCSS reports whether a page has a highlighted fence.
func needsHighlightCSS(body []byte) bool {
	return bytes.Contains(body, []byte(` md2html-hl"`)) || bytes.Contains(body, []byte(` md2html-hl `))
}
```

In both `renderPage` and `renderFragment`, after the `siteCSS` write and before `css`, add `if needsHighlightCSS(body) { b.WriteString(highlightCSS) }`.

- [ ] **Step 4: Run** `go test ./...` and `just e2e`. Expected: PASS.

- [ ] **Step 5: Commit.** `git add highlight.css theme.go page.go page_test.go && git commit -m "feat: highlighted code is coloured by --syntax-* tokens"`

---

### Task 8: Syntax palettes and the style check (stops for the owner's palette choice)

**Files:**
- Modify: `internal/stylecheck/check.go`, `internal/stylecheck/check_test.go`
- Modify: `plugins/default/default.css`, `plugins/proof/proof.css`, `plugins/claude-artifact/artifact.css`, `stylesheet_test.go`
- Create (scratch, not committed): a palette comparison page

**Interfaces:**
- Consumes: `highlightCSS` (Task 7), the conformance samples (Task 5).
- Produces:
  - `stylecheck.Tokens` grows by the thirteen `syntax-*` names, after `status-critical`, in kind order.
  - The new rule's constant, `syntaxApart`.
  - The thirteen tokens in every built-in style, in all four theme blocks.

Order matters here. The check rules are written first and used to validate candidates, but they are committed **after** the palettes. Otherwise `TestBuiltinStylesPassTheStyleCheck` would fail on the commit in between.

- [ ] **Step 1: Write the failing style-check tests** in `internal/stylecheck/check_test.go`:
  - Add the thirteen syntax values to `referenceLight` and `referenceDark`. Use the claude-artifact candidate chosen in Step 5. Until then, use placeholders that pass, and replace them in Step 6.
  - Add these tests:

```go
func TestSyntaxTokensAreTextOnCodeBg(t *testing.T) {
	fs := Check(sheet(referenceLight, referenceDark, map[string]string{"syntax-keyword": "#e9edf1"}, nil))
	if !strings.Contains(messages(fs), "error: light --syntax-keyword on --code-bg is 1.00:1, under 3:1") {
		t.Errorf("findings:\n%s", messages(fs))
	}
}

func TestSyntaxKindsMustBeApart(t *testing.T) {
	fs := Check(sheet(referenceLight, referenceDark,
		map[string]string{"syntax-string": referenceLight["syntax-keyword"]}, nil))
	if !strings.Contains(messages(fs), "warning: light --syntax-keyword and --syntax-string are ΔE 0.0 apart, under 8") {
		t.Errorf("findings:\n%s", messages(fs))
	}
}

func TestSyntaxKindMustStandApartFromFg(t *testing.T) {
	fs := Check(sheet(referenceLight, referenceDark,
		map[string]string{"syntax-comment": referenceLight["fg"]}, nil))
	if !strings.Contains(messages(fs), "warning: light --syntax-comment is ΔE 0.0 from --fg, under 8") {
		t.Errorf("findings:\n%s", messages(fs))
	}
}

// Inserted, deleted and the rest are only held to contrast, not to the
// separation rule.
func TestSeparationCoversFiveKinds(t *testing.T) {
	fs := Check(sheet(referenceLight, referenceDark,
		map[string]string{"syntax-tag": referenceLight["syntax-attribute"]}, nil))
	if strings.Contains(messages(fs), "ΔE") {
		t.Errorf("findings:\n%s", messages(fs))
	}
}
```

  - Extend `TestEachRule` with the new rule if that test lists every rule by name.

- [ ] **Step 2: Run them.** Expected: FAIL.

- [ ] **Step 3: Implement the rules** in `check.go`:
  - Append `"syntax-comment", "syntax-keyword", "syntax-string", "syntax-number", "syntax-constant", "syntax-function", "syntax-type", "syntax-property", "syntax-tag", "syntax-attribute", "syntax-inserted", "syntax-deleted", "syntax-heading"` to `Tokens`.
  - Add a `syntaxTokens` slice copied from `Tokens[23:36]`. Check `seriesTokens` and `statusTokens` still slice the same indices.
  - In the text-contrast loop, measure each syntax token on `code-bg` only. Add a second loop rather than adding them to `textTokens`, which are also measured on `bg`. Use the same messages and thresholds.
  - Add `syntaxApart = 8.0` with the comment "a starting point, set so every built-in palette passes with margin". Then add the separation rule over `syntax-comment, syntax-keyword, syntax-string, syntax-function, syntax-type`:

```go
		apart := []string{"syntax-comment", "syntax-keyword", "syntax-string", "syntax-function", "syntax-type"}
		for i, a := range apart {
			if both(a, "fg") {
				if d := deltaE(c[a], c["fg"], normal); d < syntaxApart {
					add(Warning, theme, fmt.Sprintf("%s --%s is ΔE %s from --fg, under 8", theme, a, down(d, 1)), a, "fg")
				}
			}
			for _, b := range apart[i+1:] {
				if !both(a, b) {
					continue
				}
				if d := deltaE(c[a], c[b], normal); d < syntaxApart {
					add(Warning, theme, fmt.Sprintf("%s --%s and --%s are ΔE %s apart, under 8", theme, a, b, down(d, 1)), a, b)
				}
			}
		}
```

  - Update the package comment's first sentence to mention syntax colours.

Do not commit yet.

- [ ] **Step 4: Candidate palettes.** For each of `default`, `proof` and `claude-artifact`, build three candidate syntax palettes, each a light set and a dark set of the thirteen colours. Base each on the style's own accent family, and set `--syntax-inserted: var(--ok)` and `--syntax-deleted: var(--warn)` in every candidate, as the spec says for `default`. The grounds:

| Style | light `--fg` / `--code-bg` / `--accent` | dark `--fg` / `--code-bg` / `--accent` |
|---|---|---|
| default | `#23201d` / `#f2eee9` / `#9a4f2c` | `#ece7e1` / `#221f1c` / `#e08a5c` |
| proof | `#1a2830` / `#e8eeec` / `#0a6668` | `#dce6e8` / `#15242b` / `#5cc3bf` |
| claude-artifact | `#1b2027` / `#e9edf1` / `#2c5a85` | `#e2e7ec` / `#1a2027` / `#8db6df` |

Check each candidate by writing it into the style's CSS in a scratch copy and running `stylecheck.Check` on the assembled stylesheet, from a throwaway `go test -run` in the scratchpad or with `go run`. Keep only candidates with no errors and no warnings. Adjust the colours, not the rule, until three per style pass. If a style cannot reach three passing candidates at ΔE 8, record the best ΔE reached and bring that to the owner in Step 5 rather than lowering `syntaxApart`.

- [ ] **Step 5: STOP for the owner.** Render a scratch comparison page. For each style, show its three passing candidates applied to the Go, Python, shell, JSON, YAML, diff and Markdown conformance samples, in light and in dark, side by side. Use md2html itself: convert one page per candidate with the candidate's CSS as `--css`, and join them. Publish it as an artifact for review and give the owner the link. Include in it the kind table rendered as a legend, and the `keyword.operator` decision, so both can be judged on real code. **Do not continue until the owner has picked one palette per style.** If the owner asks for a change to the kind table, make it in `internal/highlight/kinds.go` with its test, as a `--fixup` of Task 2's commit, before Step 6.

- [ ] **Step 6: Write the chosen palettes.**
  - Write each style's chosen tokens into all four theme blocks of its CSS, next to its other colours, in the same order as `stylecheck.Tokens`: bare `:root`, the guarded dark media block, `:root[data-theme="dark"]` and `:root[data-theme="light"]`.
  - Replace the placeholders in `referenceLight`/`referenceDark` with claude-artifact's chosen values.
  - Add to `stylesheet_test.go`:

```go
// Every syntax token is defined in every built-in style, in every theme
// block, so highlighted code renders in every theme state.
func TestBuiltinStylesDefineEverySyntaxToken(t *testing.T) {
	for _, style := range []string{"default", "proof", "claude-artifact"} {
		css, err := NewPluginCache().Stylesheet([]string{style}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range highlight.Kinds {
			name := "--syntax-" + k.Name() + ":"
			if n := strings.Count(css, name); n < 4 {
				t.Errorf("%s defines %s %d times, want it in all four theme blocks", style, name, n)
			}
		}
	}
}
```

  `default.css` is in every stacked stylesheet, so a style layered on `default` counts `default`'s blocks too. Count within the style's own file, read with `p.readFile`, rather than within the assembled sheet.

- [ ] **Step 7: Run** `go test ./...` and `just e2e`, then commit in two steps:

```bash
git add plugins stylesheet_test.go
git commit -m "feat: syntax palettes in the built-in styles"
go test ./... && git add internal/stylecheck && git commit -m "feat: --check-style checks syntax colours"
```

The palette commit body names each chosen palette and its smallest separation ΔE and contrast in each theme. The check commit body says the separation threshold is 8 and why.

---

### Task 9: The CLI

**Files:**
- Modify: `cmd/md2html/main.go`, `cmd/md2html/main_test.go`
- Create: `cmd/md2html/size_test.go`

**Interfaces:**
- Consumes: `Options.NoHighlight` (Task 6), `md2html.Languages` and `Grammar` (Tasks 3 and 4).
- Produces: the `--no-highlight` and `--list-languages` flags.

- [ ] **Step 1: Write the failing tests** in `main_test.go`, using its existing `run` helper and temp-dir conventions:
  - `TestNoHighlightFlag`: converting a file with a Go fence and `--no-highlight` writes no `md2html-hl`, and without the flag it writes one.
  - `TestListLanguages`: `--list-languages` exits 0 and prints lines such as `go  grammars  MIT  https://github.com/microsoft/vscode/blob/…`. Assert that a line starts with `go ` and contains `grammars` and `MIT`, that `shellscript`'s line names its aliases `(bash, sh, shell, zsh)`, and that the output is sorted.
  - `TestListLanguagesWithAPlugin`: with `--plugin` naming a grammar-only plugin from a temp dir, its language appears with the plugin's name, and a broken plugin exits 2 with the reason, as `--list-styles` does.
  - `TestListLanguagesTakesNoFiles`: `--list-languages page.md` behaves as `--list-styles` does with an entry. Read what that does today and match it.

- [ ] **Step 2: Run them.** Expected: FAIL.

- [ ] **Step 3: Implement.**
  - Add `noHighlight = fs.Bool("no-highlight", false, "leave fenced code plain instead of highlighting it")` beside `noCharts`, and pass it through `buildOptions` as a new `noHighlight bool` parameter after `noCharts`, setting `NoHighlight`. Update every caller of `buildOptions`, including tests. `TestBuildOptionsMatchesBuiltinsOrder` calls it.
  - Add `listLanguages = fs.Bool("list-languages", false, "list the languages fences can be highlighted in, from built-in grammars and any --plugin, and exit")` beside `listStyles`. Handle it right after `--list-styles`:

```go
	if *listLanguages {
		return listAvailableLanguages(plugins, stdout, stderr)
	}
```

```go
// listAvailableLanguages prints every language a fence can name: its
// name and aliases, the plugin it comes from, and its licence and source
// when known. Plugins load as for a run, so a broken one exits 2.
func listAvailableLanguages(dirs []string, stdout, stderr io.Writer) int {
	cache := md2html.NewPluginCache()
	if _, err := checkPlugins(cache, dirs, nil, stderr); err != nil {
		fmt.Fprintf(stderr, "md2html: %v\n", err)
		return 2
	}
	loaded, err := cache.LoadAll(dirs, "", nil)
	if err != nil {
		fmt.Fprintf(stderr, "md2html: %v\n", err)
		return 2
	}
	warn := func(m string) { fmt.Fprintf(stderr, "md2html: %s\n", m) }
	for _, g := range md2html.Languages(cache, loaded, warn) {
		name := g.Name
		if len(g.Aliases) > 0 {
			name += " (" + strings.Join(g.Aliases, ", ") + ")"
		}
		fields := []string{name, g.PluginName()}
		for _, f := range []string{g.License, g.Source} {
			if f != "" {
				fields = append(fields, f)
			}
		}
		fmt.Fprintln(stdout, strings.Join(fields, "  "))
	}
	return 0
}
```

  - Add a sentence to the usage text beside the `--list-styles` one at line 68: `md2html --list-languages lists the languages code fences are highlighted in.`

- [ ] **Step 4: The size budget.** `cmd/md2html/size_test.go`:

```go
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// sizeBudget is the stripped binary's ceiling: 23.7 MB at v0.12.0, about
// +2.5 MB for highlighting by the spike's measure, and some margin. Raise
// it deliberately, in a commit that says why.
const sizeBudget = 27 << 20

func TestBinaryStaysUnderItsSizeBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	out := filepath.Join(t.TempDir(), "md2html")
	cmd := exec.Command("go", "build", "-ldflags", "-s -w", "-o", out, ".")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, b)
	}
	fi, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() > sizeBudget {
		t.Errorf("binary is %.1f MB, over the %.1f MB budget", float64(fi.Size())/(1<<20), float64(sizeBudget)/(1<<20))
	}
	t.Logf("binary is %.1f MB", float64(fi.Size())/(1<<20))
}
```

  Run it once with `-v` and record the measured size. Set `sizeBudget` to the measured size rounded up to the next 0.5 MB plus 1 MB, and update the comment to give the measured figure.

- [ ] **Step 5: Run** `go test ./...`. Expected: PASS.

- [ ] **Step 6: Commit.** `git add cmd/md2html && git commit -m "feat: --no-highlight and --list-languages"`

---

### Task 10: Browser test

**Files:**
- Modify: `e2e/browser_test.go`

**Interfaces:**
- Consumes: everything above, through `md2html.Convert`.

- [ ] **Step 1: Write the test.** It follows `TestBrowserMermaidFollowsTheStyle`'s shape, using `serveDir`, `writeFiles`, `newBrowserCtx`, `figResolveVar`, `figSetTheme` and `emulation.SetEmulatedMedia`:

```go
// Every token kind takes its colour from the style's --syntax-* token, in
// the system light and dark themes and in both theme-toggle states, so a
// highlighted fence reads in whatever theme the reader is in.
func TestBrowserSyntaxColoursFollowTheTheme(t *testing.T) {
	dir, baseURL := serveDir(t)
	src := "```go\n// c\nfunc F() string { return \"s\" + 1 }\ntype T struct{ X int }\nconst k = true\n```\n\n" +
		"```html\n<a href=\"x\">y</a>\n```\n\n```json\n{\"key\": 1}\n```\n\n```diff\n--- a\n+++ b\n@@ -1 +1 @@\n-old\n+new\n```\n"
	files := map[string][]byte{}
	for _, style := range []string{"default", "proof", "claude-artifact"} {
		page, err := md2html.Convert([]byte(src), md2html.Options{Styles: []string{style}})
		if err != nil {
			t.Fatalf("%s: %v", style, err)
		}
		files[style+".html"] = page
	}
	writeFiles(t, dir, files)
	ctx := newBrowserCtx(t)

	const probe = figResolveVar + `
	(() => {
		const out = {};
		for (const el of document.querySelectorAll('code.md2html-hl span[class^="hl-"]')) {
			const kind = el.className.slice(3);
			if (out[kind]) continue;
			const probe = document.createElement("span");
			probe.style.color = "var(--syntax-" + kind + ")";
			document.body.appendChild(probe);
			out[kind] = {got: getComputedStyle(el).color, want: getComputedStyle(probe).color};
			probe.remove();
		}
		return out;
	})()`
	type pair struct{ Got, Want string }
	states := []struct {
		name, scheme, stamp string
	}{{"system light", "light", ""}, {"system dark", "dark", ""}, {"toggled dark", "light", "dark"}, {"toggled light", "dark", "light"}}
	for _, style := range []string{"default", "proof", "claude-artifact"} {
		for _, s := range states {
			var got map[string]pair
			actions := []chromedp.Action{
				emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: s.scheme}}),
				chromedp.Navigate(baseURL + "/" + style + ".html"),
				chromedp.WaitVisible("code.md2html-hl", chromedp.ByQuery),
			}
			if s.stamp != "" {
				actions = append(actions, figSetTheme(s.stamp))
			}
			actions = append(actions, chromedp.Evaluate(probe, &got))
			if err := chromedp.Run(ctx, actions...); err != nil {
				t.Fatalf("%s %s: %v", style, s.name, err)
			}
			for _, k := range []string{"comment", "keyword", "string", "number", "constant", "function",
				"type", "property", "tag", "attribute", "inserted", "deleted", "heading"} {
				p, ok := got[k]
				if !ok {
					t.Errorf("%s %s: no %s span on the page", style, s.name, k)
					continue
				}
				if p.Got != p.Want {
					t.Errorf("%s %s: %s is %s, want --syntax-%s %s", style, s.name, k, p.Got, k, p.Want)
				}
			}
		}
	}
}
```

If a kind does not appear in the fixture with the real grammars, for example if no `property` comes out of this JSON, add a line to the fixture that produces it. Find one by checking the conformance goldens for the scope. Do not drop the kind from the list.

- [ ] **Step 2: Run** `just e2e`. Expected: PASS. If it fails, the cause is in Task 7 or 8. Fix it there with `--fixup`.

- [ ] **Step 3: Commit.** `git add e2e && git commit -m "test: syntax colours follow the theme in the browser"`

---

### Task 11: Documentation

**Files:**
- Modify: `docs/guide/code-blocks.md`, `docs/guide/styles.md`, `docs/guide/silent-traps.md`, `docs/guide/warnings.md`, `docs/guide/quick-reference.md`, `README.md`, `CHANGELOG.md`, `guide_test.go`

- [ ] **Step 1: `code-blocks.md` gains `## Highlighting`,** after `## Captions`. It covers:
  - A fence is highlighted when its language names a grammar, ignoring case. Show one example fence and say there is nothing to configure.
  - Aliases: `sh`, `bash` and `zsh` reach `shellscript`, `yml` reaches `yaml`, `py` reaches `python`. `md2html --list-languages` lists them all.
  - No match, no change. A near miss warns, as in `fence at line 12: no grammar for "pyhton"; did you mean "python"?`.
  - Opting out, at three levels: `{.go .no-highlight}`, `highlight: false` and `--no-highlight`.
  - `mermaid`, `fig` and `vega-lite` are never highlighted, even when they fall back to a code block.
  - Supplying a language md2html does not ship: a local plugin with a `grammars` key and a `.tmLanguage.json`. Link to the `styles` section's manifest reference. Name the languages left out for licence, taken from `plugins/grammars/REPORT.md`, and say each can be supplied this way.
  - Credit: grammars come from the `tm-grammars` collection, each under its own licence (`md2html --licenses`).

  Use the `md2html-authoring` skill for the page's conventions.

- [ ] **Step 2: `styles.md`** gains:
  - the `grammars` manifest key, with the spec's example manifest and key table, adding the `scope` key and saying it is required only in built-in plugins;
  - the language-name rules;
  - precedence, clashes and cross-plugin includes;
  - the thirteen `--syntax-*` tokens, what each colours, and that `default` sets `inserted` and `deleted` from `--ok` and `--warn`;
  - under **Checking a style**, the syntax rules: contrast on `--code-bg` and the ΔE 8 separation of comment, keyword, string, function and type, from `--fg` and from each other.

  Extend the starter style the guide gives (`TestGuideStarterStyleBuilds` checks it) with the thirteen tokens, so a reader who copies it still passes `--check-style`.

- [ ] **Step 3: `silent-traps.md`** gains: **a fence whose language has no grammar renders plain**, with no warning unless the name is a near miss of a known one; `--list-languages` shows what exists. Add a `TestGuideClaims` entry checking it against the binary: a `text` fence and a `madeuplang` fence both convert with no warning and no `md2html-hl`.

- [ ] **Step 4: `warnings.md`** gains each new message, in the file's existing `**message** — what to do` form:
  - `fence at line N: no grammar for "X"; did you mean "Y"?`
  - `fence at line N: highlighting "X" took over 2s; showing it plain`
  - `fence at line N: highlighting "X" failed: …; showing it plain`
  - `grammar "X" from plugin P: N patterns cannot be translated and are skipped: …`
  - `plugins A and B both provide language "X"; using B's`
  - `front matter highlight "X" is not true or false; highlighting the page's fences`
  - `front matter plugins: …; ignoring it`

  Revise the existing `front matter plugins has no effect without style` entry to say a plugin that provides languages does have an effect. Run `TestGuideWarningsExistInTheCode`.

- [ ] **Step 5: `quick-reference.md`** gains rows for "Highlighted code" and "Plain code in a known language".

- [ ] **Step 6: README and CHANGELOG.**
  - **README:** one paragraph on highlighting, and flag rows for `--no-highlight` and `--list-languages`.
  - **CHANGELOG, `## Unreleased`:**
    - Under **Added**:
      - build-time highlighting;
      - the built-in `grammars` plugin, with its count;
      - the `grammars` manifest key;
      - the `--syntax-*` tokens in every built-in style;
      - `--no-highlight` and `--list-languages`;
      - the style check's syntax rules.
    - Under **Changed**:
      - fences in known languages now carry spans and `md2html-hl`;
      - `--list-styles` omits plugins with no styles;
      - the binary's size, from Task 9;
      - a **scope-to-kind table** paragraph that lists the table, says it is part of the output, and says a later change to it will be announced here, as the spec requires.

- [ ] **Step 7: Run** `go test ./...`, then commit: `git add docs README.md CHANGELOG.md guide_test.go && git commit -m "docs: highlighting in the guide, README and changelog"`

---

### Task 12: Close out

- [ ] **Step 1: Read the whole diff once** against the spec's Testing section, item by item. List any item no test covers, and add the test with a `--fixup` to the task that owns the code.
- [ ] **Step 2: Measure.**
  - Run `go build -ldflags "-s -w" -o /tmp/md2html ./cmd/md2html && ls -l /tmp/md2html`.
  - Time a run over the repo's docs with and without `--no-highlight`, using `just site` and the same command with `--no-highlight` appended, three runs each.
  - Put both figures in the CHANGELOG entry with a `--fixup` of Task 11's commit.
- [ ] **Step 3: Update the spec** to match what shipped: set its status line, record the seven decisions from this plan's "Decisions" section, and answer its three open questions with what was chosen. Commit with `docs:`.
- [ ] **Step 4: Autosquash.**
  1. Find the base with `git merge-base main HEAD`. Re-read `git log --oneline` right before this step, since the owner may have committed in parallel.
  2. Back up with `git branch backup-syntax-highlighting-$(date +%Y%m%d-%H%M%S)`.
  3. Run `GIT_SEQUENCE_EDITOR=: git -c core.hooksPath=/dev/null rebase -i --autosquash <base>`.
  4. Check that `git diff <backup>..HEAD` is empty.
  5. Run `git rebase --exec 'go test ./...' <base>`.
- [ ] **Step 5: Report.** Give:
  - the commits;
  - test and e2e results;
  - the grammar counts and every exclusion with its reason;
  - binary size and build-time cost;
  - the chosen palettes;
  - anything skipped.
