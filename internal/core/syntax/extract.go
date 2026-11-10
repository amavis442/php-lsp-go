package syntax

import (
	"strings"

	ts "github.com/odvcencio/gotreesitter"

	"github.com/amavis442/php-lsp-go/internal/core/php"
)

// Extract parses src and returns its declarations. Broken code still yields
// every declaration the parser could recover around the errors.
func (p *Parser) Extract(src []byte) (php.FileSummary, error) {
	var summary php.FileSummary
	err := p.withTree(src, func(root *ts.Node) {
		x := extractor{lang: p.lang, src: src}
		summary.Declarations = x.program(root)
	})
	return summary, err
}

// extractor turns a syntax tree into declarations. It copies everything it
// keeps out of the tree, so the result outlives the tree.
type extractor struct {
	lang *ts.Language
	src  []byte
}

// classLikes are the declarations whose body holds members.
var classLikes = map[string]php.SymbolKind{
	"class_declaration":     php.KindClass,
	"interface_declaration": php.KindInterface,
	"trait_declaration":     php.KindTrait,
	"enum_declaration":      php.KindEnum,
}

// program extracts the top level. A namespace statement without braces
// applies to everything up to the next one, so those declarations become
// its children.
func (x extractor) program(root *ts.Node) []php.Declaration {
	var out []php.Declaration
	var ns *php.Declaration
	flush := func() {
		if ns != nil {
			out = append(out, *ns)
			ns = nil
		}
	}
	for i := range root.NamedChildCount() {
		n := root.NamedChild(i)
		if x.kind(n) == "namespace_definition" && x.field(n, "body") == nil {
			flush()
			if d, ok := x.named(n, php.KindNamespace); ok {
				ns = &d
			}
			continue
		}
		decls := x.declarations(n)
		if ns == nil {
			out = append(out, decls...)
			continue
		}
		for _, d := range decls {
			ns.Children = append(ns.Children, d)
			ns.End = max(ns.End, d.End)
		}
	}
	flush()
	return out
}

// declarations returns the declarations n makes, searching through
// statements (such as if blocks) that are not declarations themselves.
func (x extractor) declarations(n *ts.Node) []php.Declaration {
	t := x.kind(n)
	if kind, ok := classLikes[t]; ok {
		return x.withBody(n, kind)
	}
	switch t {
	case "namespace_definition":
		return x.bracedNamespace(n)
	case "function_definition":
		return x.withBody(n, php.KindFunction)
	case "method_declaration":
		return x.method(n)
	case "const_declaration":
		return x.elements(n, "const_element", php.KindConstant)
	case "property_declaration":
		return x.elements(n, "property_element", php.KindProperty)
	case "enum_case":
		return x.single(n, php.KindEnumCase)
	case "anonymous_function", "arrow_function", "anonymous_class":
		return nil // nameless, so not part of the outline
	}
	return x.children(n)
}

func (x extractor) children(n *ts.Node) []php.Declaration {
	var out []php.Declaration
	for i := range n.NamedChildCount() {
		out = append(out, x.declarations(n.NamedChild(i))...)
	}
	return out
}

// withBody is a named declaration whose body holds further declarations.
func (x extractor) withBody(n *ts.Node, kind php.SymbolKind) []php.Declaration {
	d, ok := x.named(n, kind)
	if !ok {
		return x.children(n)
	}
	if body := x.field(n, "body"); body != nil {
		d.Children = x.children(body)
	}
	return []php.Declaration{d}
}

// bracedNamespace is "namespace X { ... }". The global form without a name
// adds nothing to the outline but its contents.
func (x extractor) bracedNamespace(n *ts.Node) []php.Declaration {
	body := x.field(n, "body")
	d, ok := x.named(n, php.KindNamespace)
	if !ok {
		if body == nil {
			return nil
		}
		return x.children(body)
	}
	if body != nil {
		d.Children = x.children(body)
	}
	return []php.Declaration{d}
}

// method lists constructor-promoted parameters as the properties they are.
func (x extractor) method(n *ts.Node) []php.Declaration {
	d, ok := x.named(n, php.KindMethod)
	if !ok {
		return x.children(n)
	}
	if strings.EqualFold(d.Name, "__construct") {
		d.Kind = php.KindConstructor
	}
	if params := x.field(n, "parameters"); params != nil {
		for i := range params.NamedChildCount() {
			p := params.NamedChild(i)
			if x.kind(p) == "property_promotion_parameter" {
				d.Children = append(d.Children, x.single(p, php.KindProperty)...)
			}
		}
	}
	if body := x.field(n, "body"); body != nil {
		d.Children = append(d.Children, x.children(body)...)
	}
	return []php.Declaration{d}
}

// elements handles statements that may declare several names at once, such
// as "const A = 1, B = 2;" or "private $a, $b;". A statement with one name
// spans the whole statement, including its type and property hooks.
func (x extractor) elements(n *ts.Node, element string, kind php.SymbolKind) []php.Declaration {
	var out []php.Declaration
	for i := range n.NamedChildCount() {
		if c := n.NamedChild(i); x.kind(c) == element {
			out = append(out, x.single(c, kind)...)
		}
	}
	if len(out) == 1 {
		out[0].Start, out[0].End = int(n.StartByte()), int(n.EndByte())
	}
	return out
}

func (x extractor) single(n *ts.Node, kind php.SymbolKind) []php.Declaration {
	if d, ok := x.named(n, kind); ok {
		return []php.Declaration{d}
	}
	return nil
}

// named builds a declaration spanning n. It fails when n has no name, as
// in code the parser had to recover from.
func (x extractor) named(n *ts.Node, kind php.SymbolKind) (php.Declaration, bool) {
	name := x.nameOf(n)
	if name == nil || name.StartByte() == name.EndByte() {
		return php.Declaration{}, false
	}
	return php.Declaration{
		Name:      string(x.src[name.StartByte():name.EndByte()]),
		Kind:      kind,
		Start:     int(n.StartByte()),
		End:       int(n.EndByte()),
		NameStart: int(name.StartByte()),
		NameEnd:   int(name.EndByte()),
	}, true
}

// nameOf finds the node that names n: its "name" field, or else its first
// name or variable child (const and property elements have no field).
func (x extractor) nameOf(n *ts.Node) *ts.Node {
	if name := x.field(n, "name"); name != nil {
		return name
	}
	for i := range n.NamedChildCount() {
		switch c := n.NamedChild(i); x.kind(c) {
		case "name", "variable_name", "namespace_name":
			return c
		}
	}
	return nil
}

func (x extractor) field(n *ts.Node, name string) *ts.Node {
	return n.ChildByFieldName(name, x.lang)
}

func (x extractor) kind(n *ts.Node) string { return n.Type(x.lang) }
