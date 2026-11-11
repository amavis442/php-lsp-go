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

// hoverAt asks for the hover of the name at the § in marked, a line of
// src, and returns its Markdown, or "null".
func hoverAt(t *testing.T, c *lspclient.Client, uri, src, marked string) string {
	t.Helper()
	at := strings.Index(src, strings.ReplaceAll(marked, "§", ""))
	if at < 0 {
		t.Fatalf("%q not in source", marked)
	}
	offset := at + strings.Index(marked, "§")
	line := strings.Count(src[:offset], "\n")
	character := offset - (strings.LastIndex(src[:offset], "\n") + 1) // ASCII only
	resp, err := c.Call("textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line, "character": character},
	})
	if err != nil || resp.Error != nil {
		t.Fatalf("hover: err=%v resp.Error=%+v", err, resp.Error)
	}
	if string(resp.Result) == "null" {
		return "null"
	}
	var got struct {
		Contents struct {
			Value string `json:"value"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(resp.Result, &got); err != nil {
		t.Fatalf("decoding %s: %v", resp.Result, err)
	}
	return got.Contents.Value
}

func TestHoverDescribesDeclarationsFromTheWorkspaceTheStubsAndTheOpenDocument(t *testing.T) {
	root := t.TempDir()
	writePHP(t, filepath.Join(root, "src", "Models", "User.php"),
		"<?php\nnamespace App\\Models;\n\n/**\n * A person who can sign in.\n * @property string $email\n */\nclass User\n{\n"+
			"    /** Saves the user. */\n    public function save(array $options = []): bool { return true; }\n}\n")
	stubs := idxdata.NewStubs(fstest.MapFS{
		"Core/Core.php": {Data: []byte("<?php\n/**\n * Get <b>string</b> length\n * @param string $string\n */\nfunction strlen(string $string): int {}\n")},
	}, "test", t.TempDir())
	parser := syntax.NewParser()
	c, _ := startAppWith(t, newApp(server.Info{Name: "php-lsp-go", Version: "test"}, log.New(io.Discard, "", 0),
		time.Millisecond, parser, parser, stubs, noCache),
		map[string]any{
			"processId": nil, "rootUri": fileuri.FromPath(root),
			"capabilities": map[string]any{"window": map[string]any{"workDoneProgress": true}},
		})
	waitIndexed(t, c)

	src := "<?php\nnamespace App\\Http;\n\nuse App\\Models\\User;\n\nclass Controller\n{\n" +
		"    /** Shows one user. */\n    public function show(User $user): int\n    {\n" +
		"        User::save();\n        $this->show($user);\n        return strlen('x') + $count;\n    }\n}\n"
	uri := fileuri.FromPath(filepath.Join(root, "src", "Http", "Controller.php"))
	notify(t, c, "textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": uri, "languageId": "php", "version": 1, "text": src,
	}})

	tests := []struct{ at, want string }{
		{"show(Us§er $user", "```php\n<?php\nclass User\n```\n\n`App\\Models`\n\nA person who can sign in."},
		{"User::sa§ve", "```php\n<?php\npublic function save(array $options = []): bool\n```\n\n`App\\Models\\User`\n\nSaves the user."},
		{"$this->sh§ow", "```php\n<?php\npublic function show(User $user): int\n```\n\n`App\\Http\\Controller`\n\nShows one user."},
		{"return str§len", "```php\n<?php\nfunction strlen(string $string): int\n```\n\nGet **string** length"},
		{"$cou§nt", "null"},
	}
	for _, tt := range tests {
		if got := hoverAt(t, c, uri, src, tt.at); got != tt.want {
			t.Errorf("hover at %q:\n got %q\nwant %q", tt.at, got, tt.want)
		}
	}
}
