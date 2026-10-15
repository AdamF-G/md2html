package md2html

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/AdamF-G/md2html/internal/highlight"
)

// langName is the shape of a language name or alias: real fence tags
// include c++, c# and objective-c. Fences match it ignoring case.
var langName = regexp.MustCompile(`^[a-z][a-z0-9+#.-]*$`)

// Grammar is one language a plugin provides: a TextMate grammar file, the
// names a fence can use for it, and where it came from.
type Grammar struct {
	Name    string
	Aliases []string
	// License is an SPDX identifier and Source is where the grammar came
	// from, both shown by --list-languages. Built-in grammars always have
	// both.
	License, Source string
	// Scope is the grammar's scopeName, such as source.go.
	Scope string

	plugin *Plugin
	file   string
}

// PluginName is the name of the plugin the grammar comes from.
func (g *Grammar) PluginName() string { return g.plugin.Name }

// GrammarNames returns the plugin's language names in sorted order.
func (p *Plugin) GrammarNames() []string { return sortedJSONKeys(p.Grammars) }

// raw reads the grammar file, decompressing a .gz.
func (g *Grammar) raw() ([]byte, error) {
	b, err := readContained(g.plugin.dir, g.plugin.fsys, g.file)
	if err != nil || !strings.HasSuffix(g.file, ".gz") {
		return b, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %s: %w", g.plugin.Name, g.file, err)
	}
	return io.ReadAll(zr)
}

// parseGrammars reads a manifest's grammars block into p. A built-in must
// give each grammar's scope, licence and source, so nothing is read until a
// page needs it; a local plugin's files are read and checked at load, in
// loadDir.
func parseGrammars(p *Plugin, grammars map[string]map[string]json.RawMessage, where string, builtin bool, warn func(string)) error {
	fail := func(format string, a ...any) error {
		return fmt.Errorf("plugin %s: "+format, append([]any{where}, a...)...)
	}
	for _, name := range sortedJSONKeys(grammars) {
		raw := grammars[name]
		if !langName.MatchString(name) {
			return fail("language name %q is not lowercase letters, digits, -, +, # and . starting with a letter", name)
		}
		g := &Grammar{Name: name, plugin: p}
		for _, f := range []struct {
			key string
			dst any
		}{{"file", &g.file}, {"aliases", &g.Aliases}, {"license", &g.License}, {"source", &g.Source}, {"scope", &g.Scope}} {
			if _, err := take(raw, f.key, f.dst); err != nil {
				return fail("grammar %q: %s: %v", name, f.key, err)
			}
		}
		if g.file == "" {
			return fail("grammar %q has no file", name)
		}
		for _, a := range g.Aliases {
			if !langName.MatchString(a) {
				return fail("grammar %q: alias %q is not lowercase letters, digits, -, +, # and . starting with a letter", name, a)
			}
		}
		if builtin && (g.Scope == "" || g.License == "" || g.Source == "") {
			return fail("built-in grammar %q needs scope, license and source", name)
		}
		for _, k := range sortedJSONKeys(raw) {
			if warn != nil {
				warn(fmt.Sprintf("plugin %s: unknown key %q in grammar %q; ignoring it", where, k, name))
			}
		}
		p.Grammars[name] = g
	}
	return nil
}

// checkGrammars reads each of a local plugin's grammar files, so a broken
// one is an error when the plugin loads, as a missing CSS file is.
func (p *Plugin) checkGrammars() error {
	for _, name := range p.GrammarNames() {
		g := p.Grammars[name]
		data, err := g.raw()
		if err != nil {
			return fmt.Errorf("plugin %s: grammar %q: %w", p.Name, name, err)
		}
		scope, err := highlight.CheckGrammar(data)
		if err != nil {
			return fmt.Errorf("plugin %s: grammar %q: %s %v", p.Name, name, g.file, err)
		}
		if g.Scope != "" && g.Scope != scope {
			return fmt.Errorf("plugin %s: grammar %q: scope %q, but %s says %q", p.Name, name, g.Scope, g.file, scope)
		}
		g.Scope = scope
	}
	return nil
}

// sortedGrammars returns gs sorted by name; listings use it.
func sortedGrammars(gs []*Grammar) []*Grammar {
	sort.Slice(gs, func(i, j int) bool { return gs[i].Name < gs[j].Name })
	return gs
}
