package data

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/amavis442/php-lsp-go/internal/features/indexing/domain"
)

func write(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func walk(t *testing.T, fs *FileSystem, root string) []string {
	t.Helper()
	var got []string
	err := fs.Walk(context.Background(), root, func(p string, _ domain.Stamp) error {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		got = append(got, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	sort.Strings(got)
	return got
}

func TestWalkFindsPHPFilesAndSkipsExcludedDirectoriesAndLargeFiles(t *testing.T) {
	root := t.TempDir()
	write(t, root, "app/User.php", "<?php")
	write(t, root, "app/Upper.PHP", "<?php")
	write(t, root, "vendor/lib/Lib.php", "<?php")
	write(t, root, "README.md", "# hi")
	write(t, root, ".git/hooks/x.php", "<?php")
	write(t, root, "node_modules/pkg/y.php", "<?php")
	write(t, root, "big.php", strings.Repeat("x", 101))

	got := walk(t, NewFileSystem([]string{".git", "node_modules"}, 100), root)

	want := "app/Upper.PHP app/User.php vendor/lib/Lib.php"
	if strings.Join(got, " ") != want {
		t.Errorf("walked %v, want %s", got, want)
	}
}

func TestWalkStampsEachFileWithItsSizeAndModificationTime(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.php", "<?php // 17 bytes")
	when := time.Date(2026, 1, 2, 3, 4, 5, 6000, time.UTC)
	if err := os.Chtimes(filepath.Join(root, "a.php"), when, when); err != nil {
		t.Fatal(err)
	}

	var got domain.Stamp
	err := NewFileSystem(nil, DefaultMaxSize).Walk(context.Background(), root, func(_ string, s domain.Stamp) error {
		got = s
		return nil
	})

	if err != nil || got.Size != 17 || got.ModTime != when.UnixNano() {
		t.Errorf("stamp = %+v, %v; want 17 bytes at %d", got, err, when.UnixNano())
	}
}

func TestWalkStopsWhenCancelled(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.php", "<?php")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := NewFileSystem(nil, 1<<20).Walk(ctx, root, func(string, domain.Stamp) error { return nil })

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestWalkOfAMissingRootFails(t *testing.T) {
	err := NewFileSystem(nil, 1<<20).Walk(context.Background(), filepath.Join(t.TempDir(), "missing"),
		func(string, domain.Stamp) error { return nil })

	if err == nil {
		t.Error("Walk of a missing root succeeded, want error")
	}
}

func TestReadReturnsTheFileContent(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.php", "<?php echo 1;")

	got, err := NewFileSystem(nil, 1<<20).Read(filepath.Join(root, "a.php"))

	if err != nil || string(got) != "<?php echo 1;" {
		t.Errorf("Read = %q, %v", got, err)
	}
}

func TestReadRefusesFilesAboveTheSizeLimit(t *testing.T) {
	root := t.TempDir()
	write(t, root, "big.php", strings.Repeat("x", 101))

	if _, err := NewFileSystem(nil, 100).Read(filepath.Join(root, "big.php")); err == nil {
		t.Error("Read of a file above the limit succeeded, want error")
	}
}
