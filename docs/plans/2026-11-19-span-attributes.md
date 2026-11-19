# Span Attributes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A bracketed span keeps the attributes a link block keeps, so `[beta]{.chip title="May change before 1.0."}` renders `<span class="chip" title="May change before 1.0.">beta</span>`. A block that isn't clean attribute syntax stays as written, and the build warns when the author can fix it.

**Architecture:** `parseAttrs` starts recording which keys were written bare. One pure function, `spanBlock`, decides what a span's block means: the attributes to write, or that it stays literal and why. `spanNode` and `stripChipTokens` both call it, so the page and its title cannot disagree. An unexported `chips(warn)` raises the warning. The exported `Chips()` becomes `chips(nil)`, so the API doesn't change, and `builtins` passes its `warn`.

**Tech Stack:** Go 1.26; `golang.org/x/net/html`; goldmark's `renderer/html` `GlobalAttributeFilter`.

**Spec:** `docs/specs/2026-11-19-span-attributes-design.md`. §1 says what a span keeps, §2 when a block stays literal and when it warns, §3 covers page titles and §4 compatibility.

## Global Constraints

- Work on `feat/keep-span-titles` in the user's checkout. Conventional prefixes. Commit bodies are wrapped at 72 columns and start from what was wrong before. No attribution trailer. Nothing is pushed.
- No exported API changes. `Chips()` keeps its signature.
- A kept name is lowercased, then kept if `gmhtml.GlobalAttributeFilter` contains it or it starts with `data-` or `aria-`. Any other safe name is dropped without a warning, and the rest of the block applies.
- Attribute order on the span: `id`, `class`, then the kept names in sorted order.
- Only the body rewrite warns. `stripChipTokens` never does.
- Every warning has an entry in `docs/guide/warnings.md` (`TestEveryWarningIsInTheGuide`).
- Run `gofmt -l .` (empty output), `go vet ./...` and `go test ./...` before every commit.

## Review Focus

1. **A quoted value containing `}`**: `[x]{title="a } b"}`. `chipRe`'s block is `[^}\n]*`, so it stops at the first `}` and would write a truncated title with ` b"}` left over as text. The whole value must reach the title. Pinned in Task 1, which changes `chipRe`.
2. **A quote inside a value**: Markdown removes a backslash before punctuation before Chips ever sees the text. `title="a \"b\""` therefore reaches the parser as `title="a "b""`, which reads as a bare key and stays literal with a warning. The way to write it is with the other quote kind: `title='a "b"'`. Pinned in Task 2, end to end, and stated in the guide in Task 3.
3. **A status word with a literal block**: `[proven]{.chip hidden}` must stay literal as a whole. The regex has already consumed the match, so the bare `[proven]` inside it must not become a chip. Pinned in Task 1.
4. **The same name in two cases**: `{title="a" Title="b"}` must render the same way on every run. Map iteration order must not decide which one wins. Walk `sortedKeys` of the raw keys, so `Title` (sorted first) is overwritten by `title`. Pinned in Task 1.
5. **A heading with a literal block** must warn once, not once from the body and once from the title. Pinned in Task 2.

---

### Task 1: A span keeps its attributes, or stays as written

**Files:**
- Modify: `attrs.go` (`attrToken`, `attrTokens`, `attrs`, `parseAttrs`)
- Modify: `chip.go` (`chipRe`, `spanNode`, `stripChipTokens`, new `spanBlock`)
- Test: `attrs_test.go`, `chip_test.go`

**Interfaces:**
- Produces: `attrs.bare []string`, the keys written with no `=`, in source order, excluding `#id` and `.class`. These keys are still put in `kv` with an empty value, as today, so containers and link blocks are unchanged.
- Produces: `func spanBlock(label, block string) (span spanAttrs, why string, ok bool)` in `chip.go`, where `type spanAttrs struct { id string; classes []string; extra []html.Attribute }`. `extra` holds the kept names, lowercased and sorted. `ok` is false when the block stays literal. `why` is the warning text when the author can fix the block, and is empty when the block stays literal quietly.

