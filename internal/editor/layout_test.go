package editor

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/harness"
)

func TestLayoutScrollAndClick(t *testing.T) {
	body := []byte("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n")
	doc := buffer.FromBytes("a.go", bytes.Repeat([]byte("a\n"), 80))
	v := NewView(doc)
	sh := text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	// Focused stays false so Layout does not run IME.
	style := Style{
		FontSize:   unit.Sp(14),
		LineHeight: 1.3,
		TabWidth:   4,
		Colors: Colors{
			Foreground: color.NRGBA{A: 255},
			Background: color.NRGBA{A: 255},
		},
	}

	d, err := harness.New(image.Pt(400, 300))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		return v.Layout(gtx, sh, style, nil)
	}
	d.Scroll(image.Pt(10, 10), f32.Pt(0, 1))
	if got := v.ScrollLine(); got != 3 {
		t.Fatalf("ScrollLine() = %d, want 3", got)
	}

	click := NewView(buffer.FromBytes("a.go", body))
	click.MoveLineEnd(false)
	cd, err := harness.New(image.Pt(400, 300))
	if err != nil {
		t.Fatal(err)
	}
	cd.Layout = func(gtx layout.Context) layout.Dimensions {
		return click.Layout(gtx, sh, style, nil)
	}
	cd.Click(image.Pt(1, 1))
	line, col := click.CaretPos()
	if line != 0 || col != 0 {
		t.Fatalf("caret line %d col %d, want 0, 0", line, col)
	}
}

func TestFoldColumnToggles(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("one\ntwo\nthree\nfour"))
	v := NewView(doc)
	v.SetFolds(doc.Version(), []Fold{{Start: 0, End: 2}})
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
	d, err := harness.New(image.Pt(400, 300))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		return v.Layout(gtx, sh, style, nil)
	}
	d.Step()
	if v.ui.linePx < 1 {
		t.Fatalf("linePx %d", v.ui.linePx)
	}
	// PxPerDp is 1. x is the center of the fold column: git stripe 4, plus 7.
	at := image.Pt(4+7, v.ui.linePx/2)
	d.Click(at)
	if got := v.lineAt(1); got == 1 {
		t.Fatal("fold column click left the next line visible")
	}
	if got := v.lineAt(1); got != 3 {
		t.Fatalf("lineAt(1) = %d, want 3", got)
	}
	d.Click(at)
	if got := v.lineAt(1); got != 1 {
		t.Fatalf("second click lineAt(1) = %d, want 1", got)
	}
}

func TestGutterCursor(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("one\ntwo\nthree\nfour"))
	v := NewView(doc)
	v.SetFolds(doc.Version(), []Fold{{Start: 0, End: 2}})
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
	d, err := harness.New(image.Pt(400, 300))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		return v.Layout(gtx, sh, style, nil)
	}
	d.Step()
	if v.ui.linePx < 1 {
		t.Fatalf("linePx %d", v.ui.linePx)
	}
	gtx := layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	gutterW := v.gutterWidth(gtx, v.Doc.Snapshot().LineCount())
	y := v.ui.linePx / 2
	// PxPerDp is 1. The fold column is the disclosure, centered at git stripe 4 plus 7.
	for _, tc := range []struct {
		name string
		at   image.Point
		want pointer.Cursor
	}{
		{name: "git stripe", at: image.Pt(1, y), want: pointer.CursorPointer},
		{name: "fold column", at: image.Pt(4+7, y), want: pointer.CursorPointer},
		{name: "line number", at: image.Pt(gutterW-1, y), want: pointer.CursorPointer},
		{name: "text", at: hitCell(v, 0, 0), want: pointer.CursorText},
	} {
		d.Move(tc.at)
		if got := d.Cursor(); got != tc.want {
			t.Errorf("%s at %v: cursor %v, want %v", tc.name, tc.at, got, tc.want)
		}
	}
}

