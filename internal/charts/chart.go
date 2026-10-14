package charts

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Style is how a page's stylesheet asks for its charts to be drawn.
type Style struct {
	Measurer   Measurer // widths of the face chart text renders in; nil means SystemSans(0.1)
	FontFamily string   // the CSS stack chart text renders in
	Width      int      // plot width in px when the spec sets none
	// Literal, when non-nil, holds the colour each token takes, and
	// "rule", the tooltip box border's: the chart is written with these
	// values instead of var() references, for a stylesheet that defines
	// no chart tokens.
	Literal map[string]string
	// Light and Dark are the tokens' values in each theme. A chart whose
	// own colours are not tokens is rendered once with each, and the two
	// copies are switched by the stylesheet's theme rules.
	Light, Dark map[string]string
	// Tokens are colour tokens the style adds, which a spec may name after
	// the chart and page tokens. Literal, Light and Dark hold their values
	// too.
	Tokens []string
}

// Request is one chart to render.
type Request struct {
	Body    []byte // the fence body, YAML or JSON
	Caption string
	Classes []string // the author's own classes, written on the figure after md2html-chart
	Index   int      // the chart's number on the page, from 1
	Style   Style
	Load    func(string) (string, error) // nil: no data files
}

// composite holds the top-level keys of a spec whose views are laid out
// side by side or in a grid. A top-level width does not apply to them, so
// the style's width is left out.
var composite = []string{"hconcat", "vconcat", "concat", "facet", "repeat"}

// Render returns the chart's markup, or an error the caller turns into one
// warning and a code block.
func Render(req Request) (string, []string, error) {
	var parsed any
	if err := yaml.Unmarshal(req.Body, &parsed); err != nil {
		return "", nil, fmt.Errorf("the spec is not YAML or JSON: %v", err)
	}
	spec, ok := normalize(parsed).(map[string]any)
	if !ok {
		if parsed == nil {
			return "", nil, errors.New("the fence is empty; it needs a Vega-Lite spec")
		}
		return "", nil, errors.New("the spec is not a mapping of Vega-Lite keys")
	}
	ts := newTokenSet(req.Style.Tokens)
	v, warns := substituteTokenNames(spec, ts)
	spec = mergeConfig(v, themeConfig(req.Style.FontFamily)).(map[string]any)
	// A build has no container to measure, so "container" is no width or
	// height: the width is the style's, and the height Vega-Lite's default.
	if w, set := spec["width"]; (!set || w == "container") && req.Style.Width > 0 && !hasAny(spec, composite) {
		spec["width"] = req.Style.Width
	}
	for _, k := range []string{"width", "height"} {
		if spec[k] == "container" {
			delete(spec, k)
		}
	}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		return "", nil, err
	}
	m := req.Style.Measurer
	if m == nil {
		m = SystemSans(0.1)
	}
	r, err := renderSpec(specJSON, m, req.Load)
	if err != nil {
		return "", nil, err
	}
	warns = append(warns, r.Warnings...)

	prefix := fmt.Sprintf("md2html-c%d", req.Index)
	st := req.Style
	type copyOf struct{ class, svg, css string }
	var copies []copyOf
	// layer adds the hover layer to one copy of the SVG. values, when
	// non-nil, also resolves the var() references the layer writes, so a
	// copy drawn in literal colours holds no reference the stylesheet
	// cannot answer.
	matched, lost, why := true, 0, ""
	layer := func(class, svg, pfx string, values map[string]string) {
		h := buildHover(svg, r, pfx, m)
		matched = matched && h.matched
		lost, why = h.lost, h.why // the same in every copy
		out := h.svg
		if values != nil {
			out = resolveVars(out, values)
		}
		copies = append(copies, copyOf{class, out, h.css})
	}
	switch {
	case st.Literal != nil:
		layer("", withColours(r.SVG, st.Literal, ts), prefix, st.Literal)
	default:
		svg, literal, err := tokenize(r.SVG, ts)
		if err != nil {
			return "", nil, err
		}
		if literal && st.Light != nil && st.Dark != nil {
			// The second copy takes its own prefix, so no id is on the
			// page twice.
			layer("md2html-chart-light", withColours(r.SVG, st.Light, ts), prefix, st.Light)
			layer("md2html-chart-dark", withColours(r.SVG, st.Dark, ts), prefix+"-dark", st.Dark)
		} else {
			layer("", svg, prefix, nil)
		}
	}
	if !matched {
		warns = append(warns, fmt.Sprintf("chart: %d tooltip marks could not be matched to the SVG; the chart has no hover", len(r.Marks)))
	} else if lost > 0 {
		warns = append(warns, fmt.Sprintf("chart: %d line or area tooltips have no hover: %s; the table view holds their values", lost, why))
	}

	var b strings.Builder
	class := strings.Join(append([]string{"md2html-chart"}, req.Classes...), " ")
	fmt.Fprintf(&b, `<figure class="%s" id="%s"`, html.EscapeString(class), prefix)
	if r.elements() > maxTabStops {
		// Too many marks to tab through: the figure is the one stop, and
		// the table view below it holds every value.
		b.WriteString(` tabindex="0"`)
	}
	b.WriteString(">")
	var css strings.Builder
	for _, c := range copies {
		if c.class != "" {
			b.WriteString(`<div class="` + c.class + `">` + c.svg + "</div>")
		} else {
			b.WriteString(c.svg)
		}
		css.WriteString(c.css)
	}
	if css.Len() > 0 {
		b.WriteString("<style>\n" + css.String() + "</style>")
	}
	if req.Caption != "" {
		b.WriteString("<figcaption>" + html.EscapeString(req.Caption) + "</figcaption>")
	}
	b.WriteString(tableView(req.Caption, r.Fields, r.Rows))
	b.WriteString("</figure>")
	return b.String(), warns, nil
}

// normalize turns the maps YAML decodes into map[string]any throughout, so
// the spec marshals as JSON. A key that is not a string is written as YAML
// would print it.
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = normalize(e)
		}
		return x
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[fmt.Sprint(k)] = normalize(e)
		}
		return out
	case []any:
		for i, e := range x {
			x[i] = normalize(e)
		}
		return x
	}
	return v
}

func hasAny(m map[string]any, keys []string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

var cssVar = regexp.MustCompile(`var\(--([a-z0-9-]+)\)`)

// resolveVars replaces each var(--token) whose token values names with its
// value, leaving any other reference as it is.
func resolveVars(s string, values map[string]string) string {
	return cssVar.ReplaceAllStringFunc(s, func(ref string) string {
		if v, ok := values[cssVar.FindStringSubmatch(ref)[1]]; ok {
			return v
		}
		return ref
	})
}

// mergeConfig lays theme under the spec's own config: a key the spec sets
// wins, and nested objects merge key by key. It returns the spec with the
// merged config; theme is not modified.
func mergeConfig(spec any, theme map[string]any) any {
	s, ok := spec.(map[string]any)
	if !ok {
		return spec
	}
	own, _ := s["config"].(map[string]any)
	s["config"] = deepMerge(theme, own)
	return s
}

// deepMerge returns base overlaid with over. Neither argument is modified.
func deepMerge(base, over map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(over))
	for k, v := range base {
		if m, ok := v.(map[string]any); ok {
			v = deepMerge(m, nil)
		}
		out[k] = v
	}
	for k, v := range over {
		bm, bok := out[k].(map[string]any)
		om, ook := v.(map[string]any)
		if bok && ook {
			out[k] = deepMerge(bm, om)
			continue
		}
		out[k] = v
	}
	return out
}
