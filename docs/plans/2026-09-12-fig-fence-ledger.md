# Structured diagram fence — execution ledger

**Date:** 2026-09-12
**Plan:** [2026-09-12-fig-fence.md](./2026-09-12-fig-fence.md)
**Spec:** [2026-09-12-fig-fence-design.md](../specs/2026-09-12-fig-fence-design.md)
**Status:** Complete — 25 commits, both suites green, nothing pushed

## What this file is

The running record of executing the `fig` fence plan: one subagent per task,
a review after each, and a whole-branch review at the end. It is kept for one
reason — it is the only place the decisions taken *between* the spec and the
commits are written down.

Two kinds of entry are worth reading later:

- **`Ruling:` lines** record every judgement call made during execution, each
  with what it costs if it turns out wrong. Where the code and the spec
  disagree, a ruling usually explains why.
- **Findings and their verdicts** record what each review caught. Two are
  load-bearing for anyone changing this code: `.fig-cols > .fig-arrow` was
  dead CSS because `cols` wraps every item in a `.fig-panel`, and `inline()`
  was a full block-level Markdown pass rather than an inline one, which let a
  figure label inject headings into the host document.


Spec: docs/specs/2026-09-12-fig-fence-design.md (read — binding authority)
BASE at start: cf43379

## Pre-flight scan

### Shared-file / interface pairs

| Tasks | Producer -> consumer | Finding |
|---|---|---|
| T1 -> T2 | renderFig, figDoc, figItem -> validateFig call site | OK. T2 inserts the call right after decode. |
| T1 -> T3 | figConvert helper in fig_test.go -> used by figrender_test.go | RISK: T3 may redefine figConvert (duplicate symbol, same package). Carry in dispatch. |
| T1 -> T4,T5,T6 | (*figRenderer).item switch -> new cases appended | OK. Each task appends disjoint cases. |
| T1 -> T7 | renderFig body -> rewritten into a layout switch | RISK: T7's replacement text begins at `r := newFigRenderer()`, i.e. AFTER T2's validateFig call. Call survives only if implementer does not over-replace. Carry in dispatch. |
| T2 -> T7 | validateFig guarantees split has exactly 2 items -> T7 indexes Items[0]/[1] | OK. T7 comments the dependency. |
| T2 -> T9 | validation warnings -> gallery expects exactly 1 warning | OK. Gallery has exactly one bad fence (boxes: typo). |
| T6 -> T7 | (*figRenderer).items -> used by T7's rows default | OK. T6 lands it first. |
| T7 -> T9 | --fig-weight:N -> gallery asserts --fig-weight:3 | OK. Gallery's cols figure sets weight: 3. |
| T7 -> T10 | .fig-panel wrapper -> browser test queries .fig-panel | OK. Test uses layout: cols. |
| T1,T4..T7 -> T8 | emitted class names -> CSS selector existence test | OK. Verified name-by-name against the emitters. |
| T8 -> T10 | @media (max-width: 34rem) -> stacking assertion | OK. T10 step 2 disables it to prove the test fails. |
| T1 -> T11 | newParser(warn) -> Warn doc comment rewrite | OK. Different region of md2html.go. |
| T3..T7 | all append to figrender_test.go created by T3 | OK. Append-only, disjoint funcs. |

### Per-task self-consistency

| Task | Own text agrees with itself? |
|---|---|
| T1 | DEFECT: Files block omits `figrender.go`, but Step 6 creates it and Step 8 commits it. |
| T2 | OK. Tests name messages the validate code actually produces. |
| T3 | OK, but unusual: may legitimately change no production code (regression guard only). |
| T4 | OK. |
| T5 | OK. Golden strings match the emitter byte-for-byte. |
| T6 | OK. Also simplifies renderFig's loop; T7 later replaces that region. |
| T7 | OK. TestFigSplitNeedsExactlyTwoItems passes from T2 — noted in the task. |
| T8 | OK. Colour-literal regex cannot match `figure.fig {` (requires `.fig-`). |
| T9 | OK. Gallery is a live `fig` corpus; docs examples stay `yaml`. |
| T10 | OK. |
| T11 | OK. |

## Rulings

Ruling: T1's Files block gains `figrender.go` (Create) — Step 6 and Step 8 already
create and commit it, so the omission is a transcription slip, not a design choice.
Cost if wrong: none; the file is created either way.

Ruling: T3 may land with no production-code change. Its value is as a regression
guard on the reparse architecture the spec depends on. If TestFigTransformsReachInsideFigures
FAILS, that is a spec-level finding, not a test bug — escalate rather than weaken it.
Cost if wrong: one task whose diff is tests only, which a reviewer may flag as thin.

