package md2html

import (
	"strings"
	"testing"

	"github.com/AdamF-G/md2html/internal/highlight"
)

// Every built-in grammar decompresses, parses, and declares the scope its
// manifest entry says, so a broken one cannot ship.
func TestBuiltinGrammarsAreValid(t *testing.T) {
	p := findPlugin(BuiltinPlugins(), "grammars")
	if p == nil {
		t.Fatal("no built-in grammars plugin")
	}
	if n := len(p.Grammars); n < 190 {
		t.Errorf("%d built-in grammars, want about 200", n)
	}
	for _, name := range p.GrammarNames() {
		g := p.Grammars[name]
		data, err := g.raw()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		scope, err := highlight.CheckGrammar(data)
		if err != nil || scope != g.Scope {
			t.Errorf("%s: scope %q, manifest says %q (%v)", name, scope, g.Scope, err)
		}
	}
}

// The aliases the guide promises reach their grammars.
func TestBuiltinAliases(t *testing.T) {
	ls := NewPluginCache().languages(nil, nil)
	for alias, want := range map[string]string{"sh": "shellscript", "bash": "shellscript", "zsh": "shellscript",
		"yml": "yaml", "py": "python", "go": "go", "json": "json", "diff": "diff", "toml": "toml"} {
		if g := ls.lookup(alias); g == nil || g.Name != want {
			t.Errorf("%s reaches %v, want %s", alias, g, want)
		}
	}
}

// No copyleft or unlicensed grammar ships.
func TestBuiltinGrammarLicences(t *testing.T) {
	allowed := map[string]bool{"MIT": true, "Apache-2.0": true, "BSD-2-Clause": true, "BSD-3-Clause": true,
		"ISC": true, "0BSD": true, "Unlicense": true, "CC0-1.0": true, "LicenseRef-TextMate-Bundle": true}
	for _, g := range findPlugin(BuiltinPlugins(), "grammars").Grammars {
		if !allowed[g.License] {
			t.Errorf("%s ships under %q", g.Name, g.License)
		}
	}
}

// THIRD_PARTY_NOTICES credits every shipped grammar with its own licence
// and source, so a grammar the generator adds cannot ship uncredited.
func TestNoticesCreditEveryBuiltinGrammar(t *testing.T) {
	credited := map[string]bool{}
	for _, section := range strings.Split(ThirdPartyNotices(), "\n== ") {
		head, body, _ := strings.Cut(section, "\n")
		open := strings.LastIndex(head, " grammar (")
		if open < 0 || !strings.HasSuffix(head, ") ==") {
			continue
		}
		licence := head[open+len(" grammar (") : len(head)-len(") ==")]
		for _, line := range strings.Split(body, "\n") {
			if src, ok := strings.CutPrefix(line, "Source: "); ok {
				credited[licence+" "+src] = true
			}
		}
	}
	for _, g := range findPlugin(BuiltinPlugins(), "grammars").Grammars {
		if !credited[g.License+" "+g.Source] {
			t.Errorf("notices do not credit %s under %s from %s", g.Name, g.License, g.Source)
		}
	}
}

// Every grammar the report says was left out is in left-out.txt, so its
// fences stay plain and draw no "did you mean" warning.
func TestLeftOutListMatchesReport(t *testing.T) {
	report, err := builtinFS.ReadFile("plugins/grammars/REPORT.md")
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(report), "\n## Left out")
	if !ok {
		t.Fatal("REPORT.md has no Left out section")
	}
	section, _, _ = strings.Cut(section, "\n## ")
	ls := NewPluginCache().languages(nil, nil)
	n := 0
	for _, line := range strings.Split(section, "\n") {
		name, _, ok := strings.Cut(strings.TrimPrefix(line, "- "), ":")
		if !strings.HasPrefix(line, "- ") || !ok {
			continue
		}
		n++
		if !ls.leftOut[name] {
			t.Errorf("%s is left out in REPORT.md but not in %s", name, leftOutFile)
		}
	}
	if n == 0 {
		t.Error("REPORT.md lists no left-out grammars")
	}
	for name := range ls.leftOut {
		if ls.byName[name] != nil {
			t.Errorf("%s is in %s but a built-in grammar serves it", name, leftOutFile)
		}
	}
}
