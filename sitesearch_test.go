package md2html

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var searchFiles = map[string]string{
	"shell/site.md":   "---\nsite: true\nsearch-facets: kind, area\n---\n[[search]]\n\n[[content]]\n",
	"docs/index.md":   "# Home\n\nWelcome to the docs.\n",
	"docs/api/exp.md": "---\nkind: class\narea: experiments\n---\n# Experiment\n\nA plan of runs.\n\n## Fields {#fields}\n\n::: defs\nkey\n: the comparability key\n:::\n",
}

func TestSearchIndexIsSortedAndStable(t *testing.T) {
	s, res, root := siteFixture(t, searchFiles, "shell/site.md")
	// Converted in parallel, as the CLI does. The goroutines call Convert
	// directly and report with t.Error: t.Fatal (inside sitePage) must only
	// run on the test's own goroutine, and go vet says so.
	type job struct {
		src []byte
		opt Options
	}
	var jobs []job
	for _, rel := range []string{"docs/api/exp.md", "docs/index.md"} {
		p := filepath.Join(root, rel)
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var lm map[string]string
		for _, d := range res.Docs {
			if d.Src == p {
				lm = d.LinkMap
			}
		}
		jobs = append(jobs, job{b, Options{SourcePath: p, LinkMap: lm, Site: s, NoSource: true}})
	}
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			if _, err := Convert(j.src, j.opt); err != nil {
				t.Error(err)
			}
		}(j)
	}
	wg.Wait()
	idx := s.SearchIndex()
	if !strings.HasPrefix(string(idx), "// "+MarkerPrefix) {
		t.Fatalf("the index must start with the provenance marker:\n%s", idx)
	}
	body := strings.TrimSuffix(strings.TrimPrefix(string(idx)[strings.Index(string(idx), "\n")+1:], "window.md2htmlSearch = "), ";\n")
	var entries []struct {
		URL      string                      `json:"url"`
		Title    string                      `json:"title"`
		Headings []struct{ Text, ID string } `json:"headings"`
		Terms    []string                    `json:"terms"`
		Text     string                      `json:"text"`
		Facets   map[string]string           `json:"facets"`
	}
	if err := json.Unmarshal([]byte(body), &entries); err != nil {
		t.Fatalf("index is not JSON after the prefix: %v\n%s", err, body)
	}
	if len(entries) != 2 || entries[0].URL != "api/exp.html" || entries[1].URL != "index.html" {
		t.Fatalf("entries = %+v, want api/exp.html then index.html", entries)
	}
	e := entries[0]
	if e.Title != "Experiment" || e.Text != "A plan of runs." || e.Facets["kind"] != "class" ||
		e.Facets["area"] != "experiments" || len(e.Terms) != 1 || e.Terms[0] != "key" {
		t.Errorf("entry = %+v", e)
	}
	if len(e.Headings) == 0 || e.Headings[len(e.Headings)-1].ID != "fields" {
		t.Errorf("headings = %+v", e.Headings)
	}
	// Converting again in the other order gives the same bytes.
	sitePage(t, s, res, root, "docs/index.md")
	sitePage(t, s, res, root, "docs/api/exp.md")
	if again := s.SearchIndex(); string(again) != string(idx) {
		t.Error("the index changed when the same pages were converted again")
	}
}

func TestSearchBoxPointsAtTheIndex(t *testing.T) {
	s, res, root := siteFixture(t, searchFiles, "shell/site.md")
	out, _ := sitePage(t, s, res, root, "docs/api/exp.md")
	if !strings.Contains(out, `<div class="md2html-search" hidden="" data-index="../md2html-search.js">`) {
		t.Errorf("search box wrong:\n%s", out)
	}
	if !strings.Contains(out, "window.md2htmlSearch") {
		t.Error("the page has a search box but no search runtime")
	}
	if got, want := s.IndexPath(), filepath.Join(root, "site", "md2html-search.js"); got != want {
		t.Errorf("IndexPath = %s, want %s", got, want)
	}
}

func TestSearchIndexIsOursToOverwrite(t *testing.T) {
	s, _, root := siteFixture(t, searchFiles, "shell/site.md")
	p := filepath.Join(root, "idx.js")
	if err := os.WriteFile(p, s.SearchIndex(), 0o644); err != nil {
		t.Fatal(err)
	}
	if ours, err := IsOurs(p); err != nil || !ours {
		t.Errorf("IsOurs = %v, %v; SafeWrite would refuse to replace the index", ours, err)
	}
}

// An index URL is a link, so a name holding '#' or a space is encoded.
func TestSearchIndexURLsAreEncoded(t *testing.T) {
	s, res, root := siteFixture(t, map[string]string{
		"shell/site.md": minimalShell,
		"docs/index.md": "# Home\n",
		"docs/c#.md":    "# C sharp\n",
	}, "shell/site.md")
	sitePage(t, s, res, root, "docs/c#.md")
	if idx := string(s.SearchIndex()); !strings.Contains(idx, `"url":"c%23.html"`) {
		t.Errorf("want the url encoded:\n%s", idx)
	}
}
