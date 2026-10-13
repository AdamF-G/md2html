package md2html

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// PluginCache loads plugins and assembles stylesheets, keeping both, so a
// run that converts many pages reads each plugin and inlines its fonts
// once. It is safe for concurrent use: the CLI converts pages in parallel
// against one cache.
type PluginCache struct {
	mu      sync.Mutex
	plugins map[string]*pluginEntry // by real directory
	sheets  map[string]*sheetEntry  // by resolved style list; see stylesheet.go
}

// pluginEntry is one directory's load, done once. Its warnings are kept so
// that every caller hears them: a page loading a plugin another page
// already loaded is still told about that plugin's unknown keys.
type pluginEntry struct {
	once     sync.Once
	plugin   *Plugin
	warnings []string
	err      error
}

// NewPluginCache returns an empty cache.
func NewPluginCache() *PluginCache {
	return &PluginCache{plugins: map[string]*pluginEntry{}, sheets: map[string]*sheetEntry{}}
}

// Load reads and validates the plugin in dir. A relative dir is taken
// against baseDir, or the working directory when baseDir is empty.
//
// Entries are keyed by the directory with symlinks resolved, so two
// spellings of one plugin load it once and come back as one *Plugin.
// Errors are kept too: a broken plugin named by every page is read once and
// reported to each.
func (c *PluginCache) Load(dir, baseDir string, warn func(string)) (*Plugin, error) {
	p := dir
	if !filepath.IsAbs(p) && baseDir != "" {
		p = filepath.Join(baseDir, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", dir, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", dir, err)
	}

	c.mu.Lock()
	e, ok := c.plugins[real]
	if !ok {
		e = &pluginEntry{}
		c.plugins[real] = e
	}
	c.mu.Unlock()

	e.once.Do(func() {
		e.plugin, e.err = loadDir(real, dir, func(w string) { e.warnings = append(e.warnings, w) })
	})
	if warn != nil {
		for _, w := range e.warnings {
			warn(w)
		}
	}
	return e.plugin, e.err
}

// LoadAll loads each of dirs, in order, stopping at the first error.
func (c *PluginCache) LoadAll(dirs []string, baseDir string, warn func(string)) ([]*Plugin, error) {
	var out []*Plugin
	for _, d := range dirs {
		p, err := c.Load(d, baseDir, warn)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// loadDir reads the plugin at real, a directory with symlinks resolved.
// where is how the caller named it, for messages.
func loadDir(real, where string, warn func(string)) (*Plugin, error) {
	fi, err := os.Stat(real)
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", where, err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("plugin %s: not a directory", where)
	}
	fsys := os.DirFS(real)
	data, err := readContained(real, fsys, ManifestName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("plugin %s: no %s in it", where, ManifestName)
	} else if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", where, err)
	}
	p, err := parseManifest(data, where, false, warn)
	if err != nil {
		return nil, err
	}
	p.key, p.fsys, p.dir = real, fsys, real
	if err := p.loadChartFonts(); err != nil {
		return nil, err
	}
	return p, nil
}

// readFile reads rel, a slash-separated path in the plugin, refusing one
// that leads outside it: by "..", by being absolute, or, on disk, by a
// symlink pointing elsewhere. That is what stops a plugin pulling an
// arbitrary local file into every page it styles.
func (p *Plugin) readFile(rel string) ([]byte, error) {
	b, err := readContained(p.dir, p.fsys, rel)
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", p.Name, err)
	}
	return b, nil
}

// readContained reads rel from fsys, the plugin rooted at dir, or at no
// directory for a built-in, refusing a path that leads outside it. The
// manifest is read this way too, before the plugin has a name, so that a
// manifest symlinked from elsewhere cannot make a directory pass for a
// plugin it does not hold.
func readContained(dir string, fsys fs.FS, rel string) ([]byte, error) {
	clean := path.Clean(rel)
	if !fs.ValidPath(clean) {
		return nil, fmt.Errorf("%s is outside the plugin", rel)
	}
	if dir != "" {
		real, err := filepath.EvalSymlinks(filepath.Join(dir, filepath.FromSlash(clean)))
		if err != nil {
			return nil, err
		}
		if r, err := filepath.Rel(dir, real); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("%s is outside the plugin", rel)
		}
	}
	return fs.ReadFile(fsys, clean)
}

// builtinPlugins parses the embedded plugins once. A built-in that does not
// parse is a defect in the build, not in anyone's input, and the tests load
// every one, so it cannot ship.
var builtinPlugins = sync.OnceValues(func() ([]*Plugin, error) {
	entries, err := fs.ReadDir(builtinFS, "plugins")
	if err != nil {
		return nil, err
	}
	var out []*Plugin
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		root := "plugins/" + e.Name()
		data, err := fs.ReadFile(builtinFS, root+"/"+ManifestName)
		if err != nil {
			return nil, fmt.Errorf("built-in plugin %s: %w", e.Name(), err)
		}
		var warned error
		p, err := parseManifest(data, e.Name(), true, func(w string) { warned = errors.New(w) })
		if err != nil {
			return nil, err
		}
		if warned != nil {
			return nil, fmt.Errorf("built-in plugin must load without warnings: %w", warned)
		}
		if p.Name != e.Name() {
			return nil, fmt.Errorf("built-in plugin %s: its manifest names it %q", e.Name(), p.Name)
		}
		sub, err := fs.Sub(builtinFS, root)
		if err != nil {
			return nil, err
		}
		p.key, p.fsys = "builtin:"+p.Name, sub
		if err := p.loadChartFonts(); err != nil {
			return nil, fmt.Errorf("built-in %w", err)
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
})

// BuiltinPlugins returns the plugins compiled into this binary, sorted by
// name.
func BuiltinPlugins() []*Plugin {
	ps, err := builtinPlugins()
	if err != nil {
		panic(err)
	}
	return ps
}

// AvailablePlugins is every plugin a page can draw styles from: the
// built-ins and extra, sorted by name. One plugin listed twice is one
// plugin; two different plugins with one name are an error, since a
// qualified style name could not tell them apart.
func AvailablePlugins(extra []*Plugin) ([]*Plugin, error) {
	byName := map[string]*Plugin{}
	var out []*Plugin
	for _, p := range append(append([]*Plugin{}, BuiltinPlugins()...), extra...) {
		if q, ok := byName[p.Name]; ok {
			if q.key == p.key {
				continue
			}
			return nil, fmt.Errorf("plugins %s and %s are both named %q; rename one", where(q), where(p), p.Name)
		}
		byName[p.Name] = p
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// where names a plugin's location for a message.
func where(p *Plugin) string {
	if p.dir == "" {
		return "built-in " + p.Name
	}
	return p.dir
}

// findPlugin returns the plugin named name in ps, or nil.
func findPlugin(ps []*Plugin, name string) *Plugin {
	for _, p := range ps {
		if p.Name == name {
			return p
		}
	}
	return nil
}
