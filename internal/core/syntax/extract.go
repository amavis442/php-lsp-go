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
		x := extractor{lang: p.lang, src: src, scope: &php.Scope{}}
		summary.Declarations = x.program(root)
	})
	return summary, err
}

// extractor turns a syntax tree into declarations. It copies everything it
// keeps out of the tree, so the result outlives the tree.
type extractor struct {
	lang *ts.Language
	src  []byte

	// inNamespace is set inside a named namespace. define is left out
	// there: it always makes a global constant, which the namespace would
	// misname.
	inNamespace bool

	// scope is the namespace and the imports at the statement being
	// extracted; use statements add to it as they are met.
	scope *php.Scope
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
			x.scope = &php.Scope{}
			if d, ok := x.named(n, php.KindNamespace); ok {
				ns = &d
				x.scope.Namespace = d.Name
			}
			continue
		}
		if ns == nil {
			out = append(out, x.declarations(n)...)
			continue
		}
		for _, d := range x.namespaced().declarations(n) {
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
		return x.classLike(n, kind)
	}
	switch t {
	case "namespace_use_declaration":
		x.imports(n, x.scope)
		return nil
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
	case "function_call_expression":
		return x.call(n)
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
	inner := x
	inner.scope = &php.Scope{}
	d, ok := x.named(n, php.KindNamespace)
	if !ok {
		if body == nil {
			return nil
		}
		return inner.children(body)
	}
	if body != nil {
		inner = inner.namespaced()
		inner.scope.Namespace = d.Name
		d.Children = inner.children(body)
	}
	return []php.Declaration{d}
}

// classLike is a class, interface, trait or enum with the names it builds
// on: an interface's base clause lists the interfaces it extends, a
// class's its parent; traits are the use statements in the body.
func (x extractor) classLike(n *ts.Node, kind php.SymbolKind) []php.Declaration {
	d, ok := x.named(n, kind)
	if !ok {
		return x.children(n)
	}
	for i := range n.NamedChildCount() {
		switch c := n.NamedChild(i); x.kind(c) {
		case "base_clause":
			d.Extends = x.names(c)
		case "class_interface_clause":
			d.Implements = x.names(c)
		}
	}
	if kind == php.KindEnum {
		d.Implements = append(d.Implements, x.enumInterface(n))
	}
	if body := x.field(n, "body"); body != nil {
		d.Children = x.children(body)
		for i := range body.NamedChildCount() {
			if c := body.NamedChild(i); x.kind(c) == "use_declaration" {
				d.Traits = append(d.Traits, x.names(c)...)
			}
		}
	}
	return []php.Declaration{d}
}

// namespaced returns the extractor for the inside of a named namespace.
func (x extractor) namespaced() extractor {
	x.inNamespace = true
	return x
}

// call is define('NAME', value) with a literal name: a constant. Other
// calls may hold declarations in their arguments.
func (x extractor) call(n *ts.Node) []php.Declaration {
	if !x.inNamespace && x.isDefine(n) {
		if name := x.definedName(n); name != nil {
			return []php.Declaration{{
				Name:      string(x.src[name.StartByte():name.EndByte()]),
				Kind:      php.KindConstant,
				Start:     int(n.StartByte()),
				End:       int(n.EndByte()),
				NameStart: int(name.StartByte()),
				NameEnd:   int(name.EndByte()),
			}}
		}
	}
	return x.children(n)
}

// isDefine reports whether call n calls the global function define. PHP
// function names ignore case.
func (x extractor) isDefine(n *ts.Node) bool {
	fn := x.field(n, "function")
	if fn == nil {
		return false
	}
	switch x.kind(fn) {
	case "name", "qualified_name":
		name := strings.TrimPrefix(string(x.src[fn.StartByte():fn.EndByte()]), `\`)
		return strings.EqualFold(name, "define")
	}
	return false
}

// definedName is the content of the string literal define names, when it
// is a plain constant name: no interpolation, escapes or namespace.
func (x extractor) definedName(n *ts.Node) *ts.Node {
	args := x.field(n, "arguments")
	if args == nil || args.NamedChildCount() == 0 {
		return nil
	}
	arg := args.NamedChild(0)
	if x.kind(arg) != "argument" || arg.NamedChildCount() != 1 {
		return nil
	}
	lit := arg.NamedChild(0)
	if k := x.kind(lit); (k != "string" && k != "encapsed_string") || lit.NamedChildCount() != 1 {
		return nil
	}
	content := lit.NamedChild(0)
	if x.kind(content) != "string_content" || !isIdentifier(x.src[content.StartByte():content.EndByte()]) {
		return nil
	}
	return content
}

// isIdentifier reports whether b is a PHP name: a letter, underscore or
// byte above 0x7f, followed by those or digits.
func isIdentifier(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for i, c := range b {
		letter := c == '_' || c >= 0x80 || (c|0x20 >= 'a' && c|0x20 <= 'z')
		if !letter && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return true
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

// enumInterface is the built-in interface every enum implements: UnitEnum,
// or BackedEnum (which extends it) when the enum has a backing type.
func (x extractor) enumInterface(n *ts.Node) string {
	for i := range n.NamedChildCount() {
		if x.kind(n.NamedChild(i)) == "primitive_type" {
			return "BackedEnum"
		}
	}
	return "UnitEnum"
}
