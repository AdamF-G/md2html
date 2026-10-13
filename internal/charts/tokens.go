package charts

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Tokens lists every chart token, in the order their placeholders are
// numbered.
var Tokens = []string{"chart-surface", "fg", "muted", "chart-grid", "chart-axis",
	"series-1", "series-2", "series-3", "series-4", "series-5", "series-6", "series-7", "series-8",
	"status-good", "status-warning", "status-serious", "status-critical"}

func placeholder(token string) string {
	for i, t := range Tokens {
		if t == token {
			return fmt.Sprintf("#0a0b%02x", i+1)
		}
	}
	return ""
}

func tokenOfPlaceholder(hex string) (string, bool) {
	for _, t := range Tokens {
		if placeholder(t) == hex {
			return t, true
		}
	}
	return "", false
}

// themeConfig is merged under a spec's own config, so the author's
// explicit choices win.
func themeConfig(fontFamily string) map[string]any {
	p := placeholder
	series := make([]any, 8)
	for i := range series {
		series[i] = p(fmt.Sprintf("series-%d", i+1))
	}
	return map[string]any{
		"background": nil, "font": fontFamily, "view": map[string]any{"stroke": nil},
		"axis": map[string]any{"domainColor": p("chart-axis"), "gridColor": p("chart-grid"), "gridWidth": 1,
			"tickColor": p("chart-axis"), "labelColor": p("muted"), "titleColor": p("muted"),
			"labelFontSize": 12, "titleFontSize": 12, "titleFontWeight": 500},
		"legend": map[string]any{"labelColor": p("fg"), "titleColor": p("muted"), "labelFontSize": 12},
		"title":  map[string]any{"color": p("fg")},
		"text":   map[string]any{"color": p("fg"), "fontSize": 12},
		"mark":   map[string]any{"color": p("series-1")},
		"point":  map[string]any{"filled": true, "size": 100, "strokeWidth": 2, "stroke": p("chart-surface")},
		"bar":    map[string]any{"cornerRadiusEnd": 4, "stroke": p("chart-surface"), "strokeWidth": 2},
		"rule":   map[string]any{"color": p("chart-axis")},
		"line":   map[string]any{"strokeWidth": 2, "strokeCap": "round", "strokeJoin": "round"},
		"range":  map[string]any{"category": series},
	}
}

var placeholderRe = regexp.MustCompile(`#0a0b[0-9a-f]{2}`)

// substituteTokenNames rewrites each "token:<name>" in a spec to its
// token's placeholder, and warns of an unknown name or a placeholder the
// spec names itself. Data, under any "data" or "datasets" key, is the
// author's values, not colours, and is left alone. The warnings are
// sorted, so their order does not follow map iteration.
func substituteTokenNames(v any) (any, []string) {
	var warns []string
	var walk func(any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				if k == "data" || k == "datasets" {
					continue
				}
				x[k] = walk(e)
			}
		case []any:
			for i, e := range x {
				x[i] = walk(e)
			}
		case string:
			if name, ok := strings.CutPrefix(x, "token:"); ok {
				if p := placeholder(name); p != "" {
					return p
				}
				warns = append(warns, fmt.Sprintf("chart: %q names no chart token; the tokens are %s", x, strings.Join(Tokens, ", ")))
			} else if placeholderRe.MatchString(strings.ToLower(x)) {
				warns = append(warns, fmt.Sprintf("chart: the colour %s is reserved by md2html for a chart token; it will render as that token", x))
			}
		}
		return v
	}
	out := walk(v)
	sort.Strings(warns)
	return out, warns
}

var startTag = regexp.MustCompile(`<[A-Za-z][^>]*>`)
var tagAttr = regexp.MustCompile(`(\s)(fill|stroke|style)="([^"]*)"`)

