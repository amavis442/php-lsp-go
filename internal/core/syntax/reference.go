package syntax

import (
	"strings"

	ts "github.com/odvcencio/gotreesitter"

	"github.com/amavis442/php-lsp-go/internal/core/php"
)

// ReferenceAt parses src and returns what the name at byte offset refers
// to. It reports false when there is no name there, or when finding out
// needs types: "$object->member" for any object but $this.
func (p *Parser) ReferenceAt(src []byte, offset int) (php.Reference, bool, error) {
	var ref php.Reference
	var ok bool
	err := p.withTree(src, func(root *ts.Node) {
		x := extractor{lang: p.lang, src: src}
		x.scope = x.scopeAt(root, offset)
		ref, ok = x.referenceAt(root, offset)
	})
	return ref, ok, err
}

// scopeAt follows the namespaces and use statements of the file up to
// offset.
func (x extractor) scopeAt(root *ts.Node, offset int) *php.Scope {
	scope := &php.Scope{}
	for i := range root.NamedChildCount() {
		n := root.NamedChild(i)
		if int(n.StartByte()) > offset {
			break
		}
		switch x.kind(n) {
		case "namespace_use_declaration":
			x.imports(n, scope)
		case "namespace_definition":
			scope = &php.Scope{}
			if name := x.field(n, "name"); name != nil {
				scope.Namespace = x.text(name)
			}
			body := x.field(n, "body")
			if body == nil {
				continue
			}
			if int(n.EndByte()) <= offset {
				scope = &php.Scope{} // a braced namespace ends its scope
				continue
			}
			for j := range body.NamedChildCount() {
				if c := body.NamedChild(j); int(c.StartByte()) <= offset && x.kind(c) == "namespace_use_declaration" {
					x.imports(c, scope)
				}
			}
			return scope
		}
	}
	return scope
}

func (x extractor) referenceAt(root *ts.Node, offset int) (php.Reference, bool) {
	n := root.NamedDescendantForByteRange(uint32(offset), uint32(offset))
	if n == nil {
		return php.Reference{}, false
	}
	// A part of a qualified name stands for the whole name, and the name
	// inside "$name" for the variable.
	if x.kind(n) == "name" {
		if p := n.Parent(); p != nil {
			switch x.kind(p) {
			case "namespace_name", "qualified_name", "variable_name":
				n = p
			}
		}
	}
	if x.kind(n) == "namespace_name" {
		p := n.Parent()
		if p == nil || x.kind(p) != "qualified_name" {
			return php.Reference{}, false
		}
		n = p
	}
	switch x.kind(n) {
	case "name", "qualified_name", "variable_name":
	default:
		return php.Reference{}, false
	}
	parent := n.Parent()
	if parent == nil {
		return php.Reference{}, false
	}
	ref, ok := x.classify(n, parent)
	ref.Start, ref.End = int(n.StartByte()), int(n.EndByte())
	return ref, ok
}

// classify decides what name n means from where it stands in parent.
func (x extractor) classify(n, parent *ts.Node) (php.Reference, bool) {
	name := x.text(n)
	isVariable := x.kind(n) == "variable_name"
	switch x.kind(parent) {
	case "object_creation_expression", "base_clause", "class_interface_clause", "named_type",
		"use_declaration", "attribute", "optional_type":
		return x.class(name), !isVariable
	case "binary_expression":
		if x.operator(parent) == "instanceof" && same(n, x.lastNamed(parent)) {
			return x.class(name), true
		}
	case "scoped_call_expression", "class_constant_access_expression", "scoped_property_access_expression":
		return x.scoped(n, parent)
	case "member_call_expression", "nullsafe_member_call_expression":
		return x.onThis(parent, php.KindMethod, name)
	case "member_access_expression", "nullsafe_member_access_expression":
		return x.onThis(parent, php.KindProperty, name)
	case "function_call_expression":
		if same(n, x.field(parent, "function")) {
			return php.Reference{Kind: php.RefFunction, Candidates: x.scope.FunctionCandidates(name)}, true
		}
	case "namespace_use_clause":
		return x.used(n, parent)
	case "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration":
		return php.Reference{Kind: php.RefClass, Candidates: []string{x.declared(name)}}, true
	case "function_definition":
		return php.Reference{Kind: php.RefFunction, Candidates: []string{x.declared(name)}}, true
	case "method_declaration":
		return x.member(n, php.KindMethod, name)
	case "property_element", "property_promotion_parameter":
		return x.member(n, php.KindProperty, name)
	case "enum_case":
		return x.member(n, php.KindConstant, name)
	case "const_element":
		if _, inClass := x.enclosingClass(n); inClass {
			return x.member(n, php.KindConstant, name)
		}
		return php.Reference{Kind: php.RefConstant, Candidates: []string{x.declared(name)}}, true
	case "argument":
		if same(n, x.field(parent, "name")) {
			return php.Reference{}, false // a named argument
		}
	}
	if isVariable || !constantContexts(x.kind(parent)) {
		return php.Reference{}, false
	}
	return php.Reference{Kind: php.RefConstant, Candidates: x.scope.ConstantCandidates(name)}, true
}

// constantContexts are the parents where a bare name is a constant: an
// expression, not a label, namespace or type.
func constantContexts(parent string) bool {
	switch parent {
	case "namespace_definition", "named_label_statement", "goto_statement", "primitive_type",
		"declare_directive", "relative_scope", "cast_type", "visibility_modifier":
		return false
	}
	return true
}

