package md2html

import (
	"strings"
	"testing"
)

func markersIn(t *testing.T, src string) []marker {
	t.Helper()
	root, err := parseDoc([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return findMarkers(root)
}

func TestFindMarkersReadsNamesAndAttributes(t *testing.T) {
	ms := markersIn(t, "[[pages depth=\"1\" from=\"section\"]]\n\n[[content]]\n\n[[toc]]\n")
	if len(ms) != 2 {
		t.Fatalf("found %d markers, want 2 ([[toc]] is not one of these)", len(ms))
	}
	if ms[0].name != "pages" || ms[0].attrs["depth"] != "1" || ms[0].attrs["from"] != "section" {
		t.Errorf("first marker = %q %v", ms[0].name, ms[0].attrs)
	}
	if ms[1].name != "content" || len(ms[1].attrs) != 0 {
		t.Errorf("second marker = %q %v", ms[1].name, ms[1].attrs)
	}
}

func TestFindMarkersSeesThroughAutolinks(t *testing.T) {
	ms := markersIn(t, "[[list key=\"v\" link=\"https://docs.example.com/{item}/\"]]\n")
	if len(ms) != 1 {
		t.Fatalf("found %d markers, want 1: a URL GFM autolinks is still part of the marker", len(ms))
	}
	if got := ms[0].attrs["link"]; got != "https://docs.example.com/{item}/" {
		t.Errorf("link = %q", got)
	}
}

func TestFindMarkersIgnoresProseAndCode(t *testing.T) {
	for _, src := range []string{
		"See [[pages]] below.\n", // inside a sentence
		"`[[pages]]`\n",          // a code span
		"```\n[[pages]]\n```\n",  // a fence
		"[[Pages]]\n",            // names are lowercase
		"[[pages depth=1]]\n",    // values must be quoted
	} {
		if ms := markersIn(t, src); len(ms) != 0 {
			t.Errorf("%q: found a marker %q, want none", strings.TrimSpace(src), ms[0].name)
		}
	}
}
