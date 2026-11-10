// Package domain decides which problems the server reports for a document.
package domain

import "github.com/amavis442/php-lsp-go/internal/core/php"

// SyntaxChecker finds the syntax errors in PHP source.
type SyntaxChecker interface {
	SyntaxErrors(src []byte) ([]php.SyntaxError, error)
}

// Diagnostic is one problem in a document. Start and End are byte offsets.
type Diagnostic struct {
	Start   int
	End     int
	Message string
}

// Collector gathers the diagnostics for a document's source.
type Collector struct {
	checker SyntaxChecker
}

// NewCollector returns a collector that reports what checker finds.
func NewCollector(checker SyntaxChecker) *Collector {
	return &Collector{checker: checker}
}

// Collect returns the diagnostics for source, in source order.
func (c *Collector) Collect(source string) ([]Diagnostic, error) {
	errs, err := c.checker.SyntaxErrors([]byte(source))
	if err != nil {
		return nil, err
	}
	diags := make([]Diagnostic, len(errs))
	for i, e := range errs {
		diags[i] = Diagnostic{Start: e.Start, End: e.End, Message: e.Message}
	}
	return diags, nil
}
