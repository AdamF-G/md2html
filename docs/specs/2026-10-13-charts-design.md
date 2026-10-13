# Charts — Design

**Date:** 2026-10-13
**Author:** AdamF-G
**Status:** Draft for review

## Purpose

Let a page carry data charts (dot plots, bars, lines, intervals on an axis)
written as data in a fence, the way a page carries a mermaid diagram, and
render them **when md2html builds the page**, as static SVG that follows the
page's style and theme. The page gains no script and fetches nothing new:
`mermaid is the only thing a page can fetch` and `a fragment has no script`
both stay true.

The chart grammar is Vega-Lite. md2html does not invent one: it runs Vega
and Vega-Lite inside an embedded JavaScript engine, gives them the style's
colours and the style's real font metrics, and post-processes the SVG they
produce into markup that themes itself and carries a hover layer built from
CSS.

````markdown
```vega-lite {caption="Inside-window ratio, against the baseline median"}
data:
  values:
    - {label: Baseline, v: 1.00}
    - {label: Cap at 300 W, v: 1.20}
    - {label: Cap at 200 W, v: 1.98}
mark: point
encoding:
  y: {field: label, type: nominal, sort: null}
  x: {field: v, type: quantitative, scale: {zero: false}}
  tooltip: {field: v}
```
````

## How we got here

Recorded so the decisions do not get relitigated. **Decided** means the
project owner chose it; **recommended** means it is this design's call,
with its reason, and can be struck in review.

| # | Decision | By | Reason |
|---|---|---|---|
| 1 | Charts are a general md2html feature, not one page's needs | Decided | Any document benefits; the work is the same |
| 2 | Bind an existing charting library rather than build native chart kinds | Decided | Scales, ticks, label layout and SVG geometry in Go is a large surface that is not md2html's job |
| 3 | Vega-Lite, not ECharts, Plotly, Observable Plot or Chart.js | Recommended | The only candidate whose charts are pure data with a complete theme surface; ECharts needed JavaScript functions for a lollipop and a timeline; Observable Plot specs are code |
| 4 | Render at build time, not with a script on the page | Decided | No new script in pages or Artifacts; the spike showed the output matches the runtime's |
| 5 | Embed SVG, not PNG | Recommended | One copy themes itself, uses the page's fonts, scales, prints, and is about a fifth of the size |
| 6 | Run Vega in an embedded pure-Go engine (goja), not by shelling out to Node | Decided | md2html stays one binary that `go install` produces |
| 7 | Build time only; no runtime chart path | Decided | One output and one code path; runtime-only capabilities are out of scope (below) |
| 8 | Hover tooltips from a CSS layer; no native `<title>` tooltips | Decided | Native titles were plain, slow and invisible on touch; the CSS layer matched the runtime's look |
| 9 | Each style has its own chart defaults; `default` and `claude-artifact` differ | Decided | `claude-artifact` follows the dataviz skill's reference palette; `default` keeps its own identity |
| 10 | v1 scope: smarter tooltip placement, nearest-point hover, keyboard focus, legend-hover highlighting | Decided | Closes the dataviz hover rule for dots and bars at low cost |
| 11 | Line crosshairs and a second narrow layout are follow-ups | Decided | Each has a size cost and needs its own design |
| 12 | A table view under each chart | Recommended | The dataviz skill requires one, and it is the fallback when the hover layer cannot run |
| 13 | Data inline and from local files beside the page | Decided | Small amounts of data are easier inline; larger sets live in CSV or JSON files, read inside the entry tree as assets are |
| 14 | The binary may grow by several MB; no build tag to drop charts | Decided | Size is not a concern for md2html's users |
| 15 | A style whose text is the system sans keeps chart text in it, measured with a proxy face and padded | Decided | The dataviz skill sets chart text in the system sans, and artifact-design wants the chart's face to agree with the page's; the reader's system font cannot be measured at build time |
| 16 | `default`'s chart palette is warm, led by its accent family | Decided | It must differ from `claude-artifact`'s (decision 9) and suit `default`'s warm neutrals |

## Scope

**In scope (v1):**

- A `vega-lite` fence whose body is a Vega-Lite spec in YAML or JSON.
- Inline data and local data files (CSV, TSV, JSON) resolved against the
  page.
- Chart tokens in every built-in style, and a placeholder scheme that turns
  each colour into `var(--token)`.
- Real text metrics from the style's own fonts.
- A hover layer: tooltip boxes, nearest-point hover regions, keyboard
  focus, legend-hover highlighting.
