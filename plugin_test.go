package md2html

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseManifestReadsStyles(t *testing.T) {
	p, err := parseManifest([]byte(`{
		"format": 1, "name": "acme", "description": "Acme looks",
		"styles": {
			"plain": {"css": ["a.css", "b.css"]},
			"bare": {"description": "No base", "base": "none", "css": ["c.css"]}
		}
	}`), "acme", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "acme" || p.Description != "Acme looks" {
		t.Errorf("identity = %q, %q", p.Name, p.Description)
	}
	if got := p.StyleNames(); !reflect.DeepEqual(got, []string{"bare", "plain"}) {
		t.Errorf("StyleNames = %v", got)
	}
	plain := p.Styles["plain"]
	if plain.Name != "plain" || plain.BaseNone || !reflect.DeepEqual(plain.css, []string{"a.css", "b.css"}) {
		t.Errorf("plain = %+v", plain)
	}
	if bare := p.Styles["bare"]; !bare.BaseNone || bare.Description != "No base" {
		t.Errorf("bare = %+v", bare)
	}
}

func TestParseManifestRefuses(t *testing.T) {
	for _, c := range []struct{ name, json, want string }{
		{"not json", `{`, "not valid JSON"},
		{"no format", `{"name": "a", "styles": {"s": {"css": ["x.css"]}}}`, "has no format"},
		{"future format", `{"format": 2, "name": "a", "styles": {"s": {"css": ["x.css"]}}}`, "format 2 is not supported"},
		{"no name", `{"format": 1, "styles": {"s": {"css": ["x.css"]}}}`, "has no name"},
		{"bad name", `{"format": 1, "name": "Acme", "styles": {"s": {"css": ["x.css"]}}}`, `name "Acme" is not`},
		{"reserved name", `{"format": 1, "name": "default", "styles": {"s": {"css": ["x.css"]}}}`, "reserved"},
		{"no styles", `{"format": 1, "name": "a"}`, "provides nothing"},
		{"empty styles", `{"format": 1, "name": "a", "styles": {}}`, "provides nothing"},
		{"styles and grammars empty", `{"format": 1, "name": "a", "styles": {}, "grammars": {}}`, "provides nothing"},
		{"bad style name", `{"format": 1, "name": "a", "styles": {"Big": {"css": ["x.css"]}}}`, `style name "Big" is not`},
		{"reserved style", `{"format": 1, "name": "a", "styles": {"default": {"css": ["x.css"]}}}`, "reserved"},
		{"no css", `{"format": 1, "name": "a", "styles": {"s": {}}}`, `style "s" has no css`},
		{"bad base", `{"format": 1, "name": "a", "styles": {"s": {"base": "other", "css": ["x.css"]}}}`, `base "other"`},
		{"wrong type", `{"format": "1", "name": "a", "styles": {"s": {"css": ["x.css"]}}}`, "format"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseManifest([]byte(c.json), "where", false, nil)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to contain %q", err, c.want)
			}
			if err != nil && !strings.HasPrefix(err.Error(), "plugin where: ") {
				t.Errorf("err = %v, want it to name the plugin", err)
			}
		})
	}
}

func TestParseManifestBuiltinMayUseReservedName(t *testing.T) {
	_, err := parseManifest([]byte(`{"format": 1, "name": "default",
		"styles": {"default": {"base": "none", "css": ["default.css"]}}}`), "default", true, nil)
	if err != nil {
		t.Fatal(err)
	}
}

// A plugin written for a later release that only adds keys must still load
// here, minus what this release does not know, and say what it skipped.
func TestParseManifestWarnsOnUnknownKeys(t *testing.T) {
	var warnings []string
	_, err := parseManifest([]byte(`{"format": 1, "name": "a", "homepage": "x",
		"rules": {}, "styles": {"s": {"css": ["x.css"], "dark-only": true}}}`),
		"a", false, func(w string) { warnings = append(warnings, w) })
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`plugin a: unknown key "homepage" in md2html-plugin.json; ignoring it`,
		`plugin a: unknown key "rules" in md2html-plugin.json; ignoring it`,
		`plugin a: unknown key "dark-only" in style "s"; ignoring it`,
	}
	if !reflect.DeepEqual(warnings, want) {
		t.Errorf("warnings = %q\nwant %q", warnings, want)
	}
}
