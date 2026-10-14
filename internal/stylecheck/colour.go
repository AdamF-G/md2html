package stylecheck

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// rgb is an opaque colour in sRGB, each channel 0 to 1.
type rgb [3]float64

// parsed is what reading one value gives: a colour the rules can measure,
// a colour they cannot, or something that is no colour at all.
type parsed int

const (
	measurable   parsed = iota
	translucent         // a colour with alpha below 1
	unmeasurable        // a colour form the check does not evaluate
	notColour
)

var funcRe = regexp.MustCompile(`^([a-z-]+)\((.*)\)$`)

// unmeasurableFuncs are colour functions the check recognises but does
// not evaluate.
var unmeasurableFuncs = map[string]bool{"color-mix": true, "light-dark": true, "color": true, "var": true}

// systemColours depend on the reader's browser and settings, so the check
// cannot measure them. The second list is CSS2's, deprecated but still
// drawn from the same settings.
var systemColours = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`canvas canvastext linktext visitedtext activetext buttonface
		buttontext buttonborder field fieldtext highlight highlighttext selecteditem
		selecteditemtext mark marktext graytext accentcolor accentcolortext

		activeborder activecaption appworkspace background buttonhighlight buttonshadow
		captiontext inactiveborder inactivecaption inactivecaptiontext infobackground
		infotext menu menutext scrollbar threeddarkshadow threedface threedhighlight
		threedlightshadow threedshadow window windowframe windowtext`) {
		systemColours[n] = true
	}
}

// parseColour reads a CSS colour value. A measurable colour is rounded to
// 8 bits a channel, so it measures as the same colour written in hex does,
// which is how the dataviz skill's validator reads it.
func parseColour(v string) (rgb, parsed) {
	c, kind := parseRaw(v)
	if kind == measurable {
		for i := range c {
			c[i] = math.Round(c[i]*255) / 255
		}
	}
	return c, kind
}

func parseRaw(v string) (rgb, parsed) {
	s := strings.ToLower(strings.TrimSpace(v))
	switch {
	case s == "currentcolor":
		return rgb{}, unmeasurable
	case s == "transparent":
		return rgb{}, translucent
	case strings.HasPrefix(s, "#"):
		return parseHex(s[1:])
	}
	if systemColours[s] {
		return rgb{}, unmeasurable
	}
	if hex, ok := namedColours[s]; ok {
		return parseHex(hex)
	}
	m := funcRe.FindStringSubmatch(s)
	if m == nil {
		return rgb{}, notColour
	}
	name, body := m[1], m[2]
	if unmeasurableFuncs[name] {
		return rgb{}, unmeasurable
	}
	switch name {
	case "rgb", "rgba", "hsl", "hsla", "oklch", "oklab":
	default:
		return rgb{}, notColour
	}
	if strings.HasPrefix(strings.TrimSpace(body), "from ") || strings.Contains(body, "var(") {
		return rgb{}, unmeasurable
	}
	args, alpha, ok := splitArgs(body)
	if !ok || len(args) != 3 {
		return rgb{}, notColour
	}
	if alpha != "" {
		a, ok := number(alpha, 1)
		if alpha == "none" { // a missing alpha is opaque
			a, ok = 1, true
		}
		if !ok {
			return rgb{}, notColour
		}
		if a < 1 {
			return rgb{}, translucent
		}
	}
	var c rgb
	switch name {
	case "rgb", "rgba":
		for i, a := range args {
			n, ok := number(a, 255)
			if !ok {
				return rgb{}, notColour
			}
			c[i] = clamp01(n / 255)
		}
	case "hsl", "hsla":
		h, ok1 := angle(args[0])
		sat, ok2 := number(args[1], 1)
		l, ok3 := number(args[2], 1)
		if !ok1 || !ok2 || !ok3 {
			return rgb{}, notColour
		}
		if strings.Contains(body, ",") {
			// The comma syntax needs the %; the space syntax takes a bare
			// number as a percentage.
			if !strings.HasSuffix(args[1], "%") || !strings.HasSuffix(args[2], "%") {
				return rgb{}, notColour
			}
		} else {
			sat, l = percent(args[1]), percent(args[2])
		}
		c = hslToRGB(h, clamp01(sat), clamp01(l))
	case "oklch":
		l, ok1 := number(args[0], 1)
		ch, ok2 := number(args[1], 0.4)
		h, ok3 := angle(args[2])
		if !ok1 || !ok2 || !ok3 {
			return rgb{}, notColour
		}
		rad := h * math.Pi / 180
		c = oklabToSRGB(l, ch*math.Cos(rad), ch*math.Sin(rad))
	case "oklab":
		l, ok1 := number(args[0], 1)
		a, ok2 := number(args[1], 0.4)
		b, ok3 := number(args[2], 0.4)
		if !ok1 || !ok2 || !ok3 {
			return rgb{}, notColour
		}
		c = oklabToSRGB(l, a, b)
	}
	return c, measurable
}

