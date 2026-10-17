---
site: true
style: proof, docs
plugins: style
repo-url: https://github.com/AdamF-G/md2html/blob/main/{path}
---
[[mount source="../index.md"]]

[[mount path="overview" source="../../README.md" nav-title="Overview" nav-order="1"]]

[[mount path="guide" source="../guide" nav-title="Guide" nav-order="2"]]

[[mount path="design-notes" source="../specs" nav-title="Design Notes" nav-order="3"]]

[[mount path="changelog" source="../../CHANGELOG.md" nav-order="4"]]

::: header {.topbar}
[**md2html**](../index.md)

[[pages depth="1"]]

[[search]]

[[theme-toggle]]
:::

::: nav {.sidebar aria-label="Section"}
[[pages from="section"]]
:::

[[breadcrumb]]

[[content]]

[[prev-next]]

::: footer {.foot}
md2html is MIT-licensed. [Source on GitHub](https://github.com/AdamF-G/md2html).
:::
