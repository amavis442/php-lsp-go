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

// A cancelled request is answered (with its result or RequestCancelled, as
// the race goes) and the session carries on.
func TestCancellingARequestKeepsTheSessionHealthy(t *testing.T) {
	const uri = "file:///D:/work/a.php"
	p := start(t)
	p.initialize(t)
	if err := p.client.Notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": uri, "languageId": "php", "version": 1, "text": "<?php\nclass A {}\n",
	}}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	params := map[string]any{"textDocument": map[string]any{"uri": uri}}
	id, err := p.client.Send("textDocument/documentSymbol", params)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := p.client.Notify("$/cancelRequest", map[string]any{"id": id}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	cancelled, err := p.client.Await(id)
	if err != nil {
		t.Fatalf("Await: %v\nstderr:\n%s", err, p.stderr)
	}
	if cancelled.Error != nil && cancelled.Error.Code != -32800 {
		t.Errorf("cancelled request error = %+v, want none or -32800", cancelled.Error)
	}

	again, err := p.client.Call("textDocument/documentSymbol", params)
	if err != nil || again.Error != nil {
		t.Fatalf("documentSymbol after cancel: err=%v resp.Error=%+v", err, again.Error)
	}
	var symbols []documentSymbol
	if err := json.Unmarshal(again.Result, &symbols); err != nil || len(symbols) != 1 || symbols[0].Name != "A" {
		t.Errorf("symbols after cancel = %s, want class A", again.Result)
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

// Exiting while a request is still in flight ends the session cleanly.
func TestExitWithARequestInFlightIsClean(t *testing.T) {
	const uri = "file:///D:/work/a.php"
	p := start(t)
	p.initialize(t)
	if err := p.client.Notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": uri, "languageId": "php", "version": 1, "text": "<?php\nclass A {}\n",
	}}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}
	if _, err := p.client.Send("textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": uri}}); err != nil {
		t.Fatalf("Send: %v", err)
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
