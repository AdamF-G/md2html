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

## Highlighting

A fence is highlighted when its language names a grammar md2html has,
ignoring case. There is nothing to configure:

````markdown
```python
def greet(name):
    return f"Hello, {name}"  # a comment
```
````

md2html colours the code when it builds the page, so the page gains no
script. Each token is wrapped in a `<span>` classed by its kind, such as
`hl-keyword`, and the `<code>` element gains the class `md2html-hl`. The
text is unchanged, so copying the code and site search see what they did
before. The colours come from the page's style; see
[Syntax colours](styles.md#syntax-colours).

A fence can name a language by an alias: `sh`, `bash` and `zsh` reach
`shellscript`, `yml` reaches `yaml`, and `py` reaches `python`.
`md2html --list-languages` lists every language, with its aliases, the
plugin it comes from, and its licence and source.

**No match, no change.** A fence with no language, or one no grammar
names, renders exactly as it did before, and so does a fence in a known
language that has nothing to colour. A near miss of a known name warns
and stays plain: `fence at line 12: no grammar for "pyhton"; did you mean "python"?`.
Only a tag of four letters or more is checked, and only against a name
one edit away, or two from eight letters up. Tags that mean "no language",
such as `text`, `txt`, `plain`, `none`, `output` and `tree`, never warn,
and neither do the languages md2html leaves out, listed below with their
aliases, such as `sass` and `glsl`, or a few real languages no built-in
grammar covers: `patch`, `nasm`, `masm`, `ninja` and `rest`. These are
plain and silent.

Opting out, at three levels:

- one fence: a `no-highlight` class, as in ```` ```go {.no-highlight} ````.
  The class stays on the `<code>` element, as any other class does;
- one page: `highlight: false` in front matter;
- one run: `--no-highlight`.

`mermaid`, `fig` and `vega-lite` fences are never highlighted, even when
one falls back to a code block.

### Languages md2html does not ship

The built-in grammars come from the `tm-grammars` collection, each under
its own licence; `md2html --licenses` prints them. These are left out,
because their licence is not one md2html can ship or is not given:
abap, ada, ahk2, apache, apex, apl, applescript, asciidoc, bird2, cue,
dax, dream-maker, elixir, erb, glsl, gnuplot, hcl, hurl, kusto, llvm,
logo, luau, matlab, mipsasm, nginx, nim, nsis, org, po, purescript,
racket, rel, sass, sparql, ssh-config, tcl, terraform, ts-tags, turtle
and wolfram. Any of them can be supplied by a local plugin, under a
licence you have checked. `bicep` is left out for another reason:
md2html's tokenizer does not read its grammar the way VS Code does.

To supply a language, put its TextMate grammar, a `.tmLanguage.json`
file, in a plugin with a `grammars` key, and load the plugin with
`--plugin` or a page's `plugins:`. A plugin can provide only languages;
it needs no style. A language a plugin provides replaces a built-in one
of the same name. [Providing languages](styles.md#providing-languages)
gives the manifest.