func (x extractor) class(name string) php.Reference {
	return php.Reference{Kind: php.RefClass, Candidates: []string{x.scope.ResolveClass(name)}}
}

// declared is the qualified name of something declared in the current
// namespace.
func (x extractor) declared(name string) string {
	if x.scope.Namespace == "" {
		return name
	}
	return x.scope.Namespace + `\` + name
}

// scoped handles "Scope::member", "Scope::$property" and "Scope::CONST".
func (x extractor) scoped(n, parent *ts.Node) (php.Reference, bool) {
	scope := parent.NamedChild(0)
	if same(n, scope) {
		return x.class(x.text(n)), true
	}
	classes, ok := x.scopeClass(scope)
	if !ok {
		return php.Reference{}, false
	}
	name := x.text(n)
	kind := php.KindConstant
	switch x.kind(parent) {
	case "scoped_call_expression":
		kind = php.KindMethod
	case "scoped_property_access_expression":
		kind = php.KindProperty
	default:
		if name == "class" {
			return php.Reference{Kind: php.RefClass, Candidates: classes}, true
		}
	}
	return php.Reference{Kind: php.RefMember, Candidates: classes, Member: name, MemberKind: kind}, true
}

// scopeClass is the class "X::" names: a class name, or self, static or
// parent relative to the enclosing class.
func (x extractor) scopeClass(scope *ts.Node) ([]string, bool) {
	switch x.kind(scope) {
	case "name", "qualified_name":
		return []string{x.scope.ResolveClass(x.text(scope))}, true
	case "relative_scope":
		class, ok := x.enclosingClass(scope)
		if !ok {
			return nil, false
		}
		if strings.EqualFold(x.text(scope), "parent") {
			parents := x.parentOf(class)
			return parents, len(parents) > 0
		}
		return []string{x.declared(x.text(x.field(class, "name")))}, true
	}
	return nil, false
}

// onThis handles "$this->member"; other objects need types.
func (x extractor) onThis(parent *ts.Node, kind php.SymbolKind, name string) (php.Reference, bool) {
	object := x.field(parent, "object")
	if object == nil || x.text(object) != "$this" {
		return php.Reference{}, false
	}
	class, ok := x.enclosingClass(parent)
	if !ok {
		return php.Reference{}, false
	}
	return php.Reference{
		Kind: php.RefMember, Candidates: []string{x.declared(x.text(x.field(class, "name")))},
		Member: name, MemberKind: kind,
	}, true
}

// member is a member declaration naming itself.
func (x extractor) member(n *ts.Node, kind php.SymbolKind, name string) (php.Reference, bool) {
	class, ok := x.enclosingClass(n)
	if !ok {
		return php.Reference{}, false
	}
	return php.Reference{
		Kind: php.RefMember, Candidates: []string{x.declared(x.text(x.field(class, "name")))},
		Member: name, MemberKind: kind,
	}, true
}

// used is a name in a use statement, which is always fully qualified.
func (x extractor) used(n, clause *ts.Node) (php.Reference, bool) {
	if !same(n, clause.NamedChild(0)) {
		return php.Reference{}, false // the alias
	}
	decl := clause.Parent()
	prefix := ""
	if decl != nil && x.kind(decl) == "namespace_use_group" {
		group := decl
		decl = group.Parent()
		if decl != nil {
			if ns := decl.NamedChild(0); ns != nil && x.kind(ns) == "namespace_name" {
				prefix = x.text(ns) + `\`
			}
		}
	}
	kind := php.UseClass
	if decl != nil {
		kind = x.useKind(decl, kind)
	}
	kind = x.useKind(clause, kind)
	name := strings.TrimPrefix(prefix+x.text(n), `\`)
	switch kind {
	case php.UseFunction:
		return php.Reference{Kind: php.RefFunction, Candidates: []string{name}}, true
	case php.UseConstant:
		return php.Reference{Kind: php.RefConstant, Candidates: []string{name}}, true
	}
	return php.Reference{Kind: php.RefClass, Candidates: []string{name}}, true
}

// enclosingClass is the named class, interface, trait or enum around n.
// An anonymous class has no name to look members up by.
func (x extractor) enclosingClass(n *ts.Node) (*ts.Node, bool) {
	for p := n.Parent(); p != nil; p = p.Parent() {
		switch x.kind(p) {
		case "anonymous_class":
			return nil, false
		case "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration":
			return p, x.field(p, "name") != nil
		}
	}
	return nil, false
}

// parentOf resolves the parent class named in class's extends clause.
func (x extractor) parentOf(class *ts.Node) []string {
	for i := range class.NamedChildCount() {
		if c := class.NamedChild(i); x.kind(c) == "base_clause" {
			return x.names(c)
		}
	}
	return nil
}

// operator is the text of the unnamed operator token of a binary
// expression.
func (x extractor) operator(n *ts.Node) string {
	for i := range n.ChildCount() {
		if c := n.Child(i); !c.IsNamed() {
			return strings.ToLower(x.text(c))
		}
	}
	return ""
}

func (x extractor) lastNamed(n *ts.Node) *ts.Node {
	if c := n.NamedChildCount(); c > 0 {
		return n.NamedChild(c - 1)
	}
	return nil
}

// same reports whether a and b are the same node.
func same(a, b *ts.Node) bool {
	return a != nil && b != nil && a.StartByte() == b.StartByte() && a.EndByte() == b.EndByte()
}
