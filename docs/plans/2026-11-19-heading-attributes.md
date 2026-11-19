# Heading Attributes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A heading's trailing `{…}` is read by md2html's parser with a span's rules, and a block touching a `]` or a `)` goes to the span or link before it.

**Architecture:** `spanBlock`'s checks become `checkAttrBlock`, shared by spans and headings. md2html builds its goldmark parser from the default block parsers, with the ATX and setext heading parsers wrapped (`headingAttrParser`) and `parser.WithAttribute()` gone. Each wrapper reads, sets and trims a heading's trailing block before inline parsing. A heading node renderer writes the attributes, so `aria-` names reach the page without touching goldmark's globals. Chips warns about a heading-ending block nothing took, and the page title drops a link's block.

**Tech Stack:** Go 1.26; goldmark v1.8.6 (`parser`, `ast`, `renderer/html`, `text`); `golang.org/x/net/html`.

**Spec:** `docs/specs/2026-11-19-heading-attributes-design.md`. §1 is the rule, §2 what a block keeps, §3 where it happens, the title, closing hashes and `aria-`, §4 compatibility and the release notes, and §5 the docs.

## Global Constraints

- Work on `feat/keep-span-titles`. Conventional prefixes. Commit bodies are wrapped at 72 columns and start from what was wrong before. No attribution trailer. Nothing is pushed.
- No exported API changes.
- goldmark's package-level filters (`HeadingAttributeFilter` and the rest) are never modified.
- `figrender.go`'s own parser is unaffected.
- A plain `## Title {#id .class}` renders exactly as today, and `TestUnchangedOutput`'s goldens stay byte-identical.
- Every warning is raised as `warn(fmt.Sprintf(...))` where it is built and has a `docs/guide/warnings.md` entry.
- Run `gofmt -l .`, `go vet ./...` and `go test ./...` before every commit.

## Review Focus

1. **An apostrophe or quote before the block**: `## Don't panic {#calm}` and `## It's 5" tall {#x}` keep their ids. Pinned in Task 2.
2. **An API path**: `## GET /v1/jobs/{id}` renders as written, with no warning. Pinned in Task 2.
3. **Closing hashes**: `## Title ## {#id}` renders "Title" with id `id`, and `## Title ##` renders "Title". Pinned in Task 2.
4. **A setext heading**, `Title [z]{.chip}` over `---`, gives a chip, and `Title {#t}` over `===` gives an `<h1 id="t">`. Pinned in Task 2.
5. **The heading's block containing Markdown or a `}` in quotes**: `## T {title="a } *b*"}` gives `title="a } *b*"`, not emphasis. Pinned in Task 2.

---

### Task 1: One set of checks for a span's and a heading's block

**Files:**
- Modify: `chip.go` (`spanBlock` becomes a call to `checkAttrBlock`)
- Modify: `docs/guide/warnings.md` (the three span entries name "a span's or a heading's block")
- Test: `chip_test.go`

**Interfaces:**
- Produces: `func checkAttrBlock(who, block string, tips bool, warn func(string)) (spanAttrs, bool)`. `who` is the message prefix, `bracketed span [x]` or `heading "x"`. `tips` is false for a heading, which drops `tip` and `tip-delay` with no word, as any unlisted name. `spanBlock(label, block, warn)` becomes `checkAttrBlock("bracketed span ["+label+"]", block, true, warn)`.

- [ ] **Step 1: Write the failing test**

`TestCheckAttrBlockHeadingWho`: `checkAttrBlock(`heading "Setup"`, "#s data-flag", false, w)` returns false, and one warning starting `heading "Setup": data-flag has no value`. `checkAttrBlock(`heading "T"`, `#t tip="n" tip-delay="0ms"`, false, w)` returns ok with `id` `t`, no `tip`, and no warning.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -run TestCheckAttrBlockHeadingWho .`
Expected: compile failure, because `checkAttrBlock` is undefined.

- [ ] **Step 3: Implement**

