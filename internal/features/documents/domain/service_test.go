package domain

import (
	"errors"
	"reflect"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/text"
)

type fakeStore map[string]Document

func (s fakeStore) Put(d Document) { s[d.URI] = d }

func (s fakeStore) Get(uri string) (Document, bool) {
	d, ok := s[uri]
	return d, ok
}

func (s fakeStore) Delete(uri string) { delete(s, uri) }

type event struct {
	kind    string
	uri     string
	version int32
	text    string
}

type recorder struct{ events []event }

func (r *recorder) DocumentChanged(uri string, version int32, text string) {
	r.events = append(r.events, event{"changed", uri, version, text})
}

func (r *recorder) DocumentClosed(uri string) {
	r.events = append(r.events, event{kind: "closed", uri: uri})
}

const uri = "file:///p/a.php"

func TestOpenStoresTheDocumentAndNotifies(t *testing.T) {
	store, rec := fakeStore{}, &recorder{}
	s := NewService(store, rec)

	s.Open(uri, 1, "<?php")

	if got := store[uri]; got != (Document{URI: uri, Version: 1, Text: "<?php"}) {
		t.Errorf("stored %+v", got)
	}
	want := []event{{"changed", uri, 1, "<?php"}}
	if !reflect.DeepEqual(rec.events, want) {
		t.Errorf("events = %+v, want %+v", rec.events, want)
	}
}

func TestChangeAppliesEditsAndNotifies(t *testing.T) {
	store, rec := fakeStore{}, &recorder{}
	s := NewService(store, rec)
	s.Open(uri, 1, "<?php\necho;")

	r := text.Range{Start: text.Position{Line: 1, Character: 4}, End: text.Position{Line: 1, Character: 4}}
	err := s.Change(uri, 2, []text.Change{{Range: &r, Text: " 1"}}, text.UTF16)

	if err != nil {
		t.Fatalf("Change: %v", err)
	}
	if got := store[uri]; got.Version != 2 || got.Text != "<?php\necho 1;" {
		t.Errorf("stored %+v", got)
	}
	if last := rec.events[len(rec.events)-1]; last != (event{"changed", uri, 2, "<?php\necho 1;"}) {
		t.Errorf("last event = %+v", last)
	}
}

func TestChangeOfUnknownDocumentFails(t *testing.T) {
	rec := &recorder{}
	s := NewService(fakeStore{}, rec)

	err := s.Change(uri, 2, []text.Change{{Text: "x"}}, text.UTF16)

	if !errors.Is(err, ErrNotOpen) {
		t.Errorf("err = %v, want ErrNotOpen", err)
	}
	if len(rec.events) != 0 {
		t.Errorf("listener notified for unknown document: %+v", rec.events)
	}
}

func TestCloseForgetsTheDocumentAndNotifies(t *testing.T) {
	store, rec := fakeStore{}, &recorder{}
	s := NewService(store, rec)
	s.Open(uri, 1, "<?php")

	s.Close(uri)

	if _, ok := store[uri]; ok {
		t.Error("document still stored after close")
	}
	if last := rec.events[len(rec.events)-1]; last != (event{kind: "closed", uri: uri}) {
		t.Errorf("last event = %+v", last)
	}
}

func TestGetReturnsTheOpenDocument(t *testing.T) {
	s := NewService(fakeStore{})
	s.Open(uri, 3, "x")

	got, ok := s.Get(uri)

	if !ok || got.Version != 3 {
		t.Errorf("Get = %+v, %v", got, ok)
	}
}
