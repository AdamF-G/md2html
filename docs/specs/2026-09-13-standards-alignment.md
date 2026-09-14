---
title: Standards alignment
subtitle: what our Markdown extensions share with prior art, and what to change
date: 2026-09-13
---

# Standards alignment

[[toc]]

## 1. Summary

md2html's extended syntax was designed without a survey of prior art. This
document records what that survey found, what a measured comparison against
`pandoc` 3.11 says, and five changes that close the gap.

The headline result is better than expected: most of our surface already
matches Pandoc exactly, and the divergences are concentrated in four
constructs. Two of those divergences are worse than "non-standard" — we
actively mangle input written in the standard form.

::: callout
**The measured claim.** For the constructs both tools implement, md2html
agrees with `pandoc -f commonmark_x` on every one except task lists.
That sentence is the most legible thing we can say about our dialect, and
§5 proposes a test suite to keep it true.
:::

## 2. What the comparison measured

A corpus of 21 single-construct documents was run through `pandoc 3.11` and
`md2html v0.2.0`. Three Pandoc readers were compared: `markdown` (Pandoc's
own dialect), `gfm`, and `commonmark_x` (CommonMark plus Pandoc's
extensions).

`commonmark_x` is the closest match and the one we should cite. Every
extension we care about — `fenced_divs`, `bracketed_spans`,
`fenced_code_attributes`, `header_attributes`, `definition_lists`,
`footnotes`, `pipe_tables`, `yaml_metadata_block` — is on by default in it.
No `+flag` chain is needed.

### 2.1 Where we already agree

| Construct | Pandoc `commonmark_x` | md2html | |
|---|---|---|---|
| `::: callout` | `<div class="callout">` | identical | [proven] |
| `::: {.callout}` | `<div class="callout">` | identical | [proven] |
| `::: {#note .callout .compact}` | `<div id="note" class="callout compact">` | identical | [proven] |
| `## T {#id .lead}` | `<h2 class="lead" id="id">` | same, plus our anchor | [proven] |
| Definition lists | `<dl><dt><dd>` | identical structure | [proven] |
| Pipe tables | `<table>` | same, plus `.table-scroll` wrapper | [proven] |
| Footnotes | `<sup><a>` + endnote list | same semantics, different ids | [proven] |
| YAML front matter | consumed | consumed, plus subtitle/date | [proven] |

Heading slugs match too: `commonmark_x` slugs `## 4.2 Rollback` to
`42-rollback`, exactly as we do. Pandoc's `markdown` reader gives
`rollback` instead — a reason to cite `commonmark_x` and not `markdown`.

### 2.2 Where our extensions degrade cleanly

Pandoc passes these through without corrupting them. This is the outcome we
want for anything we invent.

| Construct | Pandoc output | |
|---|---|---|
| `[proven]` | literal text | [proven] |
| `[[toc]]` | literal text | [proven] |
| `§4.2` | literal text | [proven] |
| ` ```fig ` | `<pre class="fig"><code>` | [proven] |
| ` ```mermaid ` | `<pre class="mermaid"><code>` | [proven] |

### 2.3 Where we are wrong

Four constructs. Two of them are live defects, not stylistic divergence.

::: warning
**`[proven]{.chip}` renders visibly broken today.** Pandoc's
`bracketed_spans` produces `<span class="chip">proven</span>`. We produce
`<span class="chip chip-proven">proven</span>{.chip}` — the chip transform
fires on the bracket and the attribute block is left on the page as
literal text. A non-vocabulary word fares worse: `[needs review]{.chip}`
is emitted entirely literally, braces included.
:::

::: warning
**` ```{.go caption="x"} ` is mangled.** The braced form is Pandoc's
standard `fenced_code_attributes` syntax, and it is what a document
written for any other tool will use. We emit
`class="language-{.go"` and a caption of `server.go"}`.
:::

The other two are gaps rather than corruption:

| Construct | Pandoc | md2html | Note |
|---|---|---|---|
| `::: aside Title` | **not a div at all** — literal paragraph | `<details>` + `<summary>` | Our title form is unportable |
| `> [!WARNING]` | `<div class="warning">` | blockquote with literal `[!WARNING]` | GitHub renders this natively; we do not |

The `::: aside Title` result is worth dwelling on. Pandoc does not merely
ignore the title — the trailing words make the whole fence unrecognisable,
so the colons appear on the page. Our most-used titled container is the
least portable thing we emit.

### 2.4 A correction to an earlier assumption

`:::aside[Why this matters]` — the label form from the CommonMark generic
directives proposal — is **not** Pandoc-compatible. Pandoc implements
neither the proposal nor the label syntax, and renders it as a literal
paragraph exactly like our current form. Measured, not assumed.

