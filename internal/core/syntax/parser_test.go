package syntax

import (
	"reflect"
	"sync"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/php"
)

func TestSyntaxErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []php.SyntaxError
	}{
		{
			name: "valid code has no errors",
			src:  "<?php\necho 1;\n",
			want: nil,
		},
		{
			name: "html around php is valid",
			src:  "<html><?php echo 1 ?></html>",
			want: nil,
		},
		{
			name: "missing semicolon is a zero-width error after the expression",
			src:  "<?php\necho 1\n",
			want: []php.SyntaxError{{Start: 12, End: 12, Message: "Syntax error: missing ';'"}},
		},
		{
			name: "unexpected token covers the token",
			src:  "<?php\n$x = ;\n",
			want: []php.SyntaxError{{Start: 9, End: 10, Message: "Syntax error: unexpected '='"}},
		},
		{
			name: "several errors are reported in source order",
			src:  "<?php\nclass A { public function f() { return 1 }\n",
			want: []php.SyntaxError{
				{Start: 46, End: 46, Message: "Syntax error: missing ';'"},
				{Start: 48, End: 48, Message: "Syntax error: missing '}'"},
			},
		},
	}
	p := NewParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.SyntaxErrors([]byte(tt.src))
			if err != nil {
				t.Fatalf("SyntaxErrors: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SyntaxErrors = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestUnexpectedTextIsShortenedToItsFirstLine(t *testing.T) {
	got := unexpectedMessage("  foo bar baz qux quux corge\nnext line")

	if want := "Syntax error: unexpected 'foo bar baz qux quux…'"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestParserIsSafeForConcurrentUse(t *testing.T) {
	p := NewParser()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if errs, err := p.SyntaxErrors([]byte("<?php\necho 1\n")); err != nil || len(errs) != 1 {
				t.Errorf("got %d errors (err %v), want 1", len(errs), err)
			}
		}()
	}
	wg.Wait()
}
