# Span Labels — Design

**Date:** 2026-11-19
**Author:** AdamF-G
**Status:** Approved (option B).

## Purpose

A bracketed span was matched by a regular expression over the page's text
after goldmark had rendered it. So a label holding Markdown,
`[need *a*]{.chip}`, was text around an `<em>` the pattern could not match,
and the span stayed literal with no word; and a label could not hold a `]`
at all, neither balanced, `[a [b] c]{.x}`, nor escaped, `[a \] b]{.x}`,
since goldmark had already dropped the backslash.

This parses a bracketed span the way Pandoc does: a label is inline
Markdown, brackets nest, and `\]` is a bracket.

## Design

- **Pairing on goldmark's tree.** goldmark leaves an unmatched `[`…`]` as
  text nodes that still point at the source, and `rawBlockParser` leaves a
  block straight after a `]` as its own node. A goldmark AST transformer,
  run after inline parsing, finds each such block, walks back through its
  siblings for the `[` that opens the `]` before it, counting nested
  brackets and skipping escaped ones by their source bytes, and moves the
  nodes between into a span node. A link or image, and code, are atomic:
  their insides are not counted, and a span is never made inside a link's
  text, as the guide promises for badges.
- **Chips still decides.** The span node renders as
  `<span md2html-span="{…}">label</span>`. Chips reads the attribute
  through `checkAttrBlock`, as it read the block before. A block it takes
  becomes the span's attributes, chip modifier and tip included. A block
  it refuses unwraps the span back to the literal `[label]{…}`, with the
  same warnings.
- **The regex keeps only the bare badges.** `chipRe` loses its
  bracketed-span alternatives. `[proven]` and `[c:…]` are unchanged. Left
  in, the regex would make a span of text the tree had already ruled out:
  `[x\]{.c}` renders as `[x]{.c}`.
- **The block's end** is found as `chipRe` found it: a quote opens a value
  only after `=`, and a block whose quote never closes ends at its first
  `}`. So `title=don't` keeps its span, and an unclosed quote still warns.
- **The page title** is read from the tree after goldmark, so its text
  walker drops a chip span and its block, and writes a refused one as the
  literal text the page shows.
- **Fig labels** get the transformer too.

## Compatibility

`[**x**]{.chip}`, documented as staying literal, is now a chip with bold
text, and `[a [b] c]{.x}` and `[a \] b]{.x}` are spans. The guide's "the
label must be plain text" goes. The changelog says so.

## Approaches

- **Pair on goldmark's tree (chosen).** Source positions and backslashes
  are still there, Markdown in the label is already parsed, and Chips keeps
  every rule it had.
- **Warn, and keep labels plain.** A label with markup would warn, and `\]`
  would need the raw source anyway; labels would never take Markdown.
