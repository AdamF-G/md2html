# Fast Tips — Design

**Date:** 2026-11-19
**Author:** AdamF-G
**Status:** Built, 2026-11-19.

## Purpose

A bracketed span keeps its `title` ([Span Attributes](2026-11-19-span-attributes-design.md)),
so a chip can carry a hover note. The browser's native tooltip shows that
note only after its own delay of about a second, and no CSS can shorten it.
An author who wants the note to appear when the pointer lands has to build
their own tooltip.

This note adds one: a `tip` written on a span or a link shows after a short
delay the author can set, and a plain `title` keeps the browser's tooltip.

## 1. An example {#example}

```markdown
---
tip-delay: 150ms
---
Ships in [beta]{.chip tip="May change before 1.0."}. The
[migration guide](migrate.md){tip="Covers 0.x to 1.0 only."} has the
details, and [stable]{.chip tip="No breaking changes." tip-delay="0ms"}
needs none.
```

Hovering or focusing "beta" shows "May change before 1.0." after 150ms.
"stable" shows its note at once. The link shows its note on hover, and when
a keyboard user reaches it with Tab.

## 2. Principles

- **Opt-in.** Nothing that exists today has a fast tip, so nothing changes
  unless the author writes `tip`. A `title` keeps the browser's own
  tooltip, and an author who wants that keeps it.
- **No script.** A fragment, which is what a Claude Artifact uses, carries
  no script, so the tip is HTML and CSS. It is the approach the charts'
  hover layer already takes: a focusable host, a tip shown on `:hover` and
  `:focus-visible`, and its fade turned off under reduced motion.
- **The note stays the reader's.** A screen reader hears it once, as the
  element's description. A keyboard user reaches it with Tab. Print shows
  it.
- **Configured where the page is configured.** The delay is a front matter
  key, read the way `style:` is: the shell's sets the site, a page's own
  wins for that page. It is not a CLI flag.

## 3. Writing a tip {#writing}

`tip="…"` in an attribute block:

- **A bracketed span:** `[beta]{.chip tip="May change before 1.0."}`.
- **A link:** `[guide](migrate.md){tip="Covers 0.x to 1.0 only."}`.

An image's attribute block does not take one. An image has `alt`, and a tip
on an image inside a link would compete with the link's. A `tip` on an
image is dropped as any other unlisted name is.

`tip` is not an HTML attribute, so it never reaches the element as one.
The rest of the block applies as it does today, and `title` may sit beside
`tip`: the element then has both tooltips, the author's choice.

An empty `tip=""` makes no tip. It is not an error, since an author may
template the value.

### Per-element delay

`tip-delay="…"` beside a `tip` overrides the page's delay for that one
element: `[stable]{.chip tip="…" tip-delay="0ms"}`. Without a `tip` it does
nothing and warns, since it can only be a mistake.

## 4. The delay {#delay}

- **The front matter key:** `tip-delay:`, a CSS time. The shell's front
  matter sets it for every page of the site, and a page's own front matter
  wins for that page. A single page, or a fragment, sets it in its own
  front matter.
- **The default:** `100ms` where nothing sets one. At zero, tips flash as
  the pointer crosses a line of chips, and 100ms still reads as at once.
- **Precedence:** the element's `tip-delay`, then the page's front matter,
  then the shell's, then the default.
- **What a value is:** a number followed by `ms` or `s`, such as `0ms`,
  `150ms`, `0.2s` (`^[0-9]+(\.[0-9]+)?m?s$`). Any other value, on an
  element or in front matter, is ignored with a warning naming it, and the
  next level down applies. The value is written into CSS, so this check is
  what keeps a value like `0ms; background: url(…)` out of it.
- **Where it is written:** as a custom property, `--md2html-tip-delay`. The
  page's value goes on `main.md2html`, which is the element a fragment's
  stylesheet already scopes itself to, so a fragment never sets anything on
  the host's `:root`. An element's value goes in the `style` attribute of
  its tip's wrap: a custom property inherits only from an ancestor, and
  the tooltip is the host's sibling, so the host's own style would never
  reach it.

