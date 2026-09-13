# Structured diagram fence — Design

**Date:** 2026-09-12
**Author:** AdamF-G
**Module:** `github.com/AdamF-G/md2html`
**Status:** Proposed

## Purpose

Implements item 1 of
[the extended content model backlog](./2026-09-11-extended-content-model.md),
the last of its nine items still outstanding.

The only diagramming path today is a `mermaid` fence, which lays a graph out
algorithmically. Figures that are *hand-laid-out* — side-by-side panels, a
vertical flow of labeled boxes, parallel lanes, a row of stat tiles — have no
path into mermaid that preserves the layout. This adds a second fenced
language, `fig`, whose body is a small structured document describing a
figure as a tree of typed items, rendered to plain HTML and CSS.

## Scope

**In:** the `fig` fence language, its full item vocabulary, the renderer that
emits its markup, and the stylesheet component set that styles it.

**Out:**

- Any diagram whose layout should be *computed*. That is mermaid's job and
  stays mermaid's job.
- User-defined item kinds. The vocabulary is fixed and shipped, consistent
  with the backlog's standing position that shipped features are closed
  vocabularies and `Transform` is the extension point for anything further.
- Images inside figures. A figure is boxes, labels and connectors; an
  illustration is an `![]()` in body text.
- Any JavaScript. Figures are markup and CSS, with no runtime, so nothing
  here touches the click-to-expand runtimes in `page.go`.

## Architecture

### Where it hooks

The backlog proposed handling this "at the goldmark-extension layer, before
the tree reparse". Since it was written, `codefence.go` landed for code
captions, and it already owns exactly that seam: `codeFenceRenderer`
(`codefence.go:100`) replaces goldmark's fenced-code-block rendering
wholesale, registered at priority 100 in `newParser` so it outranks
goldmark's own 1000. It already resolves the language through
`splitFenceInfo`.

So `fig` needs no new goldmark extension — it is a branch inside a renderer
this repo already owns:

```
codeFenceRenderer.render
  splitFenceInfo(info) -> lang, caption
    lang == "fig" -> renderFig(lines, warn) -> figure markup, written direct
    otherwise     -> today's <pre><code> path, unchanged
```

Mermaid is unaffected for the same reason captions do not disturb it: its
extension rewrites its fences into its own AST node during parsing, so a
mermaid fence never reaches this function.

### Why the renderer, not a transform

