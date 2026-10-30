package ui

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/russoj88/gio-ide/internal/harness"
)

func TestClickHit(t *testing.T) {
	d, err := harness.New(image.Pt(80, 80))
	if err != nil {
		t.Fatal(err)
	}
	var tag struct{}
	var hit bool
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		dims, clicked := Click(gtx, &tag, color.NRGBA{A: 255}, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(40, 40)}
		})
		hit = clicked
		return dims
	}
	// The hit area is the child rectangle at the origin, (0,0)-(40,40).
	d.Click(image.Pt(10, 10))
	if !hit {
		t.Fatal("click at (10, 10) = false, want true")
	}
	d.Click(image.Pt(40, 40))
	if hit {
		t.Fatal("click at (40, 40) = true, want false")
	}
}

func TestClickInsideFlexStaysInTheButton(t *testing.T) {
	box := func(gtx layout.Context, tag event.Tag, size image.Point) bool {
		stack := clip.Rect{Max: size}.Push(gtx.Ops)
		event.Op(gtx.Ops, tag)
		hit := pressed(gtx, tag)
		stack.Pop()
		return hit
	}

	// One clip, translated. Nothing is registered before it, so a hit can
	// only come from the row at x=[40,240), y=[0,40).
	t.Run("single offset", func(t *testing.T) {
		d, err := harness.New(image.Pt(400, 200))
		if err != nil {
			t.Fatal(err)
		}
		var row struct{}
		var rowHit bool
		d.Layout = func(gtx layout.Context) layout.Dimensions {
			off := op.Offset(image.Pt(40, 0)).Push(gtx.Ops)
			rowHit = box(gtx, &row, image.Pt(200, 40))
			off.Pop()
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}
		checks := []struct {
			at   image.Point
			want bool
		}{
			{image.Pt(10, 10), false},
			{image.Pt(90, 10), true},
			{image.Pt(250, 10), false},
			{image.Pt(10, 80), false},
		}
		for _, c := range checks {
			d.Click(c.at)
			t.Logf("%v row=%v", c.at, rowHit)
			if rowHit != c.want {
				t.Errorf("click at %v row=%v, want %v", c.at, rowHit, c.want)
			}
		}
	})

	// Tool at the origin, then the row beside it. A press in the row must
	// not be delivered to the earlier clip.
	t.Run("sibling clips", func(t *testing.T) {
		d, err := harness.New(image.Pt(400, 200))
		if err != nil {
			t.Fatal(err)
		}
		var tool, row byte
		var toolHit, rowHit bool
		d.Layout = func(gtx layout.Context) layout.Dimensions {
			toolHit = box(gtx, &tool, image.Pt(40, 40))
			off := op.Offset(image.Pt(40, 0)).Push(gtx.Ops)
			rowHit = box(gtx, &row, image.Pt(200, 40))
			off.Pop()
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}
		checks := []struct {
			at        image.Point
			tool, row bool
		}{
			{at: image.Pt(10, 10), tool: true, row: false},
			{at: image.Pt(90, 10), tool: false, row: true},
			{at: image.Pt(250, 10), tool: false, row: false},
			{at: image.Pt(10, 80), tool: false, row: false},
		}
		for _, c := range checks {
			d.Click(c.at)
			t.Logf("%v tool=%v row=%v", c.at, toolHit, rowHit)
			if toolHit != c.tool || rowHit != c.row {
				t.Errorf("click at %v tool=%v row=%v, want tool=%v row=%v", c.at, toolHit, rowHit, c.tool, c.row)
			}
		}
	})
}
