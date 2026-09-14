package md2html

import (
	"strings"
	"testing"
)

// title extracts what parseFenceInfo marked as the title, so a test can
// assert on the string rather than on two offsets.
func title(info string, i fenceInfoResult) string {
	if i.TitleEnd <= i.TitleStart {
		return ""
	}
	return info[i.TitleStart:i.TitleEnd]
}

func TestParseFenceInfoLabelForm(t *testing.T) {
	const info = "aside[Why this matters]{#w .compact}"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "aside" {
		t.Errorf("kind = %q, want %q", got.Kind, "aside")
	}
	if s := title(info, got); s != "Why this matters" {
		t.Errorf("title = %q, want %q", s, "Why this matters")
	}
	want := map[string]string{"id": "w", "class": "compact"}
	for _, a := range got.Attrs {
		if want[a.Name] != a.Value {
			t.Errorf("attr %s = %q, want %q", a.Name, a.Value, want[a.Name])
		}
		delete(want, a.Name)
	}
	if len(want) > 0 {
		t.Errorf("missing attributes: %v", want)
	}
}

func TestParseFenceInfoLabelFormNoAttrs(t *testing.T) {
	const info = "aside[Why]"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "aside" {
		t.Errorf("kind = %q, want %q", got.Kind, "aside")
	}
	if s := title(info, got); s != "Why" {
		t.Errorf("title = %q, want %q", s, "Why")
	}
	if len(got.Attrs) != 0 {
		t.Errorf("attrs = %v, want none", got.Attrs)
	}
}

// An unclosed bracket is not a label form. It falls through to the bare
// path as one unusable kind word, so the unknown-kind warning fires on it.
func TestParseFenceInfoUnclosedBracketIsNotALabel(t *testing.T) {
	const info = "aside[Why"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "aside[Why" {
		t.Errorf("kind = %q, want the whole word %q", got.Kind, "aside[Why")
	}
	if s := title(info, got); s != "" {
		t.Errorf("title = %q, want none", s)
	}
}

// unsafeAttrNameBytes are the characters that let an attribute name break
// out of the attribute list it is written into: a quote closes the
// surrounding value, "=" and a space start a new attribute, and "<" or ">"
// end the tag. This is deliberately spelled out here rather than expressed
// as !safeAttrName(...): asserting the filter's own predicate on the
// filter's own output tests nothing, because fenceAttrs selected those names
// with that very predicate.
const unsafeAttrNameBytes = "\"'=<> \t"

// An attribute name reaches the output unescaped, so a key carrying a quote
// would close the attribute and make the rest of it an event handler.
// goldmark's own ParseAttributes used to reject such a block; this package's
// parser does not, so the guard lives here.
func TestParseFenceInfoRejectsUnsafeAttributeNames(t *testing.T) {
	for _, info := range []string{
		`card[T]{data-y"onmouseover="alert(1)}`,
		`card[T]{aria-x"onmouseover="alert(1)}`,
		`card[T]{<script>=1}`,
	} {
		got, ok := parseFenceInfo(info)
		if !ok {
			t.Fatalf("parseFenceInfo(%q) reported false", info)
		}
		for _, a := range got.Attrs {
			if strings.ContainsAny(a.Name, unsafeAttrNameBytes) {
				t.Errorf("%s: unsafe attribute name survived: %q", info, a.Name)
			}
		}
	}
}

// The guard's real claim is about the rendered page, not about a struct:
// html.RenderAttributes writes names verbatim and exempts every data- name
// from its allowlist, so safeAttrName is the only thing between an injected
// key and a live event handler in the output.
func TestContainerLabelFormAttributesCannotInjectAHandler(t *testing.T) {
	for _, src := range []string{
		":::card[T]{data-y\"onmouseover=\"alert(1)}\nbody\n:::\n",
		":::card[T]{aria-x\"onmouseover=\"alert(1)}\nbody\n:::\n",
		":::card[T]{<script>=1}\nbody\n:::\n",
	} {
		got := convert(t, src, nil)
		if strings.Contains(got, "onmouseover") {
			t.Errorf("%q: event handler reached the output\ngot: %s", src, got)
		}
		if strings.Contains(got, "<script") {
			t.Errorf("%q: script tag reached the output\ngot: %s", src, got)
		}
	}
}

