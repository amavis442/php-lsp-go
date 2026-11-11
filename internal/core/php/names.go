package php

import "strings"

// UseKind is what a use statement imports.
type UseKind int

const (
	UseClass UseKind = iota
	UseFunction
	UseConstant
)

// Scope is what a name written in a file resolves against: the namespace
// it is in and the use statements before it. The zero Scope is the global
// namespace without imports.
type Scope struct {
	Namespace string

	classes   map[string]string // lower-case alias -> qualified name
	functions map[string]string // lower-case alias -> qualified name
	constants map[string]string // alias -> qualified name
}

// Import records "use name as alias" of kind. Without an alias the last
// part of name is the alias.
func (s *Scope) Import(kind UseKind, name, alias string) {
	name = strings.TrimPrefix(name, `\`)
	if alias == "" {
		alias = lastPart(name)
	}
	switch kind {
	case UseClass:
		s.classes = put(s.classes, strings.ToLower(alias), name)
	case UseFunction:
		s.functions = put(s.functions, strings.ToLower(alias), name)
	case UseConstant:
		s.constants = put(s.constants, alias, name)
	}
}

// Clone returns a copy whose imports can change without changing s.
func (s Scope) Clone() Scope {
	return Scope{
		Namespace: s.Namespace,
		classes:   clone(s.classes),
		functions: clone(s.functions),
		constants: clone(s.constants),
	}
}

// ResolveClass returns the qualified name, without a leading backslash, of
// the class, interface, trait or enum that name refers to.
func (s Scope) ResolveClass(name string) string {
	if q, ok := s.qualified(name); ok {
		return q
	}
	if q, ok := s.classes[strings.ToLower(name)]; ok {
		return q
	}
	return s.inNamespace(name)
}

// FunctionCandidates returns the qualified names an unqualified function
// call may mean, in the order PHP tries them: the current namespace first,
// then the global function.
func (s Scope) FunctionCandidates(name string) []string {
	if q, ok := s.qualified(name); ok {
		return []string{q}
	}
	if q, ok := s.functions[strings.ToLower(name)]; ok {
		return []string{q}
	}
	return s.withFallback(name)
}

// ConstantCandidates is FunctionCandidates for constants, whose aliases
// are case-sensitive.
func (s Scope) ConstantCandidates(name string) []string {
	if q, ok := s.qualified(name); ok {
		return []string{q}
	}
	if q, ok := s.constants[name]; ok {
		return []string{q}
	}
	return s.withFallback(name)
}

// qualified resolves the names whose meaning does not depend on the kind
// of import: fully qualified, namespace-relative, and qualified names,
// whose first part may be a class alias.
func (s Scope) qualified(name string) (string, bool) {
	if strings.HasPrefix(name, `\`) {
		return name[1:], true
	}
	first, rest, ok := strings.Cut(name, `\`)
	if !ok {
		return "", false
	}
	if strings.EqualFold(first, "namespace") {
		return s.inNamespace(rest), true
	}
	if q, ok := s.classes[strings.ToLower(first)]; ok {
		return q + `\` + rest, true
	}
	return s.inNamespace(name), true
}

func (s Scope) inNamespace(name string) string {
	if s.Namespace == "" {
		return name
	}
	return s.Namespace + `\` + name
}

func (s Scope) withFallback(name string) []string {
	if s.Namespace == "" {
		return []string{name}
	}
	return []string{s.inNamespace(name), name}
}

func lastPart(name string) string {
	return name[strings.LastIndex(name, `\`)+1:]
}

func put(m map[string]string, k, v string) map[string]string {
	if m == nil {
		m = make(map[string]string)
	}
	m[k] = v
	return m
}

func clone(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
