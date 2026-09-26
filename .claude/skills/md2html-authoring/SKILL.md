---
name: md2html-authoring
description: Use when writing or editing Markdown that will be converted to HTML by md2html, including docs in this repo's docs/ tree. Also use when a page needs a callout, admonition, diagram, table, footnote, definition list, or a link to another document, or when deciding between Markdown and hand-written HTML for a page.
---

# Authoring Markdown for md2html

**Run `md2html --guide` and read its output before writing the page.** It
prints the authoring guide for the binary that will convert the page, so it
cannot describe a different version. In the md2html repo the same guide is
`docs/authoring.md`. It opens with a quick reference from need to syntax,
and every claim in it is checked against the binary.

Three mistakes still give valid output, a zero exit and no warning, so check
for them even after a skim:

- **A fence inside a fence needs more backticks on the outside.** Showing
  three-backtick source takes a four-backtick fence around it.
- **Link to the `.md` source, never the `.html`.** An `.html` link is left
  as written and usually 404s.
- **A braced class outside the shipped container kinds is unstyled.**
  `::: {.house-style}` is a plain classed div; only the kind *word* form
  warns.

These and the rest are under Traps in the guide.
