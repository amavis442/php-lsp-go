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
	// byKey finds symbols by php.KeyOf, per file so a file's entries can
	// be dropped when it changes.
	byKey map[string]map[string][]php.WorkspaceSymbol
}

// NewMemoryIndex returns an empty index.
func NewMemoryIndex() *MemoryIndex {
	return &MemoryIndex{
		files: make(map[string][]php.WorkspaceSymbol),
		byKey: make(map[string]map[string][]php.WorkspaceSymbol),
	}
}

// Replace sets the symbols of the file uri, dropping what it held before.
func (ix *MemoryIndex) Replace(uri string, symbols []php.WorkspaceSymbol) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.forget(uri)
	ix.files[uri] = symbols
	for _, s := range symbols {
		key := php.KeyOf(s)
		perFile := ix.byKey[key]
		if perFile == nil {
			perFile = make(map[string][]php.WorkspaceSymbol)
			ix.byKey[key] = perFile
		}
		perFile[uri] = append(perFile[uri], s)
	}
}

// Remove forgets the file uri.
func (ix *MemoryIndex) Remove(uri string) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.forget(uri)
}

// forget drops uri; the caller holds the write lock.
func (ix *MemoryIndex) forget(uri string) {
	for _, s := range ix.files[uri] {
		key := php.KeyOf(s)
		delete(ix.byKey[key], uri)
		if len(ix.byKey[key]) == 0 {
			delete(ix.byKey, key)
		}
	}
	delete(ix.files, uri)
}

// Lookup returns the symbols found under key (see php.KeyOf), from every
// file that declares them.
func (ix *MemoryIndex) Lookup(key string) []php.WorkspaceSymbol {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []php.WorkspaceSymbol
	for _, symbols := range ix.byKey[key] {
		out = append(out, symbols...)
	}
	return out
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
