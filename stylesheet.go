package md2html

import (
	"fmt"
	"strings"
	"sync"
)

// sheetEntry is one assembled stylesheet, built once per resolved style
// list however many pages select it.
type sheetEntry struct {
	once sync.Once
	css  string
	err  error
}

// selected is one resolved style and the plugin it came from.
type selected struct {
	plugin *Plugin
	style  *Style
}

func (s selected) qualified() string { return s.plugin.Name + "/" + s.style.Name }

// Stylesheet returns the stylesheet for a page that selects names, stacked
// in order: the default stylesheet, unless the first style starts from
// nothing, and then each style's CSS with its assets inlined. Names resolve
// against the built-in plugins plus plugins. No names means the default.
//
// A style other than the first may not start from nothing: it would
// discard styles the same list asked to keep. Selecting one style twice
// keeps the first and warns, since the second adds nothing but bytes.
func (c *PluginCache) Stylesheet(names []string, plugins []*Plugin, warn func(string)) (string, error) {
	if len(names) == 0 {
		return defaultCSS, nil
	}
	avail, err := AvailablePlugins(plugins)
	if err != nil {
		return "", err
	}
	var picks []selected
	seen := map[string]bool{}
	for _, n := range names {
		s, err := resolveStyle(n, avail)
		if err != nil {
			return "", err
		}
		if seen[s.plugin.key+"/"+s.style.Name] {
			if warn != nil {
				warn(fmt.Sprintf("style %s is selected twice; using it once", s.qualified()))
			}
			continue
		}
		seen[s.plugin.key+"/"+s.style.Name] = true
		if len(picks) > 0 && s.style.BaseNone {
			return "", fmt.Errorf("style %s starts from nothing (base: none), so it can only come first", s.qualified())
		}
		picks = append(picks, s)
	}

	keys := make([]string, len(picks))
	for i, s := range picks {
		keys[i] = s.plugin.key + "/" + s.style.Name
	}
	key := strings.Join(keys, "\x00")
	c.mu.Lock()
	e, ok := c.sheets[key]
	if !ok {
		e = &sheetEntry{}
		c.sheets[key] = e
	}
	c.mu.Unlock()
	e.once.Do(func() { e.css, e.err = assemble(picks) })
	return e.css, e.err
}

// assemble concatenates the default stylesheet, unless the first pick
// starts from nothing, and each pick's CSS files in order, one newline
// between parts. A lone base-none pick is its CSS exactly, so selecting
// the default style reproduces defaultCSS byte for byte.
func assemble(picks []selected) (string, error) {
	var parts []string
	if !picks[0].style.BaseNone {
		parts = append(parts, defaultCSS)
	}
	for _, s := range picks {
		for _, f := range s.style.css {
			b, err := s.plugin.readFile(f)
			if err != nil {
				return "", fmt.Errorf("style %s: %w", s.qualified(), err)
			}
			css, err := inlineAssets(string(b), f, s.plugin.readFile)
			if err != nil {
				return "", fmt.Errorf("plugin %s: %w", s.plugin.Name, err)
			}
			parts = append(parts, css)
		}
	}
	return strings.Join(parts, "\n"), nil
}

// resolveStyle finds the style a name selects. A qualified name picks from
// one plugin; a bare one must match exactly one available style, and the
// error for an ambiguous one lists the qualified names to choose between.
func resolveStyle(name string, avail []*Plugin) (selected, error) {
	if pl, st, ok := strings.Cut(name, "/"); ok {
		p := findPlugin(avail, pl)
		if p == nil {
			return selected{}, fmt.Errorf("no plugin named %q; load it with --plugin or front matter plugins:", pl)
		}
		s, ok := p.Styles[st]
		if !ok {
			return selected{}, fmt.Errorf("plugin %s has no style %q; it has %s", pl, st, strings.Join(p.StyleNames(), ", "))
		}
		return selected{p, s}, nil
	}
	var matches []selected
	var all []string
	for _, p := range avail {
		for _, n := range p.StyleNames() {
			all = append(all, p.Name+"/"+n)
			if n == name {
				matches = append(matches, selected{p, p.Styles[n]})
			}
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return selected{}, fmt.Errorf("no style named %q; available: %s", name, strings.Join(all, ", "))
	}
	var q []string
	for _, m := range matches {
		q = append(q, m.qualified())
	}
	return selected{}, fmt.Errorf("style %q is ambiguous; name one of: %s", name, strings.Join(q, ", "))
}

// splitList reads a comma-separated list, as --style, --plugin and their
// front matter keys take, dropping blank entries.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
