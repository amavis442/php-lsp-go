// Package domain answers questions about the symbols a document declares.
package domain

import "github.com/amavis442/php-lsp-go/internal/core/php"

// Extractor finds the declarations in PHP source.
type Extractor interface {
	Extract(src []byte) (php.FileSummary, error)
}

// DocumentSource gives the current text of open documents.
type DocumentSource interface {
	Text(uri string) (string, bool)
}

// Outline is the declarations of one version of a document, together with
// the text their byte offsets point into.
type Outline struct {
	Text         string
	Declarations []php.Declaration
}

// DocumentSymbols builds the outline of open documents.
type DocumentSymbols struct {
	docs      DocumentSource
	extractor Extractor
}

// NewDocumentSymbols returns a use case that outlines the documents in docs.
func NewDocumentSymbols(docs DocumentSource, extractor Extractor) *DocumentSymbols {
	return &DocumentSymbols{docs: docs, extractor: extractor}
}

// For returns the outline of the open document uri. A document that is not
// open has an empty outline.
func (s *DocumentSymbols) For(uri string) (Outline, error) {
	content, ok := s.docs.Text(uri)
	if !ok {
		return Outline{}, nil
	}
	summary, err := s.extractor.Extract([]byte(content))
	if err != nil {
		return Outline{}, err
	}
	return Outline{Text: content, Declarations: summary.Declarations}, nil
}
