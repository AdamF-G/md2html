package md2html

import (
	"strings"
	"testing"
)

// Every component md2html carries must be named in the notices, since the
// bundles themselves carry no headers.
func TestThirdPartyNoticesNameEveryBundledComponent(t *testing.T) {
	got := ThirdPartyNotices()
	for _, name := range []string{"Vega 5.30.0", "Vega-Lite 5.21.0", "d3-array", "delaunator",
		"robust-predicates", "json-stringify-pretty-compact", "topojson-client", "internmap",
		"d3-geo-projection", "d3-path", "d3-scale ", "d3-scale-chromatic", "d3-shape"} {
		if !strings.Contains(got, "== "+name) {
			t.Errorf("notices do not name %s", name)
		}
	}
	if !strings.Contains(got, "does not imply endorsement") {
		t.Error("notices lack the non-endorsement sentence the BSD-3 licences ask for")
	}
}
