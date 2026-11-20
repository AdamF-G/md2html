# Span Labels Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A bracketed span's label is inline Markdown, brackets nest, and `\]` is a bracket.

**Architecture:**
- `spanTransformer`, a goldmark AST transformer, pairs a raw block after `]` with its `[` and wraps the nodes between in a `spanNode`.
- `spanNode` renders as `<span md2html-span="{…}">`, which Chips resolves through `checkAttrBlock`, or unwraps back to literal text.
- `chipRe` keeps only the bare badges.

**Tech Stack:** Go 1.26; goldmark v1.8.6.

**Spec:** `docs/specs/2026-11-19-span-labels-design.md`.

## Global Constraints

- Work on `feat/keep-span-titles`. House commit style; no attribution; nothing pushed.
- Every existing span behaviour holds: warnings, chip modifier, tips, stray quotes, empty and bare-word blocks literal.
- `gofmt -l .`, `go vet ./...`, `go test ./...`, the fuzz seeds and `just e2e` before the commit.

## Review Focus

1. `[x\]{.c}` stays literal: an escaped bracket closes nothing.
2. `[see [x]{.chip}](u)`: the link wins, and no span is made in its text.
3. A refused block, `[b]{.chip data-flag}`, still renders `[b]{.chip data-flag}` literally and warns once.
4. A page title: `# A [b]{.chip} c` is titled "A c"; `# A [b]{.lead} c` is "A b c".
5. Two spans in a row, `[a]{.x}[b]{.y}`, and a span right after a link, `[l](u) [s]{.x}`.

### Task 1: Spans are paired on goldmark's tree

**Files:**
- Create: `spantree.go` (`spanNode`, `spanTransformer`, its renderer, `spanBlockEnd`)
- Modify: `rawblock.go` (`spanBlockEnd` in place of `braceSpan`), `md2html.go` and `figrender.go` (register them), `chip.go` (`chipRe`, `chipNodes`, `stripChipTokens`, resolving `md2html-span`), `page.go` (`titleText`)
- Modify: `docs/guide/inline-marks.md`, `CHANGELOG.md`
- Test: `spantree_test.go`; existing chip, title and fuzz tests updated where they pinned a plain label

- [ ] **Step 1: Write the failing tests**

| Source | Want |
|---|---|
| `x [need *a*]{.chip} y` | `<span class="chip">need <em>a</em></span>` |
| `x [**proven**]{.chip} y` | `<span class="chip chip-proven"><strong>proven</strong></span>` |
| `x [a [b] c]{.x} y` | `<span class="x">a [b] c</span>` |
| `x [a \] b]{.x} y` | `<span class="x">a ] b</span>` |
| `x [x\]{.c} y` | no `<span`; text `[x]{.c}` |
| `x [see [x]{.chip}](https://e.x) y` | an `<a>` with text `see [x]{.chip}` or the badge literal; no span inside it |
| `x [b]{.chip data-flag} y` | text `[b]{.chip data-flag}`, one warning |
| `x [a]{.x}[b]{.y} y` | two spans |
| `x [l](https://e.x) [s]{.x} y` | a link and a span |
| `x [b]{.c title=don't} y` | `title="don&#39;t"` |
| `## R [beta *x*]{.chip}` | a chip in the h2 with an `<em>` |
| title `# A [b]{.chip} c` | `<title>A c</title>` |
| title `# A [b]{.lead} c` | `<title>A b c</title>` |
| fig box `"[a *b*]{.chip}"` | a chip with an `<em>` |

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run TestSpanTree .`
Expected: FAIL on the Markdown, nested and escaped rows.

- [ ] **Step 3: Implement** as the spec says. Update the guide, which loses "the label must be plain text" and gains the three forms, and the changelog's Changed.

- [ ] **Step 4: Verify**

Run: `go test ./... && (cd e2e && go test -count=1 ./...)`, then fuzz `FuzzChipLabel` and `FuzzSpanTitle` for 30s each.
Expected: PASS.

- [ ] **Step 5: Commit**

`feat: a bracketed span's label is inline Markdown, and brackets nest`
