package syntax

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/php"
	"github.com/amavis442/php-lsp-go/internal/core/text"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

var kindNames = map[php.SymbolKind]string{
	php.KindNamespace:   "namespace",
	php.KindClass:       "class",
	php.KindInterface:   "interface",
	php.KindTrait:       "trait",
	php.KindEnum:        "enum",
	php.KindEnumCase:    "case",
	php.KindFunction:    "function",
	php.KindMethod:      "method",
	php.KindConstructor: "constructor",
	php.KindProperty:    "property",
	php.KindConstant:    "constant",
}

// outline renders declarations one per line, indented by nesting. With
// src, each line also shows the declaration's span and where its name
// starts, as 1-based line:column (columns count bytes).
func outline(decls []php.Declaration, src string) string {
	var b strings.Builder
	var ix *text.LineIndex
	if src != "" {
		ix = text.NewLineIndex(src)
	}
	pos := func(offset int) string {
		p := ix.Position(offset, text.UTF8)
		return fmt.Sprintf("%d:%d", p.Line+1, p.Character+1)
	}
	var walk func([]php.Declaration, string)
	walk = func(ds []php.Declaration, indent string) {
		for _, d := range ds {
			fmt.Fprintf(&b, "%s%s %s", indent, kindNames[d.Kind], d.Name)
			if ix != nil {
				fmt.Fprintf(&b, " [%s-%s] name@%s", pos(d.Start), pos(d.End), pos(d.NameStart))
			}
			b.WriteString("\n")
			walk(d.Children, indent+"  ")
		}
	}
	walk(decls, "")
	return b.String()
}

func extract(t *testing.T, src string) []php.Declaration {
	t.Helper()
	summary, err := NewParser().Extract([]byte(src))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	return summary.Declarations
}

