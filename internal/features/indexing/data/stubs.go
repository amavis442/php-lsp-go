package data

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// installedMarker is written last, so a directory holding it is complete.
const installedMarker = ".installed"

// Stubs puts the built-in PHP stubs on disk, so they can be indexed like
// library code and the editor can open them. Each version is installed once,
// in its own directory under the cache directory.
type Stubs struct {
	files   fs.FS
	version string
	dir     string
}

// NewStubs returns stubs that install files as version under cacheDir.
func NewStubs(files fs.FS, version, cacheDir string) *Stubs {
	return &Stubs{files: files, version: version, dir: cacheDir}
}

// Roots installs the stubs when they are not installed yet and returns the
// directory that holds them.
func (s *Stubs) Roots() ([]string, error) {
	root := filepath.Join(s.dir, "stubs", s.version)
	if !installed(root) {
		if err := s.install(root); err != nil {
			return nil, fmt.Errorf("installing stubs in %s: %w", root, err)
		}
	}
	return []string{root}, nil
}

// install writes the stubs to a temporary directory and renames it into
// place, so another server never sees a half-written install.
func (s *Stubs) install(root string) error {
	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, s.version+".tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp) // already gone after a successful rename

	if err := os.CopyFS(tmp, s.files); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, installedMarker), nil, 0o644); err != nil {
		return err
	}
	if !installed(root) {
		// What is there was left by an install that did not finish.
		if err := os.RemoveAll(root); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, root); err != nil {
		if installed(root) {
			return nil // another server installed them at the same time
		}
		return err
	}
	return nil
}

func installed(root string) bool {
	_, err := os.Stat(filepath.Join(root, installedMarker))
	return err == nil
}
