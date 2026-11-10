// Package syntax is the only package that uses the parser library. It turns
// PHP source into the parser-independent types of package php.
package syntax

import (
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	ts "github.com/odvcencio/gotreesitter"
	phpgrammar "github.com/odvcencio/gotreesitter/grammars/php"

	"github.com/amavis442/php-lsp-go/internal/core/php"
)

// maxUnexpectedRunes caps how much of the offending text a message quotes.
const maxUnexpectedRunes = 20

// Parser parses PHP source. It is safe for concurrent use: a tree-sitter
// parser is not, so calls take turns on one underlying parser.
type Parser struct {
	mu     sync.Mutex
	lang   *ts.Language
	parser *ts.Parser
}

// NewParser returns a parser for PHP files (HTML with <?php blocks).
func NewParser() *Parser {
	lang := phpgrammar.Language()
	return &Parser{lang: lang, parser: ts.NewParser(lang)}
}

// SyntaxErrors parses src and returns its syntax errors in source order.
// Each call parses the whole file; the tree is released before returning.
func (p *Parser) SyntaxErrors(src []byte) ([]php.SyntaxError, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	tree, err := p.parser.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parsing PHP: %w", err)
	}
	defer tree.Release()

	var errs []php.SyntaxError
	p.collect(tree.RootNode(), src, &errs)
	return errs, nil
}

// collect walks only into subtrees that contain an error, so a valid file
// costs a single check on the root.
func (p *Parser) collect(n *ts.Node, src []byte, errs *[]php.SyntaxError) {
	switch {
	case n.IsMissing():
		*errs = append(*errs, php.SyntaxError{
			Start:   int(n.StartByte()),
			End:     int(n.EndByte()),
			Message: fmt.Sprintf("Syntax error: missing '%s'", n.Type(p.lang)),
		})
	case n.IsError():
		start, end := int(n.StartByte()), int(n.EndByte())
		*errs = append(*errs, php.SyntaxError{
			Start:   start,
			End:     end,
			Message: unexpectedMessage(string(src[start:end])),
		})
	case n.HasError():
		for i := range n.ChildCount() {
			p.collect(n.Child(i), src, errs)
		}
	}
}

// unexpectedMessage quotes the first line of the offending text, shortened
// so a large unparsable region does not flood the editor.
func unexpectedMessage(text string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	first = strings.TrimSpace(first)
	if first == "" {
		return "Syntax error"
	}
	if utf8.RuneCountInString(first) > maxUnexpectedRunes {
		first = string([]rune(first)[:maxUnexpectedRunes]) + "…"
	}
	return fmt.Sprintf("Syntax error: unexpected '%s'", first)
}
