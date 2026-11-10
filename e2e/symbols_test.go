package e2e

import (
	"encoding/json"
	"testing"
)

type documentSymbol struct {
	Name     string           `json:"name"`
	Kind     int              `json:"kind"`
	Children []documentSymbol `json:"children"`
}

func TestDocumentSymbolsOfAnOpenFile(t *testing.T) {
	const uri = "file:///D:/work/Account.php"
	p := start(t)
	p.initialize(t)

	if err := p.client.Notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": uri, "languageId": "php", "version": 1,
		"text": "<?php\nenum Status { case Open; }\nfunction open_account() {}\n",
	}}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}
	resp, err := p.client.Call("textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": uri}})
	if err != nil || resp.Error != nil {
		t.Fatalf("documentSymbol: err=%v resp.Error=%+v\nstderr:\n%s", err, resp.Error, p.stderr)
	}
	var got []documentSymbol
	if err := json.Unmarshal(resp.Result, &got); err != nil {
		t.Fatalf("decoding %s: %v", resp.Result, err)
	}
	if len(got) != 2 ||
		got[0].Name != "Status" || got[0].Kind != 10 || len(got[0].Children) != 1 || got[0].Children[0].Name != "Open" ||
		got[1].Name != "open_account" || got[1].Kind != 12 {
		t.Errorf("symbols = %s, want enum Status with case Open, then function open_account", resp.Result)
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
