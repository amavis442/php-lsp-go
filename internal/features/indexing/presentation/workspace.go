// Package presentation drives the workspace index from the LSP session: it
// scans the workspace once the editor is initialized, shows progress, and
// follows the file changes the editor reports.
package presentation

import (
	"context"
	"log"
	"sync"

	"go.lsp.dev/protocol"

	"github.com/amavis442/php-lsp-go/internal/core/fileuri"
	"github.com/amavis442/php-lsp-go/internal/features/indexing/domain"
	"github.com/amavis442/php-lsp-go/internal/server"
)

// Editor is the part of the LSP client the workspace talks to.
type Editor interface {
	Call(ctx context.Context, method string, params, result any) error
	Notify(ctx context.Context, method string, params any) error
}

// fileChangeType values from the LSP specification.
const fileDeleted = 3

type fileEvent struct {
	URI  string `json:"uri"`
	Type int    `json:"type"`
}

type didChangeWatchedFilesParams struct {
	Changes []fileEvent `json:"changes"`
}

// Workspace keeps the index in step with the files of the workspace.
type Workspace struct {
	indexer *domain.Indexer
	editor  Editor
	log     *log.Logger

	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	scanned chan struct{}

	mu      sync.Mutex
	pending []fileEvent
	wake    chan struct{}
}

// NewWorkspace returns a workspace that fills indexer's index.
func NewWorkspace(indexer *domain.Indexer, editor Editor, logger *log.Logger) *Workspace {
	ctx, cancel := context.WithCancel(context.Background())
	return &Workspace{
		indexer: indexer,
		editor:  editor,
		log:     logger,
		ctx:     ctx,
		cancel:  cancel,
		scanned: make(chan struct{}),
		wake:    make(chan struct{}, 1),
	}
}

// Register adds the watched-files handler to r.
func (w *Workspace) Register(r *server.Router) {
	server.HandleNotification(r, protocol.MethodWorkspaceDidChangeWatchedFiles, w.didChangeWatchedFiles)
}

// Scanned is closed when the first scan of the workspace has finished.
func (w *Workspace) Scanned() <-chan struct{} { return w.scanned }

// Start scans the workspace in the background and then follows file
// changes. It runs on the read loop (OnInitialized), so it only starts the
// work.
func (w *Workspace) Start(session server.Session) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.scan(session)
		close(w.scanned)
		w.follow()
	}()
}

// Stop cancels the work and waits for it. It is safe to call more than once,
// and without Start.
func (w *Workspace) Stop() {
	w.cancel()
	w.wg.Wait()
}

func (w *Workspace) scan(session server.Session) {
	var roots []string
	for _, uri := range session.Roots {
		path, err := fileuri.ToPath(uri)
		if err != nil {
			w.log.Printf("skipping workspace root %s: %v", uri, err)
			continue
		}
		roots = append(roots, path)
	}
	if len(roots) == 0 {
		return
	}

	// Watch before scanning: a change during the scan then waits in pending
	// and is applied after it, instead of being lost.
	if session.WatchedFilesRegistration {
		w.registerWatcher()
	}
	var progress domain.Progress = silent{}
	if session.WorkDoneProgress {
		if p, err := newWorkDone(w.ctx, w.editor); err == nil {
			progress = p
		}
	}

	stats, err := w.indexer.IndexWorkspace(w.ctx, roots, progress)
	if err != nil {
		if w.ctx.Err() == nil {
			w.log.Printf("indexing the workspace: %v", err)
		}
		return
	}
	if stats.Failed > 0 {
		w.log.Printf("indexed %d files, skipped %d that could not be read or parsed", stats.Files, stats.Failed)
	}
}

func (w *Workspace) registerWatcher() {
	params := map[string]any{"registrations": []any{map[string]any{
		"id":     "php-lsp-go/watched-files",
		"method": protocol.MethodWorkspaceDidChangeWatchedFiles,
		"registerOptions": map[string]any{
			"watchers": []any{map[string]any{"globPattern": "**/*.php"}},
		},
	}}}
	if err := w.editor.Call(w.ctx, "client/registerCapability", params, nil); err != nil && w.ctx.Err() == nil {
		w.log.Printf("registering file watchers: %v", err)
	}
}

// didChangeWatchedFiles runs on the read loop, so it only queues the
// changes for follow.
func (w *Workspace) didChangeWatchedFiles(_ context.Context, p *didChangeWatchedFilesParams) error {
	w.mu.Lock()
	w.pending = append(w.pending, p.Changes...)
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
	return nil
}

// follow applies queued file changes, in the order they arrived, until the
// workspace stops.
func (w *Workspace) follow() {
	for {
		w.mu.Lock()
		changes := w.pending
		w.pending = nil
		w.mu.Unlock()
		for _, c := range changes {
			w.apply(c)
		}
		select {
		case <-w.ctx.Done():
			return
		case <-w.wake:
		}
	}
}

func (w *Workspace) apply(c fileEvent) {
	path, err := fileuri.ToPath(c.URI)
	if err != nil {
		return
	}
	if c.Type == fileDeleted {
		w.indexer.RemoveFile(path)
		return
	}
	if err := w.indexer.IndexFile(path); err != nil {
		// A file deleted right after it changed cannot be read; keep the
		// index free of it.
		w.indexer.RemoveFile(path)
	}
}
