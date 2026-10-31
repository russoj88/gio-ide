package editor

import (
	"bytes"
	"image"
	"image/color"
	"slices"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/russoj88/gio-ide/internal/buffer"
)

// MarkKind selects the color of a persistent range.
type MarkKind int

const (
	// MarkNote is the neutral mark. It uses editor.findMatch.
	MarkNote MarkKind = iota
	// MarkError marks one error. It uses diag.error.
	MarkError
	// MarkSmell marks one smell. It uses diag.warning.
	MarkSmell
)

// Mark is a half-open byte range on one document.
// It stays when the caret moves. It is not a selection.
// Start == End is the caret line when that line has no bytes.
type Mark struct {
	Start, End buffer.Offset
	Kind       MarkKind
}

// SetMarks replaces the marks painted on this view. The slice is copied.
func (v *View) SetMarks(marks []Mark) {
	v.marks = append([]Mark(nil), marks...)
}

// ToggleMark adds m. The same start, end, and kind removes that mark.
// The same start and end with a different kind replaces the kind.
// End < Start is ignored. A point mark (Start == End) is kept.
// Other marks stay in order.
func ToggleMark(marks []Mark, m Mark) []Mark {
	if m.End < m.Start {
		return append([]Mark(nil), marks...)
	}
	out := make([]Mark, 0, len(marks)+1)
	found := false
	for _, cur := range marks {
		if cur.Start == m.Start && cur.End == m.End {
			found = true
			if cur.Kind == m.Kind {
				continue
			}
			out = append(out, m)
			continue
		}
		out = append(out, cur)
	}
	if !found {
		out = append(out, m)
	}
	return out
}

// LinesMark is the half-open byte range of inclusive lines first through last.
// Lines are clamped to the snapshot. The end is the start of the next line,
// or the buffer length on the last line. A nil snapshot returns 0, 0.
func LinesMark(snap buffer.Snapshot, first, last int) (start, end buffer.Offset) {
	if snap == nil || snap.LineCount() == 0 {
		return 0, 0
	}
	if first > last {
		first, last = last, first
	}
	n := snap.LineCount()
	if first < 0 {
		first = 0
	}
	if first >= n {
		first = n - 1
	}
	if last < 0 {
		last = 0
	}
	if last >= n {
		last = n - 1
	}
	start = snap.LineStart(first)
	if last+1 < n {
		end = snap.LineStart(last + 1)
	} else {
		end = buffer.Offset(snap.Len())
	}
	return start, end
}

// SelectionMark is the range a highlight command stores.
// start and end are byte offsets. Order does not matter.
// fullLines marks every line the range touches. A caret (start == end)
// marks that line. A selection that ends on a later line start does not
// include that line. ok is false when snap is nil or the range is empty
// and the caret line has no bytes to stand on. A point mark is returned
// for a caret on a line that has no bytes, including an empty buffer.
func SelectionMark(snap buffer.Snapshot, start, end buffer.Offset, kind MarkKind, fullLines bool) (Mark, bool) {
	if snap == nil {
		return Mark{}, false
	}
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	n := buffer.Offset(snap.Len())
	if end > n {
		end = n
	}
	if start > n {
		start = n
	}
	if fullLines || start == end {
		ls, _ := snap.OffsetToLineCol(start)
		le, col := snap.OffsetToLineCol(end)
		if start != end && col == 0 && le > ls {
			le--
		}
		start, end = LinesMark(snap, ls, le)
	}
	if end < start {
		return Mark{}, false
	}
	if end == start {
		// The caret line has no bytes. Keep a point so the row can still paint.
		return Mark{Start: start, End: end, Kind: kind}, true
	}
	return Mark{Start: start, End: end, Kind: kind}, true
}

// ShiftMarks moves marks across one edit of old. old is the buffer before ed.
// The edit is split on changed lines. Pieces apply from the end of the buffer,
// so a mark between two sites stays. A caller that applies several hunks calls
// ShiftMarks once per hunk, last hunk first, and passes the same old bytes
// every time. ClipMarks after the last hunk.
func ShiftMarks(old []byte, ed buffer.Edit, marks []Mark) []Mark {
	if old == nil {
		old = []byte{}
	}
	ed = clampEdit(old, ed)
	if bytes.Equal(old[ed.Start:ed.End], []byte(ed.Text)) {
		return append([]Mark(nil), marks...)
	}
	edits := splitLineEdits(old, ed)
	if len(edits) == 0 {
		edits = []buffer.Edit{ed}
	}
	out := append([]Mark(nil), marks...)
	for _, piece := range slices.Backward(edits) {
		next := make([]Mark, 0, len(out))
		for _, m := range out {
			if shifted, ok := shiftOneMark(m, piece); ok {
				next = append(next, shifted)
			}
		}
		out = next
	}
	return out
}

