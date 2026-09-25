# Extended content model — feature backlog

**Date:** 2026-09-11
**Status:** All nine items shipped. Item 1 (structured diagram fence) was
scoped as its own design,
[the fig fence design](./2026-09-12-fig-fence-design.md), and shipped in
v0.2.0.

## Purpose

md2html converts plain Markdown well: headings, tables, mermaid, footnotes,
definition lists, raw HTML passthrough. But documentation corpora commonly
grow conventions beyond plain Markdown — a diagram DSL for layouts mermaid
can't express, richer containers than a single callout class, inline status
markers, cross-reference shorthand, a generated table of contents, and a
build that must not touch certain subtrees.

This document lists those conventions as candidate features for md2html
itself, in enough detail to scope and prioritize each one independently,
without committing to any of them. Nothing here should be read as
"decided" — it's a survey of what "plain Markdown" doesn't cover yet, so
that ground can be covered deliberately, feature by feature, rather than
each gap being discovered mid-migration by whoever first points the tool
at a corpus that relies on it.

Each item below is independent. None require the others.

## Priority key

- **P0** — blocks adoption for any corpus that already leans on this
  convention; no reasonable workaround exists (content would need to be
  rewritten, or the tool can't be pointed at the tree at all).
  Ranking below reflects that all three P0 items are load-bearing in the
  same way (unrepresentable content, or unsafe to run at all) — order
  among them is not a further priority signal.
- **P1** — content renders in a degraded-but-safe way without it (an inert
  `<div>`, a plain-text token); adopting md2html doesn't require touching
  those documents immediately, but they read worse until it lands.
- **P2** — quality-of-life; no existing content breaks or degrades without
  it.

## P0 — blocks adoption without content rewrites

### 1. Structured diagram fence

**Gap.** The only diagramming path is a `mermaid` fence, rendered by a
graph-layout algorithm from a graph-description language. That covers
flowcharts and sequence diagrams well. It does not cover diagrams that are
fundamentally **hand-laid-out**: side-by-side panels, a vertical flow of
labeled boxes and connectors, an annotated file tree, parallel lanes of
chained steps, a row of stat tiles. Content built for that kind of figure
has no path into mermaid without discarding the layout and re-expressing it
as a graph — a lossy, one-way rewrite, not a syntax swap.

**Proposed shape.** A second fenced-code language (`fig`, or similar) whose
body is a small structured document — YAML is the natural choice, since it
is already a dependency-free way to describe nested key/value layout data —
describing a figure as a tree of typed items:

- a handful of **leaf kinds**: a labeled box, a connector/arrow, an
  emphasized result bar, a full-width accent rail, a stat tile row, a
  term/definition grid
- a couple of **grouping kinds**: a labeled container (a box that holds
  other items — a loop body, a call frame), a chain of steps, parallel
  lanes of stacked boxes
- a couple of **panel-level layouts**: side-by-side panels with optional
  weights, and a two-panel split with a labeled boundary between them
- a caption, rendered as inline Markdown, same as any other user-facing
  text in the figure

Each item's text fields render through the same inline-Markdown path used
for prose, so status markers, links, and code spans work inside a figure
exactly as they do in body text — see Section 2 below on chips.

**Where it fits architecturally.** This is not a tree transform — it is a
custom fence renderer, which the current pipeline has no hook for. The two
existing fenced-language special cases (mermaid, and titled code blocks)
are both handled at the goldmark-extension layer, before the tree reparse.
The narrowest implementation is a third case there: recognize the fence's
language tag, parse the body as structured data instead of code, and emit
the figure's HTML directly in place of a `<pre><code>` block. A transform
walking the already-parsed tree is a viable alternative — the fence's raw
text content is still recoverable from inside a `<pre><code
class="language-fig">` node at that point — but doing it at the extension
layer keeps figure markup out of goldmark's HTML-escaping pass entirely,
which a post-hoc transform has to work around.

**Degrades to:** a fenced code block showing the raw structured text. Not
broken, not silently wrong, just unstyled — which is the right default
for a feature nobody has opted into.

**Scope note.** This is by a wide margin the largest single item in this
list — a new fence language, a rendering module comparable in size to the
whole transform layer, and its own stylesheet component set. It should be
scoped as its own design, not folded into a general "extensibility" pass.

### 2. Container variety and brace-free syntax

**Gap.** Two frictions compound here:

1. The default stylesheet gives real styling to exactly one container
   class (`.callout`). A team wanting a collapsible aside, an
   always-visible highlighted card, a worked-example block, or a
   warning/risk callout gets a correctly classed but visually inert `<div>`
   for anything past that one class — the container renders, but looks
   like a plain paragraph until the caller supplies their own CSS.
2. The brace syntax (`::: {.callout}`) is stricter than the bare-name form
   many hand-rolled renderers accept (`::: callout`), and the bare form
   fails **silently** — no warning, no error, just an unclassed div. A
   corpus authored against a bare-syntax convention converts with every
   container quietly losing its styling, and nothing in the output points
   at why.

**Proposed shape.**
- Ship stylesheet rules for a small fixed set of container kinds beyond
  `.callout`: something collapsible with a summary line (a `<details>`
  wrapper), something for a worked example (same shape, different label
  prefix), an always-visible highlighted card, and a warning/risk variant
  of the callout. Four or five kinds covers what most corpora reach for;
  this is a fixed vocabulary, not an open one — arbitrary classes stay
  exactly as inert as they are today.
- Accept the bare `::: kind Title text` form as an alias for the braced
  one, at minimum for the shipped kinds, so a corpus that already writes
  bare containers doesn't need a syntax migration on top of a tool
  migration. If bare and unbraced forms both exist, a bare name that
  doesn't match a known kind should warn rather than silently emit an
  unclassed div — the current failure mode is the actual problem, not just
  the missing styling.
- Support nesting (a container inside a container) and an inline-Markdown
  title on the same line as the opening fence, both of which the shipped
  kinds should support symmetrically.

**Degrades to:** a correctly-structured but unstyled `<div>` for any kind
not in the shipped set — already true today, and the right fallback for a
kind nobody has designed CSS for.

### 3. Exclusion directories and traversal modes

**Gap.** Traversal today has exactly one policy: any linked `.md` is
pulled in, unconditionally, however far away it is. There is no way to
tell the crawler "this subtree exists, may be linked to, and must never be
entered or overwritten" — which matters whenever some other tool already
owns a subtree's HTML output (a slide-deck renderer, a docs generator for
a vendored dependency, a frozen historical archive nobody wants touched).
Pointed at a tree containing such a subtree, the crawler either follows a
link into it and overwrites files it doesn't own, or someone has to keep
that subtree out of the source tree entirely — which usually isn't
possible, since it's still real content that belongs where it lives.

