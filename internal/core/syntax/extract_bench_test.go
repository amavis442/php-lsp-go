package syntax

import (
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkExtract measures parse plus extraction of the PHP 8.4 fixture.
func BenchmarkExtract(b *testing.B) {
	src, err := os.ReadFile(filepath.Join("testdata", "extract", "php84.php"))
	if err != nil {
		b.Fatal(err)
	}
	p := NewParser()
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := p.Extract(src); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseOnly measures the parser alone, as the baseline for
// BenchmarkExtract.
func BenchmarkParseOnly(b *testing.B) {
	src, err := os.ReadFile(filepath.Join("testdata", "extract", "php84.php"))
	if err != nil {
		b.Fatal(err)
	}
	p := NewParser()
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		tree, err := p.parser.Parse(src)
		if err != nil {
			b.Fatal(err)
		}
		tree.Release()
	}
}
