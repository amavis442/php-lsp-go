package e2e

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fileURI builds a file URI the way an editor does, without using the
// server's own code.
func fileURI(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

func TestWorkspaceSymbolsFindDeclarationsAcrossTheProject(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		filepath.Join(root, "app", "Billing", "Invoice.php"): "<?php\nnamespace App\\Billing;\n\nfinal class Invoice\n{\n    public function total(): int { return 0; }\n}\n",
		filepath.Join(root, "app", "helpers.php"):            "<?php\nfunction format_invoice_number(int $n): string { return ''; }\n",
		filepath.Join(root, "node_modules", "x", "y.php"):    "<?php\nclass InvoiceFromNodeModules {}\n",
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	p := start(t)
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

	resp, err = p.client.Call("workspace/symbol", map[string]any{"query": "invoice"})
	if err != nil || resp.Error != nil {
		t.Fatalf("workspace/symbol: err=%v resp.Error=%+v\nstderr:\n%s", err, resp.Error, p.stderr)
	}
	var all []symbol
	if err := json.Unmarshal(resp.Result, &all); err != nil {
		t.Fatalf("decoding %s: %v", resp.Result, err)
	}
	// Built-in declarations from the stubs may match as well.
	var got []symbol
	for _, s := range all {
		if strings.HasPrefix(s.Location.URI, fileURI(root)+"/") {
			got = append(got, s)
		}
	}
	if len(got) != 2 || got[0].Name != "Invoice" || got[0].ContainerName != `App\Billing` ||
		!strings.HasSuffix(got[0].Location.URI, "/app/Billing/Invoice.php") ||
		got[1].Name != "format_invoice_number" {
		t.Errorf("workspace/symbol(invoice) = %s, want Invoice then format_invoice_number, and nothing from node_modules", resp.Result)
	}

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

func TestWorkspaceSymbolsFindBuiltInDeclarations(t *testing.T) {
	p := start(t)
	resp, err := p.client.Call("initialize", map[string]any{
		"processId": os.Getpid(), "rootUri": nil,
		"capabilities": map[string]any{"window": map[string]any{"workDoneProgress": true}},
	})
	if err != nil || resp.Error != nil {
		t.Fatalf("initialize: err=%v resp.Error=%+v", err, resp.Error)
	}
	if err := p.client.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("initialized: %v", err)
	}
	waitForIndexing(t, p)

	// The stubs are installed as files, so the editor can open each location.
	installed := map[string]bool{}
	err = filepath.WalkDir(filepath.Join(p.cache, "stubs"), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			installed[fileURI(path)] = true
		}
		return err
	})
	if err != nil {
		t.Fatalf("listing the installed stubs: %v", err)
	}

	for query, kind := range map[string]int{"array_map": 12, "ArrayIterator": 5, "Stringable": 11, "PHP_EOL": 14} {
		resp, err := p.client.Call("workspace/symbol", map[string]any{"query": query})
		if err != nil || resp.Error != nil {
			t.Fatalf("workspace/symbol: err=%v resp.Error=%+v\nstderr:\n%s", err, resp.Error, p.stderr)
		}
		var got []symbol
		if err := json.Unmarshal(resp.Result, &got); err != nil {
			t.Fatalf("decoding %s: %v", resp.Result, err)
		}
		if len(got) == 0 || got[0].Name != query || got[0].Kind != kind || !installed[got[0].Location.URI] {
			t.Errorf("workspace/symbol(%s) = %s, want it first, kind %d, in a stub file under %s", query, resp.Result, kind, p.cache)
		}
	}

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

type symbol struct {
	Name          string `json:"name"`
	Kind          int    `json:"kind"`
	ContainerName string `json:"containerName"`
	Location      struct {
		URI string `json:"uri"`
	} `json:"location"`
}

func waitForIndexing(t *testing.T, p *process) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		msg, err := p.client.WaitNotification("$/progress")
		if err != nil {
			t.Fatalf("waiting for indexing: %v\nstderr:\n%s", err, p.stderr)
		}
		if strings.Contains(string(msg.Params), `"kind":"end"`) {
			return
		}
	}
	t.Fatal("indexing did not finish within 20s")
}
