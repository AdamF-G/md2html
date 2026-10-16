---
nav-order: 5
---
# Callouts and Containers

[[toc]]

## Alerts

Use GitHub's alert syntax for a note or a warning. It renders natively on
GitHub, Obsidian and Typora, and as a blockquote everywhere else.

```markdown
> [!WARNING]
> This overwrites state.
```

`NOTE`, `TIP` and `IMPORTANT` render as a `callout`; `WARNING` and
`CAUTION` as a warning callout. The marker must be upper case and alone on
its line: `[!note]`, `[!HINT]` and `> [!NOTE] Title` stay ordinary
blockquotes. The box shows no "Note" or "Warning" label, as GitHub does, so
do not write text that relies on one. An alert takes no title, id or class;
use a container when you need one.

## Containers

A container is a block between `:::` fences. Write the kind after the
opening fence, then an optional title, then an optional attribute block:

```markdown
::: warning Before you migrate
Back up the database first.
:::

::: aside Why this matters {#why .compact}
Because.
:::
```

The space after `:::` is optional. The opening fence needs a kind or an
attribute block: `:::` alone is literal text, and so is the closing line of
a container that was never opened.

| Kind | Element | Use it for |
|---|---|---|
| `callout` | `<div class="callout">` | A highlighted note that needs a title, id or class |
| `warning` | `<div class="callout callout-warning">` | The same, for a warning |
| `card` | `<div class="card">` | A bordered panel for a self-contained item |
| `details` | `<details class="container">` | Content the reader expands on demand |
| `aside` | `<details class="container aside">` | A tangent, collapsed by default |
| `example` | `<details class="container example">` | A worked example; its summary reads "Example — Title" |
| `stats` | `<div class="stats">` | A row of stat tiles |
| `defs` | `<div class="defs">` | A term/definition grid |
| `group` | `<div class="group">` | A titled panel |
| `nav` | `<nav>` | A navigation landmark; adds no class |
| `header` | `<header>` | A banner landmark, such as a site's top bar; adds no class |
| `footer` | `<footer>` | A contentinfo landmark, such as a site's foot; adds no class |
| `wide` | `<div class="wide">` | A table, code block or card too wide for the text column |

`details` with no title shows "Details"; `aside` with none shows "Aside".

**Other spellings.** Pandoc's braced form, `::: {.warning #id}`, and the
directive form, `:::aside[Title]{#id}`, both work. Use the braced form,
with no title, only in a file that Pandoc also reads; every titled form
breaks there. Use the directive form when a title ends in something shaped
like `{…}`: in any other form a trailing brace group is read as
attributes, so `::: card The {x}` is titled "The".

**Titles.** A title is inline Markdown: code spans, emphasis, links and
badges all work. A badge or link ends in a brace group or brackets, so
put such a title in the directive form, whose brackets may nest:
`:::card[Result [stable]{.chip}]`. A `]` inside a code span or written
`\]` does not close the title.

**Title as a heading.** A title is a paragraph unless the attribute
block asks for a heading level, 2 to 6: `:::card[Result]{title-level=3}`
gives `<h3 class="container-title">`, styled as before but in the page's
outline, its contents list and its heading anchors. It warns on a
collapsible kind, whose title is its `<summary>`, and on a container with
no title.

**Only the first class names the kind.** In the braced form,
`{.warning .compact}` is a warning, but `{.compact .warning}` is a plain
`<div class="compact warning">`, unstyled, with no warning. Put the kind
first, or outside the braces.

**Unknown kinds.** `::: house-style` warns and emits an unclassed `<div>`,
with the title kept as a plain paragraph. `::: {.house-style}` emits a
classed, unstyled `<div>` without a warning, as a hook for a stylesheet you
supply. `::: {#only-an-id}` is an unclassed `<div>`.

**Attributes.** The block takes an id, classes, the global attributes and
any `data-` or `aria-` name, case-insensitively. Anything else, such as an
event handler, is dropped, as is any name starting `data-fence`, which md2html
reserves. A bare key (`{data-flag}`) and a single-quoted value work.
Classes merge with the kind's own.

**Nesting.** Containers nest. An inner `:::` closes the nearest open
container, so a container whose body shows `:::` source, inside a code
block or not, needs a longer outer fence:

````markdown
:::: example Writing a callout
```markdown
::: callout
Text.
:::
```
::::
````

## stats, defs and group

`stats` and `defs` hold a definition list. In `stats` each term is a
tile's value, its first definition the label, and an optional second
definition a detail line. Either one without a definition list warns.

```markdown
::: stats
99.9%
: uptime
: over the last 90 days

4
: regions
:::

::: defs
fence
: a line of colons
:::

::: group Ingest
Parse each file, then validate it.
:::
```

These look the same as the `fig` items of the same names. Use the container
unless the item sits inside a diagram: its entries are ordinary Markdown,
and the source stays readable on GitHub, which shows them as plain lines.

A plain definition list, with no container, is already styled as a list of
terms. Use `::: defs` when you want the terms and definitions laid out as a
grid.

## nav

`::: nav` makes a `<nav>` landmark. Name it with `aria-label`, since the
contents list is already a `<nav>` and screen readers need to tell them
apart. Mark the current page on its link:

```markdown
::: nav {aria-label="Section contents"}
- [One](./one.md){aria-current=page}
- [Two](./two.md)
:::
```

## header and footer

`::: header` and `::: footer` make a `<header>` and a `<footer>`. As
direct children of `<body>`, as in a site shell, they are the banner and
contentinfo landmarks; inside `<main>` or another container they are not.
Neither needs an `aria-label`, since a page has one of each. They suit a site shell's top
bar and foot; add a class to style them:

```markdown
::: header {.topbar}
**Acme**
:::
```

## Collapsible blocks read on GitHub

`::: details` shows on GitHub as literal text with its body always open.
For a file read there too, write the HTML, with a blank line after
`<summary>` and before `</details>` so the body stays Markdown:

```markdown
<details class="container">
<summary>Title</summary>

Body in **Markdown**.

</details>
```

It renders here exactly as `::: details` does. Without the `container`
class it is unstyled.