func parseHex(h string) (rgb, parsed) {
	switch len(h) {
	case 3, 4:
		var b strings.Builder
		for _, r := range h {
			b.WriteRune(r)
			b.WriteRune(r)
		}
		h = b.String()
	case 6, 8:
	default:
		return rgb{}, notColour
	}
	n, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return rgb{}, notColour
	}
	if len(h) == 8 {
		if n&0xff != 0xff {
			return rgb{}, translucent
		}
		n >>= 8
	}
	return rgb{float64(n>>16&0xff) / 255, float64(n>>8&0xff) / 255, float64(n&0xff) / 255}, measurable
}

// splitArgs splits a function body in either syntax, "a, b, c[, alpha]" or
// "a b c[ / alpha]", into its three channels and its alpha, if any.
func splitArgs(body string) (args []string, alpha string, ok bool) {
	if strings.Contains(body, ",") {
		for _, p := range strings.Split(body, ",") {
			args = append(args, strings.TrimSpace(p))
		}
		if len(args) == 4 {
			return args[:3], args[3], true
		}
		return args, "", len(args) == 3
	}
	main, a, slash := strings.Cut(body, "/")
	if slash {
		alpha = strings.TrimSpace(a)
		if alpha == "" {
			return nil, "", false
		}
	}
	return strings.Fields(main), alpha, true
}

// number reads a plain number, or a percentage of full.
func number(s string, full float64) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "none" { // a missing component is 0
		return 0, true
	}
	if p, ok := strings.CutSuffix(s, "%"); ok {
		n, err := strconv.ParseFloat(p, 64)
		return n / 100 * full, err == nil
	}
	n, err := strconv.ParseFloat(s, 64)
	return n, err == nil
}

// percent reads a number as a percentage, with the % or without, as the
// space syntax of hsl() allows: 0 to 1.
func percent(s string) float64 {
	n, _ := number(strings.TrimSuffix(strings.TrimSpace(s), "%"), 1)
	return n / 100
}

