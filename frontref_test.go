package md2html

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRefs(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"versions.md": "---\nall-versions: latest, v0.2, v0.1\nchained: @other.md#x\n---\n# Versions\n",
	})
	var warnings []string
	got := resolveRefs(map[string]string{
		"versions": "@versions.md#all-versions",
		"plain":    "unchanged",
		"missing":  "@nope.md#all-versions",
		"nokey":    "@versions.md#absent",
		"chain":    "@versions.md#chained",
		"shape":    "@versions.md",
		"author":   "@someone",
	}, dir, "", []string{"versions", "plain", "missing", "nokey", "chain", "shape", "absent"},
		func(m string) { warnings = append(warnings, m) })

	if got["versions"] != "latest, v0.2, v0.1" || got["plain"] != "unchanged" {
		t.Errorf("resolved = %v", got)
	}
	for _, k := range []string{"missing", "nokey", "chain"} {
		if got[k] != "" {
			t.Errorf("%s = %q, want empty: an unresolved reference must not leak its @ text", k, got[k])
		}
	}
	if got["shape"] != "@versions.md" {
		t.Errorf("shape = %q, want the value as written", got["shape"])
	}
	all := strings.Join(warnings, "\n")
	// author is not among the keys asked for: nothing reads it as data, so
	// it is not a reference, and says nothing.
	if got["author"] != "@someone" || strings.Contains(all, "author") {
		t.Errorf("author = %q, warnings:\n%s; want it untouched and unmentioned", got["author"], all)
	}
	for _, want := range []string{"nope.md", "absent", "itself a reference", "is not @file#key"} {
		if !strings.Contains(all, want) {
			t.Errorf("no warning mentions %q; got:\n%s", want, all)
		}
	}
}

func TestConvertLeavesUnreadReferencesAlone(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"meta.md": "---\nheading: From elsewhere\n---\n",
	})
	var warnings []string
	out, err := Convert([]byte("---\ntitle: \"@meta.md#heading\"\nauthor: \"@someone\"\n---\nbody\n"),
		Options{SourcePath: filepath.Join(dir, "page.md"), Warn: func(m string) { warnings = append(warnings, m) }})
	if err != nil {
		t.Fatal(err)
	}
	// Only a key md2html reads as data is resolved, so a page without
	// markers is what it was before references existed, warnings included.
	if !strings.Contains(string(out), "<title>@meta.md#heading</title>") {
		t.Errorf("an unread reference should be used as written:\n%s", out)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %q, want none", warnings)
	}
}

func TestNavigationMarkersWithoutASiteWarn(t *testing.T) {
	var warnings []string
	out := convert(t, "[[pages]]\n", func(m string) { warnings = append(warnings, m) })
	if !strings.Contains(out, "[[pages]]") {
		t.Errorf("the marker should stay as written without a site:\n%s", out)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "needs a site shell") {
		t.Errorf("warnings = %q", warnings)
	}
}

// With a root, a reference to a file outside it is empty and warns, as an
// unreadable one does; without one, it is read as before.
func TestResolveRefsStaysInsideARoot(t *testing.T) {
	root := writeTree(t, map[string]string{
		"site/a.md":    "# A\n",
		"site/vals.md": "---\nv: in\n---\n",
		"outside.md":   "---\nv: out\n---\n",
	})
	dir := filepath.Join(root, "site")
	meta := map[string]string{"x": "@vals.md#v", "y": "@../outside.md#v"}
	var warned []string
	got := resolveRefs(meta, dir, dir, []string{"x", "y"}, func(m string) { warned = append(warned, m) })
	if got["x"] != "in" || got["y"] != "" || len(warned) != 1 || !strings.Contains(warned[0], "outside the site's source root") {
		t.Errorf("bounded: %v %q", got, warned)
	}
	if got := resolveRefs(meta, dir, "", []string{"y"}, nil); got["y"] != "out" {
		t.Errorf("unbounded: %v", got)
	}
}
