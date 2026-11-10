package text

// Change is one edit from the editor. A nil Range replaces the whole text.
type Change struct {
	Range *Range
	Text  string
}

// Apply applies changes in order; each change is relative to the result of
// the previous one, as textDocument/didChange specifies.
func Apply(text string, changes []Change, enc Encoding) string {
	for _, c := range changes {
		if c.Range == nil {
			text = c.Text
			continue
		}
		ix := NewLineIndex(text)
		start := ix.Offset(c.Range.Start, enc)
		end := ix.Offset(c.Range.End, enc)
		if end < start {
			start, end = end, start
		}
		text = text[:start] + c.Text + text[end:]
	}
	return text
}
