// Package fileuri converts between file:// URIs and file system paths, and
// gives every file one canonical URI so the same file is never indexed
// twice under different spellings (file:///D:/x and file:///d%3A/x).
package fileuri

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// FromPath returns the canonical file URI of path.
func FromPath(path string) string {
	p := filepath.ToSlash(Canonical(path))
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // a Windows drive path: D:/x becomes /D:/x
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// ToPath returns the file system path of a file URI.
func ToPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("parsing %q: %w", uri, err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("%q is not a file URI", uri)
	}
	p := u.Path
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:] // /D:/x is the Windows path D:/x
	}
	return Canonical(filepath.FromSlash(p)), nil
}

// Canonical cleans path and upper-cases a Windows drive letter.
func Canonical(path string) string {
	p := filepath.Clean(path)
	if vol := filepath.VolumeName(p); len(vol) == 2 && vol[1] == ':' {
		p = strings.ToUpper(vol) + p[2:]
	}
	return p
}

// CanonicalURI rewrites a file URI into its canonical form. Other URIs are
// returned unchanged.
func CanonicalURI(uri string) string {
	path, err := ToPath(uri)
	if err != nil {
		return uri
	}
	return FromPath(path)
}
