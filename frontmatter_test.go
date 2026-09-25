package md2html

import (
	"strings"
	"testing"
)

func TestSplitFrontMatterExtractsFlatKeys(t *testing.T) {
	meta, body, _ := splitFrontMatter([]byte("---\ntitle: Rollback\nsubtitle: how it works\ndate: 2026-09-11\n---\n\n# H\n"))
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
	meta, _, _ := splitFrontMatter([]byte("---\ntitle: A: B\n---\nx\n"))
	if meta["title"] != "A: B" {
		t.Errorf("title = %q", meta["title"])
	}
}

// Anything that is not flat key/value is left alone and rendered, which is
// today's behavior and an obvious signal that something needs fixing. It
// must also be reported malformed: the block opened as front matter, so
// this is the shape Convert's new warning exists for.
func TestSplitFrontMatterRejectsNonFlatBlock(t *testing.T) {
	src := []byte("---\ntags:\n  - a\n---\nx\n")
	meta, body, malformed := splitFrontMatter(src)
	if meta != nil {
		t.Errorf("parsed a nested block: %v", meta)
	}
	if string(body) != string(src) {
		t.Errorf("body altered: %q", body)
	}
	if !malformed {
		t.Error("nested block not reported malformed")
	}
}

// A horizontal rule at the top of a document is not front matter, and must
// not be reported malformed either: that would warn on every document that
// legitimately opens with a plain "---" rule.
func TestSplitFrontMatterIgnoresPlainRule(t *testing.T) {
	src := []byte("---\n\nsome prose\n")
	meta, body, malformed := splitFrontMatter(src)
	if meta != nil {
		t.Errorf("parsed a plain rule as front matter: %v", meta)
	}
	if string(body) != string(src) {
		t.Errorf("body altered: %q", body)
	}
	if malformed {
		t.Error("plain rule reported malformed")
	}
}

func TestSplitFrontMatterIgnoresBlockNotAtStart(t *testing.T) {
	src := []byte("# H\n\n---\ntitle: X\n---\n")
	meta, _, _ := splitFrontMatter(src)
	if meta != nil {
		t.Errorf("parsed a mid-document block: %v", meta)
	}
}

// A "---" with no closing delimiter is a setext heading underline or an
// unterminated block, not front matter, and must degrade to rendering the
// source as-is rather than swallowing the rest of the document. Not
// malformed either, for the same ambiguity-with-a-rule reason as above.
func TestSplitFrontMatterIgnoresUnterminatedBlock(t *testing.T) {
	src := []byte("---\ntitle: X\n\nno closing delimiter here\n")
	meta, body, malformed := splitFrontMatter(src)
	if meta != nil {
		t.Errorf("parsed an unterminated block: %v", meta)
	}
	if string(body) != string(src) {
		t.Errorf("body altered: %q", body)
	}
	if malformed {
		t.Error("unterminated block reported malformed")
	}
}

// An empty block ("---\n---\n") is still a well-formed, if pointless, front
// matter block: no lines between the delimiters means no keys, but the
// block must still be recognized and stripped rather than treated as an
// unterminated one.
func TestSplitFrontMatterAcceptsEmptyBlock(t *testing.T) {
	meta, body, _ := splitFrontMatter([]byte("---\n---\n\n# H\n"))
	if meta == nil {
		t.Errorf("empty block not recognized as front matter")
	}
	if len(meta) != 0 {
		t.Errorf("meta = %v, want empty", meta)
	}
	if !strings.Contains(string(body), "# H") {
		t.Errorf("body lost: %q", body)
	}
}

// A repeated key overwrites rather than erroring — last-wins, the same
// rule a plain map assignment gives for free.
func TestSplitFrontMatterDuplicateKeyLastWins(t *testing.T) {
	meta, _, _ := splitFrontMatter([]byte("---\ntitle: First\ntitle: Second\n---\nx\n"))
	if meta["title"] != "Second" {
		t.Errorf("title = %q, want %q", meta["title"], "Second")
	}
}

