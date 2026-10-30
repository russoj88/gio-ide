package editor

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/harness"
)

func TestDiffScrollClamps(t *testing.T) {
	sh := text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	st := diffTestStyle()
	dim := diffTestDim()
	long := strings.Repeat("M", 400)
	rows := make([]DiffRow, 40)
	for i := range rows {
		kind := byte('~')
		if i%2 == 0 {
			kind = '+'
		}
		text := long
		if i%7 == 0 {
			text = ""
		}
		rows[i] = DiffRow{
			Left:  DiffSide{Line: i + 1, Text: text, Kind: '-'},
			Right: DiffSide{Line: i + 1, Text: text, Kind: kind},
		}
	}
	d, err := harness.New(image.Pt(200, 80))
	if err != nil {
		t.Fatal(err)
	}
	var gotX, gotY int
	var dims layout.Dimensions
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		dims, gotX, gotY = LayoutDiff(gtx, sh, st, rows, 1_000_000, 1_000_000, dim, nil)
		return dims
	}
	d.Step()
	if dims.Size != image.Pt(200, 80) {
		t.Fatalf("viewport = %v, want 200x80", dims.Size)
	}
	if gotX <= 0 || gotX >= 1_000_000 {
		t.Fatalf("scrollX = %d, want clamped above 0", gotX)
	}
	if gotY <= 0 || gotY >= 1_000_000 {
		t.Fatalf("scrollY = %d, want clamped above 0", gotY)
	}
	clampedX, clampedY := gotX, gotY
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		dims, gotX, gotY = LayoutDiff(gtx, sh, st, rows, clampedX+5000, clampedY+5000, dim, nil)
		return dims
	}
	d.Step()
	if gotX != clampedX || gotY != clampedY {
		t.Fatalf("scroll = %d,%d, want clamped %d,%d", gotX, gotY, clampedX, clampedY)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		dims, gotX, gotY = LayoutDiff(gtx, sh, st, rows, 10, 10, dim, nil)
		return dims
	}
	d.Step()
	if gotX != 10 || gotY != 10 {
		t.Fatalf("in-range scroll = %d,%d, want 10,10", gotX, gotY)
	}

	short := []DiffRow{{
		Left:  DiffSide{Line: 1, Text: "hi", Kind: ' '},
		Right: DiffSide{Line: 1, Text: "hi", Kind: ' '},
	}}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		dims, gotX, gotY = LayoutDiff(gtx, sh, st, short, 1_000_000, 1_000_000, dim, nil)
		return dims
	}
	d.Step()
	if gotX != 0 || gotY != 0 {
		t.Fatalf("short diff scroll = %d,%d, want 0,0", gotX, gotY)
	}
}