// tokenize rewrites each placeholder fill or stroke into a style
// declaration. Per start tag it merges those with any existing style
// attribute, so an element never carries two.
func tokenize(svg string) (string, bool, error) {
	literal := false
	out := startTag.ReplaceAllStringFunc(svg, func(tag string) string {
		var rewritten []string
		var existing []string
		for _, m := range tagAttr.FindAllStringSubmatch(tag, -1) {
			v := strings.ToLower(m[3])
			switch {
			case m[2] == "style":
				existing = append(existing, strings.TrimRight(strings.TrimSpace(m[3]), ";"))
			default:
				if tok, ok := tokenOfPlaceholder(v); ok {
					rewritten = append(rewritten, fmt.Sprintf("%s: var(--%s)", m[2], tok))
				} else if v != "none" && v != "transparent" && !strings.HasPrefix(v, "url(") {
					literal = true
				}
			}
		}
		if len(rewritten) == 0 {
			return tag
		}
		merged := strings.Join(append(rewritten, existing...), "; ")
		placed := false
		return tagAttr.ReplaceAllStringFunc(tag, func(s string) string {
			m := tagAttr.FindStringSubmatch(s)
			if m[2] != "style" {
				if _, ok := tokenOfPlaceholder(strings.ToLower(m[3])); !ok {
					return s
				}
			}
			if placed {
				return ""
			}
			placed = true
			return m[1] + `style="` + merged + `"`
		})
	})
	if left := placeholderRe.FindString(out); left != "" {
		return "", false, fmt.Errorf("chart: placeholder %s survived the rewrite", left)
	}
	return out, literal, nil
}

var themeBlock = regexp.MustCompile(`:root\[data-theme="(light|dark)"\]\s*\{([^}]*)\}`)

// declRe reads one declaration of a block's body. The last may end at the
// block's end with no ";", as minified CSS writes it.
var declRe = regexp.MustCompile(`--([a-z0-9-]+)\s*:\s*([^;}]+)(?:;|$)`)
var varRef = regexp.MustCompile(`^var\(\s*--([a-z0-9-]+)\s*\)$`)

// boxBorder is the page colour a tooltip box's border is drawn in. It is
// not a chart token, but a chart written in literal colours needs its value.
const boxBorder = "rule"

// TokenValues reads each token's light and dark value from an assembled
// stylesheet, and --rule's, for the tooltip box border. ok is false when
// the stylesheet defines none of the tokens. A value of var(--x) becomes
// --x's value from the same block.
func TokenValues(css string) (light, dark map[string]string, ok bool) {
	light, dark = map[string]string{}, map[string]string{}
	want := map[string]bool{boxBorder: true}
	for _, t := range Tokens {
		want[t] = true
	}
	// Every declaration of each theme, tokens or not, so a reference such
	// as var(--bg) can be resolved.
	all := map[string]map[string]string{"light": {}, "dark": {}}
	for _, b := range themeBlock.FindAllStringSubmatch(css, -1) {
		dst := light
		if b[1] == "dark" {
			dst = dark
		}
		decls := all[b[1]]
		for _, d := range declRe.FindAllStringSubmatch(b[2], -1) {
			v := strings.TrimSpace(d[2])
			decls[d[1]] = v // later blocks win, as the cascade does
			if want[d[1]] {
				dst[d[1]] = v
				ok = ok || d[1] != boxBorder
			}
		}
	}
	for name, dst := range map[string]map[string]string{"light": light, "dark": dark} {
		for k, v := range dst {
			dst[k] = resolveVar(v, all[name])
		}
	}
	return light, dark, ok
}

func resolveVar(v string, decls map[string]string) string {
	for range 8 { // bounded, so a reference cycle cannot loop
		m := varRef.FindStringSubmatch(v)
		if m == nil {
			return v
		}
		next, found := decls[m[1]]
		if !found {
			return v
		}
		v = next
	}
	return v
}

func withColours(svg string, values map[string]string) string {
	return placeholderRe.ReplaceAllStringFunc(svg, func(p string) string {
		if t, ok := tokenOfPlaceholder(p); ok {
			if v, ok := values[t]; ok {
				return v
			}
		}
		return p
	})
}
