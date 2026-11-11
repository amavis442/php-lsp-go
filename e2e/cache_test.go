package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// indexedSession starts the server on root with cache, waits for the scan,
// and returns the process and a search that counts exact name matches.
func indexedSession(t *testing.T, root, cache string) (*process, func(string) int) {
	t.Helper()
	p := startWithCache(t, cache)
	resp, err := p.client.Call("initialize", map[string]any{
		"processId": os.Getpid(), "rootUri": fileURI(root),
		"capabilities": map[string]any{"window": map[string]any{"workDoneProgress": true}},
	})
	if err != nil || resp.Error != nil {
		t.Fatalf("initialize: err=%v resp.Error=%+v", err, resp.Error)
	}
	if err := p.client.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("initialized: %v", err)
	}
	waitForIndexing(t, p)
	count := func(name string) int {
		t.Helper()
		resp, err := p.client.Call("workspace/symbol", map[string]any{"query": name})
		if err != nil || resp.Error != nil {
			t.Fatalf("workspace/symbol: err=%v resp.Error=%+v", err, resp.Error)
		}
		var got []symbol
		if err := json.Unmarshal(resp.Result, &got); err != nil {
			t.Fatalf("decoding %s: %v", resp.Result, err)
		}
		n := 0
		for _, s := range got {
			if s.Name == name {
				n++
			}
		}
		return n
	}
	return p, count
}

func endSession(t *testing.T, p *process) {
	t.Helper()
	if resp, err := p.client.Call("shutdown", nil); err != nil || resp.Error != nil {
		t.Fatalf("shutdown: err=%v resp.Error=%+v", err, resp.Error)
	}
	if err := p.client.Notify("exit", nil); err != nil {
		t.Fatalf("exit: %v", err)
	}
	if code := p.exitCode(t); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if p.stderr.Len() != 0 {
		t.Errorf("stderr not empty:\n%s", p.stderr)
	}
}

func TestTheNextSessionStartsFromTheSavedScan(t *testing.T) {
	root, cache := t.TempDir(), t.TempDir()
	same := filepath.Join(root, "src", "Same.php")
	if err := os.MkdirAll(filepath.Dir(same), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(same, []byte("<?php\nclass Alpha {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, count := indexedSession(t, root, cache)
	if count("Alpha") != 1 || count("array_map") != 1 {
		t.Errorf("first session: Alpha %d, array_map %d; want both", count("Alpha"), count("array_map"))
	}
	endSession(t, p)
	saved, _ := filepath.Glob(filepath.Join(cache, "php-lsp-go", "index", "*.gob"))
	if len(saved) != 1 {
		t.Fatalf("saved scans: %v, want one", saved)
	}

	// Same content size and time, other content: only a reused scan still
	// knows Alpha.
	info, err := os.Stat(same)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(same, []byte("<?php\nclass Gamma {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(same, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	p, count = indexedSession(t, root, cache)
	t.Logf("second session indexed in %s", time.Since(started))
	if count("Alpha") != 1 || count("Gamma") != 0 || count("array_map") != 1 {
		t.Errorf("second session: Alpha %d, Gamma %d, array_map %d; want the saved scan, stubs included",
			count("Alpha"), count("Gamma"), count("array_map"))
	}
	endSession(t, p)
}
