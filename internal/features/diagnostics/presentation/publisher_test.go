package presentation

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"go.lsp.dev/protocol"

	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
	"github.com/amavis442/php-lsp-go/internal/features/diagnostics/domain"
)

const docURI = "file:///D:/work/a.php"

type published struct {
	method string
	params protocol.PublishDiagnosticsParams
}

type fakeNotifier chan published

func (f fakeNotifier) Notify(_ context.Context, method string, params any) error {
	f <- published{method, params.(protocol.PublishDiagnosticsParams)}
	return nil
}

// errorAtDollar reports one syntax error on the first "$" in the source, so
// tests control where the diagnostic lands.
type errorAtDollar struct{}

func (errorAtDollar) SyntaxErrors(src []byte) ([]php.SyntaxError, error) {
	for i, b := range src {
		if b == '$' {
			return []php.SyntaxError{{Start: i, End: i + 1, Message: "bad"}}, nil
		}
	}
	return nil, nil
}

func newPublisher(t *testing.T, delay time.Duration) (*Publisher, fakeNotifier) {
	t.Helper()
	n := make(fakeNotifier, 10)
	p := NewPublisher(domain.NewCollector(errorAtDollar{}), n, func() text.Encoding { return text.UTF16 },
		delay, log.New(io.Discard, "", 0))
	t.Cleanup(p.Stop)
	return p, n
}

func next(t *testing.T, n fakeNotifier) published {
	t.Helper()
	select {
	case got := <-n:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("no publishDiagnostics within 2s")
		return published{}
	}
}

func expectNothing(t *testing.T, n fakeNotifier, wait time.Duration) {
	t.Helper()
	select {
	case got := <-n:
		t.Fatalf("unexpected publish: %+v", got)
	case <-time.After(wait):
	}
}

func TestPublishesDiagnosticsInTheNegotiatedEncoding(t *testing.T) {
	p, n := newPublisher(t, time.Millisecond)

	// The emoji is two UTF-16 units, so "$" sits at character 4 on line 1.
	p.DocumentChanged(docURI, 7, "<?php\n😀 =$")

	got := next(t, n)
	if got.method != "textDocument/publishDiagnostics" {
		t.Errorf("method = %q", got.method)
	}
	if string(got.params.URI) != docURI {
		t.Errorf("uri = %q, want %q", got.params.URI, docURI)
	}
	if v, ok := got.params.Version.Get(); !ok || v != 7 {
		t.Errorf("version = %v, %v; want 7", v, ok)
	}
	if len(got.params.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want one", got.params.Diagnostics)
	}
	d := got.params.Diagnostics[0]
	wantRange := protocol.Range{Start: protocol.Position{Line: 1, Character: 4}, End: protocol.Position{Line: 1, Character: 5}}
	if d.Range != wantRange {
		t.Errorf("range = %+v, want %+v", d.Range, wantRange)
	}
	if d.Severity != protocol.DiagnosticSeverityError || d.Message != protocol.String("bad") {
		t.Errorf("diagnostic = %+v", d)
	}
	if src, _ := d.Source.Get(); src != "php-lsp-go" {
		t.Errorf("source = %q", src)
	}
}

func TestValidDocumentPublishesAnEmptyList(t *testing.T) {
	p, n := newPublisher(t, time.Millisecond)

	p.DocumentChanged(docURI, 1, "<?php echo 1;")

	got := next(t, n)
	if got.params.Diagnostics == nil || len(got.params.Diagnostics) != 0 {
		t.Errorf("diagnostics = %#v, want an empty, non-nil list", got.params.Diagnostics)
	}
}

func TestRapidChangesPublishOnlyTheLastVersion(t *testing.T) {
	p, n := newPublisher(t, 50*time.Millisecond)

	p.DocumentChanged(docURI, 1, "<?php $a")
	p.DocumentChanged(docURI, 2, "<?php $b")
	p.DocumentChanged(docURI, 3, "<?php ok")

	got := next(t, n)
	if v, _ := got.params.Version.Get(); v != 3 || len(got.params.Diagnostics) != 0 {
		t.Errorf("published version %d with %d diagnostics, want version 3 with none", v, len(got.params.Diagnostics))
	}
	expectNothing(t, n, 150*time.Millisecond)
}

func TestCloseClearsDiagnosticsAndCancelsThePendingPublish(t *testing.T) {
	p, n := newPublisher(t, 50*time.Millisecond)
	p.DocumentChanged(docURI, 1, "<?php $a")

	p.DocumentClosed(docURI)

	got := next(t, n)
	if string(got.params.URI) != docURI || got.params.Diagnostics == nil || len(got.params.Diagnostics) != 0 {
		t.Errorf("close published %+v, want an empty list for %s", got.params, docURI)
	}
	if _, ok := got.params.Version.Get(); ok {
		t.Error("close published a version, want none")
	}
	expectNothing(t, n, 150*time.Millisecond)
}

func TestStopCancelsPendingPublishes(t *testing.T) {
	p, n := newPublisher(t, 50*time.Millisecond)
	p.DocumentChanged(docURI, 1, "<?php $a")

	p.Stop()
	p.DocumentChanged(docURI, 2, "<?php $b")

	expectNothing(t, n, 150*time.Millisecond)
}
