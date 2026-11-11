package php

import (
	"strings"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/text"
)

func decl(name string, kind SymbolKind, children ...Declaration) Declaration {
	return Declaration{Name: name, Kind: kind, Children: children}
}

func TestFlattenGivesEachSymbolItsContainer(t *testing.T) {
	decls := []Declaration{
		decl(`App\Models`, KindNamespace,
			decl("User", KindClass,
				decl("NAME", KindConstant),
				decl("__construct", KindConstructor, decl("$id", KindProperty)),
				decl("find", KindMethod, decl("DEFINED_IN_METHOD", KindConstant)),
			),
			decl("helper", KindFunction, decl("inner", KindFunction), decl("DEFINED_IN_FUNCTION", KindConstant)),
			decl("Status", KindEnum, decl("Active", KindEnumCase)),
		),
		decl("global_function", KindFunction),
		decl("GlobalClass", KindClass, decl("run", KindMethod)),
	}

	got := Flatten("file:///a.php", "", decls, text.UTF16)

	var lines []string
	for _, s := range got {
		line := s.Container + " :: " + s.Name
		if s.Member {
			line += " (member)"
		}
		lines = append(lines, line)
	}
	want := []string{
		`App\Models :: User`,
		`App\Models\User :: NAME (member)`,
		`App\Models\User :: __construct (member)`,
		`App\Models\User :: $id (member)`,
		`App\Models\User :: find (member)`,
		`App\Models :: DEFINED_IN_METHOD`, // define makes a global constant
		`App\Models :: helper`,
		`App\Models :: inner`,
		`App\Models :: DEFINED_IN_FUNCTION`,
		`App\Models :: Status`,
		`App\Models\Status :: Active (member)`,
		` :: global_function`,
		` :: GlobalClass`,
		`GlobalClass :: run (member)`,
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
	decls := []Declaration{{Name: "A", Kind: KindClass, Start: 10, End: 20, NameStart: 16, NameEnd: 17}}

	got := Flatten("file:///a.php", src, decls, text.UTF16)

	if len(got) != 1 {
		t.Fatalf("flattened %+v", got)
	}
	wantRange := text.Range{Start: text.Position{Line: 1, Character: 2}, End: text.Position{Line: 1, Character: 12}}
	wantName := text.Range{Start: text.Position{Line: 1, Character: 8}, End: text.Position{Line: 1, Character: 9}}
	if got[0].Range != wantRange || got[0].SelectionRange != wantName || got[0].Kind != KindClass {
		t.Errorf("symbol = %+v, want range %+v and name %+v", got[0], wantRange, wantName)
	}
}

func TestFlattenCopiesWhatAClassBuildsOn(t *testing.T) {
	decls := []Declaration{{
		Name: "User", Kind: KindClass,
		Extends: []string{`App\Model`}, Implements: []string{"Countable"}, Traits: []string{`App\HasName`},
	}}

	got := Flatten("file:///a.php", "", decls, text.UTF8)

	if len(got) != 1 || len(got[0].Extends) != 1 || got[0].Extends[0] != `App\Model` ||
		got[0].Implements[0] != "Countable" || got[0].Traits[0] != `App\HasName` {
		t.Errorf("flattened %+v", got)
	}
}
