package app

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/amavis442/php-lsp-go/internal/core/fileuri"
	"github.com/amavis442/php-lsp-go/internal/core/syntax"
	idxdata "github.com/amavis442/php-lsp-go/internal/features/indexing/data"
	"github.com/amavis442/php-lsp-go/internal/server"
)

// indexSession starts a server with cache, waits until root is indexed and
// returns a search, and a function that ends the session.
func indexSession(t *testing.T, root string, cache *idxdata.FileCache) (func(string) []symbolInformation, func()) {
	t.Helper()
	parser := syntax.NewParser()
	c, done := startAppWith(t, newApp(server.Info{Name: "php-lsp-go", Version: "test"}, log.New(io.Discard, "", 0),
		time.Millisecond, parser, parser, noLibraries{}, cache),
		map[string]any{
			"processId": nil, "rootUri": fileuri.FromPath(root),
			"capabilities": map[string]any{"window": map[string]any{"workDoneProgress": true}},
		})
	waitIndexed(t, c)
	search := func(query string) []symbolInformation { return workspaceSymbols(t, c, query) }
	end := func() {
		t.Helper()
		if _, err := c.Call("shutdown", nil); err != nil {
			t.Fatalf("shutdown: %v", err)
		}
		notify(t, c, "exit", nil)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("server did not stop after exit")
		}
	}
	return search, end
}

func TestASecondSessionTakesUnchangedFilesFromTheCache(t *testing.T) {
	root := t.TempDir()
	same, changed := filepath.Join(root, "src", "Same.php"), filepath.Join(root, "src", "Changed.php")
	writePHP(t, same, "<?php\nclass Alpha {}\n")
	writePHP(t, changed, "<?php\nclass Beta {}\n")
	cache := idxdata.NewFileCache(t.TempDir(), "test-build")

	_, end := indexSession(t, root, cache)
	end()

	// Same.php gets new content of the same size and keeps its time, so its
	// stamp still matches: the cache wins, which shows it is used.
	info, err := os.Stat(same)
	if err != nil {
		t.Fatal(err)
	}
	writePHP(t, same, "<?php\nclass Gamma {}\n")
	if err := os.Chtimes(same, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	// Changed.php really changes.
	writePHP(t, changed, "<?php\nclass Delta {}\n")
	later := info.ModTime().Add(time.Minute)
	if err := os.Chtimes(changed, later, later); err != nil {
		t.Fatal(err)
	}

	search, end := indexSession(t, root, cache)
	defer end()

	for query, want := range map[string]int{"Alpha": 1, "Gamma": 0, "Delta": 1, "Beta": 0} {
		got := search(query)
		n := 0
		for _, s := range got {
			if s.Name == query {
				n++
			}
		}
		if n != want {
			t.Errorf("workspace/symbol(%s) found it %d times, want %d: %+v", query, n, want, got)
		}
	}
}