The delay applies to showing the tip. Hiding is immediate, so a tip never
lingers over the text below it.

## 5. What md2html writes {#markup}

For a span:

```html
<span class="md2html-tip-wrap"><span class="chip md2html-tip-host" tabindex="0" aria-describedby="md2html-tooltip-1">beta</span><span class="md2html-tooltip" id="md2html-tooltip-1" role="tooltip" aria-hidden="true">May change before 1.0.</span></span>
```

For a link, the same, except that the host is the `<a>`, which already
takes focus, so it gets no `tabindex`:

```html
<span class="md2html-tip-wrap"><a href="migrate.html" class="md2html-tip-host" aria-describedby="md2html-tooltip-2">guide</a><span class="md2html-tooltip" id="md2html-tooltip-2" role="tooltip" aria-hidden="true">Covers 0.x to 1.0 only.</span></span>
```

- **The tip sits next to the host, not inside it.** Inside a link, its
  text would become part of the link's spoken name.
- **A wrapper holds the two** (my call). It is a plain `<span>` with no
  role, so a screen reader hears nothing of it, and it is what lets CSS
  centre the tip over its host in every browser, as a positioned box.
  Without it, a browser lacking anchor positioning could place the tip
  above the host only from the host's end.
- **The class names are their own** (my call). The charts' tooltips
  already use `md2html-tip`, on SVG, and every rule here would hit them
  too. So the box is `md2html-tooltip`, the host `md2html-tip-host` and
  the wrapper `md2html-tip-wrap`, and nothing here touches the chart
  rules.
- **`aria-describedby` and `aria-hidden` together** make a screen reader
  announce the note once, as the description, and not again when it reads
  past the tip in the flow. A description may name a hidden element.
- **Ids** are `md2html-tooltip-N`, numbered from 1 in document order on
  each page, a prefix nothing else in md2html uses. A site shell's own
  tips can clash with a page's. The shell's ids are already renumbered on
  a clash (`renumberShellIDs`), and that renumbering must update
  `aria-describedby` as well as `href="#…"`.
- **A host's own `tip-delay`** becomes `style="--md2html-tip-delay: 0ms"`
  on the wrap, leaving any `style` the author wrote on the host alone.

### Where the markup comes from

A late transform writes it. Chips and LinkAttrs record the note on the
element, and a transform after HeadingAnchors, SectionLinks and TOC turns
it into the markup above. Those transforms then never see the note's text,
so a tipped chip in a heading changes neither its slug nor its line in the
contents list.

It also runs after a site's `linkDrop`, which unwraps a link it drops
(my call). A dropped link's tip is not lost: its text becomes a span
host, focusable, as a tipped span's is.

### A tipped span inside a link

`[[beta]{tip="…"}](x.md)` would put a focusable host inside the `<a>` and
the note into the link's name, which is what §5 avoids. Markdown cannot
make one: Chips leaves a link's text alone, so the span stays literal.
The tip pass still drops a tip it finds inside a link, without a word,
since no author input reaches it.

### Without the default stylesheet

A style with `"base": "none"`, or a caller's own `Options.CSS`, leaves out
`default.css`. Left unstyled, a tip would sit in the text as a second copy
of the note. So the rules that make a tip work are written into the page's
stylesheet whenever the page has a tip, whatever the style (my call):
hiding it, showing it on hover and focus after the delay, its placement
over the host, reduced motion and print. This is how a chart carries its
own hover rules. The look, colours, padding, radius and the `cursor: help`
cue, goes in the same rules, ahead of the page's style so a style can
restyle it. In `default.css` it would change every page, tip or not,
which the v0.12.0 output goldens forbid.

## 6. What the reader sees {#look}

