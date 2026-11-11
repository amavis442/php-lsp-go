// Package domain finds the declarations a name in an open document refers
// to.
package domain

import (
	"strings"

	"github.com/amavis442/php-lsp-go/internal/core/fileuri"
	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
)

// Parser reads PHP source.
type Parser interface {
	ReferenceAt(src []byte, offset int) (php.Reference, bool, error)
	Extract(src []byte) (php.FileSummary, error)
}

// DocumentSource gives the text of open documents.
type DocumentSource interface {
	Text(uri string) (string, bool)
}

// SymbolLookup finds indexed symbols by php.KeyOf.
type SymbolLookup interface {
	Lookup(key string) []php.WorkspaceSymbol
}

// Definitions answers goto-definition.
type Definitions struct {
	docs   DocumentSource
	parser Parser
	index  SymbolLookup
}

// NewDefinitions returns definitions that read documents from docs and
// look declarations up in index.
func NewDefinitions(docs DocumentSource, parser Parser, index SymbolLookup) *Definitions {
	return &Definitions{docs: docs, parser: parser, index: index}
}

// At returns the declarations the name at pos in the open document uri
// refers to, with positions in enc. It returns nothing when the document
// is not open, there is no name at pos, or nothing declares it.
func (d *Definitions) At(uri string, pos text.Position, enc text.Encoding) ([]php.WorkspaceSymbol, error) {
	src, ok := d.docs.Text(uri)
	if !ok {
		return nil, nil
	}
	offset := text.NewLineIndex(src).Offset(pos, enc)
	ref, ok, err := d.parser.ReferenceAt([]byte(src), offset)
	if err != nil || !ok {
		return nil, err
	}
	summary, err := d.parser.Extract([]byte(src))
	if err != nil {
		return nil, err
	}
	l := newOverlay(uri, php.Flatten(uri, src, summary.Declarations, enc), d.index)

	switch ref.Kind {
	case php.RefClass:
		return first(ref.Candidates, func(c string) []php.WorkspaceSymbol { return l.lookup(php.ClassKey(c)) }), nil
	case php.RefFunction:
		return first(ref.Candidates, func(c string) []php.WorkspaceSymbol { return l.lookup(php.FunctionKey(c)) }), nil
	case php.RefConstant:
		return first(ref.Candidates, func(c string) []php.WorkspaceSymbol { return l.lookup(php.ConstantKey(c)) }), nil
	case php.RefMember:
		return first(ref.Candidates, func(c string) []php.WorkspaceSymbol {
			return l.member(c, ref.MemberKind, ref.Member, map[string]bool{})
		}), nil
	}
	return nil, nil
}

// first returns what the first candidate that finds anything finds.
func first(candidates []string, find func(string) []php.WorkspaceSymbol) []php.WorkspaceSymbol {
	for _, c := range candidates {
		if found := find(c); len(found) > 0 {
			return found
		}
	}
	return nil
}

// overlay looks symbols up in the open document first, then in the index,
// whose copy of that document may be older.
type overlay struct {
	uri   string // canonical URI of the open document
	own   map[string][]php.WorkspaceSymbol
	index SymbolLookup
}

func newOverlay(uri string, symbols []php.WorkspaceSymbol, index SymbolLookup) overlay {
	own := make(map[string][]php.WorkspaceSymbol)
	for _, s := range symbols {
		own[php.KeyOf(s)] = append(own[php.KeyOf(s)], s)
	}
	return overlay{uri: fileuri.CanonicalURI(uri), own: own, index: index}
}

func (o overlay) lookup(key string) []php.WorkspaceSymbol {
	out := append([]php.WorkspaceSymbol(nil), o.own[key]...)
	for _, s := range o.index.Lookup(key) {
		if fileuri.CanonicalURI(s.URI) != o.uri {
			out = append(out, s)
		}
	}
	return out
}

// member finds name in class, then in what it builds on: its traits, whose
// members it takes in, its parent class, and its interfaces. seen stops
// inheritance cycles.
func (o overlay) member(class string, kind php.SymbolKind, name string, seen map[string]bool) []php.WorkspaceSymbol {
	if seen[strings.ToLower(class)] {
		return nil
	}
	seen[strings.ToLower(class)] = true
	if found := o.lookup(php.MemberKey(class, kind, name)); len(found) > 0 {
		return found
	}
	for _, c := range o.lookup(php.ClassKey(class)) {
		for _, next := range [][]string{c.Traits, c.Extends, c.Implements} {
			for _, n := range next {
				if found := o.member(n, kind, name, seen); len(found) > 0 {
					return found
				}
			}
		}
	}
	return nil
}
