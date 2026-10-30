package editor

import (
	"hash/maphash"
	"image"
	"image/color"
	"time"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"golang.org/x/image/math/fixed"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/ui"
)

// Colors are the theme entries the editor paints.
type Colors struct {
	Background    color.NRGBA
	Foreground    color.NRGBA
	Selection     color.NRGBA
	Caret         color.NRGBA
	LineHighlight color.NRGBA
	GutterBG      color.NRGBA
	GutterFG      color.NRGBA
	Bracket       color.NRGBA
	Whitespace    color.NRGBA
	MarkNote      color.NRGBA
	MarkError     color.NRGBA
	MarkSmell     color.NRGBA
}

// Style is the editor presentation for one frame.
type Style struct {
	FontSize   unit.Sp
	LineHeight float32
	TabWidth   int
	ShowSpace  bool
	// Blink is how long one caret on/off cycle lasts. Zero keeps the caret lit.
	Blink    time.Duration
	ThemeGen uint64
	Focused  bool
	Colors   Colors
}

type shapeKey struct {
	text uint64
	span uint64
	size int
	gen  uint64
}

// viewUI holds Gio state so the editing logic stays free of a window.
type viewUI struct {
	cache        map[shapeKey][]text.Glyph
	charW        int
	charPx       int
	charGen      uint64
	linePx       int
	dragging     bool
	suppressDrag bool
	// lastPress is the time of the previous primary press. Zero means there
	// is none: a press at Time 0 does not arm a double-click.
	lastPress time.Duration
	caretX    int
	caretY    int
	caretAsc  int
	blinkAt   time.Time
	blinkHead buffer.Offset
	snippet   key.Snippet
	imeSel    key.Range
	imeCaret  key.Caret
	bar       widget.Scrollbar
	// barFrac keeps the fractional line from a scrollbar drag.
	barFrac float32
}

// Span is a colored range inside a line. Start and End are byte columns.
type Span struct {
	Start, End int
	Color      color.NRGBA
}

var shapeSeed = maphash.MakeSeed()

