package server

import (
	"errors"
	"testing"

	"go.lsp.dev/jsonrpc2"
)

func TestLifecycleAdmit(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(l *lifecycle)
		method   string
		wantCode jsonrpc2.Code // 0 means admitted
	}{
		{"initialize before init", func(*lifecycle) {}, "initialize", 0},
		{"exit before init", func(*lifecycle) {}, "exit", 0},
		{"request before init", func(*lifecycle) {}, "textDocument/hover", codeServerNotInitialized},
		{"shutdown before init", func(*lifecycle) {}, "shutdown", codeServerNotInitialized},
		{"request after init", initialized, "textDocument/hover", 0},
		{"second initialize", initialized, "initialize", jsonrpc2.InvalidRequest},
		{"exit after shutdown", shutDown, "exit", 0},
		{"request after shutdown", shutDown, "textDocument/hover", jsonrpc2.InvalidRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var l lifecycle
			tt.setup(&l)

			err := l.admit(tt.method)

			if tt.wantCode == 0 {
				if err != nil {
					t.Fatalf("admit(%q) = %v, want admitted", tt.method, err)
				}
				return
			}
			var rpcErr *jsonrpc2.Error
			if !errors.As(err, &rpcErr) || rpcErr.Code != tt.wantCode {
				t.Fatalf("admit(%q) = %v, want code %d", tt.method, err, tt.wantCode)
			}
		})
	}
}

func TestLifecycleExitCode(t *testing.T) {
	var withoutShutdown lifecycle
	initialized(&withoutShutdown)
	if got := withoutShutdown.exitCode(); got != 1 {
		t.Errorf("exit code without shutdown = %d, want 1", got)
	}

	var afterShutdown lifecycle
	shutDown(&afterShutdown)
	if got := afterShutdown.exitCode(); got != 0 {
		t.Errorf("exit code after shutdown = %d, want 0", got)
	}
}

func initialized(l *lifecycle) { l.markInitialized() }

func shutDown(l *lifecycle) {
	l.markInitialized()
	l.markShutdown()
}
