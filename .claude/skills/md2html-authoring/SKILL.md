---
name: md2html-authoring
description: Use when writing or editing Markdown that will be converted to HTML by md2html, including docs in this repo's docs/ tree. Also use when a page needs a callout, admonition, diagram, table, footnote, definition list, or a link to another document, or when deciding between Markdown and hand-written HTML for a page.
---

# Authoring Markdown for md2html

## Overview

`md2html` converts a Markdown tree to HTML. Several of its features are not
discoverable from the README, and one of them fails silently — you get valid
output that quietly lacks what you asked for, with no warning and a zero exit.

**Read the full reference before writing the page.** It is `authoring.md`,
sitting beside this file in an installed skill, and `docs/authoring.md` in
the md2html repo itself. Every claim in it is verified against the binary.

## The remaining silent failure

**A container kind outside the shipped set has no styling.**

```markdown
::: callout        -> <div class="callout">
::: {.callout}     -> <div class="callout">          (same thing)
::: house-style    -> <div>, and the run warns
```

The shipped kinds are `callout`, `warning`, `card`, `aside` and `example`.
`aside` and `example` are collapsible `<details>`; the brace-free form takes
a title on the fence line (`::: aside Why this matters`). A braced class
outside the set still emits a correctly classed but unstyled div, silently —
that is deliberate, since the author supplies the CSS.

**A link to `.html` is never rewritten.**

```markdown
[Auth](./api/auth.md)     correct -> href="api/auth.html", fragments kept
[Auth](./api/auth.html)   WRONG   -> left as written, usually a 404
```

Link to the `.md` source. The crawler resolves it and computes the relative
path to the emitted page.

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
per-page contents list does exist — `[[toc]]` alone on a line — but heading
slugs are still worth knowing: they are stable and keep letters from any
script, so `## 日本語の見出し` yields `#日本語の見出し`.

## Quick reference

| Need | Write |
|---|---|
| Callout | `::: callout` … `:::` |
| Warning | `::: warning` … `:::` |
| Collapsible aside | `::: aside Title` … `:::` |
| Worked example | `::: example Title` … `:::` |
| Status marker | `[proven]`, or `[c:any label]` |
| Cross-reference | `§4.2` (resolves to the heading numbered 4.2) |
| Contents list | `[[toc]]` alone on a line |
| Title/subtitle/date | `---` front matter, or an italic line under the H1 |
| Code caption | ` ```go caption="server.go" ` |
| Stable anchor | `## Title {#my-id}` |
| Diagram | `` ```mermaid `` fence |
| Hand-laid-out figure | ` ```fig ` fence, YAML body |
| Cross-document link | `[x](./other.md)` — never `.html` |
| Artifact-shaped output | `--fragment` |

For anything beyond this table, read `docs/authoring.md`.
