# Test-only browser bundle

`vega-embed.min.js` is used by `TestBrowserChartMetricsMatchVega` alone, to let
Vega lay a chart out in a real browser for comparison. It is never embedded in
md2html or written into a page md2html builds. The test serves it from its own
local server, beside the Vega and Vega-Lite bundles in
`../../internal/charts/vendor`.

| File | Source | Version | SHA-256 |
|------|--------|---------|---------|
| `vega-embed.min.js` | https://cdn.jsdelivr.net/npm/vega-embed@6.26.0/build/vega-embed.min.js | 6.26.0 | `072c054f2a6310725e038c38a71e00052705e31835632462c9717a23a384e895` |