// Layout paints the visible lines and handles pointer input.
// sh is the process-wide shaper owned by the UI goroutine.
func (v *View) Layout(gtx layout.Context, sh *text.Shaper, st Style, spans map[int][]Span) layout.Dimensions {
	if st.TabWidth <= 0 {
		st.TabWidth = 4
	}
	if st.LineHeight <= 0 {
		st.LineHeight = 1.3
	}
	v.TabWidth = st.TabWidth
	v.ensureCache()
	v.measureChar(gtx, sh, st.FontSize, st.ThemeGen)
	linePx := int(float32(gtx.Sp(st.FontSize)) * st.LineHeight)
	if linePx < gtx.Sp(st.FontSize)+2 {
		linePx = gtx.Sp(st.FontSize) + 4
	}
	v.ui.linePx = linePx

	snap := v.Doc.Snapshot()
	lines := snap.LineCount()
	gutterW := v.gutterWidth(gtx, lines)
	pad := gtx.Dp(textPad)

	size := gtx.Constraints.Max
	if size.X < 1 {
		size.X = 1
	}
	if size.Y < 1 {
		size.Y = 1
	}
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	viewH := size.Y
	visible := max(viewH/linePx, 1)
	v.SetPageLines(visible)
	v.SetScrollLine(v.scrollLn)
	rowCount := v.visualCount()
	showBar := rowCount > visible
	v.pointer(gtx, snap, gutterW, pad, linePx, st.TabWidth, size.X, showBar)
	if showBar {
		v.applyScrollbar(gtx, rowCount, visible, size)
	}
	paint.FillShape(gtx.Ops, st.Colors.Background, clip.Rect{Max: size}.Op())

	// Gutter.
	paint.FillShape(gtx.Ops, st.Colors.GutterBG, clip.Rect{Max: image.Pt(gutterW, size.Y)}.Op())

	top := v.visualRow(v.scrollLn)
	first := max(top-20, 0)
	last := min(top+visible+20, rowCount)
	caretLine, _ := snap.OffsetToLineCol(v.primary().Head)
	caretRow := v.visualRow(caretLine)
	gitW := gtx.Dp(gitStripeDp)
	foldW := gtx.Dp(foldSlot)
	numX := gitW + foldW

	face := font.Font{Typeface: "Go Mono"}
	for row := first; row < last; row++ {
		line := v.lineAt(row)
		y := (row - top) * linePx
		if y+linePx < 0 || y > viewH {
			continue
		}
		if row == caretRow {
			paint.FillShape(gtx.Ops, st.Colors.LineHighlight, clip.Rect{
				Min: image.Pt(gutterW, y),
				Max: image.Pt(size.X, y+linePx),
			}.Op())
		}
		v.paintMarks(gtx, snap, line, y, gutterW, pad, linePx, st, size.X)
		v.paintSelection(gtx, snap, line, y, gutterW, linePx, st)
		if kind, ok := v.gutterMark(line); ok {
			col := color.NRGBA{R: 98, G: 150, B: 85, A: 255}
			switch kind {
			case 'm':
				col = color.NRGBA{R: 64, G: 120, B: 200, A: 255}
			case 'd':
				col = color.NRGBA{R: 200, G: 80, B: 80, A: 255}
			}
			paint.FillShape(gtx.Ops, col, clip.Rect{Min: image.Pt(0, y), Max: image.Pt(gitW, y+linePx)}.Op())
		}
		num := itoa(line + 1)
		v.paintString(gtx, sh, face, st.FontSize, st.Colors.GutterFG, num, image.Pt(numX, y), st.ThemeGen, 0, v.ui.charW)
		if _, ok := v.foldStarting(line); ok {
			v.paintFoldMark(gtx, gitW, foldW, y, linePx, st.Colors.GutterFG, !v.foldClosed(line))
		}
		textBytes := snap.Line(line)
		x := gutterW + pad - v.scrollX
		v.paintLineText(gtx, sh, face, st, textBytes, spans[line], image.Pt(x, y))
		v.paintFoldTail(gtx, sh, face, st, snap, line, y, linePx, gutterW, pad)
	}
	v.paintCaret(gtx, snap, gutterW, pad, linePx, st)
	if v.Doc.Notice() != "" {
		v.paintString(gtx, sh, font.Font{Typeface: "Go"}, unit.Sp(13), st.Colors.Caret, v.Doc.Notice(), image.Pt(gutterW+12, 8), st.ThemeGen, 1, 0)
	}
	if st.Focused {
		v.syncIME(gtx, snap, gutterW, linePx, st)
	}
	if showBar {
		v.paintScrollbar(gtx, rowCount, visible, size, st.Colors.GutterFG)
	}
	return layout.Dimensions{Size: size}
}

// scrollTrackDp is the hit width of the vertical scrollbar. The thumb is
// narrower and drawn over the text, so the gutter and hit testing stay put.
const scrollTrackDp = unit.Dp(10)

func scrollTrackWidth(gtx layout.Context, viewW int) int {
	w := max(min(gtx.Dp(scrollTrackDp), viewW), 1)
	return w
}

// applyScrollbar turns a track click or thumb drag into a line offset.
// The thumb is painted later, on top of the text.
func (v *View) applyScrollbar(gtx layout.Context, rows, visible int, size image.Point) {
	if rows < 1 {
		rows = 1
	}
	trackW := scrollTrackWidth(gtx, size.X)
	sgtx := gtx
	sgtx.Constraints = layout.Exact(image.Pt(trackW, size.Y))
	top := v.visualRow(v.scrollLn)
	start := float32(top) / float32(rows)
	end := float32(top+visible) / float32(rows)
	if end > 1 {
		end = 1
	}
	v.ui.bar.Update(sgtx, layout.Vertical, start, end)
	dist := v.ui.bar.ScrollDistance()
	if dist == 0 {
		return
	}
	v.ui.barFrac += dist * float32(rows)
	step := int(v.ui.barFrac)
	if step == 0 {
		return
	}
	before := top
	v.setVisualTop(top + step)
	after := v.visualRow(v.scrollLn)
	v.ui.barFrac -= float32(after - before)
	if after == 0 && v.ui.barFrac < 0 {
		v.ui.barFrac = 0
	}
	if v.scrollLn == v.maxScrollLine() && v.ui.barFrac > 0 {
		v.ui.barFrac = 0
	}
}

