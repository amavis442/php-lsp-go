package presentation

import (
	"context"
	"fmt"
)

const progressToken = "php-lsp-go/indexing"

// workDone reports a scan as LSP work-done progress. It sends a report only
// when the percentage changes, so a large workspace sends at most a hundred.
type workDone struct {
	ctx     context.Context
	editor  Editor
	percent int
}

// newWorkDone asks the editor for a progress token. Without one the scan
// runs silently.
func newWorkDone(ctx context.Context, editor Editor) (*workDone, error) {
	if err := editor.Call(ctx, "window/workDoneProgress/create", map[string]any{"token": progressToken}, nil); err != nil {
		return nil, fmt.Errorf("creating progress: %w", err)
	}
	return &workDone{ctx: ctx, editor: editor, percent: -1}, nil
}

func (p *workDone) Begin(total int) {
	p.send(map[string]any{
		"kind": "begin", "title": "Indexing PHP files", "percentage": 0,
		"message": fmt.Sprintf("0/%d files", total),
	})
}

func (p *workDone) Report(done, total int) {
	percent := 100
	if total > 0 {
		percent = done * 100 / total
	}
	if percent == p.percent {
		return
	}
	p.percent = percent
	p.send(map[string]any{
		"kind": "report", "percentage": percent, "message": fmt.Sprintf("%d/%d files", done, total),
	})
}

func (p *workDone) End() {
	p.send(map[string]any{"kind": "end"})
}

// send drops errors: progress is a courtesy, and a closed connection ends
// the scan anyway.
func (p *workDone) send(value map[string]any) {
	_ = p.editor.Notify(p.ctx, "$/progress", map[string]any{"token": progressToken, "value": value})
}

// silent is the progress of a scan the editor does not show.
type silent struct{}

func (silent) Begin(int)       {}
func (silent) Report(int, int) {}
func (silent) End()            {}