func TestScrollbarAndEndClamp(t *testing.T) {
	sh := text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	style := Style{
		FontSize:   unit.Sp(14),
		LineHeight: 1.3,
		TabWidth:   4,
		Colors: Colors{
			Background: color.NRGBA{A: 255},
			Foreground: color.NRGBA{B: 255, A: 255},
			GutterBG:   color.NRGBA{R: 20, G: 20, B: 20, A: 255},
			GutterFG:   color.NRGBA{G: 255, A: 255},
		},
	}
	layoutOf := func(v *View) func(layout.Context) layout.Dimensions {
		return func(gtx layout.Context) layout.Dimensions {
			return v.Layout(gtx, sh, style, nil)
		}
	}

	fits := NewView(buffer.FromBytes("a.go", []byte("a\nb\nc\n")))
	fd, err := harness.New(image.Pt(400, 200))
	if err != nil {
		t.Fatal(err)
	}
	fd.Layout = layoutOf(fits)
	fd.Scroll(image.Pt(20, 20), f32.Pt(0, 1))
	if got := fits.ScrollLine(); got != 0 {
		t.Fatalf("short file scrolled to %d", got)
	}

	v := NewView(buffer.FromBytes("a.go", bytes.Repeat([]byte("a\n"), 60)))
	d, err := harness.New(image.Pt(400, 90))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = layoutOf(v)
	d.Step()
	lines := v.Doc.Snapshot().LineCount()
	if lines <= v.pageLines {
		t.Fatalf("fixture fits: %d lines, page %d", lines, v.pageLines)
	}
	maxScroll := lines - v.pageLines
	v.SetScrollLine(10000)
	if got := v.ScrollLine(); got != maxScroll {
		t.Fatalf("SetScrollLine %d, want %d", got, maxScroll)
	}
	d.Scroll(image.Pt(20, 20), f32.Pt(0, 1))
	if got := v.ScrollLine(); got != maxScroll {
		t.Fatalf("wheel past end %d, want %d", got, maxScroll)
	}

	v.SetScrollLine(0)
	d.Step()
	d.Click(image.Pt(396, 80))
	if line, _ := v.CaretPos(); line != 0 {
		t.Fatalf("scrollbar click moved caret to line %d", line)
	}
	if got := v.ScrollLine(); got <= 0 || got > maxScroll {
		t.Fatalf("track click scroll %d, want 1..%d", got, maxScroll)
	}

	t.Run("pixels", func(t *testing.T) {
		assertRightGreen(t, fd, false)
		assertRightGreen(t, d, true)
	})
}

func assertRightGreen(t *testing.T, d *harness.Desktop, want bool) {
	t.Helper()
	img, err := d.Image()
	if err != nil {
		t.Skip(err)
	}
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Max.X - 8; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.G > 120 && c.R < 40 && c.B < 40 && c.A > 100 {
				n++
			}
		}
	}
	if want && n == 0 {
		t.Fatal("no scrollbar pixels on the right edge")
	}
	if !want && n != 0 {
		t.Fatalf("short file drew %d scrollbar pixels", n)
	}
}

func TestDragSelectsText(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("hello world\n"))
	v := NewView(doc)
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
	d.Drag(image.Pt(1, 1), image.Pt(300, 1))
	if got := v.SelectedText(); got != "hello world" {
		t.Fatalf("SelectedText() = %q", got)
	}
	start, end := v.SelectionBounds()
	if start != 0 || end != 11 {
		t.Fatalf("SelectionBounds() = %d, %d, want 0, 11", start, end)
	}
}

func TestClickLandsOnCell(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("abcd\n\thello\n"))
	v := NewView(doc)
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

	// Column 2 of "abcd" is 'c'.
	d.Click(hitCell(v, 0, 2))
	line, col := v.CaretPos()
	if line != 0 || col != 2 {
		t.Fatalf("abcd caret line %d col %d, want 0, 2 (at %v)", line, col, hitCell(v, 0, 2))
	}

	// Four tab cells, then 'h' is byte column 1.
	d.Click(hitCell(v, 1, 4))
	line, col = v.CaretPos()
	if line != 1 || col != 1 {
		t.Fatalf("hello caret line %d col %d, want 1, 1 (at %v)", line, col, hitCell(v, 1, 4))
	}
}