Move `spanBlock`'s body into `checkAttrBlock`, with `who` in place of `"bracketed span [%s]"` in each `Sprintf` (`"%s: …", who, …`) and the tip handling behind `tips`. Rewrite the three warning entries' bold text to `**… is not an attribute name**`, `**… has no value**` and `**… has no closing quote**`, saying "a span's or a heading's block".

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./...`
Expected: PASS, every existing span test included.

- [ ] **Step 5: Commit**

`refactor: a span's block checks become checkAttrBlock, for headings too`

---

### Task 2: Headings read their block with md2html's parser

**Files:**
- Create: `heading.go`
- Modify: `md2html.go` (`newParser`)
- Test: `heading_test.go`

**Interfaces:**
- Consumes: `checkAttrBlock` from Task 1.
- Produces:
  - `func headingParsers(warn func(string)) []util.PrioritizedValue`, the default block parsers with the ATX (600) and setext (100) parsers wrapped;
  - `type headingAttrParser struct{ parser.BlockParser; setext bool; warn func(string) }`, overriding `Open` for ATX and `Close` for setext;
  - `func takeHeadingBlock(line []byte) (start int, block string, ok bool)`, the §1 scan;
  - a node renderer for `ast.KindHeading`, registered in `newParser` at priority 100.

- [ ] **Step 1: Write the failing tests**

`heading_test.go`, through `convert(t, src, w)`. Each row asserts the `<hN …>` opening tag, or the shown text, and the number of warnings:

| Source | Want |
|---|---|
| `## A {#a .k}` | `<h2 id="a" class="k">A` (exactly as today) |
| `## Rollback [beta]{.chip}` | `<span class="chip">beta</span>` in the h2; the h2 has no class |
| `## See [d](https://e.x){.x}` | the `<a>` has `class="x"`; the h2 has no class |
| `## T {title='q'}` | `title="q"` on the h2 |
| `## T {lang=fr}` | `lang="fr"` |
| `## T {data-step=1}` | `data-step="1"` |
| `## T {aria-label="L"}` | `aria-label="L"` |
| `## T {#t onclick="x"}` | `id="t"` and no `onclick` |
| `## Don't panic {#calm}` | `id="calm"`, text `Don't panic` |
| `## It's 5" tall {#x}` | `id="x"` |
| `## GET /v1/jobs/{id}` | text `GET /v1/jobs/{id}`, no warning |
| `## T {}` | text `T {}`, no warning |
| `## Setup {#s data-flag}` | text contains `{#s data-flag}`, one warning `has no value` |
| `## T {#t a/b=1}` | the block stays, one warning `is not an attribute name` |
| `## Title ## {#id}` | `id="id"`, text `Title` |
| `## Title ##` | text `Title` |
| `## T {title="a } *b*"}` | `title="a } *b*"`, no `<em>` |
| `## T {#t tip="n"}` | `id="t"`, no tooltip, no warning |
| setext `Title [z]{.chip}\n---` | a chip in the h2 |
| setext `Title {#t}\n===` | `<h1 id="t">` |

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run TestHeading .`
Expected: FAIL on the `]{`, single-quote, unquoted, `aria-` and setext-chip rows. The plain row and the API row pass already, as pins.

- [ ] **Step 3: Implement**

- **`takeHeadingBlock`:** scans left to right, skipping a backslash and its next byte. At each `{` it asks whether `braceSpan(line, i)` is the last non-space index. The first that is gives the block, and `ok` is false if the byte before that `{` is `]` or `)`. A quote outside the block is ordinary.
- **ATX `Open`:** calls the wrapped parser, built with no options. On the node's last line it takes the block, checks it with `checkAttrBlock(`heading "`+text+`"`, block, false, warn)`, sets each attribute with `node.SetAttribute`, and trims the line. Then it re-applies goldmark's closing-hash trim (the loop in goldmark's `atxHeadingParser.Open`) to what's left.
- **Setext `Close`:** calls the wrapped `Close`, then does the same on the last line, if the node is still in the tree. A setext heading has no closing hashes.
- **`newParser`:** builds the parser with `goldmark.WithParser(parser.NewParser(parser.WithBlockParsers(headingParsers(warn)...), parser.WithInlineParsers(parser.DefaultInlineParsers()...), parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...)))` and drops `parser.WithAttribute()`.
- **The heading renderer:** writes `<hN`, then each attribute whose name is a global attribute, or starts with `data-` or `aria-`, as ` name="escaped"`, then `>`. The close tag is as goldmark writes it.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./...`
Expected: PASS, `TestUnchangedOutput` included. A guide claim that pins the old behavior fails here, such as "puts the class on the `<h2>`" or silent trap 17's. Update it in Task 4's guide text, but keep the suite green within this task by moving the claim to the new behavior now, and ledger it.

