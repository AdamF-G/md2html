package md2html

import "testing"

// TestInSite checks that a link is read as a site path against the pages a
// site holds, and that it answers nothing without mounts.
func TestInSite(t *testing.T) {
	docs := []Doc{
		{SitePath: "index.md", Out: "/out/index.html"},
		{SitePath: "guides/deploy.md", Out: "/out/guides/deploy.html"},
		{SitePath: "guides/my page.md", Out: "/out/guides/my page.html"},
		{SitePath: "reference/index.md", Out: "/out/reference/index.html"},
		{SitePath: "reference/engine.md", Out: "/out/reference/engine.html"},
		{SitePath: "notes/README.md", Out: "/out/notes/index.html"},
		{SitePath: "both/index.md", Out: "/out/both/index.html"},
		{SitePath: "both/README.md", Out: "/out/both/README.html"},
		{SitePath: "bare/page.md", Out: "/out/bare/page.html"},
	}
	r := &siteRules{outDir: "/out", mounts: []mountRule{{}}}
	r.indexPages(docs)

	cases := []struct {
		href, from string
		out, at    string
		ok         bool
	}{
		{"../reference/engine.md#options", "guides", "/out/reference/engine.html", "reference/engine", true},
		{"../reference/index.md", "guides", "/out/reference/index.html", "reference/index", true},
		{"../reference/", "guides", "/out/reference/index.html", "reference/", true},
		{"../reference", "guides", "/out/reference/index.html", "reference/", true},
		{"../notes/README.md", "guides", "/out/notes/index.html", "notes/README", true},
		{"../notes/index.md", "guides", "/out/notes/index.html", "notes/", true},
		{"../both/README.md", "guides", "/out/both/README.html", "both/README", true},
		{"../bare/index.md", "guides", "/out/bare/index.html", "bare/", true},
		{"../bare/README.md", "guides", "", "", false},
		{"../../x.md", "guides", "", "", false},
		{"/reference/engine.md", "guides", "", "", false},
		{"../reference/missing.md", "guides", "", "", false},
		{"my%20page.md", "guides", "/out/guides/my page.html", "guides/my page", true},
		{"reference/engine.md", "", "/out/reference/engine.html", "reference/engine", true},
		{"../", "guides", "/out/index.html", "the site's root", true},
	}
	for _, c := range cases {
		out, at, ok := r.inSite(Link{Href: c.href}, c.from)
		if out != c.out || at != c.at || ok != c.ok {
			t.Errorf("inSite(%q from %q) = %q, %q, %v; want %q, %q, %v",
				c.href, c.from, out, at, ok, c.out, c.at, c.ok)
		}
	}

	if _, _, ok := r.inSite(Link{Href: "../reference/engine.md", Loaded: true}, "guides"); ok {
		t.Error("a loaded asset was resolved as a site path")
	}

	r.mounts = nil
	for _, c := range cases {
		if _, _, ok := r.inSite(Link{Href: c.href}, c.from); ok {
			t.Errorf("inSite(%q) answered without mounts", c.href)
		}
	}
	var none *siteRules
	if _, _, ok := none.inSite(Link{Href: "a.md"}, ""); ok {
		t.Error("a nil siteRules answered")
	}
}
