// Package data implements the indexing ports on the file system and in
// memory.
package data

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/amavis442/php-lsp-go/internal/features/indexing/domain"
)

// DefaultExclude are the directories never indexed by default.
var DefaultExclude = []string{".git", "node_modules"}

// DefaultMaxSize is the largest file indexed by default. Bigger PHP files
// are almost always generated and not worth the parse.
const DefaultMaxSize = 1 << 20

// FileSystem finds and reads PHP files on disk.
type FileSystem struct {
	exclude map[string]bool
	maxSize int64
}

// NewFileSystem returns a file system that skips directories named in
// exclude and files larger than maxSize bytes.
func NewFileSystem(exclude []string, maxSize int64) *FileSystem {
	ex := make(map[string]bool, len(exclude))
	for _, name := range exclude {
		ex[name] = true
	}
	return &FileSystem{exclude: ex, maxSize: maxSize}
}

// Walk calls visit for every .php file under root, with its stamp.
// Symbolic links are not followed, so a link cycle cannot trap the walk.
// Entries that cannot be read below root are skipped.
func (f *FileSystem) Walk(ctx context.Context, root string, visit func(path string, stamp domain.Stamp) error) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if p == root {
				return err
			}
			return nil
		}
		if d.IsDir() {
			if p != root && f.exclude[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !strings.EqualFold(filepath.Ext(p), ".php") {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > f.maxSize {
			return nil
		}
		return visit(p, domain.Stamp{Size: info.Size(), ModTime: info.ModTime().UnixNano()})
	})
}

// Read returns the content of path, refusing files above the size limit.
func (f *FileSystem) Read(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > f.maxSize {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, f.maxSize)
	}
	return os.ReadFile(path)
}
