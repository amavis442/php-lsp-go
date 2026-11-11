package data

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
	"github.com/amavis442/php-lsp-go/internal/features/indexing/domain"
)

func scan() map[string]domain.CachedFile {
	return map[string]domain.CachedFile{
		filepath.Join("work", "a.php"): {
			Stamp: domain.Stamp{Size: 10, ModTime: 1234567890},
			Symbols: []php.WorkspaceSymbol{{
				Name: "User", Kind: php.KindClass, Container: "App", URI: "file:///work/a.php",
				Range:   text.Range{Start: text.Position{Line: 1}, End: text.Position{Line: 9, Character: 1}},
				Extends: []string{`App\Model`},
			}},
		},
	}
}

func TestACacheReturnsWhatWasSavedForTheSameRootsAndEncoding(t *testing.T) {
	dir := t.TempDir()
	if err := NewFileCache(dir, "build-1").Save([]string{"b", "a"}, text.UTF16, scan()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Another server with the same build; the roots in another order.
	got := NewFileCache(dir, "build-1").Load([]string{"a", "b"}, text.UTF16)

	a, ok := got[filepath.Join("work", "a.php")]
	if len(got) != 1 || !ok || a.Stamp.ModTime != 1234567890 || len(a.Symbols) != 1 ||
		a.Symbols[0].Name != "User" || a.Symbols[0].Range.End.Line != 9 || a.Symbols[0].Extends[0] != `App\Model` {
		t.Errorf("Load = %+v", got)
	}
}

func TestACacheIsNotUsedForOtherRootsEncodingsOrBuilds(t *testing.T) {
	dir := t.TempDir()
	if err := NewFileCache(dir, "build-1").Save([]string{"a"}, text.UTF16, scan()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	tests := []struct {
		name  string
		build string
		roots []string
		enc   text.Encoding
	}{
		{"other roots", "build-1", []string{"a", "b"}, text.UTF16},
		{"other encoding", "build-1", []string{"a"}, text.UTF8},
		{"other build", "build-2", []string{"a"}, text.UTF16},
	}
	for _, tt := range tests {
		if got := NewFileCache(dir, tt.build).Load(tt.roots, tt.enc); got != nil {
			t.Errorf("%s: Load = %+v, want nothing", tt.name, got)
		}
	}
}

func TestACorruptCacheIsIgnored(t *testing.T) {
	dir := t.TempDir()
	c := NewFileCache(dir, "build-1")
	if err := c.Save([]string{"a"}, text.UTF16, scan()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("cache dir holds %v, want one file", entries)
	}
	if err := os.WriteFile(filepath.Join(dir, entries[0].Name()), []byte("not gob"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := c.Load([]string{"a"}, text.UTF16); got != nil {
		t.Errorf("Load = %+v, want nothing", got)
	}
}

func TestSavingReplacesTheCacheAndLeavesNoTemporaryFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "index") // created on demand
	c := NewFileCache(dir, "build-1")
	if err := c.Save([]string{"a"}, text.UTF16, scan()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := c.Save([]string{"a"}, text.UTF16, map[string]domain.CachedFile{}); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	if got := c.Load([]string{"a"}, text.UTF16); got == nil || len(got) != 0 {
		t.Errorf("Load = %+v, want the second, empty scan", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("cache dir holds %v, want one file", entries)
	}
}

func TestNoCacheFileMeansNothingToLoad(t *testing.T) {
	if got := NewFileCache(t.TempDir(), "build-1").Load([]string{"a"}, text.UTF16); got != nil {
		t.Errorf("Load = %+v, want nothing", got)
	}
}
