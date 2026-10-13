// Package charts renders Vega-Lite specs to static SVG when md2html builds
// a page, inside an embedded JavaScript engine. Nothing it produces needs a
// script to display.
package charts

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
)

var (
	//go:embed vendor/vega.min.js
	vegaJS string
	//go:embed vendor/vega-lite.min.js
	vegaLiteJS string
	//go:embed driver.js
	driverJS string
)

// budget is how long one chart may run before it is interrupted. A var so
// a test can shorten it.
var budget = 5 * time.Second

// beforeStop is a test hook run just before the budget timer is stopped.
var beforeStop func()

// utcJS makes the engine's local time UTC. goja reads dates in the host's
// zone (time.Local), so without it a chart's dates would move with the
// machine that builds the page. It is set here, not on time.Local, so a
// program that embeds md2html keeps its own zone.
const utcJS = `(function () {
	var D = Date, P = D.prototype, parse = D.parse;
	["FullYear", "Month", "Date", "Hours", "Minutes", "Seconds", "Milliseconds"].forEach(function (n) {
		P["get" + n] = P["getUTC" + n];
		P["set" + n] = P["setUTC" + n];
	});
	P.getDay = P.getUTCDay;
	P.getTimezoneOffset = function () { return 0; };
	var days = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
	var months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
	function p2(n) { return (n < 10 ? "0" : "") + n; }
	function valid(d) { return !isNaN(d.getTime()); }
	function hms(d) { return p2(d.getUTCHours()) + ":" + p2(d.getUTCMinutes()) + ":" + p2(d.getUTCSeconds()); }
	function mdy(d) { return p2(d.getUTCMonth() + 1) + "/" + p2(d.getUTCDate()) + "/" + d.getUTCFullYear(); }
	P.toDateString = function () { return valid(this) ? days[this.getUTCDay()] + " " + months[this.getUTCMonth()] + " " + p2(this.getUTCDate()) + " " + this.getUTCFullYear() : "Invalid Date"; };
	P.toTimeString = function () { return valid(this) ? hms(this) + " GMT+0000 (UTC)" : "Invalid Date"; };
	P.toString = function () { return valid(this) ? this.toDateString() + " " + this.toTimeString() : "Invalid Date"; };
	P.toLocaleDateString = function () { return valid(this) ? mdy(this) : "Invalid Date"; };
	P.toLocaleTimeString = function () { return valid(this) ? hms(this) : "Invalid Date"; };
	P.toLocaleString = function () { return valid(this) ? mdy(this) + ", " + hms(this) : "Invalid Date"; };
	// A string with no zone is read as UTC: an ISO date-time by adding
	// "Z", any other form by adding " GMT". One that names a zone, or an
	// ISO date alone (UTC already), is read as written.
	var isoLocal = /^([+-]\d{6}|\d{4})(-\d\d(-\d\d)?)?T\d\d:\d\d(:\d\d(\.\d+)?)?$/;
	var isoForm = /^([+-]\d{6}|\d{4})(-\d\d(-\d\d)?)?(T|$)/;
	var zoned = /(^|[^a-z])(gmt|utc|ut|z|[ecmp][sd]t|wet|west|cet|cest|eet|eest)([^a-z]|$)|\d:\d\d(:\d\d(\.\d+)?)?\s*([ap]m)?\s*[+-]\d/i;
	function utcParse(s) {
		s = String(s);
		if (isoLocal.test(s)) return parse(s + "Z");
		if (isoForm.test(s) || zoned.test(s.replace(/\([^)]*\)/g, " "))) return parse(s);
		var t = parse(s + " GMT");
		return t === t ? t : parse(s);
	}
	function UTCDate(y, m) {
		if (!(this instanceof UTCDate)) return new D().toString();
		switch (arguments.length) {
		case 0: return new D();
		case 1: return typeof y === "string" ? new D(utcParse(y)) : new D(y);
		default: return new D(D.UTC.apply(null, arguments));
		}
	}
	UTCDate.prototype = P;
	P.constructor = UTCDate;
	UTCDate.UTC = D.UTC;
	UTCDate.now = D.now;
	UTCDate.parse = utcParse;
	globalThis.Date = UTCDate;
})();`

