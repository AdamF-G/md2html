package stylecheck

import "math"

// The colour maths of the dataviz skill's validate_palette.js, ported
// line for line: WCAG contrast, OKLab and OKLCH, and the Machado,
// Oliveira and Fernandes (2009) colour-vision simulation at severity 1.0.
// Its thresholds are calibrated to exactly these formulas, so a change
// here is a change to every rule.

func toLinear(s float64) float64 {
	if s <= 0.04045 {
		return s / 12.92
	}
	return math.Pow((s+0.055)/1.055, 2.4)
}

func toSRGB(l float64) float64 {
	if l <= 0.0031308 {
		return 12.92 * l
	}
	return 1.055*math.Pow(l, 1/2.4) - 0.055
}

func (c rgb) linear() [3]float64 {
	return [3]float64{toLinear(c[0]), toLinear(c[1]), toLinear(c[2])}
}

func luminance(c rgb) float64 {
	l := c.linear()
	return 0.2126*l[0] + 0.7152*l[1] + 0.0722*l[2]
}

// contrast is the WCAG contrast ratio of two colours, 1 to 21.
func contrast(a, b rgb) float64 {
	x, y := luminance(a), luminance(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

func oklabFromLinear(l [3]float64) [3]float64 {
	r, g, b := l[0], l[1], l[2]
	lc := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	mc := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	sc := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return [3]float64{
		0.2104542553*lc + 0.7936177850*mc - 0.0040720468*sc,
		1.9779984951*lc - 2.4285922050*mc + 0.4505937099*sc,
		0.0259040371*lc + 0.7827717662*mc - 0.8086757660*sc,
	}
}

// oklabToSRGB converts an OKLab colour to sRGB, clipped to the gamut.
func oklabToSRGB(L, a, b float64) rgb {
	l := math.Pow(L+0.3963377774*a+0.2158037573*b, 3)
	m := math.Pow(L-0.1055613458*a-0.0638541728*b, 3)
	s := math.Pow(L-0.0894841775*a-1.2914855480*b, 3)
	lin := [3]float64{
		4.0767416621*l - 3.3077115913*m + 0.2309699292*s,
		-1.2684380046*l + 2.6097574011*m - 0.3413193965*s,
		-0.0041960863*l - 0.7034186147*m + 1.7076147010*s,
	}
	return rgb{clamp01(toSRGB(clamp01(lin[0]))), clamp01(toSRGB(clamp01(lin[1]))), clamp01(toSRGB(clamp01(lin[2])))}
}

// lightnessChroma is a colour's OKLCH L and C.
func lightnessChroma(c rgb) (float64, float64) {
	lab := oklabFromLinear(c.linear())
	return lab[0], math.Hypot(lab[1], lab[2])
}

type vision int

const (
	normal vision = iota
	protan
	deutan
)

var machado = map[vision][3][3]float64{
	protan: {{0.152286, 1.052583, -0.204868}, {0.114503, 0.786281, 0.099216}, {-0.003882, -0.048116, 1.051998}},
	deutan: {{0.367322, 0.860646, -0.227968}, {0.280085, 0.672501, 0.047413}, {-0.011820, 0.042940, 0.968881}},
}

func simulate(c rgb, v vision) [3]float64 {
	l := c.linear()
	if v == normal {
		return l
	}
	m := machado[v]
	var out [3]float64
	for i := range out {
		out[i] = clamp01(m[i][0]*l[0] + m[i][1]*l[1] + m[i][2]*l[2])
	}
	return out
}

// deltaE is the distance between two colours in OKLab, ×100, as a reader
// with the given vision sees them.
func deltaE(a, b rgb, v vision) float64 {
	x, y := oklabFromLinear(simulate(a, v)), oklabFromLinear(simulate(b, v))
	return 100 * math.Sqrt((x[0]-y[0])*(x[0]-y[0])+(x[1]-y[1])*(x[1]-y[1])+(x[2]-y[2])*(x[2]-y[2]))
}

// colourBlind is the smaller of the protan and deutan distances.
func colourBlind(a, b rgb) float64 {
	return math.Min(deltaE(a, b, protan), deltaE(a, b, deutan))
}
