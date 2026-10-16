---
nav-order: 2
---
# Silent Traps

Each of these gives valid output, a zero exit and no warning. Check for them
before you finish.

1. **A fence inside a fence needs a longer outer fence.** Showing a
   three-backtick block takes four backticks around it, and showing a `:::`
   container inside a container takes `::::` on the outer one. See
   [Nesting fences](code-blocks.md#nesting-fences).
2. **An unclosed `:::` container runs to the end of the document**, headings
   and all.
3. **Link to the `.md` source, never the `.html` output.** An `.html` link
   is never matched to the page md2html emits, and when the file exists
   nothing warns. See [Links and Images](links-and-images.md).
4. **Only the first class names a container's kind.** `::: {.compact
   .warning}` is a plain, unstyled div; write `::: warning {.compact}`.
5. **A container class outside the shipped kinds is unstyled.**
   `::: {.house-style}` is a plain classed div. Only the bare-word form,
   `::: house-style`, warns.
6. **`title:` in front matter sets only the browser tab.** Still write the
   `# Title` heading.
7. **Front matter must be the first bytes of the file and closed by `---`.**
   After a byte order mark, or without the closing line, it renders as body
   text.
8. **`[word]` next to `{…}` becomes a span.** `map[key]{value}` in prose
   renders `map<span>key</span>` and loses `{value}`. Put it in a code span.
9. **A backslash does not escape a status word or `§`.** `\[draft]` is
   still a badge. Put it in a code span.
10. **An unquoted ` #` in a `fig` label starts a comment.** `box: Step #3`
    renders "Step". Other unquoted labels that YAML cannot read fail with a
    warning. See [Quote labels](diagrams.md#quote-labels).
11. **A backtick in a code caption breaks every fence after it.** Use a
    `~~~` fence for that block.
12. **A caption on a `mermaid` fence is dropped.** Caption the diagram in
    prose.
13. **A `|` inside a code span in a table still splits the cell.** Write
    `\|`.
14. **Markdown inside an HTML block needs a blank line before and after
    it.** Without them, `**bold**` stays literal.
15. **`## 4.2. Title` claims no number.** Anything but a space after the
    number, `.`, `)` or `:`, stops `§4.2` from linking to it.
16. **A fragment in a link is never checked.** `./b.md#no-such` passes.
17. **Two explicit `{#same}` ids both stay.** The page then has duplicate
    ids, and links go to the first.
18. **A heading's attribute block needs double-quoted values.** A bare key
    or a single-quoted value, as in `## Setup {#setup data-flag}`, leaves
    the whole block in the heading text, id included. See
    [Headings and anchors](page-structure.md#headings-and-anchors).
19. **An attribute block works only after a heading, link, image or span,
    or on a container or code fence's opening line.** After a paragraph or
    list item, `{.lead}` stays literal text.
20. **A style's light colours set only on `:root`.** The default stylesheet
    also sets them on `:root[data-theme="light"]`, which wins. A page
    switched to light then shows the default's colours. Set each colour on
    `:root[data-theme="light"]` too. See
    [What a style can change](styles.md#what-a-style-can-change).
21. **A chart that tells its categories apart only by
    opacity has a weak legend.** Shades of one colour are hard to match to
    their legend entries, more so where overlapping marks add up, and
    hovering the legend highlights nothing. Give each category its own
    colour with a `color` encoding.
22. **A literal colour in a chart's spec doubles its size.** It cannot
    follow the theme, so the chart is drawn twice, once per theme. Use a
    `token:` colour. See [Colours](charts.md#colours).
23. **A template outside a marker stays literal.** `{version}` expands only
    in a marker's attributes.
24. **A fence whose language has no grammar renders plain.** Only a near
    miss of a known name warns. `--list-languages` lists the names.
