package server

import (
	"sync"

	"go.lsp.dev/jsonrpc2"
)

// codeServerNotInitialized is the LSP error for requests sent before initialize.
const codeServerNotInitialized jsonrpc2.Code = -32002

type phase int

const (
	phaseStarting phase = iota
	phaseRunning
	phaseShuttingDown
)

// lifecycle tracks the LSP session state machine:
// starting --initialize--> running --shutdown--> shutting down --exit.
type lifecycle struct {
	mu    sync.Mutex
	phase phase
}

// admit reports whether a message for method is allowed in the current phase.
// The error is a JSON-RPC error the caller can send back as is.
func (l *lifecycle) admit(method string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if method == "exit" {
		return nil
	}
	switch l.phase {
	case phaseStarting:
		if method == "initialize" {
			return nil
		}
		return jsonrpc2.NewError(codeServerNotInitialized, "server not initialized")
	case phaseRunning:
		if method == "initialize" {
			return jsonrpc2.NewError(jsonrpc2.InvalidRequest, "server already initialized")
		}
		return nil
	default:
		return jsonrpc2.NewError(jsonrpc2.InvalidRequest, "server is shutting down")
	}
}

func (l *lifecycle) markInitialized() { l.set(phaseRunning) }

func (l *lifecycle) markShutdown() { l.set(phaseShuttingDown) }

func (l *lifecycle) set(p phase) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.phase = p
}

// exitCode is 0 when the client asked for shutdown before exit, 1 otherwise,
// as the LSP specification requires.
func (l *lifecycle) exitCode() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.phase == phaseShuttingDown {
		return 0
	}
	return 1
}
