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

func uris(symbols []php.WorkspaceSymbol) []string {
	var out []string
	for _, s := range symbols {
		out = append(out, s.URI)
	}
	sort.Strings(out)
	return out
}

func TestLookupFindsSymbolsByKeyAcrossFiles(t *testing.T) {
	ix := NewMemoryIndex()
	user := php.WorkspaceSymbol{Name: "User", Kind: php.KindClass, Container: "App", URI: "file:///a.php"}
	save := php.WorkspaceSymbol{Name: "save", Kind: php.KindMethod, Container: `App\User`, Member: true, URI: "file:///a.php"}
	ix.Replace("file:///a.php", []php.WorkspaceSymbol{user, save})
	ix.Replace("file:///b.php", []php.WorkspaceSymbol{{Name: "user", Kind: php.KindClass, Container: "app", URI: "file:///b.php"}})

	if got := uris(ix.Lookup(php.ClassKey(`APP\USER`))); len(got) != 2 || got[0] != "file:///a.php" || got[1] != "file:///b.php" {
		t.Errorf("Lookup(class App User) found %v, want both files", got)
	}
	if got := ix.Lookup(php.MemberKey(`App\User`, php.KindMethod, "Save")); len(got) != 1 || got[0].Name != "save" {
		t.Errorf("Lookup(method save) = %+v", got)
	}
	if got := ix.Lookup(php.FunctionKey(`App\User`)); len(got) != 0 {
		t.Errorf("Lookup(function App User) = %+v, want none", got)
	}
}

func TestLookupForgetsReplacedAndRemovedFiles(t *testing.T) {
	ix := NewMemoryIndex()
	ix.Replace("file:///a.php", []php.WorkspaceSymbol{{Name: "Old", Kind: php.KindClass, URI: "file:///a.php"}})
	ix.Replace("file:///b.php", []php.WorkspaceSymbol{{Name: "Gone", Kind: php.KindClass, URI: "file:///b.php"}})

	ix.Replace("file:///a.php", []php.WorkspaceSymbol{{Name: "New", Kind: php.KindClass, URI: "file:///a.php"}})
	ix.Remove("file:///b.php")

	for _, name := range []string{"Old", "Gone"} {
		if got := ix.Lookup(php.ClassKey(name)); len(got) != 0 {
			t.Errorf("Lookup(%s) = %+v, want it forgotten", name, got)
		}
	}
	if got := ix.Lookup(php.ClassKey("New")); len(got) != 1 {
		t.Errorf("Lookup(New) = %+v", got)
	}
}