- A collapsed table view of each chart's data.
- Build warnings, and the fence falling back to a code block, on any fault.

**Out of scope:** reasons to reach for a runtime chart library, should a
need ever appear.

- Cursor-following tooltips. A tooltip sits at one place per mark.
- Zoom, pan, brushing, click-to-filter, sliders, linked views, and toggling
  a series by clicking its legend entry.
- Live or network data. Data is fixed when the page is built.
- Re-layout at the reader's width beyond what the follow-up narrow layout
  provides.
- Raw Vega specs (the lower-level grammar). Vega-Lite only.

**Follow-ups** (each its own design):

- Crosshair bands for line and area charts: one invisible band per x value,
  each with a rule and a tooltip. Roughly 300 bytes per band, so it needs a
  cap or bucketing for dense series.
- A second, narrow layout switched by a media query, so text stays full size
  on a phone. Roughly doubles a chart's size.
- Continuous colour scales that theme themselves (see Theming).

## Authoring: the `vega-lite` fence

- The fence is spelled ```` ```vega-lite ````. Its body is a Vega-Lite spec.
  YAML is accepted, and JSON is YAML, so either works.
- **A caption** goes in the info string, as on a code block:
  ```` ```vega-lite {caption="…"} ````. The body belongs to Vega-Lite's
  schema, so md2html adds no keys of its own to it. A spec's own `title`
  still renders inside the chart.
- **Data** is `data: {values: […]}`, or `data: {url: "./results.csv"}` naming
  a file. A relative URL resolves against the page's source directory and
  must stay inside the entry tree, exactly as an asset link does. A URL with
  a scheme or host is refused: md2html never fetches.
- **Width.** A chart is laid out at the style's chart width (see Text
  metrics) unless the spec sets `width`. Its SVG scales down with its
  column; it never makes the page scroll sideways.