Adopting `[label]` therefore buys alignment with the *directive* family
(remark-directive, Docusaurus, MDX), not with Pandoc. It is still worth
doing, but for a structural reason rather than a portability one — see
§3.3.

## 3. Proposed changes

All five are additive. Nothing written today changes meaning.

### 3.1 Accept `[text]{.chip}` and stop mangling attribute blocks [proven]

Adopt Pandoc's `bracketed_spans` as the general form. `[proven]` stays as
the shorthand.

```markdown
[proven]                    ->  <span class="chip chip-proven">proven</span>
[proven]{.chip}             ->  <span class="chip chip-proven">proven</span>
[needs review]{.chip}       ->  <span class="chip">needs review</span>
[shipped]{.chip .chip-ok}   ->  <span class="chip chip-ok">shipped</span>
```

This retires `[c:…]`, which exists only because the bare form has a closed
vocabulary. Keep parsing `[c:…]` for compatibility; drop it from the docs.

Two things it fixes beyond conformance: the literal-`{.chip}` defect in
§2.3, and the latent CommonMark conflict where `[proven]` is a shortcut
reference link if a document happens to define `[proven]: <url>`.

`chip.go` currently matches a fixed alternation in `chipRe`. The change is
a second pattern for `\[([^\]\n]+)\]\{([^}\n]+)\}` plus an attribute
parser, which §3.2 needs anyway.

### 3.2 Accept braced code fence attributes [proven]

````markdown
```go caption="server.go"        # current, keep
```{.go caption="server.go"}     # Pandoc form, add
````

Both must yield `class="language-go"` and a `<figcaption>`. This shares one
attribute parser with §3.1 and §3.3, which also fixes the documented hole
where `caption="has \"quote\""` cannot express an embedded quote.

::: aside Why a shared parser matters
Three constructs currently do three different ad-hoc jobs on the same
`{#id .class key=value}` grammar — headings via goldmark's
`parser.WithAttribute`, containers via the fences library, code fences via
a bespoke token strip. Only the first is a real parser. Consolidating is
most of the work in this document, and the rest is vocabulary.
:::

### 3.3 Accept `:::kind[Title]` [proven]

```markdown
::: aside Why this matters      # current, keep
:::aside[Why this matters]      # add
:::aside[Why this matters]{.compact}   # add — currently impossible
```

The gain is structural, not portability (§2.4). `authoring.md` documents
that the braced form "structurally cannot" carry a title because an
undelimited trailing string is indistinguishable from body text. A
delimited label removes that constraint, so a titled container can also
carry an id and classes — which today it cannot.

### 3.4 Accept GitHub alert syntax [proven]

```markdown
> [!WARNING]
> Overwrites state.
```

should produce the same `<div class="callout callout-warning">` that
`::: warning` produces.

This is the highest-value change in the document and the one that is not
about Pandoc. Our input is existing Markdown in a repository, and those
files are read on GitHub far more often than through md2html. `> [!NOTE]`
renders natively on GitHub, in Obsidian, and in Typora, degrades to a plain
blockquote everywhere else, and — as measured — Pandoc's `gfm` and
`commonmark_x` readers both normalise it to a classed div. It is the only
callout syntax on which GitHub, Pandoc and md2html can all agree.

`::: warning` renders on GitHub as the literal text `::: warning`.

