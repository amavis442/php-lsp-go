package data

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

var stubFiles = fstest.MapFS{
	"LICENSE":       {Data: []byte("license")},
	"Core/Core.php": {Data: []byte("<?php\nfunction strlen() {}\n")},
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestStubsAreInstalledUnderTheirVersion(t *testing.T) {
	cache := t.TempDir()

	roots, err := NewStubs(stubFiles, "v1", cache).Roots()

	want := filepath.Join(cache, "stubs", "v1")
	if err != nil || len(roots) != 1 || roots[0] != want {
		t.Fatalf("Roots() = %v, %v; want [%s]", roots, err, want)
	}
	if got := readFile(t, filepath.Join(want, "Core", "Core.php")); got != "<?php\nfunction strlen() {}\n" {
		t.Errorf("Core.php = %q", got)
	}
	if got := readFile(t, filepath.Join(want, "LICENSE")); got != "license" {
		t.Errorf("LICENSE = %q", got)
	}
}

func TestInstalledStubsAreNotWrittenAgain(t *testing.T) {
	cache := t.TempDir()
	if _, err := NewStubs(stubFiles, "v1", cache).Roots(); err != nil {
		t.Fatal(err)
	}
	core := filepath.Join(cache, "stubs", "v1", "Core", "Core.php")
	if err := os.Remove(core); err != nil {
		t.Fatal(err)
	}

	if _, err := NewStubs(stubFiles, "v1", cache).Roots(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(core); !os.IsNotExist(err) {
		t.Errorf("Core.php was written again (stat err = %v); want the installed copy reused", err)
	}
}

func TestAnotherVersionIsInstalledBesideTheFirst(t *testing.T) {
	cache := t.TempDir()
	if _, err := NewStubs(stubFiles, "v1", cache).Roots(); err != nil {
		t.Fatal(err)
	}

	roots, err := NewStubs(stubFiles, "v2", cache).Roots()

	if err != nil || roots[0] != filepath.Join(cache, "stubs", "v2") {
		t.Fatalf("Roots() = %v, %v", roots, err)
	}
	readFile(t, filepath.Join(cache, "stubs", "v2", "Core", "Core.php"))
}

func TestAnUnfinishedInstallIsReplaced(t *testing.T) {
	cache := t.TempDir()
	partial := filepath.Join(cache, "stubs", "v1")
	if err := os.MkdirAll(filepath.Join(partial, "Core"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(partial, "Core", "Core.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := NewStubs(stubFiles, "v1", cache).Roots(); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, filepath.Join(partial, "Core", "Core.php")); got != "<?php\nfunction strlen() {}\n" {
		t.Errorf("Core.php = %q, want the complete file", got)
	}
}

func TestInstallLeavesNoTemporaryDirectories(t *testing.T) {
	cache := t.TempDir()

	if _, err := NewStubs(stubFiles, "v1", cache).Roots(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Join(cache, "stubs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "v1" {
		t.Errorf("stubs directory holds %v, want only v1", entries)
	}
}

func TestStubsReportAnUnusableCacheDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := NewStubs(stubFiles, "v1", file).Roots(); err == nil {
		t.Error("Roots() succeeded under a file; want an error")
	}
}
