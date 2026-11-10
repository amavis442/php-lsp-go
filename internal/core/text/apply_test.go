package text

import "testing"

func TestApplyChanges(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		changes []Change
		enc     Encoding
		want    string
	}{
		{
			name:    "whole document replaces the text",
			text:    "old",
			changes: []Change{{Text: "new"}},
			want:    "new",
		},
		{
			name:    "insert",
			text:    "<?php\necho;",
			changes: []Change{{Range: &Range{Position{1, 4}, Position{1, 4}}, Text: " 1"}},
			want:    "<?php\necho 1;",
		},
		{
			name:    "delete across lines",
			text:    "a\nb\nc",
			changes: []Change{{Range: &Range{Position{0, 1}, Position{2, 0}}, Text: ""}},
			want:    "ac",
		},
		{
			name: "changes apply in order, each on the previous result",
			text: "abc",
			changes: []Change{
				{Range: &Range{Position{0, 0}, Position{0, 1}}, Text: "xy"},
				{Range: &Range{Position{0, 2}, Position{0, 3}}, Text: "B"},
			},
			want: "xyBc",
		},
		{
			name:    "utf-16 positions after an emoji",
			text:    "😀a",
			changes: []Change{{Range: &Range{Position{0, 2}, Position{0, 3}}, Text: "b"}},
			want:    "😀b",
		},
		{
			name:    "utf-8 positions after an emoji",
			text:    "😀a",
			changes: []Change{{Range: &Range{Position{0, 4}, Position{0, 5}}, Text: "b"}},
			enc:     UTF8,
			want:    "😀b",
		},
		{
			name:    "reversed range is treated as start..end",
			text:    "abc",
			changes: []Change{{Range: &Range{Position{0, 2}, Position{0, 1}}, Text: ""}},
			want:    "ac",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Apply(tt.text, tt.changes, tt.enc); got != tt.want {
				t.Errorf("Apply = %q, want %q", got, tt.want)
			}
		})
	}
}