// A flat key outside title/subtitle/date is valid front matter — it is
// still stripped from the body — but is not one of the three keys Convert
// reads, so it is silently dropped rather than rendered or warned about.
func TestSplitFrontMatterKeepsUnrecognizedKeyOutOfBodyOnly(t *testing.T) {
	meta, body, _ := splitFrontMatter([]byte("---\nauthor: Jane\ntitle: T\n---\n\n# H\n"))
	if meta["author"] != "Jane" {
		t.Errorf("author = %q, want %q", meta["author"], "Jane")
	}
	if strings.Contains(string(body), "author:") || strings.Contains(string(body), "Jane") {
		t.Errorf("unrecognized key not stripped from body: %q", body)
	}
	got := convert(t, "---\nauthor: Jane\ntitle: T\n---\n\n# H\n", nil)
	if strings.Contains(got, "Jane") {
		t.Errorf("unrecognized key leaked into rendered output\ngot: %s", got)
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

// the fragile case is a comment BETWEEN the h1 and the italic line,
// not one before the h1 (the heading search skips over that regardless).
func TestConvertLiftsSubtitlePastCommentBetweenHeadingAndItalic(t *testing.T) {
	got := convert(t, "# Title\n\n<!-- generated -->\n\n*sub*\n\nbody\n", nil)
	if !strings.Contains(got, `<p class="subtitle">sub</p>`) {
		t.Errorf("comment between heading and italic line blocked the lift\ngot: %s", got)
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
// both facts are asserted independently rather than joined by &&, so
// a broken guard on either one is caught rather than the check
// short-circuiting away.
func TestConvertFrontMatterSubtitleBeatsItalicLift(t *testing.T) {
	got := convert(t, "---\nsubtitle: declared\n---\n\n# T\n\n*lifted*\n", nil)
	if !strings.Contains(got, `<p class="subtitle">declared</p>`) {
		t.Errorf("front matter lost\ngot: %s", got)
	}
	if strings.Count(got, "subtitle") != 1 {
		t.Errorf("more than one subtitle emitted\ngot: %s", got)
	}
	if !strings.Contains(got, "<p><em>lifted</em></p>") {
		t.Errorf("italic line under the heading should render as plain prose\ngot: %s", got)
	}
}

// the lift is a fallback for documents with no explicit title, not
// merely no explicit subtitle. Front matter declaring only "title:" must
// leave an italic line under the h1 alone, or a title-only document would
// silently lose a line of body text to a subtitle it never asked for.
func TestConvertFrontMatterTitleOnlySuppressesItalicLift(t *testing.T) {
	got := convert(t, "---\ntitle: Real Title\n---\n\n# T\n\n*just italics*\n", nil)
	if strings.Contains(got, "subtitle") {
		t.Errorf("lift fired despite an explicit title\ngot: %s", got)
	}
	if !strings.Contains(got, "<p><em>just italics</em></p>") {
		t.Errorf("italic line should render as an ordinary paragraph\ngot: %s", got)
	}
}

// an explicit Options.Title also suppresses the lift, the same as a
// front-matter title.
func TestConvertOptionsTitleSuppressesItalicLift(t *testing.T) {
	out, err := Convert([]byte("# T\n\n*just italics*\n"),
		Options{Fragment: true, CSS: "/**/", Title: "From Caller"})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Contains(got, "subtitle") {
		t.Errorf("lift fired despite Options.Title\ngot: %s", got)
	}
	if !strings.Contains(got, "<p><em>just italics</em></p>") {
		t.Errorf("italic line should render as an ordinary paragraph\ngot: %s", got)
	}
}

// front matter supplying a date but no subtitle, combined with an
// italic line under the h1 (so the lift still fires), must place
// the date AFTER the lifted subtitle. applyDocMeta inserts relative to the
// heading; if it fires before the lift, or advances its anchor
// incorrectly, the date lands between the heading and the subtitle.
func TestConvertFrontMatterDatePlusItalicLiftOrdersDateAfterSubtitle(t *testing.T) {
	got := convert(t, "---\ndate: 2026-09-11\n---\n\n# T\n\n*lifted*\n\nbody\n", nil)
	if !strings.Contains(got, `<p class="subtitle">lifted</p>`) {
		t.Errorf("italic line not lifted\ngot: %s", got)
	}
	if !strings.Contains(got, `<p class="docdate">2026-09-11</p>`) {
		t.Errorf("no date\ngot: %s", got)
	}
	subIdx := strings.Index(got, `class="subtitle"`)
	dateIdx := strings.Index(got, `class="docdate"`)
	if subIdx < 0 || dateIdx < 0 || dateIdx < subIdx {
		t.Errorf("date must come after the lifted subtitle\ngot: %s", got)
	}
}

// Fix for the reviewer-reported bug: applyDocMeta's "anchor == nil" branch
// (no leading h1 at all) returned before advancing anchor to the just-
// inserted subtitle paragraph, so the date's insertion still saw anchor
// == nil and landed before root.FirstChild — ahead of the subtitle it was
// meant to follow. This is the no-h1 counterpart of
// TestConvertFrontMatterSubtitleAndDateRender, which only exercises the
// h1-anchored path.
func TestConvertFrontMatterSubtitleAndDateOrderWithoutHeading(t *testing.T) {
	got := convert(t, "---\nsubtitle: Sub\ndate: 2026-01-01\n---\n\nSome prose.\n", nil)
	if !strings.Contains(got, `<p class="subtitle">Sub</p>`) {
		t.Errorf("no subtitle\ngot: %s", got)
	}
	if !strings.Contains(got, `<p class="docdate">2026-01-01</p>`) {
		t.Errorf("no date\ngot: %s", got)
	}
	subIdx := strings.Index(got, `class="subtitle"`)
	dateIdx := strings.Index(got, `class="docdate"`)
	if subIdx < 0 || dateIdx < 0 || dateIdx < subIdx {
		t.Errorf("date must come after subtitle even with no leading h1\ngot: %s", got)
	}
}

// Malformed front matter degrades to rendering (an <hr> plus a setext
// heading) with no warning today, which is the one new failure mode not
// reaching Options.Warn. Convert must report it.
func TestConvertWarnsOnMalformedFrontMatter(t *testing.T) {
	var msgs []string
	got := convert(t, "---\ntags:\n  - a\n---\n\n# H\n",
		func(m string) { msgs = append(msgs, m) })
	var found bool
	for _, m := range msgs {
		if strings.Contains(m, "flat") {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning for malformed front matter, got %v", msgs)
	}
	// Still degrades to rendering the block as body text, exactly as today.
	if !strings.Contains(got, "<hr") {
		t.Errorf("malformed block did not degrade to rendered markup\ngot: %s", got)
	}
}

// A nil sink must stay safe for the front-matter warning too, same as every
// other Warn call site.
func TestConvertMalformedFrontMatterNilWarnSinkIsSafe(t *testing.T) {
	if _, err := Convert([]byte("---\ntags:\n  - a\n---\n\n# H\n"), Options{Fragment: true}); err != nil {
		t.Fatalf("Convert: %v", err)
	}
}

// Front matter is written as YAML, where quoting a value is ordinary, and
// the quotes are YAML syntax rather than part of the value. So one pair
// around the whole value is removed, with YAML's escapes inside it: \" and
// \\ in double quotes, a doubled single quote in single quotes. Quotes
// that do not wrap the whole value are text.
func TestSplitFrontMatterUnquotesValues(t *testing.T) {
	meta, _, _ := splitFrontMatter([]byte("---\n" +
		"lang: \"de\"\n" +
		"toc-title: 'Inhalt'\n" +
		"title: \"Say \\\"hi\\\" \\\\ bye\"\n" +
		"subtitle: 'it''s here'\n" +
		"date: \"2026\" and more\n" +
		"a: \"\n" +
		"b: \"mismatched'\n" +
		"---\n# H\n"))
	for k, want := range map[string]string{
		"lang":      "de",
		"toc-title": "Inhalt",
		"title":     `Say "hi" \ bye`,
		"subtitle":  "it's here",
		"date":      `"2026" and more`,
		"a":         `"`,
		"b":         `"mismatched'`,
	} {
		if meta[k] != want {
			t.Errorf("%s = %q, want %q", k, meta[k], want)
		}
	}
}

// The case that surfaced it: a quoted lang used to fail the language-tag
// check and fall back to en.
func TestConvertQuotedLangIsUsed(t *testing.T) {
	if got, warns := langOf(t, "---\nlang: \"de\"\n---\n# T\n", Options{}); got != "de" || len(warns) != 0 {
		t.Errorf("lang = %q, warnings %v", got, warns)
	}
}
