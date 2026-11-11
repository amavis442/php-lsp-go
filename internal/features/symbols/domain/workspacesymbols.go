package domain

import (
	"sort"
	"strings"

	"github.com/amavis442/php-lsp-go/internal/core/php"
)

// SymbolIndex gives every symbol in the workspace.
type SymbolIndex interface {
	// Each calls visit for every symbol until visit returns false.
	Each(visit func(php.WorkspaceSymbol) bool)
}

// DefaultLimit is how many workspace symbols a search returns at most.
const DefaultLimit = 250

// Match quality, best first.
const (
	matchExact = iota
	matchPrefix
	matchSubstring
	matchFuzzy
	noMatch
)

// WorkspaceSymbols searches the symbols of the whole workspace.
type WorkspaceSymbols struct {
	index SymbolIndex
	limit int
}

// NewWorkspaceSymbols returns a search over index that returns at most
// limit symbols.
func NewWorkspaceSymbols(index SymbolIndex, limit int) *WorkspaceSymbols {
	return &WorkspaceSymbols{index: index, limit: limit}
}

type scored struct {
	symbol php.WorkspaceSymbol
	match  int
	length int
}

// Search returns the symbols that match query, best first. Matching ignores
// case and the "$" of properties. An exact name beats a prefix, a prefix a
// substring, and a substring a fuzzy match (the query's characters in
// order); within one kind of match shorter names come first. A query with a
// backslash is matched against the qualified name, so "models\user" finds
// App\Models\User.
func (w *WorkspaceSymbols) Search(query string) []php.WorkspaceSymbol {
	q := normalize(query)
	qualified := strings.Contains(q, `\`)

	var found []scored
	w.index.Each(func(s php.WorkspaceSymbol) bool {
		target := normalize(s.Name)
		if qualified && s.Container != "" {
			target = strings.ToLower(s.Container) + `\` + target
		}
		if m := match(q, target); m != noMatch {
			length := len(target)
			if q == "" {
				length = 0 // nothing to rank by but the name
			}
			found = append(found, scored{symbol: s, match: m, length: length})
		}
		return true
	})

	sort.Slice(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if a.match != b.match {
			return a.match < b.match
		}
		if a.length != b.length {
			return a.length < b.length
		}
		if a.symbol.Name != b.symbol.Name {
			return a.symbol.Name < b.symbol.Name
		}
		return a.symbol.Container < b.symbol.Container
	})

	n := min(len(found), w.limit)
	out := make([]php.WorkspaceSymbol, n)
	for i := range n {
		out[i] = found[i].symbol
	}
	return out
}

func normalize(name string) string {
	return strings.ToLower(strings.TrimPrefix(name, "$"))
}

// match rates how well q matches target; both are normalized. An empty
// query matches everything equally.
func match(q, target string) int {
	switch {
	case q == "" || q == target:
		return matchExact
	case strings.HasPrefix(target, q):
		return matchPrefix
	case strings.Contains(target, q):
		return matchSubstring
	case isSubsequence(q, target):
		return matchFuzzy
	}
	return noMatch
}

func isSubsequence(q, target string) bool {
	i := 0
	for j := 0; i < len(q) && j < len(target); j++ {
		if q[i] == target[j] {
			i++
		}
	}
	return i == len(q)
}
