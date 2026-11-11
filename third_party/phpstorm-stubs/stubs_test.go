package phpstormstubs

import (
	"io/fs"
	"strings"
	"testing"
)

func TestFilesHoldTheStubsAndTheirLicense(t *testing.T) {
	for _, name := range []string{
		"LICENSE",
		"Core/Core.php",
		"standard/standard_1.php",
		"standard/_standard_manual.php", // names starting with _ are embedded too
		"Zend OPcache/OPcache.php",      // and directory names with a space
		"PDO/PDO.php",
		"SPL/SPL.php",
	} {
		if _, err := fs.Stat(Files, name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestFilesHoldOnlyPHPBesidesTheLicense(t *testing.T) {
	err := fs.WalkDir(Files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && path != "LICENSE" && !strings.HasSuffix(path, ".php") {
			t.Errorf("unexpected file %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestVersionNamesThePinnedTag(t *testing.T) {
	if Version != "v2026.2" {
		t.Errorf("Version = %q, want the tag the files were taken from", Version)
	}
}
