package php

// RefKind is what kind of declaration a name refers to.
type RefKind int

const (
	RefClass RefKind = iota + 1 // class, interface, trait or enum
	RefFunction
	RefConstant
	RefMember // method, property, class constant or enum case
)

// Reference is a name in the source and what it may refer to.
type Reference struct {
	Kind RefKind
	// Candidates are qualified names to try in order: the class, function
	// or constant itself, or for a member the class to start looking in.
	Candidates []string
	// For RefMember: the member's name and kind (KindMethod, KindProperty
	// or KindConstant, which also stands for enum cases).
	Member     string
	MemberKind SymbolKind
	// Start and End span the name as byte offsets.
	Start, End int
}
