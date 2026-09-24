# md2html

Convert existing Markdown documentation to HTML. One static binary, no
configuration, no runtime.

```bash
go install github.com/AdamF-G/md2html/cmd/md2html@latest
md2html ./docs -o ./site
```

## What it does

Walks your docs, follows links between documents across directories, and
writes a browsable HTML tree. Tables get scroll containers, headings get
anchors, `.md` links become `.html` links, and mermaid fences render as
diagrams. Markdown also gets a few extras beyond CommonMark/GFM: fenced
containers (`::: callout` … `:::`, or `:::callout[With a title]`) for
callouts, warnings, cards, collapsible asides/examples and named `<nav>`
landmarks; GitHub alerts
(`> [!WARNING]`) as an alias for the same; `[proven]`-style status chips
and the general `[label]{.chip}` form; `§4.2` cross-references that
autolink to numbered headings; a `[[toc]]` or `[TOC]` marker for a
per-page contents list; `title`/`subtitle`/`date` front matter; a
`caption="…"` attribute on code fences, braced or bare; and a `fig` fence
for hand-laid-out diagrams — boxes, arrows and panels described in YAML,
for layouts a mermaid graph cannot express.

## The dialect

md2html reads **CommonMark plus fenced divs, bracketed spans, fenced code
attributes, header attributes, definition lists, footnotes, pipe tables and
YAML front matter** — which is to say Pandoc's `commonmark_x` — plus GitHub
alerts, status chips, `§` cross-references, `[[toc]]` and `fig`.

That is a measured claim, not an aspiration. `compat/` runs each construct
through both tools and sorts every construct into one of three buckets:
the ones where md2html and `pandoc -f commonmark_x` produce the same
structure, the ones Pandoc passes through untouched, and the ones Pandoc
renders as a labelled code block. Task lists are the single divergence
inside the shared subset — `commonmark_x` does not implement them and
md2html follows GFM.

The practical consequence is that a document written for md2html mostly
survives `pandoc -f commonmark_x`, and a document written for Pandoc
mostly converts here. Where a construct is ours alone, Pandoc degrades it
legibly rather than corrupting it.

The one thing a generated page fetches at view time is MermaidJS, and only
a page that actually contains a diagram: it loads a pinned build from a CDN.
Pages without diagrams reference nothing external, and `--fragment` output
never loads it at all, because a fragment's host — a Claude Artifact, for
one — renders mermaid itself. A build
that must not reach a CDN can name its own copy — see `Options.MermaidURL`
below.

## Why Markdown, not HTML

Every page here could be hand-written as HTML instead. The reason not to is
that the Markdown costs less to write, less to revise, and less to review —
and the first of those is measurable.

### Writing it

Rendering the 13 Markdown documents this repository held at v0.2.0 and
counting tokens on both sides:

| | tokens | vs Markdown |
|---|---:|---:|
| Markdown source | 121,534 | 1.00× |
| Generated HTML, body markup only | 166,113 | 1.37× |
| Generated HTML, complete pages | 216,040 | 1.78× |

Hand-writing this documentation set as finished pages costs about 94,500
more tokens.

The corpus figure understates the gap, because 58% of these documents is
fenced source code, which passes through at roughly 1:1 and dilutes
everything around it. Strip the fences and compare only prose and structure
and the ratio is **1.57×**.

It concentrates in exactly the constructs a document is built from:

| One instance of | Markdown | HTML | |
|---|---:|---:|---:|
| `## Where output goes` | 4 | 35 | 8.75× |
| a 2×3 table | 42 | 112 | 2.67× |
| a code fence with `caption=` | 22 | 49 | 2.23× |
| an internal and an external link | 22 | 43 | 1.95× |
| a three-item bullet list | 29 | 51 | 1.76× |
| `::: callout` | 26 | 36 | 1.38× |
| a paragraph of prose | 39 | 43 | 1.10× |

Headings are the outlier and a document is full of them: four tokens of
Markdown against an id, an anchor and an `aria-hidden` attribute. Prose is
the floor, at 1.10× — `<p>` and `</p>` and nothing else to pay for.

The embedded stylesheet is a further 3,766 tokens you never write at all,
which is why short pages gain most: `CHANGELOG.md` is 4.95× as a complete
page against 1.45× on body markup alone.

### Changing it

Three ordinary revisions to this README — rewording a sentence, adding a row
to the flags table, renaming a heading — produce a 325-token diff in the
Markdown and a 491-token diff in the HTML. The ratio roughly holds, but the
*shape* of the difference matters more than the size.

Renaming a heading is one edit in Markdown. In HTML it is three, and they
have to agree:

```html
-<h3 id="where-output-goes">Where output goes<a class="anchor" href="#where-output-goes" aria-hidden="true">#</a></h3>
+<h3 id="where-the-output-lands">Where the output lands<a class="anchor" href="#where-the-output-lands" aria-hidden="true">#</a></h3>
```

Miss one and the anchor points at nothing, silently, and so does every
inbound link that used the old slug. Derived, the three cannot drift apart.

