package presentation

import (
	"context"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/amavis442/php-lsp-go/internal/core/text"
	"github.com/amavis442/php-lsp-go/internal/features/symbols/domain"
	"github.com/amavis442/php-lsp-go/internal/server"
)

type workspaceSymbolParams struct {
	Query string `json:"query"`
}

// WorkspaceHandlers serves workspace/symbol.
type WorkspaceHandlers struct {
	search *domain.WorkspaceSymbols
}

// NewWorkspaceHandlers returns handlers that answer from search.
func NewWorkspaceHandlers(search *domain.WorkspaceSymbols) *WorkspaceHandlers {
	return &WorkspaceHandlers{search: search}
}

// Register adds the handlers to r.
func (h *WorkspaceHandlers) Register(r *server.Router) {
	server.HandleRequest(r, protocol.MethodWorkspaceSymbol, h.workspaceSymbol)
}

// Capabilities announces workspace symbols.
func (h *WorkspaceHandlers) Capabilities(c *protocol.ServerCapabilities) {
	c.WorkspaceSymbolProvider = protocol.Boolean(true)
}

// workspaceSymbol answers with SymbolInformation, which every client
// understands. Its location is the whole declaration, as the specification
// asks; the positions were converted to the editor's encoding when the file
// was indexed.
func (h *WorkspaceHandlers) workspaceSymbol(_ context.Context, p *workspaceSymbolParams) ([]protocol.SymbolInformation, error) {
	found := h.search.Search(p.Query)
	out := make([]protocol.SymbolInformation, len(found))
	for i, s := range found {
		out[i] = protocol.SymbolInformation{
			BaseSymbolInformation: protocol.BaseSymbolInformation{Name: s.Name, Kind: toKind(s.Kind)},
			Location:              protocol.Location{URI: uri.URI(s.URI), Range: toRange(s.Range)},
		}
		if s.Container != "" {
			container := s.Container
			out[i].ContainerName = &container
		}
	}
	return out, nil
}

func toRange(r text.Range) protocol.Range {
	return protocol.Range{Start: toPosition(r.Start), End: toPosition(r.End)}
}

func toPosition(p text.Position) protocol.Position {
	return protocol.Position{Line: uint32(p.Line), Character: uint32(p.Character)}
}
