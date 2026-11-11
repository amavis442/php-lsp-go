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

type fakeIndex []php.WorkspaceSymbol

func (f fakeIndex) Each(visit func(php.WorkspaceSymbol) bool) {
	for _, s := range f {
		if !visit(s) {
			return
		}
	}
}

func searchWorkspace(t *testing.T, index fakeIndex, query string) []protocol.SymbolInformation {
	t.Helper()
	r := server.NewRouter(log.New(io.Discard, "", 0))
	NewWorkspaceHandlers(domain.NewWorkspaceSymbols(index, 10)).Register(r)

	params, _ := json.Marshal(map[string]string{"query": query})
	result, err := r.Handle(context.Background(), "workspace/symbol", params, true)
	if err != nil {
		t.Fatalf("workspace/symbol: %v", err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encoding %v: %v", result, err)
	}
	var got []protocol.SymbolInformation
	if err := json.Unmarshal(raw, &got); err != nil || got == nil {
		t.Fatalf("decoding %s: %v (want a list)", raw, err)
	}
	return got
}

func TestWorkspaceSymbolsAnswerWithLocationsAndContainers(t *testing.T) {
	index := fakeIndex{{
		Name: "User", Kind: php.KindClass, Container: `App\Models`, URI: "file:///D:/work/User.php",
		Range:          text.Range{Start: text.Position{Line: 2, Character: 0}, End: text.Position{Line: 9, Character: 1}},
		SelectionRange: text.Range{Start: text.Position{Line: 3, Character: 6}, End: text.Position{Line: 3, Character: 10}},
	}}

	got := searchWorkspace(t, index, "user")

	if len(got) != 1 {
		t.Fatalf("symbols = %+v, want one", got)
	}
	s := got[0]
	if s.Name != "User" || s.Kind != protocol.SymbolKindClass || s.ContainerName == nil || *s.ContainerName != `App\Models` {
		t.Errorf("symbol = %+v", s)
	}
	if s.Location.Range != rng(2, 0, 9, 1) {
		t.Errorf("location = %+v, want the whole declaration", s.Location)
	}
}

// The protocol library rewrites URIs while decoding, so the exact URI on
// the wire is checked in plain JSON.
func TestWorkspaceSymbolsKeepTheIndexedURIExactly(t *testing.T) {
	r := server.NewRouter(log.New(io.Discard, "", 0))
	index := fakeIndex{{Name: "User", Kind: php.KindClass, URI: "file:///D:/work/User.php"}}
	NewWorkspaceHandlers(domain.NewWorkspaceSymbols(index, 10)).Register(r)

	result, err := r.Handle(context.Background(), "workspace/symbol", []byte(`{"query":"user"}`), true)
	if err != nil {
		t.Fatalf("workspace/symbol: %v", err)
	}
	raw, _ := json.Marshal(result)
	var got []struct {
		Location struct {
			URI string `json:"uri"`
		} `json:"location"`
	}
	if err := json.Unmarshal(raw, &got); err != nil || len(got) != 1 || got[0].Location.URI != "file:///D:/work/User.php" {
		t.Errorf("result = %s, want the URI unchanged", raw)
	}
}

func TestWorkspaceSymbolsLeaveOutAnEmptyContainer(t *testing.T) {
	got := searchWorkspace(t, fakeIndex{{Name: "helper", Kind: php.KindFunction, URI: "file:///a.php"}}, "help")

	if len(got) != 1 || got[0].ContainerName != nil {
		t.Errorf("symbols = %+v, want helper without a container", got)
	}
}

func TestWorkspaceSymbolsWithoutMatchesAreAnEmptyList(t *testing.T) {
	if got := searchWorkspace(t, fakeIndex{}, "x"); len(got) != 0 {
		t.Errorf("symbols = %+v, want none", got)
	}
}

func TestWorkspaceCapabilitiesAnnounceWorkspaceSymbols(t *testing.T) {
	var c protocol.ServerCapabilities

	NewWorkspaceHandlers(nil).Capabilities(&c)

	if c.WorkspaceSymbolProvider != protocol.Boolean(true) {
		t.Errorf("workspaceSymbolProvider = %#v, want true", c.WorkspaceSymbolProvider)
	}
}
