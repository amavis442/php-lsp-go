package php

import "testing"

func TestKeysFollowPHPsCaseRules(t *testing.T) {
	same := []struct{ a, b string }{
		{ClassKey(`App\User`), ClassKey(`app\USER`)},
		{FunctionKey(`App\helper`), FunctionKey(`APP\Helper`)},
		{ConstantKey(`App\LIMIT`), ConstantKey(`app\LIMIT`)}, // the namespace ignores case
		{MemberKey(`App\User`, KindMethod, "save"), MemberKey(`app\user`, KindMethod, "SAVE")},
		{MemberKey(`App\User`, KindConstructor, "__construct"), MemberKey(`App\User`, KindMethod, "__CONSTRUCT")},
		{MemberKey(`App\S`, KindEnumCase, "Active"), MemberKey(`App\S`, KindConstant, "Active")},
		{MemberKey(`App\User`, KindProperty, "$name"), MemberKey(`App\User`, KindProperty, "name")},
	}
	for _, s := range same {
		if s.a != s.b {
			t.Errorf("%q != %q, want the same key", s.a, s.b)
		}
	}
	different := []struct{ a, b string }{
		{ConstantKey("LIMIT"), ConstantKey("limit")},
		{MemberKey(`App\User`, KindProperty, "$name"), MemberKey(`App\User`, KindProperty, "$Name")},
		{MemberKey(`App\User`, KindConstant, "MAX"), MemberKey(`App\User`, KindConstant, "max")},
		{ClassKey(`App\User`), FunctionKey(`App\User`)},
		{MemberKey(`App\User`, KindMethod, "name"), MemberKey(`App\User`, KindProperty, "name")},
	}
	for _, d := range different {
		if d.a == d.b {
			t.Errorf("%q == %q, want different keys", d.a, d.b)
		}
	}
}

func TestKeyOfASymbol(t *testing.T) {
	tests := []struct {
		s    WorkspaceSymbol
		want string
	}{
		{WorkspaceSymbol{Name: "User", Kind: KindClass, Container: "App"}, ClassKey(`App\User`)},
		{WorkspaceSymbol{Name: "Status", Kind: KindEnum}, ClassKey("Status")},
		{WorkspaceSymbol{Name: "helper", Kind: KindFunction, Container: "App"}, FunctionKey(`App\helper`)},
		{WorkspaceSymbol{Name: "LIMIT", Kind: KindConstant, Container: "App"}, ConstantKey(`App\LIMIT`)},
		{WorkspaceSymbol{Name: "save", Kind: KindMethod, Container: `App\User`, Member: true}, MemberKey(`App\User`, KindMethod, "save")},
		{WorkspaceSymbol{Name: "MAX", Kind: KindConstant, Container: `App\User`, Member: true}, MemberKey(`App\User`, KindConstant, "MAX")},
	}
	for _, tt := range tests {
		if got := KeyOf(tt.s); got != tt.want {
			t.Errorf("KeyOf(%+v) = %q, want %q", tt.s, got, tt.want)
		}
	}
}
