package md2html

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func apply(t *testing.T, in string, tr Transform) string {
	t.Helper()
	root, err := parseFragment([]byte(in))
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	if err := tr.Fn(root); err != nil {
		t.Fatalf("%s: %v", tr.Name, err)
	}
	out, err := renderTree(root)
	if err != nil {
		t.Fatalf("renderTree: %v", err)
	}
	return string(out)
}

// Both a generated table and a hand-written one inside raw HTML must be
// wrapped. This is the capability the whole x/net/html reparse buys.
func TestTableScrollWrapsBothGeneratedAndRawTables(t *testing.T) {
	in := `<figure><table id="raw"></table></figure><table id="gen"></table>`
	got := apply(t, in, TableScroll())
	if n := strings.Count(got, `<div class="table-scroll">`); n != 2 {
		t.Errorf("wrapped %d tables, want 2\ngot: %s", n, got)
	}
	if !strings.Contains(got, `<div class="table-scroll"><table id="raw">`) {
		t.Errorf("raw-HTML table not wrapped\ngot: %s", got)
	}
}

func TestTableScrollDoesNotDoubleWrap(t *testing.T) {
	got := apply(t, `<div class="table-scroll"><table></table></div>`, TableScroll())
	if n := strings.Count(got, `table-scroll`); n != 1 {
		t.Errorf("double-wrapped: %s", got)
	}
}

func TestHeadingAnchorsAddsIDAndLink(t *testing.T) {
	got := apply(t, `<h2>Hello World</h2>`, HeadingAnchors())
	if !strings.Contains(got, `id="hello-world"`) {
		t.Errorf("no slug id\ngot: %s", got)
	}
	if !strings.Contains(got, `<a class="anchor" href="#hello-world"`) {
		t.Errorf("no anchor link\ngot: %s", got)
	}
}

func TestHeadingAnchorsPreservesExistingID(t *testing.T) {
	got := apply(t, `<h2 id="custom-id">Hello</h2>`, HeadingAnchors())
	if !strings.Contains(got, `id="custom-id"`) {
		t.Errorf("clobbered explicit id\ngot: %s", got)
	}
	if strings.Contains(got, `id="hello"`) {
		t.Errorf("added a slug id despite explicit one\ngot: %s", got)
	}
}

// Repeats are numbered from 1, as GitHub, GitLab and Pandoc number them, so
// a link to the second "Setup" written against any of them resolves here.
func TestHeadingAnchorsDeduplicatesSlugs(t *testing.T) {
	got := apply(t, `<h2>Setup</h2><h2>Setup</h2>`, HeadingAnchors())
	if !strings.Contains(got, `id="setup"`) || !strings.Contains(got, `id="setup-1"`) {
		t.Errorf("duplicate slugs not disambiguated\ngot: %s", got)
	}
}

// A generated slug and an explicit id can collide in either direction: a
// later heading's natural slug can land on an earlier heading's suffixed
// slug, and a generated suffix can land on an explicit id that appears
// further down. Both must be disambiguated, and the explicit id must be
// the one left as the author wrote it.
func TestHeadingAnchorsAvoidsSuffixCollision(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		exact string // an id that must survive verbatim; "" for none
	}{
		{
			name: "generated slug collides with an earlier suffix",
			in:   `<h2>Setup</h2><h2>Setup</h2><h2>Setup 1</h2>`,
		},
		{
			name:  "generated suffix collides with a later explicit id",
			in:    `<h2>Setup</h2><h2>Setup</h2><h2 id="setup-1">Other</h2>`,
			exact: "setup-1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := apply(t, tc.in, HeadingAnchors())
			root, err := parseFragment([]byte(got))
			if err != nil {
				t.Fatalf("parseFragment: %v", err)
			}
			seen := map[string]bool{}
			walk(root, func(n *html.Node) {
				if n.Type != html.ElementNode || n.DataAtom != atom.H2 {
					return
				}
				id, ok := attr(n, "id")
				if !ok {
					t.Fatalf("heading missing id\ngot: %s", got)
				}
				if seen[id] {
					t.Errorf("duplicate id %q\ngot: %s", id, got)
				}
				seen[id] = true
			})
			if len(seen) != 3 {
				t.Fatalf("expected 3 distinct ids, got %d\ngot: %s", len(seen), got)
			}
			if tc.exact != "" && !seen[tc.exact] {
				t.Errorf("explicit id %q was rewritten\ngot: %s", tc.exact, got)
			}
		})
	}
}

// Headings in scripts other than Latin must still get an id and an anchor.
// Dropping every non-ASCII rune left CJK, Cyrillic and Greek headings with no
// id at all, so nothing in a non-English document was deep-linkable.
func TestSlugifyKeepsLettersFromAnyScript(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Hello World", "hello-world"},
		{"Step 1: Init", "step-1-init"},
		// Previously collapsed to "" and lost their anchors entirely.
		{"日本語の見出し", "日本語の見出し"},
		{"Привет мир", "привет-мир"},
		{"Καλημέρα", "καλημέρα"},
		{"Café Menu", "café-menu"},
	} {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Slugs follow the rules GitHub, GitLab and Pandoc's commonmark_x share:
// punctuation is dropped but the whitespace around it still becomes a
// hyphen each, and nothing is merged or trimmed afterwards. That is what
// in-page links written against those renderers already say.
func TestSlugifyMatchesGitHubRules(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Why Go / goldmark", "why-go--goldmark"},
		{"Ünicode — Dashes!", "ünicode--dashes"},
		{"— Intro", "-intro"},
		{"foo_bar baz", "foo_bar-baz"},
		{"a  b", "a--b"},
		{"a\tb", "a-b"},
		{"a\u00a0b", "a-b"},
		{"x² ½ y", "x²-½-y"},
		{"42 rollback", "42-rollback"},
		{"+ +", "-"},
	} {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHeadingAnchorsLinksNonASCIIHeadings(t *testing.T) {
	got := apply(t, `<h2>日本語の見出し</h2>`, HeadingAnchors())
	if !strings.Contains(got, `id="日本語の見出し"`) {
		t.Errorf("non-ASCII heading got no id: %s", got)
	}
	if !strings.Contains(got, `href="#日本語の見出し"`) {
		t.Errorf("non-ASCII heading got no anchor: %s", got)
	}
}

// A heading with no letters or digits at all has nothing to slugify, so it
// falls back to its position. Positional ids are unstable across edits, which
// is why they are a last resort rather than the general rule.
func TestHeadingAnchorsFallsBackForHeadingsWithNoLetters(t *testing.T) {
	got := apply(t, `<h2>Real</h2><h2>+++</h2>`, HeadingAnchors())
	if !strings.Contains(got, `id="real"`) {
		t.Errorf("lost the normal slug: %s", got)
	}
	if !strings.Contains(got, `id="section-2"`) {
		t.Errorf("symbol-only heading got no fallback id: %s", got)
	}
}

func TestExternalLinksMarksOffSiteOnly(t *testing.T) {
	in := `<a href="https://example.com">e</a><a href="./local.html">l</a><a href="#frag">f</a>`
	got := apply(t, in, ExternalLinks())
	if !strings.Contains(got, `href="https://example.com" target="_blank" rel="noopener noreferrer"`) {
		t.Errorf("external link not marked\ngot: %s", got)
	}
	if strings.Count(got, `target="_blank"`) != 1 {
		t.Errorf("marked a non-external link\ngot: %s", got)
	}
}

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
