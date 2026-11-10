// Package server wires the LSP transport to the features: it owns the session
// lifecycle and routes each JSON-RPC message to the handler registered for it.
package server

import (
	"context"
	"io"
	"log"
	"sync"
	"time"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"

	"github.com/amavis442/php-lsp-go/internal/core/text"
)

// Info identifies the server in the initialize response.
type Info struct {
	Name    string
	Version string
}

// DefaultCloseTimeout is how long each step of ending a session may take.
const DefaultCloseTimeout = 2 * time.Second

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

	// closeTimeout bounds each wait while the session ends, so work that
	// ignores cancellation cannot keep the process alive.
	closeTimeout time.Duration

	encMu    sync.Mutex
	encoding text.Encoding

	exitOnce sync.Once
	exited   chan struct{}
}

// New returns a server that answers the lifecycle methods. Features register
// their methods on Router before Serve is called.
func New(info Info, logger *log.Logger) *Server {
	s := &Server{
		info:         info,
		log:          logger,
		router:       NewRouter(logger),
		closeTimeout: DefaultCloseTimeout,
		exited:       make(chan struct{}),
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
// can finish first. Serve waits for f, but at most DefaultCloseTimeout.
// Call it before Serve.
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
	s.end(conn)
	return s.life.exitCode()
}

// end tears the session down: running requests are cancelled and waited
// for, then the BeforeClose hooks run, then the connection closes (which
// itself waits for every handler). Each step waits at most closeTimeout, so
// work that ignores cancellation cannot keep the process alive.
func (s *Server) end(conn jsonrpc2.Conn) {
	s.running.cancelAll()
	s.within("requests still running", s.running.wait)
	for _, f := range s.beforeClose {
		s.within("a close hook did not finish", f)
	}
	s.within("the connection did not close", func() {
		if err := conn.Close(); err != nil {
			s.log.Printf("closing connection: %v", err)
		}
	})
}

// within runs f and waits at most closeTimeout for it. When f is slower it
// logs what and carries on; f keeps running in the background.
func (s *Server) within(what string, f func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(s.closeTimeout):
		s.log.Printf("%s after %s; ending the session anyway", what, s.closeTimeout)
	}
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
