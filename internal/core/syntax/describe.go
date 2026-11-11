package syntax

import (
	"strings"

	ts "github.com/odvcencio/gotreesitter"

	"github.com/amavis442/php-lsp-go/internal/core/php"
)

// Describe parses src and describes the declaration whose name starts at
// byte offset, as goto-definition finds it. It reports false when no
// declaration is named there.
func (p *Parser) Describe(src []byte, offset int) (php.Description, bool, error) {
	var d php.Description
	var ok bool
	err := p.withTree(src, func(root *ts.Node) {
		x := extractor{lang: p.lang, src: src}
		d, ok = x.describe(root, offset)
	})
	return d, ok, err
}

func (x extractor) describe(root *ts.Node, offset int) (php.Description, bool) {
	n := root.NamedDescendantForByteRange(uint32(offset), uint32(offset))
	if n == nil {
		return php.Description{}, false
	}
	if p := n.Parent(); p != nil && x.kind(n) == "name" && x.kind(p) == "variable_name" {
		n = p
	}
	parent := n.Parent()
	if parent == nil {
		return php.Description{}, false
	}
	switch x.kind(parent) {
	case "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration",
		"function_definition", "method_declaration":
		if !same(n, x.field(parent, "name")) {
			return php.Description{}, false
		}
		return php.Description{Signature: x.header(parent), Summary: x.summary(parent)}, true
	case "const_element", "property_element":
		decl := parent.Parent()
		if decl == nil {
			return php.Description{}, false
		}
		return php.Description{Signature: x.element(decl, parent), Summary: x.summary(decl)}, true
	case "property_promotion_parameter":
		return php.Description{Signature: x.signature(parent, int(parent.EndByte())), Summary: x.summary(parent)}, true
	case "enum_case":
		return php.Description{Signature: x.signature(parent, int(parent.EndByte())), Summary: x.summary(parent)}, true
	case "string", "encapsed_string":
		return x.describeDefine(parent)
	}
	return php.Description{}, false
}

// header is a class or function up to its body; an abstract or interface
// method has none and ends at its semicolon.
func (x extractor) header(decl *ts.Node) string {
	end := int(decl.EndByte())
	if body := x.field(decl, "body"); body != nil {
		end = int(body.StartByte())
	}
	return x.signature(decl, end)
}

// element is one name of a statement that may declare several, with the
// statement's modifiers and type: "public const A = 1" of
// "public const A = 1, B = 2;".
func (x extractor) element(decl, element *ts.Node) string {
	var first *ts.Node
	for i := range decl.NamedChildCount() {
		if c := decl.NamedChild(i); x.kind(c) == x.kind(element) {
			first = c
			break
		}
	}
	prefix := x.signature(decl, int(first.StartByte()))
	return strings.TrimSpace(prefix + " " + x.signature(element, int(element.EndByte())))
}

// describeDefine describes define('NAME', value) as the constant it makes.
func (x extractor) describeDefine(literal *ts.Node) (php.Description, bool) {
	arg := literal.Parent()
	if arg == nil || x.kind(arg) != "argument" {
		return php.Description{}, false
	}
	args := arg.Parent()
	call := args.Parent()
	if call == nil || x.kind(call) != "function_call_expression" || !x.isDefine(call) ||
		args.NamedChildCount() < 2 || !same(args.NamedChild(0), arg) {
		return php.Description{}, false
	}
	name := x.definedName(call)
	if name == nil {
		return php.Description{}, false
	}
	value := args.NamedChild(1)
	sig := "const " + x.text(name) + " = " + x.signature(value, int(value.EndByte()))
	doc := call
	if stmt := call.Parent(); stmt != nil && x.kind(stmt) == "expression_statement" {
		doc = stmt
	}
	return php.Description{Signature: sig, Summary: x.summary(doc)}, true
}

// signature is the source of n from after its attributes up to end, on
// one line, without a trailing semicolon or property hooks.
func (x extractor) signature(n *ts.Node, end int) string {
	start := int(n.StartByte())
	leading := true
	for i := range n.NamedChildCount() {
		c := n.NamedChild(i)
		switch k := x.kind(c); {
		case leading && (k == "attribute_list" || k == "comment"):
			start = int(c.EndByte())
		case k == "property_hook_list":
			end = min(end, int(c.StartByte()))
		default:
			leading = false
		}
	}
	return oneLine(x.withoutComments(n, start, max(start, end)))
}

// withoutComments is the source between start and end with the comments
// inside n left out, such as docblocks of promoted parameters.
func (x extractor) withoutComments(n *ts.Node, start, end int) string {
	var b strings.Builder
	at := start
	var walk func(*ts.Node)
	walk = func(c *ts.Node) {
		if int(c.StartByte()) >= end || int(c.EndByte()) <= at {
			return
		}
		if x.kind(c) == "comment" {
			b.Write(x.src[at:c.StartByte()])
			at = int(c.EndByte())
			return
		}
		for i := range c.NamedChildCount() {
			walk(c.NamedChild(i))
		}
	}
	walk(n)
	if at < end {
		b.Write(x.src[at:end])
	}
	return b.String()
}

// oneLine collapses the whitespace of multi-line parameter lists and drops
// the trailing comma and semicolon.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.NewReplacer("( ", "(", ", )", ")", " )", ")").Replace(s)
	return strings.TrimSpace(strings.TrimSuffix(s, ";"))
}

// summary is the text before the first tag of the PHPDoc comment right
// before n.
func (x extractor) summary(n *ts.Node) string {
	prev := n.PrevSibling()
	if prev == nil || x.kind(prev) != "comment" {
		return ""
	}
	if strings.TrimSpace(string(x.src[prev.EndByte():n.StartByte()])) != "" {
		return ""
	}
	return docSummary(x.text(prev))
}

func docSummary(comment string) string {
	if !strings.HasPrefix(comment, "/**") || !strings.HasSuffix(comment, "*/") || len(comment) < 5 {
		return ""
	}
	body := comment[3 : len(comment)-2]
	var lines []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if strings.HasPrefix(line, "@") {
			break
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
