---
name: md2html-authoring
description: Use when writing or editing Markdown that will be converted to HTML by md2html, including docs in this repo's docs/ tree. Also use when a page needs a callout, admonition, diagram, table, footnote, definition list, or a link to another document, or when deciding between Markdown and hand-written HTML for a page.
---

# Authoring Markdown for md2html

## Overview

`md2html` converts a Markdown tree to HTML. Several of its features are not
discoverable from the README, and two of them fail silently — you get valid
output that quietly lacks what you asked for, with no warning and a zero exit.

**Read `docs/authoring.md` in this repo before writing the page.** It is the
full reference, and every claim in it is verified against the binary.

## The two silent failures

Both produce working output with no error, so nothing tells you they happened.

**A container without braces loses its class.**

```markdown
::: {.callout}     correct   -> <div class="callout">
::: warning        WRONG     -> <div> with no class at all
```

Use `.callout` specifically: it is the only container class the default
stylesheet styles. Other names give you a correctly classed but unstyled div.

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

There is no navigation, sidebar, or table-of-contents generation. Write the
contents list yourself with anchor links; heading slugs are stable and keep
letters from any script, so `## 日本語の見出し` yields `#日本語の見出し`.

## Quick reference

| Need | Write |
|---|---|
| Callout | `::: {.callout}` … `:::` |
| Stable anchor | `## Title {#my-id}` |
| Diagram | `` ```mermaid `` fence |
| Cross-document link | `[x](./other.md)` — never `.html` |
| Artifact-shaped output | `--fragment` |

For anything beyond this table, read `docs/authoring.md`.
