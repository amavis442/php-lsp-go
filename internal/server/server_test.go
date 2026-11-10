package server

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"testing"
	"time"

	"github.com/amavis442/php-lsp-go/internal/testsupport/lspclient"
)

func newTestServer() *Server {
	return New(Info{Name: "php-lsp-go", Version: "test"}, log.New(io.Discard, "", 0))
}

// session runs a Server in-process over a net.Pipe and returns a client for
// the other end plus a channel that delivers Serve's exit code.
func session(t *testing.T) (*lspclient.Client, <-chan int) {
	t.Helper()
	return serve(t, newTestServer())
}

// serve runs s, which the test may have configured, like session does.
func serve(t *testing.T, s *Server) (*lspclient.Client, <-chan int) {
	t.Helper()
	serverEnd, clientEnd := net.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	done := make(chan int, 1)
	go func() { done <- s.Serve(ctx, serverEnd) }()
	t.Cleanup(func() { _ = clientEnd.Close() })

	return lspclient.New(clientEnd, clientEnd), done
}

func waitExit(t *testing.T, done <-chan int) int {
	t.Helper()
	select {
	case code := <-done:
		return code
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after exit")
		return -1
	}
}

func initialize(t *testing.T, c *lspclient.Client) lspclient.Message {
	t.Helper()
	resp, err := c.Call("initialize", map[string]any{"processId": nil, "rootUri": nil, "capabilities": map[string]any{}})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := c.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("initialized: %v", err)
	}
	return resp
}

func TestInitializeReturnsServerInfo(t *testing.T) {
	c, _ := session(t)

	resp := initialize(t, c)

	if resp.Error != nil {
		t.Fatalf("initialize error: %+v", resp.Error)
	}
	var result struct {
		Capabilities map[string]any `json:"capabilities"`
		ServerInfo   struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decoding result %s: %v", resp.Result, err)
	}
	if result.ServerInfo.Name != "php-lsp-go" || result.ServerInfo.Version != "test" {
		t.Errorf("serverInfo = %+v, want php-lsp-go/test", result.ServerInfo)
	}
	if result.Capabilities == nil {
		t.Error("capabilities missing from initialize result")
	}
}

func TestRequestBeforeInitializeIsRejected(t *testing.T) {
	c, _ := session(t)

	resp, err := c.Call("textDocument/hover", map[string]any{})

	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != int(codeServerNotInitialized) {
		t.Errorf("error = %+v, want code %d", resp.Error, codeServerNotInitialized)
	}
}

func TestUnknownRequestIsMethodNotFound(t *testing.T) {
	c, _ := session(t)
	initialize(t, c)

	resp, err := c.Call("php/doesNotExist", nil)

	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Errorf("error = %+v, want code -32601", resp.Error)
	}
}

func TestShutdownThenExitStopsWithCodeZero(t *testing.T) {
	c, done := session(t)
	initialize(t, c)

	resp, err := c.Call("shutdown", nil)
	if err != nil || resp.Error != nil {
		t.Fatalf("shutdown: err=%v resp.Error=%+v", err, resp.Error)
	}
	if string(resp.Result) != "null" {
		t.Errorf("shutdown result = %s, want null", resp.Result)
	}
	if err := c.Notify("exit", nil); err != nil {
		t.Fatalf("exit: %v", err)
	}

	if code := waitExit(t, done); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestRequestAfterShutdownIsInvalid(t *testing.T) {
	c, _ := session(t)
	initialize(t, c)
	if _, err := c.Call("shutdown", nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	resp, err := c.Call("textDocument/hover", map[string]any{})

	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != -32600 {
		t.Errorf("error = %+v, want code -32600", resp.Error)
	}
}

func TestExitWithoutShutdownStopsWithCodeOne(t *testing.T) {
	c, done := session(t)
	initialize(t, c)

	if err := c.Notify("exit", nil); err != nil {
		t.Fatalf("exit: %v", err)
	}

	if code := waitExit(t, done); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}
