package syntax

import (
	"fmt"
	"strings"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/php"
)

// cursor marks the position in a test source; it is removed before
// parsing.
const cursor = "§"

const refSource = `<?php
namespace App\Http;

use App\Models\User;
use App\Models\{Post, Comment as Note};
use function App\Support\helper;
use const App\Support\LIMIT;

class Controller extends Base implements Contract
{
    use Authorizes;

    public const MAX = 10;
    private int $count = 0;

    public function __construct(private Repo $repo) {}

    public function show(User $user, Note $note): ?Post
    {
        $post = new Post();
        User::find(1);
        self::MAX;
        static::create();
        parent::boot();
        Note::$cache;
        $this->count;
        $this?->show($user, $note);
        $this->repo->find();
        $other->count;
        helper();
        strlen('x');
        \strlen('x');
        LIMIT;
        PHP_EOL;
        $user instanceof User;
        try {} catch (\RuntimeException|Failure $e) {}
        User::class;
        named(arg: 1);
        return \App\Models\Post::latest();
    }
}

function make(): Controller {}
enum Status { case Open; }
`

func refAt(t *testing.T, src, marked string) string {
	t.Helper()
	at := strings.Index(src, strings.ReplaceAll(marked, cursor, ""))
	if at < 0 {
		t.Fatalf("%q not in source", marked)
	}
	offset := at + strings.Index(marked, cursor)
	ref, ok, err := NewParser().ReferenceAt([]byte(src), offset)
	if err != nil {
		t.Fatalf("ReferenceAt: %v", err)
	}
	if !ok {
		return "none"
	}
	name := src[ref.Start:ref.End]
	switch ref.Kind {
	case php.RefClass:
		return fmt.Sprintf("class %v (%s)", ref.Candidates, name)
	case php.RefFunction:
		return fmt.Sprintf("function %v (%s)", ref.Candidates, name)
	case php.RefConstant:
		return fmt.Sprintf("constant %v (%s)", ref.Candidates, name)
	case php.RefMember:
		kinds := map[php.SymbolKind]string{php.KindMethod: "method", php.KindProperty: "property", php.KindConstant: "constant"}
		return fmt.Sprintf("%s %v::%s (%s)", kinds[ref.MemberKind], ref.Candidates, ref.Member, name)
	}
	return fmt.Sprintf("kind %d", ref.Kind)
}

func TestReferenceAt(t *testing.T) {
	tests := []struct{ at, want string }{
		// Class names, resolved through the imports and the namespace.
		{"new P§ost()", `class [App\Models\Post] (Post)`},
		{"show(Us§er $user", `class [App\Models\User] (User)`},
		{"No§te $note", `class [App\Models\Comment] (Note)`},
		{"?Po§st", `class [App\Models\Post] (Post)`},
		{"extends Ba§se", `class [App\Http\Base] (Base)`},
		{"implements Con§tract", `class [App\Http\Contract] (Contract)`},
		{"use Author§izes", `class [App\Http\Authorizes] (Authorizes)`},
		{"private Re§po $repo", `class [App\Http\Repo] (Repo)`},
		{"Us§er::find", `class [App\Models\User] (User)`},
		{"instanceof Us§er", `class [App\Models\User] (User)`},
		{`\Runtime§Exception|`, `class [RuntimeException] (\RuntimeException)`},
		{"|Fail§ure", `class [App\Http\Failure] (Failure)`},
		{"User::cla§ss", `class [App\Models\User] (class)`},
		{`\App\Mo§dels\Post::latest`, `class [App\Models\Post] (\App\Models\Post)`},
		{"make(): Contr§oller", `class [App\Http\Controller] (Controller)`},
		// The use statements themselves.
		{"use App\\Models\\Us§er;", `class [App\Models\User] (App\Models\User)`},
		{"{Post, Comm§ent as", `class [App\Models\Comment] (Comment)`},
		{"use function App\\Support\\hel§per", `function [App\Support\helper] (App\Support\helper)`},
		{"use const App\\Support\\LIM§IT", `constant [App\Support\LIMIT] (App\Support\LIMIT)`},
		// Declarations name themselves.
		{"class Contr§oller", `class [App\Http\Controller] (Controller)`},
		{"function ma§ke", `function [App\Http\make] (make)`},
		{"function sh§ow", `method [App\Http\Controller]::show (show)`},
		{"const MA§X", `constant [App\Http\Controller]::MAX (MAX)`},
		{"int $cou§nt", `property [App\Http\Controller]::$count ($count)`},
		{"Repo $re§po", `property [App\Http\Controller]::$repo ($repo)`},
		{"case Op§en", `constant [App\Http\Status]::Open (Open)`},
		// Functions and constants fall back to the global ones.
		{"hel§per()", `function [App\Support\helper] (helper)`},
		{"strl§en('x');\n        \\", `function [App\Http\strlen strlen] (strlen)`},
		{`\strl§en`, `function [strlen] (\strlen)`},
		{"        LIM§IT;", `constant [App\Support\LIMIT] (LIMIT)`},
		{"PHP_§EOL", `constant [App\Http\PHP_EOL PHP_EOL] (PHP_EOL)`},
		// Members.
		{"User::fi§nd", `method [App\Models\User]::find (find)`},
		{"self::MA§X", `constant [App\Http\Controller]::MAX (MAX)`},
		{"static::cre§ate", `method [App\Http\Controller]::create (create)`},
		{"parent::bo§ot", `method [App\Http\Base]::boot (boot)`},
		{"Note::$cac§he", `property [App\Models\Comment]::$cache ($cache)`},
		{"$this->cou§nt", `property [App\Http\Controller]::count (count)`},
		{"$this?->sh§ow", `method [App\Http\Controller]::show (show)`},
		// Without types, other objects are unknown.
		{"$this->repo->fi§nd", "none"},
		{"$other->cou§nt", "none"},
		// Not a reference.
		{"nam§ed(arg", `function [App\Http\named named] (named)`},
		{"named(ar§g: 1)", "none"},
		{"$po§st = ", "none"},
		{"namespace App\\Ht§tp", "none"},
	}
	for _, tt := range tests {
		t.Run(tt.at, func(t *testing.T) {
			if got := refAt(t, refSource, tt.at); got != tt.want {
				t.Errorf("at %q: got %s, want %s", tt.at, got, tt.want)
			}
		})
	}
}

func TestReferenceAtUsesTheScopeOfItsNamespace(t *testing.T) {
	src := `<?php
namespace A { use X\Thing; new Thing(); }
namespace B { new Thing(); }
`
	if got := refAt(t, src, "; new Thi§ng"); got != `class [X\Thing] (Thing)` {
		t.Errorf("in A: %s", got)
	}
	if got := refAt(t, src, "namespace B { new Thi§ng"); got != `class [B\Thing] (Thing)` {
		t.Errorf("in B: %s", got)
	}
}

func TestReferenceAtOutsideAClassHasNoSelf(t *testing.T) {
	src := "<?php\nfunction f() { self::X; $this->y; }\n$o = new class { function g() { self::Z; } };\n"
	for _, at := range []string{"self::§X", "$this->§y", "self::§Z"} {
		if got := refAt(t, src, at); got != "none" {
			t.Errorf("at %q: got %s, want none", at, got)
		}
	}
}
