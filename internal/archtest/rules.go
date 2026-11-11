// Package archtest enforces the dependency rules of the architecture. Its
// test runs them against the real module, so a forbidden import fails the
// build instead of slipping through review.
package archtest

import "strings"

// Package is a module package and the packages it imports.
type Package struct {
	Path    string
	Imports []string
}

const (
	protocolLibrary = "go.lsp.dev/"
	parserLibrary   = "github.com/odvcencio/gotreesitter"
)

// Check returns one "<package> imports <import>" line per forbidden import:
//   - pure packages (core/text, core/php, core/fileuri, core/resolve and every domain layer) import only
//     the standard library and other pure packages;
//   - only internal/server and feature presentation layers use the LSP library;
//   - only internal/core/syntax uses the parser library.
func Check(module string, pkgs []Package) []string {
	var violations []string
	for _, p := range pkgs {
		rel := strings.TrimPrefix(p.Path, module+"/")
		for _, imp := range p.Imports {
			if !allowed(module, rel, imp) {
				violations = append(violations, p.Path+" imports "+imp)
			}
		}
	}
	return violations
}

func allowed(module, rel, imp string) bool {
	if isPure(rel) {
		if isStandardLibrary(imp) {
			return true
		}
		inner, ok := strings.CutPrefix(imp, module+"/")
		return ok && isPure(inner)
	}
	if strings.HasPrefix(imp, protocolLibrary) {
		return rel == "internal/server" || isPresentation(rel)
	}
	if imp == parserLibrary || strings.HasPrefix(imp, parserLibrary+"/") {
		return rel == "internal/core/syntax"
	}
	return true
}

func isPure(rel string) bool {
	return rel == "internal/core/text" || rel == "internal/core/php" || rel == "internal/core/fileuri" || rel == "internal/core/resolve" ||
		strings.HasSuffix(rel, "/domain") || strings.Contains(rel, "/domain/")
}

func isPresentation(rel string) bool {
	return strings.HasPrefix(rel, "internal/features/") &&
		(strings.HasSuffix(rel, "/presentation") || strings.Contains(rel, "/presentation/"))
}

// isStandardLibrary relies on the Go convention that only standard library
// import paths lack a dot in their first element.
func isStandardLibrary(imp string) bool {
	first, _, _ := strings.Cut(imp, "/")
	return !strings.Contains(first, ".")
}
