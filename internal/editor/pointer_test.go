package editor

import (
	"image"
	"image/color"
	"testing"
	"time"

	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/harness"
)

func TestDoubleClickSelectsWord(t *testing.T) {
	v, d := layoutClicks(t, "hello world\n")
	// Time 0 means unset, so the first press is after the epoch.
	d.Advance(100 * time.Millisecond)
	at := hitCell(v, 0, 2)
	d.Click(at)
	assertCaretColumn(t, v, 2)

	d.Advance(100 * time.Millisecond)
	// Press, move with the button held, then release. The move is a Drag.
	d.Drag(at, hitCell(v, 0, 8))
	got := v.Selections()
	if len(got) != 1 || got[0].Anchor != 0 || got[0].Head != 5 {
		t.Fatalf("selection = %+v, want anchor 0 head 5", got)
	}
	if text := v.SelectedText(); text != "hello" {
		t.Fatalf("SelectedText() = %q, want hello", text)
	}
	if v.ui.suppressDrag || v.ui.dragging {
		t.Fatalf("after release suppress=%v dragging=%v", v.ui.suppressDrag, v.ui.dragging)
	}
}

func TestSameTimePressDoesNotSelectWord(t *testing.T) {
	v, d := layoutClicks(t, "hello world\n")
	at := hitCell(v, 0, 2)
	d.Click(at)
	d.Click(at)
	assertCaretColumn(t, v, 2)

	d.Advance(100 * time.Millisecond)
	d.Click(at)
	assertCaretColumn(t, v, 2)
	d.Click(at)
	assertCaretColumn(t, v, 2)
}

func TestSlowPressDoesNotSelectWord(t *testing.T) {
	v, d := layoutClicks(t, "hello world\n")
	d.Advance(100 * time.Millisecond)
	at := hitCell(v, 0, 2)
	d.Click(at)
	assertCaretColumn(t, v, 2)
	d.Advance(500 * time.Millisecond)
	d.Click(at)
	assertCaretColumn(t, v, 2)
}

func assertCaretColumn(t *testing.T, v *View, col int) {
	t.Helper()
	line, got := v.CaretPos()
	if line != 0 || got != col {
		t.Fatalf("caret line %d col %d, want 0, %d", line, got, col)
	}
	start, end := v.SelectionBounds()
	if start != end || start != col {
		t.Fatalf("SelectionBounds() = %d, %d, want caret at %d", start, end, col)
	}
}

func layoutClicks(t *testing.T, body string) (*View, *harness.Desktop) {
	t.Helper()
	v := NewView(buffer.FromBytes("a.go", []byte(body)))
	sh := text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	style := Style{
		FontSize:   unit.Sp(14),
		LineHeight: 1.3,
		TabWidth:   4,
		Colors: Colors{
			Foreground: color.NRGBA{A: 255},
			Background: color.NRGBA{A: 255},
		},
	}
	d, err := harness.New(image.Pt(400, 80))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		return v.Layout(gtx, sh, style, nil)
	}
	d.Step()
	if v.ui.charW < 1 || v.ui.linePx < 1 {
		t.Fatalf("charW %d linePx %d", v.ui.charW, v.ui.linePx)
	}
	return v, d
}
