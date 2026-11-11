package syntax

import (
	ts "github.com/odvcencio/gotreesitter"

	"github.com/amavis442/php-lsp-go/internal/core/php"
)

// imports adds what the use statement n imports to scope: plain clauses,
// and groups that share a namespace prefix ("use A\{B, C as D};"). The
// statement may set the kind for all its clauses ("use function ..."), or
// a clause in a group for itself.
func (x extractor) imports(n *ts.Node, scope *php.Scope) {
	kind := x.useKind(n, php.UseClass)
	prefix := ""
	for i := range n.NamedChildCount() {
		c := n.NamedChild(i)
		switch x.kind(c) {
		case "namespace_name":
			prefix = x.text(c) + `\`
		case "namespace_use_clause":
			x.useClause(c, kind, "", scope)
		case "namespace_use_group":
			for j := range c.NamedChildCount() {
				if clause := c.NamedChild(j); x.kind(clause) == "namespace_use_clause" {
					x.useClause(clause, kind, prefix, scope)
				}
			}
		}
	}
}

// useClause imports one "Name" or "Name as Alias".
func (x extractor) useClause(c *ts.Node, kind php.UseKind, prefix string, scope *php.Scope) {
	kind = x.useKind(c, kind)
	var name, alias string
	for i := range c.NamedChildCount() {
		switch k := c.NamedChild(i); x.kind(k) {
		case "name", "qualified_name":
			if name == "" {
				name = x.text(k)
			} else {
				alias = x.text(k)
			}
		}
	}
	if name != "" {
		scope.Import(kind, prefix+name, alias)
	}
}

// useKind reads the "function" or "const" keyword of a use statement or
// clause, and returns def without one.
func (x extractor) useKind(n *ts.Node, def php.UseKind) php.UseKind {
	for i := range n.ChildCount() {
		c := n.Child(i)
		if c.IsNamed() {
			continue
		}
		switch x.kind(c) {
		case "function":
			return php.UseFunction
		case "const":
			return php.UseConstant
		}
	}
	return def
}

// names returns the class names written as named children of n, resolved
// in the current scope.
func (x extractor) names(n *ts.Node) []string {
	var out []string
	for i := range n.NamedChildCount() {
		switch c := n.NamedChild(i); x.kind(c) {
		case "name", "qualified_name":
			out = append(out, x.scope.ResolveClass(x.text(c)))
		}
	}
	return out
}

func (x extractor) text(n *ts.Node) string {
	return string(x.src[n.StartByte():n.EndByte()])
}