- **Faults** follow the `fig` rule: a spec that does not parse, compile or
  render is a build warning naming the fence and the error, and the fence
  renders as a code block so the author sees their own source. Vega-Lite's
  own messages can be cryptic (the spike hit "Cannot read properties of
  undefined (reading 'type')" for one legal-looking encoding), so the
  warning also names the top-level key being compiled when it can.
- A compile **warning** from Vega-Lite (for example, about an encoding it
  will ignore) is passed through as an md2html warning.

## Rendering pipeline

1. **Parse** the fence body as YAML into a JSON value.
2. **Resolve data.** For each `url`, read the file inside the entry tree and
   hand its text to Vega's loader, which md2html replaces with one that
   serves only those reads.
3. **Theme.** Merge the style's chart config (below) under the spec's own
   `config`, so an author's explicit choice wins.
4. **Compile and render** inside the engine: Vega-Lite compiles to Vega,
   Vega renders headless with `renderer: "none"`, and `toSVG()` produces the
   SVG. The same pass walks the scenegraph and returns every mark that
   carries a tooltip, with its absolute bounds and its legend value.
5. **Post-process** in Go: placeholder colours become tokens, ids are
   namespaced, and the hover layer, focus targets and table view are added.

**The engine.** goja, a pure-Go ECMAScript engine, with Vega and Vega-Lite's
minified bundles embedded in the binary at pinned versions (the spike used
Vega 5.30.0 and Vega-Lite 5.21.0). One engine is created lazily, on the first
chart of a run, and reused for every chart after it. It needs three shims:
`structuredClone` as a JSON round trip, `setTimeout` driven by Go, and the
text measurer below. It has no file system, network or host access beyond
what md2html hands it.

**Determinism.** The same input produces byte-identical output, so pages
diff cleanly and golden tests hold. `Math.random` is replaced with a seeded
generator, since jitter and some transforms draw from it.

**Limits.** A chart that runs longer than a fixed budget (a few seconds) is
interrupted through goja's `Interrupt`, and the fence falls back with a
warning, so one bad spec cannot hang a build.

**Licences.** Every component is permissive and compatible with md2html's
MIT licence; nothing in the chain is copyleft. Checked 2026-10-13:

| Component | Licence |
|---|---|
| Vega, Vega-Lite and the `vega-*` modules bundled in them | BSD-3-Clause (University of Washington Interactive Data Lab) |
| `d3-*`, `delaunator`, `topojson-client`, `internmap` (bundled) | ISC |
| `json-stringify-pretty-compact` (bundled in Vega-Lite) | MIT |
| `robust-predicates` (bundled) | Unlicense |
| goja (parts derived from otto) | MIT |
| `dlclark/regexp2` | MIT |
| `go-sourcemap/sourcemap` | BSD-2-Clause |
| `google/pprof/profile`, the only part of pprof linked | Apache-2.0, no `NOTICE` file |
| `golang.org/x/image`, `golang.org/x/text` | BSD-3-Clause |

The minified bundles carry no licence headers, so md2html ships the notices
itself. A `THIRD_PARTY_NOTICES` file holds every licence above. It lives in
the repository beside the vendored bundles, and the binary prints it with
`--licenses`, as `claude-artifact` already carries its font licences. BSD-3
also forbids using the copyright holders' names to endorse md2html, so the
guide credits Vega without implying endorsement. The Node-only packages in
Vega's install tree (node-fetch, yargs and others) are not in the bundles
and need no notice.

**Pinning.** The bundles are vendored at exact releases. goja has no tagged
releases, so `go.mod` pins it to a commit pseudo-version, which `go.sum`
verifies. Any commit that passes the golden tests will do; its age does not
matter. Moving either is the same step: update, re-run the golden tests, and
review any diff in the golden SVGs.

## Theming

**Chart tokens.** Every built-in style defines them in the same three-block
theme shape as its other colours, and on `:root[data-theme="light"]` too:

| Token | Use |
|---|---|
| `--series-1` … `--series-8` | Categorical colours, in a fixed order |
| `--status-good`, `--status-warning`, `--status-serious`, `--status-critical` | Status meanings only, never a series |
| `--chart-surface` | The surface a chart sits on (rings and gaps between marks) |
| `--chart-grid`, `--chart-axis` | Hairline gridlines and the axis or baseline |

Text in charts uses the page's existing `--fg` and `--muted`, never a series
colour.

**Each style chooses its own values** (decision 9). `claude-artifact` takes the
dataviz skill's reference instance. The spike validated it on
`claude-artifact`'s surfaces: every hard gate passes in both modes. On the
light page surface, four slots are under 3:1 contrast, which the table view
and direct labels answer. `default` gets its own warm palette, led by its accent family
(decision 16), in a fixed order chosen among orders that pass the
validator on `default`'s light and dark surfaces. The plan proposes
candidates rendered side by side, and the owner picks. A
third-party style may define the tokens; one that does not inherits
`default`'s.

**Placeholders.** md2html renders every chart with one placeholder colour
per token, then rewrites each into `style="fill: var(--token)"`. CSS
variables are not valid in SVG presentation attributes, so the attribute
form will not do. One SVG then follows light, dark and the theme toggle with
no script. The spike rewrote all of them, and the check that no placeholder
survives is part of the post-process.

**Colours that are not tokens.** A spec that names literal colours, or uses
a continuous colour scale (Vega interpolates new colours between ramp
steps), produces colours md2html cannot map. Such a chart is rendered twice,
once with each theme's resolved token values, and the two SVGs are switched
by the same media query and `data-theme` rules the stylesheet uses. It costs
double the size and loses nothing else. Continuous scales that theme
themselves are a follow-up.

**Lighter steps of a colour** come from opacity, not a mixed colour, so they
stay tokens. A legend does not show opacity, so a chart that tells its
categories apart only by opacity misleads. The guide says to use distinct
series colours, as the dataviz skill does.

## Text metrics

Vega lays out axes, legends and labels by measuring text. Headless, it falls
back to a fixed width per character. For IBM Plex Sans at 12 px that
estimate is about 50 px too wide for every label column the spike drew
(159 px against 108). md2html replaces Vega's measurer with one that reads
the real advance widths of the face the chart's text will render in. In the
spike this matched the browser's own layout to within 1 px on every chart.

**Where the face comes from.** A style's manifest gains an optional
`charts` block:

```json
"styles": {
  "claude-artifact": {
    "css": ["artifact.css"],
    "charts": { "font": "fonts/ibm-plex-sans-400-600.woff2", "width": 640 }
  }
}
```

- `font` names a font file in the plugin, usually one the stylesheet already
  embeds. Charts render with the CSS font stack the style sets, so the
  metrics and the face agree.
- `width` is the plot width in CSS pixels that charts are laid out at.

**Reading the face.** The style fonts are woff2. Go's standard font package
reads TrueType and OpenType but not woff2, so md2html needs a woff2 decoder,
brotli included. A pure-Go dependency is preferred, and the plan confirms
one. Only `cmap`, `hmtx` and kerning are needed, not outlines. Variable
fonts need the `wght` axis applied to advances through `HVAR`. The spike
used the default instance and scaled heavier text by a constant, which is
where its 1 px error came from.

**A style without an embedded face** (decision 15). `default` renders in
the reader's system font, which md2html cannot know when it builds the
page. Its charts keep that same system sans, as the dataviz skill's
reference does, so the chart and the page agree. Layout is measured with a
proxy: the advance widths of a freely licensed face close in width to the
common system UI faces. Inter (OFL) is the candidate, being close to San
Francisco. md2html embeds only the widths it needs and never serves the
face to readers. Label columns and tooltip boxes are padded by about 10 %,
so a wider system face (Segoe UI, a Linux sans) still fits. A style's
`charts` block names the proxy through `metrics` when its stack differs:

```json
"charts": { "metrics": "builtin:system-sans", "pad": 0.1, "width": 640 }
```

Where a style embeds its face (`font`), there is no padding, because the
widths are exact.

## Interaction layer

All of it is markup and CSS that md2html writes. None of it is script.

**Tooltip boxes.** Every mark with a `tooltip` encoding gets one box: a
rounded rectangle in `--chart-surface` with a `--rule` hairline and `--fg`
text in the chart's face. It is sized with the same metrics, so it fits its
text. All boxes are drawn last in the SVG, above every mark, since SVG has
no z-index, and a rule shows each one while its mark is hovered:

```css
.md2html-chart:has(#c3-m7:hover, #c3-m7:focus) #c3-t7 { opacity: 1 }
```

The layer needs `:has()`, available in every major browser since late 2023.
Without it no box ever shows and the chart is otherwise unchanged.

**Placement** (decision 10). A box goes above its mark, centred on the mark.
Where there is no room it flips below, and it shifts sideways to stay inside
the chart. For a mark wider than its box (a long interval, a wide bar) the
box sits at the mark's centre, not where the pointer is: that is the one
property a script-free layer cannot have (out of scope).

**Nearest-point hover.** For point, line and area marks, md2html asks Vega
for the Voronoi cells of the mark centres and writes them as invisible hover
regions above the marks. Hovering anywhere in a cell shows that point's box,
so a 10 px dot is not a 10 px target. Bars and other area marks are their
own targets.

**Keyboard.** Each tooltip target gets `tabindex="0"`, and `:focus` shows its
box as `:hover` does, with a visible focus ring. Above 50 marks a chart gets
one tab stop for the whole chart instead, which leads to the table view, so
a dense chart is not hundreds of tab stops.

**Legend-hover highlighting.** Hovering or focusing a legend entry dims
every mark of the other categories:

```css
.md2html-chart:has(.md2html-legend-s2:hover) .md2html-mark:not(.md2html-s2) { opacity: .25 }
```

**Accessibility.** Marks keep Vega's `aria-label`s. The boxes are
`aria-hidden`, since they repeat those labels.

## Table view

Under each chart, a collapsed `<details class="container">` titled with the
chart's caption (or "Chart data") holds a table of the data the chart drew:
one row per datum, one column per field the spec encodes, in encoding order,
formatted as the axis formats them. It answers the dataviz skill's table
requirement and its contrast relief rule. It is the fallback where `:has()`
is missing, and it is readable with no SVG support at all.

## Output shape

```html
<figure class="md2html-chart">
  <svg …>…marks…<g class="md2html-tips" aria-hidden="true">…</g></svg>
  <figcaption>…</figcaption>
  <details class="container"><summary>Chart data</summary><table>…</table></details>
</figure>
```

- **Ids are namespaced** per chart (`c3-m7`), since a page holds many charts.
  Any id Vega emits, such as a clip path, is rewritten with the chart's
  prefix.
- **Size.** Measured in the spike, before any minification: 8 to 10 KB for a
  dot plot and about 30 KB for a 28-interval timeline as plain SVG. The hover
  layer adds 1.5 to 10 KB, and those figures cover every theme. SVG gzips to
  about a tenth.
- **Fragments are unchanged.** A chart is static markup, so `--fragment`
  output carries it the same way and still has no script.
- **The stylesheet** gains the `md2html-chart` rules: the hover layer,
  the focus ring, legend dimming and the table view.

## Library API and CLI

- Charts render by default, as `fig` figures do. `--no-charts` (Options:
  `NoCharts`, following `NoSource`) leaves a `vega-lite` fence as a code
  block, for a caller who does not want the engine started.
- `--licenses` prints `THIRD_PARTY_NOTICES` and exits.
- No option names the Vega build. Unlike mermaid's, it is embedded, not
  fetched, so there is nothing to point elsewhere.

## Costs

- **Binary size.** Stripped, md2html is 4.7 MB today. goja and the two
  bundles add about 9 MB, so the binary grows to roughly 13 to 14 MB.
  Accepted (decision 14); the plan records the exact figure.
- **Build time.** The engine starts in about 55 ms, once per run and only
  when a page has a chart. Each chart then takes 30 to 50 ms. A page with no
  chart costs nothing.

## Testing

- **Golden SVGs** for each chart form (dot, bar, line, interval, legend) in
  both metrics modes, byte-identical across runs.
- **Metrics:** a browser test lays out each golden chart at runtime with the
  real font and compares its plot offset and width with md2html's, within
  1 px.
- **Theme:** a browser test confirms every colour in a rendered chart
  resolves to its token in light, dark and both toggle states, and that no
  placeholder survives.
- **Hover layer:** a browser test drives a real pointer (CSS `:hover` ignores
  synthetic events) over a dot, a bar and a Voronoi cell, and asserts the
  right box shows. Keyboard focus does the same, and so does legend hover.
- **Faults:** a bad spec, a spec whose compile throws, a timeout, a URL with a
  scheme, a file outside the entry tree. Each gives one warning and a code
  block.
- **Fragments** carry charts with no script.
- **Size budget:** a golden chart's output stays under a recorded size.
- **Notices:** `THIRD_PARTY_NOTICES` names every module the bundles and the
  linked Go modules contain, and `--licenses` prints it whole.

## Documentation

A `charts` section in the authoring guide: the fence, data sources, the
caption, widths, the table view, and what hover can and cannot do. New
silent traps where they apply: a chart that tells categories apart only by
opacity shows a misleading legend, and an author's literal colours double
the chart's size. A `--guide styles` addition covers the `charts` manifest
block and the chart tokens.

## Alternatives considered

- **Native chart kinds in `fig`.** Rejected (decision 2): the scale, tick,
  label and layout work is the bulk of a charting library.
- **ECharts.** Rendered the spike's charts as well as Vega-Lite, but its
  lollipop and timeline needed `renderItem` functions and its labels needed
  formatter functions. A fence cannot carry JavaScript.
- **Plotly** (1.34 MB gzip, or 357 KB for its basic build). Its mode bar and
  hover chrome look foreign to the Artifact style. **Observable Plot**: specs
  are JavaScript. **Chart.js**: canvas only, so it cannot follow CSS.
- **Vega at runtime in the page.** The dataviz skill's ideal, with
  cursor-following tooltips. Rejected (decisions 4 and 7) for the script it
  puts in every page and Artifact, 277 KB gzip, and for the guarantees it
  would end.
- **PNG at build time.** 30 to 51 KB per chart in base64 at 2x, needed once
  per theme, not text, and needing the face installed in the rasteriser.
- **Node at build time.** Proven, but it ends md2html being one binary.
- **QuickJS, transpiled to Go.** Viable, and larger than goja. goja ran
  every spec the spike tried.
- **Native `<title>` tooltips.** Rejected (decision 8).

## Open questions for review

None. Decisions 13, 15 and 16 settled the three raised in the first draft.

## Spike evidence

A throwaway spike on 2026-10-13 rendered three charts: a five-point dot plot
with a hollow reference marker, the same form on a negative axis, and a
28-interval timeline with an eight-entry legend. It rendered them four ways
on one page, with a theme toggle and a real pointer driving the hover
states.

| | Runtime Vega in the browser | Build time, estimated metrics | Build time, real metrics, CSS hover |
|---|---|---|---|
| Label column, dot plot | 108 px | 159 px | 109 px |
| Label column, timeline | 84 px | 130 px | 84 px |
| Hover | Styled, follows the cursor | Styled, fixed per mark | Styled, fixed per mark |
| Script on the page | 277 KB gzip | None | None |
| Follows the theme toggle | Re-renders | With no script | With no script |

Native `<title>` tooltips were dropped after the spike (decision 8). The
spike also found three faults of its own, each now a rule above. A literal
`<title>` in page text swallowed a section, so tooltip text is escaped.
Opacity-only categories gave a misleading legend, now a documented trap. A
replacement string `$1aria-label` was read as a group named `1aria` and
mangled every label, so the post-process does not use that construct.
