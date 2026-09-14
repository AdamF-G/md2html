package md2html

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// figDoc is one figure: the whole YAML document inside a ```fig fence.
type figDoc struct {
	Caption  string    `yaml:"caption"`
	Layout   string    `yaml:"layout"`
	Boundary string    `yaml:"boundary"`
	Items    []figItem `yaml:"items"`
}

// figStat is one tile in a stats row.
type figStat struct {
	Value string `yaml:"value"`
	Label string `yaml:"label"`
}

// figDef is one row of a term/definition grid.
type figDef struct {
	Term string `yaml:"term"`
	Def  string `yaml:"def"`
}

// figItem is one item in a figure. Exactly one kind key must be set; the
// rest are modifiers.
//
// Every string-valued kind is a *string rather than a string so that an
// absent key and an empty one are distinguishable: `box: ""` is a
// deliberately blank spacer and renders, while a missing box names no kind
// at all. A nil pointer is also what `box:` with no value decodes to, so
// that reads as "no kind" too — validation says as much, and names the fix.
type figItem struct {
	Box    *string `yaml:"box"`
	Arrow  *string `yaml:"arrow"`
	Result *string `yaml:"result"`
	Rail   *string `yaml:"rail"`
	Group  *string `yaml:"group"`

	Stats []figStat   `yaml:"stats"`
	Defs  []figDef    `yaml:"defs"`
	Chain []figItem   `yaml:"chain"`
	Lanes [][]figItem `yaml:"lanes"`

	Items  []figItem `yaml:"items"`
	Weight int       `yaml:"weight"`

	// Modifiers are plain values, not pointers. The absent-versus-empty
	// distinction that makes every kind key a *string does not apply: a
	// modifier names no kind, so an empty one and a missing one mean the
	// same thing and render the same way.
	Note   string `yaml:"note"`
	Accent bool   `yaml:"accent"`
}

// kinds reports every kind key set on an item. Exactly one is legal; the
// slice is returned rather than a count so a diagnostic can name them.
func (it figItem) kinds() []string {
	var out []string
	for _, k := range []struct {
		name string
		set  bool
	}{
		{"box", it.Box != nil},
		{"arrow", it.Arrow != nil},
		{"result", it.Result != nil},
		{"rail", it.Rail != nil},
		{"group", it.Group != nil},
		{"stats", it.Stats != nil},
		{"defs", it.Defs != nil},
		{"chain", it.Chain != nil},
		{"lanes", it.Lanes != nil},
	} {
		if k.set {
			out = append(out, k.name)
		}
	}
	return out
}

// validateFig enforces what the decoder cannot see. KnownFields rejects a
// key no kind defines, but one struct carries every kind's fields, so only
// validation knows whether a key is legal *here*: that an item names one
// kind and not two, and that weight appears only on a panel of a cols
// layout.
//
// Errors carry an item path (items[1].chain[0]) rather than a line number.
// Retaining lines would need a custom UnmarshalYAML on figItem, and
// yaml.v3 does not apply KnownFields inside one — the strictness is worth
// more than the line.
func validateFig(doc figDoc) error {
	if doc.Layout != "" && doc.Layout != "rows" && doc.Layout != "cols" && doc.Layout != "split" {
		return fmt.Errorf("layout %q is not rows, cols or split", doc.Layout)
	}
	if doc.Layout == "split" && len(doc.Items) != 2 {
		return fmt.Errorf("a split layout needs exactly 2 items, got %d", len(doc.Items))
	}
	return validateItems(doc.Items, "items", doc.Layout == "cols")
}

// validateItems walks one level of items. weightOK is true only for the
// top level of a cols layout, so nesting cannot smuggle a weight in.
func validateItems(items []figItem, path string, weightOK bool) error {
	for i, it := range items {
		at := fmt.Sprintf("%s[%d]", path, i)
		switch ks := it.kinds(); len(ks) {
		case 1:
		case 0:
			return fmt.Errorf(`%s names no kind (a blank box is box: "")`, at)
		default:
			return fmt.Errorf("%s names %d kinds (%s); an item is exactly one",
				at, len(ks), strings.Join(ks, ", "))
		}
		if it.Weight != 0 && !weightOK {
			return fmt.Errorf("%s carries weight, which is only legal on a panel of a cols layout", at)
		}
		// note and accent share a predicate: both are legal on any kind
		// that has a label to hang them beside. A group's label is its
		// title, so a group takes both; Task 2 renders them.
		if it.Note != "" && it.Box == nil && it.Result == nil && it.Rail == nil && it.Group == nil {
			return fmt.Errorf("%s carries note, which only a box, result, rail or group takes", at)
		}
		if it.Accent && it.Box == nil && it.Result == nil && it.Rail == nil && it.Group == nil {
			return fmt.Errorf("%s carries accent, which only a box, result, rail or group takes", at)
		}
		if it.Items != nil && it.Group == nil {
			return fmt.Errorf("%s carries items, which only a group takes", at)
		}
		if err := validateItems(it.Items, at+".items", false); err != nil {
			return err
		}
		if err := validateItems(it.Chain, at+".chain", false); err != nil {
			return err
		}
		for j, lane := range it.Lanes {
			if err := validateItems(lane, fmt.Sprintf("%s.lanes[%d]", at, j), false); err != nil {
				return err
			}
		}
	}
	return nil
}