// A legitimate data attribute still passes, so the guard is not a blanket ban.
func TestParseFenceInfoKeepsSafeDataAttribute(t *testing.T) {
	const info = `card[T]{data-sort="name"}`
	got, _ := parseFenceInfo(info)
	var found bool
	for _, a := range got.Attrs {
		if a.Name == "data-sort" && a.Value == "name" {
			found = true
		}
	}
	if !found {
		t.Errorf("data-sort dropped\ngot: %v", got.Attrs)
	}
}

func TestParseFenceInfoBracedWithTitle(t *testing.T) {
	const info = "{.card} Why this matters"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "" {
		t.Errorf("kind = %q, want empty — the kind comes from the class", got.Kind)
	}
	if s := title(info, got); s != "Why this matters" {
		t.Errorf("title = %q, want %q", s, "Why this matters")
	}
	if len(got.Attrs) != 1 || got.Attrs[0].Name != "class" || got.Attrs[0].Value != "card" {
		t.Errorf("attrs = %v, want class=card", got.Attrs)
	}
}

func TestParseFenceInfoBracedWithoutTitle(t *testing.T) {
	const info = "{#note .callout .compact}"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if s := title(info, got); s != "" {
		t.Errorf("title = %q, want none", s)
	}
	var id, class string
	for _, a := range got.Attrs {
		switch a.Name {
		case "id":
			id = a.Value
		case "class":
			class = a.Value
		}
	}
	if id != "note" || class != "callout compact" {
		t.Errorf("id = %q, class = %q; want note / \"callout compact\"", id, class)
	}
}

// A brace inside a quoted value must not end the block early.
func TestParseFenceInfoBracedQuotedBrace(t *testing.T) {
	const info = `{.card data-x="a } b"} Title`
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if s := title(info, got); s != "Title" {
		t.Errorf("title = %q, want %q", s, "Title")
	}
}

func TestParseFenceInfoBareKind(t *testing.T) {
	const info = "callout"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "callout" {
		t.Errorf("kind = %q, want %q", got.Kind, "callout")
	}
	if s := title(info, got); s != "" {
		t.Errorf("title = %q, want none", s)
	}
}

func TestParseFenceInfoBareKindWithTitle(t *testing.T) {
	const info = "aside Why this matters"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "aside" {
		t.Errorf("kind = %q, want %q", got.Kind, "aside")
	}
	if s := title(info, got); s != "Why this matters" {
		t.Errorf("title = %q, want %q", s, "Why this matters")
	}
}

// The kind outside the braces, with a real attribute block. Today this
// spelling puts the literal braces in the title; nothing can be relying on
// that, so making it work breaks nothing.
func TestParseFenceInfoKindThenAttrs(t *testing.T) {
	const info = "aside {#id .compact}"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "aside" {
		t.Errorf("kind = %q, want %q", got.Kind, "aside")
	}
	if s := title(info, got); s != "" {
		t.Errorf("title = %q, want none — the braces are attributes", s)
	}
	var id, class string
	for _, a := range got.Attrs {
		switch a.Name {
		case "id":
			id = a.Value
		case "class":
			class = a.Value
		}
	}
	if id != "id" || class != "compact" {
		t.Errorf("id = %q, class = %q; want id / compact", id, class)
	}
}

func TestParseFenceInfoKindThenTitleThenAttrs(t *testing.T) {
	const info = "aside Why this matters {#w}"
	got, ok := parseFenceInfo(info)
	if !ok {
		t.Fatalf("parseFenceInfo(%q) reported false", info)
	}
	if got.Kind != "aside" {
		t.Errorf("kind = %q, want %q", got.Kind, "aside")
	}
	if s := title(info, got); s != "Why this matters" {
		t.Errorf("title = %q, want %q", s, "Why this matters")
	}
	if len(got.Attrs) != 1 || got.Attrs[0].Name != "id" || got.Attrs[0].Value != "w" {
		t.Errorf("attrs = %v, want id=w", got.Attrs)
	}
}
