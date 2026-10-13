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
// rest the id <prefix>-m<i>, a tab stop and the md2html-mark class. It
// returns how many elements it tagged and how many it walked.
func tagMarks(svg, prefix string, untipped []int) (string, int, int) {
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
		attrs := fmt.Sprintf(` id="%s-m%d" tabindex="0" class="md2html-mark"`, prefix, n)
		b.WriteString(svg[last:loc[0]])
		b.WriteString(strings.Replace(tag, "<"+name, "<"+name+attrs, 1))
		last = loc[1]
		n++
	}
	b.WriteString(svg[last:])
	return b.String(), n, seen
}

// hoverLayer namespaces Vega's ids, tags each tooltip mark, and appends a
// box per mark that css shows while the mark is hovered or focused. If the
// marks in the SVG do not match r.Marks it returns the SVG with only its
// ids namespaced and no css, since wrong ids would show the wrong boxes.
func hoverLayer(svg string, r rendered, prefix string, m Measurer) (out string, css string) {
	svg = namespaceIDs(svg, prefix)
	if len(r.Marks) == 0 {
		return svg, ""
	}
	tagged, n, seen := tagMarks(svg, prefix, r.Untipped)
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
	for i, b := range placeBoxes(r, m) {
		fmt.Fprintf(&g, `<g class="md2html-tip" id="%s-t%d" opacity="0" transform="translate(%s,%s)">`, prefix, i, num(b.X), num(b.Y))
		fmt.Fprintf(&g, `<rect width="%s" height="%s" rx="4" style="fill: var(--chart-surface); stroke: var(--rule)"/>`, num(b.W), num(b.H))
		for j, line := range r.Marks[i].Tooltip {
			fmt.Fprintf(&g, `<text x="%d" y="%d" style="fill: var(--fg)">%s</text>`, tipPadX, tipBaseline+j*tipLine, html.EscapeString(line))
		}
		g.WriteString("</g>")
		fmt.Fprintf(&c, ".md2html-chart:has(#%[1]s-m%[2]d:hover, #%[1]s-m%[2]d:focus) #%[1]s-t%[2]d { opacity: 1 }\n", prefix, i)
	}
	g.WriteString("</g>")
	return tagged[:end] + g.String() + tagged[end:], c.String()
}

// num writes a coordinate to one decimal place, without trailing zeros.
func num(v float64) string {
	return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64)
}
