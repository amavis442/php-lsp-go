package domain

import (
	"errors"
	"fmt"

	"github.com/amavis442/php-lsp-go/internal/core/text"
)

// ErrNotOpen means a change arrived for a document that was never opened.
var ErrNotOpen = errors.New("document is not open")

// Service tracks open documents. Open, Change and Close must be called in
// the order the editor sent them; Get may be called from any goroutine.
type Service struct {
	store     Store
	listeners []Listener
}

// NewService returns a service that keeps documents in store and tells
// listeners about every change.
func NewService(store Store, listeners ...Listener) *Service {
	return &Service{store: store, listeners: listeners}
}

// Open starts tracking a document with the editor's content.
func (s *Service) Open(uri string, version int32, content string) {
	s.update(Document{URI: uri, Version: version, Text: content})
}

// Change applies the editor's edits, in order, and records the new version.
func (s *Service) Change(uri string, version int32, changes []text.Change, enc text.Encoding) error {
	doc, ok := s.store.Get(uri)
	if !ok {
		return fmt.Errorf("change %s: %w", uri, ErrNotOpen)
	}
	s.update(Document{URI: uri, Version: version, Text: text.Apply(doc.Text, changes, enc)})
	return nil
}

// Close stops tracking a document.
func (s *Service) Close(uri string) {
	s.store.Delete(uri)
	for _, l := range s.listeners {
		l.DocumentClosed(uri)
	}
}

// Get returns the open document for uri.
func (s *Service) Get(uri string) (Document, bool) { return s.store.Get(uri) }

func (s *Service) update(doc Document) {
	s.store.Put(doc)
	for _, l := range s.listeners {
		l.DocumentChanged(doc.URI, doc.Version, doc.Text)
	}
}
