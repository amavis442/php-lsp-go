// Package php holds the PHP domain model shared by several features. It
// knows nothing about the parser or the LSP protocol.
package php

// SyntaxError is a parse error in a PHP file. Start and End are byte
// offsets; a zero-width span marks where a missing token was expected.
type SyntaxError struct {
	Start   int
	End     int
	Message string
}
