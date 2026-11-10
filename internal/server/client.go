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

func (c *Client) connect(conn jsonrpc2.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn = conn
}
