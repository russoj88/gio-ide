package ui

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/font/gofont"
	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/harness"
)

func TestListClick(t *testing.T) {
	d, sh := newListDesktop(t, image.Pt(200, 80))
	rows := []string{"one", "two"}
	// A field keeps each tag a distinct pointer. struct{} aliases in Gio.
	var tags [2]struct{ n byte }
	var picked int
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		dims, clicked := List(gtx, sh, unit.Sp(12), rows, -1, listFG, listSel, listBG, nil, func(i int) event.Tag {
			return &tags[i]
		})
		picked = clicked
		return dims
	}
	checks := []struct {
		at   image.Point
		want int
	}{
		{image.Pt(8, 10), 0},
		{image.Pt(8, 36), 1},
		{image.Pt(8, 60), -1},
	}
	for _, c := range checks {
		d.Click(c.at)
		if picked != c.want {
			t.Errorf("click at %v = %d, want %d", c.at, picked, c.want)
		}
	}
}

func TestListNilTagIgnoresClick(t *testing.T) {
	d, sh := newListDesktop(t, image.Pt(200, 80))
	var picked int
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		dims, clicked := List(gtx, sh, unit.Sp(12), []string{"one", "two"}, -1, listFG, listSel, listBG, nil, nil)
		picked = clicked
		return dims
	}
	d.Click(image.Pt(8, 10))
	if picked != -1 {
		t.Fatalf("click at (8, 10) = %d, want -1", picked)
	}
}

func TestListActiveNilTag(t *testing.T) {
	d, sh := newListDesktop(t, image.Pt(200, 80))
	var picked int
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		dims, clicked := List(gtx, sh, unit.Sp(12), []string{"one", "two"}, 1, listFG, listSel, listBG, nil, nil)
		picked = clicked
		return dims
	}
	d.Click(image.Pt(8, 36))
	if picked != -1 {
		t.Fatalf("active row click = %d, want -1", picked)
	}
}

func newListDesktop(t *testing.T, size image.Point) (*harness.Desktop, *text.Shaper) {
	t.Helper()
	d, err := harness.New(size)
	if err != nil {
		t.Fatal(err)
	}
	sh := text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	return d, sh
}

var (
	listFG  = color.NRGBA{R: 0xFF, A: 0xFF}
	listSel = color.NRGBA{G: 0x80, A: 0xFF}
	listBG  = color.NRGBA{B: 0x40, A: 0xFF}
)
