---
title: Fence line capture
subtitle: owning the ::: fence line, and the four traps that come from not owning it
date: 2026-09-13
---

# Fence line capture

[[toc]]

## 1. Summary

md2html's brace-free container form, `::: kind`, is not a syntax the fenced
div parser implements. It is recovered after the fact, by reading the first
word of the container's first paragraph. This document records why that
recovery fails, the four inputs where it does, and a fix that removes the
whole family by having the parser own the fence line.

The fix is net-negative code. Two workaround functions and one load-bearing
transform ordering exist only to compensate for the fence line leaking into
the document body, and all three go away.

::: callout
**Scope.** This precedes the "Splitting the `fig` vocabulary" section of
[the standards alignment spec](./2026-09-13-standards-alignment.md), which
promotes `stats` and `defs` to containers. Those two kinds are
*defined* to hold a definition list, and a definition list is one of the two
constructs that steals the fence line — so that promotion is unbuildable
as designed until this lands.
:::

## 2. The mechanism

`goldmark-fences` parses an attribute block and nothing else. From its
`fencedContainerParser.Open`:

```go
attrs, ok := parser.ParseAttributes(reader)
if ok {
    for _, attr := range attrs {
        node.SetAttribute(attr.Name, attr.Value)
    }
}
```

When `ok` is false — which is every brace-free fence — **the reader is never
advanced past the remainder of the line**. The container node is still
created and the nesting machinery still works; only the attributes are
missing, and the text after `:::` stays in the content stream as the opening
characters of the container's first block, still physically on the fence
line.

So `::: card` parses as a container whose content begins with the literal
text `card`, joined by a newline to whatever followed.

md2html's `::: kind` form is built entirely on mining that leftover:
`firstWord` takes the first word of the container's first *paragraph* and
looks it up in `containerKinds`. That recovery is sound only when the
leftover text does in fact become a paragraph.

## 3. What can steal the fence line

Almost every block construct begins on its own line and cannot reach
backwards, so the leftover becomes a paragraph and the kind is recovered
normally. Exactly two constructs in this dialect reinterpret a line that is
*already in progress*:

| Construct | What the fence line becomes |
|---|---|
| A definition list `:` marker on the next line | a `<dt>` |
| A setext underline `===` or `---` on the next line | part of an `<h1>` or `<h2>` |

Both work by looking back at a paragraph line already open. Nothing else
does, which is what makes this trap precisely boundable rather than an
open-ended hazard.

### 3.1 Measured

`::: card` against each possible first block:

| Case | First block | Result |
|---|---|---|
| A | `term` then `: def` | `<div><dl><dt>card</dt><dt>term</dt><dd>def</dd></dl></div>`, warns |
| B | `Heading` then `===` | `<h1 id="card-heading-text">card Heading text</h1>`, warns |
| C | `Heading` then `---` | as B, an `<h2>`, warns |
| D | `::: card[Title]`, then `term` / `: def` | `<dt>card[Title]</dt>`, warns |
| E | `just text` | correct — control |
| F | `::: {.card}` then `Heading` / `===` | correct — the braced form is immune |
| G | `::: {.card}` then `term` / `: def` | correct — the braced form is immune |
| H | a blank line, then any of the above | correct |
| I | an ATX heading, `# x` | correct |
| J | a pipe table | correct |
| K | a bullet list | correct |

Two properties matter and neither is cosmetic.

**Every failing case warns.** `container has no class and no recognizable
kind name`, with an unclassed `<div>` as output. This is not a silent
failure like the nested-fence trap; it is discoverable from the CLI.

**The setext cases are worse than the definition-list case.** The kind word
does not merely land in the wrong element — it lands in the heading's text,
and therefore in the generated anchor id. A permalink or a `§` cross-reference
can be minted from the mistake.

### 3.2 Why the house style meets it head-on

Two workarounds exist and both are verified: a blank line after the fence
line, or the braced form. Neither is what this repository writes. Every
brace-free container in `docs/authoring.md`, in both existing specs and in
the README puts content on the line immediately after the fence:

