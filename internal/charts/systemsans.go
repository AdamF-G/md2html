package charts

type systemSans struct{ pad float64 }

// SystemSans measures text set in the reader's system sans, which md2html
// cannot know when it builds the page, using Inter's advances as a proxy
// and widening every width by pad (0.1 is ten percent), so a wider system
// face still fits.
func SystemSans(pad float64) Measurer { return systemSans{pad} }

func (m systemSans) Width(s string, size, weight float64) float64 {
	fallback := float64(systemSansAdvance['n'-0x20])
	var units float64
	for _, r := range s {
		if r >= 0x20 && int(r-0x20) < len(systemSansAdvance) {
			units += float64(systemSansAdvance[r-0x20])
		} else {
			units += fallback
		}
	}
	return units * size / systemSansUPM * (1 + (weight-400)/100*0.018) * (1 + m.pad)
}
