# Raw Attribute Blocks — Design

**Date:** 2026-11-19
**Author:** AdamF-G
**Status:** Approved.

## Purpose

A bracketed span's and a link's `{…}` block was read after goldmark had
parsed its inside as inline Markdown. So:

- `[b]{.chip tip="a *b* c"}` stayed as literal text, the `*b*` an `<em>`,
  with no warning;
- `[c]{.chip tip="x <b> y"}` stayed literal and put a real `<b>` in the page,
  running to the end of the paragraph;
- `\"` could not escape a quote in a value, since goldmark had dropped the
  backslash, so a value could not hold both kinds of quote;
- `[l](u){title="a *b*"}` lost its block in the same way.

A heading's block is already taken before inline parsing
([Heading Attributes](2026-11-19-heading-attributes-design.md)). This takes
a span's and a link's the same way.

## Design

- **A goldmark inline parser** triggered by `{` takes a block when the
  source byte straight before it is `]` or `)`: everything to the matching
  `}`, found by `braceSpan` on the line (quotes and `\` escapes count only
  inside the block). It becomes an `ast.String` written raw, so goldmark
  escapes it and parses nothing in it. Chips and LinkAttrs then read the
  characters the author typed. A block with no closing `}` on its line is
  not taken.
- **Values are decoded once,** with `html.UnescapeString`, in
  `checkAttrBlock` (spans and headings) and in LinkAttrs. So `&amp;` still
  means `&`, as it did when goldmark resolved it.
- **The quote workaround goes.** `title="say \"hi\""` works. The guide's,
  the warnings page's and the warning's "the other kind of quote" advice
  goes, and a name holding a `"` hints to escape the quote with `\"`.

## Compatibility

A `{…}` straight after `]` or `)` no longer gets inline Markdown inside it,
anywhere: prose like `f(x){*a*}` shows `{*a*}` where it showed
`{<em>a</em>}`. A brace anywhere else is unchanged. The changelog says so.

## Approaches

- **Take the block raw, before inline parsing (chosen).** It matches the
  heading fix, and the block is exactly what the author typed.
- **Re-join the pieces after goldmark.** It cannot recover a dropped
  backslash, or tell an author's `<b>` from one goldmark made.
- **Parse the whole bracketed span in goldmark.** A much larger change,
  since `[label]` is entangled with goldmark's link parser.
