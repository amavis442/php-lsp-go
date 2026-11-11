package domain

import (
	"strings"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
)

const docURI = "file:///D:/app/Controller.php"

type fakeDocs map[string]string

func (d fakeDocs) Text(uri string) (string, bool) {
	t, ok := d[uri]
	return t, ok
}

// fakeParser answers with a fixed reference and outline, and records the
// offset it was asked about.
type fakeParser struct {
	ref     php.Reference
	ok      bool
	decls   []php.Declaration
	offset  int
	asked   bool
	refErr  error
	summary error
}

func (p *fakeParser) ReferenceAt(_ []byte, offset int) (php.Reference, bool, error) {
	p.offset, p.asked = offset, true
	return p.ref, p.ok, p.refErr
}

func (p *fakeParser) Extract([]byte) (php.FileSummary, error) {
	return php.FileSummary{Declarations: p.decls}, p.summary
}

type fakeIndex map[string][]php.WorkspaceSymbol

func (ix fakeIndex) add(symbols ...php.WorkspaceSymbol) fakeIndex {
	for _, s := range symbols {
		ix[php.KeyOf(s)] = append(ix[php.KeyOf(s)], s)
	}
	return ix
}

func (ix fakeIndex) Lookup(key string) []php.WorkspaceSymbol { return ix[key] }

func class(container, name, uri string, extends, implements, traits []string) php.WorkspaceSymbol {
	return php.WorkspaceSymbol{Name: name, Kind: php.KindClass, Container: container, URI: uri,
		Extends: extends, Implements: implements, Traits: traits}
}

func member(class, name string, kind php.SymbolKind, uri string) php.WorkspaceSymbol {
	return php.WorkspaceSymbol{Name: name, Kind: kind, Container: class, Member: true, URI: uri}
}

func found(symbols []php.WorkspaceSymbol) string {
	var out []string
	for _, s := range symbols {
		out = append(out, s.Container+" "+s.Name+" @"+s.URI)
	}
	return strings.Join(out, ", ")
}

func definitionsOf(t *testing.T, parser *fakeParser, index fakeIndex) string {
	t.Helper()
	d := NewDefinitions(fakeDocs{docURI: "<?php\n"}, parser, index)
	got, err := d.At(docURI, text.Position{}, text.UTF8)
	if err != nil {
		t.Fatalf("At: %v", err)
	}
	return found(got)
}

func TestAClassIsFoundInTheIndex(t *testing.T) {
	parser := &fakeParser{ok: true, ref: php.Reference{Kind: php.RefClass, Candidates: []string{`app\models\USER`}}}
	index := fakeIndex{}.add(class(`App\Models`, "User", "file:///D:/app/User.php", nil, nil, nil))

	if got := definitionsOf(t, parser, index); got != `App\Models User @file:///D:/app/User.php` {
		t.Errorf("found %q", got)
	}
}

func TestFunctionsAndConstantsTryTheNamespaceFirst(t *testing.T) {
	global := php.WorkspaceSymbol{Name: "strlen", Kind: php.KindFunction, URI: "file:///stubs/Core.php"}
	local := php.WorkspaceSymbol{Name: "strlen", Kind: php.KindFunction, Container: "App", URI: "file:///D:/app/f.php"}
	parser := &fakeParser{ok: true, ref: php.Reference{Kind: php.RefFunction, Candidates: []string{`App\strlen`, "strlen"}}}

	if got := definitionsOf(t, parser, fakeIndex{}.add(global)); got != " strlen @file:///stubs/Core.php" {
		t.Errorf("without a namespaced function: %q", got)
	}
	if got := definitionsOf(t, parser, fakeIndex{}.add(global, local)); got != `App strlen @file:///D:/app/f.php` {
		t.Errorf("with a namespaced function: %q", got)
	}

	eol := php.WorkspaceSymbol{Name: "PHP_EOL", Kind: php.KindConstant, URI: "file:///stubs/Core_d.php"}
	parser.ref = php.Reference{Kind: php.RefConstant, Candidates: []string{`App\PHP_EOL`, "PHP_EOL"}}
	if got := definitionsOf(t, parser, fakeIndex{}.add(eol)); got != " PHP_EOL @file:///stubs/Core_d.php" {
		t.Errorf("constant: %q", got)
	}
}

