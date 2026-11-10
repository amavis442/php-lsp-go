package data

import (
	"sync"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/features/documents/domain"
)

func TestMemoryStorePutGetDelete(t *testing.T) {
	s := NewMemoryStore()
	doc := domain.Document{URI: "file:///a.php", Version: 1, Text: "<?php"}

	s.Put(doc)
	got, ok := s.Get(doc.URI)
	if !ok || got != doc {
		t.Fatalf("Get = %+v, %v; want %+v, true", got, ok, doc)
	}

	s.Delete(doc.URI)
	if _, ok := s.Get(doc.URI); ok {
		t.Error("Get after Delete found the document")
	}
}

func TestMemoryStoreIsSafeForConcurrentUse(t *testing.T) {
	s := NewMemoryStore()
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Put(domain.Document{URI: "file:///a.php", Version: int32(i)})
			s.Get("file:///a.php")
		}()
	}
	wg.Wait()
}
