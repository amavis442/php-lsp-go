// Command php-lsp-go is a PHP language server. Editors start it and talk LSP
// over stdin and stdout; diagnostics about the server itself go to stderr.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/amavis442/php-lsp-go/internal/app"
	diagnostics "github.com/amavis442/php-lsp-go/internal/features/diagnostics/presentation"
	"github.com/amavis442/php-lsp-go/internal/server"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger := log.New(os.Stderr, "php-lsp-go: ", log.LstdFlags)
	a := app.New(server.Info{Name: "php-lsp-go", Version: version}, logger, diagnostics.DefaultDelay)
	return a.Serve(ctx, stdio{in: openStdin()})
}

// stdio joins stdin and stdout into the io.ReadWriteCloser the transport
// needs. Closing it closes both, which unblocks a pending read.
type stdio struct {
	in *os.File
}

func (s stdio) Read(p []byte) (int, error) { return s.in.Read(p) }
func (stdio) Write(p []byte) (int, error)  { return os.Stdout.Write(p) }

func (s stdio) Close() error {
	inErr := s.in.Close()
	if err := os.Stdout.Close(); err != nil {
		return err
	}
	return inErr
}
