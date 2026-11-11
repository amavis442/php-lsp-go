package domain

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/fileuri"
	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
)

// fakeFiles is a file system of path -> content under the roots given. A
// file's stamp is its length, unless stamps says otherwise.
type fakeFiles struct {
	files   map[string]string
	stamps  map[string]Stamp
	walkErr error
}

func (f fakeFiles) stamp(p string) Stamp {
	if s, ok := f.stamps[p]; ok {
		return s
	}
	return Stamp{Size: int64(len(f.files[p]))}
}

func (f fakeFiles) Walk(ctx context.Context, root string, visit func(string, Stamp) error) error {
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
		if err := visit(p, f.stamp(p)); err != nil {
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

// functionPerFile declares one function named after the file content, and
// counts the files it parsed.
type functionPerFile struct {
	parsed *atomic.Int32
}

func (f functionPerFile) Extract(src []byte) (php.FileSummary, error) {
	if f.parsed != nil {
		f.parsed.Add(1)
	}
	if string(src) == "broken" {
		return php.FileSummary{}, errors.New("parse failure")
	}
	return php.FileSummary{Declarations: []php.Declaration{{Name: string(src), Kind: php.KindFunction}}}, nil
}

// fakeCache holds one saved scan in memory.
type fakeCache struct {
	loaded  map[string]CachedFile
	saved   map[string]CachedFile
	roots   []string
	enc     text.Encoding
	saves   int
	saveErr error
}

func (c *fakeCache) Load(roots []string, enc text.Encoding) map[string]CachedFile {
	c.roots, c.enc = roots, enc
	return c.loaded
}

func (c *fakeCache) Save(roots []string, enc text.Encoding, files map[string]CachedFile) error {
	c.saves++
	c.saved = files
	return c.saveErr
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
		func() text.Encoding { return text.UTF16 }, 3, NoCache{})
}

func TestAScanReusesCachedFilesThatDidNotChange(t *testing.T) {
	files := fakeFiles{files: map[string]string{
		path("same.php"): "same", path("changed.php"): "changed", path("new.php"): "new",
	}}
	cached := func(name string, stamp Stamp) CachedFile {
		return CachedFile{Stamp: stamp, Symbols: []php.WorkspaceSymbol{{Name: name, Kind: php.KindFunction}}}
	}
	cache := &fakeCache{loaded: map[string]CachedFile{
		path("same.php"):    cached("fromCache", files.stamp(path("same.php"))),
		path("changed.php"): cached("stale", Stamp{Size: 1}),
		path("deleted.php"): cached("gone", Stamp{Size: 4}),
	}}
	var parsed atomic.Int32
	index := newFakeIndex()
	indexer := NewIndexer(files, func() Extractor { return functionPerFile{parsed: &parsed} }, index,
		func() text.Encoding { return text.UTF8 }, 2, cache)

	stats, err := indexer.IndexWorkspace(context.Background(), []string{root}, &recordedProgress{})

	if err != nil {
		t.Fatalf("IndexWorkspace: %v", err)
	}
	if got := strings.Join(index.names(), "; "); got != "changed.php: changed; new.php: new; same.php: fromCache" {
		t.Errorf("index = %s", got)
	}
	if parsed.Load() != 2 || stats != (Stats{Files: 3, Cached: 1}) {
		t.Errorf("parsed %d, stats %+v; want 2 parsed and 1 from the cache", parsed.Load(), stats)
	}
	if len(cache.roots) != 1 || cache.roots[0] != root || cache.enc != text.UTF8 {
		t.Errorf("loaded for roots %v in %v", cache.roots, cache.enc)
	}
}

func TestAScanSavesWhatItIndexed(t *testing.T) {
	files := fakeFiles{files: map[string]string{path("a.php"): "alpha", path("bad.php"): "broken"}}
	cache := &fakeCache{}
	indexer := NewIndexer(files, func() Extractor { return functionPerFile{} }, newFakeIndex(),
		func() text.Encoding { return text.UTF16 }, 2, cache)

	if _, err := indexer.IndexWorkspace(context.Background(), []string{root}, &recordedProgress{}); err != nil {
		t.Fatalf("IndexWorkspace: %v", err)
	}

	a, ok := cache.saved[path("a.php")]
	if cache.saves != 1 || len(cache.saved) != 1 || !ok || a.Stamp != files.stamp(path("a.php")) ||
		len(a.Symbols) != 1 || a.Symbols[0].Name != "alpha" || a.Symbols[0].URI != fileuri.FromPath(path("a.php")) {
		t.Errorf("saved %d times: %+v; want a.php with its stamp and symbols, and not the broken file", cache.saves, cache.saved)
	}
}

func TestACancelledScanSavesNothing(t *testing.T) {
	files := fakeFiles{files: map[string]string{path("a.php"): "a"}}
	cache := &fakeCache{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	indexer := NewIndexer(files, func() Extractor { return functionPerFile{} }, newFakeIndex(),
		func() text.Encoding { return text.UTF16 }, 2, cache)

	if _, err := indexer.IndexWorkspace(ctx, []string{root}, &recordedProgress{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if cache.saves != 0 {
		t.Errorf("saved %d times, want none: a partial scan would hide files next time", cache.saves)
	}
}

func TestAFailedSaveIsReportedAfterIndexing(t *testing.T) {
	files := fakeFiles{files: map[string]string{path("a.php"): "a"}}
	boom := errors.New("disk full")
	index := newFakeIndex()
	indexer := NewIndexer(files, func() Extractor { return functionPerFile{} }, index,
		func() text.Encoding { return text.UTF16 }, 2, &fakeCache{saveErr: boom})

	stats, err := indexer.IndexWorkspace(context.Background(), []string{root}, &recordedProgress{})

	if !errors.Is(err, boom) || stats.Files != 1 || len(index.names()) != 1 {
		t.Errorf("stats %+v, err %v, index %v; want the file indexed and the save error", stats, err, index.names())
	}
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