func (v *View) paintScrollbar(gtx layout.Context, rows, visible int, size image.Point, col color.NRGBA) {
	if rows < 1 {
		rows = 1
	}
	trackW := scrollTrackWidth(gtx, size.X)
	trackLen := size.Y
	top := v.visualRow(v.scrollLn)
	thumbTop := roundRatio(top, rows, trackLen)
	thumbBot := roundRatio(min(top+visible, rows), rows, trackLen)
	thumbH := thumbBot - thumbTop
	minThumb := gtx.Dp(unit.Dp(24))
	if thumbH < minThumb {
		thumbH = minThumb
	}
	if thumbH > trackLen {
		thumbH = trackLen
	}
	if thumbTop+thumbH > trackLen {
		thumbTop = trackLen - thumbH
	}
	if thumbTop < 0 {
		thumbTop = 0
	}
	thumbW := min(gtx.Dp(unit.Dp(6)), trackW)
	inset := (trackW - thumbW) / 2

	off := op.Offset(image.Pt(size.X-trackW, 0)).Push(gtx.Ops)
	defer off.Pop()

	area := clip.Rect{Max: image.Pt(trackW, trackLen)}
	stack := area.Push(gtx.Ops)
	v.ui.bar.AddDrag(gtx.Ops)
	stack.Pop()

	stack = area.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	v.ui.bar.AddTrack(gtx.Ops)
	pass.Pop()
	stack.Pop()

	thumb := image.Rect(inset, thumbTop, inset+thumbW, thumbTop+thumbH)
	ind := clip.Rect(thumb).Push(gtx.Ops)
	pass = pointer.PassOp{}.Push(gtx.Ops)
	v.ui.bar.AddIndicator(gtx.Ops)
	pass.Pop()
	ind.Pop()

	if col.A == 0 {
		col.A = 255
	}
	if v.ui.bar.IndicatorHovered() || v.ui.bar.Dragging() {
		if col.A < 230 {
			col.A = 230
		}
	} else if col.A > 180 {
		col.A = 180
	}
	paint.FillShape(gtx.Ops, col, clip.Rect(thumb).Op())
}

func roundRatio(num, den, length int) int {
	if den <= 0 || length <= 0 || num <= 0 {
		return 0
	}
	if num > den {
		num = den
	}
	return (num*length + den/2) / den
}

// gitStripeDp is the diff bar at the left of the gutter.
// foldSlot is the disclosure column. It is always reserved so line numbers
// do not jump when a file has no folds.
const (
	gitStripeDp = unit.Dp(4)
	foldSlot    = unit.Dp(14)
)

// textPad is the gap between the gutter and the first cell. Hit testing,
// the caret, and the glyphs all use it.
const textPad = unit.Dp(8)

// doubleClickGap is the longest gap between two primary presses that selects
// a word. A zero gap is the same timestamp, not a second click.
const doubleClickGap = 200 * time.Millisecond

func (v *View) gutterWidth(gtx layout.Context, lines int) int {
	digits := 1
	for n := lines; n >= 10; n /= 10 {
		digits++
	}
	if digits < 2 {
		digits = 2
	}
	cw := v.ui.charW
	if cw < 1 {
		cw = gtx.Sp(unit.Sp(14)) / 2
	}
	return gtx.Dp(unit.Dp(16)) + gtx.Dp(foldSlot) + digits*cw
}

