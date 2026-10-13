package md2html

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
)

// ManifestName is the file that makes a directory an md2html plugin. It is
// named for the tool rather than generically, so a plugin is recognisable
// in a repository full of other tools' files, and so a future remote source
// can tell a repository is a plugin by this file alone.
const ManifestName = "md2html-plugin.json"

// pluginFormat is the one manifest format this release reads. A later
// format that only adds keys keeps this number; one that changes what an
// existing key means bumps it, and this release then refuses it rather than
// misreading it.
const pluginFormat = 1

// pluginName is the shape of a plugin or style name: short enough to type
// in front matter unquoted, and never mistakable for a path.
var pluginName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// reservedName belongs to the built-in default plugin and its one style, so
// that "style: default" always means the plain default.
const reservedName = "default"

// Plugin is one loaded plugin: its manifest's identity and what it
// provides. In this release that is styles only.
type Plugin struct {
	Name, Description string
	Styles            map[string]*Style

	key  string // cache identity: "builtin:<name>", or the real directory
	fsys fs.FS  // the plugin's files, rooted at its directory
	dir  string // the real directory on disk; "" for a built-in plugin
}

// Style is one named stylesheet a plugin provides.
type Style struct {
	Name, Description string
	// BaseNone means the style starts from nothing rather than from the
	// default stylesheet. Only the first style a page selects may.
	BaseNone bool
	// Charts is the style's charts block: the face chart text is measured
	// in and the width charts are laid out at. Nil when it has none.
	Charts *chartSettings

	css []string // CSS files, relative to the plugin directory, in order
}

