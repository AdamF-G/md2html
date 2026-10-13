package charts

import (
	"math"
	"os"
	"regexp"
	"strconv"
	"testing"
)

func plex(t *testing.T) Measurer {
	t.Helper()
	b, err := os.ReadFile("testdata/ibm-plex-sans-400-600.woff2")
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewFontMeasurer(b)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

var firstTranslate = regexp.MustCompile(`<g [^>]*transform="translate\(([0-9.]+),`)

func labelColumn(svg string) float64 {
	m := firstTranslate.FindStringSubmatch(svg)
	if m == nil {
		return -1
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return v
}

func TestFontMeasurerReadsWOFF2Advances(t *testing.T) {
	m := plex(t)
	for _, c := range []struct {
		s    string
		want float64
	}{{"Reference 200 W", 93.13}, {"30 s buckets", 68.69}} {
		if got := m.Width(c.s, 12, 400); math.Abs(got-c.want) > 0.05 {
			t.Errorf("%q at 12px: got %.2f, want %.2f", c.s, got, c.want)
		}
	}
}

// Real metrics change layout: the label column of the spike's dot plot is
// 109 px with Plex, against 159 px from Vega's own estimate. The browser
// laid it out at 108.
func TestRealMetricsSetTheLabelColumn(t *testing.T) {
	spec := `{"width":480,"padding":0,"data":{"values":[{"l":"T1 baseline","v":1},{"l":"Reference 200 W","v":1.8}]},
	 "mark":"point","encoding":{"y":{"field":"l","type":"nominal","title":null,"axis":{"labelPadding":10,"labelFontSize":12}},
	 "x":{"field":"v","type":"quantitative"}}}`
	r, err := renderSpec([]byte(spec), plex(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	off := labelColumn(r.SVG)
	t.Logf("label column %v px", off)
	if off < 104 || off > 112 {
		t.Errorf("label column %v px; the browser lays this out near 108", off)
	}
}

// The writer's stdout must stay clean: the deprecated ParseFont prints.
func TestFontMeasurerPrintsNothing(t *testing.T) {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	plex(t)
	w.Close()
	os.Stdout = old
	buf := make([]byte, 64)
	if n, _ := r.Read(buf); n > 0 {
		t.Errorf("measurer wrote to stdout: %q", buf[:n])
	}
}
