package main

import (
	"os"
	"slices"
	"testing"
)

// The pager follows git's rules: unset $PAGER means less, and an empty
// $PAGER or "cat" is a reader asking for none.
func TestPagerCommand(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		set   bool
		want  []string
	}{
		{"unset", "", false, []string{"less"}},
		{"empty", "", true, nil},
		{"blank", "  ", true, nil},
		{"cat", "cat", true, nil},
		{"with args", "less -R", true, []string{"less", "-R"}},
		{"other pager", "most", true, []string{"most"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(string) (string, bool) { return tc.value, tc.set }
			if got := pagerCommand(lookup); !slices.Equal(got, tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A regular file is not a terminal, so redirecting --guide to one gets the
// guide unpaged. $PAGER upper-cases its input, so paging by mistake would
// change the file rather than pass unnoticed.
func TestPageWritesFilesDirectly(t *testing.T) {
	t.Setenv("PAGER", "tr a-z A-Z")
	f, err := os.Create(t.TempDir() + "/out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	page(f, []byte("guide\n"))
	got, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "guide\n" {
		t.Errorf("got %q, want the text unchanged", got)
	}
}
