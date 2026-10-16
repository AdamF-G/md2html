---
nav-order: 3
---
# Page structure

## Front matter

A `---` block at the very top of the file, of flat `key: value` lines:

```markdown
---
title: Rollback runbook
subtitle: What to do when a deploy fails
date: 2026-09-11
lang: en-GB
toc: float
toc-title: Contents
---

# Rollback runbook
```

| Key | Effect |
|---|---|
| `title` | The `<title>`, shown in the browser tab. Nothing on the page. |
| `subtitle` | A `<p class="subtitle">` under the first `<h1>`. |
| `date` | A `<p class="docdate">` under the subtitle. |
| `title-block` | `above` puts the subtitle and date over the `<h1>` instead, as a short eyebrow line, each also classed `eyebrow`. Default `below`. |
| `lang` | The page's `<html lang>`. Overrides `--lang`; default `en`. |
| `toc` | The contents list's layout: `inline`, `float` or `none`. See [Contents list](#contents-list). |
| `toc-title` | The contents list's name for screen readers. Default `Table of Contents`. |
| `style` | The page's styles, stacked in order. Replaces `--style`. See [Styles](styles.md). |
| `plugins` | Plugin directories, relative to the page: styles this page's `style:` can draw on, and languages its fences are highlighted in. |
| `highlight` | `false` leaves the page's code plain. See [Highlighting](code-blocks.md#highlighting). |

`lang` is what screen readers use to pick a voice. A value that is not a
language tag, such as `en_US`, warns and is ignored.

Keys are case-insensitive, a value may be quoted (`title: "Rollback: why"`),
and a repeated key keeps its last value. Any other key does nothing on the
page, but it still travels in the embedded source unless you build with
`--no-source`.

The block must be flat. A nested value, a YAML comment, or a line without a
colon makes the whole block fail to parse, and it renders as body text with
a warning.

Without front matter, an italic line directly under the first `<h1>` becomes
the subtitle. That also happens to an italic line you meant as prose, such
as `*Draft, do not circulate*`; give the page a `subtitle:` or put the
line lower down.

## Title

Write the title as the first `# heading`. The page `<title>` comes from
front matter `title:`, else from that first `<h1>`, else from the file name.

## Headings and anchors

Every heading gets an `id` and a hover anchor link. Slugs follow the rules
GitHub, GitLab and Pandoc share: lower case, punctuation dropped, each space
a hyphen, nothing merged afterwards. `## Sizes: small × large` is
`#sizes-small--large`. Letters and digits from any script are kept, so
`## はじめに` is `#はじめに`; a heading with none gets `section-N`. A repeated
heading is numbered: the second `## Setup` is `#setup-1`. A status badge in
a heading stays out of its slug.

For an anchor that survives rewording, give the heading an explicit id:

```markdown
## Breaking change in v2 {#breaking-v2}
```

A heading's attribute block takes an `id`, classes, and `key="value"`
pairs, with a stricter grammar than a container's. Write every value in
double quotes: `{data-step="1"}`. An unquoted number becomes an empty
value, and a bare key (`{data-flag}`) or a single-quoted value leaves the
whole block, id included, as literal heading text. It takes the common
global attributes (`title`, `lang`, `dir`, `role`, `style`…) and `data-`
names; `aria-` names and event handlers are dropped. An explicit id is
never rewritten, and generated slugs avoid it. GitHub shows the block as
literal text.

An attribute block at the end of a heading line belongs to the heading,
even straight after a link: `## See [docs](./docs.md){.ext}` puts the class
on the `<h2>`.

## Contents list

`[[toc]]` alone in its paragraph becomes a `<nav class="toc">` listing the
page's headings, each linking to its anchor. `[TOC]` works too, in any
case. Depth shows as `›` marks, up to three. Headings inside a `details`, `aside`
or `example` block are listed too, even though the block starts closed.

```markdown
[[toc]]
```

On a long page, `toc: float` in front matter pins the list beside the text
on a wide screen. On a narrow screen and in print it is the inline list
again. `--toc float` does the same for a whole run, and `toc: inline` opts
one page out. Only the first list on a page floats.

A run can add the list itself. `--autotoc all` gives every page without a
marker one, and `--autotoc long` only pages over 1000 lines or five
headings. It goes under the title, subtitle and date, and floats unless
`--toc` or the page's `toc:` says otherwise. `toc: none` opts one page out.

Things that leave no list:

- `toc: float` or `toc: inline` without a marker, unless `--autotoc` adds
  a list. Front matter warns; `--toc float` does not.
- A marker inside a code span or a link, or sharing its paragraph with
  other text. It stays literal.
- A page with no headings, or built with `--no-anchors`. The marker stays
  as literal text.
- GitLab's `[[_TOC_]]`, which is not recognized.