Writing markup straight to the `util.BufWriter` keeps figure markup out of
goldmark's HTML-escaping pass entirely. A transform walking the parsed tree
could recover the fence's raw text from inside `<pre><code
class="language-fig">`, but would then have to undo the escaping goldmark
had already applied to it.

The reparse is what makes this cheap. `Convert` hands the rendered HTML to
`parseFragment` and *then* runs transforms, so a figure's text is ordinary
tree nodes by the time anything walks the document:

| Transform | What it does inside a figure |
|---|---|
| `Chips` | `[proven]` in a box label becomes a chip |
| `SectionLinks` | `§4.2` in a figure resolves like any prose reference |
| `LinkRewrite` | a `.md` link in a label is rewritten to its `.html` target |
| `ExternalLinks` | an off-site link in a label gets `target`/`rel` |

`ExtractLinks` sees figure links too, so a document linked only from inside
a figure is still crawled. None of this needs figure-specific code in any
transform, and `inlineSkip` (`inline.go:19`) already does the right thing
with a code span inside a label.

Figures also survive `--fragment` untouched: the markup is self-contained
and the stylesheet travels with the fragment, so unlike mermaid — which
depends on the host rendering it natively — a figure looks the same in an
Artifact as on a standalone page.

Two consequences worth stating, because they differ from every other feature
in the backlog:

- A caller who supplies their own `Options.Transforms` still gets figures.
  Figure rendering is not a transform, so it cannot be switched off by
  replacing the transform list — the same way front matter cannot.
- A figure's warnings always reach `Options.Warn`, for that same reason.
  `Warn`'s doc comment currently divides warnings into "Convert raises it
  itself, so it always arrives" and "reaches transforms only through the
  default list". Figure warnings join the first group, and that comment
  needs updating to say so.

### Layout

```
fig.go                 the fence branch, the YAML model, validation
figrender.go           per-kind HTML emitters, the inline-Markdown pass
default.css            the fig component block
fig_test.go            model, validation, degradation
figrender_test.go      per-kind golden markup, inline rendering
testdata/figures.md    the rendered gallery, every kind and layout
testdata/conformance.md  gains fig coverage
e2e_browser_test.go    gains one narrow-width stacking test
```

Two files rather than one: nine item kinds and three layouts in a single
file lands near 500 lines, and the parse/validate half shares only the model
type with the emit half. Each half ends up near `container.go`'s size, which
is this repo's established ceiling for a feature module.

## The fence language

### Body format

The body is YAML, decoded with `yaml.Decoder` and `KnownFields(true)`.

This adds `gopkg.in/yaml.v3` as the first new *runtime* dependency since the
original design — `chromedp` is test-only, and Go has no stdlib YAML, so the
backlog's aside that YAML is "dependency-free" does not hold. The dependency
buys nested trees, lists, quoting and block scalars, which are the parts of
a hand-laid-out figure's description that are genuinely unpleasant to
hand-parse. `frontmatter.go` hand-parses flat `key: value` lines and that
remains the right call for flat data; a layout tree is not flat data.

`KnownFields(true)` is the point of using a real decoder: a misspelled key
is an error that degrades visibly rather than an item that silently vanishes.

### Document shape

```yaml
caption: Request path          # optional, inline Markdown
layout: rows                   # rows | cols | split; default rows
items:
  - box: Client
  - arrow: HTTP POST
  - group: Server
    items:
      - box: "`auth` middleware"
      - result: 200 OK
```

Each item is a map with exactly one *kind key*, plus that kind's modifiers.
Decoding targets one `figItem` struct carrying every kind's field; validation
then asserts exactly one kind key is set. This keeps `KnownFields` useful —
it rejects keys no kind defines — while validation catches the case it
cannot see, an item that names two kinds or none.

### Vocabulary

Leaves:

| Key | Value | Renders as |
|---|---|---|
| `box` | label | `<div class="fig-box">` |
| `arrow` | optional label | `<div class="fig-arrow">` |
| `result` | label | `<div class="fig-result">`, an emphasized bar |
| `rail` | label | `<div class="fig-rail">`, a full-width accent rail |
| `stats` | list of `{value, label}` | `fig-stats` > `fig-stat` |
| `defs` | list of `{term, def}` | a real `<dl class="fig-defs">` |

Groups. `group` is the one that takes a title, so its content hangs off a
separate `items` key; `chain` and `lanes` have no title and carry their
content directly as the kind key's value:

| Key | Value | Content | Renders as |
|---|---|---|---|
| `group` | title | nested `items` | `fig-group` with a title line |
| `chain` | list of items | the value itself | `fig-chain`, steps with connectors between |
| `lanes` | list of lists of items | the value itself | `fig-lanes` > `fig-lane` |

Panel-level `layout` values, set once at the top level:

| Value | Meaning |
|---|---|
| `rows` | items stacked vertically; the default |
| `cols` | each top-level item is a panel, side by side |
| `split` | exactly two top-level items, with a labeled boundary between |

Under `cols`, a top-level item may carry `weight`. Anywhere else it is
rejected by validation rather than by `KnownFields`: one struct carries
every kind's fields, so the decoder cannot tell where a key is legal — only
validation knows the item's position. A panel that needs a heading is a
`group`; there is no separate panel kind.

Under `split`, the label between the two panels is a top-level `boundary`
key, not a third item:

```yaml
layout: split
boundary: before / after
items:
  - group: Before
    items: [{box: Handler}]
  - group: After
    items: [{box: Handler}, {box: Cache}]
