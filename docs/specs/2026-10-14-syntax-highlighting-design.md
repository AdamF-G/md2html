# Syntax highlighting — Design

**Date:** 2026-10-14
**Author:** AdamF-G
**Status:** Designed, not built. Awaiting review.

## Purpose

Highlight the code in a page's fences **when md2html builds the page**, so
that a reader sees keywords, strings and comments told apart in every
theme, with no script on the page and nothing fetched.

md2html contains no knowledge of any language. It carries one generic
engine that runs TextMate grammars, the data format VS Code, Shiki and Zola
use, and every language arrives as an unmodified grammar file through the
plugin mechanism. The built-in grammars are a pinned, licence-checked copy
of a public collection; an author adds or replaces a language by dropping a
`.tmLanguage.json` file into a local plugin.

````markdown
```go {caption="server.go"}
func main() { http.ListenAndServe(":8080", nil) }
```
````

renders as it does today, with each token wrapped in a classed span whose
colour comes from the page's style (illustrative; the exact kinds come from
the grammar):

```html
<pre><code class="language-go md2html-hl"><span class="hl-keyword">func</span> <span class="hl-function">main</span>() { http.<span class="hl-function">ListenAndServe</span>(<span class="hl-string">":8080"</span>, <span class="hl-constant">nil</span>) }
</code></pre>
```

## How we got here

Recorded so the decisions do not get relitigated. **Decided** means the
project owner chose it; **recommended** means it is this design's call,
with its reason, and can be struck in review.

