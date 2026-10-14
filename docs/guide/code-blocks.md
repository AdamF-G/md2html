---
nav-order: 6
---
# Code blocks

## Nesting fences

A fence closes at the first line with at least as many backticks as it
opened with.
To show a three-backtick block, open the outer fence with four:

`````markdown
````markdown
```go
func main() {}
```
````
`````

Get this wrong and the example ends early, the rest becomes body text, and
nothing warns. A `~~~` fence avoids the question for code that contains
backticks.

## Captions

A `caption` in a fence's attribute block adds a caption bar and wraps the
block in a `<figure class="code-figure">`:

````markdown
```go {caption="server.go"}
func main() {}
```
````

Write the language first and the braced block after it. GitHub reads the
first word as the language and still highlights the block, Pandoc reads the
caption, and `\"` inside the value is a literal quote.

The caption is plain text apart from status badges and `§` references,
which render as they do in prose; `*emphasis*` and links stay literal. A
backtick in a caption is invalid on a backtick fence and breaks every fence
after it; use `~~~` for that block. An `id` reaches the `<pre>` and further
classes reach the `<code>`. Other keys, including `title=`, are ignored.

Two other spellings work: brace-free, ```` ```go caption="server.go" ````,
which cannot escape a quote, and fully braced, ```` ```{.go caption="x"} ````,
which GitHub does not highlight.