// ClipMarks drops a mark with End < Start, Start < 0, or End > n.
// A point mark at n stays. n is the buffer length after the edits.
func ClipMarks(marks []Mark, n int) []Mark {
	if n < 0 {
		n = 0
	}
	out := make([]Mark, 0, len(marks))
	limit := buffer.Offset(n)
	for _, m := range marks {
		if m.End < m.Start || m.Start < 0 || m.End > limit {
			continue
		}
		out = append(out, m)
	}
	return out
}

// shiftOneMark applies one edit whose offsets are in the same coordinate
// space as the mark. Callers apply later edits first.
func shiftOneMark(m Mark, ed buffer.Edit) (Mark, bool) {
	a, b := ed.Start, ed.End
	delta := buffer.Offset(len(ed.Text)) - (b - a)
	s, e := m.Start, m.End
	switch {
	case b <= s:
		s += delta
		e += delta
	case a >= e:
	case a <= s && b >= e:
		return Mark{}, false
	case a >= s && b <= e:
		e += delta
	case a < s && b > s && b < e:
		s = a
		e += delta
	default:
		e = a + buffer.Offset(len(ed.Text))
	}
	if e < s {
		return Mark{}, false
	}
	m.Start, m.End = s, e
	return m, true
}

func (v *View) paintMarks(gtx layout.Context, snap buffer.Snapshot, line, y, gutterW, pad, linePx int, st Style, sizeX int) {
	if len(v.marks) == 0 {
		return
	}
	start := snap.LineStart(line)
	textEnd := start + buffer.Offset(len(snap.Line(line)))
	next := buffer.Offset(snap.Len())
	if line+1 < snap.LineCount() {
		next = snap.LineStart(line + 1)
	}
	cw := v.ui.charW
	if cw < 1 {
		cw = 8
	}
	ln := snap.Line(line)
	for _, m := range v.marks {
		if !markHitsLine(snap, m, line, start, next) {
			continue
		}
		col := markColor(st, m.Kind)
		if markFillsLine(m, start, textEnd, next) {
			x0 := gutterW + pad - v.scrollX
			if x0 >= sizeX {
				continue
			}
			paint.FillShape(gtx.Ops, col, clip.Rect{
				Min: image.Pt(x0, y),
				Max: image.Pt(sizeX, y+linePx),
			}.Op())
			continue
		}
		from := max(m.Start, start)
		to := min(m.End, textEnd)
		if from >= to {
			continue
		}
		c1 := byteToCell(ln, int(from-start), st.TabWidth)
		c2 := byteToCell(ln, int(to-start), st.TabWidth)
		if m.Start < start {
			c1 = 0
		}
		x0 := gutterW + pad - v.scrollX + c1*cw
		x1 := max(gutterW+pad-v.scrollX+c2*cw, x0+2)
		paint.FillShape(gtx.Ops, col, clip.Rect{
			Min: image.Pt(x0, y),
			Max: image.Pt(x1, y+linePx),
		}.Op())
	}
}

func markHitsLine(snap buffer.Snapshot, m Mark, line int, start, next buffer.Offset) bool {
	if m.End < m.Start {
		return false
	}
	if m.Start == m.End {
		at, _ := snap.OffsetToLineCol(m.Start)
		return at == line
	}
	return m.End > start && m.Start < next
}

func markFillsLine(m Mark, start, textEnd, next buffer.Offset) bool {
	if m.Start == m.End {
		return true
	}
	if m.Start > start || m.End < textEnd {
		return false
	}
	if textEnd == start {
		return m.End > start && m.Start < next
	}
	return true
}

func markColor(st Style, k MarkKind) color.NRGBA {
	switch k {
	case MarkError:
		return st.Colors.MarkError
	case MarkSmell:
		return st.Colors.MarkSmell
	default:
		return st.Colors.MarkNote
	}
}

func (v *View) foldTailMark(st Style, snap buffer.Snapshot, line int) (color.NRGBA, bool) {
	f, ok := v.foldStarting(line)
	if !ok || !v.foldClosed(line) {
		return color.NRGBA{}, false
	}
	from := snap.LineStart(line) + buffer.Offset(len(snap.Line(line)))
	to := buffer.Offset(snap.Len())
	if f.End+1 < snap.LineCount() {
		to = snap.LineStart(f.End + 1)
	}
	var col color.NRGBA
	found := false
	for _, m := range v.marks {
		hit := m.End > from && m.Start < to
		if m.Start == m.End {
			hit = m.Start >= from && m.Start < to
		}
		if !hit {
			continue
		}
		col = markColor(st, m.Kind)
		found = true
	}
	return col, found
}
