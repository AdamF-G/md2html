# Order by Links — Design

**Date:** 2026-10-16
**Author:** AdamF-G
**Status:** Draft for review.

## Purpose

A hand-written `index.md` often lists its directory's pages in the order
the author wants them read. Site mode orders the tree by `nav-order:`, then
by label, so the order has to be written twice: once as the index's list,
and again as a number in each page's front matter. The two drift, and a
reader then meets the pages in one order on the index and another in the
sidebar and in previous and next. A directory mounted from elsewhere
([Mounts](2026-10-16-mounts-design.md)) has an index the site cannot add
numbers to at all.

This note lets an index's own links set the order.

## 1. The key {#key}

`nav-sort: links` in the front matter of a directory's page orders that
directory's children by that page's links. The directory's page is its
`index.md` or, when it has none, its `README.md`
([Mounts](2026-10-16-mounts-design.md), §5). `nav-sort: label`, the
default, is today's order. A directory with only a page md2html writes has
no links of its own to follow, so it is ordered by label whatever it is
given.

A mount takes the same setting as an attribute, `nav-sort="links"`, for a
directory whose pages it does not own; the mount wins over the page, as its
other settings do. The attribute applies to the mounted directory and to
every directory below it that does not set `nav-sort:` itself, since
nobody can add the key to each page of a source the site does not own. A
directory that sets its own value, `label` included, keeps it.

The key is not `nav-order: links`: an index's `nav-order:` already places
its directory among the directory's own siblings, and one key cannot say
both.

## 2. The order {#order}

A directory's children are a list of pages and subdirectories. With
`nav-sort: links` they are ordered:

1. Those with a `nav-order:`, by it, as today. An explicit number always
   wins over an inferred place.
2. Those the index links, in the order of each one's first link in the
   index's body.
3. The rest, by label, as today.

A link counts when it reaches the child's own page: a page, or a
subdirectory's directory page. A link deeper into a subdirectory does not
rank the subdirectory, and links in the shell do not count. The setting is
the directory's own: a subdirectory orders its children by its own index.

`nav-sort` with another value warns and orders by label.

## 3. Testing {#testing}

- An index's links set its children's order, pages and subdirectories
  alike, by first link.
- A child with `nav-order:` comes first, and a child the index never links
  comes last, by label.
- A link deeper into a subdirectory does not rank it.
- A mount's `nav-sort="links"` orders a mounted directory whose index has
  no `nav-sort:`, and each directory below it that has none, by each one's
  own page; a directory below it that sets `nav-sort: label` keeps it.
- A `README.md` that is a directory's page carries `nav-sort:` as an
  `index.md` does, and a directory with a written page is ordered by label.
- A subdirectory without the key keeps today's order.
- An unknown value warns.
- A site without `nav-sort:` is ordered exactly as before.
