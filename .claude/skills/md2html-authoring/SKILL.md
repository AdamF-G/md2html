---
name: md2html-authoring
description: Use when writing or editing Markdown that will be converted to HTML by md2html, including docs in this repo's docs/ tree. Also use when a page needs a callout, admonition, diagram, table, footnote, definition list, or a link to another document, or when deciding between Markdown and hand-written HTML for a page.
---

# Authoring Markdown for md2html

## Overview

`md2html` converts a Markdown tree to HTML. Several of its features are not
discoverable from the README, and two of them fail silently — you get valid
output that quietly lacks what you asked for, with no warning and a zero exit.

**Read the full reference before writing the page.** It is `authoring.md`,
sitting beside this file in an installed skill, and `docs/authoring.md` in
the md2html repo itself. Every claim in it is verified against the binary.

## The remaining silent failures

**A fence inside a fence needs more backticks on the outside.**

````markdown
```markdown        <- ends at the FIRST ``` in its body, not the one you meant
```
````

Showing three-backtick source requires a four-backtick fence around it.
Nothing warns; the page renders as *something*, with your example split in
half and the next block inheriting the wrong language.

**A container kind outside the shipped set has no styling.**

```markdown
::: callout        -> <div class="callout">
::: {.callout}     -> <div class="callout">          (same thing)
::: house-style    -> <div>, and the run warns
```

The shipped kinds are `callout`, `warning`, `card`, `stats`, `defs`,
`group`, `details`, `aside`, `example` and `nav`. `stats`, `defs` and
`group` are the `fig` kinds as block containers, and the form to use
unless the tiles, terms or panel are part of a diagram; `stats` and `defs` hold a
definition list (in `stats`, term = value, first `: ` = label, second
`: ` = detail) and warn without one. `details`, `aside` and `example` are collapsible
`<details>` — `details` uses its title as the `<summary>` verbatim, the other
two add wording of their own; `nav` is a `<nav>`
landmark and the one kind that adds no class of its own — name it with
`aria-label` (`::: nav {aria-label="Section"}`) whenever it is not the only
`<nav>` on the page, which it usually isn't: `[[toc]]` already emits its own
`<nav class="toc">`. A braced class outside the shipped set still emits a
correctly classed but unstyled div, silently — that is deliberate, since the
author supplies the CSS. An unknown *kind word* — bare, labelled or
kind-outside-braces — still consumes the word and warns, and still renders
any title it found as a plain, unstyled paragraph.

Every container form carries a title now: the bare form
(`::: aside Why this matters`), the braced form with a trailing title
(`::: {.aside} Why this matters`), the kind-outside-braces form with a
trailing attribute block (`::: aside Why this matters {#w .compact}`), and
the label form (`:::aside[Why this matters]{#w .compact}`). Reach for the
label form specifically when the title needs to stay unambiguous next to
attributes — every other spelling reads a trailing `{...}` as the attribute
block, so a title that itself ends in a brace-like group gets misread as
attributes rather than words.

A container also passes through a fixed attribute set: `id`, `class`,
goldmark's global attributes, and any `data-` or `aria-` name. Anything
else — an event handler, most obviously — is dropped. The braced
attribute grammar accepts a bare key with no value and a single-quoted
value too — `{.callout data-flag}` and `{.callout data-x='single'}` both
parse, not just `key="value"`. `data-fence` is a reserved *namespace*, not
three literal names: any name **beginning with** `data-fence` is dropped,
no hyphen required at the boundary and no regard for case, so
`data-fence-kind`, `data-Fence-Title` and an unrelated `data-fencepost` are
all silently dropped rather than honored. The `data-`/`aria-` allowlist
ignores case too: `ARIA-label` is honored like `aria-label`.

**A link to `.html` is never rewritten.**

```markdown
[Auth](./api/auth.md)     correct -> href="api/auth.html", fragments kept
[Auth](./api/auth.html)   WRONG   -> left as written, usually a 404
```

Link to the `.md` source. The crawler resolves it and computes the relative
path to the emitted page.

## Choosing between forms

Six constructs accept more than one spelling. None is deprecated, and the
right one depends on **where the file is read**, which is usually not only
through md2html. Ask that before picking.

| Construct | Forms | Choose by |
|---|---|---|
| Warning, note | `> [!WARNING]` / `::: warning` | The alert renders natively on GitHub, Obsidian and Typora and degrades to a readable blockquote elsewhere; `::: warning` shows on GitHub as literal text. In a repository file, use the alert. |
| Container kind | `::: callout` / `::: {.callout}` / `::: callout {#id .class}` | The braced form is Pandoc's `fenced_divs`, so it survives in a document shared with Pandoc, kramdown or MyST — which is also why the braced form's first class token, not some marker of ours, is what selects the kind. The kind-outside-braces form is the clearest of the three to read, but Pandoc will not parse it as a fenced div at all. Neither unbraced form renders on GitHub. |
| Container title | `::: aside Why` / `:::aside[Why]` | Every form now carries a title. Pick the label form when the title needs to stay unambiguous next to an id or classes — it delimits the title instead of reading to end of line, so it is the only spelling a trailing brace-shaped title cannot confuse. |
| Chip, span | `[proven]` / `[proven]{.chip}` | The bare form covers only the six status words; the attribute form is Pandoc's `bracketed_spans` and takes any label or class. Prefer the attribute form in new writing. |
| Code caption | `` ```go caption="x" `` / `` ```{.go caption="x"} `` | GitHub reads the first word as the language and ignores the rest, so the brace-free form still highlights there. The braced form does not highlight on GitHub, but is what Pandoc reads and the only one that can escape a `"` in the caption. |
| Contents list | `[[toc]]` / `[TOC]` | GitLab builds its own list from `[TOC]`, so use that for a document also read there; GitHub builds one from neither. Otherwise choose for the reader of the source. `[[toc]]` is markdown-it and VitePress; `[TOC]` is Python-Markdown, MkDocs, Typora and StackEdit. |
| Title, subtitle | front matter / italic line under the `<h1>` | Front matter is machine-readable and hidden by GitHub, and is the only one that can carry a date. The italic line is visible prose everywhere. |

A form another renderer does not understand should still degrade to
something readable rather than to noise — that is the whole argument for the
alert syntax. Forms mix freely in one document; there is no need to convert a
file wholesale.

## Don't hand-write what you get for free

Headings get ids and anchors. Tables get scroll containers. Off-site links get
`target="_blank" rel="noopener noreferrer"`. Diagrams work as
`` ```mermaid `` fences and are themed automatically.

So reach for raw HTML only when Markdown genuinely cannot express the thing —
not for callouts (`::: {.callout}`), not for diagrams (mermaid fence), and not
for table wrappers.

Assets are linked, never copied: `site/` is self-contained for documents but
not for images.

## What has no shortcut

There is no sidebar or site index, and no cross-document navigation. A
per-page contents list does exist — `[[toc]]` alone on a line —
but heading slugs are still worth knowing: they are stable and keep letters
from any script, so `## 日本語の見出し` yields `#日本語の見出し`.
They follow GitHub's and GitLab's rules, so an in-page link written against
either resolves here: `## Sizes: small × large` is `#sizes-small--large`,
and the second `## Setup` is `#setup-1`.

## Quick reference

| Need | Write |
|---|---|
| Callout | `::: callout` … `:::` |
| Warning | `> [!WARNING]` in a repo file; `::: warning` otherwise |
| Collapsible block | `::: details Title` … `:::` |
| Collapsible aside | `::: aside Title` … `:::` |
| Titled container with classes | `:::aside[Title]{#id .cls}` … `:::` |
| Worked example | `::: example Title` … `:::` |
| Named nav landmark | `::: nav {aria-label="Section"}` … `:::` |
| Stat tiles outside a figure | `::: stats` with `99.9%` / `: uptime` / `: detail` … `:::` |
| Term grid, titled panel | `::: defs` (a definition list) / `::: group[Title]` … `:::` |
| Status marker | `[proven]` for the six status words; `[any label]{.chip}` otherwise |
| Any classed span | `[text]{.cls}` |
| Cross-reference | `§4.2` (resolves to the heading numbered 4.2) |
| Contents list | `[[toc]]` alone on a line (`[TOC]` also accepted) |
| Title/subtitle/date | `---` front matter; an italic line under the H1 shows elsewhere but carries no date |
| Page language | `lang: de` in front matter, or `--lang` for the whole run; default `en` |
| Contents list's name, non-English page | `toc-title: Inhalt` in front matter; default `Table of Contents` |
| Code caption | ` ```go caption="server.go" ` — keeps GitHub highlighting |
| Code caption, Pandoc form | ` ```{.go caption="server.go"} ` — needed to escape a `"` |
| Stable anchor | `## Title {#my-id}` |
| Diagram | `` ```mermaid `` fence |
| Hand-laid-out figure | ` ```fig ` fence, YAML body |
| File or config hierarchy | `tree:` item in a ` ```fig ` fence, one node per line |
| Context above or below a split | `split:` item between other items in a ` ```fig ` fence |
| Cross-document link | `[x](./other.md)` — never `.html` |
| Bare fragment, no page shell (e.g. a Claude Artifact) | `--fragment` |

For anything beyond this table, read `docs/authoring.md`.
