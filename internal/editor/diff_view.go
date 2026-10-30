package editor

import (
	"image"
	"image/color"
	"strconv"
	"unicode/utf8"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"golang.org/x/image/math/fixed"
)

// DiffSpan is a highlighted byte range. Kind is '-' or '+'.
type DiffSpan struct {
	Start int
	End   int
	Kind  byte
}

// DiffSide is one column of a row.
// Line is 1-based, or 0 when this side is a pad.
// Kind is ' ' unchanged, '+' added, '-' removed, '~' modified, or 0 for a pad.
type DiffSide struct {
	Line  int
	Text  string
	Kind  byte
	Spans []DiffSpan
}

// DiffRow is one aligned pair. Left is the before side, right is the after side.
type DiffRow struct {
	Left, Right DiffSide
}

// LayoutDiff paints a side-by-side diff in the editor monospace face.
// There is no caret and no editor pointer handler. scrollX and scrollY are
// pixel offsets of the text. tag receives the wheel and must not be shared.
// A nil tag leaves the scroll unchanged. The returned scroll is clamped.
// Added lines use color.NRGBA{R: 98, G: 150, B: 85, A: 255} and removed lines
// use color.NRGBA{R: 200, G: 80, B: 80, A: 255}, the same literals as the
// gutter bar in View.Layout. The two ints are the clamped scrollX and scrollY.
func LayoutDiff(gtx layout.Context, shaper *text.Shaper, st Style, rows []DiffRow, scrollX, scrollY int, dim color.NRGBA, tag event.Tag) (layout.Dimensions, int, int) {
	if st.FontSize <= 0 {
		st.FontSize = 14
	}
	if st.LineHeight <= 0 {
		st.LineHeight = 1.3
	}
	size := gtx.Constraints.Max
	if size.X < 1 {
		size.X = 1
	}
	if size.Y < 1 {
		size.Y = 1
	}
	stack := clip.Rect{Max: size}.Push(gtx.Ops)
	defer stack.Pop()
	if tag != nil {
		event.Op(gtx.Ops, tag)
		dx, dy := diffWheel(gtx, tag)
		scrollX += dx
		scrollY += dy
	}
	paint.FillShape(gtx.Ops, st.Colors.Background, clip.Rect{Max: size}.Op())

	linePx := max(int(float32(gtx.Sp(st.FontSize))*st.LineHeight), 1)
	pad := gtx.Dp(unit.Dp(4))
	face := font.Font{Typeface: "Go Mono"}
	px := gtx.Sp(st.FontSize)
	charW := diffAdvance(shaper, face, px, "0")
	if charW < 1 {
		charW = max(px/2, 1)
	}
	maxLine, maxRunes := 1, 0
	for i := range rows {
		if rows[i].Left.Line > maxLine {
			maxLine = rows[i].Left.Line
		}
		if rows[i].Right.Line > maxLine {
			maxLine = rows[i].Right.Line
		}
		if n := utf8.RuneCountInString(rows[i].Left.Text); n > maxRunes {
			maxRunes = n
		}
		if n := utf8.RuneCountInString(rows[i].Right.Text); n > maxRunes {
			maxRunes = n
		}
	}
	gutterW := charW*len(strconv.Itoa(maxLine)) + gtx.Dp(unit.Dp(10))
	rule := max(gtx.Dp(unit.Dp(1)), 1)
	colW := max((size.X-rule)/2, 1)
	textW := max(colW-gutterW, 1)
	scrollX = clampDiffScroll(scrollX, maxRunes*charW, textW)
	scrollY = clampDiffScroll(scrollY, pad+len(rows)*linePx, size.Y)

	removed := color.NRGBA{R: 200, G: 80, B: 80, A: 255}
	added := color.NRGBA{R: 98, G: 150, B: 85, A: 255}
	paint.FillShape(gtx.Ops, dim, clip.Rect{
		Min: image.Pt(colW, 0),
		Max: image.Pt(colW+rule, size.Y),
	}.Op())

	first := 0
	if scrollY > pad {
		first = (scrollY - pad) / linePx
	}
	start := max(first-1, 0)
	end := min(first+size.Y/linePx+3, len(rows))
	for i := start; i < end; i++ {
		y := pad + i*linePx - scrollY
		paintDiffColumn(gtx, shaper, face, st.FontSize, px, rows[i].Left, image.Pt(0, y), colW, linePx, gutterW, scrollX, removed, dim, st.Colors.Foreground)
		paintDiffColumn(gtx, shaper, face, st.FontSize, px, rows[i].Right, image.Pt(colW+rule, y), colW, linePx, gutterW, scrollX, added, dim, st.Colors.Foreground)
	}
	return layout.Dimensions{Size: size}, scrollX, scrollY
}