func (v *View) pointer(gtx layout.Context, snap buffer.Snapshot, gutterW, pad, linePx, tabWidth, viewW int, showBar bool) {
	event.Op(gtx.Ops, v)
	// The view clip stays at the default cursor. A cursor set on it is what
	// later paint clips inherit, including the disclosure drawn in the gutter,
	// so the gutter and the text each name their own.
	h := max(gtx.Constraints.Max.Y, 1)
	w := max(gtx.Constraints.Max.X, gutterW)
	gutter := clip.Rect{Max: image.Pt(gutterW, h)}.Push(gtx.Ops)
	pointer.CursorPointer.Add(gtx.Ops)
	gutter.Pop()
	text := clip.Rect{Min: image.Pt(gutterW, 0), Max: image.Pt(w, h)}.Push(gtx.Ops)
	pointer.CursorText.Add(gtx.Ops)
	text.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  v,
			Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Scroll,
			ScrollY: pointer.ScrollRange{Min: -1e6, Max: 1e6},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Scroll:
			step := 3
			if e.Scroll.Y < 0 {
				v.scrollRows(-step)
			} else if e.Scroll.Y > 0 {
				v.scrollRows(step)
			}
			if e.Scroll.X != 0 {
				v.scrollX += int(e.Scroll.X)
				if v.scrollX < 0 {
					v.scrollX = 0
				}
			}
		case pointer.Press:
			if showBar && int(e.Position.X) >= viewW-scrollTrackWidth(gtx, viewW) {
				break
			}
			if v.inFoldColumn(gtx, int(e.Position.X)) {
				line, _ := v.posToLineCol(snap, e.Position, gutterW, pad, linePx, tabWidth)
				if e.Buttons.Contain(pointer.ButtonPrimary) {
					v.toggleFoldAt(line)
					v.ui.lastPress = e.Time
				}
				v.ui.dragging = false
				v.ui.suppressDrag = true
				if v.OnFocus != nil {
					v.OnFocus()
				}
				break
			}
			line, col := v.posToLineCol(snap, e.Position, gutterW, pad, linePx, tabWidth)
			if e.Buttons.Contain(pointer.ButtonPrimary) && v.ellipsisHit(snap, line, int(e.Position.X), gutterW, pad, tabWidth) {
				v.expandEllipsis(line)
				v.ui.lastPress = e.Time
				v.ui.dragging = false
				v.ui.suppressDrag = true
				if v.OnFocus != nil {
					v.OnFocus()
				}
				break
			}
			primary := e.Buttons.Contain(pointer.ButtonPrimary)
			inText := int(e.Position.X) >= gutterW
			delta := e.Time - v.ui.lastPress
			// Stored press (lastPress != 0) and a positive gap: Time 0 is unset,
			// and two presses at one timestamp are not a double-click.
			word := primary && inText && v.ui.lastPress != 0 && delta > 0 && delta <= doubleClickGap
			if word {
				v.selectWordAt(snap, line, col)
				v.ui.dragging = false
				v.ui.suppressDrag = true
			} else {
				extend := e.Modifiers.Contain(key.ModShift)
				v.Click(line, col, extend)
				v.ui.dragging = true
				v.ui.suppressDrag = false
			}
			if primary {
				v.ui.lastPress = e.Time
			}
			if v.OnFocus != nil {
				v.OnFocus()
			}
		case pointer.Drag:
			if showBar && int(e.Position.X) >= viewW-scrollTrackWidth(gtx, viewW) {
				break
			}
			if v.ui.suppressDrag || !v.ui.dragging {
				break
			}
			line, col := v.posToLineCol(snap, e.Position, gutterW, pad, linePx, tabWidth)
			v.Click(line, col, true)
		case pointer.Release:
			v.ui.dragging = false
			v.ui.suppressDrag = false
		}
	}
}

func (v *View) selectWordAt(snap buffer.Snapshot, line, col int) {
	off := v.offsetAt(snap, line, col)
	a, z := wordBounds(snap.Bytes(), int(off))
	v.sels = []Sel{{Anchor: buffer.Offset(a), Head: buffer.Offset(z)}}
	_, v.prefCol = snap.OffsetToLineCol(buffer.Offset(z))
	v.Doc.BreakGroup()
	v.Reveal()
}

func (v *View) posToLineCol(snap buffer.Snapshot, p f32.Point, gutterW, pad, linePx, tabWidth int) (int, int) {
	y := int(p.Y)
	row := v.visualRow(v.scrollLn)
	if linePx > 0 && y > 0 {
		row += y / linePx
	}
	x := max(int(p.X)-gutterW-pad, 0)
	x += v.scrollX
	cw := v.ui.charW
	if cw < 1 {
		cw = 8
	}
	if snap.LineCount() == 0 {
		return 0, 0
	}
	line := v.lineAt(row)
	ln := snap.Line(line)
	cell := x / cw
	return line, cellToByte(ln, cell, tabWidth)
}

