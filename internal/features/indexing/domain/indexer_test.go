package domain

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/fileuri"
	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
)

// fakeFiles is a file system of path -> content under the roots given.
type fakeFiles struct {
	files   map[string]string
	walkErr error
}

func (f fakeFiles) Walk(ctx context.Context, root string, visit func(string) error) error {
	if f.walkErr != nil {
		return f.walkErr
	}
	var paths []string
	for p := range f.files {
		if strings.HasPrefix(p, root) {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := visit(p); err != nil {
			return err
		}
	}
	return nil
}

func (f fakeFiles) Read(path string) ([]byte, error) {
	content, ok := f.files[path]
	if !ok || content == "unreadable" {
		return nil, fmt.Errorf("cannot read %s", path)
	}
	return []byte(content), nil
}

// functionPerFile declares one function named after the file content.
type functionPerFile struct{}

func (functionPerFile) Extract(src []byte) (php.FileSummary, error) {
	if string(src) == "broken" {
		return php.FileSummary{}, errors.New("parse failure")
	}
	return php.FileSummary{Declarations: []php.Declaration{{Name: string(src), Kind: php.KindFunction}}}, nil
}

type fakeIndex struct {
	mu    sync.Mutex
	files map[string][]php.WorkspaceSymbol
}

func newFakeIndex() *fakeIndex { return &fakeIndex{files: make(map[string][]php.WorkspaceSymbol)} }

func (f *fakeIndex) Replace(uri string, symbols []php.WorkspaceSymbol) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[uri] = symbols
}

func (f *fakeIndex) Remove(uri string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, uri)
}

// names lists "file name: symbol names" for every indexed file, sorted.
func (f *fakeIndex) names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for uri, symbols := range f.files {
		var names []string
		for _, s := range symbols {
			names = append(names, s.Name)
		}
		out = append(out, uri[strings.LastIndex(uri, "/")+1:]+": "+strings.Join(names, ","))
	}
	sort.Strings(out)
	return out
}

type recordedProgress struct {
	events []string
}

func (r *recordedProgress) Begin(total int) {
	r.events = append(r.events, fmt.Sprintf("begin %d", total))
}
func (r *recordedProgress) Report(done, total int) {
	r.events = append(r.events, fmt.Sprintf("%d/%d", done, total))
}
func (r *recordedProgress) End() { r.events = append(r.events, "end") }

var root = filepath.Join(string(filepath.Separator), "work")

func path(name string) string { return filepath.Join(root, name) }

func newTestIndexer(files fakeFiles, index Index) *Indexer {
	return NewIndexer(files, func() Extractor { return functionPerFile{} }, index,
		func() text.Encoding { return text.UTF16 }, 3)
}

func TestIndexWorkspaceIndexesEveryFileUnderItsCanonicalURI(t *testing.T) {
	files := fakeFiles{files: map[string]string{
		path("a.php"): "alpha", path("b.php"): "beta", path(filepath.Join("sub", "c.php")): "gamma",
	}}
	index := newFakeIndex()

	stats, err := newTestIndexer(files, index).IndexWorkspace(context.Background(), []string{root}, &recordedProgress{})

	if err != nil {
		t.Fatalf("IndexWorkspace: %v", err)
	}
	if got := strings.Join(index.names(), "; "); got != "a.php: alpha; b.php: beta; c.php: gamma" {
		t.Errorf("index = %s", got)
	}
	if stats != (Stats{Files: 3}) {
		t.Errorf("stats = %+v, want 3 files", stats)
	}
	if _, ok := index.files[fileuri.FromPath(path("a.php"))]; !ok {
		t.Errorf("a.php not indexed under %s", fileuri.FromPath(path("a.php")))
	}
}

func TestIndexWorkspaceReportsProgress(t *testing.T) {
	files := fakeFiles{files: map[string]string{path("a.php"): "a", path("b.php"): "b"}}
	progress := &recordedProgress{}

	if _, err := newTestIndexer(files, newFakeIndex()).IndexWorkspace(context.Background(), []string{root}, progress); err != nil {
		t.Fatalf("IndexWorkspace: %v", err)
	}

	if got := strings.Join(progress.events, " "); got != "begin 2 1/2 2/2 end" {
		t.Errorf("progress = %s, want begin, one report per file, end", got)
	}
}

func TestIndexWorkspaceSkipsFilesItCannotReadOrParse(t *testing.T) {
	files := fakeFiles{files: map[string]string{
		path("good.php"): "good", path("locked.php"): "unreadable", path("bad.php"): "broken",
	}}
	index := newFakeIndex()

	stats, err := newTestIndexer(files, index).IndexWorkspace(context.Background(), []string{root}, &recordedProgress{})

	if err != nil {
		t.Fatalf("IndexWorkspace: %v", err)
	}
	if got := strings.Join(index.names(), "; "); got != "good.php: good" {
		t.Errorf("index = %s", got)
	}
	if stats != (Stats{Files: 1, Failed: 2}) {
		t.Errorf("stats = %+v, want 1 indexed and 2 failed", stats)
	}
}

func TestIndexWorkspaceStopsWhenCancelled(t *testing.T) {
	files := fakeFiles{files: map[string]string{path("a.php"): "a"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	progress := &recordedProgress{}

	_, err := newTestIndexer(files, newFakeIndex()).IndexWorkspace(ctx, []string{root}, progress)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if n := len(progress.events); n > 0 && progress.events[n-1] != "end" {
		t.Errorf("progress = %v, want it ended when begun", progress.events)
	}
}

func TestIndexWorkspaceReportsWalkFailures(t *testing.T) {
	boom := errors.New("walk failed")

	_, err := newTestIndexer(fakeFiles{walkErr: boom}, newFakeIndex()).IndexWorkspace(context.Background(), []string{root}, &recordedProgress{})

	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestIndexFileAndRemoveFileKeepTheIndexCurrent(t *testing.T) {
	files := fakeFiles{files: map[string]string{path("a.php"): "first"}}
	index := newFakeIndex()
	indexer := newTestIndexer(files, index)

	if err := indexer.IndexFile(path("a.php")); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	files.files[path("a.php")] = "second"
	if err := indexer.IndexFile(path("a.php")); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	if got := strings.Join(index.names(), "; "); got != "a.php: second" {
		t.Errorf("after update index = %s", got)
	}

	indexer.RemoveFile(path("a.php"))
	if got := index.names(); len(got) != 0 {
		t.Errorf("after remove index = %v", got)
	}
}

func TestIndexFileReportsFailures(t *testing.T) {
	files := fakeFiles{files: map[string]string{path("bad.php"): "broken"}}

	if err := newTestIndexer(files, newFakeIndex()).IndexFile(path("bad.php")); err == nil {
		t.Error("IndexFile of a broken file succeeded, want error")
	}
}
