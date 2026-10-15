package highlight

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Compare names the first token that differs, by line and scopes, and is
// silent when the dumps agree.
func TestCompareFindsADifference(t *testing.T) {
	d := Dump{Language: "go", Scope: "source.go", Lines: []string{"x := 1"}, Tokens: [][]DumpTok{{
		{Start: 0, End: 1, Scopes: "source.go variable.other.go"},
		{Start: 1, End: 6, Scopes: "source.go"},
	}}}
	if got := Compare(d, d); got != "" {
		t.Errorf("Compare(d, d) = %q, want empty", got)
	}
	e := Dump{Language: d.Language, Scope: d.Scope, Lines: d.Lines, Tokens: [][]DumpTok{{d.Tokens[0][0], d.Tokens[0][1]}}}
	e.Tokens[0][1].Scopes = "source.go keyword.operator.go"
	got := Compare(d, e)
	if !strings.Contains(got, "line 1") || !strings.Contains(got, "keyword.operator.go") {
		t.Errorf("Compare = %q, want it to name line 1 and keyword.operator.go", got)
	}
}

// DumpOf counts columns in UTF-16 code units, as the reference does: an
// astral rune is two units and a two-byte rune is one.
func TestDumpOfCountsUTF16(t *testing.T) {
	at := utf16Offsets("é😀x")
	// Bytes: é is 0-1, 😀 is 2-5, x is 6, end is 7.
	for byteOff, want := range map[int]int{0: 0, 2: 1, 6: 3, 7: 4} {
		if at[byteOff] != want {
			t.Errorf("byte %d maps to %d, want %d", byteOff, at[byteOff], want)
		}
	}
}

// Every shipped grammar tokenizes its sample exactly as VS Code's own
// tokenizer did when the grammars were generated: same tokens, same full
// scope stacks. This is the gate for an engine upgrade or swap.
func TestConformance(t *testing.T) {
	files, _ := filepath.Glob("testdata/conformance/*.json.gz")
	if len(files) == 0 {
		t.Skip("no golden files; run just grammars")
	}
	if len(files) < 150 {
		t.Fatalf("%d golden files; run just grammars", len(files))
	}
	e := shippedEngine(t)
	for _, f := range files {
		var want Dump
		if err := json.Unmarshal(gunzipFile(t, f), &want); err != nil {
			t.Fatal(err)
		}
		t.Run(want.Language, func(t *testing.T) {
			got, err := DumpOf(e, want.Language, want.Scope, want.Lines)
			if err != nil {
				t.Fatal(err)
			}
			if d := Compare(want, got); d != "" {
				t.Error(d)
			}
		})
	}
}

// The conformance check fails when it should: one changed scope in a
// golden is reported.
func TestConformanceCatchesADifference(t *testing.T) {
	files, _ := filepath.Glob("testdata/conformance/*.json.gz")
	if len(files) == 0 {
		t.Skip("no golden files; run just grammars")
	}
	var want Dump
	if err := json.Unmarshal(gunzipFile(t, files[0]), &want); err != nil {
		t.Fatal(err)
	}
	got, err := DumpOf(shippedEngine(t), want.Language, want.Scope, want.Lines)
	if err != nil {
		t.Fatal(err)
	}
	for i, row := range want.Tokens {
		if len(row) == 0 {
			continue
		}
		row[0].Scopes += " invalid.corrupted"
		if d := Compare(want, got); !strings.Contains(d, "invalid.corrupted") {
			t.Errorf("Compare missed a corrupted scope on line %d: %q", i+1, d)
		}
		return
	}
	t.Fatalf("%s has no tokens", files[0])
}

// shippedEngine loads the built-in grammars by their scope names.
func shippedEngine(t *testing.T) *TextMate {
	t.Helper()
	byScope := map[string][]byte{}
	matches, _ := filepath.Glob("../../plugins/grammars/*.json.gz")
	for _, f := range matches {
		raw := gunzipFile(t, f)
		var g struct {
			ScopeName string `json:"scopeName"`
		}
		if err := json.Unmarshal(raw, &g); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		byScope[g.ScopeName] = raw
	}
	// Conformance checks correctness, not speed: the per-fence budget is a
	// run-time guard tested on its own, and a slow machine must not fail here.
	return NewTextMate(func(s string) ([]byte, error) { return byScope[s], nil }, Options{Budget: time.Minute})
}

func gunzipFile(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if _, err := io.Copy(&b, zr); err != nil {
		t.Fatal(err)
	}
	return []byte(b.String())
}
