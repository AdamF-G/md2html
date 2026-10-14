package md2html

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// searchEntry is one page in the search index.
type searchEntry struct {
	URL      string            `json:"url"`
	Title    string            `json:"title"`
	Headings []searchHeading   `json:"headings,omitempty"`
	Terms    []string          `json:"terms,omitempty"`
	Text     string            `json:"text,omitempty"`
	Facets   map[string]string `json:"facets,omitempty"`
}

type searchHeading struct {
	Text string `json:"text"`
	ID   string `json:"id"`
}

// searchTextLimit bounds the first paragraph kept per page, in bytes.
const searchTextLimit = 300

// IndexName is the search index's file name, at the site's output root.
const IndexName = "md2html-search.js"

// record adds the page at src to the search index, replacing any entry it
// already has. root is the page's own tree, before the shell wraps it.
func (s *Site) record(src, title string, meta map[string]string, root *html.Node) {
	rel, err := filepath.Rel(s.outRoot, s.outBySrc[src])
	if err != nil {
		return
	}
	e := searchEntry{URL: encodePath(filepath.ToSlash(rel)), Title: title}
	for _, h := range headingNodes(root) {
		if id, ok := attr(h, "id"); ok && id != "" {
			if t := strings.TrimSpace(headingText(h)); t != "" {
				e.Headings = append(e.Headings, searchHeading{Text: t, ID: id})
			}
		}
	}
	walk(root, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		switch {
		case n.DataAtom == atom.Dt:
			if t := strings.TrimSpace(textOf(n)); t != "" {
				e.Terms = append(e.Terms, t)
			}
		case n.DataAtom == atom.P && e.Text == "" && !hasClass(n, "subtitle") && !hasClass(n, "docdate") && !hasClass(n, "eyebrow"):
			t := strings.Join(strings.Fields(textOf(n)), " ")
			if len(t) > searchTextLimit {
				t = t[:searchTextLimit]
				for !utf8.ValidString(t) {
					t = t[:len(t)-1]
				}
			}
			e.Text = t
		}
	})
	for _, f := range s.facets {
		if v := meta[f]; v != "" {
			if e.Facets == nil {
				e.Facets = map[string]string{}
			}
			e.Facets[f] = v
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if i, ok := s.entryAt[e.URL]; ok {
		s.entries[i] = e
		return
	}
	if s.entryAt == nil {
		s.entryAt = map[string]int{}
	}
	s.entryAt[e.URL] = len(s.entries)
	s.entries = append(s.entries, e)
}

// IndexPath is where the search index belongs: the site's output root.
func (s *Site) IndexPath() string { return filepath.Join(s.outRoot, IndexName) }

// SearchIndex is the index as a script that sets window.md2htmlSearch.
// A script, not a data file, so a page opened from disk can load it: a
// browser blocks fetching a file:// URL but not a script tag. It starts
// with the provenance marker, so SafeWrite will replace it next run.
func (s *Site) SearchIndex() []byte {
	s.mu.Lock()
	entries := append([]searchEntry(nil), s.entries...)
	s.mu.Unlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].URL < entries[j].URL })
	if entries == nil {
		entries = []searchEntry{}
	}
	data, _ := json.Marshal(entries)
	// The marker is an HTML comment; here it is a line comment, with the
	// same stable prefix IsOurs looks for.
	return []byte("// " + Marker() + "\nwindow.md2htmlSearch = " + string(data) + ";\n")
}

// searchBox is what [[search]] expands to. It stays hidden until the
// runtime has the index, so a page without scripts shows no dead box.
func (s *Site) searchBox(from string) *html.Node {
	div := elem("div", "class", "md2html-search", "hidden", "", "data-index", s.href(from, s.IndexPath()))
	div.AppendChild(elem("input", "type", "search", "class", "md2html-search-input",
		"placeholder", "Search", "aria-label", "Search this site", "autocomplete", "off"))
	div.AppendChild(elem("ol", "class", "md2html-search-results"))
	return div
}
