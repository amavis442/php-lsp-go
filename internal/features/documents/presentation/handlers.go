// Package presentation translates the LSP document synchronisation
// notifications into calls on the documents service.
package presentation

import (
	"context"

	"go.lsp.dev/protocol"

	"github.com/amavis442/php-lsp-go/internal/core/text"
	"github.com/amavis442/php-lsp-go/internal/features/documents/domain"
	"github.com/amavis442/php-lsp-go/internal/server"
)

// The parameter types below mirror the LSP ones but keep the URI as a plain
// string. The protocol library normalises URIs while decoding (file:///D:/x
// becomes file:///d%3A/x), and diagnostics must name the document exactly as
// the editor did, or the editor cannot match them to its buffer.

type documentIdentifier struct {
	URI     string `json:"uri"`
	Version int32  `json:"version"`
}

type didOpenParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version int32  `json:"version"`
		Text    string `json:"text"`
	} `json:"textDocument"`
}

type contentChange struct {
	Range *protocol.Range `json:"range"`
	Text  string          `json:"text"`
}

type didChangeParams struct {
	TextDocument   documentIdentifier `json:"textDocument"`
	ContentChanges []contentChange    `json:"contentChanges"`
}

type didCloseParams struct {
	TextDocument documentIdentifier `json:"textDocument"`
}

// Handlers serves textDocument/didOpen, didChange and didClose.
type Handlers struct {
	docs     *domain.Service
	encoding func() text.Encoding
}

// NewHandlers returns handlers that update docs. encoding reports the
// position encoding agreed with the editor.
func NewHandlers(docs *domain.Service, encoding func() text.Encoding) *Handlers {
	return &Handlers{docs: docs, encoding: encoding}
}

// Register adds the handlers to r.
func (h *Handlers) Register(r *server.Router) {
	server.HandleNotification(r, protocol.MethodTextDocumentDidOpen, h.didOpen)
	server.HandleNotification(r, protocol.MethodTextDocumentDidChange, h.didChange)
	server.HandleNotification(r, protocol.MethodTextDocumentDidClose, h.didClose)
}

// Capabilities asks the editor for open/close events and incremental edits.
func (h *Handlers) Capabilities(c *protocol.ServerCapabilities) {
	openClose := true
	change := protocol.TextDocumentSyncKindIncremental
	c.TextDocumentSync = &protocol.TextDocumentSyncOptions{OpenClose: &openClose, Change: &change}
}

func (h *Handlers) didOpen(_ context.Context, p *didOpenParams) error {
	h.docs.Open(p.TextDocument.URI, p.TextDocument.Version, p.TextDocument.Text)
	return nil
}

func (h *Handlers) didChange(_ context.Context, p *didChangeParams) error {
	changes := make([]text.Change, len(p.ContentChanges))
	for i, c := range p.ContentChanges {
		changes[i] = text.Change{Range: toRange(c.Range), Text: c.Text}
	}
	return h.docs.Change(p.TextDocument.URI, p.TextDocument.Version, changes, h.encoding())
}

func (h *Handlers) didClose(_ context.Context, p *didCloseParams) error {
	h.docs.Close(p.TextDocument.URI)
	return nil
}

func toRange(r *protocol.Range) *text.Range {
	if r == nil {
		return nil
	}
	return &text.Range{
		Start: text.Position{Line: int(r.Start.Line), Character: int(r.Start.Character)},
		End:   text.Position{Line: int(r.End.Line), Character: int(r.End.Character)},
	}
}
