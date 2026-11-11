package domain

import (
	"strings"
	"testing"

	"github.com/amavis442/php-lsp-go/internal/core/php"
)

type fakeIndex []php.WorkspaceSymbol

func (f fakeIndex) Each(visit func(php.WorkspaceSymbol) bool) {
	for _, s := range f {
		if !visit(s) {
			return
		}
	}
}

func sym(container, name string) php.WorkspaceSymbol {
	return php.WorkspaceSymbol{Name: name, Container: container, Kind: php.KindClass}
}

func search(index fakeIndex, limit int, query string) string {
	var out []string
	for _, s := range NewWorkspaceSymbols(index, limit).Search(query) {
		if s.Container != "" {
			out = append(out, s.Container+`\`+s.Name)
		} else {
			out = append(out, s.Name)
		}
	}
	return strings.Join(out, " ")
}

func TestSearchRanksExactThenPrefixThenSubstringThenFuzzy(t *testing.T) {
	index := fakeIndex{
		sym("App", "UserRepository"),
		sym("App", "AbstractUser"),
		sym("App", "UnusedRecord"),
		sym("App", "User"),
		sym("App", "Users"),
		sym("App", "Order"),
	}

	got := search(index, 10, "user")

	want := `App\User App\Users App\UserRepository App\AbstractUser App\UnusedRecord`
	if got != want {
		t.Errorf("search(user) = %s\nwant %s", got, want)
	}
}

func TestSearchIgnoresCaseAndTheDollarOfProperties(t *testing.T) {
	index := fakeIndex{
		{Name: "$userId", Container: `App\User`, Kind: php.KindProperty},
		{Name: "findUser", Container: `App\Repo`, Kind: php.KindMethod},
	}

	if got := search(index, 10, "USERID"); got != `App\User\$userId` {
		t.Errorf("search(USERID) = %s", got)
	}
	if got := search(index, 10, "$user"); got != `App\User\$userId App\Repo\findUser` {
		t.Errorf("search($user) = %s", got)
	}
}

func TestQualifiedQueriesMatchTheContainerToo(t *testing.T) {
	index := fakeIndex{sym(`App\Models`, "User"), sym(`Vendor\Auth`, "User")}

	if got := search(index, 10, `models\user`); got != `App\Models\User` {
		t.Errorf(`search(models\user) = %s`, got)
	}
}

func TestSearchKeepsTheBestResultsUpToTheLimit(t *testing.T) {
	index := fakeIndex{sym("", "Axxxxx"), sym("", "Ab"), sym("", "A"), sym("", "Aaaa")}

	if got := search(index, 2, "a"); got != "A Ab" {
		t.Errorf("search(a) with limit 2 = %s, want the two best", got)
	}
}

func TestEmptyQueryListsSymbolsByName(t *testing.T) {
	index := fakeIndex{sym("", "Zed"), sym("", "Alpha"), sym("", "Mid")}

	if got := search(index, 2, ""); got != "Alpha Mid" {
		t.Errorf("search('') = %s", got)
	}
}

func TestNoMatchGivesNoResults(t *testing.T) {
	if got := search(fakeIndex{sym("", "User")}, 10, "xyz"); got != "" {
		t.Errorf("search(xyz) = %s, want nothing", got)
	}
}
