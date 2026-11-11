package server

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// inlineRequests change the session state, so like notifications they run
// to completion on the read loop before the next message starts.
var inlineRequests = map[string]bool{
	protocol.MethodInitialize: true,
	protocol.MethodShutdown:   true,
}

// handle runs one message. Notifications run inline on the read loop, so
// document edits are applied in the order the editor sent them. Other
// requests are released to run concurrently once they start: a slow request
// then blocks nothing, can be cancelled, and may call the editor and wait
// for its answer. Such a request sees every message sent before it, but a
// later notification may be applied while it runs.
func (s *Server) handle(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	if err := s.life.admit(req.Method()); err != nil {
		if req.IsCall() {
			return nil, err
		}
		return nil, nil // notifications outside the lifecycle are dropped
	}
	if !req.IsCall() || inlineRequests[req.Method()] {
		return s.router.Handle(ctx, req.Method(), req.Params(), req.IsCall())
	}

	// Register before releasing the read loop, so a $/cancelRequest right
	// behind the request finds it. The request context is pooled by the
	// connection and must not be a parent, so the request runs under the
	// connection's own context.
	runCtx, done := s.running.start(jsonrpc2.DetachContext(ctx), req.ID())
	defer done()
	jsonrpc2.Async(ctx)
	result, err := s.router.Handle(runCtx, req.Method(), req.Params(), true)
	if runCtx.Err() != nil {
		return nil, protocol.ErrRequestCancelled
	}
	return result, err
}

// runningRequests can cancel the requests that run concurrently, and wait
// for them.
type runningRequests struct {
	mu      sync.Mutex
	cancels map[jsonrpc2.ID]context.CancelFunc
	wg      sync.WaitGroup
	closing bool // set by cancelAll; later requests start cancelled
}

// start registers request id and returns its cancellable context and a
// function to call when it has finished. Once the session is ending, the
// context is cancelled from the start and the request is not waited for.
func (r *runningRequests) start(ctx context.Context, id jsonrpc2.ID) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		cancel()
		return ctx, func() {}
	}
	if r.cancels == nil {
		r.cancels = make(map[jsonrpc2.ID]context.CancelFunc)
	}
	r.cancels[id] = cancel
	r.wg.Add(1)
	r.mu.Unlock()
	return ctx, func() {
		r.mu.Lock()
		delete(r.cancels, id)
		r.mu.Unlock()
		cancel()
		r.wg.Done()
	}
}

// cancelAll cancels every running request and every request that starts
// after it.
func (r *runningRequests) cancelAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closing = true
	for _, cancel := range r.cancels {
		cancel()
	}
}

// wait blocks until every running request has finished.
func (r *runningRequests) wait() { r.wg.Wait() }

// cancel cancels request id; an id that is not running is ignored.
func (r *runningRequests) cancel(id jsonrpc2.ID) {
	r.mu.Lock()
	cancel := r.cancels[id]
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// canceller handles $/cancelRequest on the read loop, ahead of the messages
// behind it.
type canceller struct {
	running *runningRequests
}

func (c canceller) Preempt(_ context.Context, req *jsonrpc2.Request) (any, error) {
	if req.Method() != protocol.MethodCancelRequest {
		return nil, jsonrpc2.ErrNotHandled
	}
	var params struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(req.Params(), &params); err == nil {
		if id, ok := requestID(params.ID); ok {
			c.running.cancel(id)
		}
	}
	return struct{}{}, nil // handled; a malformed cancel is ignored
}

// requestID decodes a JSON-RPC id, which is a number or a string. Null
// decodes without error into either, so it is rejected first.
func requestID(raw json.RawMessage) (jsonrpc2.ID, bool) {
	if string(bytes.TrimSpace(raw)) == "null" {
		return jsonrpc2.ID{}, false
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return jsonrpc2.NewNumberID(n), true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return jsonrpc2.NewStringID(s), true
	}
	return jsonrpc2.ID{}, false
}
