// The glue between md2html and Vega. render() is called once per chart and
// resolves to the object Go reads back as `rendered`.
// tipLines writes a tooltip as the lines of its box.
function tipLines(tip) {
  return typeof tip === "object" ? Object.keys(tip).map(function (k) { return k + ": " + tip[k]; }) : [String(tip)];
}

function render(specJSON) {
  // Vega numbers clip paths and gradients from process-wide counters.
  vega.resetSVGClipId();
  vega.resetSVGDefIds();
  var warnings = [];
  var spec = JSON.parse(specJSON);
  var compiled = vegaLite.compile(spec, { logger: { warn: function (m) { warnings.push(String(m)); return this; },
    info: function () { return this; }, debug: function () { return this; }, error: function (m) { throw new Error(String(m)); },
    level: function () { return this; } } }).spec;
  var loader = vega.loader();
  loader.load = function (uri) {
    return new Promise(function (resolve, reject) {
      try { resolve(goLoad(String(uri))); } catch (e) { reject(e); }
    });
  };
  var view = new vega.View(vega.parse(compiled), { renderer: "none", loader: loader });
  view.logger({ warn: function (m) { warnings.push(String(m)); }, info: function () {}, debug: function () {}, error: function (m) { warnings.push(String(m)); } });
  return view.toSVG().then(function (svg) {
    var marks = [], untipped = [], drawn = 0, fetch = null, drawnData = [], byName = {};
    // The datum each mark drew, by its place in marks; never sent to Go.
    var markData = [];
    // Vega translates the whole scene by its padding plus its origin.
    var pad = view.padding(), offset = [pad.left + view.origin()[0], pad.top + view.origin()[1]];
    (function walk(node, ox, oy) {
      var items = (node && node.items) || [], drew = [];
      for (var i = 0; i < items.length; i++) {
        var it = items[i], type = it.mark && it.mark.marktype;
        // An image's url is fetched by the page, so only a data: URL is
        // drawn. Go fails the chart on the first other.
        if (type === "image" && fetch == null) {
          var u = it.url == null ? "" : String(it.url);
          if (!/^\s*data:/i.test(u)) fetch = u;
        }
        // Data marks Vega draws as one element per item. Every such item
        // counts, tooltip or not, so Go can find each tooltip mark's
        // element by its place among them.
        if (it.mark.role === "mark" && (type === "symbol" || type === "rect" || type === "arc" || type === "rule" || type === "text")) {
          if (it.tooltip != null && it.bounds) {
            marks.push({ tooltip: tipLines(it.tooltip), label: it.description == null ? "" : String(it.description),
              X1: ox + it.bounds.x1, Y1: oy + it.bounds.y1, X2: ox + it.bounds.x2, Y2: oy + it.bounds.y2, symbol: type === "symbol" });
            markData.push(it.datum);
          } else {
            untipped.push(drawn);
          }
          drawn++;
        }
        // A line or an area is one path for all its items, so no item has
        // an element of its own. Each datum with a tooltip is its point,
        // whose box only a nearest-point region shows.
        if (it.mark.role === "mark" && (type === "line" || type === "area") && it.tooltip != null && isFinite(it.x) && isFinite(it.y)) {
          marks.push({ tooltip: tipLines(it.tooltip), label: it.description == null ? "" : String(it.description),
            X1: ox + it.x, Y1: oy + it.y, X2: ox + it.x, Y2: oy + it.y, datum: true });
          markData.push(it.datum);
        }
        // The data the chart drew, for the table, mark by mark.
        if (it.mark.role === "mark" && type !== "group" && it.datum && typeof it.datum === "object") drew.push(it.datum);
        var gx = ox + (type === "group" ? (it.x || 0) : 0), gy = oy + (type === "group" ? (it.y || 0) : 0);
        if (it.items) for (var j = 0; j < it.items.length; j++) walk(it.items[j], gx, gy);
      }
      // A mark Vega draws once per series (a multi-series line) is one
      // node per series under one name; its data is gathered as one mark's.
      if (drew.length) {
        var name = node.name;
        if (name != null && byName[name]) byName[name].push.apply(byName[name], drew);
        else {
          if (name != null) byName[name] = drew;
          drawnData.push(drew);
        }
      }
    })(view.scenegraph().root, offset[0], offset[1]);
    // A line drawn with its points has a point on each datum, which
    // carries its box; the datum's own is dropped. Only a point drawn
    // from the same datum counts: the same object, or, where Vega copied
    // it (a stacked area's points), one with the same fields and the same
    // tooltip. Another series' or layer's point at the same spot carries
    // another box, and the datum keeps its own.
    function same(a, b) {
      if (a.d === b.d) return true;
      try { return a.tip === b.tip && JSON.stringify(a.d) === JSON.stringify(b.d); } catch (e) { return false; }
    }
    var onPoint = {};
    function at(x, y) { return Math.round(x * 2) + "," + Math.round(y * 2); }
    marks.forEach(function (m, i) {
      if (!m.symbol) return;
      var k = at((m.X1 + m.X2) / 2, (m.Y1 + m.Y2) / 2);
      (onPoint[k] = onPoint[k] || []).push({ d: markData[i], tip: m.tooltip.join("\n") });
    });
    marks = marks.filter(function (m, i) {
      var here = m.datum && onPoint[at(m.X1, m.Y1)], me = { d: markData[i], tip: m.tooltip.join("\n") };
      return !here || !here.some(function (p) { return same(p, me); });
    });
    // The table's columns: one per field the spec encodes, in encoding
    // order, each read from the drawn data under the name Vega-Lite gives
    // it there (sum_v for an aggregate, month_d for a timeUnit).
    var cols = [];
    (function collect(s) {
      if (!s || typeof s !== "object") return;
      var enc = s.encoding || {};
      Object.keys(enc).forEach(function (ch) {
        var defs = Array.isArray(enc[ch]) ? enc[ch] : [enc[ch]];
        defs.forEach(function (d) {
          if (!d || typeof d !== "object") return;
          if (d.aggregate === "count" && d.field == null) {
            cols.push({ keys: ["__count"], name: "count" });
            return;
          }
          if (typeof d.field !== "string") return;
          var keys = [];
          if (typeof d.aggregate === "string") keys.push(d.aggregate + "_" + d.field);
          if (d.timeUnit) {
            var tu = d.timeUnit, unit = typeof tu === "string" ? tu : tu.unit;
            if (typeof tu === "object" && tu.utc && unit && unit.indexOf("utc") !== 0) unit = "utc" + unit;
            if (unit) keys.push(unit + "_" + d.field);
          }
          if (d.bin) keys.push({ bin: d.field });
          keys.push(d.field);
          cols.push({ keys: keys, path: d.field, temporal: d.type === "temporal" || !!d.timeUnit });
        });
      });
      ["layer", "hconcat", "vconcat", "concat"].forEach(function (k) { (s[k] || []).forEach(collect); });
      collect(s.spec);
    })(spec);
    var datums = [].concat.apply([], drawnData);
    // Each column reads the first of its keys the drawn data holds, or,
    // failing all, the field as a path into each datum.
    function holder(want) {
      for (var d = 0; d < datums.length; d++) {
        if (typeof want === "string") {
          if (Object.prototype.hasOwnProperty.call(datums[d], want)) return want;
        } else {
          var bin = Object.keys(datums[d]).filter(function (n) { return n.indexOf("bin_") === 0 && n.slice(-want.bin.length - 1) === "_" + want.bin; })[0];
          if (bin) return bin;
        }
      }
      return null;
    }
    var columns = [];
    cols.forEach(function (c) {
      var key = null;
      for (var k = 0; k < c.keys.length && key == null; k++) key = holder(c.keys[k]);
      var name = c.name || key || c.path;
      if (columns.some(function (o) { return o.name === name; })) return;
      var get;
      if (key != null) {
        get = function (o) { return o[key]; };
      } else {
        try { var f = vega.field(c.path); get = function (o) { try { return f(o); } catch (e) { return undefined; } }; } catch (e) { get = function () { return undefined; }; }
      }
      columns.push({ name: name, get: get, temporal: c.temporal });
    });
    // A value is written as the data holds it. A date, which Vega holds as
    // a time, is written in ISO form, as a day alone at midnight.
    function cell(v, temporal) {
      if (Object.prototype.toString.call(v) === "[object Date]" || (temporal && typeof v === "number")) {
        var t = new Date(+v);
        if (isNaN(t.getTime())) return null;
        var iso = t.toISOString().replace(".000Z", "Z");
        return /T00:00:00Z$/.test(iso) ? iso.slice(0, 10) : iso;
      }
      return v === undefined ? null : v;
    }
    // Rows come mark by mark. A mark that drew the same rows as an earlier
    // one, in any order (a layer over the same data, a line's points),
    // adds none.
    var rows = [], drawnRows = {};
    drawnData.forEach(function (drew) {
      var these = [];
      drew.forEach(function (o) {
        var row = {}, any = false;
        columns.forEach(function (c) { var v = cell(c.get(o), c.temporal); row[c.name] = v; if (v != null) any = true; });
        if (any) these.push(row);
      });
      var key = JSON.stringify(these.map(function (r) { return JSON.stringify(r); }).sort());
      if (!these.length || drawnRows[key]) return;
      drawnRows[key] = true;
      rows = rows.concat(these);
    });
    var fields = columns.map(function (c) { return c.name; });
    return { svg: svg, width: view.width() + view.origin()[0] * 2, height: view.height() + view.origin()[1] * 2,
      marks: marks, untipped: untipped, rows: rows,
      fields: fields, warnings: warnings, fetch: fetch };
  });
}