Adding a table row is worse to read than to write. In Markdown the diff is
the row. In HTML it is this:

```
+</tr>
+<tr>
+<td><code>--quiet</code></td>
+<td>suppress per-file progress; warnings still print</td>
```

Four lines, and the first of them closes the *previous* row — because a
line-based diff aligns on `</tr>`, not on the boundary a human sees. The
reviewer has to reassemble the change before judging it.

### Why it compounds for an agent

An agent edits by string replacement, re-reads the file on every pass, and
never sees the rendered page. All three favour the Markdown:

- **Edits stay local.** A Markdown unit — a sentence, a row, a heading — is
  addressable on its own. Its HTML counterpart is wrapped in tags whose
  boundaries rarely coincide with the boundaries of the change, so the
  smallest safe replacement is larger than the edit.
- **Read cost recurs.** Write cost is paid once. Read cost is paid on every
  turn that pulls the file back into context, and a 40,000-token page
  against a 28,000-token source is not a one-time difference.
- **Invariants cannot rot.** Heading ids, anchor hrefs,
  `rel="noopener noreferrer"`, table scroll wrappers and `.md` → `.html`
  rewriting are computed on every build. There is no copy of them an edit
  can leave stale.
- **Malformed structure is unrepresentable.** Markdown has no unclosed
  `<div>`. A nesting mistake in hand-written HTML renders as something
  plausible and wrong, which is the failure mode neither a human reviewer
  nor an agent reliably catches.

### What the numbers assume

They assume the hand-written HTML would be *equivalent* — that you really
would write the ids, the anchors, the `rel="noopener noreferrer"` and the
table wrappers. Drop those and the prose-and-structure ratio falls from
1.57× to 1.43×, so about a seventh of the measured win is work you might
have skipped rather than typed.

Pretty-printing, which looks like it ought to matter, does not: packing
every newline out from between tags moves 1.57× to 1.56×. The cost is in
tag names and attributes, not whitespace.

Counts are `cl100k_base`. A different tokenizer moves the absolute numbers
and leaves the ratios about where they are.

## Usage

```
md2html [flags] <entry> [entry...]
```

Entries may be files or directories.

```bash
md2html ./docs -o ./site                     # whole tree
md2html ./docs --depth 0 -o ./site           # top-level seeds, still follows links
md2html ./docs/index.md ./guides -o ./site   # mixed entries
md2html ./docs                               # in place, beside each source
md2html ./docs -o ./site --fragment          # bare fragments, no page shell
md2html README.md                            # one file, HTML beside it

# The maintained set, plus one scratch directory for this run only,
# with a vendored subtree another tool owns left strictly alone.
md2html -o ./site ./docs ./scratch/notes --exclude vendor

# Skip every document named AUDIT_*, wherever it sits.
md2html -o ./site ./docs --exclude 'AUDIT_*'
```

### Two kinds of reach

Links between documents are followed across directories, unbounded by
default. `--depth` and `--link-depth` bound two different things: `--depth`
limits how deep into a directory md2html looks for *unlinked* files to seed
from; `--link-depth` limits how many hops from a seed link-following may
travel. Both read the same way: `-1` is unlimited (the default for each),
and a non-negative number is the bound itself, so `--link-depth 0` follows
no links at all. `--depth 0` seeds only the Markdown sitting directly in
the directory — and, with `--link-depth` left at its default, still follows
every link out of it, however far that leads.

### Where output goes

With `-o`, everything lands inside it. Documents under the entry points
mirror their relative paths; documents pulled in from elsewhere on disk go
under `_external/`, mirroring their absolute path. Nothing is ever written
outside `-o`.

Without `-o`, HTML is written beside each source, and links that would
escape the entry tree are refused rather than followed.

### Assets are linked, never copied

An image stays where it is. Its link is rewritten to point back at the
original, climbing out of the output directory as far as needed:

```
docs/api/auth.md  ->  site/api/auth.html
  ./img/flow.png  ->  ../../docs/api/img/flow.png
```

So the output tree is self-contained for *documents* but not for *assets*.
Move `site/` on its own and images break; move it alongside the sources and
they do not. That is the deliberate trade for never duplicating your files.

### It will not overwrite your work

Every generated file starts with:

```html
<!-- generated by https://github.com/AdamF-G/md2html v0.5.0 - edits will be overwritten -->
```

Re-runs replace files carrying that marker silently. A file **without** it
is never touched — md2html warns, skips it, and exits non-zero. There is no
override flag. The marker carries the full repository URL, so output from
unrelated tools that share the name is never claimed.

## Flags