- [ ] **Step 1: Write the failing tests**

In `attrs_test.go`:

```go
func TestParseAttrsRecordsBareKeys(t *testing.T) {
	a, _ := parseAttrs(`#i .c b x=1 y=""`)
	if !reflect.DeepEqual(a.bare, []string{"b"}) {
		t.Errorf("bare = %v, want [b]", a.bare)
	}
	if _, ok := a.kv["b"]; !ok {
		t.Errorf("bare key b left kv: %v", a.kv)
	}
}
```

In `chip_test.go`, every case goes through `apply(t, "<p>x "+src+" y</p>", Chips())`:

| Test | `src` | Assertion |
|---|---|---|
| `TestChipsBracketedSpanKeepsTitle` | `[beta]{.chip title="May change before 1.0."}` | contains `<span class="chip" title="May change before 1.0.">beta</span>` |
| `TestChipsBracketedSpanKeepsDataAndAria` | `[x]{data-sort="name" aria-label="L"}` | contains `<span aria-label="L" data-sort="name">x</span>` |
| `TestChipsBracketedSpanAttributeOrder` | `[x]{#i .c title="t" lang="fr"}` | contains `<span id="i" class="c" lang="fr" title="t">x</span>` |
| `TestChipsBracketedSpanLowercasesNames` | `[x]{.c Title="t" ID="i"}` | contains `<span id="i" class="c" title="t">x</span>` |
| `TestChipsBracketedSpanSameNameTwoCases` | `[x]{title="a" Title="b"}` | contains `title="a"`. Run it 20 times in a loop; every run gives the same output |
| `TestChipsBracketedSpanDropsDisallowedName` | `[x]{.chip onclick="alert(1)" href="h" title="t"}` | contains `<span class="chip" title="t">x</span>` |
| `TestChipsBracketedSpanBooleanWithValue` | `[x]{.chip hidden=""}` | contains `<span class="chip" hidden="">x</span>` |
| `TestChipsBracketedSpanStatusWordKeepsTitle` | `[proven]{.chip title="t"}` | contains `<span class="chip chip-proven" title="t">proven</span>` |
| `TestChipsBracketedSpanQuotedBrace` | `[x]{title="a } b"}` | contains `<span title="a } b">x</span>` and no `b&#34;}` |
| `TestChipsBracketedSpanBareKeyStaysLiteral` | each of `[a]{b}`, `[x]{b c}`, `[x]{.chip hidden}`, `[proven]{.chip hidden}` | no `<span`; the source text is present |
| `TestChipsBracketedSpanUnsafeNameStaysLiteral` | `[T]{.chip on"x=1}` | no `<span`; contains `[T]{.chip on` |

Add to `stripChipTokens`'s table-driven tests:

| In | Want |
|---|---|
| `Rollback [beta]{.chip title="a b"}` | `Rollback` |
| `Rollback [x]{title="a } b"}` | `Rollback x` |
| `A [x]{b} B` | `A [x]{b} B` |
| `A [x]{.chip hidden} B` | `A [x]{.chip hidden} B` |
| `A [x]{} B` | `A [x]{} B` (today `A x B`; §3 says a literal block keeps its text) |

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -run 'TestParseAttrsRecordsBareKeys|TestChipsBracketedSpan|TestStripChipTokens' .`
Expected: FAIL. `a.bare` is undefined, which is a compile error, so make that test compile first by adding the field. The span tests then fail on the missing attributes and on spans emitted for bare-key blocks.

- [ ] **Step 3: Record bare keys in `attrs.go`**

Add `eq bool` to `attrToken` and set it in `attrTokens` when a `=` follows the key. In `parseAttrs`, a key with `!tok.eq` that isn't `#…` or `.…` is appended to `a.bare` and still stored in `kv`.

- [ ] **Step 4: Let `chipRe`'s block hold quoted runs**

Change the bracketed-span alternative's block from `([^}\n]*)` to `((?:[^}"'\n]|"(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*')*)`. A quoted `}` then stays inside the block. An unterminated quote fails the match, so the text stays literal. The status-word alternative is unchanged.

