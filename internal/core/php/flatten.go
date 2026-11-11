package php

import "github.com/amavis442/php-lsp-go/internal/core/text"

// classLikes contain members; their qualified name is the container of
// those members.
var classLikes = map[SymbolKind]bool{
	KindClass:     true,
	KindInterface: true,
	KindTrait:     true,
	KindEnum:      true,
}

// functionLikes are the declarations whose body is code, not members.
var functionLikes = map[SymbolKind]bool{
	KindFunction:    true,
	KindMethod:      true,
	KindConstructor: true,
}

// Flatten turns the outline of src into workspace symbols with positions
// in enc. Namespaces are not symbols themselves; they name the container
// of what they declare.
func Flatten(uri, src string, decls []Declaration, enc text.Encoding) []WorkspaceSymbol {
	f := flattener{uri: uri, index: text.NewLineIndex(src), enc: enc}
	f.walk(decls, "", "", false)
	return f.out
}

type flattener struct {
	uri   string
	index *text.LineIndex
	enc   text.Encoding
	out   []WorkspaceSymbol
}

// walk flattens decls inside namespace ns. class is the qualified name of
// the class they belong to, if any; inCode is set inside a function or
// method body.
func (f *flattener) walk(decls []Declaration, ns, class string, inCode bool) {
	for _, d := range decls {
		if d.Kind == KindNamespace {
			f.walk(d.Children, d.Name, "", false)
			continue
		}
		s := WorkspaceSymbol{
			Name:           d.Name,
			Kind:           d.Kind,
			Container:      ns,
			URI:            f.uri,
			Range:          f.span(d.Start, d.End),
			SelectionRange: f.span(d.NameStart, d.NameEnd),
			Extends:        d.Extends,
			Implements:     d.Implements,
			Traits:         d.Traits,
		}
		// Declarations inside a class are members. In method bodies only the
		// properties a constructor promotes are: a function there is
		// global, and so is a constant, which define makes.
		if class != "" && (!inCode || d.Kind == KindProperty) {
			s.Container, s.Member = class, true
		}
		f.out = append(f.out, s)

		switch {
		case classLikes[d.Kind]:
			f.walk(d.Children, ns, qualify(ns, d.Name), false)
		case functionLikes[d.Kind]:
			f.walk(d.Children, ns, class, true)
		default:
			f.walk(d.Children, ns, class, inCode)
		}
	}
}

func (f *flattener) span(start, end int) text.Range {
	return text.Range{Start: f.index.Position(start, f.enc), End: f.index.Position(end, f.enc)}
}

// qualify joins a namespace and a name inside it.
func qualify(ns, name string) string {
	if ns == "" {
		return name
	}
	return ns + `\` + name
}
