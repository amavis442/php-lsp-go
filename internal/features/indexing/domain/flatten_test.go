package domain

import (
	"strings"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
)

func decl(name string, kind php.SymbolKind, children ...php.Declaration) php.Declaration {
	return php.Declaration{Name: name, Kind: kind, Children: children}
}

func TestFlattenGivesEachSymbolItsContainer(t *testing.T) {
	decls := []php.Declaration{
		decl(`App\Models`, php.KindNamespace,
			decl("User", php.KindClass,
				decl("NAME", php.KindConstant),
				decl("__construct", php.KindConstructor, decl("$id", php.KindProperty)),
				decl("find", php.KindMethod),
			),
			decl("helper", php.KindFunction, decl("inner", php.KindFunction)),
			decl("Status", php.KindEnum, decl("Active", php.KindEnumCase)),
		),
		decl("global_function", php.KindFunction),
		decl("GlobalClass", php.KindClass, decl("run", php.KindMethod)),
	}

	got := flatten("file:///a.php", "", decls, text.UTF16)

	var lines []string
	for _, s := range got {
		lines = append(lines, s.Container+" :: "+s.Name)
	}
	want := []string{
		`App\Models :: User`,
		`App\Models\User :: NAME`,
		`App\Models\User :: __construct`,
		`App\Models\User :: $id`,
		`App\Models\User :: find`,
		`App\Models :: helper`,
		`App\Models :: inner`,
		`App\Models :: Status`,
		`App\Models\Status :: Active`,
		` :: global_function`,
		` :: GlobalClass`,
		`GlobalClass :: run`,
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("flattened:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	for _, s := range got {
		if s.URI != "file:///a.php" {
			t.Errorf("%s has uri %q", s.Name, s.URI)
		}
	}
}

func TestFlattenConvertsSpansToEditorPositions(t *testing.T) {
	src := "<?php\n😀class A {}"
	decls := []php.Declaration{{Name: "A", Kind: php.KindClass, Start: 10, End: 20, NameStart: 16, NameEnd: 17}}

	got := flatten("file:///a.php", src, decls, text.UTF16)

	if len(got) != 1 {
		t.Fatalf("flattened %+v", got)
	}
	wantRange := text.Range{Start: text.Position{Line: 1, Character: 2}, End: text.Position{Line: 1, Character: 12}}
	wantName := text.Range{Start: text.Position{Line: 1, Character: 8}, End: text.Position{Line: 1, Character: 9}}
	if got[0].Range != wantRange || got[0].SelectionRange != wantName || got[0].Kind != php.KindClass {
		t.Errorf("symbol = %+v, want range %+v and name %+v", got[0], wantRange, wantName)
	}
}