// angle reads a hue in degrees, bare or with a deg, grad, rad or turn
// unit, wrapped into [0, 360).
func angle(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "none" {
		return 0, true
	}
	scale := 1.0
	for _, u := range []struct {
		suffix string
		scale  float64
	}{{"deg", 1}, {"grad", 0.9}, {"rad", 180 / math.Pi}, {"turn", 360}} {
		if t, ok := strings.CutSuffix(s, u.suffix); ok {
			s, scale = t, u.scale
			break
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(n, 0) || math.IsNaN(n) { // no hue to wrap
		return 0, false
	}
	return math.Mod(math.Mod(n*scale, 360)+360, 360), true
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func hslToRGB(h, s, l float64) rgb {
	f := func(n float64) float64 {
		k := math.Mod(n+h/30, 12)
		a := s * math.Min(l, 1-l)
		return l - a*math.Max(-1, math.Min(k-3, math.Min(9-k, 1)))
	}
	return rgb{f(0), f(8), f(4)}
}

// namedColours maps each CSS named colour to its hex value.
var namedColours = func() map[string]string {
	m := map[string]string{}
	f := strings.Fields(namedColourList)
	for i := 0; i+1 < len(f); i += 2 {
		m[f[i]] = f[i+1]
	}
	return m
}()

const namedColourList = `aliceblue f0f8ff antiquewhite faebd7 aqua 00ffff aquamarine 7fffd4
azure f0ffff beige f5f5dc bisque ffe4c4 black 000000 blanchedalmond ffebcd blue 0000ff
blueviolet 8a2be2 brown a52a2a burlywood deb887 cadetblue 5f9ea0 chartreuse 7fff00
chocolate d2691e coral ff7f50 cornflowerblue 6495ed cornsilk fff8dc crimson dc143c
cyan 00ffff darkblue 00008b darkcyan 008b8b darkgoldenrod b8860b darkgray a9a9a9
darkgreen 006400 darkgrey a9a9a9 darkkhaki bdb76b darkmagenta 8b008b darkolivegreen 556b2f
darkorange ff8c00 darkorchid 9932cc darkred 8b0000 darksalmon e9967a darkseagreen 8fbc8f
darkslateblue 483d8b darkslategray 2f4f4f darkslategrey 2f4f4f darkturquoise 00ced1
darkviolet 9400d3 deeppink ff1493 deepskyblue 00bfff dimgray 696969 dimgrey 696969
dodgerblue 1e90ff firebrick b22222 floralwhite fffaf0 forestgreen 228b22 fuchsia ff00ff
gainsboro dcdcdc ghostwhite f8f8ff gold ffd700 goldenrod daa520 gray 808080 green 008000
greenyellow adff2f grey 808080 honeydew f0fff0 hotpink ff69b4 indianred cd5c5c indigo 4b0082
ivory fffff0 khaki f0e68c lavender e6e6fa lavenderblush fff0f5 lawngreen 7cfc00
lemonchiffon fffacd lightblue add8e6 lightcoral f08080 lightcyan e0ffff
lightgoldenrodyellow fafad2 lightgray d3d3d3 lightgreen 90ee90 lightgrey d3d3d3
lightpink ffb6c1 lightsalmon ffa07a lightseagreen 20b2aa lightskyblue 87cefa
lightslategray 778899 lightslategrey 778899 lightsteelblue b0c4de lightyellow ffffe0
lime 00ff00 limegreen 32cd32 linen faf0e6 magenta ff00ff maroon 800000
mediumaquamarine 66cdaa mediumblue 0000cd mediumorchid ba55d3 mediumpurple 9370db
mediumseagreen 3cb371 mediumslateblue 7b68ee mediumspringgreen 00fa9a
mediumturquoise 48d1cc mediumvioletred c71585 midnightblue 191970 mintcream f5fffa
mistyrose ffe4e1 moccasin ffe4b5 navajowhite ffdead navy 000080 oldlace fdf5e6 olive 808000
olivedrab 6b8e23 orange ffa500 orangered ff4500 orchid da70d6 palegoldenrod eee8aa
palegreen 98fb98 paleturquoise afeeee palevioletred db7093 papayawhip ffefd5
peachpuff ffdab9 peru cd853f pink ffc0cb plum dda0dd powderblue b0e0e6 purple 800080
rebeccapurple 663399 red ff0000 rosybrown bc8f8f royalblue 4169e1 saddlebrown 8b4513
salmon fa8072 sandybrown f4a460 seagreen 2e8b57 seashell fff5ee sienna a0522d silver c0c0c0
skyblue 87ceeb slateblue 6a5acd slategray 708090 slategrey 708090 snow fffafa
springgreen 00ff7f steelblue 4682b4 tan d2b48c teal 008080 thistle d8bfd8 tomato ff6347
turquoise 40e0d0 violet ee82ee wheat f5deb3 white ffffff whitesmoke f5f5f5 yellow ffff00
yellowgreen 9acd32`