func TestExtractOutlines(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "class members",
			src: `<?php
final class User extends Base implements Named {
    use Greets;
    public const MAX = 10, MIN = 0;
    private int $id, $age;
    public function __construct(private readonly string $name, int $plain) {}
    public static function find(int $id): ?static { return null; }
    abstract protected function label(): string;
}`,
			want: `class User
  constant MAX
  constant MIN
  property $id
  property $age
  constructor __construct
    property $name
  method find
  method label
`,
		},
		{
			name: "interface, trait and enum",
			src: `<?php
interface Named { const PREFIX = 'x'; public function name(): string; }
trait Greets { public $greeting; function greet() {} }
enum Suit: string {
    case Hearts = 'H';
    case Spades = 'S';
    const Wild = self::Spades;
    public function color(): string { return 'red'; }
}`,
			want: `interface Named
  constant PREFIX
  method name
trait Greets
  property $greeting
  method greet
enum Suit
  case Hearts
  case Spades
  constant Wild
  method color
`,
		},
		{
			name: "functions and constants",
			src: `<?php
const ANSWER = 42, OTHER = 1;
function helper($a) { function inner() {} }
if (!function_exists('fallback')) {
    function fallback() {}
}
$closure = function () {};
$arrow = fn() => 1;
$object = new class { public function hidden() {} };`,
			want: `constant ANSWER
constant OTHER
function helper
  function inner
function fallback
`,
		},
		{
			name: "constants made with define",
			src: `<?php
define('APP_VERSION', '1.0');
\define("DEBUG", true);
DEFINE('LOUD', 1);
if (!defined('MAYBE')) {
    define('MAYBE', 2);
}
function boot() { define('BOOTED', true); }
define($dynamic, 1);
define("WITH_$suffix", 1);
define('Vendor\QUALIFIED', 1);
defined('ONLY_CHECKED');
$object->define('METHOD', 1);
Foo::define('STATIC', 1);`,
			want: `constant APP_VERSION
constant DEBUG
constant LOUD
constant MAYBE
function boot
  constant BOOTED
`,
		},
		{
			// define always makes a global constant, which a namespace in
			// the outline would misname.
			name: "define in a namespace statement is left out",
			src: `<?php
namespace App;
define('IN_NAMESPACE', 1);
function boot() { define('ALSO_IN_NAMESPACE', 1); }`,
			want: `namespace App
  function boot
`,
		},
		{
			name: "define in a braced namespace is left out",
			src: `<?php
namespace App { define('BRACED', 1); }
namespace { define('GLOBAL_AGAIN', 1); }`,
			want: `namespace App
constant GLOBAL_AGAIN
`,
		},
		{
			name: "namespace statements group what follows them",
			src: `<?php
namespace App\Models;
use Foo\Bar;
class User {}
function helper() {}
namespace App\Other;
class Thing {}`,
			want: `namespace App\Models
  class User
  function helper
namespace App\Other
  class Thing
`,
		},
		{
			name: "braced namespaces",
			src: `<?php
namespace App { class User {} }
namespace { function global_helper() {} }`,
			want: `namespace App
  class User
function global_helper
`,
		},
		{
			name: "declarations before a syntax error survive",
			src: `<?php
class Kept { public function ok() {} }
function alsoKept() {}
$x = ;
`,
			want: `class Kept
  method ok
function alsoKept
`,
		},
		{
			name: "html without php",
			src:  "<html><body>Hi</body></html>",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := outline(extract(t, tt.src), ""); got != tt.want {
				t.Errorf("outline:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestExtractResolvesWhatClassesBuildOn(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "through use statements and the namespace",
			src: `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
use Contracts\{Named, Auditable as Audit};
use function Foo\Bar; // a function import does not name classes
class User extends Model implements Named, Audit, \Countable, Bar {
    use HasName, \Shared\Timestamps { HasName::name insteadof Timestamps; }
}
interface Repo extends Named, Base {}
enum Status: string implements Named { use HasName; case A = 'a'; }
trait HasName { use Other; }`,
			want: `User extends [Illuminate\Database\Eloquent\Model] implements [Contracts\Named Contracts\Auditable Countable App\Models\Bar] traits [App\Models\HasName Shared\Timestamps]
Repo extends [Contracts\Named App\Models\Base] implements [] traits []
Status extends [] implements [Contracts\Named BackedEnum] traits [App\Models\HasName]
HasName extends [] implements [] traits [App\Models\Other]
`,
		},
		{
			// Every enum implements UnitEnum; a backed one BackedEnum, which
			// extends UnitEnum. They are global, whatever the namespace.
			name: "enums implement the built-in enum interfaces",
			src: `<?php
namespace App;
enum Suit { case Hearts; }
enum Level: int { case Low = 1; }`,
			want: `Suit extends [] implements [UnitEnum] traits []
Level extends [] implements [BackedEnum] traits []
`,
		},
		{
			name: "each namespace statement starts without imports",
			src: `<?php
namespace A;
use X\Base;
class One extends Base {}
namespace B;
class Two extends Base {}`,
			want: `One extends [X\Base] implements [] traits []
Two extends [B\Base] implements [] traits []
`,
		},
		{
			name: "braced namespaces have their own imports",
			src: `<?php
namespace A { use X\Base; class One extends Base {} }
namespace { class Two extends Base {} }`,
			want: `One extends [X\Base] implements [] traits []
Two extends [Base] implements [] traits []
`,
		},
		{
			name: "use groups with a namespace prefix",
			src: `<?php
use Vendor\Pkg\{Sub\Base, Other as O};
class C extends Base implements O {}`,
			want: `C extends [Vendor\Pkg\Sub\Base] implements [Vendor\Pkg\Other] traits []
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relations(extract(t, tt.src)); got != tt.want {
				t.Errorf("relations:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestDefineSpansTheCallAndNamesTheString(t *testing.T) {
	src := "<?php\ndefine('PHP_EOL', \"\\n\");\n"

	got := outline(extract(t, src), src)

	if want := "constant PHP_EOL [2:1-2:24] name@2:9\n"; got != want {
		t.Errorf("outline = %q, want %q", got, want)
	}
}

func TestExtractRecordsSpansAsByteOffsets(t *testing.T) {
	src := "<?php\nclass Ünit {\n    function run() {}\n}\n"

	decls := extract(t, src)

	if len(decls) != 1 || len(decls[0].Children) != 1 {
		t.Fatalf("outline:\n%s", outline(decls, ""))
	}
	class, method := decls[0], decls[0].Children[0]
	checks := []struct {
		what string
		got  string
		want string
	}{
		{"class span", src[class.Start:class.End], "class Ünit {\n    function run() {}\n}"},
		{"class name", src[class.NameStart:class.NameEnd], "Ünit"},
		{"method span", src[method.Start:method.End], "function run() {}"},
		{"method name", src[method.NameStart:method.NameEnd], "run"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.what, c.got, c.want)
		}
	}
}

// A statement that declares one name spans the whole statement, so the
// cursor anywhere in it (a property hook, a type) is inside the symbol. A
// statement with several names gives each its own element.
func TestSingleElementStatementsSpanTheStatement(t *testing.T) {
	src := "<?php\nclass A {\n    public int $one { get => 1; }\n    const X = 1, Y = 2;\n}\n"

	decls := extract(t, src)

	if len(decls) != 1 || len(decls[0].Children) != 3 {
		t.Fatalf("outline:\n%s", outline(decls, ""))
	}
	members := decls[0].Children
	if got := src[members[0].Start:members[0].End]; got != "public int $one { get => 1; }" {
		t.Errorf("property span = %q", got)
	}
	if got := src[members[1].Start:members[1].End]; got != "X = 1" {
		t.Errorf("constant X span = %q", got)
	}
}

func TestNamespaceStatementSpansItsDeclarations(t *testing.T) {
	src := "<?php\nnamespace A;\nclass X {}\n\nnamespace B;\n"

	decls := extract(t, src)

	if len(decls) != 2 {
		t.Fatalf("outline:\n%s", outline(decls, ""))
	}
	if got := src[decls[0].Start:decls[0].End]; got != "namespace A;\nclass X {}" {
		t.Errorf("namespace A span = %q", got)
	}
	if got := src[decls[0].NameStart:decls[0].NameEnd]; got != "A" {
		t.Errorf("namespace A name = %q", got)
	}
	if got := src[decls[1].Start:decls[1].End]; got != "namespace B;" {
		t.Errorf("namespace B span = %q", got)
	}
}

// TestExtractGolden compares the outline of each testdata/extract/*.php file
// with its .golden file. Run with -update to rewrite them after a deliberate
// change, and review the diff.
func TestExtractGolden(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "extract", "*.php"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			got := outline(extract(t, string(src)), string(src))
			golden := strings.TrimSuffix(file, ".php") + ".golden"
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("reading golden file (run with -update to create it): %v", err)
			}
			if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
				t.Errorf("outline of %s:\n%s\nwant:\n%s", file, got, want)
			}
		})
	}
}

// relations renders what each class-like builds on, one per line.
func relations(decls []php.Declaration) string {
	var b strings.Builder
	var walk func([]php.Declaration)
	walk = func(ds []php.Declaration) {
		for _, d := range ds {
			if len(d.Extends)+len(d.Implements)+len(d.Traits) > 0 {
				fmt.Fprintf(&b, "%s extends %v implements %v traits %v\n", d.Name, d.Extends, d.Implements, d.Traits)
			}
			walk(d.Children)
		}
	}
	walk(decls)
	return b.String()
}
