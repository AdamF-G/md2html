# Raw Attribute Blocks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A span's or a link's `{…}` block reaches Chips and LinkAttrs as the author typed it, with no inline Markdown or HTML parsed inside it, and `\"` escaping a quote.

**Architecture:** `rawBlockParser`, a goldmark inline parser on `{`, takes a block after a `]` or a `)` as a raw `ast.String`. Attribute values are entity-decoded once where they are read.

**Tech Stack:** Go 1.26; goldmark v1.8.6.

**Spec:** `docs/specs/2026-11-19-raw-attribute-blocks-design.md`.

## Global Constraints

- Work on `feat/keep-span-titles`. House commit style; no attribution; nothing pushed.
- A brace not straight after `]` or `)` renders exactly as before.
- `gofmt -l .`, `go vet ./...` and `go test ./...` before the commit.

## Review Focus

1. A block with no closing `}` on its line, `[x]{title="a`, is not taken and renders as before.
2. `[x]{.chip}` inside a code span stays literal code.
3. An escaped `\]{…}` is taken raw, and nothing else changes for it.
4. A footnote reference `[^1]{#n}` is unchanged apart from its block's inside.
5. Entities: `title="a &amp; b"` gives the value `a & b`, on a span, a link and a heading.

### Task 1: Take a span's and a link's block raw

**Files:**
- Create: `rawblock.go`
- Modify: `md2html.go` (`newParser`: the inline parser)
- Modify: `chip.go` (value decoding, the quote hint)
- Modify: `linkattrs.go` (value decoding)
- Modify: `docs/guide/inline-marks.md`, `docs/guide/warnings.md`, `CHANGELOG.md`
- Test: `rawblock_test.go`, `chip_test.go`

- [ ] **Step 1: Write the failing tests**

`rawblock_test.go`, through `convert`:

| Source | Want |
|---|---|
| `x [b]{.chip tip="a *b* c"} z` | a tip host `b`, a tooltip `a *b* c`, no `<em>` |
| `x [c]{.chip tip="x <b> y"} z` | a tooltip `x &lt;b&gt; y`, no `<b>` |
| `x [d]{.chip title="say \"hi\" it's"} z` | `title="say &#34;hi&#34; it&#39;s"`, no warning |
| `x [l](https://e.x){title="a *b*"} z` | the `<a>` has `title="a *b*"` |
| `x [m]{title="a &amp; b"} z` | `title="a &amp; b"` (the value `a & b`) |
| `## T {title="a &amp; b"}` | `title="a &amp; b"` on the h2 |
| `` `[x]{.chip}` `` | `<code>[x]{.chip}</code>` |
| `x [y]{title="a` | the text as before, and no span |
| `f(x){*a*}` | `{*a*}`, no `<em>` |
| `a {*b*} c` | `{<em>b</em>}` (a brace elsewhere is unchanged) |

Change `TestChipsBackslashQuoteInValueWarns` to `TestChipsBackslashQuoteInValue`: `title="a \"b\""` gives `title="a &#34;b&#34;"` and no warning.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestRawBlock|TestChipsBackslashQuote' .`
Expected: FAIL on the rows above. The code span row and the brace-elsewhere row pass already, as pins.

- [ ] **Step 3: Implement**

- `rawBlockParser`: `Trigger` is `{`. `Parse` returns nil unless the source byte before the trigger is `]` or `)` and `braceSpan` closes the block on the line. Otherwise it returns `ast.NewString(block)` with `SetRaw(true)` and advances past the `}`. Register it in `newParser` with `parser.WithInlineParsers(util.Prioritized(rawBlockParser{}, 150))`.
- Decode each value with `html.UnescapeString` in `checkAttrBlock` and `applyLinkAttrs`.
- The quote hint becomes `; a quote inside a double-quoted value is written \"`.
- Docs:
  - `inline-marks.md` says a value is literal text and `\"` escapes a quote.
  - `warnings.md` drops "the other kind of quote".
  - The changelog's Changed gets the spec's compatibility line, and its Added tip entry is unaffected.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./... && (cd e2e && go test -count=1 ./...)`
Expected: PASS.

- [ ] **Step 5: Commit**

`fix: a span's or a link's block is taken as written, before inline Markdown`