type engine struct {
	vm      *goja.Runtime
	render  goja.Callable
	timers  []*timer
	clock   time.Duration
	measure Measurer
	load    func(string) (string, error)
	loadErr error // the first read this chart failed, which fails the chart
}

type timer struct {
	fn    goja.Callable
	at    time.Duration
	alive bool
}

var (
	mu  sync.Mutex // goja runtimes are not goroutine-safe; one chart at a time
	cur *engine    // built on first use, rebuilt after an interrupt
)

func newEngine() (*engine, error) {
	e := &engine{vm: goja.New()}
	vm := e.vm
	vm.Set("setTimeout", func(c goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(c.Argument(0))
		if !ok {
			return goja.Undefined()
		}
		e.timers = append(e.timers, &timer{fn: fn, at: e.clock + time.Duration(c.Argument(1).ToInteger())*time.Millisecond, alive: true})
		return vm.ToValue(len(e.timers)) // ids start at 1, as a browser's do
	})
	vm.Set("clearTimeout", func(id int) {
		if id >= 1 && id <= len(e.timers) {
			e.timers[id-1].alive = false
		}
	})
	noop := func(...any) {}
	vm.Set("console", map[string]any{"log": noop, "warn": noop, "error": noop, "info": noop, "debug": noop})
	vm.Set("goMeasure", func(s string, size, weight float64) float64 { return e.measure.Width(s, size, weight) })
	vm.Set("goLoad", func(u string) (string, error) {
		var s string
		var err error
		if e.load == nil {
			err = fmt.Errorf("data url %q: charts read no data files here", u)
		} else {
			s, err = e.load(u)
		}
		// Vega only logs a failed load and draws the chart empty, so it is
		// kept and fails the render instead.
		if err != nil && e.loadErr == nil {
			e.loadErr = err
		}
		return s, err
	})
	prelude := `
		globalThis.structuredClone = function (v) { return v === undefined ? v : JSON.parse(JSON.stringify(v)); };
		// A seeded generator, reset before each chart, so output is byte-identical.
		globalThis.__seed = 1;
		Math.random = function () { __seed = (__seed * 1103515245 + 12345) % 2147483648; return __seed / 2147483648; };`
	for _, s := range []struct{ name, src string }{{"prelude", prelude}, {"utc.js", utcJS}, {"vega.min.js", vegaJS}, {"vega-lite.min.js", vegaLiteJS}, {"driver.js", driverJS}} {
		if _, err := vm.RunScript(s.name, s.src); err != nil {
			return nil, fmt.Errorf("charts: load %s: %w", s.name, err)
		}
	}
	if _, err := vm.RunString(`
		vega.textMetrics.width = function (item, text) {
			var size = item.fontSize != null ? +item.fontSize : 11;
			var w = item.fontWeight == null ? 400 : (item.fontWeight === "bold" ? 700 : (+item.fontWeight || 400));
			return goMeasure(String(text == null ? "" : text), size, w);
		};`); err != nil {
		return nil, err
	}
	fn, ok := goja.AssertFunction(vm.Get("render"))
	if !ok {
		return nil, errors.New("charts: driver.js defines no render")
	}
	e.render = fn
	return e, nil
}

// runTimers fires due timers in time order until none remain.
func (e *engine) runTimers() error {
	for {
		var next *timer
		for _, t := range e.timers {
			if t.alive && (next == nil || t.at < next.at) {
				next = t
			}
		}
		if next == nil {
			return nil
		}
		next.alive = false
		e.clock = next.at
		if _, err := next.fn(goja.Undefined()); err != nil {
			return err
		}
	}
}

var svgSize = regexp.MustCompile(`^<svg[^>]* width="([0-9.]+)" height="([0-9.]+)"`)

