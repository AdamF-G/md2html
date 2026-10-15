package highlight

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// A grammar small enough to read: keywords, double-quoted strings that may
// span lines, and line comments.
const toyGrammar = `{
  "scopeName": "source.toy",
  "patterns": [
    {"match": "\\b(func|return)\\b", "name": "keyword.control.toy"},
    {"begin": "\"", "end": "\"", "name": "string.quoted.double.toy",
     "beginCaptures": {"0": {"name": "punctuation.definition.string.begin.toy"}},
     "endCaptures": {"0": {"name": "punctuation.definition.string.end.toy"}}},
    {"match": "//.*$", "name": "comment.line.double-slash.toy"}
  ]
}`

func toyLoader(scope string) ([]byte, error) {
	if scope == "source.toy" {
		return []byte(toyGrammar), nil
	}
	return nil, nil
}

// text returns the slice of line a token covers.
func text(line string, t Token) string { return line[t.Start:t.End] }

func innermost(t Token) string { return t.Scopes[len(t.Scopes)-1] }

func TestTokenizeCarriesStateAcrossLines(t *testing.T) {
	e := NewTextMate(toyLoader, Options{})
	lines := []string{`func f() { return "a`, `b" } // done`}
	got, err := e.Tokenize("source.toy", lines)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d lines, want 2", len(got))
	}
	// The second line opens inside the string the first left open.
	first := got[1][0]
	if text(lines[1], first) != "b" || !strings.HasPrefix(innermost(first), "string.quoted") {
		t.Errorf("line 2 starts with %q %v, want b inside the string", text(lines[1], first), first.Scopes)
	}
	if got[0][0].Scopes[0] != "source.toy" {
		t.Errorf("root scope %q, want source.toy first", got[0][0].Scopes[0])
	}
}

// Offsets are bytes, so a caller can slice the line; textmate-go counts
// runes. A keyword after multi-byte text must still be found exactly.
func TestTokenizeByteOffsets(t *testing.T) {
	e := NewTextMate(toyLoader, Options{})
	line := `"héllo — 世界" return`
	got, err := e.Tokenize("source.toy", []string{line})
	if err != nil {
		t.Fatal(err)
	}
	var joined strings.Builder
	found := false
	for _, tok := range got[0] {
		joined.WriteString(text(line, tok))
		if text(line, tok) == "return" && innermost(tok) == "keyword.control.toy" {
			found = true
		}
	}
	if joined.String() != line {
		t.Errorf("tokens rebuild %q, want %q", joined.String(), line)
	}
	if !found {
		t.Errorf("no keyword token for return in %+v", got[0])
	}
}

func TestTokenizeLongLineKeepsItsHead(t *testing.T) {
	e := NewTextMate(toyLoader, Options{MaxLineRunes: 10})
	line := "return xx" + strings.Repeat("é", 20) + " return"
	got, err := e.Tokenize("source.toy", []string{line})
	if err != nil {
		t.Fatal(err)
	}
	toks := got[0]
	if text(line, toks[0]) != "return" {
		t.Errorf("first token %q, want the head still tokenized", text(line, toks[0]))
	}
	last := toks[len(toks)-1]
	if last.End > len(string([]rune(line)[:10])) {
		t.Errorf("a token ends at byte %d, past the first 10 runes", last.End)
	}
}

func TestTokenizeOverBudget(t *testing.T) {
	e := NewTextMate(toyLoader, Options{Budget: time.Nanosecond})
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = `func f() { return "x" } // y`
	}
	if _, err := e.Tokenize("source.toy", lines); !errors.Is(err, ErrBudget) {
		t.Errorf("err %v, want ErrBudget", err)
	}
}

func TestTokenizeUnknownScope(t *testing.T) {
	e := NewTextMate(toyLoader, Options{})
	if _, err := e.Tokenize("source.nothing", []string{"x"}); err == nil {
		t.Error("no error for a scope the loader does not have")
	}
}

// One engine serves every page of a run at once.
func TestTokenizeConcurrently(t *testing.T) {
	e := NewTextMate(toyLoader, Options{})
	lines := make([]string, 400)
	for i := range lines {
		lines[i] = `func f() { return "x" } // y`
	}
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := e.Tokenize("source.toy", lines)
			if err == nil && len(got) != len(lines) {
				err = errors.New("short result")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestCheckGrammar(t *testing.T) {
	if s, err := CheckGrammar([]byte(toyGrammar)); err != nil || s != "source.toy" {
		t.Errorf("CheckGrammar = %q, %v", s, err)
	}
	for name, bad := range map[string]string{
		"not JSON":    `{`,
		"no scope":    `{"patterns": [{"match": "x"}]}`,
		"no patterns": `{"scopeName": "source.x"}`,
	} {
		if _, err := CheckGrammar([]byte(bad)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// An Oniguruma pattern the engine cannot translate is reported, not fatal.
func TestDiagnosticsAfterUse(t *testing.T) {
	const g = `{"scopeName": "source.bad", "patterns": [
	  {"match": "\\p{Xwd}+", "name": "keyword.bad"},
	  {"match": "ok", "name": "keyword.ok"}]}`
	e := NewTextMate(func(string) ([]byte, error) { return []byte(g), nil }, Options{})
	if _, err := e.Tokenize("source.bad", []string{"ok"}); err != nil {
		t.Fatal(err)
	}
	if len(e.Diagnostics("source.bad")) == 0 {
		t.Error("no diagnostics for an untranslatable pattern")
	}
	if d := e.Diagnostics("source.never-loaded"); d != nil {
		t.Errorf("diagnostics %v for a grammar never loaded", d)
	}
}

// The budget is for a call's own work. Calls on one grammar take turns, so
// a call that waited for the others must not have spent its budget waiting:
// each of these fits the budget many times over, all of them together do
// not.
func TestTokenizeBudgetExcludesWaiting(t *testing.T) {
	const budget = 300 * time.Millisecond
	e := NewTextMate(toyLoader, Options{Budget: budget})
	lines := make([]string, 400)
	for i := range lines {
		lines[i] = `func f() { return "x" } // y`
	}
	one := time.Hour
	for range 3 { // the first call also compiles the grammar
		start := time.Now()
		if _, err := e.Tokenize("source.toy", lines); err != nil {
			t.Fatal(err)
		}
		one = min(one, time.Since(start))
	}
	if one > budget/10 {
		t.Skipf("one call took %s, too near the %s budget to tell waiting from work", one, budget)
	}
	n := int(3*budget/one) + 1 // together, three budgets' worth of work
	var wg sync.WaitGroup
	errs := make(chan error, n)
	start := time.Now()
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.Tokenize("source.toy", lines)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	if total := time.Since(start); total < budget {
		t.Errorf("%d calls took %s together, under the %s budget, so waiting was not tested", n, total, budget)
	}
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}
