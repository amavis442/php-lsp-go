package domain

import (
	"errors"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/php"
)

type fakeDocs map[string]string

func (f fakeDocs) Text(uri string) (string, bool) {
	t, ok := f[uri]
	return t, ok
}

// fakeExtractor declares one function named after the source.
type fakeExtractor struct{ err error }

func (f fakeExtractor) Extract(src []byte) (php.FileSummary, error) {
	if f.err != nil {
		return php.FileSummary{}, f.err
	}
	return php.FileSummary{Declarations: []php.Declaration{{Name: string(src), Kind: php.KindFunction}}}, nil
}

func TestOutlineOfAnOpenDocument(t *testing.T) {
	symbols := NewDocumentSymbols(fakeDocs{"file:///a.php": "src"}, fakeExtractor{})

	got, err := symbols.For("file:///a.php")

	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if got.Text != "src" || len(got.Declarations) != 1 || got.Declarations[0].Name != "src" {
		t.Errorf("outline = %+v, want the declarations of %q with its text", got, "src")
	}
}

func TestOutlineOfAnUnknownDocumentIsEmpty(t *testing.T) {
	symbols := NewDocumentSymbols(fakeDocs{}, fakeExtractor{})

	got, err := symbols.For("file:///missing.php")

	if err != nil || got.Text != "" || len(got.Declarations) != 0 {
		t.Errorf("outline = %+v, %v; want an empty outline", got, err)
	}
}

func TestOutlineReportsExtractionErrors(t *testing.T) {
	boom := errors.New("boom")
	symbols := NewDocumentSymbols(fakeDocs{"file:///a.php": "src"}, fakeExtractor{err: boom})

	if _, err := symbols.For("file:///a.php"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}
