// Package presentation serves textDocument/hover.
package presentation

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"

	"github.com/amavis442/php-lsp-go/internal/core/text"
	"github.com/amavis442/php-lsp-go/internal/features/hover/domain"
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

type markup struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type hoverResult struct {
	Contents markup `json:"contents"`
}

// Handlers serves textDocument/hover.
type Handlers struct {
	hover    *domain.Hover
	encoding func() text.Encoding
}

// NewHandlers returns handlers that answer from hover. encoding reports the
// position encoding agreed with the editor.
func NewHandlers(hover *domain.Hover, encoding func() text.Encoding) *Handlers {
	return &Handlers{hover: hover, encoding: encoding}
}

// Register adds the handlers to r.
func (h *Handlers) Register(r *server.Router) {
	server.HandleRequest(r, protocol.MethodTextDocumentHover, h.hoverAt)
}

// Capabilities announces hover.
func (h *Handlers) Capabilities(c *protocol.ServerCapabilities) {
	c.HoverProvider = protocol.Boolean(true)
}

// hoverAt answers with Markdown, or null when there is nothing to show.
func (h *Handlers) hoverAt(_ context.Context, p *positionParams) (*hoverResult, error) {
	pos := text.Position{Line: p.Position.Line, Character: p.Position.Character}
	info, ok, err := h.hover.At(p.TextDocument.URI, pos, h.encoding())
	if err != nil || !ok {
		return nil, err
	}
	return &hoverResult{Contents: markup{Kind: "markdown", Value: render(info)}}, nil
}

// render shows the signature as PHP, then the class or namespace it belongs
// to, then the summary. The "<?php" line lets editors that highlight code
// blocks with a PHP grammar recognise the code.
func render(info domain.Info) string {
	parts := []string{"```php\n<?php\n" + info.Signature + "\n```"}
	if info.Container != "" {
		parts = append(parts, "`"+info.Container+"`")
	}
	if summary := docMarkdown(info.Summary); summary != "" {
		parts = append(parts, summary)
	}
	return strings.Join(parts, "\n\n")
}
