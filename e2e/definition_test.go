package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type definitionLocation struct {
	URI   string `json:"uri"`
	Range struct {
		Start struct{ Line, Character int } `json:"start"`
	} `json:"range"`
}

func TestDefinitionJumpsToProjectClassesAndBuiltIns(t *testing.T) {
	root := t.TempDir()
	invoice := filepath.Join(root, "app", "Billing", "Invoice.php")
	if err := os.MkdirAll(filepath.Dir(invoice), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(invoice, []byte("<?php\nnamespace App\\Billing;\n\nfinal class Invoice\n{\n}\n"), 0o644); err != nil {
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
			DefinitionProvider bool `json:"definitionProvider"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(resp.Result, &init); err != nil || !init.Capabilities.DefinitionProvider {
		t.Errorf("initialize result %s: want definitionProvider", resp.Result)
	}
	if err := p.client.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("initialized: %v", err)
	}
	waitForIndexing(t, p)

	uri := fileURI(filepath.Join(root, "app", "Http", "Pay.php"))
	src := "<?php\nnamespace App\\Http;\n\nuse App\\Billing\\Invoice;\n\n" +
		"function pay(Invoice $invoice): array\n{\n    return array_map(fn($x) => $x, []);\n}\n"
	if err := p.client.Notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": uri, "languageId": "php", "version": 1, "text": src,
	}}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	define := func(line, character int) []definitionLocation {
		t.Helper()
		resp, err := p.client.Call("textDocument/definition", map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position":     map[string]any{"line": line, "character": character},
		})
		if err != nil || resp.Error != nil {
			t.Fatalf("definition: err=%v resp.Error=%+v\nstderr:\n%s", err, resp.Error, p.stderr)
		}
		var got []definitionLocation
		if err := json.Unmarshal(resp.Result, &got); err != nil {
			t.Fatalf("decoding %s: %v", resp.Result, err)
		}
		return got
	}

	// "function pay(Invoice" on line 5: the type starts at character 13.
	if got := define(5, 15); len(got) != 1 || got[0].URI != fileURI(invoice) ||
		got[0].Range.Start.Line != 3 || got[0].Range.Start.Character != 12 {
		t.Errorf("Invoice: %+v, want Invoice.php 3:12", got)
	}
	// "    return array_map" on line 7: the call starts at character 11.
	stub := fileURI(filepath.Join(p.cache, "stubs", "v2026.2", "standard", "standard_9.php"))
	if got := define(7, 13); len(got) != 1 || got[0].URI != stub {
		t.Errorf("array_map: %+v, want %s", got, stub)
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
