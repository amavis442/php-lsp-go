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

	"github.com/amavis442/php-lsp-go/internal/core/resolve"
	"github.com/amavis442/php-lsp-go/internal/core/syntax"
	defpresentation "github.com/amavis442/php-lsp-go/internal/features/definition/presentation"
	diagdomain "github.com/amavis442/php-lsp-go/internal/features/diagnostics/domain"
	diagpresentation "github.com/amavis442/php-lsp-go/internal/features/diagnostics/presentation"
	docdata "github.com/amavis442/php-lsp-go/internal/features/documents/data"
	docdomain "github.com/amavis442/php-lsp-go/internal/features/documents/domain"
	docpresentation "github.com/amavis442/php-lsp-go/internal/features/documents/presentation"
	hovdomain "github.com/amavis442/php-lsp-go/internal/features/hover/domain"
	hovpresentation "github.com/amavis442/php-lsp-go/internal/features/hover/presentation"
	idxdata "github.com/amavis442/php-lsp-go/internal/features/indexing/data"
	idxdomain "github.com/amavis442/php-lsp-go/internal/features/indexing/domain"
	idxpresentation "github.com/amavis442/php-lsp-go/internal/features/indexing/presentation"
	symdomain "github.com/amavis442/php-lsp-go/internal/features/symbols/domain"
	sympresentation "github.com/amavis442/php-lsp-go/internal/features/symbols/presentation"
	"github.com/amavis442/php-lsp-go/internal/server"
	phpstormstubs "github.com/amavis442/php-lsp-go/third_party/phpstorm-stubs"
)

// App is a fully wired language server session.
type App struct {
	server *server.Server
}

// New wires every feature into a server. diagnosticsDelay is how long a
// document must be quiet before its diagnostics are published. The built-in
// PHP stubs are installed under cacheDir and indexed; an empty cacheDir
// leaves them out.
func New(info server.Info, logger *log.Logger, diagnosticsDelay time.Duration, cacheDir string) *App {
	parser := syntax.NewParser()
	var libraries idxpresentation.Libraries = noLibraries{}
	if cacheDir != "" {
		libraries = idxdata.NewStubs(phpstormstubs.Files, phpstormstubs.Version, cacheDir)
	}
	return newApp(info, logger, diagnosticsDelay, parser, parser, libraries)
}

// newApp is New with the parser's roles and the libraries as seams for
// tests.
func newApp(info server.Info, logger *log.Logger, diagnosticsDelay time.Duration,
	checker diagdomain.SyntaxChecker, extractor symdomain.Extractor, libraries idxpresentation.Libraries) *App {
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
	files := idxdata.NewFileSystem(idxdata.DefaultExclude, idxdata.DefaultMaxSize)
	indexer := idxdomain.NewIndexer(files,
		func() idxdomain.Extractor { return syntax.NewParser() }, index, s.Encoding, runtime.NumCPU())
	workspace := idxpresentation.NewWorkspace(indexer, libraries, s.Client(), logger)
	workspace.Register(s.Router())
	s.OnInitialized(workspace.Start)
	s.BeforeClose(workspace.Stop)

	workspaceSymbols := sympresentation.NewWorkspaceHandlers(symdomain.NewWorkspaceSymbols(index, symdomain.DefaultLimit))
	workspaceSymbols.Register(s.Router())
	s.AddCapabilities(workspaceSymbols.Capabilities)

	// Goto-definition and hover share a parser of their own, so they do not
	// wait on diagnostics.
	navParser := syntax.NewParser()
	resolver := resolve.NewResolver(documentText{docs}, navParser, index)

	definitions := defpresentation.NewHandlers(resolver, s.Encoding)
	definitions.Register(s.Router())
	s.AddCapabilities(definitions.Capabilities)

	hover := hovpresentation.NewHandlers(hovdomain.NewHover(resolver, documentText{docs}, files, navParser), s.Encoding)
	hover.Register(s.Router())
	s.AddCapabilities(hover.Capabilities)

	return &App{server: s}
}

// noLibraries is used when no libraries are indexed.
type noLibraries struct{}

func (noLibraries) Roots(context.Context) ([]string, error) { return nil, nil }

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
