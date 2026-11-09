// Package server wires the LSP transport to the features: it owns the session
// lifecycle and routes each JSON-RPC message to the handler registered for it.
package server

import (
	"context"
	"io"
	"log"
	"sync"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// Info identifies the server in the initialize response.
type Info struct {
	Name    string
	Version string
}

// Server is one LSP session.
type Server struct {
	info   Info
	log    *log.Logger
	router *Router
	life   lifecycle

	exitOnce sync.Once
	exited   chan struct{}
}

// New returns a server that answers the lifecycle methods. Features register
// their methods on Router before Serve is called.
func New(info Info, logger *log.Logger) *Server {
	s := &Server{
		info:   info,
		log:    logger,
		router: NewRouter(logger),
		exited: make(chan struct{}),
	}
	HandleRequest(s.router, protocol.MethodInitialize, s.initialize)
	HandleNotification(s.router, protocol.MethodInitialized, s.initialized)
	HandleRequest(s.router, protocol.MethodShutdown, s.shutdown)
	HandleNotification(s.router, protocol.MethodExit, s.exit)
	return s
}

// Router exposes the method registry so features can add their handlers.
func (s *Server) Router() *Router { return s.router }

// Serve runs the session over rwc until the client sends exit, the stream
// closes or ctx is cancelled. It returns the process exit code.
func (s *Server) Serve(ctx context.Context, rwc io.ReadWriteCloser) int {
	conn := jsonrpc2.NewConn(jsonrpc2.NewHeaderStream(rwc), jsonrpc2.WithCodec(codec{}))
	// Handlers run inline on the read loop, so messages are processed in the
	// order the client sent them. Document edits depend on that order.
	conn.Go(ctx, s.handle)

	select {
	case <-s.exited:
	case <-conn.Done():
	case <-ctx.Done():
	}
	if err := conn.Close(); err != nil {
		s.log.Printf("closing connection: %v", err)
	}
	return s.life.exitCode()
}

func (s *Server) handle(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	if err := s.life.admit(req.Method()); err != nil {
		if req.IsCall() {
			return nil, err
		}
		return nil, nil // notifications outside the lifecycle are dropped
	}
	return s.router.Handle(ctx, req.Method(), req.Params(), req.IsCall())
}

func (s *Server) initialize(_ context.Context, _ *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	s.life.markInitialized()
	return &protocol.InitializeResult{
		ServerInfo: protocol.ServerInfo{
			Name:    s.info.Name,
			Version: protocol.NewOptional(s.info.Version),
		},
	}, nil
}

func (s *Server) initialized(context.Context, *protocol.InitializedParams) error { return nil }

func (s *Server) shutdown(context.Context, *struct{}) (any, error) {
	s.life.markShutdown()
	return nil, nil
}

func (s *Server) exit(context.Context, *struct{}) error {
	s.exitOnce.Do(func() { close(s.exited) })
	return nil
}
