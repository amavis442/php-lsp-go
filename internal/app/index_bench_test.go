package app

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/syntax"
	"github.com/amavis442/php-lsp-go/internal/core/text"
	idxdata "github.com/amavis442/php-lsp-go/internal/features/indexing/data"
	idxdomain "github.com/amavis442/php-lsp-go/internal/features/indexing/domain"
	phpstormstubs "github.com/amavis442/php-lsp-go/third_party/phpstorm-stubs"
)

type noProgress struct{}

func (noProgress) Begin(int)       {}
func (noProgress) Report(int, int) {}
func (noProgress) End()            {}

// stubCorpus writes the embedded stubs to a directory: 139 real PHP files,
// 3.6 MB, the same on every machine.
func stubCorpus(b *testing.B) string {
	b.Helper()
	dir := b.TempDir()
	sub, err := fs.Sub(phpstormstubs.Files, ".")
	if err != nil {
		b.Fatal(err)
	}
	if err := os.CopyFS(dir, sub); err != nil {
		b.Fatal(err)
	}
	return dir
}

func scanOnce(b *testing.B, cache idxdomain.Cache, root string) {
	b.Helper()
	indexer := idxdomain.NewIndexer(idxdata.NewFileSystem(idxdata.DefaultExclude, idxdata.DefaultMaxSize),
		func() idxdomain.Extractor { return syntax.NewParser() },
		idxdata.NewMemoryIndex(), func() text.Encoding { return text.UTF16 }, runtime.NumCPU(), cache)
	if _, err := indexer.IndexWorkspace(context.Background(), []string{root}, noProgress{}); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkScanCold parses every file, as the first start does.
func BenchmarkScanCold(b *testing.B) {
	root := stubCorpus(b)
	for b.Loop() {
		scanOnce(b, noCache, root)
	}
}

// BenchmarkScanWarm takes every file from the cache, as a later start
// without changes does: walking, stamping, loading and saving the scan.
func BenchmarkScanWarm(b *testing.B) {
	root := stubCorpus(b)
	cache := idxdata.NewFileCache(filepath.Join(b.TempDir(), "index"), "bench")
	scanOnce(b, cache, root) // fills the cache before the timed rounds
	for b.Loop() {
		scanOnce(b, cache, root)
	}
}
