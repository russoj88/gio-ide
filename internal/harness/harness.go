// Package harness drives one Gio desktop frame in tests.
// It routes mouse and keyboard input the way a Linux desktop window does,
// without opening a window. Call a Desktop from the test goroutine only.
package harness

import (
	"errors"
	"fmt"
	"image"
	"time"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// epoch is the clock of a new Desktop and the base for pointer timestamps.
var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Desktop is one desktop frame. Pointer positions are window pixels.
// The pointer source is a mouse.
type Desktop struct {
	// Layout is invoked on every frame. Tests set it before Step.
	Layout func(layout.Context) layout.Dimensions

	size   image.Point
	metric unit.Metric
	now    time.Time

	router input.Router
	ops    *op.Ops

	framed bool
	wake   time.Time
	wakeOK bool

	focusTag event.Tag
	focusSet bool
}

// New returns a desktop of the given pixel size. The scale is 1 px per dp and
// per sp. The clock starts at a fixed time.
func New(size image.Point) (*Desktop, error) {
	if size.X < 1 || size.Y < 1 {
		return nil, fmt.Errorf("harness: window size must be positive, got %v", size)
	}
	return &Desktop{
		size:   size,
		metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		now:    epoch,
		ops:    new(op.Ops),
	}, nil
}

// Resize sets the frame size used by the next Step. size is in pixels.
func (d *Desktop) Resize(size image.Point) {
	if size.X < 1 || size.Y < 1 {
		panic("harness: window size must be positive")
	}
	d.size = size
}

// SetNow sets the clock read by the next frame.
func (d *Desktop) SetNow(t time.Time) {
	d.now = t
}

// Advance moves the clock forward. It does not run a frame.
func (d *Desktop) Advance(delta time.Duration) {
	d.now = d.now.Add(delta)
}

// Wakeup reports the invalidate time captured by the last Step.
// An immediate invalidate is the zero time with ok set.
func (d *Desktop) Wakeup() (time.Time, bool) {
	return d.wake, d.wakeOK
}

// Step lays out the frame and installs its handlers.
// A later Click, Press, or Edit delivers against those handlers.
func (d *Desktop) Step() layout.Dimensions {
	if d.Layout == nil {
		panic("harness: Desktop.Layout is nil")
	}
	d.ops.Reset()
	gtx := layout.Context{
		Ops:         d.ops,
		Constraints: layout.Exact(d.size),
		Metric:      d.metric,
		Now:         d.now,
		Source:      d.router.Source(),
	}
	if d.focusSet {
		gtx.Execute(key.FocusCmd{Tag: d.focusTag})
		d.focusTag = nil
		d.focusSet = false
	}
	dims := d.Layout(gtx)
	d.router.Frame(d.ops)
	d.framed = true
	// WakeupTime clears the router's flag, so cache it for Wakeup.
	d.wake, d.wakeOK = d.router.WakeupTime()
	return dims
}

// Click delivers a primary-button press and release at at, then runs a frame.
func (d *Desktop) Click(at image.Point) {
	d.deliver(
		d.pointer(pointer.Press, at, pointer.ButtonPrimary, f32.Point{}),
		d.pointer(pointer.Release, at, 0, f32.Point{}),
	)
}

// Move delivers a mouse move at at, then runs a frame.
func (d *Desktop) Move(at image.Point) {
	d.deliver(d.pointer(pointer.Move, at, 0, f32.Point{}))
}

// Cursor is the mouse cursor under the last pointer event.
func (d *Desktop) Cursor() pointer.Cursor {
	return d.router.Cursor()
}

// Pointer delivers one mouse event and runs a frame.
// A Move with the primary button held is delivered to widgets as a drag.
func (d *Desktop) Pointer(kind pointer.Kind, at image.Point, buttons pointer.Buttons) {
	d.deliver(d.pointer(kind, at, buttons, f32.Point{}))
}

// Drag delivers a primary-button press at from, a drag to to, and a release, then runs a frame.
// The motion is a move with the button held. The router delivers that as pointer.Drag;
// a queued Drag is rejected.
func (d *Desktop) Drag(from, to image.Point) {
	d.deliver(
		d.pointer(pointer.Press, from, pointer.ButtonPrimary, f32.Point{}),
		d.pointer(pointer.Move, to, pointer.ButtonPrimary, f32.Point{}),
		d.pointer(pointer.Release, to, 0, f32.Point{}),
	)
}

// Scroll delivers a scroll at at. delta uses Gio's pointer scroll sign, then a frame runs.
func (d *Desktop) Scroll(at image.Point, delta f32.Point) {
	d.deliver(d.pointer(pointer.Scroll, at, 0, delta))
}

// Focus requests key focus for tag and runs a frame so the request is applied.
func (d *Desktop) Focus(tag event.Tag) {
	d.focusTag = tag
	d.focusSet = true
	d.Step()
}

// Press delivers a key press and release to the focused handler, then runs a frame.
func (d *Desktop) Press(name key.Name, mods key.Modifiers) {
	d.deliver(
		key.Event{Name: name, Modifiers: mods, State: key.Press},
		key.Event{Name: name, Modifiers: mods, State: key.Release},
	)
}

// Edit delivers a text replacement to the focused handler, then runs a frame.
// start and end are rune offsets, matching key.EditEvent.
func (d *Desktop) Edit(start, end int, text string) {
	d.deliver(key.EditEvent{Range: key.Range{Start: start, End: end}, Text: text})
}

// Queue delivers ev in one frame, then runs that frame.
// A Linux letter is a key event and an edit event in the same call.
func (d *Desktop) Queue(ev ...event.Event) {
	d.deliver(ev...)
}

// Image renders the last frame. It returns an error when no frame has run,
// or when the machine has no surfaceless desktop GPU. Callers skip on that error.
func (d *Desktop) Image() (*image.RGBA, error) {
	if !d.framed {
		return nil, errors.New("harness: no frame to read")
	}
	w, err := headless.NewWindow(d.size.X, d.size.Y)
	if err != nil {
		return nil, fmt.Errorf("harness: desktop GPU: %w", err)
	}
	defer w.Release()
	if err := w.Frame(d.ops); err != nil {
		return nil, fmt.Errorf("harness: desktop GPU: %w", err)
	}
	img := image.NewRGBA(image.Rectangle{Max: w.Size()})
	if err := w.Screenshot(img); err != nil {
		return nil, fmt.Errorf("harness: desktop GPU: %w", err)
	}
	return img, nil
}

// deliver queues events against the handlers from the previous frame, then
// runs a frame so Layout can read them. The first call frames once up front
// when the desktop has never been stepped.
func (d *Desktop) deliver(ev ...event.Event) {
	if !d.framed {
		d.Step()
	}
	d.router.Queue(ev...)
	d.Step()
}

func (d *Desktop) pointer(kind pointer.Kind, at image.Point, buttons pointer.Buttons, scroll f32.Point) pointer.Event {
	return pointer.Event{
		Kind:     kind,
		Source:   pointer.Mouse,
		Buttons:  buttons,
		Time:     d.now.Sub(epoch),
		Position: f32.Pt(float32(at.X), float32(at.Y)),
		Scroll:   scroll,
	}
}
