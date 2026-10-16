---
nav-order: 9
---
# Inline Marks

## Status badges and spans

Six lowercase words in brackets become badges, in prose or a heading:

```markdown
[proven] [verified] [designed] [planned] [draft] [deprecated]
```

`[proven]` renders `<span class="chip chip-proven">proven</span>`.
`[Proven]` stays literal.

Any other badge uses Pandoc's bracketed span syntax with the `chip` class:

```markdown
[needs review]{.chip}      ->  <span class="chip">needs review</span>
[shipped]{.chip .chip-ok}  ->  <span class="chip chip-ok">shipped</span>
```

A chip takes a tone: `.chip-ok` and `.chip-warn` use the status colours of
`[proven]` and `[deprecated]`, and `.chip-info` the accent. Without one it
is neutral.

The span syntax takes any class: `[lead in]{.lead}` is a
`<span class="lead">`, unstyled unless your stylesheet names it. The label
must be plain text, since `[**x**]{.chip}` stays literal, and only classes
and an id are kept.

A badge stays literal inside a code span, a code block or a link's text.
A backslash does not stop it, so write `` `[draft]` `` to show the word.
`[c:label]` also makes a badge, up to 60 characters.

## Section cross-references

`§4.2` links to the heading whose text begins with `4.2` followed by a
space or the end of the heading. `§ 4.2` works too.

```markdown
## 4.2 Rollback

See §4.2 for details.   ->   <a class="xref" href="#42-rollback">§4.2</a>
```

Number headings `4.2`, not `4.2.`: any other character after the number
means the heading claims none. When two headings claim one number, the
first wins. A number no heading claims stays as plain text.

A possessive naming another document, "the design doc's §7", stays literal.
So does a `§` in a code span. A backslash does not stop it. `--no-anchors`
turns cross-references off along with anchors.

Use `§` when your headings are numbered. Otherwise link the heading's
anchor: `[Rollback](#rollback)`.