| # | Decision | By | Reason |
|---|---|---|---|
| 1 | Highlighting is a general md2html feature, for any page with code | Decided | Any document with code benefits; a downstream docs site needed it first |
| 2 | md2html depends on no lexer per language; languages plug in from outside through a hook | Decided | Language support must not grow md2html's code or dependencies one language at a time |
| 3 | Languages are TextMate grammars, reused from a public collection, not written for md2html | Decided | The largest grammar ecosystem; the format VS Code itself uses; md2html authors no grammars |
| 4 | The collection is `tm-grammars` (Shiki's), pinned to one version | Recommended | Normalised JSON, each grammar pinned to an upstream commit with its licence recorded, updated daily, and the source Zola's bundled set is curated from |
| 5 | The engine is `github.com/eugenioenko/textmate-go`, behind an internal interface | Recommended | Exact match with VS Code's tokenizer on every sample in the spike, about 4x faster than it, pure Go, about 1.2 MB |
| 6 | Embed every permissively licensed grammar in the collection; md2html never fetches one | Decided | All 200 cost about 1.2 MB compressed; `md2html never fetches` stays true |
| 7 | Token kinds are a fixed, small vocabulary coloured by style tokens, not VS Code themes | Decided | Themes, the theme toggle, style plugins and `--check-style` all keep working unchanged |
| 8 | On by default for any fence whose language resolves; plain output, byte for byte as today, otherwise | Recommended | No setup for the common case, and no change for a page whose fences name no known language |
| 9 | Opt-outs per fence (`.no-highlight`), per page (`highlight: false`) and per run (`--no-highlight`) | Recommended | Matches how charts can be turned off; `no-highlight` is highlight.js's class |
| 10 | Warn on an unknown language only when it is a near miss of a known one | Recommended | `text`, `console` and made-up tags are common and deliberate; `pyhton` is a typo worth naming |
| 11 | A conformance test against VS Code's tokenizer gates every grammar and every engine upgrade | Recommended | The spike's harness is cheap to keep; it is what makes the engine swappable |
| 12 | Each built-in style chooses its own syntax palette; candidates are rendered side by side and the owner picks | Recommended | Follows charts decision 16 |

## Scope

**In scope (v1):**

- Highlighting fenced code blocks at build time, for every language the
  loaded grammars cover.
- The built-in `grammars` plugin: about 200 permissively licensed grammars
  from `tm-grammars`, embedded compressed.
- A `grammars` key in the plugin manifest, so a local plugin can add or
  replace languages.
- A fixed vocabulary of token kinds, the scope mapping that produces it,
  and `--syntax-*` colour tokens in every built-in style.
- `--check-style` rules for the new tokens.
- `--list-languages`, opt-outs, a near-miss warning, and fault fallbacks.

**Out of scope:**

- Inline code spans (`` `x` ``). They carry no language.
- Client-side highlighting, and VS Code colour themes applied directly
  (decision 7).
- Fetching grammars from the network (decision 6).
- Tree-sitter or any engine needing cgo or compiled parsers.

**Follow-ups** (each its own design):

- Line features: line numbers, highlighted lines (`{hl="3-5"}`), and diff
  backgrounds. Spans never cross a line (see HTML) so these can be
  added.
- Converting a VS Code colour theme into a style plugin's `--syntax-*`
  values.
- Highlighting the source of a `vega-lite` or `fig` fence when it falls back
  to a code block.

## Authoring

- **On by default.** A fence is highlighted when its language matches a
  grammar's name or one of its aliases, ignoring case. Aliases come from the
  collection's metadata: `sh`, `bash` and `zsh` reach `shellscript`, `yml`
  reaches `yaml`, `py` reaches `python`. Nothing in front matter or on the
  command line is needed.
- **The language** is the one md2html already reads from the info string,
  in every form `parseCodeFenceInfo` accepts: ```` ```go ````,
  ```` ```{.go caption="x"} ```` and ```` ```go {.wide} ````.
- **No match, no change.** A fence with no language, or a language no
  loaded grammar names, renders exactly as it does today, byte for byte.
- **Near misses warn.** An unknown language within edit distance 2 of a
  known name or alias, and at least four letters long, gives one warning
  naming the fence's line and the closest match: `fence at line 12:
  no grammar for "pyhton"; did you mean "python"?`. Any other unknown
  language is silent.
- **Opting out:**
  - one fence: `{.go .no-highlight}`; the class stays on the element as any
    other class does;
  - one page: `highlight: false` in front matter;
  - one run: `--no-highlight` (Options: `NoHighlight`, following
    `NoCharts`).
- **Special fences are unchanged.** `mermaid`, `fig` and `vega-lite` are
  taken before highlighting is considered. When one falls back to a code
  block, the code block stays plain.
- **Text is unchanged.** Only spans are added, so copying the code, the
  site search index and anything else that reads the element's text see
  exactly what they see today.

## Grammars

### The `grammars` manifest key

A plugin provides languages with a new top-level key, next to `styles`, as
the plugins design left room for. The manifest stays at `format: 1`: the
key only adds, and an older md2html warns about it and ignores it.

```json
{
  "format": 1,
  "name": "house-languages",
  "description": "Grammars for our in-house config formats",
  "grammars": {
    "fabcfg": {
      "file": "grammars/fabcfg.tmLanguage.json",
      "aliases": ["fab"]
    }
  }
}
```

| Key | Required | Meaning |
|---|---|---|
| `file` | yes | The grammar, relative to the plugin directory. `.json` or `.json.gz`. |
| `aliases` | no | Other names a fence can use. Same spelling rules as the language name. |
| `license` | no | SPDX identifier, shown by `--list-languages`. Required in built-in plugins. |
| `source` | no | Where the grammar came from, shown by `--list-languages`. Required in built-in plugins. |

**Rules:**

- Language names and aliases are lowercase letters, digits, `-`, `+`, `#`
  and `.`, starting with a letter, because real fence tags include `c++`,
  `c#` and `objective-c`. Matching ignores case.
- A plugin must provide at least one thing this release knows about. That
  is now a non-empty `styles` **or** a non-empty `grammars`, so a
  grammar-only plugin is valid.
- A grammar's scope name (`source.go`) is read from the file. A grammar
  that includes another by scope name (`include: "source.css"`) finds it
  among all loaded grammars, from any plugin.
- A grammar file that is not valid JSON, has no `scopeName`, or has no
  `patterns` is an error when the plugin loads, as a missing CSS file is.

### Loading and precedence

Grammars are not selected the way styles are. Every loaded plugin's
grammars are available to every page that loads it: the built-in `grammars`
plugin always, a local plugin through `--plugin` or front matter `plugins:`.

- **A local plugin overrides a built-in grammar** that has the same
  language name, alias or scope name, silently. That is the point of the
  hook.
- **Two local plugins claiming the same name** is a warning, and the one
  loaded later wins, matching the order `--plugin` and `plugins:` already
  give.
- `--list-languages` prints every available language with its aliases, the
  plugin it comes from, and its licence and source when known, then exits.

### The built-in `grammars` plugin

- Generated, never hand-edited. A `just grammars` recipe reads a pinned
  `tm-grammars` version, keeps every grammar whose SPDX licence is on an
  allowlist (MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, 0BSD,
  Unlicense, CC0-1.0), gzips each, and writes `plugins/grammars/` with a
  manifest carrying each grammar's licence and pinned source commit.
- Grammars with no licence, `NOASSERTION`, or a copyleft licence (GPL, MPL)
  are left out: 42 of 242 at version 1.32.22. The guide names them and
  says how to supply one from a local plugin.
- A grammar whose licence field is blank in the collection but whose
  upstream licence is permissive (the TextMate bundles' YAML and TOML) is
  added by a short, reviewed override list in the generator, each entry
  with its upstream licence text. Without it, YAML and TOML would not ship.
- A grammar that fails the conformance test (below) is left out, and the
  generator writes the reason into a report beside the manifest.
- `go:embed` stores files uncompressed, so the grammars are embedded as
  `.json.gz` and decompressed on first use with `compress/gzip`.

## Engine

- **textmate-go**, pinned, wrapped by a small internal package
  (`internal/highlight`) that is the only code importing it:

  ```go
  type Token struct {
      Start, End int      // byte offsets within the line
      Scopes     []string // root scope first
  }
  type Engine interface {
      // Tokenize returns one token list per line, carrying the rule
      // stack across lines.
      Tokenize(scopeName string, lines []string) ([][]Token, error)
  }
  ```

  Swapping in nuri, the spike's other exact match, means a second
  implementation of this interface and nothing else.
- **One registry per run**, held in the same cache as loaded plugins
  (`PluginCache`). A grammar is decompressed and parsed on the first fence
  that needs it and reused after that, so a page with no code costs
  nothing. The registry is safe for concurrent use.
- **Limits.** Each fence has a time budget of 2 s, and each line is
  tokenized up to 20,000 characters, VS Code's default; the rest of a longer
  line is plain. A fence over budget renders plain, with a warning.
- **Untranslatable patterns.** textmate-go reports an Oniguruma pattern it
  cannot translate and disables that rule (the spike found two, in the Go
  grammar's nested generic brackets, with no effect on any tested code). For
  a built-in grammar these are recorded in the generator's report. For a
  local grammar, md2html gives one warning per grammar listing them.

## Token kinds

### The vocabulary

| Kind | Class | Typical scopes |
|---|---|---|
| comment | `hl-comment` | `comment` |
| keyword | `hl-keyword` | `keyword`, `storage` |
| string | `hl-string` | `string` |
| number | `hl-number` | `constant.numeric` |
| constant | `hl-constant` | `constant`, `support.constant`, `variable.language` |
| function | `hl-function` | `entity.name.function`, `support.function` |
| type | `hl-type` | `entity.name.type`, `entity.name.class`, `support.type`, `support.class`, `entity.other.inherited-class` |
| property | `hl-property` | `support.type.property-name`, `variable.other.property` |
| tag | `hl-tag` | `entity.name.tag` |
| attribute | `hl-attribute` | `entity.other.attribute-name` |
| inserted | `hl-inserted` | `markup.inserted` |
| deleted | `hl-deleted` | `markup.deleted` |
| heading | `hl-heading` | `markup.heading`, `meta.diff.header`, `meta.diff.range` |

Thirteen kinds. Anything else is plain text in
the page's `--fg`.

**The mapping is about scope names, not languages.** TextMate grammars
share naming conventions (`comment.line.number-sign.python`,
`string.quoted.double.go`), so one table covers every grammar. A rule never
names a language suffix. That is why YAML keys, which the YAML grammar
scopes as `entity.name.tag.yaml`, come out as `tag`: the table may not
special-case them, and a style colours `tag` and `property` alike if it
wants keys to match across YAML and JSON.

### Matching

For each token, walk its scope stack from innermost to outermost. At each
scope, find the rule whose selector is the longest prefix of that scope on
dot boundaries (`string` matches `string.quoted.double.go`; `str` does not).
The first scope with any match decides the kind. This is how a TextMate
theme picks the deepest rule, simplified to prefixes, which is all a
fixed vocabulary needs.

- `punctuation.*` and `meta.*` scopes never match, so a string's quotes and
  a comment's `#` take the kind of the string or comment around them.
- A `meta.embedded.*` scope, or a `source.*` scope below the root, **stops
  the walk** with no kind, so code embedded in a string (an f-string's
  expression, a heredoc's script) is not coloured as the string around it.
- The table lives in Go as data, beside a test per row, and is tuned
  against the conformance samples during implementation. Changing it later
  changes output, so it is recorded in the changelog like any output
  change.

### HTML

- The `<code>` element keeps `language-x` and any author classes, and gains
  `md2html-hl`, so CSS can tell a highlighted block from a plain one.
- **Adjacent tokens of the same kind merge into one span.** A plain token
  gets no span.
- **No span crosses a newline.** A multi-line string or comment is closed at
  each line end and reopened on the next. This keeps every line a balanced
  fragment, which line numbers and highlighted lines will need.
- Text is escaped exactly as it is today. Output is deterministic: the same
  input gives byte-identical HTML.
- Captions, the `figure` wrapper and fence ids are unchanged.

## Colours

### Tokens

Every built-in style (`default`, `proof`, `claude-artifact`) defines one
token per kind, in the same theme-block shape as its other colours, and on
`:root[data-theme="light"]` too:

| Token | Kind |
|---|---|
| `--syntax-comment`, `--syntax-keyword`, `--syntax-string`, `--syntax-number`, `--syntax-constant`, `--syntax-function`, `--syntax-type`, `--syntax-property`, `--syntax-tag`, `--syntax-attribute`, `--syntax-heading` | text colours |
| `--syntax-inserted`, `--syntax-deleted` | text colours; `default` sets them to `var(--ok)` and `var(--warn)` |

- **The rules** live in a separate `highlight.css`, put ahead of the
  page's style only when the page has a highlighted fence, as `site.css` is
  (`needsSiteCSS`). A page with no highlighted code is byte-for-byte
  unchanged, and a style's own rules for the same classes win.
- Each rule falls back to the surrounding text colour,
  `.hl-keyword { color: var(--syntax-keyword, inherit) }`, so a
  `base: "none"` style that defines no syntax tokens shows readable, plain
  code rather than a broken palette.
- `hl-comment` is also italic. No other kind changes weight or style, so
  text metrics in a code block do not change.
- A third-party style layered on `default` that defines no syntax tokens
  inherits `default`'s.

### `--check-style`

The thirteen tokens join the checked tokens. Each is a **text colour on
`--code-bg`**, so the existing rules apply to them unchanged:

| Severity | Rule | Light | Dark |
|---|---|---|---|
| error | Defined in one theme but not the other | | |
| error | Its views in one theme differ | | |
| error | Text contrast on `--code-bg` | ≥ 3:1 | ≥ 3:1 |
| warning | Text contrast on `--code-bg` | ≥ 4.5:1 | ≥ 4.5:1 |

One new rule, since a palette that passes contrast can still show nothing:

| Severity | Rule | Light | Dark |
|---|---|---|---|
| warning | Comment, keyword, string, function and type each at least ΔE 8 from `--fg` and from each other, normal vision | ≥ 8 | ≥ 8 |

The threshold is a starting point, set during implementation so that every
built-in palette passes it with margin.

### Palettes

Each built-in style gets its own syntax palette (decision 12). The plan
renders candidates for each style side by side on the conformance samples,
in light and dark, and the owner picks. Every chosen palette passes
`--check-style` with no warnings.

## Faults

Every fault leaves the fence readable and the build going, as the `fig` and
chart fences do:

| Fault | Result |
|---|---|
| Unknown language, near miss | Plain fence, one warning naming the closest match |
| Unknown language, otherwise | Plain fence, silent |
| Fence over its time budget | Plain fence, warning naming the fence and language |
| Engine error while tokenizing | Plain fence, warning with the error |
| Line over 20,000 characters | That line plain from the limit on; silent |
| Local grammar unreadable, invalid or missing `scopeName` | Error when the plugin loads |
| Local grammar with untranslatable patterns | Loads; one warning listing them |
| Two local plugins claim the same language | Warning; the later one wins |

## Library API and CLI

- `Options.NoHighlight` and `--no-highlight` turn it off for the run.
- `--list-languages` lists languages, as `--list-styles` lists styles.
- `--licenses` gains the grammars' notices and the two new Go modules.
- No option names the engine or the collection version. Both are embedded
  and pinned, so there is nothing to point elsewhere.

## Costs

Measured in the spike, on Apple Silicon, stripped binaries:

- **Binary size.** About +1.2 MB for the engine (0.78 MB of it the
  regexp2 fork) and about +1.2 MB for 200 compressed grammars, so roughly
  +2.5 MB on today's 23.7 MB.
- **A second regexp2.** textmate-go uses its own fork,
  `github.com/eugenioenko/regexp2/v2`, at a module path that cannot be
  unified with the `github.com/dlclark/regexp2/v2` goja brings. Both are
  linked.
- **Build time.** Parsing seven grammars took 11 ms. About 2,000 lines of
  code in seven languages took 36 ms the first time and 23 ms warm; an
  8,000-line file took 139 ms, with no line over 2 ms. A page with no code
  costs nothing.

## Testing

- **Conformance.** For every shipped grammar, its sample from the
  collection's `samples/` directory (238 exist) is tokenized by VS Code's own
  tokenizer, `vscode-textmate` with `vscode-oniguruma`, when the grammars
  are generated, and the scope dump is checked in as a golden file. A Go test
  tokenizes the same samples with md2html's engine and requires them to
  match exactly, character for character on the full scope stack. Columns
  are compared as UTF-16 offsets, since the reference counts in JavaScript.
  Node runs only in `just grammars`, never in `go test` or the binary.
- **Engine upgrades** must keep the conformance test green. It is also
  the acceptance test for swapping engines.
- **Mapping.** A test per row of the kind table, plus the walk rules: a
  string's punctuation takes `string`, an embedded expression stops the
  walk, a `meta.*` scope never matches.
- **HTML.** Golden output for each kind, multi-line strings and comments
  (spans closed per line), adjacent-token merging, escaping, captions and
  author classes, byte-identical across runs.
- **Unchanged output.** A fence with no language, an unknown language, a
  `.no-highlight` fence, `highlight: false`, `--no-highlight`, and a page
  with no code each produce byte-identical output to the release before.
- **Faults.** Every row of the faults table gives the stated result.
- **Plugins.** A grammar-only local plugin loads; a local grammar overrides
  a built-in by name, alias and scope name; two clashing local plugins warn;
  a cross-plugin `include` resolves.
- **Style check.** Each new rule fires and passes on the cases it names, and
  every built-in style passes with no warnings.
- **Theme.** A browser test confirms every kind's colour resolves to its
  token in light, dark and both toggle states.
- **Notices.** `THIRD_PARTY_NOTICES` names every shipped grammar's source
  and licence and both new modules, and the existing notices test lists
  them.
- **Size budget.** The binary's growth stays under a recorded figure.

## Documentation

- The guide's `code-blocks` section gains **Highlighting**: when a fence is
  highlighted, aliases, opting out, `--list-languages`, and supplying a
  language md2html does not ship.
- The guide's `styles` section gains the `grammars` manifest key and the
  `--syntax-*` tokens, and **Checking a style** gains the new rules.
- A new silent trap: **a fence whose language has no grammar renders
  plain**, with no warning unless the name is a near miss of a known one.
  `--list-languages` shows what exists.
- The changelog records the scope-to-kind table as part of the output, so a
  later change to it is announced.

## Alternatives considered

- **chroma** (Go, about 290 XML lexers plus about 20 written in Go).
  Mature, and its XML lexers load at runtime. Rejected: its lexer format is
  chroma's own, so authors could not reuse the TextMate grammars every
  editor ships, and its Go-coded lexers are exactly the per-language code
  decision 2 rules out.
- **nuri** (a Go port of Shiki running Oniguruma compiled to WASM). Matched
  VS Code exactly, like textmate-go. Not chosen: +4.6 MB, a WASM runtime
  (wazero) as a new dependency, and 7x slower warm than textmate-go even with
  its fastest setting. It remains the fallback engine behind the interface.
- **Shiki's JavaScript engine inside goja**, the engine md2html already
  embeds for charts. Smallest (+0.26 MB), but 40x slower than textmate-go,
  3.5 s to warm each VM, and 99.67% accurate: goja's RegExp lacks the `d`
  and `v` flags and panics on `\u{…}` escapes, so it needed two shims and
  lost Unicode classes. It also found a lookahead bug in regexp2 v2.8.1.
- **A `md2html grammars fetch` command** pulling grammars from the
  collection on demand. Rejected (decision 6): it would be md2html's first
  network code, against `md2html never fetches`, to save about 1 MB.
- **VS Code themes applied at build time**, as Shiki does, writing each
  span's light and dark colours as inline CSS variables. Rejected (decision
  7): colours would bypass styles and `--check-style`, and pages would be
  heavier.
- **Highlighting in the browser** (highlight.js, Prism). A script on every
  page and every Artifact, and the built HTML stays plain.
- **Lexers as external commands** (wrapping Pygments). Output would depend
  on what the build machine has installed, and md2html would stop being one
  binary.
- **Tree-sitter.** Grammars are compiled C parsers, and Go bindings need
  cgo.

## Open questions for review

1. **The kind table.** Thirteen kinds is this design's call. Fewer, such
   as folding `property` into `attribute`, means fewer tokens per style;
   more means palettes that are harder to make pass the style check.
2. **YAML keys come out as `tag`.** Following decision 2 strictly means the
   table may not special-case `entity.name.tag.yaml`. The alternative is a
   short list of language-suffixed exceptions, which is per-language data,
   though not code.
3. **The near-miss warning's threshold** (distance 2, four letters or more)
   is a guess to be checked against real fence tags.

## Spike evidence

A throwaway spike on 2026-10-14 compared three pure-Go routes with VS Code's
own tokenizer, `vscode-textmate` 9.3.2 with `vscode-oniguruma` 2.0.1, as
the reference. Each route tokenized about 2,000 lines of real-world code in
seven languages (Python, shell, JSON, Go, diff, YAML, TOML), several files
with non-ASCII text. They used the same grammar files, from `tm-grammars`
1.32.22. A second, small set of hand-written files exercised nested
generics, f-strings, heredocs, YAML block scalars and anchors. Every
token's full scope stack was compared character by character.

| | textmate-go | nuri | Shiki JS engine in goja |
|---|---|---|---|
| Match, seven samples | 100% | 100% | 99.67% |
| Match, hard set | 100% | 100% | not run |
| Warm, all samples (reference: 89 ms in Node) | 23 ms | 164 ms | about 1 s |
| 8,000-line file | 139 ms | 1.0 s | 6.5 s |
| Binary growth | +1.2 MB | +4.6 MB | +0.26 MB |
| Deterministic | yes | yes | yes |

Each 100% result was checked twice: a fresh run of the route's program, and
a deliberately corrupted output to confirm the comparison fails when it
should. The grammar collection measured 242 grammars, 200 of them
permissively licensed, 9.7 MB raw and 1.19 MB gzipped.
