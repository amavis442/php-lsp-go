package domain

import (
	"errors"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/fileuri"
	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
)

const docURI = "file:///D:/app/Controller.php"

type fakeResolver struct {
	found []php.WorkspaceSymbol
	err   error
}

func (r fakeResolver) At(string, text.Position, text.Encoding) ([]php.WorkspaceSymbol, error) {
	return r.found, r.err
}

type fakeDocs map[string]string

func (d fakeDocs) Text(uri string) (string, bool) {
	t, ok := d[uri]
	return t, ok
}

type fakeFiles map[string]string

func (f fakeFiles) Read(path string) ([]byte, error) {
	t, ok := f[path]
	if !ok {
		return nil, errors.New("no such file")
	}
	return []byte(t), nil
}

// fakeDescriber describes whatever is at the offset by the text there.
type fakeDescriber struct{}

func (fakeDescriber) Describe(src []byte, offset int) (php.Description, bool, error) {
	end := offset
	for end < len(src) && src[end] != '\n' {
		end++
	}
	if offset == end {
		return php.Description{}, false, nil
	}
	return php.Description{Signature: string(src[offset:end]), Summary: "from " + string(src[:5])}, true, nil
}

func at(line, character int) text.Range {
	p := text.Position{Line: line, Character: character}
	return text.Range{Start: p, End: p}
}

func TestHoverDescribesTheDeclarationFromItsFileOnDisk(t *testing.T) {
	path := `D:\app\User.php`
	user := php.WorkspaceSymbol{Name: "User", Kind: php.KindClass, Container: "App", URI: fileuri.FromPath(path),
		SelectionRange: at(1, 6)}
	// The open copy of User.php is edited; the index positions belong to
	// the copy on disk.
	h := NewHover(fakeResolver{found: []php.WorkspaceSymbol{user}},
		fakeDocs{docURI: "<?php new User();", fileuri.FromPath(path): "<?php\n// edited\nclass User {}"},
		fakeFiles{path: "DISK\nclass User {}"}, fakeDescriber{})

	info, ok, err := h.At(docURI, text.Position{}, text.UTF8)

	if err != nil || !ok || info.Signature != "User {}" || info.Summary != "from DISK\n" || info.Container != "App" {
		t.Errorf("At = %+v, %v, %v; want User described from the file on disk", info, ok, err)
	}
}

func TestHoverReadsTheRequestingDocumentFromTheEditor(t *testing.T) {
	// A declaration in the document itself was found in its fresh copy, so
	// its positions belong to the editor's text.
	own := php.WorkspaceSymbol{Name: "show", Kind: php.KindMethod, Container: `App\Controller`, Member: true,
		URI: docURI, SelectionRange: at(1, 0)}
	h := NewHover(fakeResolver{found: []php.WorkspaceSymbol{own}},
		fakeDocs{docURI: "EDIT\nfunction show()"}, fakeFiles{}, fakeDescriber{})

	info, ok, err := h.At(docURI, text.Position{}, text.UTF8)

	if err != nil || !ok || info.Signature != "function show()" || info.Container != `App\Controller` {
		t.Errorf("At = %+v, %v, %v", info, ok, err)
	}
}

func TestHoverConvertsTheNamePositionWithTheEncoding(t *testing.T) {
	path := `D:\app\a.php`
	s := php.WorkspaceSymbol{Name: "x", Kind: php.KindFunction, URI: fileuri.FromPath(path), SelectionRange: at(1, 3)}
	h := NewHover(fakeResolver{found: []php.WorkspaceSymbol{s}}, fakeDocs{docURI: ""},
		fakeFiles{path: "<?php\n😀 x()"}, fakeDescriber{})

	info, ok, _ := h.At(docURI, text.Position{}, text.UTF16)

	if !ok || info.Signature != "x()" {
		t.Errorf("At = %+v, %v; want the name after the emoji", info, ok)
	}
}

func TestHoverShowsTheFirstOfSeveralDeclarations(t *testing.T) {
	a := php.WorkspaceSymbol{Name: "A", Kind: php.KindClass, URI: fileuri.FromPath(`D:\a.php`), SelectionRange: at(0, 0)}
	b := php.WorkspaceSymbol{Name: "A", Kind: php.KindClass, URI: fileuri.FromPath(`D:\b.php`), SelectionRange: at(0, 0)}
	h := NewHover(fakeResolver{found: []php.WorkspaceSymbol{a, b}}, fakeDocs{docURI: ""},
		fakeFiles{`D:\a.php`: "first", `D:\b.php`: "second"}, fakeDescriber{})

	if info, ok, _ := h.At(docURI, text.Position{}, text.UTF8); !ok || info.Signature != "first" {
		t.Errorf("At = %+v, %v", info, ok)
	}
}

func TestHoverHasNothingWhenNothingIsFound(t *testing.T) {
	h := NewHover(fakeResolver{}, fakeDocs{}, fakeFiles{}, fakeDescriber{})

	if _, ok, err := h.At(docURI, text.Position{}, text.UTF8); ok || err != nil {
		t.Errorf("ok = %v, err = %v; want nothing", ok, err)
	}
}

func TestHoverReportsAFileThatCannotBeRead(t *testing.T) {
	s := php.WorkspaceSymbol{Name: "A", Kind: php.KindClass, URI: fileuri.FromPath(`D:\gone.php`)}
	h := NewHover(fakeResolver{found: []php.WorkspaceSymbol{s}}, fakeDocs{}, fakeFiles{}, fakeDescriber{})

	if _, ok, err := h.At(docURI, text.Position{}, text.UTF8); ok || err == nil {
		t.Errorf("ok = %v, err = %v; want the read error", ok, err)
	}
}
