// Package lspclient is a minimal LSP client for tests. It speaks the LSP base
// protocol by hand (a Content-Length header, a blank line, then a JSON-RPC 2.0
// body) so tests talk to the server exactly as an editor does, without
// depending on the protocol library the server itself uses.
package lspclient

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"sync"
)

// Message is any JSON-RPC message received from the server: a response
// (ID plus Result or Error), a notification (Method plus Params) or a
// server-to-client request (ID plus Method).
type Message struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *ResponseError  `json:"error,omitempty"`
}

// ResponseError is the error object of a JSON-RPC response.
type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Client sends requests and notifications to a language server. It is meant
// for sequential use from a single test goroutine.
type Client struct {
	r      *bufio.Reader
	w      io.Writer
	nextID int64

	mu            sync.Mutex
	notifications []Message
}

// New returns a client that reads server output from r and writes to w.
func New(r io.Reader, w io.Writer) *Client {
	return &Client{r: bufio.NewReader(r), w: w}
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Call sends a request and blocks until the response with the same id
// arrives. Notifications received meanwhile are kept for Notifications.
func (c *Client) Call(method string, params any) (Message, error) {
	c.nextID++
	id := c.nextID
	if err := WriteMessage(c.w, request{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		return Message{}, err
	}

	want := strconv.FormatInt(id, 10)
	for {
		body, err := ReadMessage(c.r)
		if err != nil {
			return Message{}, fmt.Errorf("waiting for response to %s: %w", method, err)
		}
		var msg Message
		if err := json.Unmarshal(body, &msg); err != nil {
			return Message{}, fmt.Errorf("decoding %s: %w", body, err)
		}
		if msg.Method == "" && string(msg.ID) == want {
			return msg, nil
		}
		c.mu.Lock()
		c.notifications = append(c.notifications, msg)
		c.mu.Unlock()
	}
}

// Notify sends a notification; the server sends no response.
func (c *Client) Notify(method string, params any) error {
	return WriteMessage(c.w, notification{JSONRPC: "2.0", Method: method, Params: params})
}

// Notifications returns the messages without a matching request that were
// received while waiting in Call.
func (c *Client) Notifications() []Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Message(nil), c.notifications...)
}

// WriteMessage encodes v as JSON and writes it with a Content-Length header.
func WriteMessage(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

// ReadMessage reads one framed message and returns its JSON body. Headers
// other than Content-Length are ignored.
func ReadMessage(r *bufio.Reader) ([]byte, error) {
	header, err := textproto.NewReader(r).ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	value := header.Get("Content-Length")
	if value == "" {
		return nil, errors.New("missing Content-Length header")
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return nil, fmt.Errorf("invalid Content-Length %q", value)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}
