// Package domain holds the rules for documents the editor has open: their
// text is the editor's, not the disk's, and every change moves the version on.
package domain

// Document is the editor's current content of one open file.
type Document struct {
	URI     string
	Version int32
	Text    string
}

// Store keeps open documents. Implementations must be safe for concurrent use.
type Store interface {
	Put(d Document)
	Get(uri string) (Document, bool)
	Delete(uri string)
}

// Listener hears about every new document content and every close. It takes
// plain values, so other features can listen without importing this package.
type Listener interface {
	DocumentChanged(uri string, version int32, text string)
	DocumentClosed(uri string)
}
