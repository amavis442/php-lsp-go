package data

import (
	"sync"

	"github.com/amavis442/php-lsp-go/internal/core/php"
)

// MemoryIndex keeps the workspace symbols of every indexed file in memory.
// It is safe for concurrent use: many readers, one writer at a time.
type MemoryIndex struct {
	mu    sync.RWMutex
	files map[string][]php.WorkspaceSymbol
}

// NewMemoryIndex returns an empty index.
func NewMemoryIndex() *MemoryIndex {
	return &MemoryIndex{files: make(map[string][]php.WorkspaceSymbol)}
}

// Replace sets the symbols of the file uri, dropping what it held before.
func (ix *MemoryIndex) Replace(uri string, symbols []php.WorkspaceSymbol) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.files[uri] = symbols
}

// Remove forgets the file uri.
func (ix *MemoryIndex) Remove(uri string) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	delete(ix.files, uri)
}

// Each calls visit for every symbol until visit returns false. Writers wait
// until it returns, so visit must be quick.
func (ix *MemoryIndex) Each(visit func(php.WorkspaceSymbol) bool) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	for _, symbols := range ix.files {
		for _, s := range symbols {
			if !visit(s) {
				return
			}
		}
	}
}

// Files returns how many files the index holds.
func (ix *MemoryIndex) Files() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.files)
}