func (v *View) inFoldColumn(gtx layout.Context, x int) bool {
	gitW := gtx.Dp(gitStripeDp)
	foldW := gtx.Dp(foldSlot)
	return x >= gitW && x < gitW+foldW
}

func (v *View) ellipsisHit(snap buffer.Snapshot, line, x, gutterW, pad, tabWidth int) bool {
	x0, x1, ok := v.ellipsisSpan(snap, line, gutterW, pad, tabWidth)
	return ok && x >= x0 && x < x1
}

func (v *View) ellipsisSpan(snap buffer.Snapshot, line, gutterW, pad, tabWidth int) (x0, x1 int, ok bool) {
	if !v.foldClosed(line) {
		return 0, 0, false
	}
	f, found := v.foldStarting(line)
	if !found {
		return 0, 0, false
	}
	cw := v.ui.charW
	if cw < 1 {
		cw = 8
	}
	cells := byteToCell(snap.Line(line), len(snap.Line(line)), tabWidth)
	x0 = gutterW + pad - v.scrollX + cells*cw
	x1 = x0 + len(foldTailLabel(f))*cw
	return x0, x1, true
}

func foldTailLabel(f Fold) string {
	return " ... " + f.Suffix
}

func (v *View) paintFoldMark(gtx layout.Context, x, w, y, h int, col color.NRGBA, open bool) {
	if w < 1 || h < 1 {
		return
	}
	stack := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	defer stack.Pop()
	cgtx := gtx
	cgtx.Constraints = layout.Exact(image.Pt(w, h))
	ui.Disclosure(cgtx, col, open)
}

func (v *View) paintFoldTail(gtx layout.Context, sh *text.Shaper, face font.Font, st Style, snap buffer.Snapshot, line, y, linePx, gutterW, pad int) {
	x0, x1, ok := v.ellipsisSpan(snap, line, gutterW, pad, st.TabWidth)
	if !ok {
		return
	}
	f, found := v.foldStarting(line)
	if !found {
		return
	}
	if col, ok := v.foldTailMark(st, snap, line); ok {
		paint.FillShape(gtx.Ops, col, clip.Rect{
			Min: image.Pt(x0, y),
			Max: image.Pt(x1, y+linePx),
		}.Op())
	}
	if v.foldTailSelected(snap, line) {
		paint.FillShape(gtx.Ops, st.Colors.Selection, clip.Rect{
			Min: image.Pt(x0, y),
			Max: image.Pt(x1, y+linePx),
		}.Op())
	}
	v.paintString(gtx, sh, face, st.FontSize, st.Colors.GutterFG, foldTailLabel(f), image.Pt(x0, y), st.ThemeGen, 3, v.ui.charW)
}

func (v *View) foldTailSelected(snap buffer.Snapshot, line int) bool {
	f, ok := v.foldStarting(line)
	if !ok || !v.foldClosed(line) {
		return false
	}
	from := snap.LineStart(line) + buffer.Offset(len(snap.Line(line)))
	to := buffer.Offset(snap.Len())
	if f.End+1 < snap.LineCount() {
		to = snap.LineStart(f.End + 1)
	}
	for _, s := range v.sels {
		if s.max() > from && s.min() < to {
			return true
		}
	}
	return false
}

func cellToByte(line []byte, cell, tabWidth int) int {
	if tabWidth < 1 {
		tabWidth = 4
	}
	col := 0
	i := 0
	for i < len(line) && col < cell {
		r, sz := utf8.DecodeRune(line[i:])
		if sz < 1 {
			break
		}
		if r < 0x20 || r == 0x7f {
			if r != '\t' {
				i += sz
				continue
			}
		}
		adv := 1
		if r == '\t' {
			adv = tabWidth - col%tabWidth
			if adv == 0 {
				adv = tabWidth
			}
		}
		if col+adv > cell {
			break
		}
		col += adv
		i += sz
	}
	// A control that paintLineText skips does not own a cell. Land on the
	// next drawn byte so the caret is not left on an invisible column.
	for i < len(line) {
		r, sz := utf8.DecodeRune(line[i:])
		if sz < 1 || r == '\t' || (r >= 0x20 && r != 0x7f) {
			break
		}
		i += sz
	}
	return i
}

