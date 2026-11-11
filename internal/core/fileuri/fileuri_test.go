package fileuri

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestFromPathAndBackOnThisPlatform(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a b", "Ünit.php")

	uri := FromPath(path)
	got, err := ToPath(uri)

	if err != nil || got != Canonical(path) {
		t.Errorf("ToPath(FromPath(%q)) = %q, %v; via %q", path, got, err, uri)
	}
}

func TestWindowsForms(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows drive letters")
	}
	tests := []struct {
		uri  string
		path string
	}{
		{"file:///D:/work/a.php", `D:\work\a.php`},
		{"file:///d%3A/work/a.php", `D:\work\a.php`},
		{"file:///d:/work/a%20b.php", `D:\work\a b.php`},
	}
	for _, tt := range tests {
		got, err := ToPath(tt.uri)
		if err != nil || got != tt.path {
			t.Errorf("ToPath(%q) = %q, %v; want %q", tt.uri, got, err, tt.path)
		}
	}
	if got := FromPath(`d:\work\a b.php`); got != "file:///D:/work/a%20b.php" {
		t.Errorf("FromPath = %q, want an upper-case drive and an escaped space", got)
	}
}

func TestUnixForms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix paths")
	}
	if got := FromPath("/home/p/a b.php"); got != "file:///home/p/a%20b.php" {
		t.Errorf("FromPath = %q", got)
	}
	if got, err := ToPath("file:///home/p/a%20b.php"); err != nil || got != "/home/p/a b.php" {
		t.Errorf("ToPath = %q, %v", got, err)
	}
}

func TestToPathRejectsOtherSchemes(t *testing.T) {
	for _, uri := range []string{"untitled:Untitled-1", "https://example.com/a.php", "::"} {
		if _, err := ToPath(uri); err == nil {
			t.Errorf("ToPath(%q) succeeded, want error", uri)
		}
	}
}

func TestCanonicalMakesEqualURIsEqual(t *testing.T) {
	a, b := "file:///d%3A/work/a.php", "file:///D:/work/a.php"
	if runtime.GOOS != "windows" {
		a, b = "file:///home/p/a%2Eb.php", "file:///home/p/a.b.php"
	}
	if CanonicalURI(a) != CanonicalURI(b) {
		t.Errorf("CanonicalURI(%q) = %q, CanonicalURI(%q) = %q; want equal", a, CanonicalURI(a), b, CanonicalURI(b))
	}
	if got := CanonicalURI("untitled:x"); got != "untitled:x" {
		t.Errorf("CanonicalURI of a non-file URI = %q, want it unchanged", got)
	}
}
