package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mountTree writes a repository whose shell, at repo/docs/site/shell.md,
// holds body after its front matter.
func mountTree(t *testing.T, body string, extra map[string]string) string {
	t.Helper()
	files := map[string]string{
		"repo/docs/site/shell.md": "---\nsite: true\n---\n" + body + "\n\n[[content]]\n",
		"repo/docs/index.md":      "# Home\n",
		"repo/docs/guide/a.md":    "# A\n",
		"repo/README.md":          "# Readme\n",
		"repo/notes/n.md":         "# N\n",
		"repo/LICENSE":            "MIT",
	}
	for k, v := range extra {
		files[k] = v
	}
	return writeTree(t, files)
}

func TestReadMountsFromTheShellAndFlags(t *testing.T) {
	root := mountTree(t, "[[mount source=\"../index.md\"]]\n\n"+
		"[[mount path=\"guide\" source=\"../guide\" nav-order=\"2\" nav-sort=\"links\" exclude=\"drafts, old*\" toc=\"shell\" title=\"The Guide\"]]\n\n"+
		"[[mount path=\"overview.md\" source=\"../../README.md\" nav-title=\"Overview\"]]\n\n"+
		"```markdown\n[[mount path=\"x\" source=\"nowhere\"]]\n```\n", nil)
	t.Chdir(filepath.Join(root, "repo"))
	ms, err := ReadMounts(filepath.Join(root, "repo/docs/site/shell.md"), []string{`path="team" source="notes"`})
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 4 {
		t.Fatalf("got %d mounts: %+v", len(ms), ms)
	}
	r, g, o, f := ms[0], ms[1], ms[2], ms[3]
	if r.Path != "" || r.IsDir || r.Source != filepath.Join(root, "repo/docs/index.md") {
		t.Errorf("root %+v", r)
	}
	if g.Path != "guide" || !g.IsDir || g.Source != filepath.Join(root, "repo/docs/guide") ||
		!g.HasNavOrder || g.NavOrder != 2 || g.NavSort != "links" || g.TOC != "shell" || g.Title != "The Guide" ||
		strings.Join(g.Exclude, "|") != "drafts|old*" {
		t.Errorf("guide %+v", g)
	}
	if o.Path != "overview" || o.NavTitle != "Overview" || o.Source != filepath.Join(root, "repo/README.md") {
		t.Errorf("overview %+v", o)
	}
	if f.Path != "team" || f.Source != filepath.Join(root, "repo/notes") || f.From != "--mount 1" {
		t.Errorf("flag %+v", f)
	}
	if !strings.Contains(g.From, "shell.md") || !strings.Contains(g.From, "[[mount]] 2") {
		t.Errorf("From %q", g.From)
	}
}

func TestReadMountsWithoutAny(t *testing.T) {
	root := mountTree(t, "No mounts here.", nil)
	ms, err := ReadMounts(filepath.Join(root, "repo/docs/site/shell.md"), nil)
	if ms != nil || err != nil {
		t.Errorf("got %+v, %v", ms, err)
	}
}

// TestReadMountsSharedDirectoryConflicts checks that a setting the first of
// several mounts at one directory path leaves empty still conflicts between
// the later two.
func TestReadMountsSharedDirectoryConflicts(t *testing.T) {
	for _, c := range []struct{ a, b, c, want string }{
		{a: ``, b: `nav-title="G"`, c: `nav-title="H"`, want: `different nav-title: "G" and "H"`},
		{a: ``, b: `nav-order="1"`, c: `nav-order="2"`, want: `different nav-order: 1 and 2`},
	} {
		root := mountTree(t, "", map[string]string{"repo/extra/e.md": "# E\n"})
		t.Chdir(filepath.Join(root, "repo"))
		body := "[[mount source=\"../index.md\"]]\n\n" +
			"[[mount path=\"g\" source=\"../guide\" " + c.a + "]]\n\n" +
			"[[mount path=\"g\" source=\"../../notes\" " + c.b + "]]\n\n" +
			"[[mount path=\"g\" source=\"../../extra\" " + c.c + "]]"
		shell := filepath.Join(root, "repo/docs/site/shell.md")
		if err := os.WriteFile(shell, []byte("---\nsite: true\n---\n"+body+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := ReadMounts(shell, nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q %q %q: err %v, want one containing %q", c.a, c.b, c.c, err, c.want)
		}
	}
}

func TestReadMountsRefuses(t *testing.T) {
	const r = "[[mount source=\"../index.md\"]]\n\n"
	for _, c := range []struct{ body, flag, want string }{
		{body: `[[mount path="x"]]`, want: "needs a source"},
		{body: r + `[[mount path="x" source="../guide" colour="red"]]`, want: `unknown attribute "colour"`},
		{body: r + `[[mount path="x" source="../nowhere"]]`, want: "does not exist"},
		{body: r + `[[mount path="x" source="../../LICENSE"]]`, want: "is not a directory or a Markdown file"},
		{body: r + `[[mount path="/x" source="../guide"]]`, want: "is not a site path"},
		{body: r + `[[mount path="x/" source="../guide"]]`, want: "is not a site path"},
		{body: r + `[[mount path="a/../b" source="../guide"]]`, want: "is not a site path"},
		{body: r + `[[mount path="../b" source="../guide"]]`, want: "is not a site path"},
		{body: r + `[[mount path="x" source="../guide" nav-order="two"]]`, want: `nav-order "two" is not a whole number`},
		{body: r + `[[mount path="x" source="../guide" nav-sort="up"]]`, want: `nav-sort "up" is not links or label`},
		{body: r + `[[mount path="x" source="../guide" toc="both"]]`, want: `toc "both" is not page or shell`},
		{body: r + `[[mount path="x" source="../../README.md" nav-sort="links"]]`, want: "apply to a directory mount"},
		{body: r + `[[mount path="x" source="../../README.md" exclude="a"]]`, want: "apply to a directory mount"},
		{body: r + `[[mount path="x" source="../../README.md" title="T"]]`, want: "nav-sort, exclude and title apply to a directory mount"},
		{body: r + `[[mount source="../guide"]]`, want: "both mount the site's root"},
		{body: r + `[[mount path="x" source="../guide"]]` + "\n\n" + `[[mount path="y" source="../guide"]]`, want: "both mount"},
		{body: r + `[[mount path="g" source="../guide" nav-title="G"]]` + "\n\n" + `[[mount path="g" source="../../notes" nav-title="H"]]`, want: `different nav-title: "G" and "H"`},
		{body: `[[mount path="x" source="../guide"]]`, want: "no mount is the site's root"},
		{body: r, flag: `source=notes`, want: "is not key=\"value\" attributes"},
	} {
		root := mountTree(t, c.body, nil)
		t.Chdir(filepath.Join(root, "repo"))
		var flags []string
		if c.flag != "" {
			flags = []string{c.flag}
		}
		_, err := ReadMounts(filepath.Join(root, "repo/docs/site/shell.md"), flags)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q %q: err %v, want one containing %q", c.body, c.flag, err, c.want)
		}
	}
}
