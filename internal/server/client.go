package server

import (
	"context"
	"errors"
	"sync"

	"go.lsp.dev/jsonrpc2"
)

var errNotConnected = errors.New("no editor connected")

// Client sends notifications to the editor. Features get it before Serve
// starts; it works once Serve has connected and until the session ends.
type Client struct {
	mu   sync.Mutex
	conn jsonrpc2.Conn
}

// Notify sends a notification to the editor. It is safe for concurrent use.
func (c *Client) Notify(ctx context.Context, method string, params any) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return errNotConnected
	}
	return conn.Notify(ctx, method, params)
}

// Call sends a request to the editor and waits for its answer, which is
// decoded into result. Request handlers may use it. Notification handlers
// must not: they run on the read loop that has to read the answer, so the
// call would never return. Start a goroutine for that instead.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return errNotConnected
	}
	_, err := conn.Call(ctx, method, params, result)
	return err
}

func (c *Client) connect(conn jsonrpc2.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn = conn
}
