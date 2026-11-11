package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.lsp.dev/jsonrpc2"
)

// blockUntilCancelled is a request handler that reports when it starts and
// returns only when its request is cancelled or release is closed.
func blockUntilCancelled(started chan<- struct{}, release <-chan struct{}) func(context.Context, *struct{}) (string, error) {
	return func(ctx context.Context, _ *struct{}) (string, error) {
		started <- struct{}{}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-release:
			return "slow done", nil
		}
	}
}

func TestASlowRequestDoesNotBlockLaterRequests(t *testing.T) {
	s := newTestServer()
	started, release := make(chan struct{}, 1), make(chan struct{})
	HandleRequest(s.Router(), "test/slow", blockUntilCancelled(started, release))
	HandleRequest(s.Router(), "test/fast", func(context.Context, *struct{}) (string, error) { return "fast", nil })
	c, _ := serve(t, s)
	initialize(t, c)

	slow, err := c.Send("test/slow", nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	<-started
	fast, err := c.Call("test/fast", nil)
	if err != nil || string(fast.Result) != `"fast"` {
		t.Fatalf("test/fast while test/slow runs = %+v, %v", fast, err)
	}
	close(release)
	if got, err := c.Await(slow); err != nil || string(got.Result) != `"slow done"` {
		t.Errorf("test/slow = %+v, %v", got, err)
	}
}

func TestCancelRequestAnswersRequestCancelled(t *testing.T) {
	s := newTestServer()
	started := make(chan struct{}, 1)
	HandleRequest(s.Router(), "test/slow", blockUntilCancelled(started, nil))
	c, _ := serve(t, s)
	initialize(t, c)

	id, err := c.Send("test/slow", nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	<-started
	if err := c.Notify("$/cancelRequest", map[string]any{"id": id}); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	got, err := c.Await(id)
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if got.Error == nil || got.Error.Code != -32800 {
		t.Errorf("response = %+v, want error code -32800 (RequestCancelled)", got)
	}
}

func TestCancellingAnUnknownRequestIsHarmless(t *testing.T) {
	s := newTestServer()
	HandleRequest(s.Router(), "test/fast", func(context.Context, *struct{}) (string, error) { return "fast", nil })
	c, _ := serve(t, s)
	initialize(t, c)

	if err := c.Notify("$/cancelRequest", map[string]any{"id": "no-such-request"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	if got, err := c.Call("test/fast", nil); err != nil || string(got.Result) != `"fast"` {
		t.Errorf("test/fast after cancel = %+v, %v", got, err)
	}
}

func TestARequestHandlerCanCallTheEditor(t *testing.T) {
	s := newTestServer()
	HandleRequest(s.Router(), "test/ask", func(ctx context.Context, _ *struct{}) (string, error) {
		var answer string
		err := s.Client().Call(ctx, "test/question", map[string]string{"q": "life"}, &answer)
		return answer, err
	})
	c, _ := serve(t, s)
	initialize(t, c)
	c.Respond("test/question", "42")

	done := make(chan struct{})
	var got json.RawMessage
	var callErr error
	go func() {
		defer close(done)
		resp, err := c.Call("test/ask", nil)
		got, callErr = resp.Result, err
		if err == nil && resp.Error != nil {
			t.Errorf("test/ask error = %+v", resp.Error)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("test/ask did not finish: the call back to the editor deadlocked")
	}

	if callErr != nil || string(got) != `"42"` {
		t.Errorf("test/ask = %s, %v; want \"42\"", got, callErr)
	}
	if reqs := c.ServerRequests(); len(reqs) != 1 || reqs[0].Method != "test/question" ||
		string(reqs[0].Params) != `{"q":"life"}` {
		t.Errorf("server requests = %+v, want one test/question", reqs)
	}
}

func TestClientCallBeforeServeFails(t *testing.T) {
	s := newTestServer()

	if err := s.Client().Call(context.Background(), "test/question", nil, nil); err == nil {
		t.Error("Call before Serve succeeded, want error")
	}
}

func TestRequestIDAcceptsNumbersAndStrings(t *testing.T) {
	tests := []struct {
		raw  string
		want jsonrpc2.ID
		ok   bool
	}{
		{`7`, jsonrpc2.NewNumberID(7), true},
		{`"abc"`, jsonrpc2.NewStringID("abc"), true},
		{`null`, jsonrpc2.ID{}, false},
		{`{"x":1}`, jsonrpc2.ID{}, false},
	}
	for _, tt := range tests {
		got, ok := requestID(json.RawMessage(tt.raw))
		if ok != tt.ok || got != tt.want {
			t.Errorf("requestID(%s) = %v, %v; want %v, %v", tt.raw, got, ok, tt.want, tt.ok)
		}
	}
}

func TestARequestStartingAfterCancelAllStartsCancelled(t *testing.T) {
	var r runningRequests
	r.cancelAll()

	ctx, done := r.start(context.Background(), jsonrpc2.NewNumberID(1))
	defer done()

	if ctx.Err() == nil {
		t.Error("request started after cancelAll is not cancelled")
	}
	waited := make(chan struct{})
	go func() { r.wait(); close(waited) }()
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Error("wait blocked on a request started after cancelAll")
	}
}
