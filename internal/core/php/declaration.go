package php

// SymbolKind is what a declaration declares.
type SymbolKind int

const (
	KindNamespace SymbolKind = iota + 1
	KindClass
	KindInterface
	KindTrait
	KindEnum
	KindEnumCase
	KindFunction
	KindMethod
	KindConstructor
	KindProperty
	KindConstant
)

// Declaration is one named declaration in a file. Start and End span the
// whole declaration; NameStart and NameEnd span its name. All are byte
// offsets into the source.
type Declaration struct {
	Name      string
	Kind      SymbolKind
	Start     int
	End       int
	NameStart int
	NameEnd   int
	Children  []Declaration
}

// FileSummary is what the server keeps of a parsed file.
type FileSummary struct {
	// Declarations is the outline of the file: namespaces contain their
	// declarations, classes their members.
	Declarations []Declaration
}
