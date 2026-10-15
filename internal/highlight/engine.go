// Package highlight tokenizes code with TextMate grammars and sorts the
// tokens into a small, fixed set of kinds. It is the only code in md2html
// that names the engine, so swapping engines means a second Engine and
// nothing else.
package highlight

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
	"unicode/utf8"

	textmate "github.com/eugenioenko/textmate-go"
)

// DefaultBudget is how long one fence may take; DefaultMaxLineRunes is how
// much of one line is tokenized, VS Code's default.
const (
	DefaultBudget       = 2 * time.Second
	DefaultMaxLineRunes = 20_000
)

// ErrBudget is returned when a call runs past its time budget.
var ErrBudget = errors.New("over its time budget")

// Token is one run of a line. Start and End are byte offsets in the line;
// Scopes are root first and must not be modified.
type Token struct {
	Start, End int
	Scopes     []string
}

// Engine tokenizes lines with a grammar named by its scope.
type Engine interface {
	// Tokenize returns one token list per line, carrying the rule
	// stack across lines. Its time budget counts only the call's own
	// work, not time spent waiting for other calls on the same grammar.
	Tokenize(scopeName string, lines []string) ([][]Token, error)
	// Diagnostics lists the patterns of a loaded grammar that could not
	// be used, each as "pattern: reason". It is nil for a grammar not yet
	// loaded, and grows as more of the grammar is exercised.
	Diagnostics(scopeName string) []string
}

// Options bounds the work of one Tokenize call. Zero fields take the
// defaults.
type Options struct {
	Budget       time.Duration
	MaxLineRunes int
}

// TextMate is the textmate-go Engine. It is safe for concurrent use: the
// registry locks, and calls on one grammar take turns.
type TextMate struct {
	reg *textmate.Registry
	opt Options

	mu     sync.Mutex
	loaded map[string]*textmate.Grammar
	turns  map[string]*sync.Mutex // one per root scope, held for a whole call
}

// NewTextMate returns an engine that reads grammars through load, which
// returns nil, nil for a scope it does not have. Grammars are read and
// compiled on first use and kept.
func NewTextMate(load func(scopeName string) ([]byte, error), opt Options) *TextMate {
	if opt.Budget <= 0 {
		opt.Budget = DefaultBudget
	}
	if opt.MaxLineRunes <= 0 {
		opt.MaxLineRunes = DefaultMaxLineRunes
	}
	reg := textmate.NewRegistry(textmate.RegistryOptions{
		LoadGrammar: func(scope string) (*textmate.RawGrammar, error) {
			data, err := load(scope)
			if err != nil || data == nil {
				return nil, err
			}
			return textmate.ParseRawGrammar(data)
		},
	})
	return &TextMate{reg: reg, opt: opt, loaded: map[string]*textmate.Grammar{}, turns: map[string]*sync.Mutex{}}
}

func (e *TextMate) grammar(scope string) (*textmate.Grammar, error) {
	g, err := e.reg.LoadGrammar(scope)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, fmt.Errorf("no grammar for scope %s", scope)
	}
	e.mu.Lock()
	e.loaded[scope] = g
	e.mu.Unlock()
	return g, nil
}

// turn is the lock a call on scope holds while it tokenizes.
func (e *TextMate) turn(scope string) *sync.Mutex {
	e.mu.Lock()
	defer e.mu.Unlock()
	m := e.turns[scope]
	if m == nil {
		m = &sync.Mutex{}
		e.turns[scope] = m
	}
	return m
}

func (e *TextMate) Tokenize(scope string, lines []string) ([][]Token, error) {
	// textmate-go serializes each grammar behind its own lock, and its
	// time limit counts the wait for that lock. With many pages in one
	// language converting at once, every fence's wait would be the sum of
	// the others' work and fences would go plain for nothing. Taking turns
	// here, and starting the clock once it is this call's turn, makes the
	// budget measure this fence alone. The grammar's own lock is then
	// always free, so its wait is nil.
	t := e.turn(scope)
	t.Lock()
	defer t.Unlock()
	g, err := e.grammar(scope)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(e.opt.Budget)
	out := make([][]Token, len(lines))
	var state *textmate.StateStack
	for i, line := range lines {
		left := time.Until(deadline)
		if left <= 0 {
			return nil, ErrBudget
		}
		head := runePrefix(line, e.opt.MaxLineRunes)
		r := g.TokenizeLineWithOptions(head, state, textmate.TokenizeOptions{TimeLimit: left})
		if r.Stopped && r.StoppedReason == textmate.StopReasonTimeLimit {
			return nil, ErrBudget
		}
		out[i] = byteTokens(head, r.Tokens)
		state = r.RuleStack
	}
	return out, nil
}

func (e *TextMate) Diagnostics(scope string) []string {
	e.mu.Lock()
	g := e.loaded[scope]
	e.mu.Unlock()
	if g == nil {
		return nil
	}
	var out []string
	for _, d := range g.Diagnostics() {
		out = append(out, fmt.Sprintf("%s: %s", d.Pattern, d.Message))
	}
	return out
}

// runePrefix is s cut to its first n runes.
func runePrefix(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for range n {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i]
}

// byteTokens converts the engine's rune offsets to byte offsets in line.
func byteTokens(line string, toks []textmate.Token) []Token {
	at := make([]int, 0, len(line)+1) // rune index -> byte offset
	for i := range line {
		at = append(at, i)
	}
	at = append(at, len(line))
	out := make([]Token, len(toks))
	for i, t := range toks {
		out[i] = Token{Start: at[min(t.Start, len(at)-1)], End: at[min(t.End, len(at)-1)], Scopes: t.Scopes}
	}
	return out
}

// CheckGrammar reports a grammar file's scope name, or why it cannot be
// used: not JSON, no scopeName, or no patterns.
func CheckGrammar(data []byte) (string, error) {
	var g struct {
		ScopeName string            `json:"scopeName"`
		Patterns  []json.RawMessage `json:"patterns"`
	}
	if err := json.Unmarshal(data, &g); err != nil {
		return "", fmt.Errorf("not valid JSON: %v", err)
	}
	if g.ScopeName == "" {
		return "", errors.New("has no scopeName")
	}
	if len(g.Patterns) == 0 {
		return "", errors.New("has no patterns")
	}
	return g.ScopeName, nil
}
