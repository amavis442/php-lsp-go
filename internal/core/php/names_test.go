package php

import (
	"reflect"
	"testing"
)

func appScope() Scope {
	s := Scope{Namespace: `App\Http`}
	s.Import(UseClass, `Foo\Bar`, "")
	s.Import(UseClass, `Foo\Baz`, "Q")
	s.Import(UseFunction, `Foo\helper`, "")
	s.Import(UseConstant, `Foo\LIMIT`, "")
	return s
}

func TestResolveClass(t *testing.T) {
	tests := []struct {
		scope Scope
		name  string
		want  string
	}{
		{appScope(), `\Fully\Qualified`, `Fully\Qualified`},
		{appScope(), "Bar", `Foo\Bar`},
		{appScope(), "bar", `Foo\Bar`}, // aliases ignore case
		{appScope(), "Q", `Foo\Baz`},
		{appScope(), "Local", `App\Http\Local`},
		{appScope(), `Bar\Sub`, `Foo\Bar\Sub`}, // the first part is an alias
		{appScope(), `Models\User`, `App\Http\Models\User`},
		{appScope(), `namespace\Local`, `App\Http\Local`},
		{Scope{}, "Global", "Global"},
		{Scope{}, `\Global`, "Global"},
	}
	for _, tt := range tests {
		if got := tt.scope.ResolveClass(tt.name); got != tt.want {
			t.Errorf("ResolveClass(%q) in %q = %q, want %q", tt.name, tt.scope.Namespace, got, tt.want)
		}
	}
}

func TestFunctionCandidatesFallBackToTheGlobalFunction(t *testing.T) {
	tests := []struct {
		scope Scope
		name  string
		want  []string
	}{
		{appScope(), "strlen", []string{`App\Http\strlen`, "strlen"}},
		{appScope(), `\strlen`, []string{"strlen"}},
		{appScope(), "helper", []string{`Foo\helper`}},
		{appScope(), "HELPER", []string{`Foo\helper`}}, // function names ignore case
		{appScope(), `Bar\make`, []string{`Foo\Bar\make`}},
		{Scope{}, "strlen", []string{"strlen"}},
	}
	for _, tt := range tests {
		if got := tt.scope.FunctionCandidates(tt.name); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("FunctionCandidates(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestConstantCandidatesFallBackToTheGlobalConstant(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{"PHP_EOL", []string{`App\Http\PHP_EOL`, "PHP_EOL"}},
		{`\PHP_EOL`, []string{"PHP_EOL"}},
		{"LIMIT", []string{`Foo\LIMIT`}},
		{"limit", []string{`App\Http\limit`, "limit"}}, // constant aliases are case-sensitive
	}
	for _, tt := range tests {
		if got := appScope().ConstantCandidates(tt.name); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ConstantCandidates(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestImportUsesTheLastPartAsAliasAndDropsALeadingBackslash(t *testing.T) {
	var s Scope
	s.Import(UseClass, `\Foo\Bar`, "")

	if got := s.ResolveClass("Bar"); got != `Foo\Bar` {
		t.Errorf("ResolveClass(Bar) = %q", got)
	}
}