// figTypeNouns translates the Go type names yaml.v3 quotes in its errors
// into the vocabulary the fence language actually uses. An author who wrote
// a fig fence has never heard of md2html.figItem, and a warning that names
// it is asking them to read this package's source to understand their typo.
var figTypeNouns = map[string]string{
	"figDoc":  "figure",
	"figItem": "item",
	"figStat": "stat tile",
	"figDef":  "definition",
}

var (
	figGoType     = regexp.MustCompile(`(\[\])?md2html\.(\w+)`)
	figUnknownKey = regexp.MustCompile(`^line (\d+): field (\S+) not found in type \S+$`)
	figLinePrefix = regexp.MustCompile(`^line (\d+): `)
	figWhitespace = regexp.MustCompile(`\s+`)
)

// figFault renders one fault as a single line of author-facing prose.
//
// Two properties matter and neither is cosmetic. One line, because the CLI
// prints every warning as "md2html: <file>: <msg>" — a second line escapes
// that prefix and breaks the one-line-per-warning format every other
// warning in this tool honours. And no Go type names, because yaml.v3
// reports a misspelled key as "field boxes not found in type
// md2html.figItem", which names an implementation detail instead of the
// mistake.
//
// The line numbers yaml.v3 reports count from the start of the *fence
// body*, not the document, so they are labelled as such: a reader who is
// not told will reasonably count from the top of the file and land
// somewhere unrelated.
func figFault(err error) string {
	var parts []string
	var te *yaml.TypeError
	if errors.As(err, &te) {
		parts = te.Errors
	} else {
		parts = []string{strings.TrimPrefix(err.Error(), "yaml: ")}
	}
	for i, p := range parts {
		p = figWhitespace.ReplaceAllString(strings.TrimSpace(p), " ")
		if m := figUnknownKey.FindStringSubmatch(p); m != nil {
			parts[i] = fmt.Sprintf("unknown key %q (line %s of the fence body)", m[2], m[1])
			continue
		}
		p = figGoType.ReplaceAllStringFunc(p, func(t string) string {
			m := figGoType.FindStringSubmatch(t)
			noun, ok := figTypeNouns[m[2]]
			if !ok {
				noun = "value"
			}
			if m[1] == "[]" {
				return "a list of " + noun + "s"
			}
			return "a " + noun
		})
		parts[i] = figLinePrefix.ReplaceAllString(p, "line $1 of the fence body: ")
	}
	return figWhitespace.ReplaceAllString(strings.Join(parts, "; "), " ")
}

// parseFig decodes a fence body. KnownFields(true) is the point of using a
// real decoder: a misspelled key is an error that degrades visibly rather
// than an item that silently vanishes.
//
// The decoder is asked for a *second* document and the body is rejected if
// one arrives. yaml.Decoder yields one document per Decode call, so a body
// containing a `---` separator would otherwise render its first document
// and discard the rest with no diagnostic at all — which is the exact
// silent-vanishing failure KnownFields exists to prevent, arriving through
// another door. A fence describes one figure; a body that describes two is
// a fault like any other and degrades to the source.
//
// io.EOF from the *first* Decode means the fence had no content. That is
// also a fault, but reporting the decoder's "EOF" tells an author nothing,
// so it is translated here into the thing that is actually wrong.
func parseFig(body []byte) (figDoc, error) {
	var doc figDoc
	dec := yaml.NewDecoder(bytes.NewReader(body))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return figDoc{}, errors.New("the fence is empty")
		}
		return figDoc{}, err
	}
	// A clean io.EOF here is the only outcome that means "that was all of
	// it". Anything else — a second document, or a syntax error inside one
	// — is content this figure would have dropped.
	var more figDoc
	if err := dec.Decode(&more); !errors.Is(err, io.EOF) {
		return figDoc{}, errors.New(
			"the body holds more than one YAML document; a fence is one figure, so remove the --- separator")
	}
	return doc, nil
}

// renderFig renders a fence body as figure markup, reporting whether it
// could. A false return means the caller emits the raw body as a code
// block; renderFig has already warned by then, exactly once.
func renderFig(body []byte, warn func(string)) (string, bool) {
	if warn == nil {
		warn = func(string) {}
	}
	doc, err := parseFig(body)
	if err != nil {
		warn(fmt.Sprintf("fig fence: %s; rendering it as a code block", figFault(err)))
		return "", false
	}
	if err := validateFig(doc); err != nil {
		warn(fmt.Sprintf("fig fence: %s; rendering it as a code block", figFault(err)))
		return "", false
	}

	r := newFigRenderer()
	var b strings.Builder
	b.WriteString(`<figure class="fig">`)
	switch doc.Layout {
	case "cols":
		b.WriteString(`<div class="fig-cols">`)
		for _, it := range doc.Items {
			b.WriteString(r.panel(it))
		}
		b.WriteString(`</div>`)
	case "split":
		// validateFig has already guaranteed exactly two items.
		b.WriteString(`<div class="fig-split">`)
		b.WriteString(r.panel(doc.Items[0]))
		b.WriteString(`<div class="fig-boundary">`)
		b.WriteString(r.inline(doc.Boundary))
		b.WriteString(`</div>`)
		b.WriteString(r.panel(doc.Items[1]))
		b.WriteString(`</div>`)
	default:
		b.WriteString(`<div class="fig-rows">`)
		b.WriteString(r.items(doc.Items))
		b.WriteString(`</div>`)
	}
	if doc.Caption != "" {
		b.WriteString(`<figcaption>`)
		b.WriteString(r.inline(doc.Caption))
		b.WriteString(`</figcaption>`)
	}
	b.WriteString(`</figure>`)
	return b.String(), true
}
