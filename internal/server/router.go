package server

import (
	"context"
	"log"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// handlerFunc handles one decoded message. For notifications the result is
// discarded.
type handlerFunc func(ctx context.Context, params []byte) (any, error)

// Router maps LSP method names to handlers. Features register only the
// methods they implement; everything else is answered with MethodNotFound.
type Router struct {
	log      *log.Logger
	requests map[string]handlerFunc
	notifies map[string]handlerFunc
}

// NewRouter returns an empty router that reports dropped messages to logger.
func NewRouter(logger *log.Logger) *Router {
	return &Router{
		log:      logger,
		requests: make(map[string]handlerFunc),
		notifies: make(map[string]handlerFunc),
	}
}

// HandleRequest registers fn for the request method. Params are decoded into
// a fresh P (left zero when the message has none).
func HandleRequest[P, R any](r *Router, method string, fn func(ctx context.Context, params *P) (R, error)) {
	r.requests[method] = func(ctx context.Context, raw []byte) (any, error) {
		var params P
		if err := decode(raw, &params); err != nil {
			return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, err.Error())
		}
		return fn(ctx, &params)
	}
}

// HandleNotification registers fn for the notification method.
func HandleNotification[P any](r *Router, method string, fn func(ctx context.Context, params *P) error) {
	r.notifies[method] = func(ctx context.Context, raw []byte) (any, error) {
		var params P
		if err := decode(raw, &params); err != nil {
			r.log.Printf("dropping %s: invalid params: %v", method, err)
			return nil, nil
		}
		// A failing notification must not end the session: jsonrpc2 closes
		// the connection when a notification handler returns an error.
		if err := fn(ctx, &params); err != nil {
			r.log.Printf("%s failed: %v", method, err)
		}
		return nil, nil
	}
}

func decode(raw []byte, v any) error {
	if len(raw) == 0 {
		return nil
	}
	return protocol.Unmarshal(raw, v)
}

// Handle dispatches one message. Unknown requests get MethodNotFound; unknown
// notifications are ignored, as the specification allows.
func (r *Router) Handle(ctx context.Context, method string, params []byte, isCall bool) (any, error) {
	if isCall {
		h, ok := r.requests[method]
		if !ok {
			return nil, jsonrpc2.ErrMethodNotFound
		}
		return h(ctx, params)
	}
	h, ok := r.notifies[method]
	if !ok {
		return nil, nil
	}
	return h(ctx, params)
}
