package md2html

import (
	"errors"
	"fmt"
	"path/filepath"
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
	ps, err := c.style(names, plugins, warn)
	return ps.css, err
}

// pageStyle is what a page's style list resolves to: its stylesheet and
// the settings its charts are drawn with.
type pageStyle struct {
	css    string
	charts chartSettings
}

// style is Stylesheet, plus the chart settings of the same list.
func (c *PluginCache) style(names []string, plugins []*Plugin, warn func(string)) (pageStyle, error) {
	if len(names) == 0 {
		return pageStyle{defaultCSS, c.chartStyle(nil)}, nil
	}
	avail, err := AvailablePlugins(plugins)
	if err != nil {
		return pageStyle{}, err
	}
	var picks []selected
	seen := map[string]bool{}
	for _, n := range names {
		s, err := resolveStyle(n, avail)
		if err != nil {
			return pageStyle{}, err
		}
		if seen[s.plugin.key+"/"+s.style.Name] {
			if warn != nil {
				warn(fmt.Sprintf("style %s is selected twice; using it once", s.qualified()))
			}
			continue
		}
		seen[s.plugin.key+"/"+s.style.Name] = true
		if len(picks) > 0 && s.style.BaseNone {
			return pageStyle{}, fmt.Errorf("style %s starts from nothing (base: none), so it can only come first", s.qualified())
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
	return pageStyle{e.css, c.chartStyle(picks)}, e.err
}

// assemble concatenates the default stylesheet, unless the first pick
// starts from nothing, and each pick's CSS files in order, one newline
// between parts. A lone base-none pick is its CSS exactly, so selecting
// the default style reproduces defaultCSS byte for byte.
//
// The joined sheet is scanned once more, as the browser will read it,
// since files that each pass alone can join into one that fetches; see
// checkInline.
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
	sheet := strings.Join(parts, "\n")
	if err := checkInline(sheet); err != nil {
		names := make([]string, len(picks))
		for i, s := range picks {
			names[i] = s.qualified()
		}
		return "", fmt.Errorf("style %s: %w", strings.Join(names, ", "), err)
	}
	return sheet, nil
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

// maxStylesheet is the size past which a page's stylesheet is warned
// about: every page carries its own copy, and an Artifact is capped at
// 16 MB, fonts and all.
const maxStylesheet = 1 << 20

// choosePageStyle chooses one page's stylesheet and chart settings: its front
// matter "style:" if it has one, otherwise the run's CSS or Styles,
// otherwise the default. A run's CSS draws charts with the default style's
// settings.
//
// A front matter choice that fails is warned about and the page falls back
// to the run's whole style list, not to the list minus the bad entry: a
// page that loses one of three stacked styles renders as a look nobody
// chose. The run's own choice failing is an error, since a caller that
// asked for a style and silently got another would ship the wrong look.
func choosePageStyle(opt Options, meta map[string]string) (pageStyle, error) {
	warn := func(s string) {
		if opt.Warn != nil {
			opt.Warn(s)
		}
	}
	if opt.CSS != "" && len(opt.Styles) > 0 {
		return pageStyle{}, errors.New("Options.CSS and Styles are both set; choose one")
	}
	cache := opt.PluginCache
	if cache == nil {
		cache = NewPluginCache()
	}
	run := func() (pageStyle, error) {
		if opt.CSS != "" {
			return pageStyle{opt.CSS, cache.chartStyle(nil)}, nil
		}
		if len(opt.Styles) == 0 {
			return cache.style(nil, nil, nil)
		}
		ps, err := cache.LoadAll(opt.Plugins, "", nil)
		if err != nil {
			return pageStyle{}, err
		}
		return cache.style(opt.Styles, ps, nil)
	}

	names := splitList(meta["style"])
	pageDirs := splitList(meta["plugins"])
	var st pageStyle
	var err error
	switch {
	case len(names) == 0:
		// The page's plugins only widen what its own style: can name; the
		// run's list resolves against the run's plugins, so loading one here
		// can never make the run's bare name ambiguous on this page.
		if len(pageDirs) > 0 {
			warn("front matter plugins has no effect without style; ignoring it")
		}
		st, err = run()
	default:
		st, err = frontMatterStyle(cache, opt, names, pageDirs, warn)
		if err != nil {
			warn(err.Error() + "; using the run's style")
			st, err = run()
		}
	}
	if err != nil {
		return pageStyle{}, err
	}
	if len(st.css) > maxStylesheet {
		warn(fmt.Sprintf("stylesheet is %d KB, over 1 MB; every page carries it", len(st.css)/1024))
	}
	return st, nil
}

// frontMatterStyle assembles a page's own style list from the run's plugins
// and the page's, the page's resolved against its own directory. The error
// says which key was at fault, for the warning choosePageStyle raises.
func frontMatterStyle(cache *PluginCache, opt Options, names, pageDirs []string, warn func(string)) (pageStyle, error) {
	ps, err := cache.LoadAll(opt.Plugins, "", nil)
	if err != nil {
		return pageStyle{}, fmt.Errorf("front matter style: %w", err)
	}
	baseDir := ""
	if opt.SourcePath != "" {
		baseDir = filepath.Dir(opt.SourcePath)
	}
	own, err := cache.LoadAll(pageDirs, baseDir, warn)
	if err != nil {
		return pageStyle{}, fmt.Errorf("front matter plugins: %w", err)
	}
	st, err := cache.style(names, append(ps, own...), warn)
	if err != nil {
		return pageStyle{}, fmt.Errorf("front matter style: %w", err)
	}
	return st, nil
}
