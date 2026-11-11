package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/syntax"
	"github.com/amavis442/php-lsp-go/internal/server"
	"github.com/amavis442/php-lsp-go/internal/testsupport/lspclient"
)

// A Windows URI checks end to end that diagnostics name the document exactly
// as the editor did.
const docURI = "file:///D:/work/a.php"

type diagnostics struct {
	URI         string `json:"uri"`
	Version     *int32 `json:"version"`
	Diagnostics []struct {
		Range struct {
			Start struct{ Line, Character int } `json:"start"`
			End   struct{ Line, Character int } `json:"end"`
		} `json:"range"`
		Severity int    `json:"severity"`
		Message  string `json:"message"`
	} `json:"diagnostics"`
}

func start(t *testing.T) (*lspclient.Client, <-chan int) {
	t.Helper()
	return startApp(t, New(server.Info{Name: "php-lsp-go", Version: "test"}, log.New(io.Discard, "", 0), time.Millisecond, ""))
}

// startApp serves a over a net.Pipe and returns an initialized client.
func startApp(t *testing.T, a *App) (*lspclient.Client, <-chan int) {
	t.Helper()
	return startAppWith(t, a, map[string]any{"processId": nil, "rootUri": nil, "capabilities": map[string]any{}})
}

// startAppWith is startApp with the initialize params given.
func startAppWith(t *testing.T, a *App, params map[string]any) (*lspclient.Client, <-chan int) {
	t.Helper()
	serverEnd, clientEnd := net.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	done := make(chan int, 1)
	go func() { done <- a.Serve(ctx, serverEnd) }()
	t.Cleanup(func() { _ = clientEnd.Close() })

	c := lspclient.New(clientEnd, clientEnd)
	resp, err := c.Call("initialize", params)
	if err != nil || resp.Error != nil {
		t.Fatalf("initialize: err=%v resp.Error=%+v", err, resp.Error)
	}
	if err := c.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("initialized: %v", err)
	}
	return c, done
}

func notify(t *testing.T, c *lspclient.Client, method string, params any) {
	t.Helper()
	if err := c.Notify(method, params); err != nil {
		t.Fatalf("%s: %v", method, err)
	}
}

func waitDiagnostics(t *testing.T, c *lspclient.Client) diagnostics {
	t.Helper()
	msg, err := c.WaitNotification("textDocument/publishDiagnostics")
	if err != nil {
		t.Fatalf("waiting for diagnostics: %v", err)
	}
	var d diagnostics
	if err := json.Unmarshal(msg.Params, &d); err != nil {
		t.Fatalf("decoding %s: %v", msg.Params, err)
	}
	return d
}

func TestEditingADocumentUpdatesItsSyntaxDiagnostics(t *testing.T) {
	c, done := start(t)

	notify(t, c, "textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": docURI, "languageId": "php", "version": 1, "text": "<?php\necho 1\n",
	}})
	opened := waitDiagnostics(t, c)
	if opened.URI != docURI || opened.Version == nil || *opened.Version != 1 || len(opened.Diagnostics) != 1 {
		t.Fatalf("after open: %+v, want one diagnostic for version 1 of %s", opened, docURI)
	}
	d := opened.Diagnostics[0]
	if d.Message != "Syntax error: missing ';'" || d.Severity != 1 ||
		d.Range.Start.Line != 1 || d.Range.Start.Character != 6 {
		t.Errorf("diagnostic = %+v, want a missing ';' error at 1:6", d)
	}

	notify(t, c, "textDocument/didChange", map[string]any{
		"textDocument": map[string]any{"uri": docURI, "version": 2},
		"contentChanges": []any{map[string]any{
			"range": map[string]any{"start": map[string]int{"line": 1, "character": 6}, "end": map[string]int{"line": 1, "character": 6}},
			"text":  ";",
		}},
	})
	fixed := waitDiagnostics(t, c)
	if fixed.Version == nil || *fixed.Version != 2 || len(fixed.Diagnostics) != 0 {
		t.Errorf("after fix: %+v, want no diagnostics for version 2", fixed)
	}

	notify(t, c, "textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": docURI}})
	closed := waitDiagnostics(t, c)
	if closed.URI != docURI || len(closed.Diagnostics) != 0 {
		t.Errorf("after close: %+v, want an empty list", closed)
	}

	if _, err := c.Call("shutdown", nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	notify(t, c, "exit", nil)
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after exit")
	}
}

