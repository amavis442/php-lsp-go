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
// for sequential use from a single test goroutine. While it reads, it answers
// every request the server sends it, as an editor would.
type Client struct {
	r      *bufio.Reader
	w      io.Writer
	nextID int64

	mu             sync.Mutex
	notifications  []Message
	responses      map[string]Message // read while awaiting another id, by raw id
	responders     map[string]any     // result per server request method
	serverRequests []Message
}

// New returns a client that reads server output from r and writes to w.
func New(r io.Reader, w io.Writer) *Client {
	return &Client{
		r:          bufio.NewReader(r),
		w:          w,
		responses:  make(map[string]Message),
		responders: make(map[string]any),
	}
}

// Respond sets the result the client answers server requests for method
// with. Methods without one are answered with a null result.
func (c *Client) Respond(method string, result any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.responders[method] = result
}

// ServerRequests returns the requests the server sent, in the order they
// were answered.
func (c *Client) ServerRequests() []Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Message(nil), c.serverRequests...)
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

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result"`
}

// Call sends a request and blocks until the response with the same id
// arrives. Notifications received meanwhile are kept for Notifications.
func (c *Client) Call(method string, params any) (Message, error) {
	id, err := c.Send(method, params)
	if err != nil {
		return Message{}, err
	}
	msg, err := c.Await(id)
	if err != nil {
		return Message{}, fmt.Errorf("waiting for response to %s: %w", method, err)
	}
	return msg, nil
}

// Send sends a request without waiting for its response; Await collects it.
func (c *Client) Send(method string, params any) (int64, error) {
	c.nextID++
	id := c.nextID
	return id, WriteMessage(c.w, request{JSONRPC: "2.0", ID: id, Method: method, Params: params})
}

// Await blocks until the response to the request with id arrives. Other
// responses and notifications read meanwhile are kept.
func (c *Client) Await(id int64) (Message, error) {
	want := strconv.FormatInt(id, 10)
	c.mu.Lock()
	if msg, ok := c.responses[want]; ok {
		delete(c.responses, want)
		c.mu.Unlock()
		return msg, nil
	}
	c.mu.Unlock()

	for {
		msg, err := c.next()
		if err != nil {
			return Message{}, err
		}
		if msg.Method == "" && string(msg.ID) == want {
			return msg, nil
		}
		c.keep(msg)
	}
}

// next reads the next response or notification, answering the server's
// requests on the way.
func (c *Client) next() (Message, error) {
	for {
		body, err := ReadMessage(c.r)
		if err != nil {
			return Message{}, err
		}
		var msg Message
		if err := json.Unmarshal(body, &msg); err != nil {
			return Message{}, fmt.Errorf("decoding %s: %w", body, err)
		}
		if msg.Method == "" || len(msg.ID) == 0 {
			return msg, nil
		}
		if err := c.answer(msg); err != nil {
			return Message{}, fmt.Errorf("answering %s: %w", msg.Method, err)
		}
	}
}

func (c *Client) answer(req Message) error {
	c.mu.Lock()
	result := c.responders[req.Method]
	c.serverRequests = append(c.serverRequests, req)
	c.mu.Unlock()
	return WriteMessage(c.w, response{JSONRPC: "2.0", ID: req.ID, Result: result})
}

// keep stores a message nobody is waiting for yet.
func (c *Client) keep(msg Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if msg.Method == "" {
		c.responses[string(msg.ID)] = msg
		return
	}
	c.notifications = append(c.notifications, msg)
}

// Notify sends a notification; the server sends no response.
func (c *Client) Notify(method string, params any) error {
	return WriteMessage(c.w, notification{JSONRPC: "2.0", Method: method, Params: params})
}

// WaitNotification returns the first notification for method, taking it
// from the queue or reading from the server until it arrives. Other
// messages read meanwhile are kept for Notifications. It blocks until the
// notification arrives or the stream ends.
func (c *Client) WaitNotification(method string) (Message, error) {
	c.mu.Lock()
	for i, msg := range c.notifications {
		if msg.Method == method {
			c.notifications = append(c.notifications[:i], c.notifications[i+1:]...)
			c.mu.Unlock()
			return msg, nil
		}
	}
	c.mu.Unlock()

	for {
		msg, err := c.next()
		if err != nil {
			return Message{}, fmt.Errorf("waiting for %s: %w", method, err)
		}
		if msg.Method == method {
			return msg, nil
		}
		c.keep(msg)
	}
}

// Notifications returns the notifications nobody has taken yet that were
// received while waiting in Call, Await or WaitNotification.
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