```

Arrow direction follows the enclosing layout — down in `rows`, right in
`cols` — rather than being author-specified. Every figure that has ever
wanted an up-arrow wanted a different layout instead, and one fewer
per-figure decision is one fewer way to draw something incoherent.

`weight` is clamped to 1–12 and emitted as `style="--fig-weight:3"`, a
custom property the stylesheet consumes. A raw `flex-grow` in an inline
style would be unoverridable by a caller's `--css` replacement; a custom
property is a value the stylesheet can reinterpret.

### Inline Markdown in text fields

Every author-facing text field — captions, labels, titles, terms,
definitions, boundary labels — renders through a real goldmark inline pass,
not `html.EscapeString`. So `` `auth` middleware `` is a code span and
`[handler](./handler.md)` is a link that the crawler will follow and rewrite.

One inline renderer is built per figure, not per field and not once per
package. Per field would rebuild it tens of times for one figure; a package
global would raise a question about sharing a parser across the CLI's
parallel document conversions that a per-figure instance simply does not
have. The pass renders the field and strips the single wrapping `<p>`.

Everything that is *not* an author text field — kind names, weights — is
generated by this code and needs no escaping, because it never comes from
the document.

## Error handling

Nothing here can fail a conversion. Three outcomes:

| Condition | Result |
|---|---|
| YAML syntax error, unknown key, or an item naming zero or two kinds | one `Warn` locating the fault; the fence emits as `<pre><code class="language-fig">` with its raw text |
| Empty text on an otherwise valid item | renders as an empty box, no warning — a blank spacer is legitimate |
| A valid figure | figure markup |

Where a fault is *located* differs by tier, because the two tiers know
different things. A YAML syntax or unknown-key error carries `yaml.v3`'s own
line number, so the warning quotes it. A validation error has no line —
`KnownFields` forbids the custom unmarshaler that would retain one — so it
names the item's path instead (`items[2].chain[0]`). Both point at one item;
only one can say which line it started on.

An item whose kind key has a null value (`box:` with nothing after it) reads
as *no* kind, not an empty one, since a nil pointer is how an absent key
arrives. Its warning says so and names the fix: an intentionally blank box
is `box: ""`.

The degradation is the one the backlog documented: the author sees their own
source, unstyled but intact, rather than a blank space or broken markup. It
is also the same shape as `Containers`' unknown-kind warning, which the
authoring guide already teaches readers to expect.

::: warning
`newParser()` takes no arguments today, and builds `codeFenceRenderer`
inside itself. Threading the warning sink through makes it
`newParser(warn func(string))`, with `Convert` passing `opt.Warn`. That is a
signature change to the file every conversion goes through — small, but it
is existing code this design must touch, not new code it adds beside it.
:::

## Stylesheet

A `fig` component block appended to `default.css`, built only from the
existing tokens (`--accent`, `--rule`, `--ok`, `--warn`, `--muted`,
`--code-bg`), so it inherits all four theme states — light, dark, and both
explicit `data-theme` overrides — without defining a single new color.

Narrow widths are the genuinely new part. `default.css` has no width media
query at all today; the page is one measure column. `fig-cols`, `fig-lanes`
and `fig-chain` all need one, stacking to a single column and ignoring
weights when stacked:

```
rows          cols (weights 2/1)      split
+--------+    +----------+-----+     +-----+ before +-----+
| Client |    |  Panel A |  B  |     |  A  |--------|  B  |
+---+----+    |          |     |     +-----+        +-----+
    | POST    +----------+-----+