func TestDiffWheelScrolls(t *testing.T) {
	var tag byte
	sh := text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	st := diffTestStyle()
	dim := diffTestDim()
	line := strings.Repeat("added line ", 40)
	rows := make([]DiffRow, 40)
	for i := range rows {
		rows[i] = DiffRow{Right: DiffSide{Line: i + 1, Text: line, Kind: '+'}}
	}
	var x, y int
	d, err := harness.New(image.Pt(320, 80))
	if err != nil {
		t.Skip(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		dims, nx, ny := LayoutDiff(gtx, sh, st, rows, x, y, dim, &tag)
		x, y = nx, ny
		return dims
	}
	d.Scroll(image.Pt(20, 20), f32.Pt(0, 1))
	if y <= 0 {
		t.Fatal("wheel did not increase scrollY")
	}
	down := y
	d.Scroll(image.Pt(20, 20), f32.Pt(0, -1))
	if y >= down {
		t.Fatalf("negative Scroll.Y scrollY = %d, want less than %d", y, down)
	}
	if y != 0 {
		t.Fatalf("scroll back y = %d, want 0", y)
	}
}

func TestDiffModifiedRowDoesNotPanic(t *testing.T) {
	sh := text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	rows := []DiffRow{{
		Left: DiffSide{
			Line: 1, Text: "hello world", Kind: '~',
			Spans: []DiffSpan{{Start: 6, End: 11, Kind: '-'}},
		},
		Right: DiffSide{
			Line: 1, Text: "hello there", Kind: '~',
			Spans: []DiffSpan{{Start: 6, End: 11, Kind: '+'}},
		},
	}}
	d, err := harness.New(image.Pt(400, 120))
	if err != nil {
		t.Fatal(err)
	}
	var x, y int
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		_, x, y = LayoutDiff(gtx, sh, diffTestStyle(), rows, 1<<20, 1<<20, diffTestDim(), nil)
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	d.Step()
	if x < 0 || y < 0 || x > 1_000_000 || y > 1_000_000 {
		t.Fatalf("scroll = %d,%d", x, y)
	}
}

func TestClampDiffSpanRuneBoundary(t *testing.T) {
	s := "é" // bytes c3 a9
	a, b, ok := clampDiffSpan(s, 1, 2)
	if !ok {
		t.Fatal("mid-rune span was dropped")
	}
	got := s[a:b]
	if got != s {
		t.Fatalf("span %q, want the whole rune", got)
	}
	if _, _, ok := clampDiffSpan(s, 1, 1); ok {
		t.Fatal("empty mid-rune span was kept")
	}
}

func TestDiffHighlightPixels(t *testing.T) {
	sh := text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	st := diffTestStyle()
	face := font.Font{Typeface: "Go Mono"}
	px := int(st.FontSize) // harness scale is 1 px per sp
	charW := diffAdvance(sh, face, px, "0")
	if charW < 1 {
		t.Fatal("zero char width")
	}
	linePx := max(int(float32(px)*st.LineHeight), 1)
	pad := 4
	gutterW := charW*len("2") + 10
	// The tail is spaces so the word band has no glyph ink.
	text := "aaaa     "
	span := DiffSpan{Start: 4, End: len(text)}
	rows := []DiffRow{
		{
			Left:  DiffSide{Line: 1, Text: "same", Kind: ' '},
			Right: DiffSide{Line: 1, Text: "same", Kind: ' '},
		},
		{
			Left:  DiffSide{Line: 2, Text: text, Kind: '~', Spans: []DiffSpan{span}},
			Right: DiffSide{Line: 2, Text: text, Kind: '~', Spans: []DiffSpan{{Start: span.Start, End: span.End, Kind: '+'}}},
		},
	}
	const sizeX, sizeY = 640, 80
	d, err := harness.New(image.Pt(sizeX, sizeY))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		dims, _, _ := LayoutDiff(gtx, sh, st, rows, 0, 0, diffTestDim(), nil)
		return dims
	}
	d.Step()
	img, err := d.Image()
	if err != nil {
		t.Skip(err)
	}
	rule := 1
	colW := (sizeX - rule) / 2
	into := diffAdvance(sh, face, px, "aaaa") + 2
	yContext := pad + linePx - 2
	yChange := pad + linePx + linePx - 2
	context := img.RGBAAt(2, yContext)
	leftLine := img.RGBAAt(2, yChange)
	leftSpan := img.RGBAAt(gutterW+into, yChange)
	rightX := colW + rule
	rightLine := img.RGBAAt(rightX+2, yChange)
	rightSpan := img.RGBAAt(rightX+gutterW+into, yChange)
	bg := st.Colors.Background
	if !near(context, bg, 4) {
		t.Fatalf("context gutter = %v, want near %v", context, bg)
	}
	if leftLine.R <= leftLine.G+8 || int(leftLine.R) <= int(bg.R)+12 {
		t.Fatalf("removed line = %v, want a light red fill", leftLine)
	}
	if int(leftSpan.R) < int(leftLine.R)+30 {
		t.Fatalf("removed word = %v, line = %v, want a darker red band", leftSpan, leftLine)
	}
	if rightLine.G <= rightLine.R+4 || int(rightLine.G) <= int(bg.G)+8 {
		t.Fatalf("added line = %v, want a light green fill", rightLine)
	}
	if int(rightSpan.G) < int(rightLine.G)+25 {
		t.Fatalf("added word = %v, line = %v, want a darker green band", rightSpan, rightLine)
	}
}

func near(c color.RGBA, want color.NRGBA, tol int) bool {
	return abs(int(c.R)-int(want.R)) <= tol &&
		abs(int(c.G)-int(want.G)) <= tol &&
		abs(int(c.B)-int(want.B)) <= tol
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func diffTestStyle() Style {
	return Style{
		FontSize:   unit.Sp(14),
		LineHeight: 1.3,
		Colors: Colors{
			Background: color.NRGBA{R: 30, G: 31, B: 34, A: 255},
			Foreground: color.NRGBA{R: 200, G: 200, B: 200, A: 255},
		},
	}
}

func diffTestDim() color.NRGBA {
	return color.NRGBA{R: 140, G: 140, B: 140, A: 255}
}