func (v *View) paintSelection(gtx layout.Context, snap buffer.Snapshot, line, y, gutterW, linePx int, st Style) {
	ln := snap.Line(line)
	start := snap.LineStart(line)
	end := start + buffer.Offset(len(ln))
	cw := v.ui.charW
	if cw < 1 {
		cw = 8
	}
	for _, s := range v.sels {
		a, b := s.min(), s.max()
		if b <= start || a >= end && a != b {
			continue
		}
		if a == b {
			continue
		}
		from := max(a, start)
		to := min(b, end)
		c1 := byteToCell(ln, int(from-start), st.TabWidth)
		c2 := byteToCell(ln, int(to-start), st.TabWidth)
		if c2 < c1 {
			c1, c2 = c2, c1
		}
		if a < start {
			c1 = 0
		}
		x0 := gutterW + gtx.Dp(textPad) - v.scrollX + c1*cw
		x1 := max(gutterW+gtx.Dp(textPad)-v.scrollX+c2*cw, x0+2)
		paint.FillShape(gtx.Ops, st.Colors.Selection, clip.Rect{
			Min: image.Pt(x0, y),
			Max: image.Pt(x1, y+linePx),
		}.Op())
	}
}

func (v *View) paintCaret(gtx layout.Context, snap buffer.Snapshot, gutterW, pad, linePx int, st Style) {
	if !st.Focused {
		return
	}
	line, _ := snap.OffsetToLineCol(v.primary().Head)
	ln := snap.Line(line)
	start := int(snap.LineStart(line))
	head := min(max(int(v.primary().Head)-start, 0), len(ln))
	cell := byteToCell(ln, head, st.TabWidth)
	cw := v.ui.charW
	if cw < 1 {
		cw = 8
	}
	x := gutterW + pad - v.scrollX + cell*cw
	y := (v.visualRow(line) - v.visualRow(v.scrollLn)) * linePx
	v.ui.caretX = x
	v.ui.caretY = y + linePx - 4
	v.ui.caretAsc = linePx - 6
	headOff := v.primary().Head
	if v.ui.blinkAt.IsZero() || headOff != v.ui.blinkHead {
		v.ui.blinkAt = gtx.Now
		v.ui.blinkHead = headOff
	}
	if caretOn(gtx.Now, v.ui.blinkAt, st.Blink) {
		paint.FillShape(gtx.Ops, st.Colors.Caret, clip.Rect{
			Min: image.Pt(x, y+2),
			Max: image.Pt(x+2, y+linePx-2),
		}.Op())
	}
	if st.Blink <= 0 {
		return
	}
	half := st.Blink / 2
	if half <= 0 {
		return
	}
	elapsed := max(gtx.Now.Sub(v.ui.blinkAt), 0)
	into := elapsed % half
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(half - into)})
}

// caretOn reports whether the caret is lit. period is one full on/off cycle.
func caretOn(now, from time.Time, period time.Duration) bool {
	if period <= 0 {
		return true
	}
	half := period / 2
	if half <= 0 {
		return true
	}
	if now.Before(from) {
		return true
	}
	return (now.Sub(from)/half)%2 == 0
}

func byteToCell(line []byte, byteCol, tabWidth int) int {
	if byteCol > len(line) {
		byteCol = len(line)
	}
	if tabWidth < 1 {
		tabWidth = 4
	}
	col := 0
	for i := 0; i < byteCol; {
		r, sz := utf8.DecodeRune(line[i:])
		if sz < 1 {
			break
		}
		if r == '\t' {
			adv := tabWidth - col%tabWidth
			if adv == 0 {
				adv = tabWidth
			}
			col += adv
		} else if r < 0x20 || r == 0x7f {
			// paintLineText does not draw these and does not advance a cell.
		} else {
			col++
		}
		i += sz
	}
	return col
}

