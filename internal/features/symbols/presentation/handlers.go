// Package presentation serves the symbol requests of the LSP.
package presentation

import (
	"context"

	"go.lsp.dev/protocol"

	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
	"github.com/amavis442/php-lsp-go/internal/features/symbols/domain"
	"github.com/amavis442/php-lsp-go/internal/server"
)

// documentSymbolParams keeps the URI as a plain string: the protocol
// library normalises URIs while decoding, and the open document is stored
// under the URI exactly as the editor sent it.
type documentSymbolParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
}

// kinds maps declarations to LSP symbol kinds. LSP has no kind for traits;
// they are closest to classes.
var kinds = map[php.SymbolKind]protocol.SymbolKind{
	php.KindNamespace:   protocol.SymbolKindNamespace,
	php.KindClass:       protocol.SymbolKindClass,
	php.KindInterface:   protocol.SymbolKindInterface,
	php.KindTrait:       protocol.SymbolKindClass,
	php.KindEnum:        protocol.SymbolKindEnum,
	php.KindEnumCase:    protocol.SymbolKindEnumMember,
	php.KindFunction:    protocol.SymbolKindFunction,
	php.KindMethod:      protocol.SymbolKindMethod,
	php.KindConstructor: protocol.SymbolKindConstructor,
	php.KindProperty:    protocol.SymbolKindProperty,
	php.KindConstant:    protocol.SymbolKindConstant,
}

// Handlers serves textDocument/documentSymbol.
type Handlers struct {
	symbols  *domain.DocumentSymbols
	encoding func() text.Encoding
}

// NewHandlers returns handlers that answer from symbols. encoding reports
// the position encoding agreed with the editor.
func NewHandlers(symbols *domain.DocumentSymbols, encoding func() text.Encoding) *Handlers {
	return &Handlers{symbols: symbols, encoding: encoding}
}

// Register adds the handlers to r.
func (h *Handlers) Register(r *server.Router) {
	server.HandleRequest(r, protocol.MethodTextDocumentDocumentSymbol, h.documentSymbol)
}

// Capabilities announces document symbols.
func (h *Handlers) Capabilities(c *protocol.ServerCapabilities) {
	c.DocumentSymbolProvider = protocol.Boolean(true)
}

// documentSymbol always answers with the hierarchical form, which every
// editor this server targets supports.
func (h *Handlers) documentSymbol(_ context.Context, p *documentSymbolParams) ([]protocol.DocumentSymbol, error) {
	outline, err := h.symbols.For(p.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	conv := converter{index: text.NewLineIndex(outline.Text), enc: h.encoding()}
	return conv.symbols(outline.Declarations), nil
}

type converter struct {
	index *text.LineIndex
	enc   text.Encoding
}

func (c converter) symbols(decls []php.Declaration) []protocol.DocumentSymbol {
	out := make([]protocol.DocumentSymbol, len(decls))
	for i, d := range decls {
		out[i] = protocol.DocumentSymbol{
			Name:           d.Name,
			Kind:           toKind(d.Kind),
			Range:          c.span(d.Start, d.End),
			SelectionRange: c.span(d.NameStart, d.NameEnd),
		}
		if len(d.Children) > 0 {
			out[i].Children = c.symbols(d.Children)
		}
	}
	return out
}

func (c converter) span(start, end int) protocol.Range {
	return protocol.Range{Start: c.position(start), End: c.position(end)}
}

func (c converter) position(offset int) protocol.Position {
	p := c.index.Position(offset, c.enc)
	return protocol.Position{Line: uint32(p.Line), Character: uint32(p.Character)}
}

func toKind(k php.SymbolKind) protocol.SymbolKind { return kinds[k] }
