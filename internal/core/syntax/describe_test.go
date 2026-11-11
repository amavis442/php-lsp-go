package syntax

import (
	"strings"
	"testing"
)

const describeSource = `<?php
namespace App\Models;

/**
 * A person who can sign in.
 *
 * Users belong to one team.
 *
 * @property string $email
 */
#[Table('users')]
final class User extends Model implements Authenticatable
{
    /** The largest team. */
    public const MAX_TEAM = 10, MIN_TEAM = 1;

    /**
     * The display name.
     * @var string
     */
    public string $name {
        get => ucfirst($this->name);
    }

    protected ?int $age = null;

    public function __construct(
        /** Where users are stored. */
        private readonly UserRepository $repository,
        private int $limit = 5,
    ) {}

    /**
     * Saves the user.
     *
     * @param array<string, mixed> $options
     * @return bool
     */
    #[Pure]
    public function save(array $options = [], ?string $reason = null): bool
    {
        return true;
    }

    abstract protected static function make(int ...$ids): static;
}

interface Authenticatable { public function id(): int; }

enum Role: string
{
    /** Can do anything. */
    case Admin = 'admin';
}

// Not a docblock.
function helper(int|string $value, callable $then = null): void {}

/** Line separator. */
define('APP_EOL', "\n");

/** Not about the next one. */
$x = 1;
function undocumented() {}
`

func describeAt(t *testing.T, marked string) string {
	t.Helper()
	at := strings.Index(describeSource, strings.ReplaceAll(marked, cursor, ""))
	if at < 0 {
		t.Fatalf("%q not in source", marked)
	}
	offset := at + strings.Index(marked, cursor)
	d, ok, err := NewParser().Describe([]byte(describeSource), offset)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if !ok {
		return "none"
	}
	return d.Signature + " | " + d.Summary
}

func TestDescribeLeavesOutParameterAttributesAndParametersOfOldPHPVersions(t *testing.T) {
	src := `<?php
function array_map(
    ?callable $callback,
    #[PhpStormStubsElementAvailable(from: '8.0')] array $array,
    #[PhpStormStubsElementAvailable(from: '5.3', to: '7.4')] $arrays,
    array ...$arrays
): array {}
function login(#[\SensitiveParameter] string $password, #[\JetBrains\PhpStorm\Internal\PhpStormStubsElementAvailable(to: '7.4')] $old = null): bool {}
`
	tests := []struct{ name, want string }{
		{"array_map", "function array_map(?callable $callback, array $array, array ...$arrays): array"},
		{"login", "function login(string $password): bool"},
	}
	for _, tt := range tests {
		d, ok, err := NewParser().Describe([]byte(src), strings.Index(src, "function "+tt.name)+len("function "))
		if err != nil || !ok || d.Signature != tt.want {
			t.Errorf("%s: %q, %v, %v\nwant %q", tt.name, d.Signature, ok, err, tt.want)
		}
	}
}

func TestDescribe(t *testing.T) {
	tests := []struct{ at, want string }{
		{"class §User", "final class User extends Model implements Authenticatable | A person who can sign in.\n\nUsers belong to one team."},
		{"const §MAX_TEAM", "public const MAX_TEAM = 10 | The largest team."},
		{"§MIN_TEAM", "public const MIN_TEAM = 1 | The largest team."},
		{"string §$name", "public string $name | The display name."},
		{"?int §$age", "protected ?int $age = null | "},
		{"function §__construct", "public function __construct(private readonly UserRepository $repository, private int $limit = 5) | "},
		{"UserRepository §$repository", "private readonly UserRepository $repository | Where users are stored."},
		{"int §$limit", "private int $limit = 5 | "},
		{"function §save", "public function save(array $options = [], ?string $reason = null): bool | Saves the user."},
		{"function §make", "abstract protected static function make(int ...$ids): static | "},
		{"function §id", "public function id(): int | "},
		{"enum §Role", "enum Role: string | "},
		{"case §Admin", "case Admin = 'admin' | Can do anything."},
		{"function §helper", "function helper(int|string $value, callable $then = null): void | "},
		{"define('§APP_EOL'", `const APP_EOL = "\n" | Line separator.`},
		{"function §undocumented", "function undocumented() | "},
		{"extends §Model", "none"}, // a reference, not a declaration
	}
	for _, tt := range tests {
		t.Run(tt.at, func(t *testing.T) {
			if got := describeAt(t, tt.at); got != tt.want {
				t.Errorf("at %q:\n got %q\nwant %q", tt.at, got, tt.want)
			}
		})
	}
}
