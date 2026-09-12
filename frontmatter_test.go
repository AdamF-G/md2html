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

// A "---" with no closing delimiter is a setext heading underline or an
// unterminated block, not front matter, and must degrade to rendering the
// source as-is rather than swallowing the rest of the document.
func TestSplitFrontMatterIgnoresUnterminatedBlock(t *testing.T) {
	src := []byte("---\ntitle: X\n\nno closing delimiter here\n")
	meta, body := splitFrontMatter(src)
	if meta != nil {
		t.Errorf("parsed an unterminated block: %v", meta)
	}
	if string(body) != string(src) {
		t.Errorf("body altered: %q", body)
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

// R20: the fragile case is a comment BETWEEN the h1 and the italic line,
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
// R18: both facts are asserted independently rather than joined by &&, so
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

// R26: the lift is a fallback for documents with no explicit title, not
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

// R26: an explicit Options.Title also suppresses the lift, the same as a
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

// R19: front matter supplying a date but no subtitle, combined with an
// italic line under the h1 (so the lift still fires per R26), must place
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
