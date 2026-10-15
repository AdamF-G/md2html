package md2html

import (
	"strings"
	"testing"
)

// Every component md2html carries must be named in the notices, since the
// bundles themselves carry no headers.
func TestThirdPartyNoticesNameEveryBundledComponent(t *testing.T) {
	got := ThirdPartyNotices()
	for _, name := range []string{"Vega 5.30.0", "Vega-Lite 5.21.0", "Inter ", "d3-array", "delaunator",
		"robust-predicates", "json-stringify-pretty-compact", "topojson-client", "internmap",
		"d3-geo-projection", "d3-path", "d3-scale ", "d3-scale-chromatic", "d3-shape",
		"github.com/dop251/goja ", "github.com/dlclark/regexp2/v2 ", "github.com/eugenioenko/regexp2/v2 ", "github.com/eugenioenko/textmate-go ", "github.com/go-sourcemap/sourcemap ",
		"github.com/google/pprof ", "golang.org/x/text ", "github.com/andybalholm/brotli ", "github.com/golang/freetype ", "github.com/tdewolff/font ", "github.com/tdewolff/parse/v2 ", "golang.org/x/image ", "Go grammar", "YAML grammar", "tm-grammars"} {
		if !strings.Contains(got, "== "+name) {
			t.Errorf("notices do not name %s", name)
		}
	}
	if !strings.Contains(got, "does not imply endorsement") {
		t.Error("notices lack the non-endorsement sentence the BSD-3 licences ask for")
	}
}
