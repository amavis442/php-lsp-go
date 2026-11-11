package data

import (
	"sort"
	"sync"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/php"
)

func names(ix *MemoryIndex) []string {
	var out []string
	ix.Each(func(s php.WorkspaceSymbol) bool {
		out = append(out, s.Name)
		return true
	})
	sort.Strings(out)
	return out
}

func TestMemoryIndexReplacesAndRemovesPerFile(t *testing.T) {
	ix := NewMemoryIndex()
	ix.Replace("file:///a.php", []php.WorkspaceSymbol{{Name: "A1"}, {Name: "A2"}})
	ix.Replace("file:///b.php", []php.WorkspaceSymbol{{Name: "B"}})
	ix.Replace("file:///a.php", []php.WorkspaceSymbol{{Name: "A3"}})

	if got := names(ix); len(got) != 2 || got[0] != "A3" || got[1] != "B" {
		t.Errorf("after replace = %v, want [A3 B]", got)
	}

	ix.Remove("file:///a.php")
	if got := names(ix); len(got) != 1 || got[0] != "B" {
		t.Errorf("after remove = %v, want [B]", got)
	}
	if ix.Files() != 1 {
		t.Errorf("Files() = %d, want 1", ix.Files())
	}
}

func TestMemoryIndexEachStopsWhenAsked(t *testing.T) {
	ix := NewMemoryIndex()
	ix.Replace("file:///a.php", []php.WorkspaceSymbol{{Name: "1"}, {Name: "2"}, {Name: "3"}})

	visited := 0
	ix.Each(func(php.WorkspaceSymbol) bool {
		visited++
		return visited < 2
	})

	if visited != 2 {
		t.Errorf("visited %d symbols, want 2", visited)
	}
}

func TestMemoryIndexIsSafeForConcurrentUse(t *testing.T) {
	ix := NewMemoryIndex()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			ix.Replace(string(rune('a'+i)), []php.WorkspaceSymbol{{Name: "x"}})
		}()
		go func() {
			defer wg.Done()
			ix.Each(func(php.WorkspaceSymbol) bool { return true })
		}()
	}
	wg.Wait()

	if ix.Files() != 8 {
		t.Errorf("Files() = %d, want 8", ix.Files())
	}
}
