package presentation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amavis442/php-lsp-go/internal/core/fileuri"
	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/syntax"
	"github.com/amavis442/php-lsp-go/internal/core/text"
	"github.com/amavis442/php-lsp-go/internal/features/indexing/data"
	"github.com/amavis442/php-lsp-go/internal/features/indexing/domain"
	"github.com/amavis442/php-lsp-go/internal/server"
)

// fakeEditor records what the workspace sends to the editor.
type fakeEditor struct {
	mu       sync.Mutex
	calls    []string
	progress []string
	callErr  error
}

func (e *fakeEditor) Call(_ context.Context, method string, params, _ any) error {
	raw, _ := json.Marshal(params)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, method+" "+string(raw))
	return e.callErr
}

func (e *fakeEditor) Notify(_ context.Context, method string, params any) error {
	raw, _ := json.Marshal(params)
	var p struct {
		Value struct {
			Kind string `json:"kind"`
		} `json:"value"`
	}
	_ = json.Unmarshal(raw, &p)
	e.mu.Lock()
	defer e.mu.Unlock()
	if method == "$/progress" {
		e.progress = append(e.progress, p.Value.Kind)
	}
	return nil
}

func (e *fakeEditor) snapshot() (calls, progress []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.calls...), append([]string(nil), e.progress...)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeLibraries hands out fixed library roots.
type fakeLibraries struct {
	roots []string
	err   error
}

func (l *fakeLibraries) Roots() ([]string, error) { return l.roots, l.err }

type fixture struct {
	root      string
	index     *data.MemoryIndex
	editor    *fakeEditor
	libraries *fakeLibraries
	logs      *strings.Builder
	workspace *Workspace
	router    *server.Router
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	libraries := &fakeLibraries{}
	index := data.NewMemoryIndex()
	indexer := domain.NewIndexer(data.NewFileSystem(data.DefaultExclude, data.DefaultMaxSize),
		func() domain.Extractor { return syntax.NewParser() }, index,
		func() text.Encoding { return text.UTF8 }, 2)
	editor := &fakeEditor{}
	logs := &strings.Builder{}
	w := NewWorkspace(indexer, libraries, editor, log.New(logs, "", 0))
	r := server.NewRouter(log.New(io.Discard, "", 0))
	w.Register(r)
	t.Cleanup(w.Stop)
	return &fixture{root: root, index: index, editor: editor, libraries: libraries, logs: logs, workspace: w, router: r}
}

func (f *fixture) names() string {
	var out []string
	f.index.Each(func(s php.WorkspaceSymbol) bool {
		out = append(out, s.Name)
		return true
	})
	sort.Strings(out)
	return strings.Join(out, " ")
}

func waitScanned(t *testing.T, w *Workspace) {
	t.Helper()
	select {
	case <-w.Scanned():
	case <-time.After(5 * time.Second):
		t.Fatal("workspace scan did not finish")
	}
}

// eventually waits until cond holds, for work done in the background.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStartIndexesTheWorkspaceWithProgressAndWatchers(t *testing.T) {
	f := newFixture(t)
	writeFile(t, filepath.Join(f.root, "src", "User.php"), "<?php\nclass User { function name() {} }\n")
	writeFile(t, filepath.Join(f.root, "vendor", "lib", "Lib.php"), "<?php\nfunction lib_helper() {}\n")

	f.workspace.Start(server.Session{
		Roots: []string{fileuri.FromPath(f.root)}, WorkDoneProgress: true, WatchedFilesRegistration: true,
	})
	waitScanned(t, f.workspace)

	if got := f.names(); got != "User lib_helper name" {
		t.Errorf("indexed %q", got)
	}
	calls, progress := f.editor.snapshot()
	if len(calls) != 2 || !strings.HasPrefix(calls[0], "client/registerCapability ") ||
		!strings.Contains(calls[0], `"globPattern":"**/*.php"`) ||
		!strings.HasPrefix(calls[1], "window/workDoneProgress/create ") {
		t.Errorf("calls = %v, want a watcher registration, then a progress token", calls)
	}
	if len(progress) < 2 || progress[0] != "begin" || progress[len(progress)-1] != "end" {
		t.Errorf("progress = %v, want begin ... end", progress)
	}
}

func TestStartWithoutProgressOrWatcherSupportOnlyIndexes(t *testing.T) {
	f := newFixture(t)
	writeFile(t, filepath.Join(f.root, "a.php"), "<?php\nclass A {}\n")

	f.workspace.Start(server.Session{Roots: []string{fileuri.FromPath(f.root)}})
	waitScanned(t, f.workspace)

	if got := f.names(); got != "A" {
		t.Errorf("indexed %q", got)
	}
	if calls, progress := f.editor.snapshot(); len(calls) != 0 || len(progress) != 0 {
		t.Errorf("calls = %v, progress = %v; want none", calls, progress)
	}
}

