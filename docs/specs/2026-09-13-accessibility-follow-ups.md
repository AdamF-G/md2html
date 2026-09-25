---
title: Accessibility follow-ups
subtitle: two gaps found while planning fence line capture, deliberately not folded into it
date: 2026-09-13
---

# Accessibility follow-ups

**Status:** Item 1 shipped, with the fixed label recommended below and
Pandoc's `toc-title` front matter key as the override for a page in
another language. Item 2 is still a proposal.

## Purpose

Both items below were found while scoping
[fence line capture](./2026-09-13-fence-line-capture.md), which adds a `nav`
container kind and lets a container carry an `aria-` attribute. Each is a
real gap, each is small, and neither is needed for that change to be correct
— so they are recorded here rather than widening a branch that has already
grown twice.

They are listed in the shape the
[extended content model backlog](./2026-09-11-extended-content-model.md)
uses: enough detail to scope and prioritise independently, without
committing to either.

Priorities follow that document's key. **P1** — renders safely without it,
but reads worse for some readers until it lands. **P2** — quality of life;
nothing breaks or degrades without it.

## P1 — renders, degraded for assistive technology

### 1. A default accessible name for the generated contents list

**Gap.** `[[toc]]` emits `<nav class="toc">` with no accessible name.
`<nav>` is a landmark, and screen readers offer a landmark list to navigate
by; an unnamed one appears there as just "navigation". There is no visible
heading either, so a sighted reader takes context from the list's position
on the page while a screen reader user gets none.

Today the cost is small because a page normally has exactly one `<nav>`.
Fence line capture changes that: once `::: nav` is a shipped kind, a page
with a hand-written navigation block plus a contents list has two landmarks
that are indistinguishable from each other. That is the situation the naming
requirement exists for, and this tool will be the one creating it.

**Proposed shape.** `buildTOC` sets `aria-label="Table of Contents"` on the
`<nav>` node it already constructs. The route is free of the usual
obstruction: `toc.go` builds that node directly in the `x/net/html` tree, so
goldmark's attribute allowlist — which silently drops every `aria-*` — never
applies. `HeadingAnchors` already sets `aria-hidden` the same way.

**Three decisions it needs, which is why it is a follow-up and not a
one-line change.**

*A name, not a heading.* A visible "Table of Contents" heading would change
the appearance of every existing page, and would itself be a heading — so it
would take a slug, an anchor, and a place in the contents list it introduces.
An `aria-label` changes nothing visually and nothing about the anchor
namespace. Recommend the label alone.

*Overriding it.* `[[toc]]` is a bare marker with no attribute syntax, so a
document wanting a different name has no way to say so. `[[toc]]{aria-label="…"}`
would reuse the shared attribute parser and fit the dialect, but inventing
syntax before anyone needs it is the wrong order. Recommend a fixed string
until asked.

*Language.* The string would be English in a document of any language. When
this was written, `page.go` wrote `<html lang="en">` unconditionally, so the
page claimed English regardless. That gap is closed: a page's language now
comes from its `lang:` front matter or `--lang`. The label should follow the
same setting, which leaves one question: what it says in a language md2html
has no string for. Falling back to English there is no worse than today.

**Degrades to:** exactly today's output — a correct landmark with no name.
Nothing renders wrongly; it is simply less navigable.

## P2 — quality of life

### 2. Attributes on links, so `aria-current="page"` can be written

**Gap.** Nothing in the dialect attaches an attribute to a link. Bracketed
spans produce a `<span>`, so `[[Home](./home.md)]{aria-current=page}` wraps
the link rather than annotating it, and the attribute lands on the wrong
element.

A navigation block's one piece of state is which entry is the page you are
on. `aria-current="page"` is how that is expressed, and it has to sit on the
link. So the `nav` kind that fence line capture adds can emit a correct
landmark containing a correct list of links, and still not say which one is
current.

**Proposed shape.** Pandoc's `link_attributes` extension —
`[text](url){#id .class key=value}` — which is the standards-aligned
spelling, reuses the shared `{#id .class key=value}` parser, and generalises
past this one attribute to a class, a `rel`, or a `download`.

goldmark has no built-in for it, so it is either an inline parser or a
post-render transform matching an `<a>` followed by a literal `{…}` text
node. The transform is the better fit: it is how `Chips` already works, and
the transform layer sets attributes directly on the tree, where goldmark's
allowlist does not reach.

**Three constraints it must carry.**

Attribute names must go through the same validation as container attributes.
A name reaches the output unescaped, and `attrTokens` accepts any byte in a
key but whitespace and `=`; the fence-line-capture plan's `safeAttrName`
exists for exactly this and must be reused rather than reinvented.

`ExternalLinks` already sets `target` and `rel` on off-site links, so an
author-supplied `rel` needs a defined merge rather than whichever transform
runs last winning.

`LinkRewrite` keys on the href exactly as written in the source, so the
interaction with a trailing attribute block needs checking — the block must
not become part of the key.

**Explicitly out of scope:** md2html deriving `aria-current` itself. It
converts one document at a time with no notion of which page is "current",
and a navigation block is hand-written per page. The author is the only party
who knows.

**Degrades to:** a literal `{…}` after the link text, visible in the rendered
body. That is the same failure bracketed spans had before they were
implemented, and the same argument for implementing it: the degradation is
visible rather than silent, but it is still wrong on the page.
