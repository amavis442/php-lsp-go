// Package text converts between byte offsets and editor positions. Editors
// count characters per line in a negotiated encoding (UTF-16 by default),
// while the parser and the rest of the server work with byte offsets.
package text

import (
	"sort"
	"unicode/utf8"
)

// Encoding is the unit in which Position.Character counts.
type Encoding int

const (
	// UTF16 is the LSP default and must always be supported.
	UTF16 Encoding = iota
	// UTF8 counts bytes, which matches Go strings and the parser directly.
	UTF8
	// UTF32 counts Unicode code points.
	UTF32
)

// Position is a zero-based line and character offset.
type Position struct {
	Line      int
	Character int
}

// Range is a half-open span between two positions.
type Range struct {
	Start Position
	End   Position
}

type line struct {
	start int // byte offset of the first character
	end   int // byte offset of the line terminator, or of the end of text
}

// LineIndex maps byte offsets to positions and back for one version of a
// text. "\n", "\r\n" and a lone "\r" all end a line, as LSP requires.
type LineIndex struct {
	text  string
	lines []line
}

// NewLineIndex indexes the line boundaries of text.
func NewLineIndex(text string) *LineIndex {
	ix := &LineIndex{text: text}
	start := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\n':
			ix.lines = append(ix.lines, line{start, i})
			start = i + 1
		case '\r':
			ix.lines = append(ix.lines, line{start, i})
			if i+1 < len(text) && text[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	ix.lines = append(ix.lines, line{start, len(text)})
	return ix
}

// Offset returns the byte offset of p. Like LSP clients expect, a character
// past the end of its line means the end of that line, and a line past the
// end of the text means the end of the text. A position inside a character
// (for example between the two halves of a UTF-16 surrogate pair) rounds
// down to the start of that character.
func (ix *LineIndex) Offset(p Position, enc Encoding) int {
	if p.Line < 0 {
		return 0
	}
	if p.Line >= len(ix.lines) {
		return len(ix.text)
	}
	l := ix.lines[p.Line]
	units := 0
	for i := l.start; i < l.end; {
		r, size := utf8.DecodeRuneInString(ix.text[i:l.end])
		units += width(r, size, enc)
		if units > p.Character {
			return i
		}
		i += size
	}
	return l.end
}

// Position returns the position of a byte offset. Offsets past the end of
// the text clamp to the end; offsets inside a line terminator clamp to the
// end of that line; offsets inside a multi-byte character round down.
func (ix *LineIndex) Position(offset int, enc Encoding) Position {
	offset = max(0, min(offset, len(ix.text)))
	n := sort.Search(len(ix.lines), func(i int) bool { return ix.lines[i].start > offset }) - 1
	l := ix.lines[n]
	offset = min(offset, l.end)
	for offset > l.start && offset < len(ix.text) && !utf8.RuneStart(ix.text[offset]) {
		offset--
	}
	units := 0
	for i := l.start; i < offset; {
		r, size := utf8.DecodeRuneInString(ix.text[i:offset])
		units += width(r, size, enc)
		i += size
	}
	return Position{Line: n, Character: units}
}

// width is the number of enc units one decoded rune occupies. An invalid
// byte decodes as a single unit in every encoding.
func width(r rune, size int, enc Encoding) int {
	switch enc {
	case UTF8:
		return size
	case UTF32:
		return 1
	default:
		if r >= 0x10000 {
			return 2
		}
		return 1
	}
}
