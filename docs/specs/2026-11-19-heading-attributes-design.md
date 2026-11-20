# Heading Attributes — Design

**Date:** 2026-11-19
**Author:** AdamF-G
**Status:** Built, 2026-11-19.

## Purpose

A heading's trailing `{…}` is read by goldmark (`parser.WithAttribute()`,
`md2html.go`) while it parses the heading line, with a grammar of its own.
Every other attribute block in md2html, on a span, a link, a container or
a code fence, is read by md2html's parser (`attrs.go`). Two problems follow:

- **A span or a link at the end of a heading loses its block to the
  heading.** `## Rollback [beta]{.chip tip="…"}` renders
  `<h2 class="chip">Rollback [beta]`: the chip is literal, its tip is gone,
  the slug falls back to `section-N`, and nothing warns. The same happens to
  `## See [docs](d.md){.x}` and to an underlined (setext) heading. A chip
  with a hover note at the end of a heading is the case chips exist for,
  and this release adds both the note and the tip.
- **A heading's block has its own silent traps.** A single-quoted value or
  a bare key leaves the whole block, id included, in the heading text
  (silent trap 17), an unquoted number becomes an empty value, and `aria-`
  names are dropped, where a span and a link keep them.

This note gives headings md2html's parser, so every attribute block reads
one grammar under one set of rules, and gives a block that touches a span
or a link back to it.

## 1. The rule

A heading's last line ends in a `{…}` block: the first unescaped `{`
whose block, closed by `braceSpan` (`attrs.go`), runs to the end of the
line (my call). Quotes count only inside the block, as goldmark counts
them today. `braceBlock`, which containers and code fences use, counts a
quote anywhere on the line, so `## Don't panic {#calm}` would lose its id;
it is not used here. Spans find their blocks with `chipRe`, which opens a
quote only after `=`. Then:

- **The block touches a `]` or a `)`**, with no space between: it is not
  the heading's. It stays in the heading's text, where Chips or LinkAttrs
  reads it as the span's or the link's, with their own warnings. This is
  how `## Rollback [beta]{.chip}` becomes a chip.
- **Such a block that nothing takes** (`## Call f(x){#call}`, `## Lit\]{#x}`,
  or a footnote reference before it) stays
  as text, as written, and warns, saying to write a space before the brace
  for the heading's block (my call). A check after Chips and LinkAttrs
  finds a heading whose text still ends in a `{…}` that `parseAttrs` reads
  as attributes.
- **Anything else** (a space, or any other character before the `{`): it
  is the heading's block, read by `parseAttrs` and checked by the same
  rules a span's block is (§2).

Both kinds of heading take this: `#` headings and setext headings. A block
that is not the last thing on the line is text, as today.

## 2. What a heading's block keeps

The same as a bracketed span, so the two read alike. The span's checks in
`spanBlock` (`chip.go`) are factored out, and both callers use them.

- `#id` and `.class`, and `id=`/`class=` in any case, the last id winning
  in the order written.
- Lowercased names that are goldmark's global attributes, or start with
  `data-` or `aria-`. Any other safe name, an event handler most obviously,
  is dropped and the rest of the block applies.
- Values may be double-quoted, single-quoted or bare: `{title='x'}` and
  `{lang=fr}` now work. A value is a string; goldmark read some as JSON
  (`{n=3}`, `{a=[1,2]}`), and a string reaches the HTML the same way.
- `tip` and `tip-delay` are not taken: a heading is not an inline host. They
  are dropped as any unlisted name is.

### When the block stays as text

As for a span, the block then stays in the heading's text, as written:

| Block | Stays as text | Warns |
|---|---|---|
| `{}` | yes | no |
| bare words alone, `## GET /v1/jobs/{id}` | yes | no |
| a bare word beside an id, a class or a `key=value`, `{#s data-flag}` | yes | yes |
| a name no attribute can have, `{#s a/b=1}` | yes | yes |
| a quoted value with no closing quote | yes | yes |
| touching `]` or `)`, and nothing before it takes it | yes | yes |

The second row matters: API headings write paths in braces, and they must
keep rendering as written.

The warnings name the heading, not a span: `heading "Setup": …`. Each has
an entry in `docs/guide/warnings.md`, raised as `warn(fmt.Sprintf(...))`
where it is built, so `TestEveryWarningIsInTheGuide` sees it.

## 3. Where it happens

At the same point goldmark reads the block today: after the heading's line
is parsed as a block and before its text is parsed inline. So the block
never reaches the heading's text, whatever Markdown is inside it, and the
page title, read from the first `<h1>` before any transform, never sees
it.

A closing `#` sequence is trimmed again once the block comes off, so
`## Title ## {#id}` renders "Title" as today (my call): goldmark trims the
closing hashes before the block, and with the block last they are still
on the line.

### The page title

The title is read from the first `<h1>` before any transform, and
`stripChipTokens` already drops a span's block from it. A heading that now
hands its block to a link, `# See [docs](d.md){.x}`, would otherwise title
the page "See docs{.x}". The title also drops a link's block, which mends
the same leak mid-heading, where it was already broken (my call).

