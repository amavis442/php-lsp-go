package server

import (
	"bytes"
	"context"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amavis442/php-lsp-go/internal/testsupport/lspclient"
)

// lockedBuffer is a log destination tests can read while the server writes.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// exitSession initializes s, runs setup, sends shutdown and exit, and
// returns how long Serve took to return after exit.
func exitSession(t *testing.T, s *Server, setup func(c *lspclient.Client)) time.Duration {
	t.Helper()
	c, done := serve(t, s)
	initialize(t, c)
	if setup != nil {
		setup(c)
	}
	if resp, err := c.Call("shutdown", nil); err != nil || resp.Error != nil {
		t.Fatalf("shutdown: err=%v resp.Error=%+v", err, resp.Error)
	}
	start := time.Now()
	if err := c.Notify("exit", nil); err != nil {
		t.Fatalf("exit: %v", err)
	}
	// Keep reading, as an editor would, so late writes cannot block.
	go func() { _, _ = c.WaitNotification("test/never") }()
	select {
	case <-done:
		return time.Since(start)
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after exit")
		return 0
	}
}

func sendAndWaitStarted(t *testing.T, c *lspclient.Client, method string, started <-chan struct{}) {
	t.Helper()
	if _, err := c.Send(method, nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not start", method)
	}
}

func TestEndingTheSessionCancelsRunningRequestsAndWaitsForThem(t *testing.T) {
	s := newTestServer()
	started := make(chan struct{}, 1)
	var finished atomic.Bool
	HandleRequest(s.Router(), "test/slow", func(ctx context.Context, _ *struct{}) (string, error) {
		started <- struct{}{}
		<-ctx.Done()
		finished.Store(true)
		return "", ctx.Err()
	})

	exitSession(t, s, func(c *lspclient.Client) { sendAndWaitStarted(t, c, "test/slow", started) })

	if !finished.Load() {
		t.Error("Serve returned before the running request saw its cancellation and finished")
	}
}

func TestARequestThatIgnoresCancellationDoesNotBlockExit(t *testing.T) {
	var logs lockedBuffer
	s := New(Info{Name: "php-lsp-go", Version: "test"}, log.New(&logs, "", 0))
	s.closeTimeout = 50 * time.Millisecond
	started, never := make(chan struct{}, 1), make(chan struct{})
	t.Cleanup(func() { close(never) })
	HandleRequest(s.Router(), "test/stuck", func(context.Context, *struct{}) (string, error) {
		started <- struct{}{}
		<-never
		return "", nil
	})

	took := exitSession(t, s, func(c *lspclient.Client) { sendAndWaitStarted(t, c, "test/stuck", started) })

	if took > 2*time.Second {
		t.Errorf("exit took %s, want about the close timeout", took)
	}
	if !strings.Contains(logs.String(), "requests still running") {
		t.Errorf("log = %q, want a note about the request still running", logs.String())
	}
}

func TestASlowBeforeCloseHookDoesNotBlockExit(t *testing.T) {
	var logs lockedBuffer
	s := New(Info{Name: "php-lsp-go", Version: "test"}, log.New(&logs, "", 0))
	s.closeTimeout = 50 * time.Millisecond
	never := make(chan struct{})
	t.Cleanup(func() { close(never) })
	s.BeforeClose(func() { <-never })

	took := exitSession(t, s, nil)

	if took > 2*time.Second {
		t.Errorf("exit took %s, want about the close timeout", took)
	}
	if !strings.Contains(logs.String(), "did not finish") {
		t.Errorf("log = %q, want a note about the hook", logs.String())
	}
}

func TestAQuickTeardownLogsNothing(t *testing.T) {
	var logs lockedBuffer
	s := New(Info{Name: "php-lsp-go", Version: "test"}, log.New(&logs, "", 0))
	ran := false
	s.BeforeClose(func() { ran = true })

	exitSession(t, s, nil)

	if !ran || logs.String() != "" {
		t.Errorf("hook ran = %v, log = %q; want it run and nothing logged", ran, logs.String())
	}
}
