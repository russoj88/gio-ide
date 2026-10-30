package terminal

import (
	"image"
	"testing"

	"gioui.org/io/pointer"
	"gioui.org/layout"

	"github.com/russoj88/gio-ide/internal/harness"
)

func TestClickDoesNotHighlightACell(t *testing.T) {
	term := New(1, 100, nil)
	var g Grid
	g.Term = term
	g.Style.FontSize = 13
	d, err := harness.New(image.Pt(400, 200))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		return g.Layout(gtx)
	}
	d.Step()
	if g.charW < 2 || g.linePx < 2 {
		t.Fatalf("cell %d x %d", g.charW, g.linePx)
	}
	cell := image.Pt(g.charW/2, g.linePx/2)
	d.Click(cell)
	if _, _, _, _, ok := term.Selection(); ok {
		t.Fatal("click left a selection")
	}

	d.Pointer(pointer.Press, cell, pointer.ButtonPrimary)
	if _, _, _, _, ok := term.Selection(); ok {
		t.Fatal("press left a selection")
	}
	d.Pointer(pointer.Move, image.Pt(cell.X+g.charW*3, cell.Y), pointer.ButtonPrimary)
	x0, y0, x1, y1, ok := term.Selection()
	if !ok || x0 != 0 || y0 != 0 || x1 != 3 || y1 != 0 {
		t.Fatalf("drag selection %d,%d %d,%d ok=%v", x0, y0, x1, y1, ok)
	}
	d.Pointer(pointer.Release, image.Pt(cell.X+g.charW*3, cell.Y), 0)
	if _, _, _, _, ok := term.Selection(); !ok {
		t.Fatal("release cleared the drag")
	}

	d.Click(cell)
	if _, _, _, _, ok := term.Selection(); ok {
		t.Fatal("second click left a selection")
	}
}
