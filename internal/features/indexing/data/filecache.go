package data

import (
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/amavis442/php-lsp-go/internal/core/text"
	"github.com/amavis442/php-lsp-go/internal/features/indexing/domain"
)

// cacheFormat is the version of the file layout below. The build check
// covers changes to the symbols themselves.
const cacheFormat = 1

type cacheFile struct {
	Format   int
	Build    string
	Encoding text.Encoding
	Roots    []string
	Files    map[string]domain.CachedFile
}

// FileCache keeps index scans on disk, one file per set of roots. A scan is
// only used by the same build of the server: another build may extract
// differently, so it starts afresh.
type FileCache struct {
	dir   string
	build string
}

// NewFileCache returns a cache in dir for the server build identified by
// build.
func NewFileCache(dir, build string) *FileCache {
	return &FileCache{dir: dir, build: build}
}

// Load returns the scan saved for roots in enc by this build, or nil.
func (c *FileCache) Load(roots []string, enc text.Encoding) map[string]domain.CachedFile {
	roots = sorted(roots)
	f, err := os.Open(c.path(roots))
	if err != nil {
		return nil
	}
	defer f.Close()
	var saved cacheFile
	if err := gob.NewDecoder(f).Decode(&saved); err != nil {
		return nil
	}
	if saved.Format != cacheFormat || saved.Build != c.build || saved.Encoding != enc ||
		!slices.Equal(saved.Roots, roots) {
		return nil
	}
	if saved.Files == nil {
		saved.Files = map[string]domain.CachedFile{}
	}
	return saved.Files
}

// Save replaces the scan kept for roots. It writes a temporary file and
// renames it into place, so a reader never sees half a file.
func (c *FileCache) Save(roots []string, enc text.Encoding, files map[string]domain.CachedFile) error {
	roots = sorted(roots)
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	target := c.path(roots)
	tmp, err := os.CreateTemp(c.dir, filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // gone after a successful rename

	err = gob.NewEncoder(tmp).Encode(cacheFile{
		Format: cacheFormat, Build: c.build, Encoding: enc, Roots: roots, Files: files,
	})
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}

// path names the cache file of roots after a hash of them.
func (c *FileCache) path(roots []string) string {
	sum := sha256.Sum256([]byte(strings.Join(roots, "\n")))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:8])+".gob")
}

func sorted(roots []string) []string {
	out := slices.Clone(roots)
	slices.Sort(out)
	return out
}
