package md2html

import (
	"strings"
	"testing"
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

func TestHeadingAnchorsDeduplicatesSlugs(t *testing.T) {
	got := apply(t, `<h2>Setup</h2><h2>Setup</h2>`, HeadingAnchors())
	if !strings.Contains(got, `id="setup"`) || !strings.Contains(got, `id="setup-2"`) {
		t.Errorf("duplicate slugs not disambiguated\ngot: %s", got)
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
