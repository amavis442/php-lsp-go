package domain

import (
	"context"
	"fmt"
	"sync"

	"github.com/amavis442/php-lsp-go/internal/core/fileuri"
	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
)

// Files lists and reads the PHP files of the workspace.
type Files interface {
	// Walk calls visit with the path of every PHP file under root that
	// should be indexed.
	Walk(ctx context.Context, root string, visit func(path string) error) error
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

// Progress hears how far a workspace scan has come. All calls come from the
// goroutine that runs the scan.
type Progress interface {
	Begin(total int)
	Report(done, total int)
	End()
}

// Stats counts the files a scan indexed and the files it had to skip.
type Stats struct {
	Files  int
	Failed int
}

// Indexer fills the index from the workspace and keeps it up to date.
type Indexer struct {
	files        Files
	newExtractor func() Extractor
	index        Index
	encoding     func() text.Encoding
	workers      int

	mu     sync.Mutex // guards single
	single Extractor  // for one-file updates
}

// NewIndexer returns an indexer that scans with workers goroutines, each
// with its own extractor from newExtractor. encoding reports the position
// encoding agreed with the editor.
func NewIndexer(files Files, newExtractor func() Extractor, index Index,
	encoding func() text.Encoding, workers int) *Indexer {
	return &Indexer{
		files:        files,
		newExtractor: newExtractor,
		index:        index,
		encoding:     encoding,
		workers:      max(workers, 1),
		single:       newExtractor(),
	}
}

// IndexWorkspace indexes every PHP file under roots. Files that cannot be
// read or parsed are skipped and counted. It stops early, with ctx's error,
// when ctx is cancelled.
func (ix *Indexer) IndexWorkspace(ctx context.Context, roots []string, progress Progress) (Stats, error) {
	paths, err := ix.list(ctx, roots)
	if err != nil {
		return Stats{}, err
	}

	progress.Begin(len(paths))
	defer progress.End()

	jobs := make(chan string)
	go func() {
		defer close(jobs)
		for _, p := range paths {
			select {
			case jobs <- p:
			case <-ctx.Done():
				return
			}
		}
	}()

	results := make(chan error)
	var wg sync.WaitGroup
	for range min(ix.workers, max(len(paths), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			extractor := ix.newExtractor()
			for p := range jobs {
				results <- ix.indexWith(extractor, p)
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	var stats Stats
	for err := range results {
		if err != nil {
			stats.Failed++
		} else {
			stats.Files++
		}
		progress.Report(stats.Files+stats.Failed, len(paths))
	}
	return stats, ctx.Err()
}

// list collects the files under roots, each once even when roots nest.
func (ix *Indexer) list(ctx context.Context, roots []string) ([]string, error) {
	seen := make(map[string]bool)
	var paths []string
	for _, root := range roots {
		err := ix.files.Walk(ctx, root, func(p string) error {
			if key := fileuri.Canonical(p); !seen[key] {
				seen[key] = true
				paths = append(paths, p)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("listing %s: %w", root, err)
		}
	}
	return paths, nil
}

// IndexFile indexes one file again, for example after it changed on disk.
func (ix *Indexer) IndexFile(path string) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	return ix.indexWith(ix.single, path)
}

// RemoveFile forgets a file, for example after it was deleted.
func (ix *Indexer) RemoveFile(path string) {
	ix.index.Remove(fileuri.FromPath(path))
}

func (ix *Indexer) indexWith(extractor Extractor, path string) error {
	src, err := ix.files.Read(path)
	if err != nil {
		return err
	}
	summary, err := extractor.Extract(src)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	uri := fileuri.FromPath(path)
	ix.index.Replace(uri, php.Flatten(uri, string(src), summary.Declarations, ix.encoding()))
	return nil
}