| Flag | Effect |
|---|---|
| `-o DIR` | output directory; default writes beside each source |
| `--depth N` | directory levels to seed from a directory entry; `-1` (default) unlimited |
| `--link-depth N` | hops from a seed that link-following may travel; `-1` (default) unlimited, `0` follows none |
| `--fragment` | emit bare HTML fragments (for hosts such as Claude Artifacts) instead of full pages |
| `--css FILE` | replace the embedded stylesheet |
| `--no-table-scroll` | do not wrap tables |
| `--no-anchors` | do not add heading anchors |
| `--no-external-links` | do not mark external links |
| `--no-md-links` | do not rewrite `.md` links |
| `--no-assets` | do not rewrite asset links |
| `--exclude DIR\|GLOB` | never enter, seed, follow into, or write to `DIR` (relative to the base — the common ancestor of the entry points — or absolute). A value containing `*`, `?` or `[` is instead a name glob, matched against every file and directory name below the base: `--exclude 'AUDIT_*'` skips `AUDIT_2026.md` at any depth. Quote it so the shell does not expand it. Repeatable, or comma-separated |
| `--version` | print the version and exit; the same version the provenance marker carries |
| `--install-skill-user` | install the Claude Code authoring skill under `~/.claude/skills` and exit |
| `--install-skill-project` | install it under `./.claude/skills` instead |

## Writing docs for it

Callouts, diagrams, heading attributes, footnotes and definition lists all
work, and one of them fails silently if you get the syntax wrong. See
[docs/authoring.md](./docs/authoring.md).

### Installing the skill

If you write those docs with Claude Code, the binary carries the authoring
skill and will install it for you:

```bash
md2html --install-skill-user      # for you, under ~/.claude/skills
md2html --install-skill-project   # for this repo, under ./.claude/skills
```

Each writes `SKILL.md` and a copy of the authoring reference into a
`md2html-authoring` directory, so the guidance travels with the binary and
always describes the version you installed — which a link to this repo's
`main` would not. Neither will create the `.claude` directory itself: a
missing one means this is not a Claude Code workspace, or you are not
standing where you meant to be, and inventing it would leave the skill
somewhere nothing reads.

Both files carry the same provenance marker as generated HTML, so a later
`--install-skill-user` replaces this tool's own copy silently and refuses a copy
you have edited, naming it.

## Library use

```go
import "github.com/AdamF-G/md2html"

out, err := md2html.Convert(src, md2html.Options{SourcePath: "doc.md"})
```

`Options.Warn`, when set, receives one message per non-fatal problem found
while converting a document. Three things report so far: a front matter
block that isn't flat `key: value`, a container naming a kind that doesn't
exist, and a `fig` fence whose body doesn't parse or doesn't validate. All
three arrive whatever `Transforms` holds — a builtin that reports is rebuilt
against this sink before it runs, so appending to `Builtins()` costs you no
diagnostics. `Convert` never writes to stderr itself; the callback runs
synchronously on the calling goroutine.

`Options.MermaidURL` names the ES module a page with a diagram imports at
view time, for a docs build that must not reach a CDN. Empty keeps the
pinned build.

Add a transform by writing a `func(*html.Node) error`:

```go
ts := append(md2html.Builtins(), md2html.Transform{
    Name: "myRule",
    Fn: func(root *html.Node) error { /* walk and mutate */ return nil },
})
out, err := md2html.Convert(src, md2html.Options{Transforms: ts})
```

Transforms run over a real HTML tree, so they reach hand-written raw HTML
in your Markdown as well as generated markup.

## Development

`just install` builds the binary and installs it to `~/.local/bin`, then
prints the version it just installed:

```bash
just install                        # to ~/.local/bin
just bindir=/somewhere/else install # anywhere else
```

It refuses to install into a directory that does not already exist, rather
than creating one: `go install` will happily create a missing `GOBIN` and
every level above it, which on a fresh machine puts the binary somewhere
nothing on `PATH` will ever read and still reports success.

`just test` (or `go test ./...`) needs no browser. A separate suite drives
real headless Chrome to exercise the click-to-expand JavaScript runtimes
end to end:

```bash
just e2e     # or: cd e2e && go test ./...
```

A third suite characterises the Markdown dialect against Pandoc:

```bash
just compat   # or: cd compat && go test ./...
```

It needs `pandoc` on PATH and skips itself without one.

The browser suite requires Chrome or Chromium installed locally, and lives
in `e2e/` as its own Go module — as `compat/` does, for the same reason. That boundary is the only opt-in — there is no build tag —
and it is what keeps chromedp out of the library: a test-only import would
sit in this module's own `go.mod` as a direct requirement and be compiled by
anyone who ran its tests. Nothing in a build or test of the library
compiles it now. (It does still appear in `go.mod` as an *indirect* entry:
`go.abhg.dev/goldmark/mermaid` requires it for a server-side renderer
md2html never imports. That one is inherited, and the split cannot remove
it.)

GitHub Actions runs all three suites on every push and pull request, along
with `gofmt`, `go vet` and `go mod tidy -diff`. Pandoc is pinned there, for
the reason the mermaid build is pinned: the compat suite records how two
tools agree, so an unannounced upgrade of the other one would report a
change in md2html that never happened. [CHANGELOG.md](./CHANGELOG.md)
records what has changed.

## License

MIT — see [LICENSE](./LICENSE). `internal/fences` is vendored from
`goldmark-fences` and keeps its own MIT licence; `testdata/vendor` holds a
test-only copy of MermaidJS under its MIT licence, with the notices of the
libraries it bundles left in place.
