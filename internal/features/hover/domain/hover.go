// Package domain builds what hover shows for the name at a position.
package domain

import (
	"fmt"

	"github.com/amavis442/php-lsp-go/internal/core/fileuri"
	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
)

// Resolver finds the declarations the name at a position refers to.
type Resolver interface {
	At(uri string, pos text.Position, enc text.Encoding) ([]php.WorkspaceSymbol, error)
}

// DocumentSource gives the text of open documents.
type DocumentSource interface {
	Text(uri string) (string, bool)
}

// Files reads files from disk.
type Files interface {
	Read(path string) ([]byte, error)
}

// Describer describes the declaration whose name starts at offset.
type Describer interface {
	Describe(src []byte, offset int) (php.Description, bool, error)
}

// Info is what hover shows: the declaration, and the class or namespace it
// belongs to.
type Info struct {
	php.Description
	Container string
}

// Hover answers hover requests.
type Hover struct {
	resolver  Resolver
	docs      DocumentSource
	files     Files
	describer Describer
}

// NewHover returns a hover that finds declarations with resolver and
// reads them from docs or files.
func NewHover(resolver Resolver, docs DocumentSource, files Files, describer Describer) *Hover {
	return &Hover{resolver: resolver, docs: docs, files: files, describer: describer}
}

// At describes the declaration the name at pos in document uri refers to.
// With several, it describes the first. It reports false when there is
// none.
func (h *Hover) At(uri string, pos text.Position, enc text.Encoding) (Info, bool, error) {
	found, err := h.resolver.At(uri, pos, enc)
	if err != nil || len(found) == 0 {
		return Info{}, false, err
	}
	s := found[0]
	src, err := h.source(uri, s.URI)
	if err != nil {
		return Info{}, false, err
	}
	offset := text.NewLineIndex(src).Offset(s.SelectionRange.Start, enc)
	d, ok, err := h.describer.Describe([]byte(src), offset)
	if err != nil || !ok {
		return Info{}, false, err
	}
	return Info{Description: d, Container: s.Container}, true, nil
}

// source is the text the positions of a declaration in file belong to: the
// editor's text for the requesting document, which the resolver read
// afresh, and the file on disk for any other, as the index read it.
func (h *Hover) source(requesting, file string) (string, error) {
	if file == requesting {
		if t, ok := h.docs.Text(file); ok {
			return t, nil
		}
	}
	path, err := fileuri.ToPath(file)
	if err != nil {
		return "", err
	}
	b, err := h.files.Read(path)
	if err != nil {
		return "", fmt.Errorf("reading the declaration: %w", err)
	}
	return string(b), nil
}