// StyleNames returns the plugin's style names in sorted order.
func (p *Plugin) StyleNames() []string {
	names := make([]string, 0, len(p.Styles))
	for n := range p.Styles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// parseManifest reads a manifest. where names the plugin in every message.
// builtin permits the reserved name, which only the default plugin uses.
//
// Unknown keys are warned about and skipped rather than refused, so that a
// plugin written for a later release that only adds keys still loads here.
func parseManifest(data []byte, where string, builtin bool, warn func(string)) (*Plugin, error) {
	fail := func(format string, a ...any) (*Plugin, error) {
		return nil, fmt.Errorf("plugin %s: "+format, append([]any{where}, a...)...)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return fail("%s is not valid JSON: %v", ManifestName, err)
	}

	var format int
	if ok, err := take(top, "format", &format); err != nil {
		return fail("%s: format: %v", ManifestName, err)
	} else if !ok {
		return fail("%s has no format", ManifestName)
	}
	if format != pluginFormat {
		return fail("format %d is not supported by md2html %s, which reads format %d", format, Version, pluginFormat)
	}

	p := &Plugin{Styles: map[string]*Style{}}
	if ok, err := take(top, "name", &p.Name); err != nil {
		return fail("%s: name: %v", ManifestName, err)
	} else if !ok {
		return fail("%s has no name", ManifestName)
	}
	if err := checkName("name", p.Name, builtin); err != nil {
		return fail("%v", err)
	}
	if _, err := take(top, "description", &p.Description); err != nil {
		return fail("%s: description: %v", ManifestName, err)
	}

	var styles map[string]map[string]json.RawMessage
	if _, err := take(top, "styles", &styles); err != nil {
		return fail("%s: styles: %v", ManifestName, err)
	}
	for _, k := range sortedJSONKeys(top) {
		if warn != nil {
			warn(fmt.Sprintf("plugin %s: unknown key %q in %s; ignoring it", where, k, ManifestName))
		}
	}
	if len(styles) == 0 {
		return fail("provides nothing: %s needs a non-empty styles", ManifestName)
	}

	for _, name := range sortedJSONKeys(styles) {
		raw := styles[name]
		if err := checkName("style name", name, builtin); err != nil {
			return fail("%v", err)
		}
		s := &Style{Name: name}
		if _, err := take(raw, "description", &s.Description); err != nil {
			return fail("style %q: description: %v", name, err)
		}
		var base string
		if _, err := take(raw, "base", &base); err != nil {
			return fail("style %q: base: %v", name, err)
		}
		switch base {
		case "", "default":
		case "none":
			s.BaseNone = true
		default:
			return fail("style %q: base %q is not default or none", name, base)
		}
		if _, err := take(raw, "css", &s.css); err != nil {
			return fail("style %q: css: %v", name, err)
		}
		if len(s.css) == 0 {
			return fail("style %q has no css files", name)
		}
		if block, ok := raw["charts"]; ok {
			delete(raw, "charts")
			cs, err := parseCharts(block, func(k string) {
				if warn != nil {
					warn(fmt.Sprintf("plugin %s: unknown key %q in style %q's charts; ignoring it", where, k, name))
				}
			}, func(msg string) {
				if warn != nil {
					warn(fmt.Sprintf("plugin %s: style %q's charts: %s", where, name, msg))
				}
			})
			if err != nil {
				return fail("style %q: charts: %v", name, err)
			}
			s.Charts = cs
		}
		for _, k := range sortedJSONKeys(raw) {
			if warn != nil {
				warn(fmt.Sprintf("plugin %s: unknown key %q in style %q; ignoring it", where, k, name))
			}
		}
		p.Styles[name] = s
	}
	return p, nil
}

// parseCharts reads a style's charts block. unknown is called with each
// key it does not know, in sorted order, and note with each key it knows
// but that has no effect.
func parseCharts(data json.RawMessage, unknown, note func(string)) (*chartSettings, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	cs := &chartSettings{}
	for _, f := range []struct {
		key string
		dst any
	}{{"font", &cs.Font}, {"metrics", &cs.Metrics}, {"pad", &cs.Pad}, {"width", &cs.Width}, {"font-family", &cs.FontFamily}} {
		ok, err := take(raw, f.key, f.dst)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", f.key, err)
		}
		if f.key == "pad" {
			cs.padSet = ok
		}
		switch {
		case !ok:
		case f.key == "metrics" && cs.Metrics != systemSansMetrics:
			return nil, fmt.Errorf("metrics %q is not %s, the one built-in", cs.Metrics, systemSansMetrics)
		case f.key == "pad" && (cs.Pad < 0 || cs.Pad > 1):
			return nil, fmt.Errorf("pad %v is not between 0 and 1", cs.Pad)
		case f.key == "width" && (cs.Width < 100 || cs.Width > 2000):
			return nil, fmt.Errorf("width %d is not between 100 and 2000", cs.Width)
		}
	}
	if cs.Font != "" && cs.Metrics != "" {
		return nil, errors.New("font and metrics are both set; name one, since a font's widths are exact")
	}
	if cs.Font != "" && cs.padSet {
		note("pad has no effect beside font, whose widths are exact; ignoring it")
	}
	for _, k := range sortedJSONKeys(raw) {
		unknown(k)
	}
	return cs, nil
}

// checkName holds a plugin or style name to pluginName, and keeps the
// reserved name for the built-in default plugin.
func checkName(what, name string, builtin bool) error {
	if !pluginName.MatchString(name) {
		return fmt.Errorf("%s %q is not lowercase letters, digits and hyphens starting with a letter", what, name)
	}
	if name == reservedName && !builtin {
		return fmt.Errorf("%s %q is reserved for the built-in default plugin", what, name)
	}
	return nil
}

// take decodes m[key] into dst and removes it from m, so that whatever is
// left in m afterwards is the keys nobody asked for. It reports whether the
// key was present.
func take(m map[string]json.RawMessage, key string, dst any) (bool, error) {
	raw, ok := m[key]
	if !ok {
		return false, nil
	}
	delete(m, key)
	return true, json.Unmarshal(raw, dst)
}

// sortedJSONKeys returns m's keys in sorted order, so that messages about
// them come out the same way on every run. It is generic because a manifest
// has both raw and decoded maps to walk; attrs.go's sortedKeys covers only
// string maps.
func sortedJSONKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
