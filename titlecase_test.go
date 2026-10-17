package md2html

import "testing"

func TestTitleCase(t *testing.T) {
	fixed := titleCaseWords("API, CLI ,SONiC")
	for in, want := range map[string]string{
		"my_dir":        "My Dir",
		"my-dir":        "My Dir",
		"design notes":  "Design Notes",
		"api-reference": "API Reference",
		"Sonic":         "SONiC",
		"cli_tools":     "CLI Tools",
		"x":             "X",
		"--":            "",
		"already Upper": "Already Upper",
	} {
		if got := titleCase(in, fixed); got != want {
			t.Errorf("titleCase(%q) = %q, want %q", in, got, want)
		}
	}
	if got := titleCase("api", nil); got != "Api" {
		t.Errorf("without a list: %q", got)
	}
	if len(titleCaseWords("")) != 0 {
		t.Error("an empty title-case: gives words")
	}
}
