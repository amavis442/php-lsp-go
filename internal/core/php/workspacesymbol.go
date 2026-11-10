package php

import "github.com/amavis442/php-lsp-go/internal/core/text"

// WorkspaceSymbol is a declaration as the workspace index keeps it: flat,
// with positions already in the editor's encoding and the qualified name of
// what contains it (its namespace, or its class for a member).
type WorkspaceSymbol struct {
	Name           string
	Kind           SymbolKind
	Container      string
	URI            string
	Range          text.Range
	SelectionRange text.Range
}
