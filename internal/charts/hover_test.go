package charts

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestHoverLayerGivesEveryTooltipMarkABox(t *testing.T) {
	r, _ := renderSpec([]byte(dotSpec), fixed{}, nil)
	out, css := hoverLayer(r.SVG, r, "md2html-c1", fixed{})
	for i := range r.Marks {
		id := fmt.Sprintf("md2html-c1-m%d", i)
		if !strings.Contains(out, `id="`+id+`"`) || !strings.Contains(out, fmt.Sprintf(`id="md2html-c1-t%d"`, i)) {
			t.Errorf("mark %d lacks its id or box", i)
		}
		if !strings.Contains(css, "#"+id+":hover") || !strings.Contains(css, "#"+id+":focus") {
			t.Errorf("css does not show box %d on hover and focus", i)
		}
	}
	if strings.Index(out, `class="md2html-tips"`) < strings.LastIndex(out, `md2html-c1-m`) {
		t.Error("boxes are not drawn after every mark")
	}
}

func TestHoverBoxesStayInsideTheChart(t *testing.T) {
	// A mark at the top edge flips its box below; one at the right edge shifts left.
	r := rendered{Width: 300, Height: 200, Marks: []mark{
		{Tooltip: []string{"top"}, X1: 140, Y1: 2, X2: 150, Y2: 12},
		{Tooltip: []string{"a long tooltip at the right edge"}, X1: 290, Y1: 100, X2: 298, Y2: 108},
	}}
	for i, b := range placeBoxes(r, fixed{}) {
		if b.X < 0 || b.X+b.W > r.Width || b.Y < 0 || b.Y+b.H > r.Height+40 {
			t.Errorf("box %d at %+v leaves the chart", i, b)
		}
	}
	if b := placeBoxes(r, fixed{})[0]; b.Y < r.Marks[0].Y2 {
		t.Error("a box with no room above was not flipped below its mark")
	}
}

func TestHoverTextIsEscaped(t *testing.T) {
	r := rendered{Width: 300, Height: 100, Marks: []mark{{Tooltip: []string{`<b>a & "b"</b>`}, X1: 10, Y1: 50, X2: 20, Y2: 60}}}
	// One data mark, shaped as Vega writes them: a graphics-symbol in a role-mark group.
	svg := `<svg width="300" height="100"><g class="mark-symbol role-mark"><path role="graphics-symbol" aria-label="x" d="M0,0"/></g></svg>`
	out, _ := hoverLayer(svg, r, "md2html-c1", fixed{})
	if strings.Contains(out, "<b>") || !strings.Contains(out, "&lt;b&gt;a &amp; &#34;b&#34;&lt;/b&gt;") {
		t.Errorf("tooltip text not escaped: %s", out)
	}
}

// Ids Vega writes itself (a clip path, a gradient) must take the chart's
// prefix, references included, or two charts on a page collide.
func TestHoverLayerNamespacesVegaIds(t *testing.T) {
	svg := `<svg width="10" height="10"><defs><clipPath id="clip1"><rect/></clipPath></defs><g clip-path="url(#clip1)"></g></svg>`
	out, _ := hoverLayer(svg, rendered{Width: 10, Height: 10}, "md2html-c4", fixed{})
	if strings.Contains(out, `id="clip1"`) || !strings.Contains(out, `id="md2html-c4-clip1"`) || !strings.Contains(out, `url(#md2html-c4-clip1)`) {
		t.Errorf("vega ids not namespaced: %s", out)
	}
}

var update = flag.Bool("update", false, "rewrite golden files")

// goldenSpecs covers every chart form the spec's testing section names.
// Their data is neutral; budget is the most each output may weigh, in bytes.
var goldenSpecs = []struct {
	name, spec string
	budget     int
}{
	{"dot", `{"width":480,"data":{"values":[{"l":"Baseline","v":1},{"l":"Step A","v":1.2},{"l":"Step <b> & \"c\"","v":1.98}]},"mark":"point","encoding":{"y":{"field":"l","type":"nominal","sort":null},"x":{"field":"v","type":"quantitative","scale":{"zero":false}},"tooltip":{"field":"v"}}}`, 16000},
	{"bar", `{"width":480,"data":{"values":[{"k":"a","g":"x","v":3},{"k":"b","g":"y","v":5},{"k":"c","g":"x","v":2}]},"mark":"bar","encoding":{"x":{"field":"k","type":"nominal"},"y":{"field":"v","type":"quantitative"},"color":{"field":"g","type":"nominal"},"tooltip":{"field":"v"}}}`, 20000},
	{"line", `{"width":480,"data":{"values":[{"t":1,"v":2},{"t":2,"v":4},{"t":3,"v":3}]},"mark":{"type":"line","point":true},"encoding":{"x":{"field":"t","type":"quantitative"},"y":{"field":"v","type":"quantitative"},"tooltip":{"field":"v"}}}`, 16000},
	{"interval", `{"width":480,"data":{"values":[{"r":"one","s":0,"e":20,"k":"a"},{"r":"one","s":20,"e":50,"k":"b"},{"r":"two","s":10,"e":40,"k":"a"}]},"mark":"bar","encoding":{"y":{"field":"r","type":"nominal"},"x":{"field":"s","type":"quantitative"},"x2":{"field":"e"},"color":{"field":"k","type":"nominal"},"tooltip":{"field":"k"}}}`, 22000},
}

