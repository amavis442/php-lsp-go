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

	"github.com/amavis442/php-lsp-go/internal/core/text"
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
	client Client

	running runningRequests

	capabilities []func(*protocol.ServerCapabilities)
	beforeClose  []func()

	encMu    sync.Mutex
	encoding text.Encoding

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

// AddCapabilities lets a feature announce what it supports in the
// initialize result. Call it before Serve.
func (s *Server) AddCapabilities(contribute func(*protocol.ServerCapabilities)) {
	s.capabilities = append(s.capabilities, contribute)
}

// BeforeClose registers f to run when the session ends, before the
// connection to the editor closes, so work that may still notify the editor
// can finish first. Serve does not return until f does. Call it before Serve.
func (s *Server) BeforeClose(f func()) {
	s.beforeClose = append(s.beforeClose, f)
}

// Client sends notifications to the editor.
func (s *Server) Client() *Client { return &s.client }

// Encoding is the position encoding agreed in initialize; UTF-16 before that.
func (s *Server) Encoding() text.Encoding {
	s.encMu.Lock()
	defer s.encMu.Unlock()
	return s.encoding
}

// Serve runs the session over rwc until the client sends exit, the stream
// closes or ctx is cancelled. It returns the process exit code.
func (s *Server) Serve(ctx context.Context, rwc io.ReadWriteCloser) int {
	conn := jsonrpc2.NewConn(jsonrpc2.NewHeaderStream(rwc), jsonrpc2.WithCodec(codec{}),
		jsonrpc2.WithPreempter(canceller{running: &s.running}))
	s.client.connect(conn)
	// Messages start in the order the client sent them; see handle for which
	// of them may then run concurrently.
	conn.Go(ctx, s.handle)

	select {
	case <-s.exited:
	case <-conn.Done():
	case <-ctx.Done():
	}
	for _, f := range s.beforeClose {
		f()
	}
	if err := conn.Close(); err != nil {
		s.log.Printf("closing connection: %v", err)
	}
	return s.life.exitCode()
}

func (s *Server) initialize(_ context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	enc, kind := negotiateEncoding(params)
	s.encMu.Lock()
	s.encoding = enc
	s.encMu.Unlock()

	capabilities := protocol.ServerCapabilities{PositionEncoding: kind}
	for _, contribute := range s.capabilities {
		contribute(&capabilities)
	}

	s.life.markInitialized()
	return &protocol.InitializeResult{
		Capabilities: capabilities,
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
