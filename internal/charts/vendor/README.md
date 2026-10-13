# Vendored Vega and Vega-Lite

Minified browser bundles, embedded unmodified. They carry no licence headers;
their notices, and those of the modules bundled inside them, are in
`THIRD_PARTY_NOTICES` at the repository root.

| File | Source | Version | SHA-256 |
|------|--------|---------|---------|
| `vega.min.js` | https://cdn.jsdelivr.net/npm/vega@5.30.0/build/vega.min.js | 5.30.0 | `e432c751a6363f4a61da62920cc7d7ebd13cf09d82949f8f486248f8071dc3ce` |
| `vega-lite.min.js` | https://cdn.jsdelivr.net/npm/vega-lite@5.21.0/build/vega-lite.min.js | 5.21.0 | `cd32314b1e76e7d879dc9f0534b62be714df03554486c7ca2381abfd0a92d2f4` |

## Updating

1. Re-download both files from the new versions' URLs and record the versions
   and `shasum -a 256` sums above.
2. Refresh `THIRD_PARTY_NOTICES`: `npm pack` each bundled module at the
   versions `npm ls --all` reports for the new Vega and Vega-Lite, and copy its
   `LICENSE`.
3. Re-run `go test ./...` and the e2e suite (`cd e2e && go test -count=1 ./...`).
4. Review any diff in the golden SVGs; a changed chart is a change to the
   output of every page that has one.