A related, narrower gap: today's traversal is single-mode (unlimited
BFS from every seed). Some corpora want a stricter policy for their
maintained set specifically — pull in whatever a *seed* document links to,
but don't keep chasing links from documents that were themselves pulled in
only because something else linked to them. That bounds how far "one
stray link" can reach into unrelated content, independent of directory
structure.

**Proposed shape.**
- An `--exclude` flag (repeatable, or comma-separated), each value a
  directory prefix relative to the resolved base. Matching paths are
  never entered as seeds, never followed as link targets, and never
  written to — a link pointing at one renders with its href intact and
  unconverted, same handling as a link that escapes `base` in in-place
  mode today.
- Excluded-directory checks apply before the existing containment check,
  not instead of it — both are real constraints and either one alone
  should be able to stop a given path.
- Optionally, a traversal-depth concept for link-following itself,
  distinct from the existing `--depth` (which only bounds *seeding*): a
  flag that limits BFS to N hops from a seed, so "pulled in something
  that pulled in something else" stops being unbounded by default for
  callers who want that. Unbounded stays the default — this is additive,
  not a behavior change.

**Degrades to:** nothing safe today — this is the one item in this list
where the current behavior isn't a degraded-but-tolerable fallback, it's
either a build that overwrites content it shouldn't, or a source tree
that has to be restructured around the tool's limitation.

## P1 — content renders, degraded

### 4. Inline status chips

**Gap.** No inline syntax exists for turning a short bracketed token into
a small styled badge — the pattern used for marking a claim as verified
vs. designed vs. planned, or attaching a short inline label to a term.
Content using such a convention converts with the bracketed text rendering
as plain inline text: `[proven]` stays literally `[proven]` in the output
rather than becoming a badge.

**Proposed shape.** A small, fixed set of recognized bracket tokens
(e.g., a handful of status words, plus a generic `[c:label]` free-text
form) rendered as `<span class="chip ...">`, applied everywhere inline
Markdown is rendered — body text and headings both, since a status marker
in a heading is a real use case and shouldn't need special-casing.
Interacts with heading-anchor slugs: the marker text should not become
part of the generated slug, so relabeling a marker later doesn't rot an
existing anchor link — this needs the same "strip before slugifying, keep
in the visible heading" handling `HeadingAnchors` already gives explicit
`{#id}` overrides.

**Degrades to:** literal bracketed text, e.g. `[proven]` — readable,
just unstyled. Genuinely low-risk to leave undone.

### 5. Section-number autolinking

**Gap.** No support for a bare cross-reference shorthand — writing `§4.2`
inline and having it resolve to a link, at render time, to whichever
heading in the same document is numbered `4.2` (a heading whose visible
text starts with that number, e.g. `## 4.2 Rollback`). Without it, such a
reference either has to be a hand-written anchor link (which rots the
moment a section is renumbered or reordered) or stays a plain-text
citation with no link at all.

