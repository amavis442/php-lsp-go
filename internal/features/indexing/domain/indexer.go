package domain

import (
	"context"
	"fmt"
	"sync"

	"github.com/amavis442/php-lsp-go/internal/core/fileuri"
	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
)

// Stamp identifies a version of a file on disk: its size and when it last
// changed (in nanoseconds since the Unix epoch).
type Stamp struct {
	Size    int64
	ModTime int64
}

// Files lists and reads the PHP files of the workspace.
type Files interface {
	// Walk calls visit with the path and stamp of every PHP file under
	// root that should be indexed.
	Walk(ctx context.Context, root string, visit func(path string, stamp Stamp) error) error
	Read(path string) ([]byte, error)
}

// Extractor finds the declarations in PHP source. An extractor is used by
// one goroutine at a time.
type Extractor interface {
	Extract(src []byte) (php.FileSummary, error)
}

// Index stores what each file declares, by canonical file URI.
type Index interface {
	Replace(uri string, symbols []php.WorkspaceSymbol)
	Remove(uri string)
}

// CachedFile is what a scan learned about one file.
type CachedFile struct {
	Stamp   Stamp
	Symbols []php.WorkspaceSymbol
}

// Cache keeps what a scan of roots learned, with positions in enc, for the
// next scan of the same roots. Load returns nothing when there is no
// usable copy.
type Cache interface {
	Load(roots []string, enc text.Encoding) map[string]CachedFile
	Save(roots []string, enc text.Encoding, files map[string]CachedFile) error
}

// NoCache is a Cache that keeps nothing.
type NoCache struct{}

func (NoCache) Load([]string, text.Encoding) map[string]CachedFile        { return nil }
func (NoCache) Save([]string, text.Encoding, map[string]CachedFile) error { return nil }

// Progress hears how far a workspace scan has come. All calls come from the
// goroutine that runs the scan.
type Progress interface {
	Begin(total int)
	Report(done, total int)
	End()
}

// Stats counts the files a scan indexed, how many of those came from the
// cache, and the files it had to skip.
type Stats struct {
	Files  int
	Cached int
	Failed int
}

// Indexer fills the index from the workspace and keeps it up to date.
type Indexer struct {
	files        Files
	newExtractor func() Extractor
	index        Index
	encoding     func() text.Encoding
	workers      int
	cache        Cache

	mu     sync.Mutex // guards single
	single Extractor  // for one-file updates
}

// NewIndexer returns an indexer that scans with workers goroutines, each
// with its own extractor from newExtractor. encoding reports the position
// encoding agreed with the editor. A scan starts from what cache kept of
// the previous one and leaves its result there.
func NewIndexer(files Files, newExtractor func() Extractor, index Index,
	encoding func() text.Encoding, workers int, cache Cache) *Indexer {
	return &Indexer{
		files:        files,
		newExtractor: newExtractor,
		index:        index,
		encoding:     encoding,
		workers:      max(workers, 1),
		cache:        cache,
		single:       newExtractor(),
	}
}

// file is a PHP file found by a scan.
type file struct {
	path  string
	stamp Stamp
}

// parsed is the outcome of indexing one file.
type parsed struct {
	file    file
	symbols []php.WorkspaceSymbol
	err     error
}

// IndexWorkspace indexes every PHP file under roots. Files whose stamp
// matches the cache are taken from it; the others are parsed. Files that
// cannot be read or parsed are skipped and counted. It stops early, with
// ctx's error, when ctx is cancelled; a complete scan is saved to the
// cache, and a failure to save is returned after the index is filled.
func (ix *Indexer) IndexWorkspace(ctx context.Context, roots []string, progress Progress) (Stats, error) {
	found, err := ix.list(ctx, roots)
	if err != nil {
		return Stats{}, err
	}
	enc := ix.encoding()
	cached := ix.cache.Load(roots, enc)

	progress.Begin(len(found))
	defer progress.End()

	var stats Stats
	learned := make(map[string]CachedFile, len(found))
	var todo []file
	for _, f := range found {
		if c, ok := cached[f.path]; ok && c.Stamp == f.stamp {
			ix.index.Replace(fileuri.FromPath(f.path), c.Symbols)
			learned[f.path] = c
			stats.Files++
			stats.Cached++
			progress.Report(stats.Files, len(found))
			continue
		}
		todo = append(todo, f)
	}

	for r := range ix.parseAll(ctx, todo, enc) {
		if r.err != nil {
			stats.Failed++
		} else {
			stats.Files++
			learned[r.file.path] = CachedFile{Stamp: r.file.stamp, Symbols: r.symbols}
		}
		progress.Report(stats.Files+stats.Failed, len(found))
	}
	if err := ctx.Err(); err != nil {
		return stats, err
	}
	if err := ix.cache.Save(roots, enc, learned); err != nil {
		return stats, fmt.Errorf("saving the index cache: %w", err)
	}
	return stats, nil
}

// parseAll indexes files with the worker pool and sends each outcome.
func (ix *Indexer) parseAll(ctx context.Context, files []file, enc text.Encoding) <-chan parsed {
	jobs := make(chan file)
	go func() {
		defer close(jobs)
		for _, f := range files {
			select {
			case jobs <- f:
			case <-ctx.Done():
				return
			}
		}
	}()

	results := make(chan parsed)
	var wg sync.WaitGroup
	for range min(ix.workers, max(len(files), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			extractor := ix.newExtractor()
			for f := range jobs {
				symbols, err := ix.indexWith(extractor, f.path, enc)
				results <- parsed{file: f, symbols: symbols, err: err}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	return results
}

// list collects the files under roots, each once even when roots nest.
func (ix *Indexer) list(ctx context.Context, roots []string) ([]file, error) {
	seen := make(map[string]bool)
	var found []file
	for _, root := range roots {
		err := ix.files.Walk(ctx, root, func(p string, stamp Stamp) error {
			if key := fileuri.Canonical(p); !seen[key] {
				seen[key] = true
				found = append(found, file{path: p, stamp: stamp})
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("listing %s: %w", root, err)
		}
	}
	return found, nil
}

// IndexFile indexes one file again, for example after it changed on disk.
func (ix *Indexer) IndexFile(path string) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	_, err := ix.indexWith(ix.single, path, ix.encoding())
	return err
}

// RemoveFile forgets a file, for example after it was deleted.
func (ix *Indexer) RemoveFile(path string) {
	ix.index.Remove(fileuri.FromPath(path))
}

// indexWith parses path, puts what it declares in the index and returns
// that.
func (ix *Indexer) indexWith(extractor Extractor, path string, enc text.Encoding) ([]php.WorkspaceSymbol, error) {
	src, err := ix.files.Read(path)
	if err != nil {
		return nil, err
	}
	summary, err := extractor.Extract(src)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	uri := fileuri.FromPath(path)
	symbols := php.Flatten(uri, string(src), summary.Declarations, enc)
	ix.index.Replace(uri, symbols)
	return symbols, nil
}
