package md2html

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"github.com/AdamF-G/md2html/internal/highlight"
)

// langSet is the languages one page can highlight: the built-in grammars,
// then each local plugin's, in load order. Names are matched ignoring case;
// scope names decide which file serves a grammar, and every include.
type langSet struct {
	byName  map[string]*Grammar // names and aliases, lower case
	byScope map[string]*Grammar
	engine  highlight.Engine
	// leftOut are the names and aliases of the collection's languages
	// that the built-in grammars plugin does not ship, from its
	// left-out.txt. They are real languages, not typos.
	leftOut map[string]bool
}

type langEntry struct {
	once     sync.Once
	set      *langSet
	warnings []string
}

// languages returns the language set for plugins, with the built-in
// grammars plugin ahead of them, building it once per distinct list. Its
// warnings are replayed to every caller, as a plugin's are.
func (c *PluginCache) languages(plugins []*Plugin, warn func(string)) *langSet {
	list := uniquePlugins(append(builtinGrammarPlugins(), plugins...))
	keys := make([]string, len(list))
	for i, p := range list {
		keys[i] = p.key
	}
	id := strings.Join(keys, "\x00")

	c.mu.Lock()
	e, ok := c.langs[id]
	if !ok {
		e = &langEntry{}
		c.langs[id] = e
	}
	c.mu.Unlock()

	e.once.Do(func() { e.set = buildLangSet(list, func(w string) { e.warnings = append(e.warnings, w) }) })
	if warn != nil {
		for _, w := range e.warnings {
			warn(w)
		}
	}
	return e.set
}

// builtinGrammarPlugins is the built-in grammars plugin, or nothing in a
// build without one.
func builtinGrammarPlugins() []*Plugin {
	if p := findPlugin(BuiltinPlugins(), "grammars"); p != nil {
		return []*Plugin{p}
	}
	return nil
}

// uniquePlugins drops a plugin listed a second time, keeping the first.
func uniquePlugins(ps []*Plugin) []*Plugin {
	seen := map[string]bool{}
	var out []*Plugin
	for _, p := range ps {
		if !seen[p.key] {
			seen[p.key] = true
			out = append(out, p)
		}
	}
	return out
}

// leftOutFile lists the languages the built-in grammars plugin leaves out.
const leftOutFile = "left-out.txt"

func buildLangSet(list []*Plugin, warn func(string)) *langSet {
	ls := &langSet{byName: map[string]*Grammar{}, byScope: map[string]*Grammar{}, leftOut: map[string]bool{}}
	// claim records g under key, and returns the local grammar from another
	// plugin that it displaced, which is worth a warning.
	claim := func(m map[string]*Grammar, key string, g *Grammar) (prev *Grammar) {
		if p, ok := m[key]; ok && p.plugin != g.plugin && p.plugin.dir != "" {
			prev = p
		}
		m[key] = g
		return prev
	}
	for _, p := range list {
		if p.dir == "" {
			// The generator writes this beside the built-in grammars;
			// a build without it just suggests more freely.
			if b, err := fs.ReadFile(p.fsys, leftOutFile); err == nil {
				for _, n := range strings.Fields(string(b)) {
					ls.leftOut[n] = true
				}
			}
		}
		for _, name := range p.GrammarNames() {
			g := p.Grammars[name]
			var clash *Grammar
			for _, n := range append([]string{g.Name}, g.Aliases...) {
				if prev := claim(ls.byName, strings.ToLower(n), g); prev != nil {
					warn(fmt.Sprintf("plugins %s and %s both provide language %q; using %s's", where(prev.plugin), where(g.plugin), n, g.plugin.Name))
					clash = prev
				}
			}
			// A shared scope is reported only when no shared name already
			// was, so one clash is one warning.
			if prev := claim(ls.byScope, g.Scope, g); prev != nil && clash == nil {
				warn(fmt.Sprintf("plugins %s and %s both provide scope %q; using %s's", where(prev.plugin), where(g.plugin), g.Scope, g.plugin.Name))
			}
		}
	}
	ls.engine = highlight.NewTextMate(func(scope string) ([]byte, error) {
		g := ls.byScope[scope]
		if g == nil {
			return nil, nil
		}
		return g.raw()
	}, highlight.Options{})
	return ls
}

func (ls *langSet) lookup(lang string) *Grammar { return ls.byName[strings.ToLower(lang)] }

// local returns the local grammar that serves scope, or nil for a built-in.
func (ls *langSet) local(scope string) *Grammar {
	if g := ls.byScope[scope]; g != nil && g.plugin.dir != "" {
		return g
	}
	return nil
}

// noLanguage are fence tags that say "this is not code in any language".
// They are common and deliberate, and several sit one edit from a real
// name (text and tex), so they never draw a near-miss warning.
var noLanguage = map[string]bool{"text": true, "txt": true, "plain": true, "plaintext": true,
	"none": true, "output": true, "tree": true, "ascii": true, "logs": true}

// unknownLanguages are languages tm-grammars lacks, not typos: "did you mean" would mislead.
var unknownLanguages = map[string]bool{"patch": true, "nasm": true, "masm": true, "ninja": true, "rest": true}

// nearMiss returns the known name closest to lang, for a likely typo: lang
// is at least four runes, not known, not a no-language tag, not a language
// the built-in grammars leave out or the collection lacks, and within one
// edit of a name at least as long (two edits from eight runes up). Ties go
// to the name that sorts first.
func (ls *langSet) nearMiss(lang string) string {
	l := strings.ToLower(lang)
	n := len([]rune(l))
	if n < 4 || ls.byName[l] != nil || noLanguage[l] || ls.leftOut[l] || unknownLanguages[l] {
		return ""
	}
	limit := 1
	if n >= 8 {
		limit = 2
	}
	names := make([]string, 0, len(ls.byName))
	for name := range ls.byName {
		names = append(names, name)
	}
	sort.Strings(names)
	best, bestD := "", limit+1
	for _, name := range names {
		if len([]rune(name)) < n {
			continue
		}
		if d := editDistance(l, name); d < bestD {
			best, bestD = name, d
		}
	}
	return best
}

// editDistance is the optimal string alignment distance between a and b,
// in runes: Levenshtein, with swapping two adjacent runes as one edit,
// since that is the commonest typo.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}

// languagesOf is every grammar a fence can reach by its own name, sorted.
func languagesOf(ls *langSet) []*Grammar {
	var out []*Grammar
	for n, g := range ls.byName {
		if n == g.Name {
			out = append(out, g)
		}
	}
	return sortedGrammars(out)
}

// Languages returns every language a page could highlight with the
// built-in grammars and plugins, sorted by name. A language another plugin
// overrides is left out. --list-languages prints it.
func Languages(cache *PluginCache, plugins []*Plugin, warn func(string)) []*Grammar {
	return languagesOf(cache.languages(plugins, warn))
}
