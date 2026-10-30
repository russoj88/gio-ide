package app

import (
	"image"
	"testing"

	"gioui.org/font/gofont"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/text"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/harness"
	"github.com/russoj88/gio-ide/internal/settings"
	"github.com/russoj88/gio-ide/internal/terminal"
	"github.com/russoj88/gio-ide/internal/theme"
)

func TestClampTermHeight(t *testing.T) {
	if got := clampTermH(10, 80, 400); got != 80 {
		t.Fatalf("min %d", got)
	}
	if got := clampTermH(900, 80, 400); got != 400 {
		t.Fatalf("max %d", got)
	}
	if got := clampTermH(200, 80, 400); got != 200 {
		t.Fatalf("mid %d", got)
	}
	if got := clampTermH(10, 80, 40); got != 80 {
		t.Fatalf("max below min %d", got)
	}
}

func TestTerminalSashDragResizesThePanel(t *testing.T) {
	s := &shell{
		bus:      bus.New(nil),
		store:    settings.NewStore("", ""),
		docs:     map[uint64]*buffer.Doc{},
		rootPane: &pane{ratio: 0.5},
		theme:    theme.Dark(),
		shaper:   text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection())),
		tags:     map[string]*handlerTag{},
	}
	s.focusPane = s.rootPane
	s.bottomOpen = true
	s.bottomTool = "terminal"
	term := terminal.New(1, 100, nil)
	tab := &termTab{term: term}
	tab.grid.Term = term
	s.terms = []*termTab{tab}

	d, err := harness.New(image.Pt(1280, 800))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		return s.layout(gtx)
	}
	d.Step()
	if s.termSashY < 100 {
		t.Fatalf("sash Y %d", s.termSashY)
	}
	y := s.termSashY + 2
	d.Pointer(pointer.Press, image.Pt(700, y), pointer.ButtonPrimary)
	if s.termH != 0 {
		t.Fatalf("press set height %d", s.termH)
	}
	d.Pointer(pointer.Move, image.Pt(700, y-80), pointer.ButtonPrimary)
	if s.termH != 240+80 {
		t.Fatalf("drag up height %d, want %d (sash was %d)", s.termH, 240+80, y-2)
	}
	d.Pointer(pointer.Release, image.Pt(700, y-80), 0)

	d.Step()
	y = s.termSashY + 2
	before := s.termH
	d.Pointer(pointer.Press, image.Pt(700, y), pointer.ButtonPrimary)
	d.Pointer(pointer.Move, image.Pt(700, y+40), pointer.ButtonPrimary)
	if s.termH != before-40 {
		t.Fatalf("drag down height %d, want %d", s.termH, before-40)
	}
	d.Pointer(pointer.Move, image.Pt(700, y+4000), pointer.ButtonPrimary)
	if s.termH != 80 {
		t.Fatalf("clamp height %d, want 80", s.termH)
	}
}
