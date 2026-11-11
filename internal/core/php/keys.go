package php

import "strings"

// Index keys put PHP's case rules in one place: class, function and
// method names and namespaces ignore case; constant, property and enum
// case names do not. Each kind of name has its own prefix, since a class
// and a function may share a name.

// ClassKey is the key of a class, interface, trait or enum.
func ClassKey(qualified string) string { return "c:" + strings.ToLower(qualified) }

// FunctionKey is the key of a function.
func FunctionKey(qualified string) string { return "f:" + strings.ToLower(qualified) }

// ConstantKey is the key of a global constant.
func ConstantKey(qualified string) string {
	i := strings.LastIndex(qualified, `\`)
	return "k:" + strings.ToLower(qualified[:i+1]) + qualified[i+1:]
}

// MemberKey is the key of member name of class: a method, a property (with
// or without its "$"), or a constant or enum case, which share one name
// space.
func MemberKey(class string, kind SymbolKind, name string) string {
	prefix := "m:" + strings.ToLower(class) + "::"
	switch kind {
	case KindMethod, KindConstructor:
		return prefix + "method:" + strings.ToLower(name)
	case KindProperty:
		return prefix + "property:" + strings.TrimPrefix(name, "$")
	default:
		return prefix + "constant:" + name
	}
}

// KeyOf is the key s is found under.
func KeyOf(s WorkspaceSymbol) string {
	if s.Member {
		return MemberKey(s.Container, s.Kind, s.Name)
	}
	q := s.Name
	if s.Container != "" {
		q = s.Container + `\` + s.Name
	}
	switch s.Kind {
	case KindFunction:
		return FunctionKey(q)
	case KindConstant:
		return ConstantKey(q)
	default:
		return ClassKey(q)
	}
}