Ruling: T1 -> T3 figConvert duplication and T1 -> T7 validateFig survival are carried
as explicit dispatch notes rather than plan edits — they are coordination facts a
fresh subagent cannot see, not defects in the plan text.
Cost if wrong: a compile error or a dropped validation call, both caught by the task review.

## Task 1

Implementer: DONE_WITH_CONCERNS, commit 627e4c5 (base 9cc9cfc).
Tests: go test ./... green both packages; go vet clean.

Two defects found in MY artifacts, both fixed by the implementer in-commit and
now also corrected in the plan text (commit follows):
- Plan Task 1 Step 4 and my dispatch both claimed newParser has one call site.
  It has two — crawl.go's parseDoc also calls it. Implementer updated it to
  newParser(nil). Ruling: correct — the discovery pass has no warning sink and
  must not invent one; a figure's warning belongs to the emit pass only.
  Cost if wrong: discovery-pass fig warnings are silently dropped (intended).
- Plan Task 1 Step 5's snippet `lines.At(i).Value(source)` does not compile:
  text.Segment.Value has a pointer receiver, At() returns a value. Implementer
  bound a local first, matching the existing loop in the same function.
  Ruling: correct and idiomatic for this file. Cost if wrong: none.

Task 1: review dispatched (package review-9cc9cfc..627e4c5.diff).
Task 1: review clean — spec ✅, quality approved, 0 Critical, 0 Important.
Task 1: minor (deferred): go.mod marks gopkg.in/yaml.v3 `// indirect` though fig.go
  imports it directly; `go mod tidy` moves it to the first require block. No
  functional effect. Carried into Task 2's dispatch (Task 2 touches fig.go).
Task 1: complete (commits 9cc9cfc..627e4c5, review clean)

## Task 2

Implementer: DONE, commits df0251c (validation) + 4874214 (go mod tidy), base 2ec5e58.
Tests: go test ./... green both packages; new tests failed pre-implementation as expected.
Deferred Minor from Task 1 (yaml.v3 // indirect) fixed here in its own commit.
Task 2: review dispatched (package review-2ec5e58..4874214.diff).
Task 2: review — spec ✅, quality approved, 1 Important (plan-mandated), 1 Minor.

Ruling: Task 2 Important (untested validation branches) — PARTIALLY UPHELD, fix now.
  Checked Task 7's brief: TestFigSplitNeedsExactlyTwoItems DOES exist there, so the
  split-count branch is covered — the reviewer lacked cross-task context. The
  layout-value branch (`layout: bogus`) is covered NOWHERE in the plan; that half of
  the finding is real. Decision: add BOTH cases to Task 2, colocated with the code
  they pin, and drop the duplicate from Task 7 (its implementer will be told it
  already exists). Colocation also means the invariant Task 7 indexes on is pinned
  before Task 7 starts, not in the same commit range that begins relying on it.
  Cost if wrong: two extra test cases in fig_test.go and one fewer in figrender_test.go.
Task 2: minor (deferred): fig.go's two warn-and-return-fallback blocks are verbatim
  duplicates (differ only in the error variable). Worth a helper only at a third caller.
Task 2: fix round 1/5 (1 addressed, 0 open — both validation branches now tested; commits 4874214..0fc8d3c)
Task 2: complete (commits 2ec5e58..0fc8d3c, review clean)
Plan amended: Task 7 no longer duplicates the split-count test; brief re-extracted.

## Task 3

Implementer: DONE_WITH_CONCERNS, commit 16e737b, base 4b847ae.
3/4 tests pass with ZERO production change — including both reparse-architecture
guards. The spec's central claim (render markup -> parseFragment -> transforms, so
transforms reach inside figures) is CONFIRMED, not assumed.

