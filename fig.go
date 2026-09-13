package md2html

import (
	"bytes"
	"fmt"
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
}

// parseFig decodes a fence body. KnownFields(true) is the point of using a
// real decoder: a misspelled key is an error that degrades visibly rather
// than an item that silently vanishes.
func parseFig(body []byte) (figDoc, error) {
	var doc figDoc
	dec := yaml.NewDecoder(bytes.NewReader(body))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return figDoc{}, err
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
		warn(fmt.Sprintf("fig fence: %v; rendering it as a code block", err))
		return "", false
	}

	r := newFigRenderer()
	var b strings.Builder
	b.WriteString(`<figure class="fig">`)
	b.WriteString(`<div class="fig-rows">`)
	for _, it := range doc.Items {
		b.WriteString(r.item(it))
	}
	b.WriteString(`</div>`)
	if doc.Caption != "" {
		b.WriteString(`<figcaption>`)
		b.WriteString(r.inline(doc.Caption))
		b.WriteString(`</figcaption>`)
	}
	b.WriteString(`</figure>`)
	return b.String(), true
}
