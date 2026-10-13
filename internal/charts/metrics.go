package charts

import (
	"bytes"

	"github.com/tdewolff/font"
)

type fontMeasurer struct{ f *font.SFNT }

// NewFontMeasurer reads woff2, TrueType or OpenType bytes. A variable
// font's default instance gives the advances; heavier text is widened by a
// fixed factor per 100 of weight, which kept the spike within 1 px of the
// browser. Applying the wght axis through HVAR is a follow-up.
func NewFontMeasurer(b []byte) (Measurer, error) {
	if bytes.HasPrefix(b, []byte("wOF2")) {
		raw, err := font.ParseWOFF2(b)
		if err != nil {
			return nil, err
		}
		b = raw
	}
	f, err := font.ParseSFNT(b, 0)
	if err != nil {
		return nil, err
	}
	return fontMeasurer{f}, nil
}

func (m fontMeasurer) Width(s string, size, weight float64) float64 {
	var w float64
	var prev uint16
	for i, r := range s {
		g := m.f.GlyphIndex(r)
		if i > 0 {
			w += float64(m.f.Kerning(prev, g))
		}
		w += float64(m.f.GlyphAdvance(g))
		prev = g
	}
	return w * size / float64(m.f.UnitsPerEm()) * (1 + (weight-400)/100*0.018)
}
