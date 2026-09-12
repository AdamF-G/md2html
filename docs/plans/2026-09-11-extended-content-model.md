# Extended Content Model Implementation Plan

**Goal:** Close the six content-model gaps that make a documentation corpus render worse than it reads — container variety and brace-free container syntax, inline status chips, `§N.M` autolinking, a per-page table of contents, front-matter/subtitle title derivation, and captioned code fences.

**Architecture:** Five of the six are tree transforms over the already-parsed HTML, joining `TableScroll`/`HeadingAnchors`/`ExternalLinks` in the existing `Transform` list — which is why they can share one plan: they are one layer, with one ordering constraint between them. Two pieces sit outside that layer: front matter is stripped from the source bytes before goldmark sees them, and code-fence captions need a goldmark node renderer, because goldmark discards the rest of the info string before an HTML tree exists. A new `Options.Warn` sink carries the one diagnostic these features need to report.

**Tech Stack:** Go 1.27.1, `golang.org/x/net/html` (already a dependency), `github.com/yuin/goldmark` v1.8.6, `github.com/stefanfritsch/goldmark-fences` v1.0.0, stdlib `regexp`. No new module dependencies.

**Spec:** `docs/specs/2026-09-11-extended-content-model.md` — items 2, 4, 5, 6, 7, 8.

## Global Constraints

- No new module dependencies.
- `gofmt` clean, `go vet ./...` clean, `go test ./...` green after every task.
- **Nothing already documented may change behavior.** `::: {.callout}` must keep emitting `<div class="callout">`; heading slugs for chip-free headings must stay byte-identical (`docs/authoring.md` promises stable, predictable ids); `--fragment` output shape is unchanged apart from new body content.
- `Builtins()` keeps its existing no-argument signature. It appears in `README.md` as public API; an internal `builtins(warn func(string))` carries the warning sink instead (Task 1).
- Transform order is a correctness constraint, not a style choice. The fixed order is: `Containers`, `TableScroll`, `Chips`, `HeadingAnchors`, `SectionLinks`, `TOC`, `ExternalLinks`. `Chips` must precede `HeadingAnchors` so a chip is already a `<span class="chip">` when slugs are computed; `SectionLinks` and `TOC` must follow `HeadingAnchors` because both resolve against assigned ids.
- Every new transform must be a no-op on a document that does not use its feature, and must degrade to the literal source text rather than to a broken link or an empty element.
- Comments explain *why*, matching the density and voice of `transform.go`.

## Two spec claims that are wrong, and what to build instead

Both were checked against the binary before this plan was written. They change what two tasks build, so they are recorded here rather than buried in a task.

1. **Item 2's title-on-the-fence-line cannot be done for the braced form.** `goldmark-fences` consumes the attribute block and lets the rest of the opening line fall into the container's first paragraph, merged with the following source line. So

   ```
   ::: {.callout} Title
   body
   ```

   and

   ```
   ::: {.callout}
   Title
   body
   ```

   both arrive at the HTML tree as the identical `<p>Title\nbody</p>`. Nothing downstream can tell a title from a first line of body. The **bare** form has no such ambiguity: once the leading word has been matched against the kind vocabulary, the first newline in that paragraph *is* the end of the opening fence line. Task 3 therefore lifts titles for the bare form only, and the braced form keeps exactly today's semantics — which is also the backward-compatible choice. Supporting titles on the braced form would mean replacing `goldmark-fences` with an in-repo block parser; that is a separate design, not a step in this plan.

2. **Item 8's stated degradation is wrong, and the real one is worse.** The spec expects an unconsumed caption to sit "visibly wrong" in the language slot. It does not: goldmark takes the first word of the info string as the language and discards the remainder without a word, so ` ```go caption="x.go" ` renders today byte-for-byte identically to a bare ` ```go `. The caption vanishes silently. That makes item 8 a silent-failure fix, not a cosmetic one — worth knowing when ranking it. (Item 1's aside that titled code blocks are an "existing fenced-language special case" is also wrong: `newParser` special-cases mermaid and nothing else. Item 1 is out of scope here regardless.)

---

### Task 1: A warning sink for conversion

Conversion currently has no way to report a non-fatal problem: `Transform.Fn` returns only `error`, and an error aborts the whole document. Task 2 needs to warn about a container naming a kind that does not exist — the spec's stated "the current failure mode is the actual problem" — so the channel has to exist first.

**Files:**
- Modify: `md2html.go` (`Options`, `Convert`), `transform.go` (`Builtins`), `cmd/md2html/main.go` (`run`, `buildOptions`)
- Test: `md2html_test.go` (append)

**Interfaces:**
- Produces:
  - `Options.Warn func(string)` — called synchronously from inside `Convert` for each non-fatal problem.
  - `func builtins(warn func(string)) []Transform` (unexported) — the ordered default list.
  - `Builtins() []Transform` keeps its signature, returning `builtins(nil)`.
  - `buildOptions` in `package main` gains a trailing `warn func(string)` parameter.

- [ ] **Step 1: Write the failing test**

Append to `md2html_test.go`:

```go
// The sink exists so a transform can report a problem without aborting the
// document. Nothing warns yet — Task 2 is the first caller — so this
// asserts only that a plain document stays silent and that Convert accepts
// and threads the callback.
func TestConvertWarnSinkSilentOnCleanInput(t *testing.T) {
	var got []string
	out, err := Convert([]byte("# Title\n\ntext\n"), Options{
		Fragment: true,
		Warn:     func(m string) { got = append(got, m) },
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("warned on clean input: %v", got)
	}
	if !strings.Contains(string(out), "<h1") {
		t.Errorf("no heading in output: %s", out)
	}
}

// A nil sink must be safe: every existing caller passes one.
func TestConvertNilWarnSinkIsSafe(t *testing.T) {
	if _, err := Convert([]byte("# T\n"), Options{Fragment: true}); err != nil {
		t.Fatalf("Convert: %v", err)
	}
}

// Builtins keeps its documented no-argument signature; README shows it.
func TestBuiltinsSignatureUnchanged(t *testing.T) {
	if len(Builtins()) == 0 {
		t.Error("Builtins() returned nothing")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestConvertWarnSink|TestConvertNilWarn|TestBuiltinsSignature' -v`
Expected: FAIL — `unknown field Warn in struct literal of type Options`.

- [ ] **Step 3: Write the implementation**

In `md2html.go`, add to `Options`, after `LinkMap`:

```go
	// Warn, when non-nil, receives one message per non-fatal problem found
	// while converting this document — a container naming a kind that does
	// not exist, so far.
	//
	// Convert never writes to stderr itself: it is a library, and the CLI
	// emits every document's output in parallel, so a transform printing
	// directly would interleave with other documents' lines. The callback
	// is invoked synchronously on the calling goroutine, so a caller may
	// append to an unsynchronized per-document slice.
	//
	// It reaches transforms only through the default list. A caller who
	// supplies Transforms builds that list themselves and is responsible
	// for passing the sink to the constructors that take one.
	Warn func(string)
```

In `Convert`, change the default-list branch:

```go
	transforms := opt.Transforms
	if transforms == nil {
		transforms = builtins(opt.Warn)
	}
```

In `transform.go`, replace `Builtins`:

```go
// Builtins returns the transforms enabled by default.
//
// It takes no arguments and reports nothing: it is the documented public
// door for callers assembling their own transform list (see README), and
// changing its signature would break them. Convert calls builtins directly
// so that a caller who sets Options.Warn gets diagnostics.
func Builtins() []Transform {
	return builtins(nil)
}

// builtins is the ordered default list. The order is load-bearing:
// Containers restructures whole blocks before anything inspects them,
// Chips must run before HeadingAnchors so a status marker is already a
// span when slugs are computed, and SectionLinks and TOC must run after it
// because both resolve against assigned ids.
func builtins(warn func(string)) []Transform {
	return []Transform{
		TableScroll(),
		HeadingAnchors(),
		ExternalLinks(),
	}
}
```

`warn` is unused for now; Go permits an unused function parameter. Later tasks insert their transforms into this list in the documented order.

In `cmd/md2html/main.go`, collect warnings per document inside the emit
goroutine and print them with the crawler's. Extend the `outcome` struct:

```go
	type outcome struct {
		src      string
		res      md2html.WriteResult
		err      error
		warnings []string
	}
```

Inside the goroutine, replace the `Convert` call site:

```go
			d := res.Docs[i]
			src, err := os.ReadFile(d.Src)
			if err != nil {
				results[i] = outcome{src: d.Src, res: md2html.WriteRefused, err: err}
				return
			}
			// One slice per document, written only by this goroutine, so
			// the sink needs no locking and lines from two documents can
			// never interleave.
			var warnings []string
			opts := buildOptions(d, *fragment, css, *noTable, *noAnchor, *noExt,
				func(m string) { warnings = append(warnings, m) })
			out, err := md2html.Convert(src, opts)
			if err != nil {
				results[i] = outcome{src: d.Src, res: md2html.WriteRefused, err: err, warnings: warnings}
				return
			}
			wr, err := md2html.SafeWrite(d.Out, out)
			results[i] = outcome{src: d.Src, res: wr, err: err, warnings: warnings}
```

Print them in the reporting loop, and count them in the summary. Replace
the summary block's counters and loop:

```go
	written, refused, failed, warned := 0, 0, 0, 0
	for i, o := range results {
		for _, w := range o.warnings {
			warned++
			fmt.Fprintf(stderr, "md2html: %s: %s\n", o.src, w)
		}
		switch {
		case o.err != nil:
			failed++
			fmt.Fprintf(stderr, "md2html: %s: %v\n", o.src, o.err)
		case o.res == md2html.WriteRefused:
			refused++
			fmt.Fprintf(stderr, "md2html: refusing to overwrite %s (not generated by md2html)\n",
				res.Docs[i].Out)
		default:
			written++
		}
	}
```

and change the final summary line to include both warning sources:

```go
	fmt.Fprintf(stderr, "md2html: %d written, %d refused, %d failed, %d warning(s)\n",
		written, refused, failed, len(res.Warnings)+warned)
```

Finally update `buildOptions`:

```go
// buildOptions assembles per-document conversion options from the flags.
func buildOptions(d md2html.Doc, fragment bool, css string,
	noTable, noAnchor, noExt bool, warn func(string)) md2html.Options {

	var ts []md2html.Transform
	if !noTable {
		ts = append(ts, md2html.TableScroll())
	}
	if !noAnchor {
		ts = append(ts, md2html.HeadingAnchors())
	}
	if !noExt {
		ts = append(ts, md2html.ExternalLinks())
	}

	return md2html.Options{
		Fragment:   fragment,
		SourcePath: d.Src,
		CSS:        css,
		Transforms: ts,
		LinkMap:    d.LinkMap,
		Warn:       warn,
	}
}
```