// paintLineText draws one line. Tabs and other C0 controls are not shaped:
// Go Mono has no glyph for U+0009, and the shaper would draw .notdef at the
// start of every indented line. Columns follow byteToCell so the caret matches.
func (v *View) paintLineText(gtx layout.Context, sh *text.Shaper, face font.Font, st Style, line []byte, spans []Span, origin image.Point) {
	cw := v.ui.charW
	if cw < 1 {
		cw = 8
	}
	tabWidth := st.TabWidth
	if tabWidth < 1 {
		tabWidth = 4
	}
	fg := st.Colors.Foreground
	col := 0
	i := 0
	for i < len(line) {
		r, sz := utf8.DecodeRune(line[i:])
		if sz < 1 {
			break
		}
		if r == '\t' {
			adv := tabWidth - col%tabWidth
			if adv == 0 {
				adv = tabWidth
			}
			if st.ShowSpace {
				v.paintString(gtx, sh, face, st.FontSize, spaceColor(st), "→", image.Pt(origin.X+col*cw, origin.Y), st.ThemeGen, 2, cw)
			}
			col += adv
			i += sz
			continue
		}
		if r < 0x20 || r == 0x7f {
			i += sz
			continue
		}
		if st.ShowSpace && r == ' ' {
			v.paintString(gtx, sh, face, st.FontSize, spaceColor(st), "·", image.Pt(origin.X+col*cw, origin.Y), st.ThemeGen, 3, cw)
			col++
			i += sz
			continue
		}
		c := colorAt(spans, i, fg)
		start := i
		startCol := col
		j := i
		for j < len(line) {
			r2, sz2 := utf8.DecodeRune(line[j:])
			if sz2 < 1 || r2 == '\t' || r2 < 0x20 || r2 == 0x7f || (st.ShowSpace && r2 == ' ') {
				break
			}
			if colorAt(spans, j, fg) != c {
				break
			}
			j += sz2
			col++
		}
		span := uint64(c.R)<<24 | uint64(c.G)<<16 | uint64(c.B)<<8 | uint64(c.A)
		v.paintString(gtx, sh, face, st.FontSize, c, string(line[start:j]), image.Pt(origin.X+startCol*cw, origin.Y), st.ThemeGen, span, cw)
		i = j
	}
}

func colorAt(spans []Span, i int, fg color.NRGBA) color.NRGBA {
	for _, sp := range spans {
		if i >= sp.Start && i < sp.End {
			return sp.Color
		}
	}
	return fg
}

func spaceColor(st Style) color.NRGBA {
	if st.Colors.Whitespace.A != 0 {
		return st.Colors.Whitespace
	}
	return st.Colors.Foreground
}

func (v *View) paintString(gtx layout.Context, sh *text.Shaper, face font.Font, size unit.Sp, col color.NRGBA, s string, origin image.Point, gen uint64, span uint64, cellW int) {
	if s == "" || sh == nil {
		return
	}
	key := shapeKey{text: hashBytes([]byte(s)), span: span, size: gtx.Sp(size), gen: gen}
	glyphs, ok := v.ui.cache[key]
	if !ok {
		sh.LayoutString(text.Parameters{
			Font:     face,
			PxPerEm:  fixed.I(gtx.Sp(size)),
			MaxWidth: 1 << 30,
			MaxLines: 1,
		}, s)
		glyphs = nil
		for g, ok := sh.NextGlyph(); ok; g, ok = sh.NextGlyph() {
			glyphs = append(glyphs, g)
		}
		cp := append([]text.Glyph(nil), glyphs...)
		if len(v.ui.cache) > 5000 {
			v.ui.cache = map[shapeKey][]text.Glyph{}
		}
		v.ui.cache[key] = cp
		glyphs = cp
	}
	if len(glyphs) == 0 {
		return
	}
	drawn := snapGlyphColumns(glyphs, cellW)
	macro := op.Record(gtx.Ops)
	paint.ColorOp{Color: col}.Add(gtx.Ops)
	mat := macro.Stop()
	// Shape ignores glyph Y. Y is the baseline, measured from the top of the line.
	stack := op.Offset(image.Pt(origin.X, origin.Y+int(glyphs[0].Y))).Push(gtx.Ops)
	path := sh.Shape(drawn)
	outline := clip.Outline{Path: path}.Op().Push(gtx.Ops)
	mat.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	outline.Pop()
	if call := sh.Bitmaps(drawn); call != (op.CallOp{}) {
		call.Add(gtx.Ops)
	}
	stack.Pop()
}

