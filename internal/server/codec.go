package server

import (
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// codec marshals JSON-RPC payloads with the protocol package's union-aware
// encoder. The protocol package has an equivalent codec, but it is unexported
// and only wired into protocol.NewServer, which requires implementing the full
// ~70-method protocol.Server interface. We route methods ourselves instead.
type codec struct{}

var _ jsonrpc2.Codec = codec{}

// Marshal implements jsonrpc2.Codec. Pre-encoded raw messages pass through.
func (codec) Marshal(v any) ([]byte, error) {
	switch m := v.(type) {
	case jsonrpc2.RawMessage:
		if m == nil {
			return []byte("null"), nil
		}
		return m, nil
	case *jsonrpc2.RawMessage:
		if m == nil || *m == nil {
			return []byte("null"), nil
		}
		return *m, nil
	}
	return protocol.Marshal(v)
}

// Unmarshal implements jsonrpc2.Codec. Raw messages are copied because the
// transport may reuse the input buffer.
func (codec) Unmarshal(data []byte, v any) error {
	if p, ok := v.(*jsonrpc2.RawMessage); ok {
		*p = append(jsonrpc2.RawMessage(nil), data...)
		return nil
	}
	return protocol.Unmarshal(data, v)
}