func TestCaretAlignsAndBlinks(t *testing.T) {
	const blink = 500 * time.Millisecond
	doc := buffer.FromBytes("a.go", []byte(strings.Repeat("M", 40)))
	v := NewView(doc)
	v.MoveLineEnd(false)
	sh := text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	black := color.NRGBA{A: 255}
	style := Style{
		FontSize:   unit.Sp(14),
		LineHeight: 1.3,
		TabWidth:   4,
		Blink:      blink,
		Focused:    true,
		Colors: Colors{
			Background:    black,
			Foreground:    color.NRGBA{B: 255, A: 255},
			Caret:         color.NRGBA{R: 255, A: 255},
			LineHighlight: black,
			GutterBG:      black,
			GutterFG:      color.NRGBA{},
		},
	}
	d, err := harness.New(image.Pt(600, 80))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		return v.Layout(gtx, sh, style, nil)
	}
	d.Focus(v.Tag())
	when, ok := d.Wakeup()
	epoch := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	want := epoch.Add(blink / 2)
	if !ok || !when.Equal(want) {
		t.Fatalf("wakeup %v ok %v, want %v", when, ok, want)
	}
	t.Run("lit", func(t *testing.T) {
		assertCaretInk(t, d, true)
	})

	d.SetNow(when)
	d.Step()
	off, ok := d.Wakeup()
	if !ok || !off.Equal(when.Add(blink/2)) {
		t.Fatalf("off wakeup %v ok %v, want %v", off, ok, when.Add(blink/2))
	}
	t.Run("off", func(t *testing.T) {
		assertCaretInk(t, d, false)
	})

	d.SetNow(when.Add(250 * time.Millisecond))
	d.Step()
	back, ok := d.Wakeup()
	if !ok || !back.Equal(when.Add(500*time.Millisecond)) {
		t.Fatalf("relight wakeup %v ok %v, want %v", back, ok, when.Add(500*time.Millisecond))
	}
	t.Run("lit again", func(t *testing.T) {
		assertCaretInk(t, d, true)
	})
}

// hitCell is the center of a 0-based cell. The insets are gutterWidth, then
// textPad — the same origin Layout uses at scale 1.
func hitCell(v *View, line, cell int) image.Point {
	gtx := layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	gutterW := v.gutterWidth(gtx, v.Doc.Snapshot().LineCount())
	pad := gtx.Dp(textPad)
	cw := v.ui.charW
	linePx := v.ui.linePx
	return image.Pt(gutterW+pad+cell*cw+cw/2, line*linePx+linePx/2)
}

func assertCaretInk(t *testing.T, d *harness.Desktop, lit bool) {
	t.Helper()
	img, err := d.Image()
	if err != nil {
		t.Skip(err)
	}
	red := scanInk(img, func(c color.RGBA) bool {
		return c.A > 200 && c.R > 200 && c.G < 30 && c.B < 30
	})
	blue := scanInk(img, func(c color.RGBA) bool {
		return c.A > 200 && c.B > 200 && c.R < 40 && c.G < 40
	})
	t.Logf("red n=%d x=%d..%d y=%d..%d blue n=%d x=%d..%d y=%d..%d",
		red.n, red.minX, red.maxX, red.minY, red.maxY,
		blue.n, blue.minX, blue.maxX, blue.minY, blue.maxY)
	if blue.n == 0 {
		t.Fatal("no blue text pixels")
	}
	if !lit {
		if red.n != 0 {
			t.Fatalf("blink off still has %d red pixels at x %d..%d y %d..%d", red.n, red.minX, red.maxX, red.minY, red.maxY)
		}
		return
	}
	if red.n == 0 {
		t.Fatal("no red caret pixels")
	}
	if red.minY > blue.maxY || blue.minY > red.maxY {
		t.Fatalf("caret y %d..%d does not overlap text y %d..%d", red.minY, red.maxY, blue.minY, blue.maxY)
	}
	if red.minX <= blue.minX {
		t.Fatalf("caret minX %d is left of text x %d..%d", red.minX, blue.minX, blue.maxX)
	}
	gap := red.minX - blue.maxX
	if gap < 0 {
		gap = -gap
	}
	if gap > 6 {
		t.Fatalf("caret minX %d is %dpx from blue maxX %d (text x %d..%d, caret x %d..%d)",
			red.minX, gap, blue.maxX, blue.minX, blue.maxX, red.minX, red.maxX)
	}
}

type inkBox struct {
	n          int
	minX, minY int
	maxX, maxY int
}

