package editor

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/harness"
)

func TestMarkdownWheelScrolls(t *testing.T) {
	var tag byte
	var y int
	src := []byte(strings.Repeat("A paragraph of preview text that wraps.\n\n", 40))
	sh := text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	st := Style{
		FontSize:   unit.Sp(14),
		LineHeight: 1.3,
		Colors: Colors{
			Background: color.NRGBA{R: 30, G: 31, B: 34, A: 255},
			Foreground: color.NRGBA{R: 200, G: 200, B: 200, A: 255},
		},
	}
	accent := color.NRGBA{R: 53, G: 116, B: 240, A: 255}
	dim := color.NRGBA{R: 140, G: 140, B: 140, A: 255}
	panel := color.NRGBA{R: 43, G: 45, B: 48, A: 255}
	d, err := harness.New(image.Pt(320, 80))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		dims, ny := LayoutMarkdown(gtx, sh, st, src, y, accent, dim, panel, &tag)
		y = ny
		return dims
	}
	d.Scroll(image.Pt(20, 20), f32.Pt(0, 1))
	if y == 0 {
		t.Fatal("wheel did not scroll the preview")
	}
	d.Scroll(image.Pt(20, 20), f32.Pt(0, -1))
	if y != 0 {
		t.Fatalf("scroll back y = %d, want 0", y)
	}
}
