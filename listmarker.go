package md2html

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// listItem is one entry of a [[list]] or [[subdirs]]: its value and, for a
// symlink, the name of the directory it points at.
type listItem struct{ item, target string }

var templateRe = regexp.MustCompile(`\{([a-z][a-z0-9-]*)\}`)

// listMarkers expands [[list]] and [[subdirs]], which work on any page.
// meta is the front matter of the file holding the markers, which their
// templates read, and dir is its directory, which [[subdirs]] paths are
// relative to.
func listMarkers(meta map[string]string, dir, refRoot string, warn func(string)) Transform {
	say := func(format string, args ...any) {
		if warn != nil {
			warn(fmt.Sprintf(format, args...))
		}
	}
	return Transform{Name: "listMarkers", Fn: func(root *html.Node) error {
		ms := findMarkers(root)
		// References resolve here, for the keys these markers read and no
		// others (see resolveRefs).
		meta := resolveRefs(meta, dir, refRoot, markerKeys(ms), warn)
		for _, m := range ms {
			var items []listItem
			switch m.name {
			case "list":
				items = valueItems(m.attrs["key"], meta, say)
			case "subdirs":
				items = subdirItems(m.attrs, dir, say)
			default:
				continue
			}
			replaceMarker(m, renderList(m, items, meta, say))
		}
		return nil
	}}
}

func valueItems(key string, meta map[string]string, say func(string, ...any)) []listItem {
	if key == "" {
		say("[[list]] has no key attribute; it lists nothing")
		return nil
	}
	v, ok := meta[strings.ToLower(key)]
	if !ok {
		say("[[list]] key %q is not in the front matter; it lists nothing", key)
		return nil
	}
	var items []listItem
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			items = append(items, listItem{item: s})
		}
	}
	return items
}

func subdirItems(attrs map[string]string, dir string, say func(string, ...any)) []listItem {
	p := attrs["path"]
	if p == "" {
		say("[[subdirs]] has no path attribute; it lists nothing")
		return nil
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, filepath.FromSlash(p))
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		say("[[subdirs]] cannot read %s; it lists nothing", attrs["path"])
		return nil
	}
	match := attrs["match"]
	if match == "" {
		match = "*"
	}
	var links, dirs []listItem
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(p, name)
		if e.Type()&os.ModeSymlink != 0 {
			st, err := os.Stat(full)
			if err != nil {
				say("[[subdirs]] symlink %s points at nothing; leaving it out", name)
				continue
			}
			if !st.IsDir() {
				continue
			}
			target, _ := os.Readlink(full)
			links = append(links, listItem{item: name, target: filepath.Base(target)})
			continue
		}
		if !e.IsDir() {
			continue
		}
		if ok, _ := filepath.Match(match, name); ok {
			dirs = append(dirs, listItem{item: name})
		}
	}
	sort.Slice(links, func(i, j int) bool { return links[i].item < links[j].item })
	switch s := attrs["sort"]; s {
	case "name":
		sort.Slice(dirs, func(i, j int) bool { return dirs[i].item < dirs[j].item })
	case "", "version":
		sort.Slice(dirs, func(i, j int) bool { return versionCompare(dirs[i].item, dirs[j].item) > 0 })
	default:
		say("[[subdirs]] sort %q is not version or name; sorting by version", s)
		sort.Slice(dirs, func(i, j int) bool { return versionCompare(dirs[i].item, dirs[j].item) > 0 })
	}
	return append(links, dirs...)
}

// versionCompare orders two names by their runs of digits as numbers and
// the text between them as text, so v0.13 is above v0.9.
func versionCompare(a, b string) int {
	ra, rb := versionRuns(a), versionRuns(b)
	for i := 0; i < len(ra) && i < len(rb); i++ {
		x, y := ra[i], rb[i]
		if isDigits(x) && isDigits(y) {
			x, y = strings.TrimLeft(x, "0"), strings.TrimLeft(y, "0")
			if len(x) != len(y) {
				return cmpInt(len(x), len(y))
			}
		}
		if c := strings.Compare(x, y); c != 0 {
			return c
		}
	}
	return cmpInt(len(ra), len(rb))
}