- [ ] **Step 5: Commit**

`feat: a heading's attribute block is read by md2html's parser`

---

### Task 3: A block nothing takes warns, and the title drops a link's block

**Files:**
- Modify: `chip.go` (`chips`: the heading check after the rewrite)
- Modify: `page.go` (`extractTitle`)
- Modify: `docs/guide/warnings.md`
- Test: `heading_test.go`, `page_test.go`

- [ ] **Step 1: Write the failing tests**

- `TestHeadingUnclaimedBlockWarns`: each of `## Call f(x){#call}`, `` ## Use `a]`{#x} `` and `## Lit\]{#x}` gives one warning containing `write a space before`, and the text keeps the block.
- `TestHeadingClaimedBlockQuiet`: `## R [b]{.chip}` gives no warning.
- `TestTitleDropsLinkBlock`: `# See [docs](d.md){.x}` and `# See [docs](d.md) and [x](y.md){.k} more` give `<title>See docs</title>` and `<title>See docs and x more</title>`.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestHeadingUnclaimed|TestHeadingClaimed|TestTitleDropsLinkBlock' .`
Expected: FAIL. No warning is raised, and the titles carry `{.x}` and `{.k}`.

- [ ] **Step 3: Implement**

- **In `chips`, after `rewriteText`:** for each `<h1>`–`<h6>`, if its last text node ends in a block that `parseAttrs` reads with an id, a class or a `key=value`, and the byte before its `{` is `]` or `)` (or the `{` starts the text node right after an element), call `warn(fmt.Sprintf("heading %q: {%s} touches a ] or ) but no span or link took it; write a space before it to give it to the heading", text, block))`.
- **In `extractTitle`:** read the `<h1>`'s text with a walker that, after an `<a>` or `<img>`, skips a following text node's leading `{…}` when `parseAttrs` reads it.
- Add the guide entry.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

`feat: a heading's block nothing takes warns, and a title drops a link's block`

---

### Task 4: The guide, the compat suite, the changelog and the TODO

**Files:**
- Modify: `docs/guide/page-structure.md` ("Headings and anchors")
- Modify: `docs/guide/silent-traps.md` (remove trap 17 and renumber)
- Modify: `guide_test.go` (`TestGuideClaims`)
- Modify: `compat/compat_test.go`
- Modify: `CHANGELOG.md`
- Modify: `TODO.md` (remove the heading item)

- [ ] **Step 1: The guide**

The "Headings and anchors" text says:
- a heading's block reads like a span's;
- the `]`/`)` rule, and the space that keeps a block on the heading;
- the API-path case;
- that `aria-` names now reach the heading.

Remove the paragraph saying a block straight after a link belongs to the heading. Add `TestGuideClaims` cases for the chip at the end of a heading, a single-quoted value and the API path, each quoting the guide's text.

- [ ] **Step 2: The compat suite**

Add `{"heading single-quoted value", "## T {title='x'}\n", "h2", "x"}` if Pandoc agrees on it. Run `just compat`; if Pandoc does not agree, record the case where the suite records a divergence instead.

- [ ] **Step 3: The changelog**

Under Unreleased, Changed, add a **breaking** entry listing spec §4's seven items, with the three `grep` commands from §4 verbatim and the note that setext headings are not searched.

- [ ] **Step 4: Verify and commit**

Run: `go test ./... && just compat`
Expected: PASS, apart from compat cases unrelated to this change.

`docs: heading attributes in the guide, the compat suite and the changelog`
