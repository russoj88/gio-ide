package harness

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

func TestNewRejectsEmptySize(t *testing.T) {
	_, err := New(image.Pt(0, 10))
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestClickInsideAndOutside(t *testing.T) {
	d := newDesk(t, image.Pt(100, 100))
	var tag struct{}
	var clicked bool
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		clicked = false
		for {
			ev, ok := gtx.Event(pointer.Filter{Target: &tag, Kinds: pointer.Press})
			if !ok {
				break
			}
			if e, ok := ev.(pointer.Event); ok && e.Kind == pointer.Press {
				clicked = true
			}
		}
		defer clip.Rect(image.Rectangle{Max: image.Pt(40, 40)}).Push(gtx.Ops).Pop()
		event.Op(gtx.Ops, &tag)
		return layout.Dimensions{Size: image.Pt(40, 40)}
	}

	cases := []struct {
		at   image.Point
		want bool
	}{
		{image.Pt(10, 10), true},
		{image.Pt(80, 80), false},
	}
	for _, tc := range cases {
		d.Click(tc.at)
		if clicked != tc.want {
			t.Fatalf("click %v = %v, want %v", tc.at, clicked, tc.want)
		}
	}
}

func TestKeyPressRequiresFocus(t *testing.T) {
	d := newDesk(t, image.Pt(20, 20))
	var tag struct{}
	var presses int
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		defer clip.Rect(image.Rectangle{Max: image.Pt(20, 20)}).Push(gtx.Ops).Pop()
		event.Op(gtx.Ops, &tag)
		// FocusFilter is what keeps the tag focused after Frame. key.Filter does not.
		for {
			ev, ok := gtx.Event(
				key.Filter{Focus: &tag, Name: key.NameReturn},
				key.FocusFilter{Target: &tag},
			)
			if !ok {
				break
			}
			if e, ok := ev.(key.Event); ok && e.State == key.Press {
				presses++
			}
		}
		return layout.Dimensions{}
	}
	d.Press(key.NameReturn, 0)
	if presses != 0 {
		t.Fatalf("press without focus = %d", presses)
	}
	d.Focus(&tag)
	d.Press(key.NameReturn, 0)
	if presses != 1 {
		t.Fatalf("press with focus = %d, want 1", presses)
	}
}

func TestEditDeliversText(t *testing.T) {
	d := newDesk(t, image.Pt(20, 20))
	var tag struct{}
	var got key.EditEvent
	var have bool
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		defer clip.Rect(image.Rectangle{Max: image.Pt(20, 20)}).Push(gtx.Ops).Pop()
		event.Op(gtx.Ops, &tag)
		for {
			ev, ok := gtx.Event(key.FocusFilter{Target: &tag})
			if !ok {
				break
			}
			if e, ok := ev.(key.EditEvent); ok {
				got = e
				have = true
			}
		}
		return layout.Dimensions{}
	}
	d.Focus(&tag)
	d.Edit(1, 3, "ab")
	if !have {
		t.Fatal("no edit event")
	}
	if got.Range.Start != 1 || got.Range.End != 3 || got.Text != "ab" {
		t.Fatalf("edit = %+v", got)
	}
}

func TestScrollDeliversDelta(t *testing.T) {
	d := newDesk(t, image.Pt(20, 20))
	var tag struct{}
	var got f32.Point
	var have bool
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		defer clip.Rect(image.Rectangle{Max: image.Pt(20, 20)}).Push(gtx.Ops).Pop()
		event.Op(gtx.Ops, &tag)
		for {
			ev, ok := gtx.Event(pointer.Filter{
				Target:  &tag,
				Kinds:   pointer.Scroll,
				ScrollY: pointer.ScrollRange{Min: -1e6, Max: 1e6},
				ScrollX: pointer.ScrollRange{Min: -1e6, Max: 1e6},
			})
			if !ok {
				break
			}
			if e, ok := ev.(pointer.Event); ok && e.Kind == pointer.Scroll {
				got = e.Scroll
				have = true
			}
		}
		return layout.Dimensions{Size: image.Pt(20, 20)}
	}
	d.Scroll(image.Pt(5, 5), f32.Pt(1, -2))
	if !have {
		t.Fatal("no scroll event")
	}
	if got.X != 1 || got.Y != -2 {
		t.Fatalf("scroll = %v", got)
	}
}

func TestDragAndMove(t *testing.T) {
	d := newDesk(t, image.Pt(20, 20))
	var tag struct{}
	var kinds []pointer.Kind
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		defer clip.Rect(image.Rectangle{Max: image.Pt(20, 20)}).Push(gtx.Ops).Pop()
		event.Op(gtx.Ops, &tag)
		for {
			ev, ok := gtx.Event(pointer.Filter{
				Target: &tag,
				Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Move,
			})
			if !ok {
				break
			}
			if e, ok := ev.(pointer.Event); ok {
				kinds = append(kinds, e.Kind)
			}
		}
		return layout.Dimensions{Size: image.Pt(20, 20)}
	}
	d.Drag(image.Pt(1, 1), image.Pt(4, 4))
	want := []pointer.Kind{pointer.Press, pointer.Drag, pointer.Release}
	if len(kinds) != len(want) {
		t.Fatalf("drag kinds %v", kinds)
	}
	for i, k := range want {
		if kinds[i] != k {
			t.Fatalf("drag kinds %v", kinds)
		}
	}
	kinds = nil
	d.Move(image.Pt(2, 2))
	if len(kinds) != 1 || kinds[0] != pointer.Move {
		t.Fatalf("move kinds %v", kinds)
	}
}

func TestWakeupImmediate(t *testing.T) {
	d := newDesk(t, image.Pt(8, 8))
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	d.Step()
	when, ok := d.Wakeup()
	if !ok || !when.IsZero() {
		t.Fatalf("wakeup %v ok %v, want zero time", when, ok)
	}
}

func TestImageRedPixel(t *testing.T) {
	d := newDesk(t, image.Pt(8, 8))
	red := color.NRGBA{R: 255, A: 255}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, red, clip.Rect{Max: image.Pt(8, 8)}.Op())
		return layout.Dimensions{Size: image.Pt(8, 8)}
	}
	d.Step()
	img, err := d.Image()
	if err != nil {
		t.Skip(err)
	}
	c := img.RGBAAt(4, 4)
	if c.R < 250 || c.G > 5 || c.B > 5 || c.A < 250 {
		t.Fatalf("center pixel = %+v, want red", c)
	}
}

func newDesk(t *testing.T, size image.Point) *Desktop {
	t.Helper()
	d, err := New(size)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