Ruling: TestFigSurvivesFragmentMode left the tree RED — it asserts both that figure
  markup survives --fragment (Task 3 can satisfy) and that the fragment carries
  .fig-box styles (Task 8's deliverable). My pre-flight scan missed this; I wrote
  that test during plan self-review and never checked which task could satisfy it.
  Decision: split the test by owner rather than skip, weaken, or let main sit red.
  Task 3 keeps the markup assertion; Task 8 gains TestFigStylesTravelWithAFragment
  for the styles assertion. Both halves survive, each in the task that can turn it
  green. Cost if wrong: one extra test function, and the styles claim goes unpinned
  until Task 8 instead of Task 3.
  Implementer was right not to patch default.css from Task 3 — that would have
  collided with Task 8's no-literal-colors check.
Task 3: fix round 1/5 (1 addressed, 0 open — fragment test split; commits 16e737b..f5a9d84)
Task 3: review clean — spec ✅, quality approved, 0 findings. Reviewer independently
  re-derived the goldmark inline expectations with a standalone program rather than
  trusting the implementation's own output.
Task 3: complete (commits 4b847ae..f5a9d84, review clean)
  KEY RESULT: the spec's reparse architecture is confirmed, not assumed — both
  transform-reach guards passed with zero production changes.

## Tasks 4+5 (batched)

Batched per the same-shape rule: both are "append cases to (*figRenderer).item's
switch, append tests to figrender_test.go", both with complete code in the plan, neither
needing judgment. Task 6 stays separate — it adds recursion, a new helper, and touches fig.go.
Tasks 4+5: review clean — spec ✅ both, quality approved both, 0 findings.
  Reviewer resolved its own ⚠️ item (kind-exclusivity lives in fig.go, outside the
  diff) by reading the file rather than deferring it to the controller.
Tasks 4+5: complete (commits f5a9d84..e9952a6, review clean)
  Batching verdict: worked well — one dispatch, one review, two clean commits.

## Task 6

Task 6: review clean — spec ✅, quality approved, 0 findings. Reviewer independently
  confirmed the validateFig guard survived the renderFig rewrite from the diff itself
  rather than trusting the implementer's claim. Controller also verified (fig.go:159).
Task 6: complete (commits e9952a6..07556e5, review clean)

## Task 7

Task 7: review clean — spec ✅, quality approved, 0 findings. Reviewer verified the
  unchecked split indexing is genuinely unreachable without validation by tracing every
  entry path, not by confirming the guard exists. Controller also verified (fig.go:159).
Task 7: complete (commits 07556e5..eade2fe, review clean)

## Task 8

Task 8: review — spec ✅, quality NEEDS FIXES. 1 Important (plan-mandated), 1 Minor.

Ruling: Task 8 Important (.fig-cols > .fig-arrow::before is dead CSS) — UPHELD, fix now.
  Real and verifiable: fig.go wraps EVERY top-level cols item in .fig-panel, so a
  top-level arrow is a grandchild of .fig-cols and the child combinator never matches.
  An arrow in a cols layout renders as an empty box with no glyph. The defect is mine —
  it came verbatim from the plan's CSS, which I wrote without checking it against the
  markup the renderers actually emit. Cost if wrong: none; the fix is strictly more
  specific than what shipped.

Ruling: Task 8 Minor (.fig-group has no arrow-direction rule) — ALSO FIX, not deferred.
  It is the identical failure mode in a second container, and a group is a column like
  .fig-rows. Deferring a known-identical bug because the brief's checklist did not name
  it is how the same defect ships twice. One line. Cost if wrong: an arrow glyph appears
  inside a group where the design might have preferred none.

Ruling: fix uses panel-aware CHILD combinators (.fig-cols > .fig-panel > .fig-arrow),
  NOT a descendant selector (.fig-cols .fig-arrow). A descendant selector would give a
  right-arrow to an arrow nested inside a column container within a cols panel, and
  would tie with .fig-group's rule on specificity — making correctness depend on rule
  order. Cost if wrong: a deeply nested arrow keeps the default (no glyph) rather than
  inheriting a wrong one.
Task 8: fix round 1/5 (2 addressed, 0 open — arrow selectors rerouted through .fig-panel,
  .fig-group rule added, selector-pinning test added; commits c3ce24d..739f482)
  Re-reviewer independently confirmed the ruling's specificity reasoning holds for the
  real nested shape (cols > panel > group > arrow), rather than just confirming the edit.
Task 8: complete (commits eade2fe..739f482, review clean)

## Task 9

Plan amended before dispatch: the gallery's cols figure now carries a panel-level arrow
AND an arrow nested inside a group, and the gallery test pins the panel-level shape.
Reason: the dead-selector bug Task 8's review caught was invisible to every existing
test, and the corpus is supposed to be the thing that catches exactly that. A corpus
that never renders the broken shape would not have caught it either.
Task 9: controller VISUAL CHECK done in a real browser (Chromium via playwright), not
  delegated — this is the step no markup test can do. Rendered testdata/figures.md and
  read computed styles:
  Desktop (1280w):
    - all 8 arrows resolve a ::before glyph; the two shapes from Task 8's bug both work:
      .fig-group > .fig-arrow inside a panel = "↓", .fig-panel > .fig-arrow in cols = "→".
      Both rendered EMPTY before the fix — this is direct proof the fix works in an engine,
      not just that the selector string changed.
    - cols panels share a row (all top=1964) and weights compute exactly 3:1 (402 vs 134px).
    - split = panel/boundary/panel in order; lanes side by side; degradation case still a
      visible code.language-fig block.
  Phone (390w):
    - every cols panel stacks (all left=20, tops differ), weights neutralized (all 335px),
      panel-level arrow flips to "↓", lanes and split stack, no horizontal scroll.
  This de-risks Task 10: the behavior its browser test asserts is already confirmed.
Task 9: review clean — spec ✅, quality approved, 0 Critical, 0 Important, 1 Minor.
Task 9: minor (deferred): TestFigGalleryRenders' want-list checks "fig-cols" and
  "fig-split" but not "fig-rows", so a regression dropping only the rows wrapper would
  slip through. One-line fix. Plan-inherited. No remaining task touches fig_test.go, so
  this goes to the FINAL REVIEW's fix wave rather than getting its own dispatch.
Task 9: complete (commits 739f482..d7fd8c7, review clean)

## Finding from the controller's visual check (raised against Task 8's stylesheet)

VISUAL DEFECT, found by looking at the rendered gallery — exactly the class of defect
the gallery exists to catch, and invisible to every markup test:

`.fig-defs` renders with NO visual container. Its only rules are `margin: 0`, a bold
`dt`, and a muted `dd`. Inside a figure where every other element is a bordered,
centered box or tile, the term/definition block sits flush-left as bare prose and reads
as unstyled content that leaked into the figure rather than part of it. The spec calls
this kind a "term/definition GRID"; rendered, it is not a grid and has no figure-like
treatment at all.

Severity: Minor (cosmetic, nothing is wrong or misleading) but REAL — and ignoring it
would mean the gallery did its job and the result was discarded, which is the one
outcome that makes the corpus worthless.

Ruling: do NOT open a fix round on an already-approved task for a cosmetic issue, and
do NOT fix it in the controller session. Route it to the FINAL REVIEW's single fix wave,
together with Task 9's deferred fig-rows minor. Cost if wrong: defs ships looking like
bare prose inside an otherwise-boxed figure until someone restyles it.

## Task 10

Task 10: review clean — spec ✅, quality approved, 0 findings. Reviewer confirmed the
  TDD failure was the RIGHT failure (left edges 20 vs 199 at 390px, not a timeout), that
  the len()!=2 guard makes a missing element fail loudly rather than pass silently, and
  that the no-sleep reasoning is sound. default.css byte-identical.
Task 10: complete (commits d7fd8c7..f723290, review clean)

## Task 11

Task 11: review clean — spec ✅, quality approved, 0 Critical, 0 Important, 2 Minor.
  Reviewer re-verified EVERY documented claim against fig.go/figrender.go rather than
  against the brief, which is the right standard for a doc that promises binary-verified
  accuracy. Also confirmed the Warn routing claim by tracing newParser(opt.Warn) (always
  called) vs builtins(opt.Warn) (only when Transforms is nil).
Task 11: minor (deferred): docs say weight is "(1-12)" without saying out-of-range is
  silently CLAMPED, not rejected. In a section whose other rules are validation errors,
  a reader may infer weight:99 is an authoring error. It is not — figWeight clamps it
  with no warning. Real accuracy gap in a doc that promises verified accuracy.
Task 11: minor (deferred): the bolded lead "A figure that does not parse renders as a
  code block and warns" covers only YAML decode failures, but the same paragraph's own
  example (bare `box:`) is a VALIDATION failure — a different code path.
Task 11: complete (commits f723290..3e602c4, review clean)

## All 11 tasks complete

Controller verification before final review: go test ./... PASS; go test -tags
e2e_browser ./... PASS; go vet clean; working tree clean; 21 commits on main.

FOR THE FINAL REVIEW'S FIX WAVE — deferred minors and the visual finding:
1. Task 9: TestFigGalleryRenders' want-list omits "fig-rows".
2. Task 11: weight documented as "(1-12)" without saying out-of-range is clamped silently.
3. Task 11: "does not parse" should be "does not parse or validate".
4. Controller visual finding: .fig-defs has no visual container and reads as bare prose
   inside an otherwise-boxed figure.

## Final whole-branch review

Verdict: READY TO MERGE WITH FIXES. Fault-path invariant HOLDS under adversarial probing
(recursion depth 9990, alias bombs, int overflow, split 0/1/3 items, null docs — no panic,
no double-warn, exactly one warning per faulty fence). All 11 rulings assessed SOUND, none
need rework. Spec Layout section fully delivered.

2 Critical, 2 Important, plus the 4 triaged deferred items.

CRITICAL 1 — figrender.go inline() is a full BLOCK-level Markdown pass, not an inline one.
  `box: "1. Validate"` becomes an ordered list; `box: "## Real"` injects a real <h2> that
  competes for heading ids, lands in [[toc]], and enters the namespace §-xrefs resolve
  against; a block scalar emits unbalanced <p>. Both the spec and the shipped docs promise
  INLINE. This is the feature's central text contract and it was never implemented.
  The per-task reviews could not see it: every test used single-word labels.

CRITICAL 2 — fig.go parseFig calls dec.Decode once, so a body with a `---` separator
  silently drops every document after the first, with ZERO warnings. Exactly the
  "item silently vanishes" failure KnownFields exists to prevent, through the other door.

IMPORTANT 3 — codefence.go computes `caption` for every fence but the fig branch returns
  before using it: an info-string caption="..." is DROPPED when the figure renders and
  DISPLAYED when it degrades. Same fence shows its caption only when broken.
  Ruling: WARN that it is ignored rather than adopting it as a fallback. Silently dropping
  author text is unacceptable, but adopting it would create a second way to spell one
  thing, against the spec's single-vocabulary stance. Cost if wrong: an author who writes
  caption= gets a diagnostic instead of a rendered caption.

IMPORTANT 4 — fig.go emits the yaml.v3 error verbatim: multi-line (breaking the CLI's
  one-line-per-warning format) and leaking the internal Go type `md2html.figItem` to authors.

Ruling: ONE fix wave covering both Criticals, both Importants, the two doc fixes, the
  .fig-defs styling, and four free minors. Per the process there is no second wave — residual
  findings surface to the user. Critical 1 is a goldmark parser reconfiguration, not a
  transcription.

## Fix-wave re-review — ALL ADDRESSED, MERGE RECOMMENDED

Every Critical and Important closed, each verified BY EXECUTION not by reading. Confirmed:
inline() now emits no block markup for any of the 6 broken inputs while code spans,
emphasis, links, autolinks, entities and raw inline HTML all still work; `box: "## Fake"`
no longer reaches the heading/TOC/anchor namespace; multi-doc bodies warn once and degrade;
one-warning invariant holds across 10 fault shapes; no false positives on 7 legitimate
`---` shapes (quoted, block scalar, folded, leading marker, trailing `...`).

Re-reviewer UPHELD my caption ruling and gave it a better reason than I did: there is no
steady state in which a fig fence both displays its info-string caption and is otherwise
correct, so the unconditional warning can never fire on a fence an author should leave alone.

Ruling: Important 4's test-expectation change ("field boxes not found" -> `unknown key
  "boxes"`) is a TIGHTENING, not a loosening — the old string was yaml.v3's literal phrasing,
  i.e. the exact bug being removed, so keeping it would have made the test assert the bug.
  Re-reviewer independently reached the same conclusion. Cost if wrong: none.

RESIDUAL MINORS — adjudicated, all PARKED (no second fix wave per the process):
Ruling: PARKED — hard line breaks in a label no longer render ("a  \nb" was a<br>b, now
  "a b"). Real regression from base, introduced by the newline-collapse the inline fix
  needed. Parked because a figure label is one line by construction, the spec promises only
  "a real goldmark inline pass" and never mentions hard breaks, no fixture/doc/test uses
  one, and `box: "<br>"` still works under WithUnsafe. Cost if wrong: an author wanting a
  two-line label must write <br> explicitly.
Ruling: PARKED — garbage after a `...` end marker reports "remove the --- separator" when
  no `---` is present. Diagnosis right, prescription names the wrong character. Rare and
  self-correcting once the author looks at the fence. Cost if wrong: one confusing message
  in an unusual case.
Ruling: PARKED — figFault assigns parts = te.Errors then writes parts[i], aliasing a
  caller-owned slice. Harmless today (error discarded immediately, rewrites idempotent) but
  a latent hazard; one-line fix is `parts = append([]string(nil), te.Errors...)`.
  Cost if wrong: a future caller reusing the error sees rewritten text.
Ruling: NOT A FINDING — trailing bare `---` rejected as multi-document. It genuinely IS a
  second (null) document and "remove the --- separator" is exactly right.
Ruling: PARKED (out of scope, pre-existing) — a `%YAML 1.2` directive yields yaml.v3's
  "found incompatible YAML document", the one remaining message an author cannot act on.

FINAL STATE: 25 commits from cf43379. go test ./... PASS (browser-free), go test -tags
e2e_browser ./... PASS, go vet clean, gofmt clean, working tree clean.
