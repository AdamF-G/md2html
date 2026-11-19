package md2html

import (
	"strings"
	"testing"
)

// A heading's block is checked by the span's rules, under the heading's
// name, and a heading is no tip's host.
func TestCheckAttrBlockHeadingWho(t *testing.T) {
	var w []string
	collect := func(m string) { w = append(w, m) }
	if _, ok := checkAttrBlock(`heading "Setup"`, "#s data-flag", false, collect); ok {
		t.Error("a bare key beside an id was taken")
	}
	if len(w) != 1 || !strings.HasPrefix(w[0], `heading "Setup": data-flag has no value`) {
		t.Errorf("warnings = %q", w)
	}
	w = nil
	a, ok := checkAttrBlock(`heading "T"`, `#t tip="n" tip-delay="0ms"`, false, collect)
	if !ok || a.id != "t" || a.tip != "" || len(w) != 0 {
		t.Errorf("got %+v ok=%v warnings %q; want id t, no tip, no warning", a, ok, w)
	}
}