Add `"strings"` to `md2html_test.go`'s imports if absent.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS — whole suite, including `e2e_test.go`'s `append(Builtins(), ...)`.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add md2html.go transform.go md2html_test.go cmd/md2html/main.go
git commit -m "feat: add Options.Warn sink for non-fatal conversion problems"
```

---

### Task 2: Brace-free container syntax and the shipped kind vocabulary

Spec item 2's core: accept `::: kind` as an alias for `::: {.kind}`, give a fixed set of kinds real classes, and warn instead of silently emitting an unclassed div. Also drops `data-fence`, an internal bookkeeping attribute the fence library leaks into every container's output.

The bare form is recognizable in the tree because `goldmark-fences` requires attributes to open a div: given `::: warning`, `parser.ParseAttributes` finds none, so the node carries *only* `data-fence`, and the word `warning` survives as the leading text of the container's first paragraph. A container carrying any other attribute — `::: {.callout}`, `::: {#id}` — is the braced form and is never sniffed for a leading word, which is what keeps the check from firing on ordinary prose.

`<details>` kinds and title lifting land in Task 3; this task gives every kind its element and classes.

**Files:**
- Create: `container.go`, `container_test.go`
- Modify: `transform.go` (`builtins`), `cmd/md2html/main.go` (`buildOptions`), `default.css`
- Test: `container_test.go`

**Interfaces:**
- Consumes: `attr`, `setAttr`, `hasClass`, `walk` (existing, `transform.go` / `tree.go`); `Options.Warn` plumbing (Task 1).
- Produces:
  - `type containerKind struct { tag, class, prefix, fallback string }`
  - `var containerKinds map[string]containerKind`
  - `func Containers(warn func(string)) Transform` — `Name: "containers"`.
  - `func firstWord(s string) (word, rest string)`

- [ ] **Step 1: Write the failing test**

Create `container_test.go`:

```go
package md2html

import (
	"strings"
	"testing"
)

// convert is a whole-pipeline helper: container recognition depends on
// exactly how goldmark-fences shapes the tree, so these tests must start
// from Markdown, not from hand-written HTML.
func convert(t *testing.T, src string, warn func(string)) string {
	t.Helper()
	out, err := Convert([]byte(src), Options{Fragment: true, CSS: "/**/", Warn: warn})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return string(out)
}

// The documented braced form must not change. This is the regression guard
// for every existing document in every corpus.
func TestContainerBracedCalloutUnchanged(t *testing.T) {
	got := convert(t, "::: {.callout}\nhello\n:::\n", nil)
	if !strings.Contains(got, `<div class="callout">`) {
		t.Errorf("braced callout changed\ngot: %s", got)
	}
}

// The trap docs/authoring.md warns about, fixed.
func TestContainerBareNameGetsClass(t *testing.T) {
	got := convert(t, "::: callout\nhello\n:::\n", nil)
	if !strings.Contains(got, `<div class="callout">`) {
		t.Errorf("bare name did not become a callout\ngot: %s", got)
	}
	if strings.Contains(got, "callout\nhello") || strings.Contains(got, ">callout") {
		t.Errorf("kind word left in the body\ngot: %s", got)
	}
}

func TestContainerWarningKindGetsVariantClass(t *testing.T) {
	got := convert(t, "::: warning\ncareful\n:::\n", nil)
	if !strings.Contains(got, `class="callout callout-warning"`) {
		t.Errorf("no warning variant\ngot: %s", got)
	}
}

func TestContainerCardKind(t *testing.T) {
	got := convert(t, "::: card\nhighlighted\n:::\n", nil)
	if !strings.Contains(got, `<div class="card">`) {
		t.Errorf("no card\ngot: %s", got)
	}
}

// A braced kind from the shipped vocabulary gets the same treatment as the
// bare spelling, so the two forms cannot drift apart.
func TestContainerBracedWarningGetsVariantClass(t *testing.T) {
	got := convert(t, "::: {.warning}\ncareful\n:::\n", nil)
	if !strings.Contains(got, `class="callout callout-warning"`) {
		t.Errorf("braced warning not normalized\ngot: %s", got)
	}
}

// The actual problem the spec names: silence. An unknown bare name must
// say so.
func TestContainerUnknownBareNameWarns(t *testing.T) {
	var msgs []string
	got := convert(t, "::: kaution\noops\n:::\n", func(m string) { msgs = append(msgs, m) })
	var found bool
	for _, m := range msgs {
		if strings.Contains(m, "kaution") {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning for unknown kind, got %v", msgs)
	}
	// Still degrades to an unclassed div, exactly as today.
	if !strings.Contains(got, "<div>") {
		t.Errorf("unknown kind did not degrade to a bare div\ngot: %s", got)
	}
}

// An arbitrary braced class stays as inert as it has always been, and must
// not warn: the author chose that class deliberately and supplies their own
// CSS for it.
func TestContainerArbitraryBracedClassIsSilentAndPreserved(t *testing.T) {
	var msgs []string
	got := convert(t, "::: {.house-style}\nx\n:::\n", func(m string) { msgs = append(msgs, m) })
	if len(msgs) != 0 {
		t.Errorf("warned about an explicit class: %v", msgs)
	}
	if !strings.Contains(got, `<div class="house-style">`) {
		t.Errorf("explicit class not preserved\ngot: %s", got)
	}
}

// A braced container carrying only an id is the braced form, so its first
// word must never be sniffed as a kind.
func TestContainerIdOnlyIsNotSniffed(t *testing.T) {
	var msgs []string
	got := convert(t, "::: {#note}\ncallout is a word\n:::\n", func(m string) { msgs = append(msgs, m) })
	if len(msgs) != 0 {
		t.Errorf("warned about an id-only container: %v", msgs)
	}
	if !strings.Contains(got, "callout is a word") {
		t.Errorf("body text was eaten\ngot: %s", got)
	}
}

// The fence library's internal bookkeeping must not reach the page.
func TestContainerDropsDataFenceAttribute(t *testing.T) {
	got := convert(t, "::: {.callout}\nx\n:::\n", nil)
	if strings.Contains(got, "data-fence") {
		t.Errorf("data-fence leaked into output\ngot: %s", got)
	}
}

// Nesting already works and must keep working.
func TestContainerNestingPreserved(t *testing.T) {
	got := convert(t, ":::: callout\n::: warning\ninner\n:::\n::::\n", nil)
	if !strings.Contains(got, `<div class="callout">`) ||
		!strings.Contains(got, `class="callout callout-warning"`) {
		t.Errorf("nesting broken\ngot: %s", got)
	}
}

func TestFirstWord(t *testing.T) {
	cases := []struct{ in, word, rest string }{
		{"warning", "warning", ""},
		{"warning Be careful", "warning", "Be careful"},
		{"warning\nbody", "warning", "\nbody"},
		{"  warning  x", "warning", "x"},
		{"", "", ""},
	}
	for _, c := range cases {
		w, r := firstWord(c.in)
		if w != c.word || r != c.rest {
			t.Errorf("firstWord(%q) = (%q, %q), want (%q, %q)", c.in, w, r, c.word, c.rest)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestContainer|TestFirstWord' -v`
Expected: FAIL — `undefined: firstWord`, and the class assertions fail because nothing normalizes containers yet.

- [ ] **Step 3: Write the implementation**

Create `container.go`:

```go
package md2html

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// containerKind describes one shipped container kind: the element it
// becomes, the classes it carries, and — for the collapsible kinds — how a
// title is presented in its summary line.
type containerKind struct {
	// tag is "div" or "details".
	tag string
	// class is the full class attribute the container ends up with.
	class string
	// prefix labels a collapsible kind's summary ahead of any title.
	prefix string
	// fallback is the summary text when the author gave no title.
	fallback string
}

// containerKinds is the shipped vocabulary: a fixed set, not an open one.
// A class outside it stays exactly as inert as it is today, because a team
// that wrote one chose it deliberately and ships its own CSS; a bare name
// outside it is a typo, and saying so is the whole point — the silent
// unclassed div is the failure this replaces.
var containerKinds = map[string]containerKind{
	"callout": {tag: "div", class: "callout"},
	"warning": {tag: "div", class: "callout callout-warning"},
	"card":    {tag: "div", class: "card"},
	"aside":   {tag: "details", class: "container aside", fallback: "Aside"},
	"example": {tag: "details", class: "container example", prefix: "Example"},
}

// firstWord returns the leading whitespace-delimited word of s and the
// remainder with the separating spaces removed. A newline ends the word but
// is kept in the remainder: for a bare container the newline is the end of
// the opening fence line, and Task 3's title lifting needs to see it.
func firstWord(s string) (word, rest string) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	j := i
	for j < len(s) && s[j] != ' ' && s[j] != '\t' && s[j] != '\n' {
		j++
	}
	word = s[i:j]
	for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
		j++
	}
	return word, s[j:]
}

// fenceDivs collects every container the fence extension produced, before
// any of them is modified. Collect-then-mutate, the same discipline
// TableScroll uses: the transform replaces nodes, and a walk that is also
// rewriting the tree it walks is where subtle ordering bugs live.
func fenceDivs(root *html.Node) []*html.Node {
	var out []*html.Node
	walk(root, func(n *html.Node) {
		if n.Type != html.ElementNode || n.DataAtom != atom.Div {
			return
		}
		if _, ok := attr(n, "data-fence"); ok {
			out = append(out, n)
		}
	})
	return out
}

// removeAttr deletes an attribute if present.
func removeAttr(n *html.Node, key string) {
	for i, a := range n.Attr {
		if a.Key == key {
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			return
		}
	}
}

// firstParagraph returns the container's first element child if it is a
// paragraph whose own first child is a text node — the shape a bare
// container's opening fence line always produces.
func firstParagraph(div *html.Node) *html.Node {
	for c := div.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) == "" {
			continue // inter-element whitespace
		}
		if c.Type == html.ElementNode && c.DataAtom == atom.P &&
			c.FirstChild != nil && c.FirstChild.Type == html.TextNode {
			return c
		}
		return nil
	}
	return nil
}

// Containers normalizes fenced containers. It accepts the brace-free
// "::: kind" form as an alias for "::: {.kind}", maps the shipped kinds
// onto their stylesheet classes, warns when a brace-free name is not one of
// them, and drops the fence library's internal data-fence attribute.
//
// warn may be nil.
func Containers(warn func(string)) Transform {
	if warn == nil {
		warn = func(string) {}
	}
	return Transform{Name: "containers", Fn: func(root *html.Node) error {
		for _, div := range fenceDivs(root) {
			removeAttr(div, "data-fence")

			if cls, ok := attr(div, "class"); ok {
				// Braced form. Normalize a shipped kind; leave anything
				// else exactly as written. Fields, not a split on space: a
				// class attribute of nothing but whitespace has no first
				// token to index.
				if f := strings.Fields(cls); len(f) > 0 {
					if k, known := containerKinds[f[0]]; known {
						applyKind(div, k, nil)
					}
					continue
				}
			}
			if len(div.Attr) > 0 {
				// Braced, but with something other than a class — an id,
				// say. Not the brace-free form, so do not sniff its text.
				continue
			}

			p := firstParagraph(div)
			if p == nil {
				warn("container has no class and no recognizable kind name")
				continue
			}
			word, rest := firstWord(p.FirstChild.Data)
			k, known := containerKinds[word]
			if !known {
				warn(fmt.Sprintf("unknown container kind %q: emitting an unclassed div "+
					"(known kinds: %s)", word, knownKindList()))
				continue
			}
			p.FirstChild.Data = rest
			applyKind(div, k, p)
		}
		return nil
	}}
}

// applyKind gives a container its element and classes. p is the first
// paragraph for a brace-free container, whose kind word has already been
// stripped, or nil for a braced one; Task 3 uses it to lift a title.
func applyKind(div *html.Node, k containerKind, p *html.Node) {
	setAttr(div, "class", k.class)
}

// knownKindList renders the vocabulary for a warning message, in a stable
// order so the text does not change between runs.
func knownKindList() string {
	names := make([]string, 0, len(containerKinds))
	for n := range containerKinds {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
```

Add `"sort"` to the import block.

In `transform.go`, put `Containers` at the head of `builtins`:

```go
func builtins(warn func(string)) []Transform {
	return []Transform{
		Containers(warn),
		TableScroll(),
		HeadingAnchors(),
		ExternalLinks(),
	}
}
```

In `cmd/md2html/main.go`, prepend it in `buildOptions` too — it is not
covered by any `--no-*` flag:

```go
	var ts []md2html.Transform
	ts = append(ts, md2html.Containers(warn))
	if !noTable {
		ts = append(ts, md2html.TableScroll())
	}
```

Add the two new kinds' styling to `default.css`. First add two palette
tokens to **all four** `:root` blocks (the bare one, the
`prefers-color-scheme: dark` one, `:root[data-theme="dark"]`, and
`:root[data-theme="light"]`), matching each block's existing lightness:

```css
/* light blocks */
  --warn: #b4530a;
  --ok: #2f6f43;
/* dark blocks */
  --warn: #f0915a;
  --ok: #6fbe89;
```

Then append, next to the existing `.callout` rule:

```css
.callout.callout-warning { border-left-color: var(--warn); }

/* An always-visible highlighted card: a box rather than a margin rail, for
   content that is a thing in its own right rather than an aside on the
   paragraph above it. */
.card {
  border: 1px solid var(--rule);
  background: var(--code-bg);
  border-radius: 8px;
  padding: 1rem 1.25rem;
  margin: 1.5rem 0;
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS. `applyKind` currently ignores `p`; `go vet` accepts an
unused parameter, and Task 3 uses it.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add container.go container_test.go transform.go cmd/md2html/main.go default.css
git commit -m "feat: accept brace-free container syntax and ship a kind vocabulary"
```

---

### Task 3: Container titles and collapsible kinds

Completes item 2: a title on the opening fence line, and the two `<details>` kinds. Titles are lifted for the brace-free form only — see "Two spec claims that are wrong" above for why the braced form cannot support one.

**Files:**
- Modify: `container.go` (`applyKind`), `default.css`
- Test: `container_test.go` (append)

**Interfaces:**
- Consumes: `containerKind`, `applyKind`, `firstWord` (Task 2).
- Produces:
  - `func detachTitle(p *html.Node) []*html.Node` — removes and returns the opening fence line's inline nodes.
  - `applyKind` now rebuilds the node as `<details>` for collapsible kinds.

- [ ] **Step 1: Write the failing test**

Append to `container_test.go`:

```go
func TestContainerBareTitleBecomesTitleParagraph(t *testing.T) {
	got := convert(t, "::: callout Read this first\nbody text\n:::\n", nil)
	if !strings.Contains(got, `<p class="container-title">Read this first</p>`) {
		t.Errorf("no title paragraph\ngot: %s", got)
	}
	if !strings.Contains(got, "body text") {
		t.Errorf("body lost\ngot: %s", got)
	}
	if strings.Contains(got, "Read this first\nbody text") {
		t.Errorf("title not separated from body\ngot: %s", got)
	}
}

// Inline Markdown in the title is already parsed by the time the transform
// sees it, so it survives as markup rather than as escaped text.
func TestContainerTitleKeepsInlineMarkdown(t *testing.T) {
	got := convert(t, "::: callout Read `this` first\nbody\n:::\n", nil)
	if !strings.Contains(got, "<code>this</code>") {
		t.Errorf("inline markdown lost from title\ngot: %s", got)
	}
}

// No title on the fence line means no title paragraph — not an empty one.
func TestContainerNoTitleEmitsNoTitleParagraph(t *testing.T) {
	got := convert(t, "::: callout\nbody\n:::\n", nil)
	if strings.Contains(got, "container-title") {
		t.Errorf("emitted an empty title\ngot: %s", got)
	}
}

// A title with a blank line after it leaves the first paragraph empty; it
// must be removed rather than rendered as <p></p>.
func TestContainerTitleWithBlankLineDropsEmptyParagraph(t *testing.T) {
	got := convert(t, "::: callout Heads up\n\nbody\n:::\n", nil)
	if strings.Contains(got, "<p></p>") {
		t.Errorf("empty paragraph left behind\ngot: %s", got)
	}
	if !strings.Contains(got, `<p class="container-title">Heads up</p>`) {
		t.Errorf("no title\ngot: %s", got)
	}
}

// The braced form keeps today's semantics exactly: its first line is body,
// because nothing can tell it from a title.
func TestContainerBracedFormNeverLiftsATitle(t *testing.T) {
	got := convert(t, "::: {.callout}\nfirst line\nsecond line\n:::\n", nil)
	if strings.Contains(got, "container-title") {
		t.Errorf("braced form lifted a title\ngot: %s", got)
	}
}

func TestContainerAsideIsCollapsible(t *testing.T) {
	got := convert(t, "::: aside Why this matters\nbecause\n:::\n", nil)
	if !strings.Contains(got, `<details class="container aside">`) {
		t.Errorf("not a details element\ngot: %s", got)
	}
	if !strings.Contains(got, "<summary>Why this matters</summary>") {
		t.Errorf("no summary\ngot: %s", got)
	}
	if !strings.Contains(got, "because") {
		t.Errorf("body lost\ngot: %s", got)
	}
}

// A collapsible kind with no title still needs a summary, or the disclosure
// triangle has nothing to label it.
func TestContainerAsideWithoutTitleUsesFallbackSummary(t *testing.T) {
	got := convert(t, "::: aside\nbecause\n:::\n", nil)
	if !strings.Contains(got, "<summary>Aside</summary>") {
		t.Errorf("no fallback summary\ngot: %s", got)
	}
}

// The example kind is the same shape with a different label prefix.
func TestContainerExamplePrefixesItsSummary(t *testing.T) {
	got := convert(t, "::: example Rolling back\nsteps\n:::\n", nil)
	if !strings.Contains(got, `<details class="container example">`) {
		t.Errorf("not a details element\ngot: %s", got)
	}
	if !strings.Contains(got, "<summary>Example — Rolling back</summary>") {
		t.Errorf("no prefixed summary\ngot: %s", got)
	}
}

func TestContainerExampleWithoutTitleIsJustThePrefix(t *testing.T) {
	got := convert(t, "::: example\nsteps\n:::\n", nil)
	if !strings.Contains(got, "<summary>Example</summary>") {
		t.Errorf("no prefix-only summary\ngot: %s", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestContainer -v`
Expected: FAIL — no `container-title`, no `<details>`; `applyKind` still only sets a class.

- [ ] **Step 3: Write the implementation**

In `container.go`, add `detachTitle` and replace `applyKind`:

```go
// detachTitle removes and returns the inline nodes that made up the rest of
// a brace-free container's opening fence line. The kind word has already
// been stripped from p's leading text node by the caller.
//
// goldmark merges the text after ":::" with the next source line into one
// paragraph, so the first newline among p's direct children is exactly the
// end of the fence line. That boundary is only trustworthy for the
// brace-free form, where the leading word has already proven this paragraph
// began on the fence line; the braced form is ambiguous and never gets here.
//
// Only direct children are scanned. Emphasis opened on the fence line and
// closed on the next one would carry the newline inside an element, and the
// title then runs to the following top-level newline — a title containing a
// newline, which HTML collapses to a space. Harmless, and not worth cloning
// elements across the split to avoid.
func detachTitle(p *html.Node) []*html.Node {
	var title []*html.Node
	for c := p.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.TextNode {
			if i := strings.IndexByte(c.Data, '\n'); i >= 0 {
				head := strings.TrimSpace(c.Data[:i])
				c.Data = c.Data[i+1:]
				if head != "" {
					title = append(title, &html.Node{Type: html.TextNode, Data: head})
				}
				return title
			}
			if strings.TrimSpace(c.Data) == "" {
				// An empty remnant of the kind word's trailing space.
				p.RemoveChild(c)
				c = next
				continue
			}
		}
		p.RemoveChild(c)
		title = append(title, c)
		c = next
	}
	// No newline anywhere: the whole paragraph was the opening fence line.
	return title
}

// applyKind gives a container its element, classes and — for a brace-free
// container with a title on the fence line — its title or summary. p is
// that container's first paragraph with the kind word already stripped, or
// nil for a braced container, which never gets a title.
func applyKind(div *html.Node, k containerKind, p *html.Node) {
	setAttr(div, "class", k.class)

	var title []*html.Node
	if p != nil {
		title = detachTitle(p)
		if p.FirstChild == nil {
			// The fence line was the paragraph's entire content.
			p.Parent.RemoveChild(p)
		}
	}

	if k.tag == "details" {
		toDetails(div, k, title)
		return
	}
	if len(title) > 0 {
		tp := &html.Node{Type: html.ElementNode, DataAtom: atom.P, Data: "p",
			Attr: []html.Attribute{{Key: "class", Val: "container-title"}}}
		for _, n := range title {
			tp.AppendChild(n)
		}
		div.InsertBefore(tp, div.FirstChild)
	}
}

// toDetails rebuilds a container as a <details> with a <summary>.
//
// The element has to be replaced rather than relabeled: x/net/html keys
// rendering off DataAtom and Data, and a <div> cannot simply become a
// <details> in place without leaving one of the two stale.
func toDetails(div *html.Node, k containerKind, title []*html.Node) {
	d := &html.Node{Type: html.ElementNode, DataAtom: atom.Details, Data: "details",
		Attr: []html.Attribute{{Key: "class", Val: k.class}}}

	sum := &html.Node{Type: html.ElementNode, DataAtom: atom.Summary, Data: "summary"}
	switch {
	case k.prefix != "" && len(title) > 0:
		sum.AppendChild(&html.Node{Type: html.TextNode, Data: k.prefix + " — "})
	case k.prefix != "":
		sum.AppendChild(&html.Node{Type: html.TextNode, Data: k.prefix})
	case len(title) == 0:
		sum.AppendChild(&html.Node{Type: html.TextNode, Data: k.fallback})
	}
	for _, n := range title {
		sum.AppendChild(n)
	}
	d.AppendChild(sum)

	for c := div.FirstChild; c != nil; {
		next := c.NextSibling
		div.RemoveChild(c)
		d.AppendChild(c)
		c = next
	}
	div.Parent.InsertBefore(d, div)
	div.Parent.RemoveChild(div)
}
```

One ordering detail: `fenceDivs` collected every container before any was
modified, so replacing an outer `<div>` with a `<details>` cannot orphan an
inner container that is still in the slice — the inner node moves with its
subtree, and `div.Parent` is re-read at replacement time.

Append the collapsible-kind styling to `default.css`:

```css
/* Collapsible containers. <details> gives open/closed state, keyboard
   access and find-in-page expansion natively; a scripted accordion would
   give up all three. */
details.container {
  border: 1px solid var(--rule);
  border-radius: 6px;
  padding: 0.5rem 1rem;
  margin: 1.5rem 0;
}
details.container > summary {
  cursor: pointer;
  font-weight: 600;
  margin: 0.25rem 0;
}
details.container[open] > summary { margin-bottom: 0.75rem; }
details.container.example { border-left: 3px solid var(--accent); }

.container-title { font-weight: 600; margin: 0 0 0.5rem; }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS — whole suite.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add container.go container_test.go default.css
git commit -m "feat: lift container titles and add collapsible container kinds"
```

---

### Task 4: The inline text-rewriting helper

Chips (Task 5) and `§` autolinking (Task 6) both need to replace runs of text inside prose while leaving code spans, code blocks and existing links alone. Building that once, tested on its own, keeps two features from growing two slightly different ideas of what "prose" means.

The existing `walk` cannot serve: it has no way to refuse a subtree, and it visits nodes an earlier callback inserted.

**Files:**
- Create: `inline.go`, `inline_test.go`
- Test: `inline_test.go`

**Interfaces:**
- Consumes: `hasClass` (existing, `transform.go`).
- Produces:
  - `func inlineSkip(n *html.Node) bool`
  - `func rewriteText(root *html.Node, fn func(string) []*html.Node)` — replaces a text node with `fn`'s result when it returns non-nil; nodes `fn` produces are never re-examined.

- [ ] **Step 1: Write the failing test**

Create `inline_test.go`:

```go
package md2html

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// upper replaces every prose text node with an uppercased copy, so the
// tests can see exactly which nodes were offered to fn.
func upper(s string) []*html.Node {
	u := strings.ToUpper(s)
	if u == s {
		return nil
	}
	return []*html.Node{{Type: html.TextNode, Data: u}}
}

func rewritten(t *testing.T, in string) string {
	t.Helper()
	root, err := parseFragment([]byte(in))
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	rewriteText(root, upper)
	out, err := renderTree(root)
	if err != nil {
		t.Fatalf("renderTree: %v", err)
	}
	return string(out)
}

func TestRewriteTextReachesNestedProse(t *testing.T) {
	got := rewritten(t, `<p>one <em>two</em> three</p>`)
	if got != `<p>ONE <em>TWO</em> THREE</p>` {
		t.Errorf("got %s", got)
	}
}

// A token inside a code span is being quoted, not used.
func TestRewriteTextSkipsCodeAndPre(t *testing.T) {
	got := rewritten(t, `<p>a <code>b</code></p><pre><code>c</code></pre>`)
	if !strings.Contains(got, "<code>b</code>") || !strings.Contains(got, "<code>c</code>") {
		t.Errorf("descended into code: %s", got)
	}
	if !strings.Contains(got, ">A ") {
		t.Errorf("skipped the prose too: %s", got)
	}
}

// Both callers produce links or badges, neither of which may nest inside
// an existing link.
func TestRewriteTextSkipsLinks(t *testing.T) {
	got := rewritten(t, `<p>a <a href="#x">b</a></p>`)
	if !strings.Contains(got, `<a href="#x">b</a>`) {
		t.Errorf("descended into a link: %s", got)
	}
}

func TestRewriteTextSkipsExistingChips(t *testing.T) {
	got := rewritten(t, `<p><span class="chip">b</span></p>`)
	if !strings.Contains(got, `<span class="chip">b</span>`) {
		t.Errorf("descended into a chip: %s", got)
	}
}

// Replacements must not be offered back to fn, or a rewrite that emits
// text containing its own trigger would loop.
func TestRewriteTextDoesNotRevisitReplacements(t *testing.T) {
	root, err := parseFragment([]byte(`<p>x</p>`))
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	rewriteText(root, func(s string) []*html.Node {
		calls++
		return []*html.Node{
			{Type: html.ElementNode, DataAtom: atom.Span, Data: "span"},
			{Type: html.TextNode, Data: "y"},
		}
	})
	if calls != 1 {
		t.Errorf("fn called %d times, want 1", calls)
	}
}

// Returning nil must leave the node untouched, identity included: the
// common case is a document with no tokens at all.
func TestRewriteTextNilLeavesTreeAlone(t *testing.T) {
	root, err := parseFragment([]byte(`<p>hello <em>there</em></p>`))
	if err != nil {
		t.Fatal(err)
	}
	rewriteText(root, func(string) []*html.Node { return nil })
	out, err := renderTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `<p>hello <em>there</em></p>` {
		t.Errorf("got %s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestRewriteText' -v`
Expected: FAIL — `undefined: rewriteText`.

- [ ] **Step 3: Write the implementation**

Create `inline.go`:

```go
package md2html

import (
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// inlineSkip reports whether a node's whole subtree is off-limits to inline
// rewriting.
//
// Code is skipped because a token inside a code span or a code block is
// being quoted, not used — `[proven]` in a code span is documentation of
// the syntax, and rewriting it would make the syntax undocumentable. Links
// are skipped because every inline rewriter here produces either a link or
// a badge, and neither may nest inside an <a>. Script and style are skipped
// because their text is not prose at all.
func inlineSkip(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	switch n.DataAtom {
	case atom.Code, atom.Pre, atom.A, atom.Script, atom.Style, atom.Kbd, atom.Samp:
		return true
	}
	return hasClass(n, "chip")
}

// rewriteText offers every prose text node under root to fn, replacing the
// node with fn's result when it returns one and leaving it alone when fn
// returns nil.
//
// Nodes fn produces are never offered back to fn: the next sibling is
// captured before the replacement happens, so the cursor is already past
// everything inserted. Without that, a rewriter emitting text that contains
// its own trigger would not terminate.
func rewriteText(root *html.Node, fn func(string) []*html.Node) {
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			switch {
			case c.Type == html.TextNode:
				if repl := fn(c.Data); repl != nil {
					for _, r := range repl {
						n.InsertBefore(r, c)
					}
					n.RemoveChild(c)
				}
			case inlineSkip(c):
				// Leave the whole subtree alone.
			default:
				visit(c)
			}
			c = next
		}
	}
	visit(root)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add inline.go inline_test.go
git commit -m "feat: add prose-only inline text rewriting helper"
```

---

### Task 5: Inline status chips

Spec item 4, including its heading interaction: the marker must not reach the slug, so relabeling `[planned]` to `[proven]` later cannot rot an anchor other documents already link to. That also means the derived `<title>` must lose the marker — and `extractTitle` runs *before* transforms (deliberately, so `HeadingAnchors`' `#` never lands in a title), so it needs a textual strip rather than the tree-level one.

**Files:**
- Create: `chip.go`, `chip_test.go`
- Modify: `transform.go` (`builtins`, `HeadingAnchors`), `page.go` (`extractTitle`), `cmd/md2html/main.go` (`buildOptions`), `default.css`
- Test: `chip_test.go`, plus additions to `transform_test.go`

**Interfaces:**
- Consumes: `rewriteText` (Task 4); `hasClass`, `slugify` (existing).
- Produces:
  - `var chipRe *regexp.Regexp`
  - `func Chips() Transform` — `Name: "chips"`.
  - `func chipNodes(s string) []*html.Node`
  - `func stripChipTokens(s string) string` — textual removal, for `extractTitle`.
  - `func headingText(n *html.Node) string` — like `textOf`, minus `.chip` subtrees.

- [ ] **Step 1: Write the failing test**

Create `chip_test.go`:

```go
package md2html

import (
	"strings"
	"testing"
)

func TestChipsRecognizedStatusWords(t *testing.T) {
	for _, word := range []string{"proven", "verified", "designed", "planned", "draft", "deprecated"} {
		got := apply(t, "<p>state ["+word+"] here</p>", Chips())
		want := `<span class="chip chip-` + word + `">` + word + `</span>`
		if !strings.Contains(got, want) {
			t.Errorf("[%s] did not become a chip\ngot: %s", word, got)
		}
	}
}

func TestChipsGenericFreeTextForm(t *testing.T) {
	got := apply(t, `<p>x [c:since v2] y</p>`, Chips())
	if !strings.Contains(got, `<span class="chip">since v2</span>`) {
		t.Errorf("generic chip not rendered\ngot: %s", got)
	}
}

// Anything outside the vocabulary stays literal text. This is what keeps
// ordinary bracketed prose and reference-style link syntax intact.
func TestChipsLeaveUnknownTokensLiteral(t *testing.T) {
	got := apply(t, `<p>see [1] and [some note]</p>`, Chips())
	if !strings.Contains(got, "[1]") || !strings.Contains(got, "[some note]") {
		t.Errorf("rewrote a non-chip token\ngot: %s", got)
	}
}

// An empty label would be an empty badge; leave the source text instead.
func TestChipsEmptyGenericLabelStaysLiteral(t *testing.T) {
	got := apply(t, `<p>x [c:] y</p>`, Chips())
	if !strings.Contains(got, "[c:]") {
		t.Errorf("emitted an empty chip\ngot: %s", got)
	}
}

func TestChipsSkipCodeAndLinks(t *testing.T) {
	got := apply(t, "<p><code>[proven]</code> and <a href=\"#x\">[proven]</a></p>", Chips())
	if strings.Contains(got, "chip") {
		t.Errorf("rewrote inside code or a link\ngot: %s", got)
	}
}

func TestChipsWorkInHeadings(t *testing.T) {
	got := apply(t, `<h2>Rollback [proven]</h2>`, Chips())
	if !strings.Contains(got, `class="chip chip-proven"`) {
		t.Errorf("no chip in heading\ngot: %s", got)
	}
}

// Surrounding text must survive the split intact.
func TestChipsPreserveSurroundingText(t *testing.T) {
	got := apply(t, `<p>before [proven] between [draft] after</p>`, Chips())
	for _, want := range []string{"before ", " between ", " after"} {
		if !strings.Contains(got, want) {
			t.Errorf("lost %q\ngot: %s", want, got)
		}
	}
}

func TestStripChipTokens(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Rollback [proven]", "Rollback"},
		{"[draft] Rollback", "Rollback"},
		{"Rollback [c:since v2] plan", "Rollback plan"},
		{"Rollback [1]", "Rollback [1]"},
		{"Rollback", "Rollback"},
	}
	for _, c := range cases {
		if got := stripChipTokens(c.in); got != c.want {
			t.Errorf("stripChipTokens(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

Append to `transform_test.go`:

```go
// A status marker must not reach the slug: relabeling it later would
// otherwise rot every anchor link pointing at that heading.
func TestHeadingAnchorsExcludeChipsFromSlug(t *testing.T) {
	got := apply(t, `<h2>Rollback <span class="chip chip-proven">proven</span></h2>`, HeadingAnchors())
	if !strings.Contains(got, `id="rollback"`) {
		t.Errorf("chip text leaked into the slug\ngot: %s", got)
	}
}

// A chip-free heading's slug must be byte-identical to what it was before
// chips existed — docs/authoring.md promises stable, predictable ids.
func TestHeadingAnchorsSlugUnchangedWithoutChips(t *testing.T) {
	got := apply(t, `<h2>Hello World</h2>`, HeadingAnchors())
	if !strings.Contains(got, `id="hello-world"`) {
		t.Errorf("slug changed\ngot: %s", got)
	}
}
```

Append to `page_test.go`:

```go
// The derived <title> must not carry a status marker either. extractTitle
// runs before transforms, so it sees the raw bracketed text.
func TestExtractTitleStripsChipTokens(t *testing.T) {
	root, err := parseFragment([]byte(`<h1>Rollback [proven]</h1>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := extractTitle(root, ""); got != "Rollback" {
		t.Errorf("got %q, want %q", got, "Rollback")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestChips|TestStripChipTokens|TestHeadingAnchorsExclude|TestExtractTitleStrips' -v`
Expected: FAIL — `undefined: Chips`, `undefined: stripChipTokens`.

- [ ] **Step 3: Write the implementation**

Create `chip.go`:

```go
package md2html

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// chipRe matches the recognized bracket tokens.
//
// The status vocabulary is fixed and short on purpose: the alternative is
// treating every bracketed word as a badge, which would swallow reference
// link syntax, footnote-looking text, and any prose that happens to use
// brackets. The c: form is the escape hatch for a label outside the
// vocabulary — explicit, so it can never fire by accident. The label is
// capped and excludes newlines so a stray "[c:" cannot swallow a paragraph.
var chipRe = regexp.MustCompile(`\[(proven|verified|designed|planned|draft|deprecated|c:[^\]\n]{0,60})\]`)

// chipNodes splits s on chip tokens, returning the replacement nodes, or
// nil when s carries none — which is almost every text node in almost every
// document, so it is the cheap path.
func chipNodes(s string) []*html.Node {
	ms := chipRe.FindAllStringSubmatchIndex(s, -1)
	if ms == nil {
		return nil
	}
	var out []*html.Node
	text := func(v string) {
		if v != "" {
			out = append(out, &html.Node{Type: html.TextNode, Data: v})
		}
	}
	last := 0
	for _, m := range ms {
		text(s[last:m[0]])
		token := s[m[2]:m[3]]
		class, label := "chip", token
		if strings.HasPrefix(token, "c:") {
			label = strings.TrimSpace(token[2:])
			if label == "" {
				// "[c:]" names nothing. An empty badge is worse than the
				// literal text, which at least shows the author what they
				// wrote.
				text(s[m[0]:m[1]])
				last = m[1]
				continue
			}
		} else {
			class = "chip chip-" + token
		}
		span := &html.Node{Type: html.ElementNode, DataAtom: atom.Span, Data: "span",
			Attr: []html.Attribute{{Key: "class", Val: class}}}
		span.AppendChild(&html.Node{Type: html.TextNode, Data: label})
		out = append(out, span)
		last = m[1]
	}
	text(s[last:])
	return out
}

// Chips turns a fixed set of bracketed tokens into small styled badges,
// everywhere inline Markdown is rendered — body text and headings alike,
// since a status marker on a heading is the case the convention exists for.
//
// A heading's marker is kept out of its slug by HeadingAnchors, which is
// why this transform must run before it.
func Chips() Transform {
	return Transform{Name: "chips", Fn: func(root *html.Node) error {
		rewriteText(root, chipNodes)
		return nil
	}}
}

// stripChipTokens removes chip tokens from a plain string.
//
// extractTitle runs before any transform — deliberately, so that
// HeadingAnchors' "#" never lands in a title — and therefore sees the raw
// bracketed source text rather than the spans Chips produces. Without this,
// a page titled "Rollback [proven]" would carry the marker in its browser
// tab and in every Artifact listing.
func stripChipTokens(s string) string {
	if !strings.ContainsRune(s, '[') {
		return s
	}
	out := chipRe.ReplaceAllString(s, "")
	return strings.TrimSpace(strings.Join(strings.Fields(out), " "))
}

// headingText returns a heading's text with chip badges left out, so a
// relabeled marker cannot change an anchor that other documents link to.
func headingText(n *html.Node) string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
			return
		}
		if x.Type == html.ElementNode && hasClass(x, "chip") {
			return
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return b.String()
}
```

In `transform.go`, `HeadingAnchors` must slugify from `headingText` rather
than `textOf`. Change the one line:

```go
				base := slugify(headingText(h))
```

and extend the function's doc comment:

```go
// HeadingAnchors gives every heading a stable id and a linkable anchor.
// An id already present — from a {#custom-id} attribute — is left alone.
//
// Status chips are excluded from the slug: a marker is metadata about the
// section, not part of its name, and relabeling one later must not rot an
// anchor other documents already link to.
```

Add `Chips` to `builtins`, before `HeadingAnchors`:

```go
func builtins(warn func(string)) []Transform {
	return []Transform{
		Containers(warn),
		TableScroll(),
		Chips(),
		HeadingAnchors(),
		ExternalLinks(),
	}
}
```

and to `buildOptions` in `cmd/md2html/main.go`, in the same position:

```go
	ts = append(ts, md2html.Chips())
	if !noAnchor {
		ts = append(ts, md2html.HeadingAnchors())
	}
```

In `page.go`, strip tokens from the derived title:

```go
		if n.Type == nethtml.ElementNode && n.DataAtom == atom.H1 {
			if t := strings.TrimSpace(stripChipTokens(textOf(n))); t != "" {
				found = t
			}
		}
```

Append to `default.css`:

```css
/* Status chips: a short marker attached to a claim or a heading. Sized in
   em so one sitting in an h2 scales with it instead of shrinking to noise. */
.chip {
  display: inline-block;
  font-size: 0.78em;
  font-weight: 600;
  line-height: 1.6;
  padding: 0 0.55em;
  border: 1px solid var(--rule);
  border-radius: 999px;
  background: var(--code-bg);
  color: var(--muted);
  vertical-align: baseline;
  white-space: nowrap;
}
.chip-proven, .chip-verified { color: var(--ok); border-color: var(--ok); }
.chip-deprecated { color: var(--warn); border-color: var(--warn); }
```

`--ok` and `--warn` were added to all four `:root` blocks in Task 2.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS — whole suite, including the existing
`TestHeadingAnchorsAddsIDAndLink`, which proves chip-free slugs are
unchanged.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add chip.go chip_test.go transform.go transform_test.go page.go page_test.go cmd/md2html/main.go default.css
git commit -m "feat: render inline status chips and keep them out of slugs and titles"
```

---

### Task 6: Section-number autolinking

Spec item 5. Runs after `HeadingAnchors`, because it resolves `§4.2` against ids that transform assigns.

**Files:**
- Create: `xref.go`, `xref_test.go`
- Modify: `transform.go` (`builtins`), `cmd/md2html/main.go` (`buildOptions`), `default.css`
- Test: `xref_test.go`

**Interfaces:**
- Consumes: `rewriteText`, `inlineSkip` (Task 4); `headingText` (Task 5); `attr`, `walk` (existing).
- Produces:
  - `func SectionLinks() Transform` — `Name: "sectionLinks"`.
  - `func sectionNumbers(root *html.Node) map[string]string` — heading number → anchor id.
  - `var sectionRe, headingNumRe *regexp.Regexp`

- [ ] **Step 1: Write the failing test**

Create `xref_test.go`:

```go
package md2html

import (
	"strings"
	"testing"
)

// The transform needs ids, so these run the pair in the real order.
func xref(t *testing.T, in string) string {
	t.Helper()
	root, err := parseFragment([]byte(in))
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	for _, tr := range []Transform{HeadingAnchors(), SectionLinks()} {
		if err := tr.Fn(root); err != nil {
			t.Fatalf("%s: %v", tr.Name, err)
		}
	}
	out, err := renderTree(root)
	if err != nil {
		t.Fatalf("renderTree: %v", err)
	}
	return string(out)
}

func TestSectionLinksResolvesAgainstNumberedHeading(t *testing.T) {
	got := xref(t, `<h2>4.2 Rollback</h2><p>see §4.2 for detail</p>`)
	if !strings.Contains(got, `<a class="xref" href="#42-rollback">§4.2</a>`) {
		t.Errorf("not autolinked\ngot: %s", got)
	}
}

func TestSectionLinksResolvesSingleLevelNumber(t *testing.T) {
	got := xref(t, `<h2>7 Appendix</h2><p>see §7</p>`)
	if !strings.Contains(got, `href="#7-appendix"`) {
		t.Errorf("single-level number not resolved\ngot: %s", got)
	}
}

// No matching heading means no link — a dangling anchor is worse than
// plain text, because it looks clickable and goes nowhere.
func TestSectionLinksLeavesUnmatchedNumberLiteral(t *testing.T) {
	got := xref(t, `<h2>4.2 Rollback</h2><p>see §9.9</p>`)
	if strings.Contains(got, "xref") {
		t.Errorf("linked an unmatched number\ngot: %s", got)
	}
	if !strings.Contains(got, "§9.9") {
		t.Errorf("lost the literal text\ngot: %s", got)
	}
}

// A reference explicitly scoped to another document must stay literal.
func TestSectionLinksSkipsPossessiveCrossDocumentReference(t *testing.T) {
	got := xref(t, `<h2>7 Appendix</h2><p>see the design doc's §7</p>`)
	if strings.Contains(got, "xref") {
		t.Errorf("linked a cross-document reference\ngot: %s", got)
	}
}

func TestSectionLinksSkipsCodeAndExistingLinks(t *testing.T) {
	got := xref(t, `<h2>7 A</h2><p><code>§7</code> <a href="#z">§7</a></p>`)
	if strings.Contains(got, "xref") {
		t.Errorf("rewrote inside code or a link\ngot: %s", got)
	}
}

// An explicit {#id} on the heading must be what the reference resolves to.
func TestSectionLinksHonorsExplicitHeadingID(t *testing.T) {
	got := xref(t, `<h2 id="rb">4.2 Rollback</h2><p>see §4.2</p>`)
	if !strings.Contains(got, `href="#rb"`) {
		t.Errorf("ignored the explicit id\ngot: %s", got)
	}
}

// A heading whose text merely starts with digits that are not a section
// number should not claim that number for the whole document. The rule is
// that the number must be followed by whitespace.
func TestSectionNumbersRequiresSeparator(t *testing.T) {
	root, err := parseFragment([]byte(`<h2 id="a">4.2 Rollback</h2><h3 id="b">2026 in review</h3><h4 id="c">4.2.1</h4>`))
	if err != nil {
		t.Fatal(err)
	}
	m := sectionNumbers(root)
	if m["4.2"] != "a" {
		t.Errorf(`m["4.2"] = %q, want "a"`, m["4.2"])
	}
	if m["2026"] != "b" {
		t.Errorf(`m["2026"] = %q, want "b"`, m["2026"])
	}
	// A heading that is only a number still counts: nothing follows it.
	if m["4.2.1"] != "c" {
		t.Errorf(`m["4.2.1"] = %q, want "c"`, m["4.2.1"])
	}
}

// Two headings claiming one number is an authoring error; the first wins,
// deterministically, rather than whichever the map iteration reached last.
func TestSectionNumbersFirstHeadingWins(t *testing.T) {
	root, err := parseFragment([]byte(`<h2 id="a">3 One</h2><h2 id="b">3 Two</h2>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := sectionNumbers(root)["3"]; got != "a" {
		t.Errorf("got %q, want %q", got, "a")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestSection' -v`
Expected: FAIL — `undefined: SectionLinks`, `undefined: sectionNumbers`.

- [ ] **Step 3: Write the implementation**

Create `xref.go`:

```go
package md2html

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// headingNumRe matches a leading section number on a heading: "4", "4.2",
// "4.2.1". The number must be followed by whitespace or end the heading, so
// "4.2rc" is not a section number and "2026 in review" is — the latter
// harmlessly, since nothing will write "§2026" unless it means that heading.
var headingNumRe = regexp.MustCompile(`^(\d+(?:\.\d+)*)(?:\s|$)`)

// sectionRe matches a bare cross-reference: "§4.2", or "§ 4.2".
var sectionRe = regexp.MustCompile(`§\s?(\d+(?:\.\d+)*)`)

// sectionNumbers maps each numbered heading's number to its anchor id.
//
// It reads the id off the heading rather than re-deriving a slug, so an
// explicit {#custom-id} is what references resolve to, and so a heading
// HeadingAnchors disambiguated with a "-2" suffix resolves to the id it was
// actually given. Headings with no id — HeadingAnchors disabled via
// --no-anchors — contribute nothing, which is why this degrades to plain
// text rather than emitting hrefs that point nowhere.
//
// The first heading claiming a number keeps it. Two headings numbered the
// same is an authoring error, and picking the first is at least stable.
func sectionNumbers(root *html.Node) map[string]string {
	out := map[string]string{}
	walk(root, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		switch n.DataAtom {
		case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		default:
			return
		}
		id, ok := attr(n, "id")
		if !ok || id == "" {
			return
		}
		m := headingNumRe.FindStringSubmatch(strings.TrimSpace(headingText(n)))
		if m == nil {
			return
		}
		if _, dup := out[m[1]]; !dup {
			out[m[1]] = id
		}
	})
	return out
}

// SectionLinks autolinks bare "§N.M" references to the heading in the same
// document numbered N.M.
//
// It is a transform rather than a goldmark extension because it needs the
// finished heading set, ids included, to resolve against — which only
// exists after HeadingAnchors has run.
//
// Left alone: a § inside a code span, code block or existing link (handled
// by rewriteText); a § whose number matches no heading here; and a
// possessive reference scoping the section to another document, as in
// "the design doc's §7". That last rule is narrow — it catches the
// possessive phrasing and nothing else — so a cross-document reference
// written any other way still needs a code span to opt out. docs/authoring.md
// records that.
func SectionLinks() Transform {
	return Transform{Name: "sectionLinks", Fn: func(root *html.Node) error {
		nums := sectionNumbers(root)
		if len(nums) == 0 {
			return nil
		}
		rewriteText(root, func(s string) []*html.Node {
			ms := sectionRe.FindAllStringSubmatchIndex(s, -1)
			if ms == nil {
				return nil
			}
			var out []*html.Node
			changed := false
			text := func(v string) {
				if v != "" {
					out = append(out, &html.Node{Type: html.TextNode, Data: v})
				}
			}
			last := 0
			for _, m := range ms {
				id, known := nums[s[m[2]:m[3]]]
				if !known || possessiveBefore(s[:m[0]]) {
					continue // leave this occurrence inside the literal run
				}
				text(s[last:m[0]])
				a := &html.Node{Type: html.ElementNode, DataAtom: atom.A, Data: "a",
					Attr: []html.Attribute{
						{Key: "class", Val: "xref"},
						{Key: "href", Val: "#" + id},
					}}
				a.AppendChild(&html.Node{Type: html.TextNode, Data: s[m[0]:m[1]]})
				out = append(out, a)
				last = m[1]
				changed = true
			}
			if !changed {
				return nil
			}
			text(s[last:])
			return out
		})
		return nil
	}}
}

// possessiveBefore reports whether the text immediately preceding a § ends
// with a possessive — "the design doc's §7" — which scopes the reference to
// another document rather than to a heading here.
func possessiveBefore(before string) bool {
	t := strings.TrimRight(before, " \t")
	return strings.HasSuffix(t, "'s") || strings.HasSuffix(t, "’s")
}
```

Add `SectionLinks` to `builtins`, after `HeadingAnchors`:

```go
		HeadingAnchors(),
		SectionLinks(),
		ExternalLinks(),
```

and to `buildOptions`, in the same position:

```go
	if !noAnchor {
		ts = append(ts, md2html.HeadingAnchors())
	}
	ts = append(ts, md2html.SectionLinks())
```

With `--no-anchors`, `sectionNumbers` finds no ids and `SectionLinks`
returns immediately — no dangling hrefs.

Append to `default.css`:

```css
/* A cross-reference is a link, but a quieter one than a prose link: it
   points inside the page the reader is already on. */
a.xref { text-decoration: underline dotted; text-underline-offset: 0.2em; }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS — whole suite.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add xref.go xref_test.go transform.go cmd/md2html/main.go default.css
git commit -m "feat: autolink bare section-number cross-references"
```

---

### Task 7: Generated table of contents

Spec item 6, scoped exactly as the spec scopes it: the current page's own headings, flat, and nothing cross-document. Runs after `HeadingAnchors` (it needs ids) and after `Chips` (link text must match what the reader sees in the heading).

**Files:**
- Create: `toc.go`, `toc_test.go`
- Modify: `transform.go` (`builtins`), `cmd/md2html/main.go` (`buildOptions`), `default.css`
- Test: `toc_test.go`

**Interfaces:**
- Consumes: `headingText` (Task 5); `attr`, `walk`, `textOf` (existing).
- Produces:
  - `const tocMarker = "[[toc]]"`
  - `func TOC() Transform` — `Name: "toc"`.

- [ ] **Step 1: Write the failing test**

Create `toc_test.go`:

```go
package md2html

import (
	"strings"
	"testing"
)

func toc(t *testing.T, in string) string {
	t.Helper()
	root, err := parseFragment([]byte(in))
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	for _, tr := range []Transform{Chips(), HeadingAnchors(), TOC()} {
		if err := tr.Fn(root); err != nil {
			t.Fatalf("%s: %v", tr.Name, err)
		}
	}
	out, err := renderTree(root)
	if err != nil {
		t.Fatalf("renderTree: %v", err)
	}
	return string(out)
}

func TestTOCReplacesMarkerWithNav(t *testing.T) {
	got := toc(t, `<h1>Doc</h1><p>[[toc]]</p><h2>First</h2><h2>Second</h2>`)
	if !strings.Contains(got, `<nav class="toc">`) {
		t.Errorf("no nav\ngot: %s", got)
	}
	if strings.Contains(got, "[[toc]]") {
		t.Errorf("marker left in the body\ngot: %s", got)
	}
	if !strings.Contains(got, `<a href="#first">First</a>`) ||
		!strings.Contains(got, `<a href="#second">Second</a>`) {
		t.Errorf("headings missing from nav\ngot: %s", got)
	}
}

// Flat, not nested: an irregular heading-level jump must not be able to
// produce broken list nesting. Level is carried as a class instead.
func TestTOCIsFlatAndCarriesLevelAsAClass(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><h2>A</h2><h4>B</h4>`)
	if strings.Count(got, "<ol") != 1 {
		t.Errorf("nested lists\ngot: %s", got)
	}
	if !strings.Contains(got, `class="toc-h2"`) || !strings.Contains(got, `class="toc-h4"`) {
		t.Errorf("no level classes\ngot: %s", got)
	}
}

// Link text must match what the reader sees, which means without chips.
func TestTOCExcludesChipsFromLinkText(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><h2>Rollback [proven]</h2>`)
	if !strings.Contains(got, `<a href="#rollback">Rollback</a>`) {
		t.Errorf("chip text leaked into the toc\ngot: %s", got)
	}
}

// A document with no marker must come out byte-identical.
func TestTOCWithoutMarkerIsANoOp(t *testing.T) {
	in := `<h1>Doc</h1><h2>First</h2>`
	root, err := parseFragment([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if err := TOC().Fn(root); err != nil {
		t.Fatal(err)
	}
	out, err := renderTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Errorf("got %s, want %s", out, in)
	}
}

// The marker only counts alone on its own line. Inside a sentence it is
// prose, and inside a code fence it is documentation of this feature.
func TestTOCIgnoresMarkerInProseAndCode(t *testing.T) {
	got := toc(t, `<p>write [[toc]] to get one</p><pre><code>[[toc]]</code></pre><h2>A</h2>`)
	if strings.Contains(got, "<nav") {
		t.Errorf("expanded a marker that was not alone on its line\ngot: %s", got)
	}
}

// The heading the marker sits under is itself a heading; excluding nothing
// keeps the rule simple, and the page title reads fine as the first entry.
func TestTOCIncludesEveryHeadingInOrder(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><h1>Doc</h1><h2>A</h2><h3>B</h3>`)
	iDoc := strings.Index(got, `href="#doc"`)
	iA := strings.Index(got, `href="#a"`)
	iB := strings.Index(got, `href="#b"`)
	if iDoc < 0 || iA < iDoc || iB < iA {
		t.Errorf("entries out of document order\ngot: %s", got)
	}
}

// A marker in a document with no headings must not leave an empty nav.
func TestTOCWithNoHeadingsRemovesTheMarker(t *testing.T) {
	got := toc(t, `<p>[[toc]]</p><p>body</p>`)
	if strings.Contains(got, "<nav") {
		t.Errorf("emitted an empty nav\ngot: %s", got)
	}
	if strings.Contains(got, "[[toc]]") {
		t.Errorf("left the marker behind\ngot: %s", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestTOC -v`
Expected: FAIL — `undefined: TOC`.

- [ ] **Step 3: Write the implementation**

Create `toc.go`:

```go
package md2html

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// tocMarker is the token a document uses to ask for a contents list. It has
// to be inert to every other Markdown renderer, so that a source file
// carrying one still reads correctly unprocessed — double brackets are not
// link syntax in CommonMark, so this renders as literal text elsewhere.
const tocMarker = "[[toc]]"

// TOC replaces a marker paragraph with a flat list of the current
// document's own headings.
//
// Flat, not nested by heading level: a document that jumps h2 to h4 would
// otherwise produce either invalid list nesting or a silently wrong tree.
// Level travels as a class on the list item, so the stylesheet can indent
// without the markup having to be a hierarchy.
//
// This is a table of contents for one page and nothing more. Cross-document
// navigation, a sidebar and a site index stay out of scope — see
// docs/specs/2026-09-05-md2html-design.md.
func TOC() Transform {
	return Transform{Name: "toc", Fn: func(root *html.Node) error {
		var markers []*html.Node
		var heads []*html.Node
		walk(root, func(n *html.Node) {
			if n.Type != html.ElementNode {
				return
			}
			switch n.DataAtom {
			case atom.P:
				// Alone on its own line: the marker must be the paragraph's
				// entire content. A marker inside a sentence is prose, and
				// one inside a fence is this feature's own documentation —
				// the fence is a <pre>, never a <p>, so it is excluded by
				// construction.
				if strings.TrimSpace(textOf(n)) == tocMarker {
					markers = append(markers, n)
				}
			case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
				heads = append(heads, n)
			}
		})
		for _, m := range markers {
			nav := buildTOC(heads)
			if nav != nil {
				m.Parent.InsertBefore(nav, m)
			}
			m.Parent.RemoveChild(m)
		}
		return nil
	}}
}

// buildTOC renders the nav, or nil when there is nothing to list. A
// document whose marker has no headings to point at gets no empty nav —
// removing the marker is still right, since leaving it would show the
// reader a token rather than a contents list.
func buildTOC(heads []*html.Node) *html.Node {
	list := &html.Node{Type: html.ElementNode, DataAtom: atom.Ol, Data: "ol"}
	n := 0
	for _, h := range heads {
		id, ok := attr(h, "id")
		if !ok || id == "" {
			continue // --no-anchors: nothing to link to
		}
		label := strings.TrimSpace(headingText(h))
		// HeadingAnchors appended an "#" anchor link inside the heading;
		// headingText picks it up, and it is not part of the title.
		label = strings.TrimSuffix(label, "#")
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		li := &html.Node{Type: html.ElementNode, DataAtom: atom.Li, Data: "li",
			Attr: []html.Attribute{{Key: "class", Val: "toc-" + h.Data}}}
		a := &html.Node{Type: html.ElementNode, DataAtom: atom.A, Data: "a",
			Attr: []html.Attribute{{Key: "href", Val: "#" + id}}}
		a.AppendChild(&html.Node{Type: html.TextNode, Data: label})
		li.AppendChild(a)
		list.AppendChild(li)
		n++
	}
	if n == 0 {
		return nil
	}
	nav := &html.Node{Type: html.ElementNode, DataAtom: atom.Nav, Data: "nav",
		Attr: []html.Attribute{{Key: "class", Val: "toc"}}}
	nav.AppendChild(list)
	return nav
}
```

Add `TOC` to `builtins`, after `SectionLinks`:

```go
		SectionLinks(),
		TOC(),
		ExternalLinks(),
```

and to `buildOptions`:

```go
	ts = append(ts, md2html.SectionLinks(), md2html.TOC())
```

Append to `default.css`:

```css
/* Flat by construction: indentation comes from the level class, so an
   h2→h4 jump indents without the markup pretending to be a hierarchy. */
nav.toc { margin: 2rem 0; }
nav.toc ol { list-style: none; margin: 0; padding-left: 0; }
nav.toc li { margin: 0.15rem 0; }
nav.toc .toc-h3 { padding-left: 1.25rem; }
nav.toc .toc-h4 { padding-left: 2.5rem; }
nav.toc .toc-h5, nav.toc .toc-h6 { padding-left: 3.75rem; }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS — whole suite.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add toc.go toc_test.go transform.go cmd/md2html/main.go default.css
git commit -m "feat: expand a [[toc]] marker into a flat per-page contents list"
```

---

### Task 8: Front matter and heading-lift subtitle

Spec item 7. Front matter has to come off the source bytes before goldmark sees them, because a `---` block is valid Markdown already — verified, it currently renders as an `<hr>` followed by a **setext `<h2>`** holding the key/value lines, not the paragraph the spec predicted. Either way it is noise above the title.

The spec proposes putting subtitle and date into the page shell. This plan puts them into the document tree immediately after the first `<h1>` instead. The shell wraps the body in `<main>` and knows nothing about where the title sits inside it, so a shell slot would print the subtitle *above* the heading it belongs to; and the tree route leaves `renderPage`/`renderFragment` signatures — and every test asserting on them — untouched.

**Files:**
- Create: `frontmatter.go`, `frontmatter_test.go`
- Modify: `md2html.go` (`Convert`), `default.css`
- Test: `frontmatter_test.go`

**Interfaces:**
- Consumes: `parseFragment`, `walk`, `extractTitle` (existing).
- Produces:
  - `func splitFrontMatter(src []byte) (map[string]string, []byte)`
  - `func applyDocMeta(root *html.Node, subtitle, date string)`
  - `func liftSubtitle(root *html.Node) bool` — promotes an italic-only line after the h1.

- [ ] **Step 1: Write the failing test**

Create `frontmatter_test.go`:

```go
package md2html

import (
	"strings"
	"testing"
)

func TestSplitFrontMatterExtractsFlatKeys(t *testing.T) {
	meta, body := splitFrontMatter([]byte("---\ntitle: Rollback\nsubtitle: how it works\ndate: 2026-09-11\n---\n\n# H\n"))
	if meta["title"] != "Rollback" || meta["subtitle"] != "how it works" || meta["date"] != "2026-09-11" {
		t.Errorf("meta = %v", meta)
	}
	if strings.Contains(string(body), "title:") {
		t.Errorf("block not stripped from body: %q", body)
	}
	if !strings.Contains(string(body), "# H") {
		t.Errorf("body lost: %q", body)
	}
}

// A value containing a colon must not be truncated at the first one.
func TestSplitFrontMatterKeepsColonsInValues(t *testing.T) {
	meta, _ := splitFrontMatter([]byte("---\ntitle: A: B\n---\nx\n"))
	if meta["title"] != "A: B" {
		t.Errorf("title = %q", meta["title"])
	}
}

// Anything that is not flat key/value is left alone and rendered, which is
// today's behavior and an obvious signal that something needs fixing.
func TestSplitFrontMatterRejectsNonFlatBlock(t *testing.T) {
	src := []byte("---\ntags:\n  - a\n---\nx\n")
	meta, body := splitFrontMatter(src)
	if meta != nil {
		t.Errorf("parsed a nested block: %v", meta)
	}
	if string(body) != string(src) {
		t.Errorf("body altered: %q", body)
	}
}

// A horizontal rule at the top of a document is not front matter.
func TestSplitFrontMatterIgnoresPlainRule(t *testing.T) {
	src := []byte("---\n\nsome prose\n")
	meta, body := splitFrontMatter(src)
	if meta != nil {
		t.Errorf("parsed a plain rule as front matter: %v", meta)
	}
	if string(body) != string(src) {
		t.Errorf("body altered: %q", body)
	}
}

func TestSplitFrontMatterIgnoresBlockNotAtStart(t *testing.T) {
	src := []byte("# H\n\n---\ntitle: X\n---\n")
	meta, _ := splitFrontMatter(src)
	if meta != nil {
		t.Errorf("parsed a mid-document block: %v", meta)
	}
}

func TestConvertFrontMatterTitleWins(t *testing.T) {
	got := convert(t, "---\ntitle: Real Title\n---\n\n# Heading\n", nil)
	if !strings.Contains(got, "<title>Real Title</title>") {
		t.Errorf("front-matter title ignored\ngot: %s", got)
	}
	if !strings.Contains(got, "<h1") {
		t.Errorf("heading lost\ngot: %s", got)
	}
}

// An explicit Options.Title still beats front matter: it is the caller's
// override, and front matter is the document's.
func TestConvertOptionsTitleBeatsFrontMatter(t *testing.T) {
	out, err := Convert([]byte("---\ntitle: From Doc\n---\n\n# H\n"),
		Options{Fragment: true, CSS: "/**/", Title: "From Caller"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<title>From Caller</title>") {
		t.Errorf("got: %s", out)
	}
}

func TestConvertFrontMatterSubtitleAndDateRender(t *testing.T) {
	got := convert(t, "---\ntitle: T\nsubtitle: the short version\ndate: 2026-09-11\n---\n\n# T\n\nbody\n", nil)
	if !strings.Contains(got, `<p class="subtitle">the short version</p>`) {
		t.Errorf("no subtitle\ngot: %s", got)
	}
	if !strings.Contains(got, `<p class="docdate">2026-09-11</p>`) {
		t.Errorf("no date\ngot: %s", got)
	}
	if strings.Index(got, "subtitle") < strings.Index(got, "<h1") {
		t.Errorf("subtitle placed above the heading\ngot: %s", got)
	}
}

// Path (b): an italic-only line right after the h1, with no front matter
// present at all.
func TestConvertLiftsItalicSubtitleWithoutFrontMatter(t *testing.T) {
	got := convert(t, "# Title\n\n*the short version*\n\nbody\n", nil)
	if !strings.Contains(got, `<p class="subtitle">the short version</p>`) {
		t.Errorf("italic line not lifted\ngot: %s", got)
	}
	if strings.Contains(got, "<em>the short version</em>") {
		t.Errorf("left the em wrapper in place\ngot: %s", got)
	}
}

// A comment before the h1 must not stop the lift.
func TestConvertLiftsSubtitlePastLeadingComment(t *testing.T) {
	got := convert(t, "<!-- generated -->\n\n# Title\n\n*sub*\n\nbody\n", nil)
	if !strings.Contains(got, `<p class="subtitle">sub</p>`) {
		t.Errorf("comment blocked the lift\ngot: %s", got)
	}
}

// Only a line that is entirely italic counts. Emphasis at the start of a
// real paragraph is prose.
func TestConvertDoesNotLiftPartialItalicParagraph(t *testing.T) {
	got := convert(t, "# Title\n\n*emphasis* then prose\n", nil)
	if strings.Contains(got, "subtitle") {
		t.Errorf("lifted a prose paragraph\ngot: %s", got)
	}
}

// Front matter wins over the heading lift; the two must not both fire.
func TestConvertFrontMatterSubtitleBeatsItalicLift(t *testing.T) {
	got := convert(t, "---\nsubtitle: declared\n---\n\n# T\n\n*lifted*\n", nil)
	if !strings.Contains(got, `<p class="subtitle">declared</p>`) {
		t.Errorf("front matter lost\ngot: %s", got)
	}
	if strings.Contains(got, "lifted</p>") && strings.Count(got, "subtitle") != 1 {
		t.Errorf("both paths fired\ngot: %s", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestSplitFrontMatter|TestConvertFrontMatter|TestConvertLifts|TestConvertOptionsTitle|TestConvertDoesNotLift' -v`
Expected: FAIL — `undefined: splitFrontMatter`.

- [ ] **Step 3: Write the implementation**

Create `frontmatter.go`:

```go
package md2html

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// splitFrontMatter peels a leading "---"-delimited block of flat
// "key: value" lines off the source, returning the keys and the remaining
// body. It returns (nil, src) unchanged when there is no such block.
//
// It is deliberately not YAML. The three keys the page can use are all flat
// strings, and taking on a YAML parser to read them would be the tool's
// first dependency for a feature this small. A block that is not flat
// key/value — a list, a nested map — is refused whole and rendered as it is
// today: an <hr> followed by a setext heading holding the lines, which is
// noisy above a title and therefore self-reporting.
func splitFrontMatter(src []byte) (map[string]string, []byte) {
	s := string(src)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return nil, src
	}
	rest := s[strings.IndexByte(s, '\n')+1:]
	end := -1
	lines := strings.Split(rest, "\n")
	meta := map[string]string{}
	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if trimmed == "---" || trimmed == "..." {
			end = i
			break
		}
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		// Flat only: an indented line is a nested structure, and a line
		// with no colon is not a key at all.
		if trimmed != strings.TrimLeft(trimmed, " \t") {
			return nil, src
		}
		k, v, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, src
		}
		key := strings.TrimSpace(k)
		if key == "" {
			return nil, src
		}
		meta[strings.ToLower(key)] = strings.TrimSpace(v)
	}
	if end < 0 {
		// No closing delimiter: this was an <hr>, not front matter.
		return nil, src
	}
	body := strings.Join(lines[end+1:], "\n")
	return meta, []byte(strings.TrimLeft(body, "\n"))
}

// firstHeading returns the document's first <h1>, or nil.
func firstHeading(root *html.Node) *html.Node {
	var found *html.Node
	walk(root, func(n *html.Node) {
		if found == nil && n.Type == html.ElementNode && n.DataAtom == atom.H1 {
			found = n
		}
	})
	return found
}

// liftSubtitle promotes a line consisting of nothing but italic text,
// immediately following the leading h1, into a subtitle paragraph. It
// reports whether it fired.
//
// "Immediately following" is the next element sibling: blank lines leave no
// node, and an HTML comment before the h1 is a CommentNode that walk passes
// over on its way to finding the heading. The paragraph must be entirely
// one <em> — emphasis at the *start* of a paragraph is prose, and lifting
// it would silently eat a line of the document.
func liftSubtitle(root *html.Node) bool {
	h := firstHeading(root)
	if h == nil {
		return false
	}
	p := h.NextSibling
	for p != nil && (p.Type == html.CommentNode ||
		(p.Type == html.TextNode && strings.TrimSpace(p.Data) == "")) {
		p = p.NextSibling
	}
	if p == nil || p.Type != html.ElementNode || p.DataAtom != atom.P {
		return false
	}
	em := p.FirstChild
	if em == nil || em.NextSibling != nil ||
		em.Type != html.ElementNode || em.DataAtom != atom.Em {
		return false
	}
	// Unwrap the <em>: the subtitle's styling is the stylesheet's job, and
	// leaving the italics would double up on it.
	p.RemoveChild(em)
	for c := em.FirstChild; c != nil; {
		next := c.NextSibling
		em.RemoveChild(c)
		p.AppendChild(c)
		c = next
	}
	setAttr(p, "class", "subtitle")
	return true
}

// applyDocMeta inserts a subtitle and a date under the document's leading
// h1 — or at the top of the document when it has no h1, which is the only
// other place they could go and still read as document metadata.
func applyDocMeta(root *html.Node, subtitle, date string) {
	if subtitle == "" && date == "" {
		return
	}
	anchor := firstHeading(root)
	insert := func(class, text string) {
		if text == "" {
			return
		}
		p := &html.Node{Type: html.ElementNode, DataAtom: atom.P, Data: "p",
			Attr: []html.Attribute{{Key: "class", Val: class}}}
		p.AppendChild(&html.Node{Type: html.TextNode, Data: text})
		if anchor == nil {
			root.InsertBefore(p, root.FirstChild)
			return
		}
		anchor.Parent.InsertBefore(p, anchor.NextSibling)
		anchor = p // keep date after subtitle
	}
	insert("subtitle", subtitle)
	insert("docdate", date)
}
```

In `md2html.go`, rework the head of `Convert`:

```go
// Convert renders Markdown to HTML, running transforms over the parsed tree.
func Convert(src []byte, opt Options) ([]byte, error) {
	// Front matter comes off the bytes: a "---" block is already valid
	// Markdown, so by the time a tree exists it has become an <hr> and a
	// setext heading, with no way back to the key/value lines.
	meta, src := splitFrontMatter(src)

	var buf bytes.Buffer
	if err := newParser().Convert(src, &buf); err != nil {
		return nil, err
	}
	root, err := parseFragment(buf.Bytes())
	if err != nil {
		return nil, err
	}

	// Before transforms: HeadingAnchors would otherwise contribute its "#"
	// anchor text to the derived title.
	title := opt.Title
	if title == "" {
		title = meta["title"]
	}
	if title == "" {
		title = extractTitle(root, opt.SourcePath)
	}

	// Also before transforms, so the subtitle paragraph is an ordinary part
	// of the tree by the time anything walks it.
	subtitle := meta["subtitle"]
	if subtitle == "" {
		// No declared subtitle: an italic-only line under the h1 is the
		// convention documents use when they carry no front matter at all.
		liftSubtitle(root)
	}
	applyDocMeta(root, subtitle, meta["date"])
```

The rest of `Convert` is unchanged.

Append to `default.css`:

```css
/* Document metadata sits under the title, where a reader expects a
   standfirst — not above it, which is why these are body nodes rather than
   a slot in the page shell. */
p.subtitle {
  font-size: 1.15rem;
  color: var(--muted);
  margin: -0.25rem 0 0.5rem;
  text-wrap: balance;
}
p.docdate {
  color: var(--muted);
  font-size: 0.9em;
  margin: 0 0 2.5rem;
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS — whole suite. `convert` is the helper added in Task 2's
`container_test.go`; it is package-level, so these tests reuse it.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add frontmatter.go frontmatter_test.go md2html.go default.css
git commit -m "feat: derive title, subtitle and date from front matter or a lifted italic line"
```

---

### Task 9: Fenced code block captions

Spec item 8. Unlike everything else here this cannot be a transform: goldmark's default renderer takes the first word of the info string as the language and drops the remainder before any HTML exists, so the caption never reaches the tree. Overriding the fenced-code-block renderer is safe for mermaid, which replaces its fences with its own AST node in a parser transform and renders that instead.

**Files:**
- Create: `codefence.go`, `codefence_test.go`
- Modify: `md2html.go` (`newParser`), `default.css`
- Test: `codefence_test.go`

**Interfaces:**
- Consumes: `newParser` (existing, `md2html.go`).
- Produces:
  - `type codeFenceRenderer struct{}` implementing `renderer.NodeRenderer`.
  - `func splitFenceInfo(info string) (lang, caption string)`
  - `func newCodeFenceRenderer() renderer.NodeRenderer`

- [ ] **Step 1: Write the failing test**

Create `codefence_test.go`:

```go
package md2html

import (
	"strings"
	"testing"
)

func TestSplitFenceInfo(t *testing.T) {
	cases := []struct{ in, lang, caption string }{
		{"go", "go", ""},
		{`go caption="server.go"`, "go", "server.go"},
		{`go caption='server.go'`, "go", "server.go"},
		{`go caption="cmd/md2html/main.go" other=1`, "go", "cmd/md2html/main.go"},
		{`caption="just a caption"`, "", "just a caption"},
		{`go caption="with spaces in it"`, "go", "with spaces in it"},
		{"", "", ""},
		{"go someattr", "go", ""},
	}
	for _, c := range cases {
		lang, cap := splitFenceInfo(c.in)
		if lang != c.lang || cap != c.caption {
			t.Errorf("splitFenceInfo(%q) = (%q, %q), want (%q, %q)",
				c.in, lang, cap, c.lang, c.caption)
		}
	}
}

// The whole point: today this caption vanishes without a word.
func TestCodeFenceCaptionRendersAsFigcaption(t *testing.T) {
	got := convert(t, "```go caption=\"server.go\"\nfoo()\n```\n", nil)
	if !strings.Contains(got, `<figure class="code-figure"><figcaption>server.go</figcaption>`) {
		t.Errorf("no caption bar\ngot: %s", got)
	}
	if !strings.Contains(got, `<pre><code class="language-go">foo()`) {
		t.Errorf("code block changed shape\ngot: %s", got)
	}
	if !strings.Contains(got, "</pre></figure>") {
		t.Errorf("figure not closed around the block\ngot: %s", got)
	}
}

// A fence with no caption must render byte-identically to before.
func TestCodeFenceWithoutCaptionIsUnchanged(t *testing.T) {
	got := convert(t, "```go\nfoo()\n```\n", nil)
	if strings.Contains(got, "figure") {
		t.Errorf("wrapped an uncaptioned block\ngot: %s", got)
	}
	if !strings.Contains(got, `<pre><code class="language-go">foo()`) {
		t.Errorf("plain fence changed\ngot: %s", got)
	}
}

func TestCodeFenceWithoutLanguageIsUnchanged(t *testing.T) {
	got := convert(t, "```\nplain\n```\n", nil)
	if !strings.Contains(got, "<pre><code>plain") {
		t.Errorf("bare fence changed\ngot: %s", got)
	}
}

// Code must still be escaped: this replaces goldmark's renderer, so its
// escaping is now this code's responsibility.
func TestCodeFenceEscapesContent(t *testing.T) {
	got := convert(t, "```go caption=\"x\"\n<script>&\n```\n", nil)
	if strings.Contains(got, "<script>") {
		t.Errorf("emitted unescaped markup\ngot: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;&amp;") {
		t.Errorf("content not escaped\ngot: %s", got)
	}
}

// A caption is author-supplied text and must be escaped too.
func TestCodeFenceEscapesCaption(t *testing.T) {
	got := convert(t, "```go caption=\"a<b&c\"\nx\n```\n", nil)
	if strings.Contains(got, "a<b&c") {
		t.Errorf("caption not escaped\ngot: %s", got)
	}
}

// Overriding the fenced-code renderer must not disturb mermaid, which
// replaces its fences with its own AST node before rendering.
func TestCodeFenceOverrideLeavesMermaidAlone(t *testing.T) {
	got := convert(t, "```mermaid\ngraph TD;\nA-->B;\n```\n", nil)
	if !strings.Contains(got, `class="mermaid"`) {
		t.Errorf("mermaid fence broken\ngot: %s", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestCodeFence|TestSplitFenceInfo' -v`
Expected: FAIL — `undefined: splitFenceInfo`.

- [ ] **Step 3: Write the implementation**

Create `codefence.go`:

```go
package md2html

import (
	gohtml "html"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	goldhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// splitFenceInfo separates a fence info string into its language and an
// optional caption:
//
//	```go caption="cmd/md2html/main.go"
//
// The caption is one more space-separated attribute alongside the language,
// not a new fence syntax — info strings already carry attributes, and
// anything this tool does not recognize keeps being ignored exactly as
// goldmark ignores it today. A quoted value may contain spaces.
func splitFenceInfo(info string) (lang, caption string) {
	for i, tok := range fenceTokens(info) {
		if v, ok := strings.CutPrefix(tok, "caption="); ok {
			caption = strings.Trim(v, `"'`)
			continue
		}
		if i == 0 {
			lang = tok
		}
	}
	return lang, caption
}

// fenceTokens splits an info string on whitespace, keeping a quoted run
// together so a caption may contain spaces.
func fenceTokens(s string) []string {
	var out []string
	var cur strings.Builder
	var quote byte
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			cur.WriteByte(c)
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
			cur.WriteByte(c)
		case c == ' ' || c == '\t':
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}

// codeFenceRenderer replaces goldmark's fenced-code-block rendering so a
// caption in the info string becomes a visible bar above the block.
//
// This has to happen at the renderer rather than as a tree transform:
// goldmark takes the first word of the info string as the language and
// discards the rest without a word, so by the time an HTML tree exists the
// caption is gone. That also means today's failure is silent — a fence
// written with a caption renders byte-for-byte like one without.
//
// Mermaid is unaffected: its extension rewrites its fences into its own AST
// node during parsing and registers a renderer for that node, so a mermaid
// fence never reaches this function.
type codeFenceRenderer struct{}

func newCodeFenceRenderer() renderer.NodeRenderer { return &codeFenceRenderer{} }

func (r *codeFenceRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.render)
}

func (r *codeFenceRenderer) render(w util.BufWriter, source []byte, node ast.Node,
	entering bool) (ast.WalkStatus, error) {

	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.FencedCodeBlock)

	var info string
	if n.Info != nil {
		info = string(n.Info.Segment.Value(source))
	}
	lang, caption := splitFenceInfo(info)

	if caption != "" {
		w.WriteString(`<figure class="code-figure"><figcaption>`)
		// The caption is author text, and it is being written into markup
		// this function builds by hand rather than through goldmark's
		// escaping writer.
		w.WriteString(gohtml.EscapeString(caption))
		w.WriteString("</figcaption>")
	}
	w.WriteString("<pre><code")
	if lang != "" {
		w.WriteString(` class="language-`)
		w.Write(util.EscapeHTML([]byte(lang)))
		w.WriteString(`"`)
	}
	w.WriteByte('>')
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		goldhtml.DefaultWriter.RawWrite(w, line.Value(source))
	}
	w.WriteString("</code></pre>")
	if caption != "" {
		w.WriteString("</figure>")
	}
	w.WriteByte('\n')

	// A fenced code block's content is raw lines, not child nodes; skipping
	// children matches what goldmark's own renderer does with it.
	return ast.WalkSkipChildren, nil
}
```

In `md2html.go`, register it in `newParser`:

```go
		goldmark.WithRendererOptions(
			goldhtml.WithUnsafe(),
			// Priority below goldmark's own (1000) so this wins for
			// fenced code blocks; every other node keeps the default
			// renderer.
			renderer.WithNodeRenderers(util.Prioritized(newCodeFenceRenderer(), 100)),
		),
```

Add `"github.com/yuin/goldmark/renderer"` and `"github.com/yuin/goldmark/util"` to `md2html.go`'s import block.

Append to `default.css`:

```css
/* A caption bar that reads as part of the block, not as a floating line of
   prose above it — which is the whole distinction the feature exists for. */
figure.code-figure { margin: 1.5rem 0; }
figure.code-figure > figcaption {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 0.8em;
  color: var(--muted);
  background: var(--code-bg);
  border-bottom: 1px solid var(--rule);
  padding: 0.4rem 1rem;
  border-radius: 6px 6px 0 0;
}
figure.code-figure > pre {
  margin-top: 0;
  border-radius: 0 0 6px 6px;
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS — whole suite, `page_test.go`'s mermaid assertions included.

- [ ] **Step 5: Verify against the binary**

The override reimplements escaping, so confirm it on real output rather
than only in unit tests:

```bash
go build -o /tmp/md2html ./cmd/md2html
mkdir -p /tmp/cf && printf '```go caption="a<b"\n<script>alert(1)</script>\n```\n' > /tmp/cf/x.md
/tmp/md2html /tmp/cf && grep -c '<script>alert' /tmp/cf/x.html
```

Expected: `0` — no unescaped script tag in the output.

- [ ] **Step 6: Commit**

```bash
gofmt -l . && go vet ./...
git add codefence.go codefence_test.go md2html.go default.css
git commit -m "feat: render a caption attribute on a code fence as a figcaption"
```

---

### Task 10: Document every new convention

`docs/authoring.md` is the reference the `md2html-authoring` skill points at, and it states that every claim in it is verified against the binary. Six new conventions land unmentioned without this task, and one of the two silent failures it currently warns about no longer exists.

**Files:**
- Modify: `docs/authoring.md`, `.claude/skills/md2html-authoring/SKILL.md`, `README.md`
- Test: none — prose. Step 4 is its verification.

**Interfaces:**
- Consumes: every feature from Tasks 2–9.
- Produces: no code.

- [ ] **Step 1: Correct the silent-failure section**

In `.claude/skills/md2html-authoring/SKILL.md`, the "two silent failures"
section says a container without braces loses its class. That is no longer
true for the shipped kinds and now warns for anything else. Replace that
subsection:

````markdown
**A container kind outside the shipped set has no styling.**

```markdown
::: callout        -> <div class="callout">
::: {.callout}     -> <div class="callout">          (same thing)
::: house-style    -> <div>, and the run warns
```

The shipped kinds are `callout`, `warning`, `card`, `aside` and `example`.
`aside` and `example` are collapsible `<details>`; the brace-free form takes
a title on the fence line (`::: aside Why this matters`). A braced class
outside the set still emits a correctly classed but unstyled div, silently —
that is deliberate, since the author supplies the CSS.
````

Update the "two silent failures" heading, since only one remains, and
update the `Quick reference` table with rows for the new syntax:

| Need | Write |
|---|---|
| Callout | `::: callout` … `:::` |
| Warning | `::: warning` … `:::` |
| Collapsible aside | `::: aside Title` … `:::` |
| Worked example | `::: example Title` … `:::` |
| Status marker | `[proven]`, or `[c:any label]` |
| Cross-reference | `§4.2` (resolves to the heading numbered 4.2) |
| Contents list | `[[toc]]` alone on a line |
| Title/subtitle/date | `---` front matter, or an italic line under the H1 |
| Code caption | ` ```go caption="server.go" ` |

Also correct the skill's "What has no shortcut" section, which states there
is no table-of-contents generation — `[[toc]]` now covers the per-page
case. Keep the cross-document part: sidebars and site indexes stay out of
scope.

- [ ] **Step 2: Extend the authoring reference**

In `docs/authoring.md`, add one section per feature, in the file's existing
voice, each stating what it renders to and what it degrades to:

- **Containers** — the shipped vocabulary, both syntaxes, the title rule
  (brace-free only, and why: the braced form cannot distinguish a title
  from a first line of body), nesting.
- **Status chips** — the fixed word list, the `[c:label]` form, the fact
  that a chip never enters a heading slug or the page title, and that a
  chip inside a code span stays literal.
- **Section cross-references** — `§4.2` resolves against a heading whose
  text begins `4.2`; no match means plain text; `` `§4.2` `` opts out.
- **Contents list** — `[[toc]]` alone on a line, flat, current page only.
- **Front matter** — `title`, `subtitle`, `date`; flat key/value only; the
  italic-line fallback.
- **Code captions** — the `caption="…"` info-string attribute.

Then update the **Traps** list. Replace the `::: name` bullet and add:

```markdown
- **A cross-reference scoped to another document may still autolink.** Only
  the possessive phrasing ("the design doc's §7") is recognized as
  cross-document. Write `` `§7` `` to keep any other phrasing literal.
- **Front matter must be flat `key: value`.** A nested value makes the whole
  block render as visible text above the title rather than being parsed.
- **Only `caption=` is read from a code fence's info string.** Every other
  attribute is ignored, exactly as before.
```

- [ ] **Step 3: Update README**

Add the new syntax to whatever feature list `README.md` carries, in its
existing style, and note that `Options.Warn` reports non-fatal conversion
problems — it is public API now.

- [ ] **Step 4: Verify every documented claim against the binary**

`docs/authoring.md` promises this. Write one fixture exercising all six
features and check the output:

````bash
go build -o /tmp/md2html ./cmd/md2html
mkdir -p /tmp/doc && cat > /tmp/doc/x.md <<'EOF'
---
title: Reference
subtitle: every new convention
date: 2026-09-11
---

# Reference

[[toc]]

## 4.2 Rollback [proven]

See §4.2. Not a chip: [1]. Not a link: `§4.2`.

::: warning Careful
This overwrites state.
:::

::: aside Why
Because.
:::

```go caption="server.go"
func main() {}
```
EOF
/tmp/md2html /tmp/doc
grep -o 'class="chip[^"]*"\|class="xref"\|<nav class="toc">\|callout-warning\|<details class="container aside">\|code-figure\|class="subtitle"\|class="docdate"' /tmp/doc/x.html | sort -u
grep -o 'id="42-rollback"' /tmp/doc/x.html
````

Expected: every class above appears exactly once in the sorted list, and
the heading id is `42-rollback` — proving the chip stayed out of the slug.
Fix the docs, not the test, if any claim does not hold.

- [ ] **Step 5: Commit**

```bash
git add docs/authoring.md .claude/skills/md2html-authoring/SKILL.md README.md
git commit -m "docs: document containers, chips, cross-references, toc, front matter and code captions"
```

---

## Spec coverage

| Spec item | Where |
|---|---|
| 2 — styling for kinds beyond `.callout` (collapsible, worked example, card, warning) | Tasks 2, 3 |
| 2 — bare `::: kind` accepted as an alias | Task 2 |
| 2 — unknown bare name warns instead of failing silently | Tasks 1, 2 |
| 2 — nesting | Task 2 (already worked; `TestContainerNestingPreserved` locks it in) |
| 2 — inline-Markdown title on the opening fence line | Task 3, **brace-free form only** — see "Two spec claims that are wrong" |
| 2 — arbitrary classes stay inert | Task 2 (`TestContainerArbitraryBracedClassIsSilentAndPreserved`) |
| 4 — fixed status words plus a generic `[c:label]` | Task 5 |
| 4 — applied in body text and headings alike | Task 5 |
| 4 — marker stripped before slugifying, kept in the visible heading | Task 5 (`headingText`, `stripChipTokens`) |
| 5 — `§N` / `§N.M` autolinked against numbered headings | Task 6 |
| 5 — runs after `HeadingAnchors`, resolves against assigned ids | Task 6 |
| 5 — leaves cross-document, in-code, in-link and unmatched references literal | Task 6 |
| 6 — a marker alone on its line becomes a flat `<nav>` | Task 7 |
| 6 — runs after anchors and chip-stripping | Task 7 (transform order; `TestTOCExcludesChipsFromLinkText`) |
| 6 — cross-document navigation stays out of scope | Task 7 (doc comment), Task 10 |
| 7 — leading `---` block with `title`, `subtitle`, `date`, stripped before parsing | Task 8 |
| 7 — italic line after the H1 lifted as a subtitle, comments tolerated | Task 8 |
| 7 — the page gets a subtitle line and a date line | Task 8, **as body nodes under the H1 rather than page-shell slots** — see the task's rationale |
| 8 — caption token in the fence info string, consumed before the language | Task 9 |
| 8 — rendered as `<figure>` + `<figcaption>` above the existing `<pre><code>` | Task 9 |

## Not in this plan

- **Item 1 (structured diagram fence).** The spec defers it to its own
  design, and calls it the largest item in the list by a wide margin.
- **Items 3 and 9 (exclusion directories, traversal modes, supplemental
  entry sets).** Planned separately in
  `docs/plans/2026-09-11-traversal-controls.md`; they share no surface with
  anything here.
- **Titles on braced containers.** Needs an in-repo replacement for
  `goldmark-fences`; a separate design, for the reason given at the top.
