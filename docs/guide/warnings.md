---
nav-order: 15
---
# Warnings

[[toc]]

What each warning means and how to fix it. The text in bold is the start of
the message.

**link target does not exist** — the `.md` file is missing. Fix the path.

**referenced asset does not exist** — an image or other file, or an
`.html` link, points at nothing. Fix the path; link pages by their `.md`.

**refusing to follow … outside … (no -o given)** — without `-o`, links
may not leave the entry tree. Add `-o`, or add the target's tree as an
entry point.

**not following …: excluded** — the link points into an `--exclude`d path.
Expected if that tree is built by another tool.

**exclude "…" matches nothing** / **is not a valid pattern** — check the
`--exclude` value; a name pattern cannot contain `/`.

**pulled in … document(s) from outside** — links reached pages outside the
entry points. Check none were unintended.

**front matter block is not flat key: value** — see
[Front matter](page-structure.md#front-matter): no nesting, comments or colon-less lines.

**front matter toc "…" has no [[toc]] or [TOC] marker** — add the marker,
or drop the `toc:` key.

**… is not a contents list layout** — use `inline` or `float`, or, in
front matter only, `none`.

**--autotoc "…" is not all or long** — use `all` or `long`. The run stops
with exit status 2; the library instead warns and ignores the value.

**… is not a language tag** — use a tag such as `en`, `de` or `pt-BR`.

**unknown container kind** — use a kind from [Containers](callouts-and-containers.md#containers), or
the braced form for your own class.

**container has no class and no recognizable kind name** — `::: {}` has
nothing in it. Name a kind.

**… container has no definition list** — in a `stats` or `defs`
container, write each entry as a term line followed by `: ` definition
lines.

**… cannot be set from an attribute block** — `href`, `src` or `srcset`;
write the target in the link itself.

**fig fence: …; rendering it as a code block** — the figure is shown as
its YAML source. The rest of the message names the fault:

- *did not find expected key*, *found character that cannot start any
  token*, *mapping values are not allowed in this context*, *unknown
  anchor … referenced*, *cannot unmarshal !!map into string* (or
  *!!seq*) — a label needs quotes. See [Quote labels](diagrams.md#quote-labels).
- *unknown key "…"* — a misspelled kind or modifier.
- *names no kind* — a bare `box:`; write `box: ""` for a blank box.
- *names … kinds* — one item per list entry.
- *carries …, which only … takes*, naming `note`, `accent`, `foot`,
  `boundary`, `weight` or `items` — move the modifier to an item that
  takes it.
- *layout … is not rows, cols or split*, *a split needs exactly 2 items* —
  fix the layout.
- *the figure's only item; write layout: cols instead* — as it says.
- *the fence is empty*, *more than one YAML document* — add items, or
  remove the `---` separator.
- *a tab in a tree's indentation*, *indented to no enclosing level* —
  indent the tree with spaces, consistently.

**fig fence: a caption in the info string is not part of a figure** —
use a `caption:` key in the body.

**vega-lite fence …: …; showing its source** — the chart is shown as its
source. The fence's number counts charts from the top of the page. The
rest of the message names the fault:

- *the spec is not YAML or JSON* — fix the syntax; the message says where.
- *the fence is empty; it needs a Vega-Lite spec*, *the spec is not a
  mapping of Vega-Lite keys* — the body is a mapping such as `mark: bar`.
- *md2html reads only files beside the page, never the network* — a data
  `url` with a scheme or host. Save the file beside the page.
- *a file has no query or fragment* — a data `url` such as
  `results.csv?v=2` or `results.csv#top`. Remove the part from the `?` or
  `#`.
- *it is outside …*, *an absolute path is outside the pages* — the data
  file leaves the run's base. Move it inside. See [Data](charts.md#data).
- *charts read no data files here* — a `Convert` call with no
  `SourcePath`. Set it, or put the data in `values`.
- *ran past its … time budget* — the chart took too long. Give it less
  data.
- *an image mark's url … would make the page fetch it* — an `image` mark
  draws only a `data:` URL. Embed the image as one, or leave the mark out.
- Anything else is Vega-Lite's or Vega's own error. Check the spec against
  the [Vega-Lite documentation](https://vega.github.io/vega-lite/docs/).

**vega-lite fence …: … names no token** — a `token:` name outside the
list in [Colours](charts.md#colours), or the style's own. The message lists the
names.

**vega-lite fence …: … is reserved by md2html for a chart token** — a
`#0a0bNN` colour. Use a `token:` name, or another colour.

**vega-lite fence …: … tooltip marks could not be matched to the SVG** —
the chart renders without hover boxes; its table view still holds its
values.

**vega-lite fence …: … line or area tooltips have no hover** — the chart
gets no nearest-point regions to show them. The message says why:

- *all points, lines and areas* — a line or an area has a tooltip beside
  a tooltip mark of another kind, such as a bar. Give the line
  `point: true`, so its points carry the boxes, or move the tooltip to the
  other mark.
- *tooltip points, over the … that get nearest-point regions* — the chart
  has over 500. Give it less data.
- *no single plot area* — the chart is several views, such as a `concat`,
  `facet` or `repeat`. Give the line `point: true`.

The table view still holds the values.

Vega-Lite's own warnings, such as an encoding it will ignore, also arrive
as **vega-lite fence …:** warnings. The chart still renders; read the
message, since what it ignores is missing from the chart.

**front matter style: … ; using the run's style** — a page's `style:`
names a style that is not available, or one that cannot be stacked where
it is, or a plugin it loads has the same name as another.
The message says which; fix the name, load its plugin with `plugins:`, or
rename one of the clashing plugins.

**front matter plugins: … ; using the run's style** — a page's
`plugins:` names a directory that is not a plugin. Fix the path; it is
relative to the page.

**front matter plugins has no effect without style** — add `style:`, or
remove `plugins:`.

**style … is selected twice; using it once** — remove the repeat.

**stylesheet is … KB, over 1 MB** — a style's fonts or images are large,
and every page carries them. Subset the fonts or drop some.

**plugin …: unknown key … ; ignoring it** — the plugin was written for a
newer md2html, or has a typo in its manifest.

**output path collision** — two sources map to one page; rename one.

**refusing to overwrite … (not generated by md2html)** — an output path
holds a file md2html did not write. Move it, or choose another `-o`.

**… needs a site shell (--site)** — a navigation marker (`pages`,
`breadcrumb`, `prev-next`, `search`) in a build with no shell. It stays as
written; build with `--site`.

**… belongs in the site's shell** — a `[[content]]` in a page's body. It
stays as written; put it in the shell.

**… has no effect in a fragment** — a `[[theme-toggle]]` in `--fragment`
output, whose host sets the theme. It stays as written.

**front matter … is not @file#key** — a reference needs a path, a `#` and
a key. The value is used as written.

**front matter … refers to …, which cannot be read** — the file is missing
or unreadable. The value is left empty; fix the path.

**… which has no such key** — the referenced file's front matter lacks the
key. The value is left empty.

**… which is itself a reference** — a reference to a reference is not
followed. Point at the file that holds the value.

**… has no key attribute** — a `[[list]]` needs `key="…"`. It lists
nothing.

**… is not in the front matter** — the `[[list]]` key names no front
matter key on its page. It lists nothing.

**… has no path attribute** — a `[[subdirs]]` needs `path="…"`.

**… cannot read …** — the `[[subdirs]]` path is not a readable directory.
It lists nothing.

**… points at nothing** — a symlink in a `[[subdirs]]` directory is
dangling. It is left out.

**… is not version or name** — a `[[subdirs]]` `sort`. It sorts by
version.

**template … names no front matter key** — a `{name}` the page does not
define expands to nothing.

**… has no link attribute** — a `[[list]]` or `[[subdirs]]` without
`link="…"` lists its entries as text.

**… is not list or inline** — an `as` value. A list is drawn.

**… is not a whole number** — a `nav-order` or a `[[pages]]` `depth`. The
value is ignored; `depth` needs 1 or more.

**… is not root or section** — a `[[pages]]` `from`. It lists from the
root.

**… is not hidden** — `nav:` takes only `hidden`. The value is ignored.

**links to …, which the build does not write** — a link in the shell to a
`.md` outside the build. It points at the Markdown file; build that page
too, or link something else.

**… has no index.md** — a directory in the tree with no page of its own;
the site lists it by its directory name.

**… outside the site's root** — a page pulled in from outside the run's
base. It is written but has no place in the navigation.

**front matter shell … is not none** — `shell:` takes only `none`. The
page is wrapped in the shell.

**front matter toc "float" has no effect under a site shell** — the
contents list is drawn inline.
