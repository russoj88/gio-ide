package editor

import (
	"testing"

	"gioui.org/text"
	"golang.org/x/image/math/fixed"
)

func TestSnapGlyphColumns(t *testing.T) {
	ltr := []text.Glyph{
		{X: 0, Runes: 1, Flags: text.FlagClusterBreak},
		{X: fixed.I(8) + 26, Runes: 1, Flags: text.FlagClusterBreak},
	}
	rtl := []text.Glyph{
		{X: 0, Runes: 1, Flags: text.FlagClusterBreak},
		{X: fixed.I(20), Runes: 1, Flags: text.FlagTowardOrigin | text.FlagClusterBreak},
		{X: fixed.I(10), Runes: 1, Flags: text.FlagTowardOrigin | text.FlagClusterBreak},
	}
	tests := []struct {
		name   string
		glyphs []text.Glyph
		wantX  []fixed.Int26_6
	}{
		{name: "ltr grid", glyphs: ltr, wantX: []fixed.Int26_6{0, fixed.I(8)}},
		{name: "toward origin", glyphs: rtl, wantX: []fixed.Int26_6{0, fixed.I(20), fixed.I(10)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := snapGlyphColumns(tt.glyphs, 8)
			if len(got) != len(tt.wantX) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.wantX))
			}
			for i, want := range tt.wantX {
				if got[i].X != want {
					t.Fatalf("glyph %d X = %v, want %v", i, got[i].X, want)
				}
			}
		})
	}
}
