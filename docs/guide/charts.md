---
nav-order: 8
---
# Charts

[[toc]]

A ```` ```vega-lite ```` fence is a chart, drawn when md2html builds the
page. Its body is a Vega-Lite spec, in YAML or in JSON, which YAML also
reads. The [Vega-Lite documentation](https://vega.github.io/vega-lite/docs/)
describes the grammar: marks, encodings, scales and transforms.

````markdown
```vega-lite {caption="Median response time by region"}
data:
  values:
    - {region: North, ms: 182}
    - {region: South, ms: 240}
    - {region: East, ms: 205}
    - {region: West, ms: 167}
mark: bar
encoding:
  y: {field: region, type: nominal, sort: null}
  x: {field: ms, type: quantitative, title: Milliseconds}
  tooltip: {field: ms}
```
````

The chart is SVG in the page, in the style's colours and face, and follows
the reader's light or dark theme. The page gains no script and fetches
nothing for it, and `--fragment` output carries it the same way.

The caption goes in the info string, as on a code block, and sits under
the chart. Classes there, such as `{.wide caption="…"}`, go on the
chart's `<figure>` after its own `md2html-chart`. The body is Vega-Lite's
alone: md2html adds no keys to it, and a spec's own `title` still draws
inside the chart.

A spec that does not parse, compile or render warns, naming the fence by
its number on the page, and shows as a code block of its source; see
[Warnings](warnings.md).
`--no-charts` leaves every `vega-lite` fence as a code block, as does
`Options.NoCharts` in the library.

## Data

`data: {values: […]}` holds the data in the spec.
`data: {url: ./results.csv}` reads a CSV, TSV or JSON file when the page
is built, relative to the page's source. The file must stay inside the
run's base: for the command, the common ancestor of its entry points; in
the library, `Options.DataRoot`, or the page's own directory when that is
empty. A `Convert` call with no `SourcePath` reads no files.
A `url` with a scheme or host is refused: md2html never fetches. So is
one with a query or a fragment, such as `results.csv?v=2`, since a file
has neither. The data is fixed when the page is built, so rebuild the
page to update a chart.

Dates show the same on every machine that builds the page. There is no
time-zone setting; the rule is:

- A value with no zone shows exactly as written. `2024-01-01` is 1
  January, and `2024-01-01T09:30` is 09:30.
- A value that names an instant shows in UTC. `2024-01-01T09:30+02:00`
  shows as 07:30, and an epoch number, in milliseconds, as its UTC time.

An `image` mark draws only a `data:` URL. Any other `url` would make the
page fetch the image, so the chart is refused.

## Colours

A colour written as `token:<name>` is the style's colour for that role,
and follows the theme. `token:status-critical` marks a value as critical:

```yaml
mark: {type: bar, color: "token:status-critical"}
```

| Token | Use |
|---|---|
| `series-1` … `series-8` | Categories, in this order; a `color` encoding takes them unless its scale names others |
| `status-good`, `status-warning`, `status-serious`, `status-critical` | Status meanings only, never a series |
| `chart-surface` | The surface a chart sits on, which rings and gaps between marks show |
| `chart-grid`, `chart-axis` | Gridlines, and the axis or baseline |
| `fg`, `muted` | Chart text, as on the page |
| `accent`, `warn`, `ok`, `rule`, `bg`, `code-bg` | The page's own colours, so a mark can match the text around it |

A style can add tokens of its own, such as `token:inject`, by listing
them in its manifest; see [Chart settings](styles.md#chart-settings). A name outside
these warns. `token:` is read only in the spec's
settings, never in its data: a data value that reads `token:fg` stays text.
Colours of the form `#0a0bNN` are reserved: md2html draws each token in
one while it builds a chart, so a spec that names one warns and gets that
token's colour.

A literal colour, such as `"#1f5fa8"`, cannot follow the theme, so the
chart is drawn twice, once in each theme's colours, and the page shows the
copy that matches. That doubles the chart's size. For a lighter step of a
colour, keep the token and set `opacity`, but tell categories apart by
colour, not by opacity alone.

## Width

A chart is laid out at its style's chart width, 640 pixels in both built-in
styles, unless the spec sets `width`. A spec that places views side by side
(`hconcat`, `facet` and the like) gets no width from the style.
`width: container` takes the style's width too, since a build has no
container to measure, and `height: container` is Vega-Lite's default
height, as if the spec set none. The chart scales down with its column
and never makes the page scroll sideways; its text scales with it, so a
wide chart on a phone has small text.

## The table view

Under each chart, a collapsed block titled with the caption, or "Chart
data" without one, holds the data the chart drew as a table: one row per
datum each mark drew, one column per field the spec encodes, in encoding
order. A field a transform makes, such as a `fold`'s `key` and `value` or
a `calculate`, is there as the chart drew it. An aggregate's column takes
the name Vega-Lite gives it, such as `sum_v`, and a count's is `count`.
A `timeUnit`'s column does too, such as `yearmonth_date`, and holds the
value the chart drew: the date cut to that unit, so `2024-03-17` under
`yearmonth` is written `2024-03-01`. A unit with no year, such as
`month`, puts every date in 2012, as Vega-Lite does: `2012-03-01`. A
layer drawn from the same data as an earlier one adds no rows, nor do a
line's points, however many series it has. A layer that draws other
rows, such as a rule at a mean, adds its own, with blanks in the columns
it does not encode. Values are as the data holds them, not as the axis
formats them; a date is written in ISO form, `2024-01-01`, or
`2024-01-01T09:30:00Z` with a time. The table is there for a reader who
cannot see the chart, and for one whose browser cannot show the hover
boxes.

## Hover

- **Tooltips.** Each mark a `tooltip` encoding covers gets a box, shown
  above the mark while it is hovered. Where there is no room the box flips
  below, and it shifts sideways to stay inside the chart. A line or an
  area is one shape for all its data, so each of its data points gets a
  box instead, above the point, shown by its nearest-point region.
- **Nearest point.** On a chart whose tooltip marks are all points, lines
  and areas, hovering anywhere nearer one point, or one data point of a
  line or an area, than any other shows that point's box, up to 500
  points. A line or an area with a tooltip beside a tooltip mark of
  another kind, such as a bar, or in a chart of several views, such as a
  `concat` or a `facet`, gets no regions, so its boxes never show and the
  build warns; give the line `point: true`, and its points carry the
  boxes.
- **Keyboard.** Tab reaches each mark with a tooltip, and focus shows its
  box. A chart with more than 50 such marks is one tab stop instead, and
  its table view holds the values. The data points of a line or an area
  are not tab stops, since no element of the chart is theirs; the table
  view holds their values.
- **Legend.** Hovering or focusing an entry of a colour legend dims the
  marks of every other series.

The box sits at one place per mark: it does not follow the cursor, so on a
long bar it is at the bar's centre. There is no zoom, pan, brushing or
click-to-filter, and no live data. The boxes need CSS `:has()`, in every
major browser since late 2023; without it a chart shows no boxes and is
otherwise unchanged.

## Credits

Charts are drawn by Vega and Vega-Lite, from the University of Washington
Interactive Data Lab, which md2html embeds under their BSD 3-Clause
licence. md2html is not affiliated with or endorsed by their authors.
`md2html --licenses` prints the licences of everything md2html embeds.
`md2html --guide styles` covers how a style sets a chart's colours, face
and width.
