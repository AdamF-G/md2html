package md2html

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/AdamF-G/md2html/internal/charts"
)

// systemSansMetrics names the built-in proxy widths for a style whose
// chart text renders in the reader's system sans.
const systemSansMetrics = "builtin:system-sans"

// chartSettings is a style's charts block, and, once a page's styles are
// stacked, the settings its charts are drawn with.
type chartSettings struct {
	Font, Metrics string  // a font file in the plugin, or the built-in proxy; never both
	Pad           float64 // widen proxy-measured text by this fraction
	padSet        bool    // the block sets pad, 0 included
	Width         int     // plot width in px when a spec sets none
	FontFamily    string  // the CSS stack chart text renders in
	plugin        *Plugin // the plugin Font is in

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

// defaultLight is the default stylesheet's light token values, the
// colours a chart is written in under a stylesheet that defines none.
var defaultLight = sync.OnceValue(func() map[string]string {
	light, _, _ := charts.TokenValues(defaultCSS)
	return light
})

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
		s := charts.Style{Measurer: cs.measure(), FontFamily: cs.FontFamily, Width: cs.Width}
		light, dark, _ := charts.TokenValues(st.css)
		if complete(light) && complete(dark) {
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
		return s
	})
	return ctx
}