// diffWheel uses the same 48px step and sign as markdownWheel on both axes.
// A negative Scroll component decreases that offset.
func diffWheel(gtx layout.Context, tag event.Tag) (int, int) {
	dx, dy := 0, 0
	const step = 48
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  tag,
			Kinds:   pointer.Scroll,
			ScrollX: pointer.ScrollRange{Min: -1e6, Max: 1e6},
			ScrollY: pointer.ScrollRange{Min: -1e6, Max: 1e6},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok || e.Kind != pointer.Scroll {
			continue
		}
		if e.Scroll.X < 0 {
			dx -= step
		} else if e.Scroll.X > 0 {
			dx += step
		}
		if e.Scroll.Y < 0 {
			dy -= step
		} else if e.Scroll.Y > 0 {
			dy += step
		}
	}
	return dx, dy
}

func paintDiffColumn(gtx layout.Context, sh *text.Shaper, face font.Font, size unit.Sp, px int, side DiffSide, origin image.Point, colW, linePx, gutterW, scrollX int, base, dim, fg color.NRGBA) {
	off := op.Offset(origin).Push(gtx.Ops)
	defer off.Pop()
	row := clip.Rect{Max: image.Pt(colW, linePx)}.Push(gtx.Ops)
	defer row.Pop()
	if fill, ok := diffLineFill(side.Kind, base, dim); ok {
		paint.FillShape(gtx.Ops, fill, clip.Rect{Max: image.Pt(colW, linePx)}.Op())
	}
	textX := gutterW
	area := clip.Rect{
		Min: image.Pt(textX, 0),
		Max: image.Pt(colW, linePx),
	}.Push(gtx.Ops)
	band := base
	band.A = 150
	for _, sp := range side.Spans {
		a, b, ok := clampDiffSpan(side.Text, sp.Start, sp.End)
		if !ok {
			continue
		}
		x0 := textX + diffAdvance(sh, face, px, side.Text[:a]) - scrollX
		x1 := textX + diffAdvance(sh, face, px, side.Text[:b]) - scrollX
		if x1 <= x0 {
			continue
		}
		paint.FillShape(gtx.Ops, band, clip.Rect{
			Min: image.Pt(x0, 0),
			Max: image.Pt(x1, linePx),
		}.Op())
	}
	paintDiffText(gtx, sh, face, size, fg, side.Text, image.Pt(textX-scrollX, 0))
	area.Pop()
	if side.Line > 0 {
		num := strconv.Itoa(side.Line)
		nw := diffAdvance(sh, face, px, num)
		paintDiffText(gtx, sh, face, size, dim, num, image.Pt(max(gutterW-nw-gtx.Dp(unit.Dp(6)), 0), 0))
	}
}

func diffLineFill(kind byte, base, dim color.NRGBA) (color.NRGBA, bool) {
	switch kind {
	case '-', '+', '~':
		base.A = 48
		return base, true
	case 0:
		dim.A = 28
		return dim, true
	default:
		return color.NRGBA{}, false
	}
}

// clampDiffSpan returns a slice range inside s.
// Offsets that land inside a rune snap back to a boundary so Text[:n] cannot panic.
func clampDiffSpan(s string, start, end int) (int, int, bool) {
	start = snapRuneStart(s, start)
	end = snapRuneStart(s, end)
	if end <= start {
		return 0, 0, false
	}
	return start, end, true
}

// snapRuneStart moves n onto a boundary of s. n == len(s) stays put.
func snapRuneStart(s string, n int) int {
	if n <= 0 {
		return 0
	}
	if n >= len(s) {
		return len(s)
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return n
}

func clampDiffScroll(scroll, content, view int) int {
	limit := max(content-view, 0)
	if scroll < 0 {
		return 0
	}
	if scroll > limit {
		return limit
	}
	return scroll
}

// diffAdvance is the pixel width of s in the diff face.
// A nil shaper or an empty string has width 0.
func diffAdvance(sh *text.Shaper, face font.Font, px int, s string) int {
	if sh == nil || s == "" || px < 1 {
		return 0
	}
	sh.LayoutString(text.Parameters{
		Font:             face,
		PxPerEm:          fixed.I(px),
		MaxWidth:         1_000_000,
		MaxLines:         1,
		Alignment:        text.Start,
		DisableSpaceTrim: true,
	}, s)
	maxX := 0
	for g, ok := sh.NextGlyph(); ok; g, ok = sh.NextGlyph() {
		if x := (g.X + g.Advance).Round(); x > maxX {
			maxX = x
		}
	}
	return maxX
}

func paintDiffText(gtx layout.Context, sh *text.Shaper, face font.Font, size unit.Sp, col color.NRGBA, s string, at image.Point) {
	if s == "" || sh == nil {
		return
	}
	macro := op.Record(gtx.Ops)
	paint.ColorOp{Color: col}.Add(gtx.Ops)
	mat := macro.Stop()
	stack := op.Offset(at).Push(gtx.Ops)
	ig := gtx
	ig.Constraints = layout.Constraints{Max: image.Pt(1_000_000, 10_000)}
	widget.Label{MaxLines: 1}.Layout(ig, sh, face, size, s, mat)
	stack.Pop()
}