**Proposed shape.** A transform, not a goldmark extension — it needs the
fully assembled heading set (with anchors already assigned) to resolve
against, so it runs after `HeadingAnchors`. Scan heading text for a
leading `N` or `N.M` number, build a number→anchor map for the document,
then autolink bare `§N` / `§N.M` occurrences elsewhere in the body against
that map. Needs to leave alone: references qualified as pointing at
*another* document ("see the design doc's §7"), any `§` already inside a
link, code span, or code block, and any `§` with no matching heading in
the same document — all of those should stay literal text rather than
producing a broken or wrongly-scoped link.

**Degrades to:** literal `§4.2` text — reads fine, just not clickable.

### 6. Generated table of contents

**Gap.** No navigation is generated at all — this is a stated non-goal
today, not an oversight (see design notes). Reconsidering it here because
"no TOC" is a different kind of gap than "no sidebar/no site nav": a
single marker token that expands, at build time, to a flat linked list of
the current document's own headings is a much smaller ask than site-wide
navigation, and some source conventions rely on exactly that (a marker
line that a hand-rolled renderer replaces with a same-page contents list,
also recognized as inert plain text by anything else that renders the
raw Markdown unprocessed).

**Proposed shape.** A single recognized marker, alone on its own line,
outside code fences, replaced with a `<nav>` linking every heading anchor
in the document in order — flat, not nested by heading level, so an
irregular heading-level jump can't produce broken list nesting. Needs to
run after heading anchors and after chip-stripping, so the TOC's link
text matches what a reader actually sees in each heading.

**Explicitly still out of scope even if this lands:** any *cross-document*
navigation, sidebar, or site index. This item is "table of contents for
the page you're on," nothing broader — widening it is a different, much
larger feature.

**Degrades to:** a literal marker line sitting in the rendered body,
visible as plain text — ugly, but not misleading, and an easy thing to
notice and fix per-document.

### 7. Front matter and heading-lift title/subtitle

**Gap.** Title derivation is single-strategy: first `<h1>`, else the
filename. No support for either (a) an explicit leading key/value block
overriding the title (and, past just a title, a subtitle and a date that
the page shell has nowhere to put), or (b) treating an italic line
immediately following the H1 as a subtitle automatically, without any
front matter present at all.

**Proposed shape.** Recognize a leading `---`-delimited block of flat
`key: value` lines before Markdown parsing begins, extracting at least
`title`, `subtitle`, and `date`; strip it from the body before handing the
rest to goldmark. Absent that block, and absent an explicit title, lift a
subtitle from a line consisting of nothing but italic text immediately
following the leading H1 (blank lines and a leading HTML comment tolerated
before the H1 is found). Both paths feed the same three optional slots;
the page shell needs a subtitle line and a date line to actually put them
on the page, which it doesn't have today.

**Degrades to:** the front-matter block rendering as a literal `<hr>`
followed by a paragraph of `key: value` text sitting above the title —
noisy but not misleading, and immediately obvious that something needs
fixing. The heading-lift path (b) degrades even more gently: the subtitle
line just renders as a normal italic paragraph.

### 8. Fenced code block captions

**Gap.** No way to attach a caption to a code fence — a short label like
"this file, this config, this command" rendered as a bar above the block,
distinct from the language tag and distinct from a floating paragraph
above the fence that isn't visually attached to it.

**Proposed shape.** Recognize an additional token in the fence info
string (info strings already carry the language; this is one more
space-separated attribute, not a new fence syntax) supplying caption text,
consumed and stripped before the remaining info string is treated as a
language for syntax purposes. Render as a `<figure>` wrapping a
`<figcaption>` above the existing `<pre><code>` output — additive to
today's fence handling, not a replacement path.

**Degrades to:** the caption attribute sitting unconsumed in the fence's
language slot — visibly wrong (an unrecognized "language") but confined
to one line of chrome, not the code itself.

## P2 — quality of life

### 9. Supplemental per-invocation entry set

**Gap.** Seeding is whole-tree or explicit-entry-list only. No way to say
"build the maintained set, *and* also pull in this one extra directory for
this run only" without permanently adding that directory to the standing
entry list. Marginal on its own — multiple explicit entry-point arguments
already cover most of what this would add — but worth naming because nested
or exploratory content (drafts, research notes, working scratch docs) that
shouldn't be part of the standing build target still sometimes needs an
occasional one-off render, and today that means a temporary edit to
whatever invokes the tool rather than an extra CLI argument.

**Proposed shape.** Nothing beyond documenting that multiple directory/file
arguments on one invocation already serve this purpose, if that isn't
already clear from existing CLI examples. Revisit only if real usage shows
the multi-argument form is insufficient.

## What this list deliberately does not include

- **Site-wide navigation, search, sidebars.** Named as explicitly out of
  scope in item 6 above; nothing here reopens that.
- **A plugin/registry system for fence languages or container kinds.**
  Every item above is scoped as a fixed, shipped feature, not an
  extension point for arbitrary user-defined ones. The library's
  `Transform` hook already covers the general extension case for anyone
  who wants to go further than what's shipped.
- **Config files.** None of the above requires one; flags and fixed
  vocabularies are sufficient for everything in this list.