Mapping: `NOTE` and `TIP` to `callout`, `IMPORTANT` to `callout`,
`WARNING` and `CAUTION` to `callout-warning`. `github.com/yuin/goldmark-alert`
(v1.0.1, by goldmark's own author) implements the parse; the alternative is
a transform over `<blockquote>` in our existing pipeline, which adds no
dependency.

### 3.5 Accept `[TOC]` [proven]

One line. `[[toc]]` is markdown-it/VitePress convention; `[TOC]` is
Python-Markdown, MkDocs, Typora and StackEdit. Accept both, document
`[[toc]]`.

## 4. Splitting the `fig` vocabulary [planned]

The `fig` fence mixes two kinds of thing, and they want opposite treatment.

This section was blocked on the definition-list and setext traps described
in [docs/specs/2026-09-13-fence-line-capture.md](./2026-09-13-fence-line-capture.md):
`stats` and `defs` are defined to hold a definition list, which is one of
the two constructs that used to steal a brace-free container's kind word.
That fix has landed, so the promotion below is unblocked.

::: card
**Content** wants composability. `stats` and `defs` hold prose with a
fixed shape and no layout relationships. Today their text is *inline*
Markdown only — no lists, no code fences, no nested callouts, no second
paragraph. As containers they would get full block Markdown for free.

**Layout** wants declaration. `layout: split` with a `boundary`, `weight`
on columns, `lanes`, `chain` — these are relationships *between* items,
and Markdown has no way to express them. sphinx-design encodes the same
thing as `::::{grid} 1 2 3 4` with `:gutter: 3 3 4 5`, which is not an
improvement on YAML.
:::

### 4.1 The split

| Kind | Disposition |
|---|---|
| `stats` | promote to `::: stats` container |
| `defs` | promote to `::: defs` container |
| `group` | container in a YAML costume; promote, keep the `fig` form |
| `box`, `arrow`, `result`, `rail` | stay — diagram primitives |
| `chain`, `lanes`, `split`, `weight` | stay — layout relationships |

Promotion is additive: the `fig` kinds keep working. A `stats` block that
does not sit inside a diagram gains a form where each label is real
Markdown.

Panel metadata resolved onto `group` rather than onto the layout primitive
— see
[the fig vocabulary extensions design](./2026-09-14-fig-vocabulary-extensions-design.md)
§1. `lanes` therefore keeps its shipped `[][]figItem` shape.

### 4.2 What not to do

Do not make diagram items composable. A `box` containing a bulleted list
is not a box. Inline-only Markdown inside genuine diagram primitives is a
constraint worth keeping, and the current `fig` is right about it.

Do not chase sphinx-design. It is a Sphinx component library — grids,
cards, dropdowns, tab-sets, badges — reached through MyST directives and
Sphinx's build. The overlap is `card`/`grid` against our `group`/`cols`,
and its layout syntax is the part we should not copy.

## 5. A compatibility test suite

A characterisation suite, not a conformance suite. [proven]

**Assert structure, never bytes.** Pandoc emits no heading anchors, no
table scroll wrapper, different footnote ids and different task-list
markup. A byte diff would be red permanently and disabled within a month.
Assert tag name and class set per construct.

**Three buckets, and the buckets are the deliverable.**

| Bucket | Assertion | Population today |
|---|---|---|
| Agree | same tag and classes from both | §2.1 — eight constructs |
| Pandoc-inert | passes through unmangled | §2.2 — five constructs |
| Ours only | degrades to a code block or literal | `fig`, mermaid |

**Where it lives.** Pandoc becomes a test dependency, which cuts against
"one static binary, no runtime". Handle it exactly as `e2e/` handles
chromedp: its own module, its own `just` target, absent from
`go test ./...`. That boundary and its rationale are already documented.

**Sequencing.** Build it after §3, so it locks in the alignment we want
rather than certifying the current state.

The suite's real output is a sentence for the README:

> md2html reads CommonMark plus fenced divs, bracketed spans, fenced code
> attributes, header attributes, definition lists, footnotes, pipe tables
> and YAML front matter — the `commonmark_x` dialect — plus GitHub alerts,
> status chips, `§` cross-references, `[[toc]]` and `fig`.

## 5.1 What implementation changed

Four things came out differently from the design above, all recorded in
the commits that made them.

- **Bracketed spans went general, not chip-only.** `[text]{.lead}` is a
  classed span. Restricting the parser to `.chip` would have meant
  recognising Pandoc's syntax and then refusing most of it.
- **`[[_TOC_]]` is out.** Its underscores are emphasis delimiters, so
  goldmark hands the transform `[[`, `<em>TOC</em>`, `]]` rather than one
  text node. Matching it would mean matching on flattened text, which is
  what the marker guard exists to avoid.
- **The label form collided with bracketed spans.** `:::aside[Why]{.compact}`
  is also a valid bracketed span, and Chips claimed it. Containers already
  runs first, so implementing the label form resolved it. The collision
  itself is gone now: the fence line no longer reaches the tree as text at
  all, since the parser owns it end to end — see
  [docs/specs/2026-09-13-fence-line-capture.md](./2026-09-13-fence-line-capture.md).
- **Alerts are a transform, not a dependency.** `github.com/yuin/goldmark-alert`
  exists and would have worked, but an alert has to land on the shipped
  container vocabulary rather than on markup of its own, and the tree layer
  is already ours.

## 6. What we keep

Deliberate divergences, with reasons.

- **`§4.2` implicit cross-references.** Every standard is explicit-label
  (MyST `{numref}`, pandoc-crossref `@sec:`, rST `:numref:`) because
  implicit matching has exactly the silent failure our docs already warn
  about. No standard offers the ergonomics, `§` degrades to plain text
  perfectly, and our slug matches `commonmark_x`. Keep.
- **Task lists.** `commonmark_x` does not implement them and
  `+task_lists` does not enable them on that reader. We follow GFM here
  and accept one documented divergence.
- **`.table-scroll` wrapper, heading anchors, `rel="noopener noreferrer"`.**
  Additions to otherwise-identical structure. They are the point of the
  tool.
- **`fig`'s layout half.** §4.
