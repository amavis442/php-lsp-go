// Package app is the composition root: it builds the server and wires each
// feature's data, domain and presentation layers together. It lives outside
// package main so the fully wired server can be tested in-process.
package app

import (
	"context"
	"io"
	"log"
	"runtime"
	"time"

	"github.com/amavis442/php-lsp-go/internal/core/syntax"
	diagdomain "github.com/amavis442/php-lsp-go/internal/features/diagnostics/domain"
	diagpresentation "github.com/amavis442/php-lsp-go/internal/features/diagnostics/presentation"
	docdata "github.com/amavis442/php-lsp-go/internal/features/documents/data"
	docdomain "github.com/amavis442/php-lsp-go/internal/features/documents/domain"
	docpresentation "github.com/amavis442/php-lsp-go/internal/features/documents/presentation"
	idxdata "github.com/amavis442/php-lsp-go/internal/features/indexing/data"
	idxdomain "github.com/amavis442/php-lsp-go/internal/features/indexing/domain"
	idxpresentation "github.com/amavis442/php-lsp-go/internal/features/indexing/presentation"
	symdomain "github.com/amavis442/php-lsp-go/internal/features/symbols/domain"
	sympresentation "github.com/amavis442/php-lsp-go/internal/features/symbols/presentation"
	"github.com/amavis442/php-lsp-go/internal/server"
)

// App is a fully wired language server session.
type App struct {
	server *server.Server
}

// New wires every feature into a server. diagnosticsDelay is how long a
// document must be quiet before its diagnostics are published.
func New(info server.Info, logger *log.Logger, diagnosticsDelay time.Duration) *App {
	parser := syntax.NewParser()
	return newApp(info, logger, diagnosticsDelay, parser, parser)
}

// newApp is New with the parser's roles as seams for tests.
func newApp(info server.Info, logger *log.Logger, diagnosticsDelay time.Duration,
	checker diagdomain.SyntaxChecker, extractor symdomain.Extractor) *App {
	s := server.New(info, logger)

	publisher := diagpresentation.NewPublisher(
		diagdomain.NewCollector(checker), s.Client(), s.Encoding, diagnosticsDelay, logger)

	docs := docdomain.NewService(docdata.NewMemoryStore(), publisher)
	documents := docpresentation.NewHandlers(docs, s.Encoding)
	documents.Register(s.Router())
	s.AddCapabilities(documents.Capabilities)
	s.BeforeClose(publisher.Stop)

	symbols := sympresentation.NewHandlers(symdomain.NewDocumentSymbols(documentText{docs}, extractor), s.Encoding)
	symbols.Register(s.Router())
	s.AddCapabilities(symbols.Capabilities)

	index := idxdata.NewMemoryIndex()
	indexer := idxdomain.NewIndexer(idxdata.NewFileSystem(idxdata.DefaultExclude, idxdata.DefaultMaxSize),
		func() idxdomain.Extractor { return syntax.NewParser() }, index, s.Encoding, runtime.NumCPU())
	workspace := idxpresentation.NewWorkspace(indexer, s.Client(), logger)
	workspace.Register(s.Router())
	s.OnInitialized(workspace.Start)
	s.BeforeClose(workspace.Stop)

	workspaceSymbols := sympresentation.NewWorkspaceHandlers(symdomain.NewWorkspaceSymbols(index, symdomain.DefaultLimit))
	workspaceSymbols.Register(s.Router())
	s.AddCapabilities(workspaceSymbols.Capabilities)

	return &App{server: s}
}

// documentText lets the symbols feature read open documents without
// depending on the documents feature.
type documentText struct {
	docs *docdomain.Service
}

func (d documentText) Text(uri string) (string, bool) {
	doc, ok := d.docs.Get(uri)
	return doc.Text, ok
}

// Serve runs the session over rwc and returns the process exit code.
func (a *App) Serve(ctx context.Context, rwc io.ReadWriteCloser) int {
	return a.server.Serve(ctx, rwc)
}
