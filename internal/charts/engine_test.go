package charts

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fixed measures every character as 7 px, so layout is predictable.
type fixed struct{}

func (fixed) Width(s string, size, weight float64) float64 {
	return 7 * float64(len([]rune(s))) * size / 12
}

const dotSpec = `{"data":{"values":[{"k":"a","v":1},{"k":"b","v":3}]},"mark":"point",
 "encoding":{"y":{"field":"k","type":"nominal"},"x":{"field":"v","type":"quantitative"},"tooltip":{"field":"v"}}}`

func TestRenderSpecProducesSVGAndMarks(t *testing.T) {
	r, err := renderSpec([]byte(dotSpec), fixed{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.SVG, "<svg") || len(r.Marks) != 2 {
		t.Fatalf("want an svg and 2 tooltip marks, got %d marks: %.80s", len(r.Marks), r.SVG)
	}
	if !r.Marks[0].Symbol || r.Marks[0].Tooltip[0] != "1" {
		t.Errorf("first mark %+v", r.Marks[0])
	}
}

// The same spec renders byte-identically, run after run, even for marks
// that draw on Math.random.
func TestRenderSpecIsDeterministic(t *testing.T) {
	jitter := `{"data":{"values":[{"v":1},{"v":2},{"v":3}]},"transform":[{"calculate":"random()","as":"j"}],
	 "mark":"point","encoding":{"x":{"field":"v","type":"quantitative"},"y":{"field":"j","type":"quantitative"}}}`
	a, err := renderSpec([]byte(jitter), fixed{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := renderSpec([]byte(jitter), fixed{}, nil)
	if a.SVG != b.SVG {
		t.Error("two renders of one spec differ")
	}
}

func TestRenderSpecReportsACompileError(t *testing.T) {
	_, err := renderSpec([]byte(`{"mark":"no-such-mark"}`), fixed{}, nil)
	if err == nil {
		t.Fatal("a bad mark compiled")
	}
}

func TestRenderSpecSerialisesConcurrentCallers(t *testing.T) {
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := renderSpec([]byte(dotSpec), fixed{}, nil)
			if err == nil && len(r.Marks) != 2 {
				err = fmt.Errorf("got %d marks", len(r.Marks))
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestRenderSpecStopsARunawaySpec(t *testing.T) {
	old := budget
	budget = 300 * time.Millisecond
	defer func() { budget = old }()
	huge := `{"data":{"sequence":{"start":0,"stop":1000000000}},"mark":"point","encoding":{"x":{"field":"data","type":"quantitative"}}}`
	start := time.Now()
	if _, err := renderSpec([]byte(huge), fixed{}, nil); err == nil || !strings.Contains(err.Error(), "time budget") {
		t.Fatalf("want a time budget error, got %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("the budget did not stop the render promptly")
	}
	// The interrupt discards the runtime, so the next chart is the first
	// on a fresh one, and under -race on a shared CI runner that alone can
	// take longer than the shortened budget. Recovery is checked against
	// the real budget: the question is whether the engine works again, not
	// how fast a cold start is.
	budget = old
	if _, err := renderSpec([]byte(dotSpec), fixed{}, nil); err != nil {
		t.Fatalf("the engine did not recover after an interrupt: %v", err)
	}
}

// Vega numbers clip paths from a process-wide counter; renders must not
// depend on which charts ran before.
func TestRenderSpecClipIdsDoNotDriftBetweenRenders(t *testing.T) {
	clipped := `{"data":{"values":[{"x":1,"y":1},{"x":5,"y":9},{"x":20,"y":3}]},
	 "mark":{"type":"point","clip":true},
	 "encoding":{"x":{"field":"x","type":"quantitative","scale":{"domain":[0,10]}},"y":{"field":"y","type":"quantitative"}}}`
	a, err := renderSpec([]byte(clipped), fixed{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.SVG, "clip") {
		t.Fatal("spec produced no clip path; the test checks nothing")
	}
	b, _ := renderSpec([]byte(clipped), fixed{}, nil)
	if a.SVG != b.SVG {
		t.Error("two renders of a clipped spec differ")
	}
}

// A budget timer that fires after the last script call returns, but before
// it is stopped, must not poison the next chart.
func TestRenderSpecSurvivesALateBudgetTimer(t *testing.T) {
	if _, err := renderSpec([]byte(dotSpec), fixed{}, nil); err != nil { // warm the engine
		t.Fatal(err)
	}
	old := budget
	budget = 2 * time.Second
	beforeStop = func() { time.Sleep(2500 * time.Millisecond) }
	defer func() { budget = old; beforeStop = nil }()
	if _, err := renderSpec([]byte(dotSpec), fixed{}, nil); err != nil {
		t.Fatal(err)
	}
	beforeStop = nil
	budget = old
	if _, err := renderSpec([]byte(dotSpec), fixed{}, nil); err != nil {
		t.Fatalf("the next chart was interrupted: %v", err)
	}
}

// A mark's bounds are in the SVG's own pixels: Vega offsets the whole
// scene by its padding as well as its origin, so the bounds must too.
func TestMarkBoundsMatchTheSVG(t *testing.T) {
	r, err := renderSpec([]byte(dotSpec), fixed{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := regexp.MustCompile(`<g [^>]*transform="translate\(([0-9.]+),([0-9.]+)\)"`).FindStringSubmatch(r.SVG)
	point := regexp.MustCompile(`role="graphics-symbol" aria-roledescription="point" transform="translate\(([0-9.]+),([0-9.]+)\)"`).FindStringSubmatch(r.SVG)
	if root == nil || point == nil {
		t.Fatalf("no root or point translate in %.300s", r.SVG)
	}
	f := func(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
	m := r.Marks[0]
	cx, cy := f(root[1])+f(point[1]), f(root[2])+f(point[2])
	if math.Abs((m.X1+m.X2)/2-cx) > 0.01 || math.Abs((m.Y1+m.Y2)/2-cy) > 0.01 {
		t.Errorf("mark 0 centred at (%v,%v), drawn at (%v,%v)", (m.X1+m.X2)/2, (m.Y1+m.Y2)/2, cx, cy)
	}
}

// Dates render in UTC whatever the zone of the machine that builds the
// page: an ISO date, a local ISO date-time, another local form, a
// timeUnit and a DateTime domain all come out the same under any zone.
func TestTemporalChartsIgnoreTheHostZone(t *testing.T) {
	spec := `{"data":{"values":[{"d":"2024-01-01","v":1},{"d":"2024-03-10T02:30","v":2},{"d":"Jan 2 2024 10:00","v":3}]},
	 "mark":"point","encoding":{"x":{"field":"d","type":"temporal","scale":{"domain":[{"year":2023,"month":12,"date":25},{"year":2024,"month":3,"date":15}]}},
	 "y":{"field":"d","timeUnit":"hours","type":"ordinal"},"tooltip":{"field":"d","type":"temporal","format":"%Y-%m-%d %H:%M"}}}`
	render := func(loc *time.Location) rendered {
		old := time.Local
		time.Local = loc
		defer func() { time.Local = old }()
		r, err := renderSpec([]byte(spec), fixed{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	utc := render(time.UTC)
	var tips []string
	for _, m := range utc.Marks {
		tips = append(tips, m.Tooltip...)
	}
	if got := strings.Join(tips, "|"); got != "2024-01-01 00:00|2024-03-10 02:30|2024-01-02 10:00" {
		t.Errorf("tooltips under UTC: %s", got)
	}
	zones := []*time.Location{time.FixedZone("west", -8*3600), time.FixedZone("east", 9*3600+1800)}
	if la, err := time.LoadLocation("America/Los_Angeles"); err == nil {
		zones = append(zones, la) // 2024-03-10T02:30 falls in its spring-forward gap
	}
	for _, zone := range zones {
		r := render(zone)
		if r.SVG != utc.SVG {
			t.Errorf("the chart differs under %s", zone)
		}
		if fmt.Sprint(r.Rows) != fmt.Sprint(utc.Rows) {
			t.Errorf("the rows differ under %s: %v, %v", zone, r.Rows, utc.Rows)
		}
	}
}

// Timer ids start at 1, as a browser's do: a script that guards with
// "if (t)" sees its first timer, and clearTimeout(0) clears nothing.
func TestTimerIdsStartAtOne(t *testing.T) {
	e, err := newEngine()
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.vm.RunString(`var fired = false; var id = setTimeout(function () { fired = true; }, 0); clearTimeout(0); id`)
	if err != nil {
		t.Fatal(err)
	}
	if id := v.ToInteger(); id < 1 {
		t.Errorf("the first timer's id is %d", id)
	}
	if err := e.runTimers(); err != nil {
		t.Fatal(err)
	}
	if !e.vm.Get("fired").ToBoolean() {
		t.Error("clearTimeout(0) cleared the first timer")
	}
}

// An image mark would make the page fetch its url, so only a data: URL is
// drawn; any other url fails the chart, naming it.
func TestImageMarksDrawOnlyDataURLs(t *testing.T) {
	image := func(u string) string {
		return `{"data":{"values":[{"x":1,"u":"` + u + `"}]},"mark":{"type":"image","width":10,"height":10},
		 "encoding":{"x":{"field":"x","type":"quantitative"},"url":{"field":"u"}}}`
	}
	for _, u := range []string{"https://example.com/a.png", "//example.com/a.png", "a.png"} {
		if _, err := renderSpec([]byte(image(u)), fixed{}, nil); err == nil || !strings.Contains(err.Error(), u) {
			t.Errorf("%s: want an error naming the url, got %v", u, err)
		}
	}
	dataURL := "data:image/png;base64,iVBORw0KGgo="
	r, err := renderSpec([]byte(image(dataURL)), fixed{}, nil)
	if err != nil || !strings.Contains(r.SVG, `href="`+dataURL+`"`) {
		t.Errorf("a data: URL was not drawn: %v", err)
	}
}

// Six values in two series, for the multi-series line cases below: one
// row each, though the line draws one node per series and its points one
// node for all.
const sixValues = `[{"s":"a","x":1,"y":1},{"s":"a","x":2,"y":2},{"s":"a","x":3,"y":3},{"s":"b","x":1,"y":4},{"s":"b","x":2,"y":5},{"s":"b","x":3,"y":6}]`

const sixRows = "[x y s] [map[s:a x:1 y:1] map[s:a x:2 y:2] map[s:a x:3 y:3] map[s:b x:1 y:4] map[s:b x:2 y:5] map[s:b x:3 y:6]]"

// The table holds the data the chart drew, so fields a transform derives,
// aggregates and named datasets are in it, not only the raw values.
func TestRowsAreTheDrawnData(t *testing.T) {
	for _, c := range []struct{ name, spec, want string }{
		{"fold", `{"data":{"values":[{"a":1,"b":2}]},"transform":[{"fold":["a","b"]}],"mark":"bar",
		 "encoding":{"x":{"field":"key","type":"nominal"},"y":{"field":"value","type":"quantitative"}}}`,
			"[key value] [map[key:a value:1] map[key:b value:2]]"},
		{"calculate", `{"data":{"values":[{"v":1},{"v":2}]},"transform":[{"calculate":"datum.v * 10","as":"w"}],"mark":"point",
		 "encoding":{"x":{"field":"v","type":"quantitative"},"y":{"field":"w","type":"quantitative"}}}`,
			"[v w] [map[v:1 w:10] map[v:2 w:20]]"},
		{"aggregate", `{"data":{"values":[{"k":"a","v":1},{"k":"a","v":2},{"k":"b","v":5}]},"mark":"bar",
		 "encoding":{"x":{"field":"k","type":"nominal"},"y":{"aggregate":"sum","field":"v","type":"quantitative"},"color":{"aggregate":"count","type":"quantitative"}}}`,
			"[k sum_v count] [map[count:2 k:a sum_v:3] map[count:1 k:b sum_v:5]]"},
		{"named dataset", `{"datasets":{"pts":[{"v":1},{"v":2}]},"data":{"name":"pts"},"mark":"point","encoding":{"x":{"field":"v","type":"quantitative"}}}`,
			"[v] [map[v:1] map[v:2]]"},
		{"layers share their data", `{"data":{"values":[{"k":"a","v":1},{"k":"b","v":2}]},"encoding":{"x":{"field":"k","type":"nominal"},"y":{"field":"v","type":"quantitative"}},
		 "layer":[{"mark":"bar"},{"mark":"text","encoding":{"text":{"field":"v"}}}]}`,
			"[k v] [map[k:a v:1] map[k:b v:2]]"},
		{"dates as written", `{"data":{"values":[{"d":"2024-01-01","v":1},{"d":"2024-01-02T10:30","v":2}]},"mark":"point",
		 "encoding":{"x":{"field":"d","type":"temporal"},"y":{"field":"v","type":"quantitative"}}}`,
			"[d v] [map[d:2024-01-01 v:1] map[d:2024-01-02T10:30:00Z v:2]]"},
		{"a date a transform makes", `{"data":{"values":[{"v":1}]},"transform":[{"calculate":"datetime(2024, 0, 2, 9)","as":"w"}],"mark":"point",
		 "encoding":{"x":{"field":"v","type":"quantitative"},"y":{"field":"w","type":"nominal"}}}`,
			"[v w] [map[v:1 w:2024-01-02T09:00:00Z]]"},
		{"a multi-series line's points", `{"data":{"values":` + sixValues + `},"mark":{"type":"line","point":true},
		 "encoding":{"x":{"field":"x","type":"quantitative"},"y":{"field":"y","type":"quantitative"},"color":{"field":"s","type":"nominal"}}}`,
			sixRows},
		{"a multi-series line layered with points", `{"data":{"values":` + sixValues + `},
		 "encoding":{"x":{"field":"x","type":"quantitative"},"y":{"field":"y","type":"quantitative"},"color":{"field":"s","type":"nominal"}},
		 "layer":[{"mark":"line"},{"mark":"point"}]}`,
			sixRows},
		{"a layer over other data adds its rows", `{"data":{"values":[{"k":"a","v":1},{"k":"b","v":3}]},
		 "layer":[{"mark":"bar","encoding":{"x":{"field":"k","type":"nominal"},"y":{"field":"v","type":"quantitative"}}},
		  {"mark":"rule","encoding":{"y":{"aggregate":"mean","field":"v","type":"quantitative"}}}]}`,
			"[k v mean_v] [map[k:a mean_v:<nil> v:1] map[k:b mean_v:<nil> v:3] map[k:<nil> mean_v:2 v:<nil>]]"},
		{"repeated data stays repeated", `{"data":{"values":[{"v":1},{"v":1}]},"mark":"point","encoding":{"x":{"field":"v","type":"quantitative"}}}`,
			"[v] [map[v:1] map[v:1]]"},
	} {
		r, err := renderSpec([]byte(c.spec), fixed{}, nil)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := fmt.Sprint(r.Fields, " ", r.Rows); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}