### The parsers

goldmark's two heading parsers are wrapped, not rewritten. md2html builds
its goldmark parser with the default block parsers, the ATX and setext
heading parsers each replaced by a wrapper at the same priority (600 and
100), and without `parser.WithAttribute()`. Each wrapper calls goldmark's
parser and then, when the heading closes, reads, sets and trims its last
line as §1 and §2 say. `figrender.go` already builds a goldmark parser
from an explicit list.

### `aria-` names

goldmark's renderer filters a heading's attributes through its global
list, which admits `data-` but not `aria-`. md2html renders headings
itself, as it already renders fenced code, so an `aria-` name reaches the
`<h1>`…`<h6>`. It does not change goldmark's package-level filters: other
code in the same process may use goldmark.

## 4. Compatibility

**Breaking.** These change what an existing page renders:

1. **A heading ending `]{…}` or `){…}`.** The block now belongs to the span
   or link before it. The guide's "`## See [docs](./docs.md){.ext}` puts
   the class on the `<h2>`" is reversed. Write a space before the brace to
   keep the block on the heading: `## See [docs](./docs.md) {.ext}`.
2. **A single-quoted or an unquoted value** now applies. Before, the whole
   block, id included, stayed in the heading text.
3. **An unquoted number** is now its value. Before, `{data-step=1}` gave an
   empty value.
4. **An empty `{}`** at the end of a heading now stays as text. Before, it
   was removed.
5. **`aria-` names** now reach the heading. Before, they were dropped.
6. **A bare word or a bad name beside an id or a class** still stays as
   text, but now warns, and so does a block touching `]` or `)` that no
   span or link takes, such as `## Call f(x){#call}`, which used to set the
   heading's id. (`## Array[0]{#a}` is a bracketed span, so its block is
   the span's; a code span before the block leaves a backtick before the
   brace, so the block is the heading's.)
7. **A page title** drops a link's block, as it already dropped a span's:
   `# See [docs](d.md) and [x](y.md){.k} more` was titled
   "See docs and x{.k} more".

A plain `{#id .class}`, with a space before it, is unchanged. That is 83 of
the 87 heading blocks in the author's local Markdown, on 2026-11-19. The
other four are paths and code (`/v1/jobs/{id}`, `{fbx,tex,meta}`, a
Python dict), which stay as text before and after. None ends `]{` or `){`.

### The release notes

The changelog marks this **breaking** and says what to check in existing
docs, with a command for each case:

```sh
# 1. a heading ending in a span's or a link's block, now the span's or link's
grep -rnE '^#{1,6} .*[])]\{[^}]*\}[[:space:]]*$' --include='*.md' .
# 2-3. a heading block with a quote or an unquoted value
grep -rnE "^#{1,6} .*\{[^}]*('|=[^\"[:space:]}])[^}]*\}[[:space:]]*$" --include='*.md' .
# 4. a heading ending in an empty {}
grep -rnE '^#{1,6} .*\{\}[[:space:]]*$' --include='*.md' .
```

Setext headings are rare and not searched; the notes say so.

## 5. Docs

- `page-structure.md`, "Headings and anchors": the block's grammar is the
  span's; the `]`/`)` rule and the space that keeps a block on the heading;
  the API-path case.
- `silent-traps.md`: trap 17 (a heading's block needs double quotes) goes,
  and the list is renumbered. Trap 18 (where a block works) stays.
- `TestGuideClaims` cases for the chip at the end of a heading, a
  single-quoted value, and the API path.
- The compat suite's heading case stays, and the suite gains one for a
  single-quoted value if Pandoc agrees with it; if Pandoc does not, the
  case is recorded where the suite records divergences.

## Approaches

- **md2html's parser for headings, in wrapped heading parsers (chosen).**
  One grammar and one set of rules for every attribute block, and the fix
  for spans and links at the end of a heading falls out of it. The cost is
  the compatibility changes in §4 and building the goldmark parser from an
  explicit list.
- **Hand the block back after goldmark parses it.** A step after goldmark's
  heading parse returns a block that touches `]` or `)` to the heading's
  text. It fixes only the original bug and leaves headings on their own
  grammar, silent trap 17 included.
- **Rewrite heading lines before goldmark sees them.** Depends on what
  goldmark does with an injected empty block; fragile around code fences
  and front matter.

## Decisions

- md2html's parser reads headings' blocks, with the span's rules.
- A block touching `]` or `)` belongs to the span or link before it; links
  are included.
- `aria-` names reach headings, without changing goldmark's globals.
- The changelog marks it breaking and gives the search commands.

## Left to the builder

- How the span's checks are factored out of `spanBlock`, and the warning
  wording.
- Whether the heading renderer is a node renderer for `ast.Heading` or
  another way of admitting `aria-` names, as long as goldmark's globals
  stay untouched.
- Where the goldmark parser is built, as long as `figrender.go`'s own
  parser is unaffected.
