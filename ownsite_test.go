package md2html

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/net/html"
)

// ownSite lays out this repository's documentation site the way `just
// site` does, from its shell's mounts, so a test can hold the real pages
// to the site's rules.
func ownSite(t *testing.T) *Site {
	t.Helper()
	shell := filepath.Join("docs", "site", "shell.md")
	ms, err := ReadMounts(shell, nil)
	if err != nil {
		t.Fatalf("ReadMounts: %v", err)
	}
	opt := CrawlOptions{Mounts: ms, SiteShell: shell, OutDir: t.TempDir()}
	res := mustCrawl(t, opt)
	s, err := BuildSite(res, shell, opt)
	if err != nil {
		t.Fatalf("BuildSite: %v", err)
	}
	return s
}

// A hand-written list of pages, like the home page's, names them in the
// order the navigation does, so a reader meets the sections in one order
// wherever they look. A list counts when each item links to one site page
// and nothing else; an item linking twice is prose, in prose's order.
func TestOwnSiteListsPagesInNavOrder(t *testing.T) {
	s := ownSite(t)
	rank := map[*siteNode]int{}
	for i, n := range s.order {
		rank[n] = i
	}
	for _, page := range s.order {
		raw, err := os.ReadFile(page.doc.Src)
		if err != nil {
			t.Fatal(err)
		}
		_, body, _ := splitFrontMatter(raw)
		root, err := parseDoc(body)
		if err != nil {
			t.Fatal(err)
		}
		walk(root, func(list *html.Node) {
			if list.Type != html.ElementNode || (list.Data != "ul" && list.Data != "ol") {
				return
			}
			var pages []*siteNode
			for li := list.FirstChild; li != nil; li = li.NextSibling {
				if li.Type != html.ElementNode {
					continue
				}
				links := ExtractLinks(li, filepath.Dir(page.doc.Src))
				if len(links) != 1 || links[0].Kind != LinkDoc {
					return
				}
				abs, err := resolve(links[0].Abs)
				n := s.bySrc[abs]
				if _, listed := rank[n]; err != nil || !listed {
					return
				}
				pages = append(pages, n)
			}
			for i := 1; i < len(pages); i++ {
				if rank[pages[i]] < rank[pages[i-1]] {
					t.Errorf("%s lists %q before %q; the navigation has them the other way round",
						page.doc.Src, pages[i-1].label, pages[i].label)
				}
			}
		})
	}
}
