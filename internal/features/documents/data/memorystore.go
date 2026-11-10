// Package data implements the document store ports.
package data

import (
	"sync"

	"github.com/amavis442/php-lsp-go/internal/features/documents/domain"
)

// MemoryStore keeps open documents in a map.
type MemoryStore struct {
	mu   sync.RWMutex
	docs map[string]domain.Document
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{docs: make(map[string]domain.Document)}
}

// Put stores d, replacing any earlier version.
func (s *MemoryStore) Put(d domain.Document) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[d.URI] = d
}

// Get returns the document for uri.
func (s *MemoryStore) Get(uri string) (domain.Document, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.docs[uri]
	return d, ok
}

// Delete forgets the document for uri.
func (s *MemoryStore) Delete(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, uri)
}
