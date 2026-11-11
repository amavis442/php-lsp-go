// Package presentation serves textDocument/definition.
package presentation

import (
	"context"

	"go.lsp.dev/protocol"

	"github.com/amavis442/php-lsp-go/internal/core/resolve"
	"github.com/amavis442/php-lsp-go/internal/core/text"
	"github.com/amavis442/php-lsp-go/internal/server"
)

// positionParams keeps the URI as a plain string: the protocol library
// normalises URIs while decoding, and the open document is stored under the
// URI exactly as the editor sent it.
type positionParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	} `json:"position"`
}

// location is an LSP Location with the URI as the index stores it.
type location struct {
	URI   string         `json:"uri"`
	Range protocol.Range `json:"range"`
}

// Handlers serves textDocument/definition.
type Handlers struct {
	definitions *resolve.Resolver
	encoding    func() text.Encoding
}

// NewHandlers returns handlers that answer from definitions. encoding
// reports the position encoding agreed with the editor.
func NewHandlers(definitions *resolve.Resolver, encoding func() text.Encoding) *Handlers {
	return &Handlers{definitions: definitions, encoding: encoding}
}

// Register adds the handlers to r.
func (h *Handlers) Register(r *server.Router) {
	server.HandleRequest(r, protocol.MethodTextDocumentDefinition, h.definition)
}

// Capabilities announces goto-definition.
func (h *Handlers) Capabilities(c *protocol.ServerCapabilities) {
	c.DefinitionProvider = protocol.Boolean(true)
}

// definition answers with the name of each declaration found, so the
// editor puts the cursor on it. Nothing found is an empty list.
func (h *Handlers) definition(_ context.Context, p *positionParams) ([]location, error) {
	pos := text.Position{Line: p.Position.Line, Character: p.Position.Character}
	found, err := h.definitions.At(p.TextDocument.URI, pos, h.encoding())
	if err != nil {
		return nil, err
	}
	out := make([]location, len(found))
	for i, s := range found {
		r := s.SelectionRange
		out[i] = location{URI: s.URI, Range: protocol.Range{
			Start: protocol.Position{Line: uint32(r.Start.Line), Character: uint32(r.Start.Character)},
			End:   protocol.Position{Line: uint32(r.End.Line), Character: uint32(r.End.Character)},
		}}
	}
	return out, nil
}
