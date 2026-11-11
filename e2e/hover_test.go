package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHoverShowsSignaturesAndDocs(t *testing.T) {
	root := t.TempDir()
	invoice := filepath.Join(root, "app", "Billing", "Invoice.php")
	if err := os.MkdirAll(filepath.Dir(invoice), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "<?php\nnamespace App\\Billing;\n\n/** A bill sent to a customer. */\nfinal class Invoice\n{\n}\n"
	if err := os.WriteFile(invoice, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p := start(t)
	resp, err := p.client.Call("initialize", map[string]any{
		"processId": os.Getpid(), "rootUri": fileURI(root),
		"capabilities": map[string]any{"window": map[string]any{"workDoneProgress": true}},
	})
	if err != nil || resp.Error != nil {
		t.Fatalf("initialize: err=%v resp.Error=%+v", err, resp.Error)
	}
	var init struct {
		Capabilities struct {
			HoverProvider bool `json:"hoverProvider"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(resp.Result, &init); err != nil || !init.Capabilities.HoverProvider {
		t.Errorf("initialize result %s: want hoverProvider", resp.Result)
	}
	if err := p.client.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("initialized: %v", err)
	}
	waitForIndexing(t, p)

	uri := fileURI(filepath.Join(root, "app", "Http", "Pay.php"))
	src := "<?php\nnamespace App\\Http;\n\nuse App\\Billing\\Invoice;\n\n" +
		"function pay(Invoice $invoice): array\n{\n    return array_map(fn($x) => $x, []);\n}\nget_required_files();\n"
	if err := p.client.Notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": uri, "languageId": "php", "version": 1, "text": src,
	}}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	hover := func(line, character int) string {
		t.Helper()
		resp, err := p.client.Call("textDocument/hover", map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position":     map[string]any{"line": line, "character": character},
		})
		if err != nil || resp.Error != nil {
			t.Fatalf("hover: err=%v resp.Error=%+v\nstderr:\n%s", err, resp.Error, p.stderr)
		}
		var got struct {
			Contents struct {
				Kind  string `json:"kind"`
				Value string `json:"value"`
			} `json:"contents"`
		}
		if err := json.Unmarshal(resp.Result, &got); err != nil {
			t.Fatalf("decoding %s: %v", resp.Result, err)
		}
		return got.Contents.Value
	}

	// "function pay(Invoice" on line 5.
	if got, want := hover(5, 15), "```php\n<?php\nfinal class Invoice\n```\n\n`App\\Billing`\n\nA bill sent to a customer."; got != want {
		t.Errorf("Invoice:\n got %q\nwant %q", got, want)
	}
	// "    return array_map" on line 7, from the real stubs.
	want := "```php\n<?php\nfunction array_map(?callable $callback, array $array, array ...$arrays): array\n```\n\n" +
		"Applies the callback to the elements of the given arrays"
	if got := hover(7, 13); got != want {
		t.Errorf("array_map:\n got %q\nwant %q", got, want)
	}
	// "get_required_files" on line 9: the stub's summary is HTML.
	want = "```php\n<?php\nfunction get_required_files(): array\n```\n\nAlias of **get_included_files**"
	if got := hover(9, 3); got != want {
		t.Errorf("get_required_files:\n got %q\nwant %q", got, want)
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
