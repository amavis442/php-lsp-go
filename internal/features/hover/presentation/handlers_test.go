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
	"github.com/amavis442/php-lsp-go/internal/features/hover/domain"
	"github.com/amavis442/php-lsp-go/internal/server"
)

// A Windows URI: the protocol library would rewrite it while decoding, and
// then the lookup would miss the open document.
const docURI = "file:///D:/work/a.php"

type fakeResolver struct {
	found []php.WorkspaceSymbol
	uri   string
	pos   text.Position
}

func (r *fakeResolver) At(uri string, pos text.Position, _ text.Encoding) ([]php.WorkspaceSymbol, error) {
	r.uri, r.pos = uri, pos
	return r.found, nil
}

type fakeDocs map[string]string

func (d fakeDocs) Text(uri string) (string, bool) {
	t, ok := d[uri]
	return t, ok
}

type noFiles struct{}

func (noFiles) Read(string) ([]byte, error) { return nil, io.EOF }

type fixedDescriber struct{ d php.Description }

func (f fixedDescriber) Describe([]byte, int) (php.Description, bool, error) { return f.d, true, nil }

func hover(t *testing.T, found []php.WorkspaceSymbol, d php.Description) (json.RawMessage, *fakeResolver) {
	t.Helper()
	resolver := &fakeResolver{found: found}
	r := server.NewRouter(log.New(io.Discard, "", 0))
	NewHandlers(domain.NewHover(resolver, fakeDocs{docURI: "<?php\n"}, noFiles{}, fixedDescriber{d}),
		func() text.Encoding { return text.UTF8 }).Register(r)
	params, _ := json.Marshal(map[string]any{
		"textDocument": map[string]any{"uri": docURI},
		"position":     map[string]any{"line": 1, "character": 4},
	})
	result, err := r.Handle(context.Background(), protocol.MethodTextDocumentHover, params, true)
	if err != nil {
		t.Fatalf("hover: %v", err)
	}
	raw, _ := json.Marshal(result)
	return raw, resolver
}

func TestHoverShowsTheSignatureContainerAndSummaryAsMarkdown(t *testing.T) {
	save := php.WorkspaceSymbol{Name: "save", Kind: php.KindMethod, Container: `App\Models\User`, Member: true, URI: docURI}

	raw, resolver := hover(t, []php.WorkspaceSymbol{save},
		php.Description{Signature: "public function save(): bool", Summary: "Saves the user."})

	var got struct {
		Contents struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
	want := "```php\n<?php\npublic function save(): bool\n```\n\n`App\\Models\\User`\n\nSaves the user."
	if got.Contents.Kind != "markdown" || got.Contents.Value != want {
		t.Errorf("contents = %q %q\nwant markdown %q", got.Contents.Kind, got.Contents.Value, want)
	}
	if resolver.uri != docURI || resolver.pos != (text.Position{Line: 1, Character: 4}) {
		t.Errorf("asked about %s %+v", resolver.uri, resolver.pos)
	}
}

func TestHoverLeavesOutWhatIsEmpty(t *testing.T) {
	strlen := php.WorkspaceSymbol{Name: "strlen", Kind: php.KindFunction, URI: docURI}

	raw, _ := hover(t, []php.WorkspaceSymbol{strlen}, php.Description{Signature: "function strlen(string $string): int"})

	if want := `{"contents":{"kind":"markdown","value":"` + "```php\\n\\u003c?php\\nfunction strlen(string $string): int\\n```" + `"}}`; string(raw) != want {
		t.Errorf("hover = %s\nwant %s", raw, want)
	}
}

func TestHoverOfNothingIsNull(t *testing.T) {
	raw, _ := hover(t, nil, php.Description{})

	if string(raw) != "null" {
		t.Errorf("hover = %s, want null", raw)
	}
}

func TestHoverAnnouncesItself(t *testing.T) {
	var c protocol.ServerCapabilities
	NewHandlers(nil, nil).Capabilities(&c)

	if c.HoverProvider == nil {
		t.Error("hoverProvider not set")
	}
}
