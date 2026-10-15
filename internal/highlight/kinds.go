package highlight

import "strings"

// Kind is what a token is, for colouring: one of a small fixed set, or
// Plain.
type Kind uint8

const (
	Plain Kind = iota
	Comment
	Keyword
	String
	Number
	Constant
	Function
	Type
	Property
	Tag
	Attribute
	Inserted
	Deleted
	Heading
)

// Kinds are every kind but Plain, in order.
var Kinds = []Kind{Comment, Keyword, String, Number, Constant, Function, Type,
	Property, Tag, Attribute, Inserted, Deleted, Heading}

var kindNames = [...]string{"", "comment", "keyword", "string", "number", "constant", "function", "type",
	"property", "tag", "attribute", "inserted", "deleted", "heading"}

// Name is the kind's name, "" for Plain.
func (k Kind) Name() string { return kindNames[k] }

// Class is the kind's span class, "" for Plain.
func (k Kind) Class() string {
	if k == Plain {
		return ""
	}
	return "hl-" + kindNames[k]
}

// rules map scope selectors to kinds. A selector matches a scope that is
// it or starts with it and a dot, and the longest match wins. No rule
// names a language suffix: TextMate grammars share these names, so one
// table serves every grammar. A rule for Plain is a decided "no colour",
// which still ends the walk.
//
// Changing this table changes output; record it in the changelog.
var rules = []struct {
	selector string
	kind     Kind
}{
	{"comment", Comment},
	{"keyword", Keyword},
	{"keyword.operator", Plain},
	{"storage", Keyword},
	{"string", String},
	{"constant", Constant},
	{"constant.numeric", Number},
	{"support.constant", Constant},
	{"variable.language", Constant},
	{"entity.name.function", Function},
	{"support.function", Function},
	{"entity.name.type", Type},
	{"entity.name.class", Type},
	{"support.type", Type},
	{"support.class", Type},
	{"entity.other.inherited-class", Type},
	{"support.type.property-name", Property},
	{"variable.other.property", Property},
	{"entity.name.tag", Tag},
	{"entity.other.attribute-name", Attribute},
	{"markup.inserted", Inserted},
	{"markup.deleted", Deleted},
	{"markup.heading", Heading},
	{"meta.diff.header", Heading},
	{"meta.diff.range", Heading},
}

// KindOf walks a scope stack from the innermost scope out. The first scope
// any rule matches decides the kind. A meta.embedded scope, or a source
// scope below the root, ends the walk with Plain, so code embedded in a
// string is not coloured as the string.
func KindOf(scopes []string) Kind {
	for i := len(scopes) - 1; i >= 0; i-- {
		s := scopes[i]
		if i > 0 && (under(s, "meta.embedded") || under(s, "source")) {
			return Plain
		}
		if k, ok := match(s); ok {
			return k
		}
	}
	return Plain
}

func match(scope string) (Kind, bool) {
	best, kind := -1, Plain
	for _, r := range rules {
		if under(scope, r.selector) && len(r.selector) > best {
			best, kind = len(r.selector), r.kind
		}
	}
	return kind, best >= 0
}

// under reports whether scope is sel or lies beneath it on a dot boundary.
func under(scope, sel string) bool {
	return scope == sel || strings.HasPrefix(scope, sel+".")
}
