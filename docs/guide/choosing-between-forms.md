---
nav-order: 11
---
# Choosing Between Forms

Where a construct has several spellings, write the default:

- **Note or warning:** an alert, `> [!NOTE]` or `> [!WARNING]`. Use
  `::: callout Title` or `::: warning Title` only for a title, id or class.
- **Container:** `::: kind Title {#id .class}`. Braced `::: {.kind}`, with
  no title, only for a file Pandoc also reads; `:::kind[Title]{…}` only when
  a title ends in braces.
- **Collapsible:** `::: details`. `aside` for a tangent, `example` for a
  worked example, and the raw `<details class="container">` for a file read
  on GitHub.
- **Code caption:** ```` ```go {caption="x"} ````.
- **Badge:** `[draft]` for the six status words, `[label]{.chip}` for any
  other.
- **Contents list:** `[[toc]]`. `[TOC]` in a file read on GitLab, which
  builds its own list from it; GitHub shows either as literal text. Prefer
  a marker to relying on `--autotoc`, so the page places its own list.
- **Title, subtitle, date:** front matter for subtitle and date, a
  `# Title` heading for the title.
- **Reference:** `§4.2` for numbered headings, `[text](#anchor)` otherwise.
- **Diagram:** mermaid, or `fig` when you need to place things yourself.
  A chart of data is a `vega-lite` fence.
- **Stat tiles, term grid, panel:** the container, unless the item sits in
  a `fig` diagram.

Forms mix freely within a document.
