package presentation

import (
	"context"
	"io"
	"log"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/amavis442/php-lsp-go/internal/core/text"
	"github.com/amavis442/php-lsp-go/internal/features/documents/data"
	"github.com/amavis442/php-lsp-go/internal/features/documents/domain"
	"github.com/amavis442/php-lsp-go/internal/server"
)

// A Windows URI: the protocol library would rewrite it to file:///d%3A/...,
// so this also checks that the URI is kept exactly as the editor sent it.
const uri = "file:///D:/work/a.php"

func setup(t *testing.T, enc text.Encoding) (*server.Router, *domain.Service) {
	t.Helper()
	docs := domain.NewService(data.NewMemoryStore())
	r := server.NewRouter(log.New(io.Discard, "", 0))
	NewHandlers(docs, func() text.Encoding { return enc }).Register(r)
	return r, docs
}

func send(t *testing.T, r *server.Router, method, params string) {
	t.Helper()
	if _, err := r.Handle(context.Background(), method, []byte(params), false); err != nil {
		t.Fatalf("%s: %v", method, err)
	}
}

const openParams = `{"textDocument":{"uri":"` + uri + `","languageId":"php","version":1,"text":"<?php\necho;"}}`

func TestDidOpenKeepsTheURIExactly(t *testing.T) {
	r, docs := setup(t, text.UTF16)

	send(t, r, "textDocument/didOpen", openParams)

	got, ok := docs.Get(uri)
	if !ok || got != (domain.Document{URI: uri, Version: 1, Text: "<?php\necho;"}) {
		t.Errorf("document = %+v, %v", got, ok)
	}
}

func TestDidChangeAppliesIncrementalEditsInTheNegotiatedEncoding(t *testing.T) {
	r, docs := setup(t, text.UTF8)
	send(t, r, "textDocument/didOpen",
		`{"textDocument":{"uri":"`+uri+`","languageId":"php","version":1,"text":"<?php\n// é\necho;"}}`)

	// Character 5 on line 1 is after "// é" in UTF-8 (é is two bytes).
	send(t, r, "textDocument/didChange", `{"textDocument":{"uri":"`+uri+`","version":2},"contentChanges":[`+
		`{"range":{"start":{"line":1,"character":5},"end":{"line":1,"character":5}},"text":"!"},`+
		`{"range":{"start":{"line":2,"character":4},"end":{"line":2,"character":4}},"text":" 1"}]}`)

	got, _ := docs.Get(uri)
	if got.Version != 2 || got.Text != "<?php\n// é!\necho 1;" {
		t.Errorf("document = %+v", got)
	}
}

func TestDidChangeWithoutRangeReplacesTheText(t *testing.T) {
	r, docs := setup(t, text.UTF16)
	send(t, r, "textDocument/didOpen", openParams)

	send(t, r, "textDocument/didChange",
		`{"textDocument":{"uri":"`+uri+`","version":2},"contentChanges":[{"text":"<?php"}]}`)

	if got, _ := docs.Get(uri); got.Text != "<?php" {
		t.Errorf("text = %q, want %q", got.Text, "<?php")
	}
}

func TestDidCloseForgetsTheDocument(t *testing.T) {
	r, docs := setup(t, text.UTF16)
	send(t, r, "textDocument/didOpen", openParams)

	send(t, r, "textDocument/didClose", `{"textDocument":{"uri":"`+uri+`"}}`)

	if _, ok := docs.Get(uri); ok {
		t.Error("document still open after didClose")
	}
}

func TestCapabilitiesAnnounceIncrementalSync(t *testing.T) {
	var c protocol.ServerCapabilities

	NewHandlers(nil, nil).Capabilities(&c)

	opts, ok := c.TextDocumentSync.(*protocol.TextDocumentSyncOptions)
	if !ok {
		t.Fatalf("textDocumentSync = %#v, want options", c.TextDocumentSync)
	}
	if opts.OpenClose == nil || !*opts.OpenClose {
		t.Error("openClose not enabled")
	}
	if opts.Change == nil || *opts.Change != protocol.TextDocumentSyncKindIncremental {
		t.Errorf("change = %v, want incremental", opts.Change)
	}
}