// snapGlyphColumns places LTR clusters on the integer cell grid the caret uses.
// A run with FlagTowardOrigin already has visual X (it decreases). Rewriting
// that X from a left-to-right counter reverses the run, so those lines keep
// the shaper's positions.
func snapGlyphColumns(glyphs []text.Glyph, cellW int) []text.Glyph {
	if cellW <= 0 || len(glyphs) == 0 || lineFlowsTowardOrigin(glyphs) {
		return glyphs
	}
	drawn := append([]text.Glyph(nil), glyphs...)
	col := 0
	for i := range drawn {
		drawn[i].X = fixed.I(col * cellW)
		if drawn[i].Flags&text.FlagClusterBreak == 0 {
			continue
		}
		n := max(int(drawn[i].Runes), 1)
		col += n
	}
	return drawn
}

func lineFlowsTowardOrigin(glyphs []text.Glyph) bool {
	for _, g := range glyphs {
		if g.Flags&text.FlagTowardOrigin != 0 {
			return true
		}
	}
	return false
}

func (v *View) ensureCache() {
	if v.ui.cache == nil {
		v.ui.cache = map[shapeKey][]text.Glyph{}
	}
}

func (v *View) syncIME(gtx layout.Context, snap buffer.Snapshot, gutterW, linePx int, st Style) {
	b := snap.Bytes()
	s := v.primary()
	a := RuneIndex(b, int(s.min()))
	h := RuneIndex(b, int(s.max()))
	caret := RuneIndex(b, int(s.Head))
	start := max(caret-2048, 0)
	end := caret + 2048
	total := utf8.RuneCount(b)
	if end > total {
		end = total
	}
	bs := ByteIndex(b, start)
	be := ByteIndex(b, end)
	snip := key.Snippet{Start: start, End: end, Text: string(b[bs:be])}
	if snip != v.ui.snippet {
		v.ui.snippet = snip
		gtx.Execute(key.SnippetCmd{Tag: v, Snippet: snip})
	}
	sel := key.Range{Start: a, End: h}
	caretCmd := key.Caret{
		Pos:     f32.Pt(float32(v.ui.caretX), float32(v.ui.caretY)),
		Ascent:  float32(v.ui.caretAsc),
		Descent: 2,
	}
	if sel != v.ui.imeSel || caretCmd != v.ui.imeCaret {
		v.ui.imeSel = sel
		v.ui.imeCaret = caretCmd
		gtx.Execute(key.SelectionCmd{Tag: v, Range: sel, Caret: caretCmd})
	}
	_ = gutterW
	_ = linePx
	_ = st
}

// ReplaceRunes applies an IME edit. Offsets are rune indexes in the document.
func (v *View) ReplaceRunes(start, end int, text string) {
	b := v.Doc.Snapshot().Bytes()
	bs := ByteIndex(b, start)
	be := ByteIndex(b, end)
	v.sels = []Sel{{Anchor: buffer.Offset(bs), Head: buffer.Offset(be)}}
	v.Insert(text, buffer.OriginTyping)
}

// Tag is the Gio event tag for this view.
func (v *View) Tag() event.Tag { return v }

func hashBytes(b []byte) uint64 {
	var h maphash.Hash
	h.SetSeed(shapeSeed)
	_, _ = h.Write(b)
	return h.Sum64()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func (v *View) measureChar(gtx layout.Context, sh *text.Shaper, size unit.Sp, gen uint64) {
	px := gtx.Sp(size)
	if v.ui.charW > 0 && v.ui.charPx == px && v.ui.charGen == gen {
		return
	}
	w := 0
	if sh != nil {
		sh.LayoutString(text.Parameters{
			Font:     font.Font{Typeface: "Go Mono"},
			PxPerEm:  fixed.I(px),
			MaxWidth: 1 << 20,
			MaxLines: 1,
		}, "M")
		for g, ok := sh.NextGlyph(); ok; g, ok = sh.NextGlyph() {
			w += g.Advance.Round()
		}
	}
	if w < 1 {
		w = px / 2
		if w < 1 {
			w = 8
		}
	}
	v.ui.charW = w
	v.ui.charPx = px
	v.ui.charGen = gen
}
