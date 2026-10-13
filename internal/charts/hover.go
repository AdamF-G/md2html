package charts

import (
	"fmt"
	"html"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// boxPlace is where one tooltip box sits, in SVG px.
type boxPlace struct{ X, Y, W, H float64 }

const (
	tipSize     = 12 // font size of tooltip text, px
	tipLine     = 16 // line height, px
	tipPadX     = 10 // horizontal padding on each side
	tipPadY     = 5  // vertical padding above and below
	tipGap      = 8  // space between a mark and its box
	tipMargin   = 2  // nearest a box may come to the chart's edge
	tipBaseline = 17 // baseline of the first line, from the box's top
)

// placeBoxes sizes each mark's box to its text and puts it above the mark,
// centred on it. With no room above it goes below, and it shifts sideways
// to stay inside the chart.
func placeBoxes(r rendered, m Measurer) []boxPlace {
	out := make([]boxPlace, len(r.Marks))
	for i, mk := range r.Marks {
		w := 0.0
		for _, line := range mk.Tooltip {
			w = math.Max(w, m.Width(line, tipSize, 400))
		}
		b := boxPlace{W: w + 2*tipPadX, H: float64(len(mk.Tooltip)*tipLine + 2*tipPadY)}
		b.X = (mk.X1+mk.X2)/2 - b.W/2
		b.Y = mk.Y1 - tipGap - b.H
		if b.Y < tipMargin {
			b.Y = mk.Y2 + tipGap
		}
		b.X = math.Max(tipMargin, math.Min(b.X, r.Width-tipMargin-b.W))
		out[i] = b
	}
	return out
}

var (
	svgTag  = regexp.MustCompile(`<(/?)([A-Za-z][A-Za-z0-9:-]*)[^>]*?(/?)>`)
	attrRe  = regexp.MustCompile(`(\s)([A-Za-z_:][-A-Za-z0-9_:.]*)="([^"]*)"`)
	urlRef  = regexp.MustCompile(`url\(#([^)]*)\)`)
	classRe = regexp.MustCompile(`\sclass="([^"]*)"`)
	fontRe  = regexp.MustCompile(`\sfont-family="([^"]*)"`)
)

// refAttrs are the attributes whose values may hold url(#id).
var refAttrs = map[string]bool{"clip-path": true, "fill": true, "stroke": true, "mask": true, "filter": true, "style": true}

// namespaceIDs prefixes every id Vega wrote, and every reference to one,
// so two charts on a page never share an id. It reads only start tags and,
// in them, only the id attribute, href and xlink:href values naming a
// fragment, and url(#…) in reference attributes. Text content and every
// other attribute, aria-label included, are author text and left alone.
func namespaceIDs(svg, prefix string) string {
	return svgTag.ReplaceAllStringFunc(svg, func(tag string) string {
		if strings.HasPrefix(tag, "</") {
			return tag
		}
		return attrRe.ReplaceAllStringFunc(tag, func(a string) string {
			m := attrRe.FindStringSubmatch(a)
			name, v := m[2], m[3]
			switch {
			case name == "id":
				v = prefix + "-" + v
			case (name == "href" || name == "xlink:href") && strings.HasPrefix(v, "#"):
				v = "#" + prefix + "-" + v[1:]
			case refAttrs[name]:
				v = urlRef.ReplaceAllStringFunc(v, func(u string) string {
					return "url(#" + prefix + "-" + u[len("url(#"):]
				})
			default:
				return a
			}
			return m[1] + name + `="` + v + `"`
		})
	})
}

// tooltipGroup reports whether a group's class marks it as a data mark
// whose items Vega draws one element each: the kinds the driver counts.
func tooltipGroup(class string) bool {
	role, kind := false, false
	for _, c := range strings.Fields(class) {
		switch c {
		case "role-mark":
			role = true
		case "mark-symbol", "mark-rect", "mark-arc", "mark-rule", "mark-text":
			kind = true
		}
	}
	return role && kind
}

// tagMarks walks the elements of data marks drawn one per item, in SVG
// order, skips those whose place is in untipped, and gives the i-th of the
// rest the id <prefix>-m<i> and, if tabStops, a tab stop. (Its classes
// come from classifyMarks.) It returns how many elements it tagged and how
// many it walked.
func tagMarks(svg, prefix string, untipped []int, tabStops bool) (string, int, int) {
	skip := make(map[int]bool, len(untipped))
	for _, u := range untipped {
		skip[u] = true
	}
	var b strings.Builder
	var groups []string // class of each open <g>
	n, seen, last := 0, 0, 0
	for _, loc := range svgTag.FindAllStringSubmatchIndex(svg, -1) {
		tag := svg[loc[0]:loc[1]]
		closing := loc[3] > loc[2]
		name := svg[loc[4]:loc[5]]
		selfClosing := loc[7] > loc[6]
		if name == "g" {
			switch {
			case closing:
				if len(groups) > 0 {
					groups = groups[:len(groups)-1]
				}
			case !selfClosing:
				class := ""
				if m := classRe.FindStringSubmatch(tag); m != nil {
					class = m[1]
				}
				groups = append(groups, class)
			}
			continue
		}
		if closing || len(groups) == 0 || !tooltipGroup(groups[len(groups)-1]) {
			continue
		}
		switch name {
		case "path", "line", "rect", "text":
		default:
			continue
		}
		seen++
		if skip[seen-1] {
			continue
		}
		attrs := fmt.Sprintf(` id="%s-m%d"`, prefix, n)
		if tabStops {
			attrs += ` tabindex="0"`
		}
		b.WriteString(svg[last:loc[0]])
		b.WriteString(strings.Replace(tag, "<"+name, "<"+name+attrs, 1))
		last = loc[1]
		n++
	}
	b.WriteString(svg[last:])
	return b.String(), n, seen
}

// maxTabStops is the most tooltip marks a chart makes tab stops of; above
// it the figure is the one tab stop. maxHitRegions is the most points that
// get nearest-point regions, since computing them is O(n²).
const (
	maxTabStops   = 50
	maxHitRegions = 500
)

// hoverLayer namespaces Vega's ids, classes every data mark and legend
// entry for legend highlighting (with or without tooltips), tags each
// tooltip mark, and appends a box per tooltip mark that css shows while the
// mark is hovered or focused. When every tooltip mark is a point it also
// draws each point's nearest-point region, which shows the same box. If the
// tooltip marks in the SVG do not match r.Marks it returns the SVG with no
// ids, boxes or css, since wrong ids would show the wrong boxes.
func hoverLayer(svg string, r rendered, prefix string, m Measurer) (out string, css string) {
	svg = tagLegend(classifyMarks(namespaceIDs(svg, prefix)))
	if len(r.Marks) == 0 {
		return svg, ""
	}
	tagged, n, seen := tagMarks(svg, prefix, r.Untipped, len(r.Marks) <= maxTabStops)
	if n != len(r.Marks) || seen != len(r.Marks)+len(r.Untipped) {
		return svg, ""
	}
	end := strings.LastIndex(tagged, "</svg>")
	if end < 0 {
		return svg, ""
	}
	var g, c strings.Builder
	g.WriteString(`<g class="md2html-tips" aria-hidden="true" pointer-events="none" font-size="12px"`)
	if f := fontRe.FindStringSubmatch(tagged); f != nil {
		g.WriteString(` font-family="` + f[1] + `"`)
	}
	g.WriteString(">")
	cells := hitCells(tagged, r)
	for i, cell := range cells {
		if len(cell) < 3 {
			continue
		}
		fmt.Fprintf(&g, `<path class="md2html-hit" id="%s-v%d" d="%s" style="fill: transparent" pointer-events="all"/>`, prefix, i, polygonPath(cell))
	}
	for i, b := range placeBoxes(r, m) {
		fmt.Fprintf(&g, `<g class="md2html-tip" id="%s-t%d" opacity="0" transform="translate(%s,%s)">`, prefix, i, num(b.X), num(b.Y))
		fmt.Fprintf(&g, `<rect width="%s" height="%s" rx="4" style="fill: var(--chart-surface); stroke: var(--rule)"/>`, num(b.W), num(b.H))
		for j, line := range r.Marks[i].Tooltip {
			fmt.Fprintf(&g, `<text x="%d" y="%d" style="fill: var(--fg)">%s</text>`, tipPadX, tipBaseline+j*tipLine, html.EscapeString(line))
		}
		g.WriteString("</g>")
		hit := ""
		if cells != nil && len(cells[i]) >= 3 {
			hit = fmt.Sprintf(", #%s-v%d:hover", prefix, i)
		}
		fmt.Fprintf(&c, ".md2html-chart:has(#%[1]s-m%[2]d:hover, #%[1]s-m%[2]d:focus%[3]s) #%[1]s-t%[2]d { opacity: 1 }\n", prefix, i, hit)
	}
	g.WriteString("</g>")
	return tagged[:end] + g.String() + tagged[end:], c.String()
}

// hitCells returns each mark's nearest-point region, clipped to the plot
// area, or nil when the chart gets none: when a tooltip mark is not a
// point (a region over it would hide its own hover), when there are more
// than maxHitRegions points, or when the plot area cannot be found.
func hitCells(svg string, r rendered) [][][2]float64 {
	if len(r.Marks) > maxHitRegions {
		return nil
	}
	sites := make([][2]float64, len(r.Marks))
	for i, mk := range r.Marks {
		if !mk.Symbol {
			return nil
		}
		sites[i] = [2]float64{(mk.X1 + mk.X2) / 2, (mk.Y1 + mk.Y2) / 2}
	}
	x0, y0, w, h, ok := plotArea(svg)
	if !ok {
		return nil
	}
	return voronoiCells(sites, x0, y0, x0+w, y0+h)
}

var (
	translateRe = regexp.MustCompile(`\stransform="translate\(([-0-9.e]+),\s*([-0-9.e]+)\)"`)
	backgroundD = regexp.MustCompile(`\sd="M([-0-9.e]+),([-0-9.e]+)h([0-9.e]+)v([0-9.e]+)h`)
)

// plotArea finds the first role-frame group's background rectangle, in
// absolute SVG px: the translates of every group enclosing it summed, and
// its corner and size from its path's M, h and v.
func plotArea(svg string) (x, y, w, h float64, ok bool) {
	type open struct{ dx, dy float64 }
	var stack []open
	inFrame := false
	for _, loc := range svgTag.FindAllStringSubmatchIndex(svg, -1) {
		tag := svg[loc[0]:loc[1]]
		closing := loc[3] > loc[2]
		name := svg[loc[4]:loc[5]]
		selfClosing := loc[7] > loc[6]
		if name == "g" && closing {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			continue
		}
		if name == "g" && !selfClosing {
			var o open
			if t := translateRe.FindStringSubmatch(tag); t != nil {
				o.dx, _ = strconv.ParseFloat(t[1], 64)
				o.dy, _ = strconv.ParseFloat(t[2], 64)
			}
			stack = append(stack, o)
			if m := classRe.FindStringSubmatch(tag); m != nil && hasClass(m[1], "role-frame") {
				inFrame = true
			}
			continue
		}
		if !inFrame || name != "path" {
			continue
		}
		m := classRe.FindStringSubmatch(tag)
		if m == nil || !hasClass(m[1], "background") {
			continue
		}
		d := backgroundD.FindStringSubmatch(tag)
		if d == nil {
			return 0, 0, 0, 0, false
		}
		x, _ = strconv.ParseFloat(d[1], 64)
		y, _ = strconv.ParseFloat(d[2], 64)
		for _, o := range stack {
			x += o.dx
			y += o.dy
		}
		w, _ = strconv.ParseFloat(d[3], 64)
		h, _ = strconv.ParseFloat(d[4], 64)
		return x, y, w, h, w > 0 && h > 0
	}
	return 0, 0, 0, 0, false
}

var (
	seriesFill   = regexp.MustCompile(`\bfill:\s*var\(--series-(\d+)\)`)
	seriesStroke = regexp.MustCompile(`\bstroke:\s*var\(--series-(\d+)\)`)
)

// seriesOf returns n when the start tag is filled with var(--series-n), or,
// with no series fill, stroked with it (a line, an unfilled point).
func seriesOf(tag string) string {
	if m := seriesFill.FindStringSubmatch(tag); m != nil {
		return m[1]
	}
	if m := seriesStroke.FindStringSubmatch(tag); m != nil {
		return m[1]
	}
	return ""
}

// classifyMarks gives every element of every data mark (a direct child of
// a role-mark group: points, bars, lines, areas, rules, text) the class
// md2html-mark, and md2html-s<n> when it is drawn in --series-n, whether
// or not it has a tooltip, so a legend can dim the other series.
func classifyMarks(svg string) string {
	var b strings.Builder
	var groups []string // class of each open <g>
	last := 0
	for _, loc := range svgTag.FindAllStringSubmatchIndex(svg, -1) {
		tag := svg[loc[0]:loc[1]]
		closing := loc[3] > loc[2]
		name := svg[loc[4]:loc[5]]
		selfClosing := loc[7] > loc[6]
		if name == "g" {
			switch {
			case closing:
				if len(groups) > 0 {
					groups = groups[:len(groups)-1]
				}
			case !selfClosing:
				class := ""
				if m := classRe.FindStringSubmatch(tag); m != nil {
					class = m[1]
				}
				groups = append(groups, class)
			}
			continue
		}
		if closing || len(groups) == 0 || !hasClass(groups[len(groups)-1], "role-mark") {
			continue
		}
		switch name {
		case "path", "line", "rect", "text", "image":
		default:
			continue
		}
		if m := classRe.FindStringSubmatch(tag); m != nil && (hasClass(m[1], "background") || hasClass(m[1], "foreground")) {
			continue
		}
		classes := "md2html-mark"
		if n := seriesOf(tag); n != "" {
			classes += " md2html-s" + n
		}
		b.WriteString(svg[last:loc[0]])
		b.WriteString(addClass(tag, classes))
		last = loc[1]
	}
	b.WriteString(svg[last:])
	return b.String()
}

// tagLegend gives each legend entry the class md2html-legend-s<n>, from
// the var(--series-n) fill (or, failing that, stroke) of its legend symbol,
// and a tab stop, so it can be hovered or focused. Vega draws a legend's
// entries as the item groups (the direct child <g>s) of a role-scope group
// inside its role-legend-entry group. Each legend is judged alone: one
// whose entries do not each name a different series (a gradient, a size
// or shape legend) gets no classes, and costs no other legend its own.
func tagLegend(svg string) string {
	type entry struct {
		at     [2]int // the entry group's start tag
		legend int    // which role-legend group holds it, from 1
		series string
	}
	type open struct {
		class  string
		legend int // set on a role-legend group
	}
	var entries []entry
	var groups []open
	legends := 0
	cur := -1     // index in entries of the open entry, if any
	curDepth := 0 // len(groups) while inside it
	for _, loc := range svgTag.FindAllStringSubmatchIndex(svg, -1) {
		tag := svg[loc[0]:loc[1]]
		closing := loc[3] > loc[2]
		name := svg[loc[4]:loc[5]]
		selfClosing := loc[7] > loc[6]
		if name == "g" && closing {
			if len(groups) > 0 {
				groups = groups[:len(groups)-1]
			}
			if len(groups) < curDepth {
				cur, curDepth = -1, 0
			}
			continue
		}
		if name == "g" && !selfClosing {
			o := open{}
			if m := classRe.FindStringSubmatch(tag); m != nil {
				o.class = m[1]
			}
			if hasClass(o.class, "role-legend") {
				legends++
				o.legend = legends
			}
			if cur < 0 && len(groups) > 0 && hasClass(groups[len(groups)-1].class, "role-scope") {
				legend, inEntry := 0, false
				for i := len(groups) - 1; i >= 0; i-- {
					if hasClass(groups[i].class, "role-legend-entry") {
						inEntry = true
					}
					if groups[i].legend > 0 {
						legend = groups[i].legend
						break
					}
				}
				if inEntry && legend > 0 {
					entries = append(entries, entry{at: [2]int{loc[0], loc[1]}, legend: legend})
					cur, curDepth = len(entries)-1, len(groups)+1
				}
			}
			groups = append(groups, o)
			continue
		}
		if cur >= 0 && entries[cur].series == "" && len(groups) > 0 && hasClass(groups[len(groups)-1].class, "role-legend-symbol") {
			entries[cur].series = seriesOf(tag)
		}
	}
	bad := map[int]bool{}
	seen := map[int]map[string]bool{}
	for _, e := range entries {
		if seen[e.legend] == nil {
			seen[e.legend] = map[string]bool{}
		}
		if e.series == "" || seen[e.legend][e.series] {
			bad[e.legend] = true
		}
		seen[e.legend][e.series] = true
	}
	var b strings.Builder
	last := 0
	for _, e := range entries {
		if bad[e.legend] {
			continue
		}
		tag := addClass(svg[e.at[0]:e.at[1]], "md2html-legend-s"+e.series)
		if loc := classRe.FindStringIndex(tag); loc != nil && !strings.Contains(tag, "tabindex=") {
			tag = tag[:loc[1]] + ` tabindex="0"` + tag[loc[1]:]
		}
		b.WriteString(svg[last:e.at[0]])
		b.WriteString(tag)
		last = e.at[1]
	}
	b.WriteString(svg[last:])
	return b.String()
}

func hasClass(list, c string) bool {
	for _, f := range strings.Fields(list) {
		if f == c {
			return true
		}
	}
	return false
}

// addClass adds classes to a start tag, into its class attribute when it
// has one, so an element never carries two. Classes it already has are
// not repeated.
func addClass(tag, classes string) string {
	if loc := classRe.FindStringSubmatchIndex(tag); loc != nil {
		have := tag[loc[2]:loc[3]]
		var add []string
		for _, c := range strings.Fields(classes) {
			if !hasClass(have, c) {
				add = append(add, c)
			}
		}
		if len(add) == 0 {
			return tag
		}
		return tag[:loc[3]] + " " + strings.Join(add, " ") + tag[loc[3]:]
	}
	name := svgTag.FindStringSubmatch(tag)[2]
	return strings.Replace(tag, "<"+name, "<"+name+` class="`+classes+`"`, 1)
}

// polygonPath writes a polygon as path data.
func polygonPath(poly [][2]float64) string {
	var b strings.Builder
	for i, v := range poly {
		if i == 0 {
			b.WriteString("M")
		} else {
			b.WriteString("L")
		}
		b.WriteString(num(v[0]) + "," + num(v[1]))
	}
	b.WriteString("Z")
	return b.String()
}

// num writes a coordinate to one decimal place, without trailing zeros.
func num(v float64) string {
	return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64)
}
