# Fences inside Markdown fences — Design

**Date:** 2026-10-16
**Author:** AdamF-G
**Status:** Built on the `syntax-highlighting` branch, 2026-10-16.
Amends [Syntax highlighting](2026-10-14-syntax-highlighting-design.md).
[As built](#as-built) records what building found.

## Purpose

A Markdown fence often shows a Markdown example that has fences of its
own:

`````markdown
````markdown
# Deploying

```yaml
on: push
```
````
`````

Today the Markdown grammar colours the inner `yaml` fence itself. VS
Code's Markdown grammar has one hand-written rule per embedded language,
61 of them, and textmate-go loads and compiles every grammar those rules
name the first time a Markdown fence is highlighted. That has three
costs:

- **Time.** A page with one one-line Markdown fence takes 0.08 s to
  build, against 0.01 s or less for a fence in Go, YAML, shell, JSON,
  CSS, Python or HTML. This repository's docs have 34 Markdown fences,
  more than any other language, and build in 0.16 s, against 0.033 s
  with `--no-highlight` (built binary, Apple Silicon, warm).
- **Coverage.** Only VS Code's 61 languages are coloured inside a
  Markdown fence, under VS Code's spellings. The other built-in
  languages, every language a plugin provides, and md2html's own aliases
  stay plain there.
- **Dead rules.** Six of the 61 name grammars md2html does not ship: abap
  and elixir are left out for their licence, and `source.twig`,
  `source.yang`, `source.ignore` and `source.js.regexp` match no grammar
  in the collection.

Measured with a Markdown grammar whose per-language rules were removed,
the one-line page takes 0.02 s and the docs 0.06 s.

## Behaviour

**A fence inside a Markdown fence is highlighted as if it stood on the
page.**

- **Markdown fence** means a fence whose language resolves to the grammar
  with scope `text.html.markdown`. That covers `markdown`, `md`, and a
  plugin's replacement Markdown grammar.
- **The inner fences** are the fenced code blocks a CommonMark parser
  finds in the outer fence's body: goldmark's default parser, the one
  md2html is built on. That includes fences inside list items and block
  quotes, where each line starts with a marker or `>`. An inner fence
  left unclosed runs to the end of the outer body, as CommonMark says.
  Indented code blocks have no language and stay plain.
- **The inner fence's language** comes from its info string, read by the
  same parser as a top-level fence's. Both the plain form (`go`) and the
  attribute form (`{.go}`) work, and `.no-highlight` turns it off. It is
  resolved in the page's language set, so every built-in language, alias
  and plugin language works. A spelling only VS Code knew, such as
  `golang`, stays plain.
- **Inner fences are text.** A `fig`, `vega-lite` or `mermaid` fence inside
  a Markdown fence is an example of one, so it is coloured by its grammar
  if it has one. It is never rendered as a figure, chart or diagram.
- **Unknown languages are silent.** An inner fence in a language no
  grammar names stays plain, with no "did you mean". The example may be
  wrong on purpose.
- **Nesting recurses.** An inner Markdown fence has its own inner fences
  highlighted the same way. This ends because each inner body is strictly
  shorter than the body around it.
- **The outer fence keeps its own colours.** Headings, links and the
  inner fence's own ```` ``` ```` lines are coloured by the Markdown
  grammar as before. The inner body's colours replace whatever the outer
  grammar gave those characters.

## The Markdown grammar

**The built-in Markdown grammar ships unchanged.** Nine other built-in
grammars include `text.html.markdown`, including vue, svelte, mdc and the
F# and Erlang doc-comment grammars, and they keep VS Code's behaviour.

To highlight a Markdown fence, md2html derives a **host grammar** from
whichever grammar serves `text.html.markdown`:

- Its `fenced_code_block` rule keeps only its last pattern,
  `#fenced_code_block_unknown`, VS Code's rule for a fence in any
  language, which embeds nothing.
- The 61 `fenced_code_block_<language>` repository entries are removed.
- Its `scopeName` becomes `text.html.markdown.md2html-host`, so a grammar
  that includes `text.html.markdown` still gets the full grammar. No rule
  in the scope-to-kind table matches on the root scope, so the rename
  changes no colour.

The host grammar still includes `text.html.derivative` (for HTML blocks)
and `source.yaml` (for front matter), as the full one does.

**If the structure is not there, the grammar is used as it is.** That
applies when a plugin's Markdown grammar has no `fenced_code_block` rule
ending in `#fenced_code_block_unknown`. The inner fences are still
highlighted by md2html, and their colours replace the grammar's. The
result is the same, only slower.

The derivation is generic over the grammar's structure, not tied to one
copy of it. It is computed once per language set and cached with it.

## Faults

| Fault | Result |
|---|---|
| Inner fence over its time budget | Inner body plain; warning naming the inner fence's line in the file and its language; outer colours kept |
| Engine error on an inner fence | Inner body plain; warning with the error; outer colours kept |
| Inner fence in an unknown language | Inner body plain; silent |
| Inner fence's language from a plugin grammar with untranslatable patterns | The same warning as a top-level fence would give |

- **Each inner fence has its own 2 s budget**, as a fence on the page
  does. A Markdown fence with many slow inner fences can take as long as
  a page of them.
- **Warning lines are file lines.** A warning names the inner fence's
  opening line in the file: the outer fence's line, plus one, plus the
  inner fence's line within the body.

## The untranslatable-pattern warning

The existing spec says a local grammar with untranslatable patterns gets
"one warning listing them, from the first page that uses the grammar". In
fact it is said again on every page, because the record of what was said
lives with the page. For the Markdown grammar the warning is several
kilobytes long. This work fixes it:

- **Once per run, after the pages.** Pages convert in parallel and share
  one engine per language set, so "the first page" to find a pattern
  depends on scheduling. The patterns found by the end of the run depend
  only on what the pages highlighted. `PluginCache.GrammarWarnings`
  returns one warning per local grammar, listing every pattern any of
  the cache's conversions needed. Warnings are sorted by plugin and
  grammar, and patterns within each sorted. The command line prints them
  after the pages, with no page named, and counts them in its warning
  total. `Convert` says them through `Options.Warn` itself only when it
  was given no cache, since its own cache serves one page.
- **Shorter.** Each entry over 160 characters keeps its first 60 and
  last 90, with ` … ` between, as `plugins/grammars/REPORT.md` already
  does. The end is kept because it carries the reason.

## Output changes

- **Markdown fences whose inner fences VS Code's grammar coloured** keep
  the same colours. A test checks this for every built-in language that
  has a VS Code rule, under every spelling that rule and md2html both
  accept. Any language where the two differ is listed in the test with
  its reason.
- **Newly coloured:** inner fences in the other built-in languages, in
  plugin languages, and under md2html-only aliases.
- **Newly plain:** inner fences under spellings only VS Code's rules
  accepted, such as `golang`.
- **Pages without a Markdown fence** are unchanged, byte for byte.

## Testing

- **Equivalence with the full grammar.** For each language with a VS Code
  fence rule and each spelling both accept, a Markdown fence holding an
  inner fence of the first lines of that language's conformance sample
  gets the same spans through md2html's path as through the full grammar.
- **The host grammar matches the full one outside inner bodies.** On the
  Markdown conformance sample, every line that is not inside an inner
  fence's body gets the same spans from both grammars.
- **Coverage:** an inner fence in a language VS Code has no rule for
  (`zig`), under an md2html alias, and from a plugin grammar.
- **Placement:** inner fences in a list item, in a block quote, with
  tildes, unclosed, with a tab before the code, and in the attribute form
  with `.no-highlight`.
- **Recursion:** a Go fence inside a Markdown fence inside a Markdown
  fence is coloured.
- **Faults:** an inner fence over budget warns with its file line and
  leaves the outer colours.
- **No eager loading:** highlighting a Markdown fence whose inner fences
  are all Go loads the host, Go and nothing from the 61-rule list. The
  engine's loader records the scopes it was asked for.
- **Warning once:** pages sharing a cache say nothing, and the cache's
  warning lists both patterns found across them; without a cache one
  page with two fences says one warning; a command-line run of twelve
  pages says it once, naming no page, the same way on every run.
- **Unchanged output:** the v0.12.0 comparison passes as before.

## Documentation

- The guide's Highlighting section says that fences inside a Markdown
  fence are coloured in any language the page knows.
- The changelog's highlighting entry gains this, and its build-time
  figures are measured again.
- The highlighting spec's Faults table, its untranslatable-pattern row,
  and its Costs as built point here.

## Out of scope

- Other grammars that embed Markdown (mdx, mdc, a Vue `<docs>` block, doc
  comments). They keep the full grammar and VS Code's 61 languages.
- Loading an embedded grammar only when it is first used, inside
  textmate-go. That is an upstream change. It would also help the
  grammars above.

## As built

- **Spellings that differ from VS Code.** 79 spellings are embedded by
  VS Code's Markdown grammar and shipped by md2html. Four colour
  differently inside a Markdown fence, each the way a top-level fence
  in that language is coloured:
  - `json5`: VS Code's rule hands it to the JSON grammar, md2html uses
    json5's own.
  - `tex`: VS Code's rule hands it to the LaTeX grammar, md2html uses
    plain TeX's.
  - `objc` and `objective-c`: the Objective-C grammar includes `$base`,
    which inside VS Code's embedding is the Markdown grammar, so VS Code
    reads a class body as Markdown paragraphs.
- **This repository's docs** change in one block: a `mermaid` example
  inside a Markdown fence in the diagrams guide is now coloured.
- **`innerFences` came forward to the host-grammar commit**, so the
  host-grammar test compares every line outside an inner body from the
  start, as the plan's Task 2 would have widened it to.
- **Tests that compare colours** use a one-minute budget, as the
  generator does: under `-race` the full grammar's comparison ran past
  2 s.
- **The warning went from once per language set to once per run.**
  The review found that saying it from the first page to reach it
  printed it under a different page from run to run (18 of 20 runs
  under one page, the others under two more), so it moved to the end of
  the run, as style checks are reported.
- **The comparison test fails on a stale entry.** A spelling listed as
  differing that now matches the full grammar fails the test, and only
  real comparisons count toward its floor of 50; 75 do.
- **Costs.** With a built binary, Apple Silicon, warm, best of five,
  process start included: the docs (35 pages, 78 highlighted fences)
  build in 0.08 s, from 0.18 s before, against 0.05 s with
  `--no-highlight`. A page with one one-line Markdown fence takes
  0.04 s, from 0.10 s; one with a Go fence 0.03 s; one with no fence
  0.02 s.
