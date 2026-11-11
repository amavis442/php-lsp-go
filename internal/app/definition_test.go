package app

import (
	"encoding/json"
	"io"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/amavis442/php-lsp-go/internal/core/fileuri"
	"github.com/amavis442/php-lsp-go/internal/core/syntax"
	idxdata "github.com/amavis442/php-lsp-go/internal/features/indexing/data"
	"github.com/amavis442/php-lsp-go/internal/server"
	"github.com/amavis442/php-lsp-go/internal/testsupport/lspclient"
)

type definitionLocation struct {
	URI   string `json:"uri"`
	Range struct {
		Start struct{ Line, Character int } `json:"start"`
	} `json:"range"`
}

// definitionAt asks for the definition of the name at the first § in
// marked, a line of src.
func definitionAt(t *testing.T, c *lspclient.Client, uri, src, marked string) []definitionLocation {
	t.Helper()
	at := strings.Index(src, strings.ReplaceAll(marked, "§", ""))
	if at < 0 {
		t.Fatalf("%q not in source", marked)
	}
	offset := at + strings.Index(marked, "§")
	line := strings.Count(src[:offset], "\n")
	character := offset - (strings.LastIndex(src[:offset], "\n") + 1) // ASCII only, so bytes are units
	resp, err := c.Call("textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line, "character": character},
	})
	if err != nil || resp.Error != nil {
		t.Fatalf("definition: err=%v resp.Error=%+v", err, resp.Error)
	}
	var got []definitionLocation
	if err := json.Unmarshal(resp.Result, &got); err != nil {
		t.Fatalf("decoding %s: %v", resp.Result, err)
	}
	return got
}

func TestDefinitionsAcrossTheWorkspaceTheStubsAndTheOpenDocument(t *testing.T) {
	root := t.TempDir()
	writePHP(t, filepath.Join(root, "src", "Models", "User.php"),
		"<?php\nnamespace App\\Models;\n\nclass User extends Model\n{\n}\n")
	writePHP(t, filepath.Join(root, "src", "Models", "Model.php"),
		"<?php\nnamespace App\\Models;\n\nabstract class Model\n{\n    use HasTimestamps;\n}\n")
	writePHP(t, filepath.Join(root, "src", "Models", "HasTimestamps.php"),
		"<?php\nnamespace App\\Models;\n\ntrait HasTimestamps\n{\n    public function touch(): void {}\n}\n")
	stubs := idxdata.NewStubs(fstest.MapFS{
		"Core/Core.php": {Data: []byte("<?php\nfunction strlen(string $string): int {}\n")},
	}, "test", t.TempDir())
	parser := syntax.NewParser()
	c, _ := startAppWith(t, newApp(server.Info{Name: "php-lsp-go", Version: "test"}, log.New(io.Discard, "", 0),
		time.Millisecond, parser, parser, stubs),
		map[string]any{
			"processId": nil, "rootUri": fileuri.FromPath(root),
			"capabilities": map[string]any{"window": map[string]any{"workDoneProgress": true}},
		})
	waitIndexed(t, c)

	// The open document is not saved: its own class only exists in the
	// editor.
	src := "<?php\nnamespace App\\Http;\n\nuse App\\Models\\User;\n\nclass Controller\n{\n" +
		"    public function show(User $user): int\n    {\n        $user->touch();\n" +
		"        User::touch();\n        $this->show($user);\n        return strlen('x');\n    }\n}\n"
	uri := fileuri.FromPath(filepath.Join(root, "src", "Http", "Controller.php"))
	notify(t, c, "textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": uri, "languageId": "php", "version": 1, "text": src,
	}})

	userURI := fileuri.FromPath(filepath.Join(root, "src", "Models", "User.php"))
	if got := definitionAt(t, c, uri, src, "show(Us§er $user"); len(got) != 1 || got[0].URI != userURI ||
		got[0].Range.Start.Line != 3 || got[0].Range.Start.Character != 6 {
		t.Errorf("User: %+v, want User.php 3:6", got)
	}
	if got := definitionAt(t, c, uri, src, "use App\\Models\\Us§er;"); len(got) != 1 || got[0].URI != userURI {
		t.Errorf("use statement: %+v", got)
	}
	if got := definitionAt(t, c, uri, src, "$this->sh§ow"); len(got) != 1 || got[0].URI != uri || got[0].Range.Start.Line != 7 {
		t.Errorf("$this->show: %+v, want the open document line 7", got)
	}
	if got := definitionAt(t, c, uri, src, "return str§len"); len(got) != 1 || !strings.HasSuffix(got[0].URI, "/stubs/test/Core/Core.php") {
		t.Errorf("strlen: %+v, want the stub", got)
	}
	traitURI := fileuri.FromPath(filepath.Join(root, "src", "Models", "HasTimestamps.php"))
	if got := definitionAt(t, c, uri, src, "User::tou§ch"); len(got) != 1 || got[0].URI != traitURI || got[0].Range.Start.Line != 5 {
		t.Errorf("User::touch: %+v, want the trait of the parent class, line 5", got)
	}
	if got := definitionAt(t, c, uri, src, "$user->tou§ch"); len(got) != 0 {
		t.Errorf("$user->touch needs types: %+v, want nothing", got)
	}
}

func TestEnumMethodsComeFromTheBuiltInEnumInterfaces(t *testing.T) {
	root := t.TempDir()
	writePHP(t, filepath.Join(root, "src", "Level.php"),
		"<?php\nnamespace App;\n\nenum Level: int\n{\n    case Low = 1;\n}\n")
	stubs := idxdata.NewStubs(fstest.MapFS{
		"Core/Core_c.php": {Data: []byte("<?php\ninterface UnitEnum\n{\n    public static function cases(): array;\n}\n" +
			"interface BackedEnum extends UnitEnum\n{\n    public static function from(int|string $value): static;\n}\n")},
	}, "test", t.TempDir())
	parser := syntax.NewParser()
	c, _ := startAppWith(t, newApp(server.Info{Name: "php-lsp-go", Version: "test"}, log.New(io.Discard, "", 0),
		time.Millisecond, parser, parser, stubs),
		map[string]any{
			"processId": nil, "rootUri": fileuri.FromPath(root),
			"capabilities": map[string]any{"window": map[string]any{"workDoneProgress": true}},
		})
	waitIndexed(t, c)

	src := "<?php\nnamespace App;\n\n$level = Level::from(1);\n$all = Level::cases();\n"
	uri := fileuri.FromPath(filepath.Join(root, "src", "use.php"))
	notify(t, c, "textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": uri, "languageId": "php", "version": 1, "text": src,
	}})

	stub := "/stubs/test/Core/Core_c.php"
	if got := definitionAt(t, c, uri, src, "Level::fr§om"); len(got) != 1 || !strings.HasSuffix(got[0].URI, stub) ||
		got[0].Range.Start.Line != 7 {
		t.Errorf("Level::from: %+v, want BackedEnum::from in the stub, line 7", got)
	}
	if got := definitionAt(t, c, uri, src, "Level::cas§es"); len(got) != 1 || !strings.HasSuffix(got[0].URI, stub) ||
		got[0].Range.Start.Line != 3 {
		t.Errorf("Level::cases: %+v, want UnitEnum::cases through BackedEnum, line 3", got)
	}
}
