package e2e

import (
	"encoding/json"
	"testing"
)

type publishedDiagnostics struct {
	URI         string `json:"uri"`
	Diagnostics []struct {
		Message string `json:"message"`
		Range   struct {
			Start struct{ Line, Character int } `json:"start"`
		} `json:"range"`
	} `json:"diagnostics"`
}

func (p *process) waitDiagnostics(t *testing.T) publishedDiagnostics {
	t.Helper()
	msg, err := p.client.WaitNotification("textDocument/publishDiagnostics")
	if err != nil {
		t.Fatalf("waiting for diagnostics: %v\nstderr:\n%s", err, p.stderr)
	}
	var d publishedDiagnostics
	if err := json.Unmarshal(msg.Params, &d); err != nil {
		t.Fatalf("decoding %s: %v", msg.Params, err)
	}
	return d
}

func TestSyntaxErrorsAreReportedAndClearedAfterAFix(t *testing.T) {
	const uri = "file:///D:/work/broken.php"
	p := start(t)
	p.initialize(t)

	if err := p.client.Notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": uri, "languageId": "php", "version": 1, "text": "<?php\n$x = ;\n",
	}}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}
	broken := p.waitDiagnostics(t)
	if broken.URI != uri || len(broken.Diagnostics) != 1 ||
		broken.Diagnostics[0].Message != "Syntax error: unexpected '='" ||
		broken.Diagnostics[0].Range.Start.Line != 1 || broken.Diagnostics[0].Range.Start.Character != 3 {
		t.Fatalf("diagnostics = %+v, want unexpected '=' at 1:3 for %s", broken, uri)
	}

	if err := p.client.Notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": 2},
		"contentChanges": []any{map[string]any{"text": "<?php\n$x = 1;\n"}},
	}); err != nil {
		t.Fatalf("didChange: %v", err)
	}
	if fixed := p.waitDiagnostics(t); len(fixed.Diagnostics) != 0 {
		t.Errorf("diagnostics after fix = %+v, want none", fixed)
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