```markdown
::: callout
This renders as a styled callout box.
:::
```

A rule of the form "leave a blank line here, but only for these two kinds"
would make `defs` and `stats` behave unlike the other five, in a codebase
whose own documentation models the tight form everywhere.

## 4. The silent case: a title on a braced fence

The trailing text after a *successful* attribute parse is absorbed too, and
this one does not warn.

| Input | Today | Wanted |
|---|---|---|
| `::: {.card} Why this matters` | title becomes body text | title |
| `::: {.aside} Why this matters` | title becomes body text, and the summary reads the generic fallback `Aside` | summary reads `Why this matters` |

Here the reader *is* advanced past `{...}`, leaving ` Why this matters` on
the line to merge into the first paragraph. The documented restriction that
the braced form "never gets a title" is the consequence, not the intent —
and for a collapsible kind the author's title is discarded in favour of a
fallback label with nothing said about it.

> [!WARNING]
> This is a genuine silent failure, and the only one in this document. The
> four cases in §3 all warn.

## 5. The fix

### 5.1 The parser

Copy the extension into `internal/fences/` — five files, 388 lines, MIT,
with the dual copyright and a provenance header naming upstream, `v1.0.0`
and the modification. The external dependency is dropped.

`Open` changes in exactly one way: after the fence characters it captures
the entire remainder of the line and advances the reader past it, so nothing
from the fence line ever reaches the content stream. It no longer calls
`parser.ParseAttributes` itself; it hands the raw remainder to md2html so
that one code path sees every spelling.

Two existing guards are preserved verbatim. A bare `:::` with nothing after
it still returns `NoChildren` — that ambiguity is what lets `:::` close a
container rather than open one — and the random `data-fence` id that drives
nested fences is untouched.

::: aside Why the parser and not the source
A source-level rewrite of `::: kind` into `::: {.kind}` before parsing was
considered and rejected. It would have to track code fence state in raw
source to avoid rewriting the literal `::: warning` examples that appear
inside fenced blocks in `docs/authoring.md` and in both specs — which is the
exact hazard behind the nested-fence trap. A block parser gets that for
free: a `:::` line inside an open code fence is consumed as raw text, and no
block parser is ever consulted for it.
:::

### 5.2 One fence-info parse

A single function reads the captured remainder and yields kind, title and
attributes, covering all five spellings in one place:

| Input | Kind | Title | Attributes |
|---|---|---|---|
| `{#id .card .compact}` | `card` | — | id, classes |
| `card` | `card` | — | — |
| `card Why this matters` | `card` | `Why this matters` | — |
| `card[Why]{#w .compact}` | `card` | `Why` | id, classes |
| `{.card} Why` | `card` | `Why` | classes |
| `card {#w .compact}` | `card` | — | id, classes |
| `card Why {#w}` | `card` | `Why` | id |

The last two rows are new syntax rather than a repair. Today the kind
outside the braces puts the attribute block in the title — `::: aside {#id}`
renders `<summary>{#id}</summary>` — so nothing can depend on the current
behaviour, and the undelimited form gains the shape the label form already
has. An attribute block is then trailing everywhere in this dialect, which
is how the label form and fenced code captions already read one.

The braced form's first class keeps selecting the kind. That rule is the
bridge which makes a document written for Pandoc's `fenced_divs` pick up
md2html's styling rather than an unclassed div, and it cannot be retired
without changing what three shipped kinds do: `{.warning}` would fall from
`<div class="callout callout-warning">` to `<div class="warning">`, and
`{.aside}` and `{.example}` would stop being collapsible.

Attributes go through the shared `{#id .class key=value}` parser added for
the standards alignment work — the same one bracketed spans and fenced code
attributes use. Unifying on it is what that parser was for.

`firstWord`, `detachTitle` and all first-paragraph mining are deleted. Both
existing warnings keep their current wording.

### 5.3 Titles as a parsed node

