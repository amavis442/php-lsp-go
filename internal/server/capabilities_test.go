package server

import (
	"context"
	"encoding/json"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/amavis442/php-lsp-go/internal/core/text"
)

type initResult struct {
	Capabilities struct {
		PositionEncoding string `json:"positionEncoding"`
		TextDocumentSync any    `json:"textDocumentSync"`
	} `json:"capabilities"`
}

func initializeWith(t *testing.T, s *Server, capabilities map[string]any) initResult {
	t.Helper()
	c, _ := serve(t, s)
	resp, err := c.Call("initialize", map[string]any{"processId": nil, "rootUri": nil, "capabilities": capabilities})
	if err != nil || resp.Error != nil {
		t.Fatalf("initialize: err=%v resp.Error=%+v", err, resp.Error)
	}
	var result initResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decoding %s: %v", resp.Result, err)
	}
	return result
}

func TestInitializePrefersUTF8WhenTheClientOffersIt(t *testing.T) {
	s := newTestServer()

	got := initializeWith(t, s, map[string]any{"general": map[string]any{"positionEncodings": []string{"utf-16", "utf-8"}}})

	if got.Capabilities.PositionEncoding != "utf-8" {
		t.Errorf("positionEncoding = %q, want utf-8", got.Capabilities.PositionEncoding)
	}
	if s.Encoding() != text.UTF8 {
		t.Errorf("Encoding() = %v, want UTF8", s.Encoding())
	}
}

func TestInitializeFallsBackToUTF16(t *testing.T) {
	tests := map[string]map[string]any{
		"no general capabilities": {},
		"only utf-32 offered":     {"general": map[string]any{"positionEncodings": []string{"utf-32"}}},
	}
	for name, capabilities := range tests {
		t.Run(name, func(t *testing.T) {
			s := newTestServer()

			got := initializeWith(t, s, capabilities)

			if got.Capabilities.PositionEncoding != "utf-16" {
				t.Errorf("positionEncoding = %q, want utf-16", got.Capabilities.PositionEncoding)
			}
			if s.Encoding() != text.UTF16 {
				t.Errorf("Encoding() = %v, want UTF16", s.Encoding())
			}
		})
	}
}

func TestFeaturesContributeCapabilities(t *testing.T) {
	s := newTestServer()
	s.AddCapabilities(func(c *protocol.ServerCapabilities) {
		c.TextDocumentSync = protocol.TextDocumentSyncKindFull
	})

	got := initializeWith(t, s, map[string]any{})

	if got.Capabilities.TextDocumentSync != float64(1) {
		t.Errorf("textDocumentSync = %v, want 1", got.Capabilities.TextDocumentSync)
	}
}

func TestClientSendsNotificationsToTheEditor(t *testing.T) {
	s := newTestServer()
	HandleRequest(s.Router(), "test/ping", func(ctx context.Context, _ *struct{}) (any, error) {
		return nil, s.Client().Notify(ctx, "test/pong", map[string]int{"n": 1})
	})
	c, _ := serve(t, s)
	initialize(t, c)

	if resp, err := c.Call("test/ping", nil); err != nil || resp.Error != nil {
		t.Fatalf("test/ping: err=%v resp.Error=%+v", err, resp.Error)
	}
	msg, err := c.WaitNotification("test/pong")

	if err != nil || string(msg.Params) != `{"n":1}` {
		t.Errorf("test/pong = %+v, %v; want params {\"n\":1}", msg, err)
	}
}

func TestClientNotifyBeforeServeFails(t *testing.T) {
	s := newTestServer()

	if err := s.Client().Notify(context.Background(), "test/pong", nil); err == nil {
		t.Error("Notify before Serve succeeded, want error")
	}
}

func TestBeforeCloseRunsWhileTheEditorIsStillConnected(t *testing.T) {
	s := newTestServer()
	s.BeforeClose(func() {
		if err := s.Client().Notify(context.Background(), "test/bye", nil); err != nil {
			t.Errorf("Notify in BeforeClose: %v", err)
		}
	})
	c, done := serve(t, s)
	initialize(t, c)

	if resp, err := c.Call("shutdown", nil); err != nil || resp.Error != nil {
		t.Fatalf("shutdown: err=%v resp.Error=%+v", err, resp.Error)
	}
	if err := c.Notify("exit", nil); err != nil {
		t.Fatalf("exit: %v", err)
	}

	if _, err := c.WaitNotification("test/bye"); err != nil {
		t.Errorf("test/bye: %v", err)
	}
	if code := waitExit(t, done); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}
