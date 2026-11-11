package server

import (
	"testing"
	"time"
)

// startSession initializes s with params and returns the Session that
// OnInitialized received.
func startSession(t *testing.T, params map[string]any) Session {
	t.Helper()
	s := newTestServer()
	got := make(chan Session, 1)
	s.OnInitialized(func(session Session) { got <- session })
	c, _ := serve(t, s)

	if resp, err := c.Call("initialize", params); err != nil || resp.Error != nil {
		t.Fatalf("initialize: err=%v resp.Error=%+v", err, resp.Error)
	}
	if err := c.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("initialized: %v", err)
	}
	select {
	case session := <-got:
		return session
	case <-time.After(5 * time.Second):
		t.Fatal("OnInitialized was not called")
		return Session{}
	}
}

func TestSessionPrefersWorkspaceFolders(t *testing.T) {
	session := startSession(t, map[string]any{
		"processId": nil, "rootUri": "file:///D:/old", "capabilities": map[string]any{},
		"workspaceFolders": []any{
			map[string]any{"uri": "file:///D:/work/api", "name": "api"},
			map[string]any{"uri": "file:///D:/work/web", "name": "web"},
		},
	})

	if len(session.Roots) != 2 {
		t.Fatalf("roots = %v, want the two folders", session.Roots)
	}
}

func TestSessionFallsBackToRootURI(t *testing.T) {
	session := startSession(t, map[string]any{
		"processId": nil, "rootUri": "file:///D:/work/app", "capabilities": map[string]any{},
	})

	if len(session.Roots) != 1 {
		t.Errorf("roots = %v, want the root URI", session.Roots)
	}
}

func TestSessionWithoutAWorkspaceHasNoRoots(t *testing.T) {
	session := startSession(t, map[string]any{"processId": nil, "rootUri": nil, "capabilities": map[string]any{}})

	if len(session.Roots) != 0 || session.WorkDoneProgress || session.WatchedFilesRegistration {
		t.Errorf("session = %+v, want nothing", session)
	}
}

func TestSessionReportsClientCapabilities(t *testing.T) {
	session := startSession(t, map[string]any{
		"processId": nil, "rootUri": nil,
		"capabilities": map[string]any{
			"window":    map[string]any{"workDoneProgress": true},
			"workspace": map[string]any{"didChangeWatchedFiles": map[string]any{"dynamicRegistration": true}},
		},
	})

	if !session.WorkDoneProgress || !session.WatchedFilesRegistration {
		t.Errorf("session = %+v, want progress and watched-file registration", session)
	}
}
