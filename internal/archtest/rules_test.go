package archtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
)

const mod = "github.com/amavis442/php-lsp-go"

func TestCheckFlagsDomainImportingThirdParty(t *testing.T) {
	pkgs := []Package{{Path: mod + "/internal/features/hover/domain", Imports: []string{"fmt", "go.lsp.dev/protocol"}}}

	got := Check(mod, pkgs)

	assertViolations(t, got, mod+"/internal/features/hover/domain imports go.lsp.dev/protocol")
}

func TestCheckFlagsDomainImportingDataLayer(t *testing.T) {
	pkgs := []Package{{Path: mod + "/internal/features/hover/domain", Imports: []string{mod + "/internal/features/hover/data"}}}

	got := Check(mod, pkgs)

	assertViolations(t, got, mod+"/internal/features/hover/domain imports "+mod+"/internal/features/hover/data")
}

func TestCheckAllowsDomainImportingPureCore(t *testing.T) {
	pkgs := []Package{{Path: mod + "/internal/features/hover/domain", Imports: []string{"strings", mod + "/internal/core/php", mod + "/internal/core/text"}}}

	assertViolations(t, Check(mod, pkgs))
}

func TestCheckRestrictsProtocolLibraryToServerAndPresentation(t *testing.T) {
	pkgs := []Package{
		{Path: mod + "/internal/server", Imports: []string{"go.lsp.dev/protocol"}},
		{Path: mod + "/internal/features/hover/presentation", Imports: []string{"go.lsp.dev/protocol"}},
		{Path: mod + "/internal/features/hover/data", Imports: []string{"go.lsp.dev/jsonrpc2"}},
	}

	got := Check(mod, pkgs)

	assertViolations(t, got, mod+"/internal/features/hover/data imports go.lsp.dev/jsonrpc2")
}

func TestCheckRestrictsParserLibraryToCoreSyntax(t *testing.T) {
	pkgs := []Package{
		{Path: mod + "/internal/core/syntax", Imports: []string{"github.com/odvcencio/gotreesitter"}},
		{Path: mod + "/internal/features/indexing/data", Imports: []string{"github.com/odvcencio/gotreesitter/grammars/php"}},
	}

	got := Check(mod, pkgs)

	assertViolations(t, got, mod+"/internal/features/indexing/data imports github.com/odvcencio/gotreesitter/grammars/php")
}

// TestModuleFollowsArchitecture applies the rules to the real module.
func TestModuleFollowsArchitecture(t *testing.T) {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = "../.."
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, stderr.String())
	}

	var pkgs []Package
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p struct {
			ImportPath string
			Imports    []string
		}
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("decoding go list output: %v", err)
		}
		pkgs = append(pkgs, Package{Path: p.ImportPath, Imports: p.Imports})
	}
	if len(pkgs) == 0 {
		t.Fatal("go list returned no packages")
	}

	assertViolations(t, Check(mod, pkgs))
}

func assertViolations(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("violations:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