func versionRuns(s string) []string {
	var runs []string
	start := 0
	for i := 1; i <= len(s); i++ {
		if i == len(s) || isDigit(s[i]) != isDigit(s[i-1]) {
			runs = append(runs, s[start:i])
			start = i
		}
	}
	return runs
}

func isDigits(s string) bool { return s != "" && isDigit(s[0]) }

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// markerKeys returns the front matter keys the list markers in ms read, in
// document order: a [[list]]'s key, and each {name} in a template that is
// not one of the marker's own variables.
func markerKeys(ms []marker) []string {
	seen := map[string]bool{}
	var keys []string
	add := func(k string) {
		if k = strings.ToLower(k); k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for _, m := range ms {
		if m.name != "list" && m.name != "subdirs" {
			continue
		}
		if m.name == "list" {
			add(m.attrs["key"])
		}
		for _, a := range []string{"link", "text", "current"} {
			for _, t := range templateRe.FindAllString(m.attrs[a], -1) {
				if n := t[1 : len(t)-1]; n != "item" && n != "target" {
					add(n)
				}
			}
		}
	}
	return keys
}

// expandTemplate fills {name} from vars, then from meta. An unknown name
// is warned about and expands to nothing.
func expandTemplate(tpl string, vars, meta map[string]string, say func(string, ...any)) string {
	return templateRe.ReplaceAllStringFunc(tpl, func(m string) string {
		name := m[1 : len(m)-1]
		if v, ok := vars[name]; ok {
			return v
		}
		if v, ok := meta[name]; ok {
			return v
		}
		say("template %s names no front matter key; it expands to nothing", m)
		return ""
	})
}

// renderList builds the list: a <ul> of links, or with as="inline" a
// paragraph of links separated by commas. Nothing is rendered for an empty list.
func renderList(m marker, items []listItem, meta map[string]string, say func(string, ...any)) *html.Node {
	if len(items) == 0 {
		return nil
	}
	// Every entry expands the same templates, so a mistake in one is said
	// once for the marker, not once per entry.
	said := map[string]bool{}
	say = func(say func(string, ...any)) func(string, ...any) {
		return func(format string, args ...any) {
			if k := fmt.Sprintf(format, args...); !said[k] {
				said[k] = true
				say("%s", k)
			}
		}
	}(say)
	text := m.attrs["text"]
	if text == "" {
		text = "{item}"
	}
	current := ""
	if c, ok := m.attrs["current"]; ok {
		current = expandTemplate(c, nil, meta, say)
	}
	link, hasLink := m.attrs["link"]
	if !hasLink {
		say("[[%s]] has no link attribute; listing its entries as text", m.name)
	}
	inline := m.attrs["as"] == "inline"
	if as := m.attrs["as"]; as != "" && as != "inline" && as != "list" {
		say("[[%s]] as %q is not list or inline; rendering a list", m.name, as)
	}
	var box *html.Node
	if inline {
		// A paragraph, not a span: the marker was a paragraph of its own, so
		// the run of links takes its place as one.
		box = elem("p", "class", "md2html-list md2html-inline")
	} else {
		box = elem("ul", "class", "md2html-list")
	}
	for i, it := range items {
		vars := map[string]string{"item": it.item, "target": it.target}
		label := textNode(expandTemplate(text, vars, meta, say))
		var entry *html.Node
		if hasLink {
			entry = elem("a", "href", expandTemplate(link, vars, meta, say))
			if current != "" && it.item == current {
				entry.Attr = append(entry.Attr, html.Attribute{Key: "aria-current", Val: "true"})
			}
			entry.AppendChild(label)
		} else {
			entry = label
		}
		if inline {
			if i > 0 {
				box.AppendChild(textNode(", "))
			}
			box.AppendChild(entry)
			continue
		}
		li := elem("li")
		li.AppendChild(entry)
		box.AppendChild(li)
	}
	return box
}