- [ ] **Step 5: Implement `spanBlock` and use it**

`spanBlock` applies these rules in order:

1. `parseAttrs` reports no attributes → `ok` false, `why` empty.
2. The block holds bare keys only, meaning no id, no class and no key=value → `ok` false, `why` empty. This is the prose, regex or terminal-markup case.
3. Any key fails `safeAttrName` → `ok` false, with `why` = `fmt.Sprintf("bracketed span [%s]: %q is not an attribute name; leaving the block as text", label, key)`, checking keys in `sortedKeys` order so the same key is named every run.
4. `a.bare` is non-empty → `ok` false, with `why` = `fmt.Sprintf("bracketed span [%s]: %s has no value; write %s=\"\" for a boolean attribute, or the block stays as text", label, k, k)`, where `k` is the first bare key.
5. Otherwise, walk `sortedKeys(a.kv)` and lowercase each key. `id` replaces `a.id`, and `class` appends its fields to the classes. A name passing the global filter or the `data-`/`aria-` prefix test goes into a map keyed by the lowercased name, so a later key in sorted order wins. Anything else is dropped. Build `extra` from that map in sorted order.

Rule 3 runs before rule 4, so a block with both problems names the unsafe name.

`spanNode(label, block)` calls `spanBlock`. It returns nil when `!ok`, and otherwise builds the span from `id`, `classes` (applying the chip modifier rule as today) and `extra`. It ignores `why`, which Task 2 uses.