// syncBuffer is a bytes.Buffer that a run still in flight may write to while
// the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// stalledChecker holds every parse until release is closed, so a test can
// end the session while diagnostics are still being computed.
type stalledChecker struct {
	started chan struct{}
	release chan struct{}
}

func (s stalledChecker) SyntaxErrors([]byte) ([]php.SyntaxError, error) {
	s.started <- struct{}{}
	<-s.release
	return nil, nil
}

func TestExitingWhileDiagnosticsAreInFlightLogsNothing(t *testing.T) {
	var logs syncBuffer
	checker := stalledChecker{started: make(chan struct{}), release: make(chan struct{})}
	c, done := startApp(t, newApp(server.Info{Name: "php-lsp-go", Version: "test"}, log.New(&logs, "", 0),
		time.Millisecond, checker, syntax.NewParser(), noLibraries{}))
	notify(t, c, "textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": docURI, "languageId": "php", "version": 1, "text": "<?php\necho 1;\n",
	}})
	select {
	case <-checker.started:
	case <-time.After(2 * time.Second):
		t.Fatal("diagnostics run did not start within 2s")
	}

	if _, err := c.Call("shutdown", nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	notify(t, c, "exit", nil)
	// Keep reading, as an editor would, so a late publish cannot block on the pipe.
	go func() { _, _ = c.WaitNotification("test/never") }()

	select {
	case <-done:
		t.Fatal("session ended while a diagnostics run was still in progress")
	case <-time.After(50 * time.Millisecond):
	}
	close(checker.release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after exit")
	}
	if got := logs.String(); got != "" {
		t.Errorf("server logged:\n%s", got)
	}
}

func TestDocumentSymbolsOutlineAnOpenDocument(t *testing.T) {
	c, _ := start(t)
	notify(t, c, "textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": docURI, "languageId": "php", "version": 1,
		"text": "<?php\nnamespace App;\nclass User {\n    public function name() {}\n}\n",
	}})

	resp, err := c.Call("textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": docURI}})

	if err != nil || resp.Error != nil {
		t.Fatalf("documentSymbol: err=%v resp.Error=%+v", err, resp.Error)
	}
	type symbol struct {
		Name           string `json:"name"`
		Kind           int    `json:"kind"`
		SelectionRange struct {
			Start struct{ Line, Character int } `json:"start"`
		} `json:"selectionRange"`
		Children []symbol `json:"children"`
	}
	var got []symbol
	if err := json.Unmarshal(resp.Result, &got); err != nil {
		t.Fatalf("decoding %s: %v", resp.Result, err)
	}
	if len(got) != 1 || got[0].Name != "App" || got[0].Kind != 3 || len(got[0].Children) != 1 {
		t.Fatalf("symbols = %s, want namespace App with one child", resp.Result)
	}
	class := got[0].Children[0]
	if class.Name != "User" || class.Kind != 5 || len(class.Children) != 1 ||
		class.Children[0].Name != "name" || class.Children[0].Kind != 6 {
		t.Errorf("class = %+v, want class User with method name", class)
	}
	if s := class.Children[0].SelectionRange.Start; s.Line != 3 || s.Character != 20 {
		t.Errorf("method name starts at %d:%d, want 3:20", s.Line, s.Character)
	}
}

func TestInitializeAnnouncesIncrementalSync(t *testing.T) {
	serverEnd, clientEnd := net.Pipe()
	t.Cleanup(func() { _ = clientEnd.Close() })
	a := New(server.Info{Name: "php-lsp-go", Version: "test"}, log.New(io.Discard, "", 0), time.Millisecond, "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	go a.Serve(ctx, serverEnd)
	c := lspclient.New(clientEnd, clientEnd)

	resp, err := c.Call("initialize", map[string]any{"processId": nil, "rootUri": nil, "capabilities": map[string]any{}})

	if err != nil || resp.Error != nil {
		t.Fatalf("initialize: err=%v resp.Error=%+v", err, resp.Error)
	}
	var result struct {
		Capabilities struct {
			TextDocumentSync struct {
				OpenClose bool `json:"openClose"`
				Change    int  `json:"change"`
			} `json:"textDocumentSync"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decoding %s: %v", resp.Result, err)
	}
	if sync := result.Capabilities.TextDocumentSync; !sync.OpenClose || sync.Change != 2 {
		t.Errorf("textDocumentSync = %+v, want openClose and incremental (2)", sync)
	}
}
