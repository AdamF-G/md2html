---
nav-order: 4
---
# Links and images

**Link to the `.md` file.** md2html rewrites it to the page it emits and
works out the relative path:

```markdown
[Auth](./api/auth.md)          -> href="api/auth.html"
[Setup](./api/auth.md#setup)   -> href="api/auth.html#setup"
```

Reference-style links are rewritten the same way. `.md` and `.markdown` are
document extensions; every other file is an asset.

- **A link to a missing `.md` warns** and is left as written.
- **An `.html` link is an asset link, not a page link.** If the file is
  missing it warns; if it exists, `-o` points the link back at the source
  tree. It is never matched to the page md2html emits.
- **A link starting with `/` is a filesystem path**, not a site path, so it
  usually warns that the target does not exist. Use relative links.
- **A link to a directory** points at the directory, not at an index page
  inside it. Link the file.
- **A query string on a document link is dropped.** `./b.md?v=2` becomes
  `b.html`. Fragments are kept.
- **Links inside code are inert.** A fenced example containing
  `[a](./nope.md)` is not followed and does not warn.

**Assets are linked, not copied.** With `-o`, an image link is rewritten to
point back at the original file, so moving the output tree on its own
breaks images. Without `-o`, each page sits beside its source and links are
left as written. `<img src>` and Markdown images are rewritten and checked;
`srcset`, `<video poster>`, `<track src>` and `<object data>` are not.

**A link or image can carry attributes.** Write a `{#id .class key=value}`
block straight after it, with no space (Pandoc's `link_attributes`):

```markdown
[Intro](./intro.md){aria-current=page}   -> <a href="intro.html" aria-current="page">
![Chart](./chart.png){width=50% .wide}   -> <img ... class="wide" width="50%">
```

It takes the global attributes, some of the element's own (`target`,
`rel`, `download`, `referrerpolicy` on a link; `width`, `height`,
`loading`, `decoding`, `sizes` on an image), and any `data-` or `aria-`
name. `hreflang` and `type` are among those dropped. Anything else, such as an event handler, is
dropped. `href`, `src` and `srcset` warn: write the target in the link. A
block with a space before it, with nothing in it, or with something that is
not an attribute name (`{{version}}`) stays as literal text.

Off-site links get `target="_blank" rel="noopener noreferrer"`, added to
any `rel` you wrote.
