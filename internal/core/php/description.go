package php

// Description is what hover shows of a declaration.
type Description struct {
	// Signature is the declaration as PHP without its body: modifiers,
	// parameters with types and defaults, and the return type.
	Signature string
	// Summary is the text of its PHPDoc comment before the first tag.
	Summary string
}
