package highlight

import "testing"

func TestKindOfEachRow(t *testing.T) {
	for _, c := range []struct {
		scope string
		want  Kind
	}{
		{"comment.line.number-sign.python", Comment},
		{"keyword.control.go", Keyword},
		{"storage.type.function.go", Keyword},
		{"keyword.operator.assignment.go", Plain}, // plan decision: operators are plain
		{"string.quoted.double.go", String},
		{"constant.numeric.integer.go", Number},
		{"constant.language.boolean.json", Constant},
		{"support.constant.property-value.css", Constant},
		{"variable.language.this.js", Constant},
		{"constant.character.escape.go", Constant},
		{"entity.name.function.go", Function},
		{"support.function.builtin.go", Function},
		{"entity.name.type.go", Type},
		{"entity.name.class.python", Type},
		{"support.type.primitive.ts", Type},
		{"support.class.builtin.python", Type},
		{"entity.other.inherited-class.python", Type},
		{"support.type.property-name.json", Property},
		{"variable.other.property.js", Property},
		{"entity.name.tag.html", Tag},
		{"entity.name.tag.yaml", Tag}, // no per-language exceptions
		{"entity.other.attribute-name.html", Attribute},
		{"markup.inserted.diff", Inserted},
		{"markup.deleted.diff", Deleted},
		{"markup.heading.markdown", Heading},
		{"meta.diff.header.from-file", Heading},
		{"meta.diff.range.unified", Heading},
		{"variable.other.readwrite.go", Plain},
		{"str", Plain}, // prefixes are on dot boundaries
	} {
		if got := KindOf([]string{"source.x", c.scope}); got != c.want {
			t.Errorf("%s: %v, want %v", c.scope, got.Name(), c.want.Name())
		}
	}
}

func TestKindOfWalk(t *testing.T) {
	for _, c := range []struct {
		name   string
		scopes []string
		want   Kind
	}{
		{"a string's quote takes string", []string{"source.go", "string.quoted.double.go", "punctuation.definition.string.begin.go"}, String},
		{"a comment's marker takes comment", []string{"source.python", "comment.line.number-sign.python", "punctuation.definition.comment.python"}, Comment},
		{"meta never matches on its own", []string{"source.go", "meta.function.go"}, Plain},
		{"innermost match wins", []string{"source.go", "string.quoted.go", "constant.character.escape.go"}, Constant},
		{"meta.embedded stops the walk", []string{"source.python", "string.interpolated.python", "meta.embedded.expression.python"}, Plain},
		{"meta.embedded still lets inner scopes match", []string{"source.python", "string.interpolated.python", "meta.embedded.expression.python", "constant.numeric.python"}, Number},
		{"an embedded source stops the walk", []string{"source.shell", "string.unquoted.heredoc.shell", "source.python"}, Plain},
		{"the root never stops it", []string{"source.go"}, Plain},
		{"empty", nil, Plain},
	} {
		if got := KindOf(c.scopes); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got.Name(), c.want.Name())
		}
	}
}

func TestKindNamesAndClasses(t *testing.T) {
	if len(Kinds) != 13 {
		t.Fatalf("%d kinds, want 13", len(Kinds))
	}
	want := []string{"comment", "keyword", "string", "number", "constant", "function", "type",
		"property", "tag", "attribute", "inserted", "deleted", "heading"}
	for i, k := range Kinds {
		if k.Name() != want[i] || k.Class() != "hl-"+want[i] {
			t.Errorf("kind %d: %q %q", i, k.Name(), k.Class())
		}
	}
	if Plain.Name() != "" || Plain.Class() != "" {
		t.Error("Plain has a name")
	}
}