func TestAFailingProgressTokenStillIndexes(t *testing.T) {
	f := newFixture(t)
	f.editor.callErr = context.DeadlineExceeded
	writeFile(t, filepath.Join(f.root, "a.php"), "<?php\nclass A {}\n")

	f.workspace.Start(server.Session{Roots: []string{fileuri.FromPath(f.root)}, WorkDoneProgress: true})
	waitScanned(t, f.workspace)

	if got := f.names(); got != "A" {
		t.Errorf("indexed %q", got)
	}
	if _, progress := f.editor.snapshot(); len(progress) != 0 {
		t.Errorf("progress = %v, want none without a token", progress)
	}
}

func TestStartWithoutRootsIndexesNothing(t *testing.T) {
	f := newFixture(t)

	f.workspace.Start(server.Session{})
	waitScanned(t, f.workspace)

	if f.index.Files() != 0 {
		t.Errorf("indexed %d files, want none", f.index.Files())
	}
}

func TestLibrariesAreIndexedWithTheWorkspace(t *testing.T) {
	f := newFixture(t)
	lib := t.TempDir()
	f.libraries.roots = []string{lib}
	writeFile(t, filepath.Join(f.root, "a.php"), "<?php\nclass A {}\n")
	writeFile(t, filepath.Join(lib, "Core", "Core.php"), "<?php\nfunction strlen() {}\n")

	f.workspace.Start(server.Session{Roots: []string{fileuri.FromPath(f.root)}})
	waitScanned(t, f.workspace)

	if got := f.names(); got != "A strlen" {
		t.Errorf("indexed %q, want the workspace and the library", got)
	}
}

func TestLibrariesAreIndexedWithoutAWorkspace(t *testing.T) {
	f := newFixture(t)
	lib := t.TempDir()
	f.libraries.roots = []string{lib}
	writeFile(t, filepath.Join(lib, "Core", "Core.php"), "<?php\nfunction strlen() {}\n")

	f.workspace.Start(server.Session{WatchedFilesRegistration: true})
	waitScanned(t, f.workspace)

	if got := f.names(); got != "strlen" {
		t.Errorf("indexed %q, want the library", got)
	}
	if calls, _ := f.editor.snapshot(); len(calls) != 0 {
		t.Errorf("calls = %v; want no watcher without a workspace to watch", calls)
	}
}

func TestLibrariesThatFailAreLoggedAndTheWorkspaceIsStillIndexed(t *testing.T) {
	f := newFixture(t)
	f.libraries.err = errors.New("disk full")
	writeFile(t, filepath.Join(f.root, "a.php"), "<?php\nclass A {}\n")

	f.workspace.Start(server.Session{Roots: []string{fileuri.FromPath(f.root)}})
	waitScanned(t, f.workspace)

	if got := f.names(); got != "A" {
		t.Errorf("indexed %q, want the workspace", got)
	}
	if !strings.Contains(f.logs.String(), "disk full") {
		t.Errorf("log = %q, want the library error", f.logs.String())
	}
}

func watched(t *testing.T, f *fixture, changes ...map[string]any) {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"changes": changes})
	if _, err := f.router.Handle(context.Background(), "workspace/didChangeWatchedFiles", params, false); err != nil {
		t.Fatalf("didChangeWatchedFiles: %v", err)
	}
}

func TestWatchedFileChangesUpdateTheIndex(t *testing.T) {
	f := newFixture(t)
	a, b := filepath.Join(f.root, "a.php"), filepath.Join(f.root, "b.php")
	writeFile(t, a, "<?php\nclass A {}\n")
	f.workspace.Start(server.Session{Roots: []string{fileuri.FromPath(f.root)}})
	waitScanned(t, f.workspace)

	writeFile(t, a, "<?php\nclass Renamed {}\n")
	writeFile(t, b, "<?php\nclass B {}\n")
	watched(t, f,
		map[string]any{"uri": fileuri.FromPath(a), "type": 2},
		map[string]any{"uri": fileuri.FromPath(b), "type": 1},
	)
	eventually(t, "changed and created files", func() bool { return f.names() == "B Renamed" })

	watched(t, f, map[string]any{"uri": fileuri.FromPath(a), "type": 3})
	eventually(t, "deleted file", func() bool { return f.names() == "B" })
}

func TestChangesBeforeTheScanAreAppliedAfterIt(t *testing.T) {
	f := newFixture(t)
	a := filepath.Join(f.root, "a.php")
	writeFile(t, a, "<?php\nclass A {}\n")

	watched(t, f, map[string]any{"uri": fileuri.FromPath(a), "type": 3})
	f.workspace.Start(server.Session{Roots: []string{fileuri.FromPath(f.root)}})
	waitScanned(t, f.workspace)

	eventually(t, "the early delete", func() bool { return f.index.Files() == 0 })
}

func TestStopEndsTheWorkspaceAndIsSafeToRepeat(t *testing.T) {
	f := newFixture(t)
	f.workspace.Start(server.Session{Roots: []string{fileuri.FromPath(f.root)}})
	waitScanned(t, f.workspace)

	done := make(chan struct{})
	go func() {
		f.workspace.Stop()
		f.workspace.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return")
	}
}

func TestStopWithoutStartReturns(t *testing.T) {
	f := newFixture(t)

	f.workspace.Stop()
}
