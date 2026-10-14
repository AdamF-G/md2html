---
nav-order: 1
---
# Quick reference

The most common needs first. Each row is the form to write; where a
construct has other spellings, they are under
[Choosing between forms](choosing-between-forms.md).

| Need | Write |
|---|---|
| Link to another page | `[Setup](./setup.md)`, never `./setup.html` |
| Title, subtitle, date | `# Title` as the first heading; `subtitle:` and `date:` in front matter |
| Contents list | `[[toc]]` alone on a line |
| Note or tip | `> [!NOTE]` on its own line, then `> text` |
| Warning | `> [!WARNING]` on its own line, then `> text` |
| Callout with a title | `::: callout Title` … `:::` (`::: warning Title` for a warning) |
| Collapsible block | `::: details Title` … `:::` |
| Code block with a caption | ```` ```go {caption="server.go"} ```` |
| Diagram | a ```` ```mermaid ```` fence |
| Stable heading anchor | `## Title {#my-id}` |
| Cross-reference to a numbered heading | `§4.2` |
| Status badge | `[draft]`, `[proven]`, `[verified]`, `[designed]`, `[planned]`, `[deprecated]` |
| Any other badge | `[needs review]{.chip}` |
| Aside or worked example | `::: aside Title` or `::: example Title` … `:::` |
| Stat tiles, term grid, titled panel | `::: stats`, `::: defs`, `::: group Title` … `:::` |
| Nav, header or footer | `::: nav`/`header`/`footer` … `:::` |
| Hand-laid-out figure | a ```` ```fig ```` fence with a YAML body |
| Chart | a ```` ```vega-lite {caption="…"} ```` fence holding a Vega-Lite spec in YAML |
| Chart colour by meaning | `color: {value: "token:status-good"}`, or `-warning`, `-serious`, `-critical`; see [Colours](charts.md#colours) |
| Page language | `lang: de` in front matter |
| Page look | `style: claude-artifact` in front matter, or `--style`; `--list-styles` lists them |
| Output for a host that supplies the page (a Claude Artifact) | `--fragment` |
| A site shell around every page | `site: true` page holding `[[content]]`; `--site shell.md` |
| Site navigation | `[[pages]]` (`depth="1"`, `from="section"`), `[[breadcrumb]]`, `[[prev-next]]` in the shell |
| Site search | `[[search]]` in the shell; `search-facets: kind` |
| Links from a front matter list | `[[list key="versions" link="/d/{item}/"]]` |
| Links from subdirectories | `[[subdirs path="/srv/docs" match="v*" link="https://example.com/{item}/"]]` |
| A value from another file | `versions: @../versions.md#all-versions` |
| Order or hide a site page | `nav-order: 2`, `nav-title: Name`, `nav: hidden` |
