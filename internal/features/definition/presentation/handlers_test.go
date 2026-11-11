package presentation

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/resolve"
	"github.com/amavis442/php-lsp-go/internal/core/text"
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

// userClass makes every position a reference to class App\User.
type userClass struct{ offset int }

func (p *userClass) ReferenceAt(_ []byte, offset int) (php.Reference, bool, error) {
	p.offset = offset
	return php.Reference{Kind: php.RefClass, Candidates: []string{`App\User`}}, true, nil
}

func (*userClass) Extract([]byte) (php.FileSummary, error) { return php.FileSummary{}, nil }

type fakeIndex []php.WorkspaceSymbol

func (ix fakeIndex) Lookup(key string) []php.WorkspaceSymbol {
	var out []php.WorkspaceSymbol
	for _, s := range ix {
		if php.KeyOf(s) == key {
			out = append(out, s)
		}
	}
	return out
}

type gotLocation struct {
	URI   string `json:"uri"`
	Range struct {
		Start struct{ Line, Character int } `json:"start"`
		End   struct{ Line, Character int } `json:"end"`
	} `json:"range"`
}

func definition(t *testing.T, index fakeIndex, uri string, line, character int) ([]gotLocation, *userClass) {
	t.Helper()
	parser := &userClass{}
	r := server.NewRouter(log.New(io.Discard, "", 0))
	h := NewHandlers(resolve.NewResolver(fakeDocs{docURI: "<?php\nnew User();\n"}, parser, index),
		func() text.Encoding { return text.UTF8 })
	h.Register(r)
	params, _ := json.Marshal(map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line, "character": character},
	})
	result, err := r.Handle(context.Background(), protocol.MethodTextDocumentDefinition, params, true)
	if err != nil {
		t.Fatalf("definition: %v", err)
	}
	raw, _ := json.Marshal(result)
	var got []gotLocation
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
	return got, parser
}

func TestDefinitionAnswersWithTheNameOfEachDeclaration(t *testing.T) {
	user := php.WorkspaceSymbol{
		Name: "User", Kind: php.KindClass, Container: "App", URI: "file:///D:/work/User.php",
		Range:          text.Range{Start: text.Position{Line: 2}, End: text.Position{Line: 9, Character: 1}},
		SelectionRange: text.Range{Start: text.Position{Line: 2, Character: 6}, End: text.Position{Line: 2, Character: 10}},
	}

	got, parser := definition(t, fakeIndex{user}, docURI, 1, 5)

	if len(got) != 1 || got[0].URI != "file:///D:/work/User.php" ||
		got[0].Range.Start.Line != 2 || got[0].Range.Start.Character != 6 || got[0].Range.End.Character != 10 {
		t.Errorf("definition = %+v, want the class name in User.php", got)
	}
	if parser.offset != len("<?php\nnew U") {
		t.Errorf("asked at offset %d", parser.offset)
	}
}

func TestDefinitionOfNothingIsAnEmptyList(t *testing.T) {
	got, _ := definition(t, fakeIndex{}, docURI, 1, 5)

	if got == nil || len(got) != 0 {
		t.Errorf("definition = %#v, want an empty list", got)
	}
}

func TestDefinitionAnnouncesItself(t *testing.T) {
	var c protocol.ServerCapabilities
	NewHandlers(nil, nil).Capabilities(&c)

	if c.DefinitionProvider == nil {
		t.Error("definitionProvider not set")
	}
}
