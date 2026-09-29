---
name: md2html-authoring
description: Use when writing or editing Markdown that will be converted to HTML by md2html, including docs in this repo's docs/ tree. Also use when a page needs a callout, admonition, diagram, table, footnote, definition list, or a link to another document, or when deciding between Markdown and hand-written HTML for a page.
---

# Authoring Markdown for md2html

No `md2html` on `PATH`? Get it from
[github.com/AdamF-G/md2html](https://github.com/AdamF-G/md2html):
`go install github.com/AdamF-G/md2html/cmd/md2html@latest` (needs Go 1.26
or later).

**Run `md2html --guide` and read its output before writing the page.** It
prints the authoring guide's quick reference and silent traps for the
binary that will convert the page, so it cannot describe a different
version, and ends with an index of the guide's other sections. Before using
a construct beyond a plain line in the quick reference, such as a
container, a `fig` diagram or a cross-reference, print its section:
`md2html --guide callouts-and-containers`, `md2html --guide diagrams`. If
the conversion prints warnings, `md2html --guide warnings` says what each
one means and how to fix it. In the md2html repo the whole guide is
`docs/authoring.md`.

Three of those traps come up most. Each gives valid output and a zero exit,
with no warning:

- **A fence inside a fence needs a longer outer fence.** Showing
  three-backtick source takes four backticks around it; showing a `:::`
  container inside a container takes `::::` on the outer one.
- **Link to the `.md` source, never the `.html`.** An `.html` link is never
  matched to the page md2html emits, and when the file exists nothing
  warns.
- **Only the first class names a container's kind.** `::: {.compact
  .warning}` is a plain, unstyled div; write `::: warning {.compact}`.
