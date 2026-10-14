package md2html

import (
	"fmt"
	"path/filepath"
	"slices"
	"sync"

	"github.com/AdamF-G/md2html/internal/charts"
)

// systemSansMetrics names the built-in proxy widths for a style whose
// chart text renders in the reader's system sans.
const systemSansMetrics = "builtin:system-sans"

// chartSettings is a style's charts block, and, once a page's styles are
// stacked, the settings its charts are drawn with.
type chartSettings struct {
	Font, Metrics string   // a font file in the plugin, or the built-in proxy; never both
	Pad           float64  // widen proxy-measured text by this fraction
	padSet        bool     // the block sets pad, 0 included
	Width         int      // plot width in px when a spec sets none
	FontFamily    string   // the CSS stack chart text renders in
	Tokens        []string // colour tokens the style adds for specs to name
	plugin        *Plugin  // the plugin Font is in

	// measurer is Font, parsed when its plugin loaded. It is kept with the
	// plugin, so the plugin cache is also the measurer cache: a font is
	// read once per plugin load however many pages draw charts with it.
	measurer charts.Measurer
}

// loadChartFonts reads and parses each style's charts font, so a font
// that is missing, outside the plugin or not a font fails the load.
func (p *Plugin) loadChartFonts() error {
	for _, name := range p.StyleNames() {
		cs := p.Styles[name].Charts
		if cs == nil || cs.Font == "" {
			continue
		}
		b, err := readContained(p.dir, p.fsys, cs.Font)
		if err == nil {
			cs.measurer, err = charts.NewFontMeasurer(b)
		}
		if err != nil {
			return fmt.Errorf("plugin %s: style %q: charts font: %v", p.Name, name, err)
		}
		cs.plugin = p
	}
	return nil
}

// defaultChartSettings is the default style's charts block, which every
// page's settings start from.
var defaultChartSettings = sync.OnceValue(func() chartSettings {
	for _, p := range BuiltinPlugins() {
		if p.Name == reservedName {
			if cs := p.Styles[reservedName].Charts; cs != nil {
				return *cs
			}
		}
	}
	return chartSettings{Metrics: systemSansMetrics, Pad: 0.1, Width: 640}
})

// chartStyle stacks the chart settings of picks, in order, over the
// default style's. A block that names a font or metrics replaces how text
// is measured, padding included; any other key it sets replaces that key
// alone. A pad, 0 included, applies only while text is measured by
// metrics: a font's widths are exact.
func (c *PluginCache) chartStyle(picks []selected) chartSettings {
	out := defaultChartSettings()
	for _, s := range picks {
		b := s.style.Charts
		if b == nil {
			continue
		}
		switch {
		case b.Font != "":
			out.Font, out.plugin, out.measurer, out.Metrics, out.Pad = b.Font, b.plugin, b.measurer, "", 0
		case b.Metrics != "":
			out.Metrics, out.Pad, out.Font, out.plugin, out.measurer = b.Metrics, b.Pad, "", nil, nil
		}
		if b.padSet {
			out.Pad = b.Pad
		}
		if b.Width != 0 {
			out.Width = b.Width
		}
		if b.FontFamily != "" {
			out.FontFamily = b.FontFamily
		}
		// Tokens add up across the stack, so a style on top keeps the
		// roles of the one under it.
		for _, t := range b.Tokens {
			if !slices.Contains(out.Tokens, t) {
				out.Tokens = append(out.Tokens[:len(out.Tokens):len(out.Tokens)], t)
			}
		}
	}
	// Each style is held to the limit, but a stack can exceed it; the
	// tokens past it are unknown to a spec, which warns if it names one.
	if len(out.Tokens) > charts.MaxStyleTokens {
		out.Tokens = out.Tokens[:charts.MaxStyleTokens]
	}
	return out
}

// measure returns the measurer cs names: its font's, or the system-sans
// proxy with its padding.
func (cs chartSettings) measure() charts.Measurer {
	if cs.measurer != nil {
		return cs.measurer
	}
	return charts.SystemSans(cs.Pad)
}

// chartContext is what the fenced-code renderer needs to draw one page's
// charts. It belongs to one Convert call: n counts that page's charts, so
// pages converted in parallel never share a count.
type chartContext struct {
	style func() charts.Style          // built on the page's first chart
	load  func(string) (string, error) // nil: data urls are refused
	off   bool
	n     *int
}

// defaultTokens is the default stylesheet's light and dark token values,
// the colours a chart is written in under a stylesheet that defines none.
var defaultTokens = sync.OnceValues(func() (map[string]string, map[string]string) {
	light, dark, _ := charts.TokenValues(defaultCSS)
	return light, dark
})

func defaultLight() map[string]string { l, _ := defaultTokens(); return l }
func defaultDark() map[string]string  { _, d := defaultTokens(); return d }

// complete reports whether values holds every chart token.
func complete(values map[string]string) bool {
	for _, t := range charts.Tokens {
		if values[t] == "" {
			return false
		}
	}
	return true
}

func newChartContext(opt Options, st pageStyle) chartContext {
	ctx := chartContext{off: opt.NoCharts, n: new(int)}
	if opt.SourcePath != "" {
		dir := filepath.Dir(opt.SourcePath)
		root := opt.DataRoot
		if root == "" {
			root = dir
		}
		ctx.load = charts.DataLoader(root, dir)
	}
	// Reading the token values scans the whole stylesheet, fonts and all,
	// so it waits for a page that has a chart.
	ctx.style = sync.OnceValue(func() charts.Style {
		cs := st.charts
		s := charts.Style{Measurer: cs.measure(), FontFamily: cs.FontFamily, Width: cs.Width, Tokens: cs.Tokens}
		light, dark, _ := charts.TokenValues(st.css, cs.Tokens...)
		// A page or style token a theme block leaves unset still needs a
		// colour wherever the chart is drawn in literal colours: the
		// default's for a page token, --fg for one the style added.
		fill := func(values, defaults map[string]string) {
			for _, t := range charts.PageTokens {
				if values[t] == "" {
					values[t] = defaults[t]
				}
			}
			for _, t := range cs.Tokens {
				if values[t] == "" {
					values[t] = values["fg"]
				}
			}
		}
		if complete(light) && complete(dark) {
			fill(light, defaultLight())
			fill(dark, defaultDark())
			s.Light, s.Dark = light, dark
			return s
		}
		// Most stylesheets define --fg and --muted, so TokenValues finding
		// some tokens does not mean a var() for each would resolve; one
		// that does not would draw black. Write colours instead: the
		// default's, under the stylesheet's own.
		s.Literal = map[string]string{}
		for k, v := range defaultLight() {
			s.Literal[k] = v
		}
		for k, v := range light {
			s.Literal[k] = v
		}
		fill(s.Literal, defaultLight())
		return s
	})
	return ctx
}
