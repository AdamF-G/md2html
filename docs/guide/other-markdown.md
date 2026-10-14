---
nav-order: 10
---
# Other Markdown

## Tables

GitHub tables. Each is wrapped in a horizontal scroll container, so do not
wrap one yourself. A `|` inside a code span in a cell still splits the
cell; write `\|`.

```markdown
| Option | Default |
|---|---|
| `--depth` | unlimited |
```

A table is as wide as the text column, and scrolls when it needs more.
To give one more room, wrap it in a `wide` container, which extends past
the column the way a [wide figure](diagrams.md#layout) does, with the same limits:
it has no effect inside another container, or in `--fragment` output that
its host places inside a box of its own. The same works for a code block;
`{.wide}` on the fence itself classes the `<code>`, not the block.

```markdown
::: wide
| Option | Default | Notes |
|---|---|---|
| `--link-depth` | unlimited | … |
:::
```

## Footnotes, definition lists, task lists

```markdown
Claim needing support.[^src]

[^src]: The source.

Term
: The definition.

- [ ] not done
- [x] done
```

All three render. Definition lists are styled; footnotes and task-list
checkboxes are plain HTML.

Also available: `~~strikethrough~~` (a single `~` pair strikes too, so
`H~2~O` is struck), autolinked bare URLs, and the rest of GitHub Flavored
Markdown. Not supported: math, `==highlight==`, `^superscript^` and
`:emoji:` codes, which stay literal.

## Raw HTML

Raw HTML passes through, including `<style>`, `<script>` and inline `<svg>`.
Use it only when Markdown cannot express the thing. Leave a blank line
between an HTML tag and Markdown inside it, or the Markdown stays literal:

```markdown
<div class="card">

**Bold**, as Markdown.

</div>
```

HTML comments stay in the page, and in its embedded source.

Images and inline SVGs that are scaled down to fit the text open full size
when clicked.
