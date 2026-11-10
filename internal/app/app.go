// Package app is the composition root: it builds the server and wires each
// feature's data, domain and presentation layers together. It lives outside
// package main so the fully wired server can be tested in-process.
package app

import (
	"context"
	"io"
	"log"
	"time"

	"github.com/amavis442/php-lsp-go/internal/core/syntax"
	diagdomain "github.com/amavis442/php-lsp-go/internal/features/diagnostics/domain"
	diagpresentation "github.com/amavis442/php-lsp-go/internal/features/diagnostics/presentation"
	docdata "github.com/amavis442/php-lsp-go/internal/features/documents/data"
	docdomain "github.com/amavis442/php-lsp-go/internal/features/documents/domain"
	docpresentation "github.com/amavis442/php-lsp-go/internal/features/documents/presentation"
	"github.com/amavis442/php-lsp-go/internal/server"
)

// App is a fully wired language server session.
type App struct {
	server    *server.Server
	publisher *diagpresentation.Publisher
}

// New wires every feature into a server. diagnosticsDelay is how long a
// document must be quiet before its diagnostics are published.
func New(info server.Info, logger *log.Logger, diagnosticsDelay time.Duration) *App {
	s := server.New(info, logger)

	publisher := diagpresentation.NewPublisher(
		diagdomain.NewCollector(syntax.NewParser()), s.Client(), s.Encoding, diagnosticsDelay, logger)

	docs := docdomain.NewService(docdata.NewMemoryStore(), publisher)
	documents := docpresentation.NewHandlers(docs, s.Encoding)
	documents.Register(s.Router())
	s.AddCapabilities(documents.Capabilities)

	return &App{server: s, publisher: publisher}
}

// Serve runs the session over rwc and returns the process exit code.
func (a *App) Serve(ctx context.Context, rwc io.ReadWriteCloser) int {
	defer a.publisher.Stop()
	return a.server.Serve(ctx, rwc)
}