In `stripChipTokens`, a span whose block is `!ok` writes the whole match `s[loc[0]:loc[1]]` and continues. A span that is `ok` but not a chip writes its label, as today.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./...`
Expected: PASS, including every existing chip, container, link-attribute and compat test.

- [ ] **Step 7: Commit**

```bash
gofmt -l . && go vet ./...
git add attrs.go attrs_test.go chip.go chip_test.go
git commit   # feat: a bracketed span keeps its title and other safe attributes
```

---

### Task 2: Warn when a span's block is left as text

**Files:**
- Modify: `chip.go` (`chipNodes`, `Chips`, new `chips`)
- Modify: `transform.go:49-61` (`builtins`)
- Modify: `docs/guide/warnings.md`
- Test: `chip_test.go`

**Interfaces:**
- Consumes: `spanBlock(label, block string) (spanAttrs, string, bool)` from Task 1.
- Produces: `func chips(warn func(string)) Transform`, where a nil `warn` is silent, and `func chipNodes(s string, warn func(string)) []*html.Node`. `Chips()` returns `chips(nil)`. `builtins` calls `chips(warn)` where it called `Chips()`, and the doc comment there ("Containers and LinkAttrs use it so far") adds Chips.

- [ ] **Step 1: Write the failing tests**

These use `convert(t, src, warn)` from `container_test.go`, collecting warnings into a slice:

| Test | Source | Assertion |
|---|---|---|
| `TestChipsWarnOnUnsafeName` | `x [T]{.chip on"x=1} y\n` | exactly one warning, containing `is not an attribute name` and `on\"x` (`%q`'s escaping) |
| `TestChipsWarnOnBareKeyBesideAttributes` | `x [s]{.chip hidden} y\n` | exactly one warning, containing `hidden=""` |
| `TestChipsQuietOnBareKeysAlone` | `[a]{b} [c]{d e} [f]{}\n` | no warnings |
| `TestChipsBackslashQuoteInValueWarns` | `x [s]{.chip title="a \"b\""} y\n` | output has no `<span`; one warning containing `has no value` |
| `TestChipsSingleQuotesHoldADoubleQuote` | `x [s]{.chip title='a "b"'} y\n` | output contains `title="a &#34;b&#34;"` |

`TestChipsWarnOnceInHeading` calls `Convert([]byte("## A [x]{.chip hidden} b\n"), Options{CSS: "/**/", Warn: w})`. It must not be a fragment, so that title extraction runs. Assert exactly one warning.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -run 'TestChipsWarn|TestChipsQuiet|TestChipsBackslash|TestChipsSingleQuotes' .`
Expected: the warning tests FAIL with no warnings raised. `TestChipsSingleQuotesHoldADoubleQuote` already passes from Task 1.

- [ ] **Step 3: Implement `chips(warn)`, thread `warn` through `chipNodes` and wire `builtins`**

In the bracketed-span branch, `chipNodes` calls `spanBlock` itself. When the result is `!ok` with a non-empty `why` and `warn` is non-nil, it calls `warn(why)`, then declines the match with a real nil. Keep `spanNode` as the builder of the node from an `ok` result, so that path has a single owner.

- [ ] **Step 4: Add the guide entries**

In `docs/guide/warnings.md`, after the `**… cannot be set from an attribute block**` entry, add:

```markdown
**bracketed span … is not an attribute name** — a span's block holds a
name with a character no attribute name can have, so the block stays on
the page as text. Fix or remove the name.

**bracketed span … has no value** — a span's block holds a bare word
beside a `.class` or `#id`, so the block stays on the page as text. Write
a boolean attribute with a value, `hidden=""`. A quote inside a value
loses its backslash to Markdown, which also lands here; write the value
in the other kind of quote, `title='a "b"'`.
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./...`
Expected: PASS, including `TestEveryWarningIsInTheGuide`.

- [ ] **Step 6: Commit**

```bash
gofmt -l . && go vet ./...
git add chip.go chip_test.go transform.go docs/guide/warnings.md
git commit   # feat: warn when a bracketed span's block is left as text
```

---

### Task 3: The guide, the changelog and the TODO

**Files:**
- Modify: `docs/guide/inline-marks.md:28-31`
- Modify: `CHANGELOG.md` (a new `## Unreleased` section above `## v0.15.0`)
- Modify: `TODO.md` (remove the first "Bracketed spans" item)
- Test: none new; `go test ./...` covers the guide (`TestGuideChipClassesAreStyled`, the guide section tests)

- [ ] **Step 1: Rewrite the guide paragraph**

Replace "and only classes and an id are kept." with text covering these points:

- The span keeps the attributes a link block keeps: `title`, `lang`, `dir`, `role` and the other global attributes, plus any `data-` or `aria-` name. Any other name is dropped. Show the example `[beta]{.chip title="May change before 1.0."}` → `<span class="chip" title="May change before 1.0.">beta</span>`.
- A boolean attribute takes a value: `hidden=""`.
- A quote inside a value is written with the other kind of quote.
- A block with a bare word, or with a name no attribute can have, stays as text. Beside a class or id, it also warns (link to `warnings.md`).

- [ ] **Step 2: Add the changelog entry**

Add `## Unreleased`, in the format the v0.15.0 section uses:

- **Added:** a bracketed span keeps `title` and the other global attributes, and any `data-` or `aria-` name, so a chip can carry a hover explanation.
- **Changed:** the three items in spec §4, in the spec's words. A block with a bare key stays as text, and warns beside a class or id. A kept key=value pair now appears where it was dropped before. A block with an unsafe name stays as text and warns. Also note that a literal block keeps its text in the page title.

- [ ] **Step 3: Remove the done TODO item**

Delete the first item under "Bracketed spans". The heading item stays under it, and the "Build" section stays.

- [ ] **Step 4: Verify**

Run: `go test ./... && go run ./cmd/md2html --guide inline-marks | grep -n 'title='`
Expected: PASS, and the guide's new example line is printed.

- [ ] **Step 5: Commit**

```bash
git add docs/guide/inline-marks.md CHANGELOG.md TODO.md
git commit   # docs: span attributes in the guide and the changelog
```