func TestMembersAreFoundUpTheInheritanceChain(t *testing.T) {
	const f = "file:///D:/app/x.php"
	index := fakeIndex{}.add(
		class("", "Child", f, []string{"Base"}, []string{"Contract"}, []string{"Helps"}),
		class("", "Base", f, []string{"Root"}, nil, nil),
		class("", "Root", f, nil, nil, nil),
		php.WorkspaceSymbol{Name: "Contract", Kind: php.KindInterface, URI: f, Extends: []string{"Wider"}},
		php.WorkspaceSymbol{Name: "Wider", Kind: php.KindInterface, URI: f},
		php.WorkspaceSymbol{Name: "Helps", Kind: php.KindTrait, URI: f},
		member("Child", "own", php.KindMethod, f),
		member("Helps", "helped", php.KindMethod, f),
		member("Helps", "shared", php.KindMethod, f),
		member("Base", "shared", php.KindMethod, f),
		member("Root", "$deep", php.KindProperty, f),
		member("Wider", "VERSION", php.KindConstant, f),
	)
	tests := []struct {
		name string
		kind php.SymbolKind
		want string
	}{
		{"OWN", php.KindMethod, "Child own"},           // method names ignore case
		{"helped", php.KindMethod, "Helps helped"},     // from a trait
		{"shared", php.KindMethod, "Helps shared"},     // a trait comes before the parent
		{"deep", php.KindProperty, "Root $deep"},       // two parents up
		{"VERSION", php.KindConstant, "Wider VERSION"}, // through interfaces
		{"missing", php.KindMethod, ""},
	}
	for _, tt := range tests {
		parser := &fakeParser{ok: true, ref: php.Reference{
			Kind: php.RefMember, Candidates: []string{"Child"}, Member: tt.name, MemberKind: tt.kind,
		}}
		got := definitionsOf(t, parser, index)
		if want := tt.want; want != "" {
			want += " @" + f
			if got != want {
				t.Errorf("%s: found %q, want %q", tt.name, got, want)
			}
		} else if got != "" {
			t.Errorf("%s: found %q, want nothing", tt.name, got)
		}
	}
}

func TestInheritanceCyclesEnd(t *testing.T) {
	index := fakeIndex{}.add(
		class("", "A", "file:///a.php", []string{"B"}, nil, nil),
		class("", "B", "file:///b.php", []string{"A"}, nil, nil),
	)
	parser := &fakeParser{ok: true, ref: php.Reference{Kind: php.RefMember, Candidates: []string{"A"}, Member: "x", MemberKind: php.KindMethod}}

	if got := definitionsOf(t, parser, index); got != "" {
		t.Errorf("found %q, want nothing", got)
	}
}

func TestTheOpenDocumentWinsOverTheIndex(t *testing.T) {
	// The index holds an older version of the open document, under its
	// canonical URI; the document itself declares the class elsewhere now.
	stale := class("App", "Controller", "file:///D:/app/Controller.php", nil, nil, nil)
	stale.SelectionRange = text.Range{Start: text.Position{Line: 9}}
	other := class("App", "Controller", "file:///D:/app/Other.php", nil, nil, nil)
	parser := &fakeParser{
		ok:  true,
		ref: php.Reference{Kind: php.RefClass, Candidates: []string{`App\Controller`}},
		decls: []php.Declaration{{Name: "App", Kind: php.KindNamespace, Children: []php.Declaration{
			{Name: "Controller", Kind: php.KindClass, Start: 6, End: 7, NameStart: 6, NameEnd: 7},
		}}},
	}
	// The same file spelled differently: canonical URIs decode escapes on
	// every platform.
	const editorURI = "file:///D%3A/app/Controller.php"
	d := NewDefinitions(fakeDocs{editorURI: "<?php\nclass Controller {}\n"}, parser, fakeIndex{}.add(stale, other))

	got, err := d.At(editorURI, text.Position{}, text.UTF8)

	if err != nil || len(got) != 2 {
		t.Fatalf("At = %+v, %v; want the document's class and the other file's", got, err)
	}
	if got[0].URI != editorURI || got[0].SelectionRange.Start.Line != 1 || got[1].URI != "file:///D:/app/Other.php" {
		t.Errorf("found %s, want the open document first (line 1), then Other.php", found(got))
	}
}

func TestThePositionIsConvertedWithTheEncoding(t *testing.T) {
	parser := &fakeParser{}
	d := NewDefinitions(fakeDocs{docURI: "<?php\n$a = '😀'; x();\n"}, parser, fakeIndex{})

	// "$a = '" is 6 units, the emoji 2 in UTF-16 but 4 bytes, "'; " 3.
	if _, err := d.At(docURI, text.Position{Line: 1, Character: 11}, text.UTF16); err != nil {
		t.Fatal(err)
	}

	if want := 6 + len("$a = '😀'; "); parser.offset != want {
		t.Errorf("offset = %d, want %d", parser.offset, want)
	}
}

func TestNothingIsFoundForAClosedDocumentOrWithoutAName(t *testing.T) {
	parser := &fakeParser{}
	d := NewDefinitions(fakeDocs{docURI: "<?php\n"}, parser, fakeIndex{})

	if got, err := d.At("file:///closed.php", text.Position{}, text.UTF8); err != nil || got != nil || parser.asked {
		t.Errorf("closed document: %+v, %v, parsed = %v", got, err, parser.asked)
	}
	if got, err := d.At(docURI, text.Position{}, text.UTF8); err != nil || got != nil {
		t.Errorf("no name: %+v, %v", got, err)
	}
}
