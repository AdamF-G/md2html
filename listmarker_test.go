package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pageWith converts src as the file name inside dir and returns the page
// and its warnings.
func pageWith(t *testing.T, dir, name, src string) (string, []string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var warnings []string
	out, err := Convert([]byte(src), Options{SourcePath: p, NoSource: true,
		Warn: func(m string) { warnings = append(warnings, m) }})
	if err != nil {
		t.Fatal(err)
	}
	return string(out), warnings
}

func TestListFromFrontMatter(t *testing.T) {
	dir := writeTree(t, map[string]string{})
	out, warnings := pageWith(t, dir, "v.md", "---\nversion: v0.2\nversions: latest, v0.2, v0.1\nbase: /docs/\n---\n"+
		`[[list key="versions" link="{base}{item}/" text="Version {item}" current="{version}"]]`+"\n")
	if len(warnings) != 0 {
		t.Fatalf("warnings: %q", warnings)
	}
	for _, want := range []string{
		`<ul class="md2html-list">`,
		`<li><a href="/docs/latest/">Version latest</a></li>`,
		`<li><a href="/docs/v0.2/" aria-current="true">Version v0.2</a></li>`,
		`<li><a href="/docs/v0.1/">Version v0.1</a></li>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
}

func TestListResolvesTheKeysItReads(t *testing.T) {
	dir := writeTree(t, map[string]string{"versions.md": "---\nall: v0.2, v0.1\nroot: /docs/\n---\n"})
	out, warnings := pageWith(t, dir, "v.md", "---\nversions: \"@versions.md#all\"\nbase: \"@versions.md#root\"\nauthor: \"@someone\"\n---\n"+
		`[[list key="versions" link="{base}{item}/"]]`+"\n")
	// author is read by no marker, so it is not a reference and says nothing.
	if len(warnings) != 0 {
		t.Errorf("warnings: %q", warnings)
	}
	if !strings.Contains(out, `<li><a href="/docs/v0.2/">v0.2</a></li>`) {
		t.Errorf("the referenced list was not used:\n%s", out)
	}
}

func TestListInline(t *testing.T) {
	dir := writeTree(t, map[string]string{})
	out, _ := pageWith(t, dir, "v.md", "---\nlangs: en, de\n---\n"+`[[list key="langs" link="./{item}/" as="inline"]]`+"\n")
	if !strings.Contains(out, `<p class="md2html-list md2html-inline"><a href="./en/">en</a>, <a href="./de/">de</a></p>`) {
		t.Errorf("inline list wrong:\n%s", out)
	}
}

func TestListTemplateUnknownNameWarns(t *testing.T) {
	dir := writeTree(t, map[string]string{})
	out, warnings := pageWith(t, dir, "v.md", "---\nxs: a\n---\n"+`[[list key="xs" link="{nope}{item}"]]`+"\n")
	if !strings.Contains(out, `<a href="a">a</a>`) {
		t.Errorf("an unknown name should expand to nothing:\n%s", out)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "{nope}") {
		t.Errorf("warnings = %q", warnings)
	}
}

func TestSubdirsListsVersionsNewestFirst(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"hosted/v0.9/index.html":  "",
		"hosted/v0.13/index.html": "",
		"hosted/v0.12/index.html": "",
		"hosted/assets/x.css":     "",
		"hosted/.git/HEAD":        "",
		"hosted/notes.txt":        "",
	})
	if err := os.Symlink("v0.13", filepath.Join(dir, "hosted/latest")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("v9.9", filepath.Join(dir, "hosted/broken")); err != nil {
		t.Fatal(err)
	}
	out, warnings := pageWith(t, dir, "v.md", "---\nversion: v0.12\n---\n"+
		`[[subdirs path="hosted" match="v*" link="/{item}/" text="{item}" current="{version}"]]`+"\n")
	want := `<ul class="md2html-list">` +
		`<li><a href="/latest/">latest</a></li>` +
		`<li><a href="/v0.13/">v0.13</a></li>` +
		`<li><a href="/v0.12/" aria-current="true">v0.12</a></li>` +
		`<li><a href="/v0.9/">v0.9</a></li></ul>`
	if !strings.Contains(out, want) {
		t.Errorf("want %s in:\n%s", want, out)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "broken") {
		t.Errorf("want one warning about the dangling symlink, got %q", warnings)
	}
}

func TestSubdirsTargetAndNameSort(t *testing.T) {
	dir := writeTree(t, map[string]string{"h/b/x": "", "h/a/x": ""})
	if err := os.Symlink("b", filepath.Join(dir, "h/stable")); err != nil {
		t.Fatal(err)
	}
	out, _ := pageWith(t, dir, "v.md", `[[subdirs path="h" sort="name" link="/{item}/" text="{item} ({target})"]]`+"\n")
	want := `<li><a href="/stable/">stable (b)</a></li><li><a href="/a/">a ()</a></li><li><a href="/b/">b ()</a></li>`
	if !strings.Contains(out, want) {
		t.Errorf("want %s in:\n%s", want, out)
	}
}

func TestSubdirsWithAnAbsoluteLink(t *testing.T) {
	dir := writeTree(t, map[string]string{"h/v1/x": ""})
	out, warnings := pageWith(t, dir, "v.md", `[[subdirs path="h" link="https://docs.example.com/{item}/"]]`+"\n")
	if len(warnings) != 0 || !strings.Contains(out, `href="https://docs.example.com/v1/"`) {
		t.Errorf("warnings %q, page:\n%s", warnings, out)
	}
}

func TestVersionCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"v0.13", "v0.9", 1}, {"v0.9", "v0.13", -1}, {"v1.0", "v1.0", 0},
		{"v1.0.1", "v1.0", 1}, {"v010", "v9", 1}, {"beta", "alpha", 1},
	} {
		if got := versionCompare(c.a, c.b); got != c.want {
			t.Errorf("versionCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// An unknown template name is one mistake, said once per marker, not once
// per entry.
func TestListUnknownTemplateWarnsOnce(t *testing.T) {
	dir := writeTree(t, map[string]string{})
	_, warnings := pageWith(t, dir, "v.md", "---\nversions: a, b, c\n---\n"+
		`[[list key="versions" link="{nope}{item}/"]]`+"\n")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "{nope}") {
		t.Errorf("warnings = %q, want one about {nope}", warnings)
	}
}
