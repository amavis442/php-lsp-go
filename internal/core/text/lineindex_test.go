package text

import "testing"

// "é" is 2 UTF-8 bytes and 1 UTF-16 unit; "😀" is 4 UTF-8 bytes and 2 UTF-16 units.
const sample = "a\r\nbé😀c\rd\nlast"

func TestOffsetPerEncoding(t *testing.T) {
	ix := NewLineIndex(sample)
	tests := []struct {
		name string
		pos  Position
		enc  Encoding
		want int
	}{
		{"start", Position{0, 0}, UTF16, 0},
		{"after CRLF", Position{1, 0}, UTF16, 3},
		{"after é in utf-16", Position{1, 2}, UTF16, 6},
		{"after emoji in utf-16", Position{1, 4}, UTF16, 10},
		{"after emoji in utf-8", Position{1, 7}, UTF8, 10},
		{"after emoji in utf-32", Position{1, 3}, UTF32, 10},
		{"lone CR is a line break", Position{2, 0}, UTF16, 12},
		{"last line", Position{3, 2}, UTF16, 16},
		{"character past line end clamps to line end", Position{0, 99}, UTF16, 1},
		{"line past end clamps to end of text", Position{9, 0}, UTF16, len(sample)},
		{"inside a surrogate pair rounds down", Position{1, 3}, UTF16, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ix.Offset(tt.pos, tt.enc); got != tt.want {
				t.Errorf("Offset(%+v) = %d, want %d", tt.pos, got, tt.want)
			}
		})
	}
}

func TestPositionPerEncoding(t *testing.T) {
	ix := NewLineIndex(sample)
	tests := []struct {
		name   string
		offset int
		enc    Encoding
		want   Position
	}{
		{"start", 0, UTF16, Position{0, 0}},
		{"on the CR of CRLF", 1, UTF16, Position{0, 1}},
		{"after emoji in utf-16", 10, UTF16, Position{1, 4}},
		{"after emoji in utf-8", 10, UTF8, Position{1, 7}},
		{"after emoji in utf-32", 10, UTF32, Position{1, 3}},
		{"inside a multi-byte rune rounds down", 8, UTF16, Position{1, 2}},
		{"end of text", len(sample), UTF16, Position{3, 4}},
		{"past end clamps", len(sample) + 5, UTF16, Position{3, 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ix.Position(tt.offset, tt.enc); got != tt.want {
				t.Errorf("Position(%d) = %+v, want %+v", tt.offset, got, tt.want)
			}
		})
	}
}

func TestEmptyTextHasOneLine(t *testing.T) {
	ix := NewLineIndex("")

	if got := ix.Offset(Position{0, 0}, UTF16); got != 0 {
		t.Errorf("Offset = %d, want 0", got)
	}
	if got := ix.Position(0, UTF16); got != (Position{0, 0}) {
		t.Errorf("Position = %+v, want {0 0}", got)
	}
}

func TestTrailingNewlineStartsAnEmptyLine(t *testing.T) {
	ix := NewLineIndex("a\n")

	if got := ix.Position(2, UTF16); got != (Position{1, 0}) {
		t.Errorf("Position = %+v, want {1 0}", got)
	}
}
