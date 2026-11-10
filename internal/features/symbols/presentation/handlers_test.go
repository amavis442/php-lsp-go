package presentation

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
	"github.com/amavis442/php-lsp-go/internal/features/symbols/domain"
	"github.com/amavis442/php-lsp-go/internal/server"
)

// A Windows URI: the protocol library would rewrite it while decoding, and
// then the lookup would miss the open document.
const docURI = "file:///D:/work/a.php"

type fakeDocs map[string]string

func (f fakeDocs) Text(uri string) (string, bool) {
	t, ok := f[uri]
	return t, ok
}

// classWithMethod declares class A (spanning the whole text) with method run.
type classWithMethod struct{}

func (classWithMethod) Extract(src []byte) (php.FileSummary, error) {
	return php.FileSummary{Declarations: []php.Declaration{{
		Name: "A", Kind: php.KindClass, Start: 6, End: len(src), NameStart: 12, NameEnd: 13,
		Children: []php.Declaration{{
			Name: "run", Kind: php.KindMethod, Start: 19, End: 36, NameStart: 28, NameEnd: 31,
		}},
	}}}, nil
}

// The emoji is two UTF-16 units, so UTF-16 positions after it differ from
// byte positions.
const source = "<?php\nclass A {😀function run() {}\n}"

func call(t *testing.T, docs fakeDocs, enc text.Encoding, uri string) []protocol.DocumentSymbol {
	t.Helper()
	r := server.NewRouter(log.New(io.Discard, "", 0))
	symbols := domain.NewDocumentSymbols(docs, classWithMethod{})
	NewHandlers(symbols, func() text.Encoding { return enc }).Register(r)

	result, err := r.Handle(context.Background(), "textDocument/documentSymbol",
		[]byte(`{"textDocument":{"uri":"`+uri+`"}}`), true)
	if err != nil {
		t.Fatalf("documentSymbol: %v", err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encoding %v: %v", result, err)
	}
	var got []protocol.DocumentSymbol
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
	if got == nil {
		t.Fatalf("result %s, want a list", raw)
	}
	return got
}

func rng(sl, sc, el, ec uint32) protocol.Range {
	return protocol.Range{Start: protocol.Position{Line: sl, Character: sc}, End: protocol.Position{Line: el, Character: ec}}
}

func TestDocumentSymbolsAreAHierarchyInTheNegotiatedEncoding(t *testing.T) {
	got := call(t, fakeDocs{docURI: source}, text.UTF16, docURI)

	if len(got) != 1 || len(got[0].Children) != 1 {
		t.Fatalf("symbols = %+v, want class A with one child", got)
	}
	class, method := got[0], got[0].Children[0]
	if class.Name != "A" || class.Kind != protocol.SymbolKindClass ||
		class.Range != rng(1, 0, 2, 1) || class.SelectionRange != rng(1, 6, 1, 7) {
		t.Errorf("class = %+v", class)
	}
	// "class A {" is 9 units, the emoji 2 more: the method starts at 11.
	if method.Name != "run" || method.Kind != protocol.SymbolKindMethod ||
		method.Range != rng(1, 11, 1, 28) || method.SelectionRange != rng(1, 20, 1, 23) {
		t.Errorf("method = %+v", method)
	}
}

func TestUnknownDocumentHasNoSymbols(t *testing.T) {
	if got := call(t, fakeDocs{}, text.UTF16, docURI); len(got) != 0 {
		t.Errorf("symbols = %+v, want none", got)
	}
}

func TestSymbolKindsMapToLSP(t *testing.T) {
	want := map[php.SymbolKind]protocol.SymbolKind{
		php.KindNamespace:   protocol.SymbolKindNamespace,
		php.KindClass:       protocol.SymbolKindClass,
		php.KindInterface:   protocol.SymbolKindInterface,
		php.KindTrait:       protocol.SymbolKindClass,
		php.KindEnum:        protocol.SymbolKindEnum,
		php.KindEnumCase:    protocol.SymbolKindEnumMember,
		php.KindFunction:    protocol.SymbolKindFunction,
		php.KindMethod:      protocol.SymbolKindMethod,
		php.KindConstructor: protocol.SymbolKindConstructor,
		php.KindProperty:    protocol.SymbolKindProperty,
		php.KindConstant:    protocol.SymbolKindConstant,
	}
	for kind, lsp := range want {
		if got := toKind(kind); got != lsp {
			t.Errorf("toKind(%d) = %d, want %d", kind, got, lsp)
		}
	}
}

func TestCapabilitiesAnnounceDocumentSymbols(t *testing.T) {
	var c protocol.ServerCapabilities

	NewHandlers(nil, nil).Capabilities(&c)

	if c.DocumentSymbolProvider != protocol.Boolean(true) {
		t.Errorf("documentSymbolProvider = %#v, want true", c.DocumentSymbolProvider)
	}
}
