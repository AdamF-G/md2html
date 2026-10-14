# md2html

md2html turns a tree of Markdown documents into self-contained HTML pages:
one file per page, its stylesheet inline, links between documents rewritten
to the pages they became. It reads GitHub-flavoured Markdown plus callouts,
containers, diagrams, charts and cross-references, and builds sites like
this one from a single Markdown shell page.

- [What md2html does and how to install it](README.md)
- [Writing Markdown for md2html](docs/guide/index.md), the authoring guide
- [What changed in each release](CHANGELOG.md)
- [Design notes](docs/specs/index.md), the reasoning behind each feature

This site is built by md2html itself: `just site`.