The `fig` renderer converts inline strings by calling `md.Convert` a second
time and stripping the `<p>` wrapper. Titles should not reuse that. Since
the parser now owns the fence line, the title becomes its own single-line
AST block whose inlines goldmark parses natively: no second conversion, and
no open paragraph for a setext underline or a `:` marker to attach to. The
block closes after one line, so the following line always starts fresh.

The renderer emits the existing `<p class="container-title">`. For the
collapsible kinds the transform promotes that node's children into
`<summary>`, exactly as `toDetails` does today. A title becomes available on
every form, which is what unblocks the `stats` and `defs` promotion.

### 5.4 What this deletes

- `detachTitle`, and its thirty lines of reasoning about which newline among
  a paragraph's direct children marks the end of the fence line.
- `firstWord`, and the first-paragraph mining it serves.
- The load-bearing ordering of Containers before Chips. `:::aside[Why]{.compact}`
  collides with a bracketed span today only because `aside[Why]{.compact}`
  sits in the content stream, where `[Why]{.compact}` is a perfectly good
  span. Once the fence line never enters the tree, the span transform cannot
  see it, and the ordering stops carrying weight.

That last item resolves one of the four divergences recorded in the
standards alignment spec: the note explaining why the ordering is
load-bearing for two reasons becomes a note on why it is load-bearing for
neither.

## 5.5 Two things the rewrite makes possible

Neither is fence-line capture, and both were found while planning it. They
are recorded here because they land in the same change.

**`nav` becomes a shipped kind.** The vendored renderer emits `<nav>` when a
container's class matches `elem-nav` — upstream behaviour, undocumented
here, used nowhere. It is not merely surplus: `fenceDivs` collects only
`<div>`, so `::: {.aside .elem-nav}` skips the container transform entirely,
losing the `<details>` and the summary and leaking the internal `data-fence`
attribute into the output. Replacing the magic class with an ordinary kind
fixes that by construction, because the renderer then always emits a `<div>`
and the element is changed afterwards, by code that has already applied the
kind and cleaned up.

**A container can carry an accessible name.** `aria-label` appears in
neither goldmark's global attribute list nor the `data-` prefix its renderer
exempts, so it is dropped silently today. A landmark that cannot be named is
noise in a screen reader's landmark list rather than help — and since
`[[toc]]` already emits `<nav class="toc">`, a hand-written nav is usually
the second on the page, which is precisely when a name stops being optional.
The container renderer passes the `aria-` prefix too.

> [!WARNING]
> Attribute *names* reach the output unescaped — goldmark writes them
> verbatim and escapes only values. `attrTokens` accepts any byte in a key
> except whitespace and `=`, so `data-x"onmouseover="alert(1)` parses as one
> key. goldmark's own `ParseAttributes` rejects such a block today, which is
> the only reason it is not already reachable; moving container attributes
> onto this package's more permissive parser removes that accident. Names
> must therefore be validated where author text becomes attributes, before
> either prefix is honoured.

## 6. Test plan

Test-driven, and the three existing suites are the behaviour-preservation
proof: every input that works today must produce identical output.

New coverage:

- The four cases of §3.1 — definition list, setext `===`, setext `---`, and
  the label form against a definition list.
- The two cases of §4, including that a collapsible kind's summary carries
  the author's title rather than the fallback.
- A bare `:::` still closes a container rather than opening one.
- Nested containers still nest, at depth.
- `:::` inside a fenced code block is still literal text.
- Every currently documented spelling in the §5.2 table.

Two existing tests are expected to change meaning rather than be worked
around: the transform-ordering guards, which protect a constraint this
change removes. Tests that assert `detachTitle` or `firstWord` behaviour go
with the functions.

## 7. What we keep

- **The `::: kind` form itself.** It is the ergonomic spelling and the one
  the docs teach. This makes it real rather than recovered.
- **Both warnings, verbatim.** An unknown kind still warns and still emits a
  correctly classed div; a fence with neither class nor recognizable kind
  still warns.
- **`data-fence` and the nesting machinery.** Untouched. The change is
  confined to how one line is read.
- **The nested-fence trap.** Unrelated to this, and still a silent failure.
  It stays documented.