func renderSpec(specJSON []byte, m Measurer, load func(string) (string, error)) (rendered, error) {
	mu.Lock()
	defer mu.Unlock()
	if cur == nil {
		e, err := newEngine()
		if err != nil {
			return rendered{}, err
		}
		cur = e
	}
	e := cur
	e.measure, e.load, e.loadErr, e.timers, e.clock = m, load, nil, nil, 0
	e.vm.Set("__seed", 1)
	var interrupted atomic.Bool
	stop := time.AfterFunc(budget, func() { interrupted.Store(true); e.vm.Interrupt("time budget") })
	defer func() {
		if beforeStop != nil {
			beforeStop()
		}
		// A timer that already fired may have left an interrupt pending on
		// the runtime; do not trust it again.
		if !stop.Stop() {
			cur = nil
		}
	}()
	fail := func(err error) (rendered, error) {
		if interrupted.Load() {
			cur = nil // a runtime cut off mid-run is not trusted again
			return rendered{}, fmt.Errorf("chart ran past its %v time budget", budget)
		}
		return rendered{}, err
	}
	pv, err := e.render(goja.Undefined(), e.vm.ToValue(string(specJSON)))
	if err != nil {
		return fail(err)
	}
	if err := e.runTimers(); err != nil {
		return fail(err)
	}
	if e.loadErr != nil {
		return rendered{}, e.loadErr
	}
	p, ok := pv.Export().(*goja.Promise)
	if !ok {
		return fail(errors.New("charts: render did not return a promise"))
	}
	switch p.State() {
	case goja.PromiseStateRejected:
		return fail(fmt.Errorf("%v", p.Result()))
	case goja.PromiseStatePending:
		return fail(errors.New("charts: render never finished"))
	}
	b, err := json.Marshal(p.Result().Export())
	if err != nil {
		return rendered{}, err
	}
	var r rendered
	if err := json.Unmarshal(b, &r); err != nil {
		return rendered{}, err
	}
	if r.Fetch != nil {
		return rendered{}, fmt.Errorf("an image mark's url %q would make the page fetch it; only a data: URL is drawn", *r.Fetch)
	}
	if m := svgSize.FindStringSubmatch(r.SVG); m != nil {
		r.Width, _ = strconv.ParseFloat(m[1], 64)
		r.Height, _ = strconv.ParseFloat(m[2], 64)
	}
	return r, nil
}

// Measurer gives the advance width, in CSS px, of text set at size px and
// weight (400 regular, 700 bold) in the chart's face.
type Measurer interface {
	Width(text string, size, weight float64) float64
}

// rendered is what the engine returns for one chart.
type rendered struct {
	SVG    string  `json:"svg"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Marks  []mark  `json:"marks"` // marks that carry a tooltip, in SVG order
	// Untipped lists, by their place among the SVG elements of data marks
	// drawn one per item, the elements that carry no tooltip.
	Untipped []int            `json:"untipped"`
	Rows     []map[string]any `json:"rows"`   // the data the chart drew, for the table
	Fields   []string         `json:"fields"` // the table's columns: encoded fields in encoding order
	Warnings []string         `json:"warnings"`
	Fetch    *string          `json:"fetch"` // an image url other than data:, which fails the chart
}

type mark struct {
	Tooltip        []string `json:"tooltip"`
	Label          string   `json:"label"`
	X1, Y1, X2, Y2 float64  // absolute bounds in SVG px
	Symbol         bool     `json:"symbol"` // a point mark (gets a nearest-point cell)
	// Datum marks a datum of a line or an area: a point (X1 = X2, Y1 = Y2)
	// with no element of its own, whose box only its nearest-point cell shows.
	Datum bool `json:"datum"`
}

// elements counts the tooltip marks drawn as elements of their own, each
// a tab stop up to maxTabStops.
func (r rendered) elements() int {
	n := 0
	for _, m := range r.Marks {
		if !m.Datum {
			n++
		}
	}
	return n
}
