package app

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/amavis442/php-lsp-go/internal/core/fileuri"
	"github.com/amavis442/php-lsp-go/internal/server"
	"github.com/amavis442/php-lsp-go/internal/testsupport/lspclient"
)

type symbolInformation struct {
	Name          string `json:"name"`
	Kind          int    `json:"kind"`
	ContainerName string `json:"containerName"`
	Location      struct {
		URI string `json:"uri"`
	} `json:"location"`
}

func writePHP(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func workspaceSymbols(t *testing.T, c *lspclient.Client, query string) []symbolInformation {
	t.Helper()
	resp, err := c.Call("workspace/symbol", map[string]any{"query": query})
	if err != nil || resp.Error != nil {
		t.Fatalf("workspace/symbol: err=%v resp.Error=%+v", err, resp.Error)
	}
	var got []symbolInformation
	if err := json.Unmarshal(resp.Result, &got); err != nil {
		t.Fatalf("decoding %s: %v", resp.Result, err)
	}
	return got
}

// waitIndexed waits for the end of the indexing progress.
func waitIndexed(t *testing.T, c *lspclient.Client) {
	t.Helper()
	for {
		msg, err := c.WaitNotification("$/progress")
		if err != nil {
			t.Fatalf("waiting for indexing progress: %v", err)
		}
		var p struct {
			Value struct {
				Kind string `json:"kind"`
			} `json:"value"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			t.Fatalf("decoding %s: %v", msg.Params, err)
		}
		if p.Value.Kind == "end" {
			return
		}
	}
}

func TestTheWorkspaceIsIndexedAndSearchable(t *testing.T) {
	root := t.TempDir()
	writePHP(t, filepath.Join(root, "src", "Models", "User.php"),
		"<?php\nnamespace App\\Models;\n\nclass User\n{\n    public function fullName(): string {}\n}\n")
	writePHP(t, filepath.Join(root, "vendor", "acme", "lib", "Helpers.php"), "<?php\nfunction acme_user_helper() {}\n")

	c, done := startAppWith(t, New(server.Info{Name: "php-lsp-go", Version: "test"}, log.New(io.Discard, "", 0), time.Millisecond),
		map[string]any{
			"processId": nil, "rootUri": fileuri.FromPath(root),
			"capabilities": map[string]any{
				"window":    map[string]any{"workDoneProgress": true},
				"workspace": map[string]any{"didChangeWatchedFiles": map[string]any{"dynamicRegistration": true}},
			},
		})
	waitIndexed(t, c)

	got := workspaceSymbols(t, c, "user")
	if len(got) != 2 || got[0].Name != "User" || got[0].Kind != 5 || got[0].ContainerName != `App\Models` ||
		got[0].Location.URI != fileuri.FromPath(filepath.Join(root, "src", "Models", "User.php")) ||
		got[1].Name != "acme_user_helper" {
		t.Errorf("workspace/symbol(user) = %+v, want class User, then the vendor helper", got)
	}
	if reqs := c.ServerRequests(); len(reqs) != 2 ||
		reqs[0].Method != "client/registerCapability" || reqs[1].Method != "window/workDoneProgress/create" {
		t.Errorf("server requests = %+v, want a watcher registration and a progress token", reqs)
	}

	// A file the editor reports as created is indexed too.
	order := filepath.Join(root, "src", "Order.php")
	writePHP(t, order, "<?php\nclass Order {}\n")
	notify(t, c, "workspace/didChangeWatchedFiles", map[string]any{"changes": []any{
		map[string]any{"uri": fileuri.FromPath(order), "type": 1},
	}})
	deadline := time.Now().Add(5 * time.Second)
	for len(workspaceSymbols(t, c, "order")) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("created file was not indexed")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if _, err := c.Call("shutdown", nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	notify(t, c, "exit", nil)
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after exit")
	}
}