+---v----+    narrow: A over B       narrow: A over B,
| Server |                            label between
+--------+
```

Reduced motion needs no new handling: nothing here animates, and the
existing `prefers-reduced-motion` block stays as it is.

## Documenting the syntax

A `fig` fence in a Markdown document *renders*, which is a problem unique to
this feature among everything in the backlog: `docs/authoring.md` cannot
show `fig` syntax in a `fig` fence, or the examples become the figures they
were meant to explain. Examples are written in `yaml` fences instead — as
they are in this document — which shows the body exactly as authored and
costs nothing, since the body genuinely is YAML.

## Testing

TDD throughout, as everywhere else in this repo.

### Unit and integration

- `fig_test.go` — the model and its failure modes: a valid document of each
  shape, an unknown key rejected by `KnownFields`, an item naming two kinds,
  an item naming none, a YAML syntax error. Each failing case asserts both
  the warning text and the code-block fallback, not just that something went
  wrong.
- `figrender_test.go` — golden markup per kind, the inline-Markdown pass,
  weight clamping at both ends, and arrow direction following the layout.
- A full-`Convert` test proving chips, `§` references and `.md` link
  rewriting all reach inside a figure, and that `ExtractLinks` returns a
  link that appears only in a figure label.

### Why markup assertions are not sufficient here

Every test above verifies *markup*. Nine item kinds and three layouts can
emit perfectly correct markup and still render as an incoherent figure —
lanes collapsing to a vertical pile, a chain's connectors landing in the
wrong gaps, weights failing to divide a row. The assertions all pass and
the output looks broken.

That failure mode barely exists for chips or containers, whose whole
contract *is* the markup. It is the dominant one for a figure that is
hand-laid-out by definition, so this feature gets two things nothing else
in the backlog needed: a rendered corpus, and one layout test in a real
browser.

::: warning
The authoring docs cannot double as that corpus. Per "Documenting the
syntax" above, their examples live in `yaml` fences precisely so they do
*not* render. The corpus has to be a separate fixture that does.
:::

### The corpus

- `testdata/conformance.md` gains `fig` coverage, with matching entries in
  `conformanceChecks` (`md2html_test.go:18`). That file is described in its
  own test as the contract for the pipeline, and a fence language that
  survives conversion belongs in it.
- `testdata/figures.md` is the gallery: every kind, every layout, nesting,
  and the degradation cases side by side in one document. It backs a golden
  test, but its real job is being the page a maintainer opens in a browser
  after touching the stylesheet. It is built by this repo's own tool, so it
  never drifts from what the tool actually emits.

### The one browser test

`e2e_browser_test.go` gains a single `fig` test, under the existing
`e2e_browser` build tag.

This is the one place this design deliberately departs from the rule that
earned that file: the browser suite exists because mermaid and media-expand
ship JavaScript whose behavior cannot be read off the HTML, and this
feature ships none. But narrow-width stacking is *behavior*, not taste, and
it is the only media query in the whole stylesheet — markup tests cannot
observe it even in principle. The harness already asserts on
`getBoundingClientRect()` (`e2e_browser_test.go:409`, `:868`), so the test
emulates a phone viewport and checks that two `cols` panels share a row
when wide and stack when narrow. One test, guarding the one thing nothing
else can see.

### Documentation

`docs/authoring.md` gains a "Structured figures" section, and the
`md2html-authoring` skill's quick-reference table gains a row.

## Alternatives considered

- **A tree transform over `<pre><code class="language-fig">` instead of a
  renderer branch.** Viable — the raw text survives into the tree — but it
  has to unpick goldmark's escaping of text it is about to re-emit as
  markup, and it would be switchable off by a caller who replaces
  `Options.Transforms`, which is not a property a fence language should have.
- **A hand-rolled indentation format, to avoid the YAML dependency.** The
  cost lands in the wrong place: a nested layout tree needs indentation
  handling, lists, and quoting, which is precisely the surface where a
  hand-rolled parser diverges from what authors expect text that looks like
  YAML to do.
- **`encoding/json`.** Zero dependencies and an unambiguous grammar, but
  quoted keys, braces, commas, no comments and no clean multi-line strings
  make it hostile to hand-author inside a fence, which is the only way a
  figure is ever written.
- **Author-specified arrow directions.** Rejected as an invitation to draw
  an incoherent figure; direction follows layout.

## Out of scope

Computed graph layout, user-defined kinds, images inside figures, any
runtime JavaScript, and cross-document navigation of any kind.