- **The box:** above the host, centred on it, at most about 30ch wide,
  wrapping. Colours come from the chart tips' tokens, `--chart-surface`,
  `--rule` and `--fg`, so it matches charts in light and dark and follows
  any style that recolours them.
- **The cue:** a tipped span, chip or not, gets `cursor: help`. A link keeps
  its pointer. Nothing is underlined. A style can add an underline, or any
  other cue, through `.md2html-tip-host`, and restyle the box through
  `.md2html-tooltip`.
- **Touch:** a tap focuses a tipped span, which shows its tip. A tap on a
  link follows it, so a link's tip does not show on a touch screen. The
  guide says so.
- **Reduced motion:** the tip appears with no fade. The delay still
  applies, since it is a pause, not a motion.
- **Clipping:** inside a scrolling table, or another box that clips, the
  tip can be cut at the box's edge. Version one accepts this, and the guide
  says so. Keeping it inside the window would need script or a much larger
  CSS layer.
- **Print:** the tip prints in brackets after its host, "beta (May change
  before 1.0.)", in the body text's colour.

## 7. Where the note must not appear {#leaks}

The note is text in the page, so everything that reads the page's text
skips `.md2html-tooltip`:

- the site's search index, its text and its headings;
- a heading's slug and its contents-list entry (§5 already keeps it out);
- the page title, which is read from the source before any transform. A
  span's `tip` is part of its block, which the title already drops.

Copying the page's text as a reader sees it should not pick up a hidden
tip. A tip that is not shown is `display: none`, which browsers leave out
of a copy. It also takes no room: a `visibility: hidden` box still counts
toward the page's overflow, and near the end of a line on a phone it
scrolled the whole page sideways. The shown tip starts from hidden by
`@starting-style`, so it still waits out the delay; a browser without
`@starting-style` shows it at once.

## 8. Warnings {#warnings}

Each goes in `docs/guide/warnings.md`, as `TestEveryWarningIsInTheGuide`
requires:

- an invalid `tip-delay`, in front matter or on an element, naming the
  value and saying which delay applies instead;
- a `tip-delay` on an element without a `tip`.

## Approaches

- **A tip element written at build time (chosen).** The note is real text
  with an id, so a screen reader hears it as a description and print can
  show it. The cost is markup, ids, and keeping the note out of the search
  index and the copy.
- **CSS reading an attribute: `data-tip` shown by `::after { content:
  attr(data-tip) }`.** This is far less code, but text in `content:` is
  unreliable for screen readers and has no element for `aria-describedby`
  to name. Screen-reader users would lose the note a `title` gives them
  today.
- **A script.** It could keep the tip inside the window and handle touch on
  links. But a fragment carries no script, so tips would vanish in
  Artifacts.

For opting in, `tip="…"` was chosen over a `.tip` class beside a `title`.
With the class, `title` would mean a slow or a fast tooltip depending on
something else in the block, and a forgotten class would quietly give the
slow one.

## Decisions

- Opt-in through `tip="…"`. A `title` keeps the native tooltip.
- Spans and links both take it, and links accept that touch screens never
  show it.
- The delay is the front matter key `tip-delay:`, shell then page, with a
  per-element `tip-delay="…"` override.
- No underline, and a class hook for a style that wants one.
- CSS only, no script.

## Left to the builder

- Exact CSS: the box's padding, radius, offset and arrow, if any, and how
  "above, centred" is positioned. Flipping below the host near the top of
  the window is welcome where CSS can do it without script, such as anchor
  positioning's `position-try-fallbacks: flip-block` in the browsers that
  have it, as long as a browser without it still shows the tip above.
- Where in the transform list the tip transform sits, as long as it runs
  after HeadingAnchors, SectionLinks and TOC.
- How Chips and LinkAttrs hand the note to it: an internal attribute
  removed when the markup is written is the obvious way.
- The wording of the two warnings.
- The guide page the feature is documented on: `inline-marks.md` for
  spans and `links-and-images.md` for links, with front matter's table in
  `page-structure.md`.