func TestMarkPixels(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("aaaa\nbbbb    \ncccc\n"))
	v := NewView(doc)
	bg := color.NRGBA{R: 16, G: 16, B: 16, A: 255}
	note := color.NRGBA{R: 220, G: 40, B: 40, A: 80}
	sel := color.NRGBA{R: 0, G: 0, B: 200, A: 255}
	style := Style{
		FontSize:   unit.Sp(14),
		LineHeight: 1.3,
		TabWidth:   4,
		Colors: Colors{
			Background:    bg,
			Foreground:    color.NRGBA{R: 255, G: 255, B: 255, A: 255},
			Selection:     sel,
			LineHighlight: bg,
			MarkNote:      note,
		},
	}
	sh := text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	start, end := LinesMark(doc.Snapshot(), 1, 1)
	v.SetMarks([]Mark{{Start: start, End: end, Kind: MarkNote}})

	d, err := harness.New(image.Pt(400, 120))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		return v.Layout(gtx, sh, style, nil)
	}
	d.Step()
	img, err := d.Image()
	if err != nil {
		t.Skip(err)
	}
	yMark := v.ui.linePx + v.ui.linePx/2
	yPlain := 2*v.ui.linePx + v.ui.linePx/2
	x := 360
	blend := linearBlend(note, bg)
	if got := img.RGBAAt(x, yMark); !nearNRGBA(got, blend) {
		t.Fatalf("mark pixel %v, want %v", got, blend)
	}
	if got := img.RGBAAt(x, yPlain); !nearNRGBA(got, bg) {
		t.Fatalf("plain pixel %v, want %v", got, bg)
	}
	origin := hitCell(v, 1, 1)
	if !glyphAboveMark(img, origin, blend) {
		t.Fatal("glyphs did not paint over the mark")
	}

	// Spaces after "bbbb" on line 1. The selection is opaque and covers the mark.
	v.SetSelections([]Sel{{Anchor: 9, Head: 13}})
	d.Step()
	img, err = d.Image()
	if err != nil {
		t.Skip(err)
	}
	space := hitCell(v, 1, 5)
	if got := img.RGBAAt(space.X, space.Y); !nearNRGBA(got, sel) {
		t.Fatalf("selection pixel at %v is %v, want %v", space, got, sel)
	}
}

// linearBlend is Gio's source-over composite. FillShape blends in linear sRGB.
func linearBlend(src, dst color.NRGBA) color.NRGBA {
	toLin := func(c uint8) float64 {
		x := float64(c) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	fromLin := func(x float64) uint8 {
		if x <= 0.0031308 {
			x *= 12.92
		} else {
			x = 1.055*math.Pow(x, 1/2.4) - 0.055
		}
		n := max(int(math.Round(x*255)), 0)
		if n > 255 {
			n = 255
		}
		return uint8(n)
	}
	a := float64(src.A) / 255
	mix := func(s, d uint8) uint8 {
		return fromLin(toLin(s)*a + toLin(d)*(1-a))
	}
	return color.NRGBA{R: mix(src.R, dst.R), G: mix(src.G, dst.G), B: mix(src.B, dst.B), A: 255}
}

func nearNRGBA(got color.RGBA, want color.NRGBA) bool {
	d := func(a, b uint8) int {
		n := int(a) - int(b)
		if n < 0 {
			return -n
		}
		return n
	}
	return d(got.R, want.R) <= 2 && d(got.G, want.G) <= 2 && d(got.B, want.B) <= 2 && d(got.A, want.A) <= 2
}

func glyphAboveMark(img *image.RGBA, origin image.Point, blend color.NRGBA) bool {
	base := int(blend.R) + int(blend.G) + int(blend.B)
	for dy := -8; dy <= 8; dy++ {
		for dx := -6; dx <= 6; dx++ {
			p := img.RGBAAt(origin.X+dx, origin.Y+dy)
			if int(p.R)+int(p.G)+int(p.B) > base+80 {
				return true
			}
		}
	}
	return false
}

func scanInk(img *image.RGBA, match func(color.RGBA) bool) inkBox {
	b := img.Bounds()
	out := inkBox{minX: b.Max.X, minY: b.Max.Y, maxX: -1, maxY: -1}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if !match(img.RGBAAt(x, y)) {
				continue
			}
			out.n++
			if x < out.minX {
				out.minX = x
			}
			if y < out.minY {
				out.minY = y
			}
			if x > out.maxX {
				out.maxX = x
			}
			if y > out.maxY {
				out.maxY = y
			}
		}
	}
	return out
}
