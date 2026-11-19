# Span Attributes — Design

**Date:** 2026-11-19
**Author:** AdamF-G
**Status:** Draft for review.

## Purpose

A bracketed span keeps only its id and classes. `parseAttrs` reads every
key=value pair, but `spanNode` (`chip.go`) never writes them out, so
`[beta]{.chip title="May change before 1.0."}` renders
`<span class="chip">` and the hover explanation is lost. An author who
wants one writes raw `<span class="chip" title="…">` instead.

This note lets a span keep the attributes a link block already keeps.

## 1. What a span keeps

A name is lowercased, then kept if it is in goldmark's
`GlobalAttributeFilter` (`title`, `lang`, `dir`, `role`, …) or starts with
`data-` or `aria-`. That is the rule `linkattrs.go` applies, with the global
list standing in for the link list, since a span is not a link. Any other
name, such as an event handler, is dropped and the rest of the block still
applies.

`id=` and `class=` are handled as they are today: the last id wins, and
classes add up in the order written. The chip modifier rule is unchanged.

Kept attributes are written in sorted order after `id` and `class`, so the
same block always renders the same way.

## 2. When a block stays literal

The braces and label stay as the author wrote them when the block:

- holds nothing (`[x]{}`), as today;
- holds a name that is not a safe attribute name (`safeAttrName`), as a
  link block does;
- holds a bare key with no `=`, other than `#id` or `.class`. This is new.

Today `[a]{b}` renders `<span>a</span>` and the braces disappear. This
isn't a security risk, because names are validated and values are escaped.
The worst a bare key could set is a boolean attribute such as `hidden` or
`contenteditable`, which the author could write in raw HTML anyway. The
real harm is that prose like `[x]{y}` gets eaten without a trace. Pandoc
has no bare-key form either, so treating it as prose follows Pandoc.

A block that holds only key=value pairs (`[c]{title="x"}`) is a span, now
with its title.

A boolean attribute is written with a value: `[x]{.chip hidden=""}`
renders `<span class="chip" hidden="">`, which HTML reads the same as a bare
`hidden`. The guide should show this form next to the bare-key rule.

### Warnings

A block that stays literal for a reason the author can fix raises a
warning. A failed build would be too much: no authoring mistake in
md2html stops generation, a missing link target only warns, and a block
md2html reads as invalid may still be prose. Staying silent is the
opposite trap, because the author sees braces on the page with no reason
given. Link blocks already warn when they refuse `href`.

| Block                                | Stays literal | Warns |
|--------------------------------------|---------------|-------|
| `{}`                                 | yes           | no    |
| `{y}`, `{b c}`: bare keys only       | yes           | no    |
| `{.chip hidden}`: a bare key beside `#id` or `.class` | yes | yes |
| `{.chip on"x=1}`: an unsafe name     | yes           | yes   |

A block of bare keys alone is usually prose, a regex or terminal markup,
so it stays quiet. Warning on it would bury the useful warnings. A
`#id` or `.class` shows the author meant attributes. Each warning names
the key that kept the block literal, and the bare-key warning names the
fix (`hidden=""`). Each gets an entry in `docs/guide/warnings.md`, as
`TestEveryWarningIsInTheGuide` requires.

`Chips()` takes a `warn func(string)`, as `LinkAttrs` does, and
`transform.go` passes the `warn` it already has. Only the body rewrite
warns. `stripChipTokens` runs on the same text for the page title and
stays silent, so a block in a heading warns once, not twice.

## 3. Page titles

`stripChipTokens` removes a chip's whole block, so a `title` never reaches
the browser tab. A non-chip span keeps only its label, as today. A block
that now stays literal (§2) is not a span, so its text stays in the title
as written. Its `parseAttrs` check should change to match.

## 4. Compatibility

No exported API changes. Three kinds of span render differently:

1. **A bare key stays literal.** `[a]{b}` and `[x]{.chip draft}` render a
   span today, and the bare key and the braces disappear. Now the text
   stays as written, and the second one warns.
2. **A kept key=value pair now appears.** `[x]{.chip title="…"}` and
   `[x]{data-sort="name"}` gain their attributes, which today are dropped.
3. **An unsafe name stays literal.** `[T]{data-y"onmouseover="alert(1)}`
   renders `<span>T</span>` today. That's safe only because no key is
   written. Now it stays as written and warns.

A scan of the `.md` files in the author's local repositories, on
2026-11-19, found 88 `[…]{…}` matches. The bare-key ones were regexes,
CSS and terminal markup, mostly in code, where spans are not made.
Anything of theirs outside code renders as a broken span today. The
key=value ones were this feature's own example and md2html's tests and
design notes. The changelog lists all three under Changed.

## Out of scope

- A span at the end of a heading is taken by goldmark's heading-attribute
  parser as the heading's own attributes: `# T [x]{.chip}` renders
  `<h1 class="chip">T [x]</h1>`. This was already true, and it's a TODO
  of its own.
- A `--strict` mode that fails the build on any warning. This is a TODO
  too.

## Approaches

- **Global filter plus data-/aria- (chosen).** This mirrors link blocks, so
  both features answer the question "what may this block set?" the same
  way.
- **`title` only.** This is the smallest change, but the next request
  (`lang`, `aria-label`) reopens it.
- **Any safe name, as containers do.** This lets `onclick` through on
  inline text, which link blocks were written to refuse.

## Left to the builder

- `parseAttrs` stores a bare key with an empty value, so `{b}` and `{b=""}`
  can't be told apart from `attrs`. Record bareness in the parser or check
  the tokens, whichever is cleaner.
- Whether span blocks and link blocks share one helper for the
  lowercase-and-filter step.
- The exact wording of the two warnings.
- Updating the guide's "only classes and an id are kept"
  (`docs/guide/inline-marks.md`), the changelog, and removing the TODO
  item.
