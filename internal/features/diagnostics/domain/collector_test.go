package domain

import (
	"errors"
	"reflect"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/php"
)

type fakeChecker struct {
	errs []php.SyntaxError
	err  error
	got  string
}

func (f *fakeChecker) SyntaxErrors(src []byte) ([]php.SyntaxError, error) {
	f.got = string(src)
	return f.errs, f.err
}

func TestCollectTurnsSyntaxErrorsIntoDiagnostics(t *testing.T) {
	checker := &fakeChecker{errs: []php.SyntaxError{
		{Start: 1, End: 2, Message: "Syntax error: unexpected '='"},
		{Start: 5, End: 5, Message: "Syntax error: missing ';'"},
	}}

	got, err := NewCollector(checker).Collect("<?php $x = ;")

	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	want := []Diagnostic{
		{Start: 1, End: 2, Message: "Syntax error: unexpected '='"},
		{Start: 5, End: 5, Message: "Syntax error: missing ';'"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Collect = %+v, want %+v", got, want)
	}
	if checker.got != "<?php $x = ;" {
		t.Errorf("checker saw %q", checker.got)
	}
}

func TestCollectReturnsCheckerFailure(t *testing.T) {
	boom := errors.New("boom")

	_, err := NewCollector(&fakeChecker{err: boom}).Collect("<?php")

	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}
