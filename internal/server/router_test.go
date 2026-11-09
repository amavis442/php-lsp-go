package server

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

	"go.lsp.dev/jsonrpc2"
)

type echoParams struct {
	Text string `json:"text"`
}

func newTestRouter() *Router {
	return NewRouter(log.New(io.Discard, "", 0))
}

func TestRouterDecodesParamsAndReturnsResult(t *testing.T) {
	r := newTestRouter()
	HandleRequest(r, "test/echo", func(_ context.Context, p *echoParams) (string, error) {
		return "echo: " + p.Text, nil
	})

	got, err := r.Handle(context.Background(), "test/echo", []byte(`{"text":"hi"}`), true)

	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got != "echo: hi" {
		t.Errorf("result = %v, want %q", got, "echo: hi")
	}
}

func TestRouterCallsHandlerWithZeroParamsWhenAbsent(t *testing.T) {
	r := newTestRouter()
	called := false
	HandleRequest(r, "shutdown", func(_ context.Context, p *struct{}) (any, error) {
		called = p != nil
		return nil, nil
	})

	if _, err := r.Handle(context.Background(), "shutdown", nil, true); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !called {
		t.Error("handler not called with non-nil params")
	}
}

func TestRouterRunsNotificationHandler(t *testing.T) {
	r := newTestRouter()
	var got string
	HandleNotification(r, "test/note", func(_ context.Context, p *echoParams) error {
		got = p.Text
		return nil
	})

	if _, err := r.Handle(context.Background(), "test/note", []byte(`{"text":"x"}`), false); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got != "x" {
		t.Errorf("notification saw %q, want %q", got, "x")
	}
}

func TestRouterUnknownCallIsMethodNotFound(t *testing.T) {
	r := newTestRouter()

	_, err := r.Handle(context.Background(), "nope", nil, true)

	assertCode(t, err, jsonrpc2.MethodNotFound)
}

func TestRouterUnknownNotificationIsIgnored(t *testing.T) {
	r := newTestRouter()

	if _, err := r.Handle(context.Background(), "$/unknown", nil, false); err != nil {
		t.Errorf("unknown notification returned %v, want nil", err)
	}
}

func TestRouterBadParamsInCallIsInvalidParams(t *testing.T) {
	r := newTestRouter()
	HandleRequest(r, "test/echo", func(_ context.Context, p *echoParams) (string, error) { return "", nil })

	_, err := r.Handle(context.Background(), "test/echo", []byte(`{"text":42}`), true)

	assertCode(t, err, jsonrpc2.InvalidParams)
}

func TestRouterBadParamsInNotificationIsDropped(t *testing.T) {
	r := newTestRouter()
	called := false
	HandleNotification(r, "test/note", func(_ context.Context, p *echoParams) error {
		called = true
		return nil
	})

	_, err := r.Handle(context.Background(), "test/note", []byte(`{"text":42}`), false)

	if err != nil || called {
		t.Errorf("Handle = %v, called = %v; want nil error and handler not called", err, called)
	}
}

func assertCode(t *testing.T, err error, want jsonrpc2.Code) {
	t.Helper()
	var rpcErr *jsonrpc2.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != want {
		t.Fatalf("error = %v, want JSON-RPC code %d", err, want)
	}
}
