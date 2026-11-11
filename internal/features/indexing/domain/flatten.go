// Package domain keeps the workspace index up to date: which files are
// indexed, and what each of them declares.
package domain

import (
	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
)

// classLikes contain members; their qualified name is the container of
// those members.
var classLikes = map[php.SymbolKind]bool{
	php.KindClass:     true,
	php.KindInterface: true,
	php.KindTrait:     true,
	php.KindEnum:      true,
}

// flatten turns the outline of src into workspace symbols with positions in
// enc. Namespaces are not symbols themselves; they name the container of
// what they declare.
func flatten(uri, src string, decls []php.Declaration, enc text.Encoding) []php.WorkspaceSymbol {
	f := flattener{uri: uri, index: text.NewLineIndex(src), enc: enc}
	f.walk(decls, "")
	return f.out
}

type flattener struct {
	uri   string
	index *text.LineIndex
	enc   text.Encoding
	out   []php.WorkspaceSymbol
}

func (f *flattener) walk(decls []php.Declaration, container string) {
	for _, d := range decls {
		if d.Kind == php.KindNamespace {
			f.walk(d.Children, d.Name)
			continue
		}
		f.out = append(f.out, php.WorkspaceSymbol{
			Name:           d.Name,
			Kind:           d.Kind,
			Container:      container,
			URI:            f.uri,
			Range:          f.span(d.Start, d.End),
			SelectionRange: f.span(d.NameStart, d.NameEnd),
		})
		inner := container
		if classLikes[d.Kind] {
			inner = qualify(container, d.Name)
		}
		f.walk(d.Children, inner)
	}
}

func (f *flattener) span(start, end int) text.Range {
	return text.Range{Start: f.index.Position(start, f.enc), End: f.index.Position(end, f.enc)}
}

// qualify joins a namespace or class name and a name inside it.
func qualify(container, name string) string {
	if container == "" {
		return name
	}
	return container + `\` + name
}