func TestGolden(t *testing.T) {
	for _, g := range goldenSpecs {
		t.Run(g.name, func(t *testing.T) {
			spec, _ := substituteTokenNames(mustJSON(t, g.spec))
			withTheme := mergeConfig(spec, themeConfig(`"IBM Plex Sans", sans-serif`))
			r, err := renderSpec(marshal(t, withTheme), plex(t), nil)
			if err != nil {
				t.Fatal(err)
			}
			tok, _, err := tokenize(r.SVG)
			if err != nil {
				t.Fatal(err)
			}
			out, css := hoverLayer(tok, r, "md2html-c1", plex(t))
			if css == "" || !strings.Contains(out, `class="md2html-tips"`) {
				t.Fatalf("%s carries no hover boxes: its marks and the svg disagree", g.name)
			}
			got := out + "\n<style>\n" + css + "</style>\n"
			path := filepath.Join("testdata", "golden", g.name+".svg")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("%s differs from its golden file; run with -update and review the diff", g.name)
			}
			if len(got) > g.budget {
				t.Errorf("%s weighs %d bytes, over its %d budget", g.name, len(got), g.budget)
			}
		})
	}
}

func mustJSON(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func marshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Namespacing touches attributes Vega writes ids into, never author text:
// an axis label or an aria-label that looks like a reference stays as is.
func TestNamespacingLeavesAuthorTextAlone(t *testing.T) {
	spec := `{"data":{"values":[{"k":"see url(#x) id=\"y\"","v":1}]},"mark":"point",
	 "encoding":{"y":{"field":"k","type":"nominal"},"x":{"field":"v","type":"quantitative"},"tooltip":{"field":"k"}}}`
	r, err := renderSpec([]byte(spec), fixed{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := hoverLayer(r.SVG, r, "md2html-c1", fixed{})
	if strings.Contains(out, "md2html-c1-x") || strings.Contains(out, "md2html-c1-y") {
		t.Errorf("author text was namespaced: %s", out)
	}
	// Before the boxes (whose own text repeats the tooltip), every
	// url(#x) Vega wrote is still there: the axis label and the aria-labels.
	marks, _, _ := strings.Cut(out, `class="md2html-tips"`)
	if n := strings.Count(r.SVG, "url(#x)"); n < 2 || strings.Count(marks, "url(#x)") != n || strings.Count(marks, `id="y"`) != strings.Count(r.SVG, `id="y"`) {
		t.Errorf("want the label's url(#x) and id=\"y\" kept in axis text and aria-label (%d in vega's svg), got %d", n, strings.Count(marks, "url(#x)"))
	}
}

// Rule and text marks are drawn one element per item, so they get boxes.
func TestRuleAndTextMarksGetBoxes(t *testing.T) {
	for _, mk := range []string{"rule", "text"} {
		spec := `{"data":{"values":[{"k":"a","v":1},{"k":"b","v":3}]},"mark":"` + mk + `",
		 "encoding":{"y":{"field":"k","type":"nominal"},"x":{"field":"v","type":"quantitative"},"text":{"field":"k"},"tooltip":{"field":"v"}}}`
		r, err := renderSpec([]byte(spec), fixed{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		out, css := hoverLayer(r.SVG, r, "md2html-c1", fixed{})
		if len(r.Marks) != 2 || css == "" || !strings.Contains(out, `id="md2html-c1-t1"`) {
			t.Errorf("%s: %d marks, boxes drawn: %v", mk, len(r.Marks), css != "")
		}
	}
}

// A layer without a tooltip must not cost the other layer its boxes.
func TestLayerWithoutTooltipKeepsOthersBoxes(t *testing.T) {
	spec := `{"data":{"values":[{"k":"a","v":1},{"k":"b","v":3},{"k":"c","v":2}]},
	 "encoding":{"x":{"field":"k","type":"nominal"},"y":{"field":"v","type":"quantitative"}},
	 "layer":[{"mark":"bar"},{"mark":"point","encoding":{"tooltip":{"field":"v"}}}]}`
	r, err := renderSpec([]byte(spec), fixed{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, css := hoverLayer(r.SVG, r, "md2html-c1", fixed{})
	if len(r.Marks) != 3 || css == "" {
		t.Fatalf("%d marks, boxes drawn: %v", len(r.Marks), css != "")
	}
	for i := range r.Marks {
		tag := regexp.MustCompile(fmt.Sprintf(`<[a-z]+ id="md2html-c1-m%d"[^>]*>`, i)).FindString(out)
		if !strings.Contains(tag, `aria-roledescription="point"`) {
			t.Errorf("mark %d tagged the wrong element: %s", i, tag)
		}
	}
}
