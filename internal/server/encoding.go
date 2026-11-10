package server

import (
	"slices"

	"go.lsp.dev/protocol"

	"github.com/amavis442/php-lsp-go/internal/core/text"
)

// negotiateEncoding picks UTF-8 when the client offers it, because Go
// strings and the parser count bytes too, so no conversion is needed.
// Otherwise it picks UTF-16, which every client must support.
func negotiateEncoding(params *protocol.InitializeParams) (text.Encoding, protocol.PositionEncodingKind) {
	general := params.Capabilities.General
	if general != nil && slices.Contains(general.PositionEncodings, protocol.PositionEncodingKindUTF8) {
		return text.UTF8, protocol.PositionEncodingKindUTF8
	}
	return text.UTF16, protocol.PositionEncodingKindUTF16
}
