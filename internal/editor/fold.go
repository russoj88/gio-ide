package editor

import (
	"bytes"
	"slices"

	"github.com/russoj88/gio-ide/internal/buffer"
)

// Fold is one collapsible range. Start and End are inclusive buffer lines.
// The header Start stays on screen. Suffix is painted after the ellipsis.
type Fold struct {
	Start, End int
	Suffix     string
}

// SetFolds replaces the ranges for a document version. A version older than
// the last one applied is ignored. A closed header stays closed when a new
// range still starts on that line.
func (v *View) SetFolds(version int32, folds []Fold) {
	if version < v.foldVer {
		return
	}
	v.foldVer = version
	v.folds = append([]Fold(nil), folds...)
	starts := make(map[int]struct{}, len(v.folds))
	for _, f := range v.folds {
		if f.End > f.Start {
			starts[f.Start] = struct{}{}
		}
	}
	for k := range v.closed {
		if _, ok := starts[k]; !ok {
			delete(v.closed, k)
		}
	}
	v.rowsDirty = true
	if v.Doc != nil {
		v.pullCarets(v.Doc.Snapshot())
	}
	v.snapScrollVisible()
}

// AdjustFolds moves ranges after an edit of the previous bytes. An edit
// before a range shifts it. An edit on the header keeps it closed and shifts
// the end. An edit in the hidden interior opens the range. A replacement that
// touches two places is applied as the separate line edits inside it, so a
// fold between them stays closed. shiftScroll moves the viewport when an edit
// is entirely before the top line. The view that just edited already revealed
// the caret, so the shell passes false for it and true for the other views
// of the same document. A caret left in a hidden line moves to the end of
// that line's header.
func (v *View) AdjustFolds(old []byte, ed buffer.Edit, shiftScroll bool) {
	if old == nil {
		old = []byte{}
	}
	ed = clampEdit(old, ed)
	edits := splitLineEdits(old, ed)
	if len(edits) == 0 {
		edits = []buffer.Edit{ed}
	}
	// Later edits first, so each one still addresses the original bytes.
	for _, edit := range slices.Backward(edits) {
		v.shiftOneFold(old, edit, shiftScroll)
	}
	if v.Doc != nil {
		v.foldVer = v.Doc.Version()
		v.rowsDirty = true
		v.pullCarets(v.Doc.Snapshot())
	}
	v.rowsDirty = true
	v.snapScrollVisible()
}

func (v *View) shiftOneFold(old []byte, ed buffer.Edit, shiftScroll bool) {
	start := int(ed.Start)
	end := int(ed.End)
	nd := countNL(ed.Text) - countNL(string(old[start:end]))
	next := make([]Fold, 0, len(v.folds))
	closed := make(map[int]struct{})
	for _, f := range v.folds {
		_, was := v.closed[f.Start]
		fs := lineStartBytes(old, f.Start)
		he := lineStartBytes(old, f.Start+1)
		fe := lineStartBytes(old, f.End+1)
		switch {
		case end <= fs:
			f.Start += nd
			f.End += nd
		case start >= fe:
		case start >= fs && end <= he:
			f.End += nd
		default:
			was = false
			if start >= fs {
				f.End += nd
			} else {
				continue
			}
		}
		if f.Start < 0 {
			f.Start = 0
		}
		if f.End <= f.Start {
			continue
		}
		next = append(next, f)
		if was {
			closed[f.Start] = struct{}{}
		}
	}
	v.folds = next
	v.closed = closed
	if shiftScroll {
		top := lineStartBytes(old, v.scrollLn)
		if end <= top {
			v.scrollLn += nd
		}
	}
}

// Fold closes the range that starts on the primary caret's line. When that
// range is already closed, it closes the next open range that contains the line.
func (v *View) Fold() {
	if v.Doc == nil {
		return
	}
	line, _ := v.Doc.Snapshot().OffsetToLineCol(v.primary().Head)
	if f, ok := v.foldStarting(line); ok && !v.foldClosed(line) {
		v.closeFold(f)
		return
	}
	if f, ok := v.outerOpen(line); ok {
		v.closeFold(f)
	}
}

// Unfold opens the smallest closed range that starts on the caret line or
// hides it.
func (v *View) Unfold() {
	if v.Doc == nil {
		return
	}
	line, _ := v.Doc.Snapshot().OffsetToLineCol(v.primary().Head)
	f, ok := v.innermostClosed(line)
	if !ok {
		return
	}
	delete(v.closed, f.Start)
	v.rowsDirty = true
}

// FoldAll closes every range.
func (v *View) FoldAll() {
	if len(v.folds) == 0 {
		return
	}
	if v.closed == nil {
		v.closed = map[int]struct{}{}
	}
	for _, f := range v.folds {
		if f.End > f.Start {
			v.closed[f.Start] = struct{}{}
		}
	}
	v.rowsDirty = true
	if v.Doc != nil {
		v.pullCarets(v.Doc.Snapshot())
	}
	v.snapScrollVisible()
}

// UnfoldAll opens every range.
func (v *View) UnfoldAll() {
	if len(v.closed) == 0 {
		return
	}
	v.closed = map[int]struct{}{}
	v.rowsDirty = true
}

// CopyFoldsTo copies ranges and which ones are closed onto dst.
func (v *View) CopyFoldsTo(dst *View) {
	if dst == nil {
		return
	}
	dst.folds = append([]Fold(nil), v.folds...)
	dst.foldVer = v.foldVer
	if len(v.closed) == 0 {
		dst.closed = nil
	} else {
		dst.closed = make(map[int]struct{}, len(v.closed))
		for k := range v.closed {
			dst.closed[k] = struct{}{}
		}
	}
	dst.rowsDirty = true
}

// SyntaxWindow is the buffer-line span [first, last) covering visual rows
// from 20 above the viewport through 80 below it.
func (v *View) SyntaxWindow() (first, last int) {
	n := v.visualCount()
	if n == 0 {
		return 0, 0
	}
	top := v.visualRow(v.scrollLn)
	a := max(top-20, 0)
	b := min(top+80, n)
	first = v.lineAt(a)
	if b >= n {
		if v.Doc == nil {
			last = 0
		} else {
			last = v.Doc.Snapshot().LineCount()
		}
	} else {
		last = v.lineAt(b)
	}
	if last < first {
		last = first
	}
	return first, last
}

func (v *View) closeFold(f Fold) {
	if f.End <= f.Start {
		return
	}
	if v.closed == nil {
		v.closed = map[int]struct{}{}
	}
	v.closed[f.Start] = struct{}{}
	v.rowsDirty = true
	if v.Doc != nil {
		v.pullCarets(v.Doc.Snapshot())
	}
	v.snapScrollVisible()
}

func (v *View) toggleFoldAt(line int) {
	f, ok := v.foldStarting(line)
	if !ok {
		return
	}
	if v.foldClosed(line) {
		delete(v.closed, line)
		v.rowsDirty = true
		return
	}
	v.closeFold(f)
}

func (v *View) expandEllipsis(line int) {
	if !v.foldClosed(line) {
		return
	}
	delete(v.closed, line)
	v.rowsDirty = true
}

func (v *View) expandTo(line int) {
	if len(v.closed) == 0 {
		return
	}
	changed := false
	for _, f := range v.folds {
		if _, ok := v.closed[f.Start]; !ok {
			continue
		}
		if f.Start < line && line <= f.End {
			delete(v.closed, f.Start)
			changed = true
		}
	}
	if changed {
		v.rowsDirty = true
	}
}

func (v *View) foldStarting(line int) (Fold, bool) {
	var best Fold
	found := false
	for _, f := range v.folds {
		if f.Start != line || f.End <= f.Start {
			continue
		}
		if !found || f.End < best.End {
			best = f
			found = true
		}
	}
	return best, found
}

func (v *View) foldClosed(line int) bool {
	if _, ok := v.closed[line]; !ok {
		return false
	}
	_, ok := v.foldStarting(line)
	return ok
}

func (v *View) outerOpen(line int) (Fold, bool) {
	var best Fold
	found := false
	for _, f := range v.folds {
		if f.End <= f.Start || v.foldClosed(f.Start) {
			continue
		}
		if f.Start >= line || line > f.End {
			continue
		}
		if !found || f.End-f.Start < best.End-best.Start {
			best = f
			found = true
		}
	}
	return best, found
}

func (v *View) innermostClosed(line int) (Fold, bool) {
	var best Fold
	found := false
	for _, f := range v.folds {
		if !v.foldClosed(f.Start) || f.End <= f.Start {
			continue
		}
		if f.Start != line && !(f.Start < line && line <= f.End) {
			continue
		}
		if !found || f.End-f.Start < best.End-best.Start {
			best = f
			found = true
		}
	}
	return best, found
}

func (v *View) visualCount() int {
	v.rebuildRows()
	return len(v.rows)
}

func (v *View) visualRow(line int) int {
	v.rebuildRows()
	if len(v.rows) == 0 {
		return 0
	}
	i := searchRow(v.rows, line)
	if i < len(v.rows) && v.rows[i] == line {
		return i
	}
	if i == 0 {
		return 0
	}
	return i - 1
}

func (v *View) lineAt(row int) int {
	v.rebuildRows()
	if len(v.rows) == 0 {
		return 0
	}
	if row < 0 {
		return v.rows[0]
	}
	if row >= len(v.rows) {
		return v.rows[len(v.rows)-1]
	}
	return v.rows[row]
}

func (v *View) lineHidden(line int) bool {
	v.rebuildRows()
	i := searchRow(v.rows, line)
	return i >= len(v.rows) || v.rows[i] != line
}

func (v *View) scrollRows(delta int) {
	v.setVisualTop(v.visualRow(v.scrollLn) + delta)
}

func (v *View) setVisualTop(row int) {
	v.scrollLn = v.lineAt(row)
	v.clampScroll()
}

func (v *View) rebuildRows() {
	n := 1
	if v.Doc != nil {
		n = v.Doc.Snapshot().LineCount()
	}
	if n < 1 {
		n = 1
	}
	if v.rows != nil && v.rowLines == n && !v.rowsDirty {
		return
	}
	diff := make([]int, n+1)
	for _, f := range v.folds {
		if _, ok := v.closed[f.Start]; !ok || f.End <= f.Start || f.Start < 0 {
			continue
		}
		from := f.Start + 1
		to := f.End + 1
		if from < 0 {
			from = 0
		}
		if from > n {
			continue
		}
		if to > n {
			to = n
		}
		if from < n {
			diff[from]++
		}
		if to <= n {
			diff[to]--
		}
	}
	rows := make([]int, 0, n)
	depth := 0
	for line := 0; line < n; line++ {
		depth += diff[line]
		if depth <= 0 {
			depth = 0
			rows = append(rows, line)
		}
	}
	if len(rows) == 0 {
		rows = []int{0}
	}
	v.rows = rows
	v.rowLines = n
	v.rowsDirty = false
}

func (v *View) snapScrollVisible() {
	if v.Doc == nil {
		if v.scrollLn < 0 {
			v.scrollLn = 0
		}
		return
	}
	n := v.Doc.Snapshot().LineCount()
	if v.scrollLn < 0 {
		v.scrollLn = 0
	}
	if n > 0 && v.scrollLn >= n {
		v.scrollLn = n - 1
	}
	if v.lineHidden(v.scrollLn) {
		v.scrollLn = v.lineAt(v.visualRow(v.scrollLn))
	}
	v.clampScroll()
}

func (v *View) pullCarets(snap buffer.Snapshot) {
	if len(v.sels) == 0 {
		return
	}
	seen := map[buffer.Offset]bool{}
	out := make([]Sel, 0, len(v.sels))
	for _, s := range v.sels {
		s.Head = v.pullOffset(snap, s.Head)
		s.Anchor = v.pullOffset(snap, s.Anchor)
		if s.empty() && seen[s.Head] {
			continue
		}
		seen[s.Head] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		out = []Sel{{}}
	}
	v.sels = out
}

func (v *View) pullOffset(snap buffer.Snapshot, off buffer.Offset) buffer.Offset {
	line, _ := snap.OffsetToLineCol(off)
	if !v.lineHidden(line) {
		return off
	}
	header := v.lineAt(v.visualRow(line))
	ln := snap.Line(header)
	return snap.LineStart(header) + buffer.Offset(len(ln))
}

func (v *View) expandFoldLines(lines []int) []int {
	if len(lines) == 0 || len(v.closed) == 0 {
		return lines
	}
	seen := map[int]bool{}
	var out []int
	add := func(line int) {
		if line < 0 || seen[line] {
			return
		}
		seen[line] = true
		out = append(out, line)
	}
	for _, line := range lines {
		add(line)
		f, ok := v.foldStarting(line)
		if !ok || !v.foldClosed(line) {
			continue
		}
		for i := f.Start; i <= f.End; i++ {
			add(i)
		}
	}
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j] < out[j-1] {
			out[j], out[j-1] = out[j-1], out[j]
			j--
		}
	}
	return out
}

// gutterMark is the git stripe for a visible line. A clean collapsed header
// shows modified when a hidden line has a mark.
func (v *View) gutterMark(line int) (byte, bool) {
	if k, ok := v.gutter[line]; ok {
		return k, true
	}
	if !v.foldClosed(line) {
		return 0, false
	}
	f, ok := v.foldStarting(line)
	if !ok {
		return 0, false
	}
	for i := f.Start + 1; i <= f.End; i++ {
		if _, marked := v.gutter[i]; marked {
			return 'm', true
		}
	}
	return 0, false
}

func searchRow(rows []int, line int) int {
	lo, hi := 0, len(rows)
	for lo < hi {
		mid := (lo + hi) / 2
		if rows[mid] < line {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func lineStartBytes(b []byte, line int) int {
	if line <= 0 {
		return 0
	}
	n := 0
	for i, c := range b {
		if c == '\n' {
			n++
			if n == line {
				return i + 1
			}
		}
	}
	return len(b)
}

func countNL(s string) int {
	n := 0
	for _, c := range s {
		if c == '\n' {
			n++
		}
	}
	return n
}

func clampEdit(old []byte, ed buffer.Edit) buffer.Edit {
	start := int(ed.Start)
	end := int(ed.End)
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if start > len(old) {
		start = len(old)
	}
	if end > len(old) {
		end = len(old)
	}
	ed.Start = buffer.Offset(start)
	ed.End = buffer.Offset(end)
	return ed
}

// splitLineEdits returns the line runs inside ed that differ, in order.
// Equal lines between two changes stay out of every edit. A region larger
// than the matcher will search is returned as the original edit.
func splitLineEdits(old []byte, ed buffer.Edit) []buffer.Edit {
	oldPart := old[int(ed.Start):int(ed.End)]
	neu := []byte(ed.Text)
	if bytes.Equal(oldPart, neu) {
		return nil
	}
	a := splitKeepNL(oldPart)
	b := splitKeepNL(neu)
	hunks := lineHunks(a, b)
	if len(hunks) == 0 {
		return nil
	}
	if len(hunks) == 1 && hunks[0].a0 == 0 && hunks[0].a1 == len(a) && hunks[0].b0 == 0 && hunks[0].b1 == len(b) {
		return []buffer.Edit{ed}
	}
	offA := lineOffsets(a)
	base := int(ed.Start)
	out := make([]buffer.Edit, 0, len(hunks))
	for _, h := range hunks {
		if h.a0 == h.a1 && h.b0 == h.b1 {
			continue
		}
		var text []byte
		for _, ln := range b[h.b0:h.b1] {
			text = append(text, ln...)
		}
		out = append(out, buffer.Edit{
			Start: buffer.Offset(base + offA[h.a0]),
			End:   buffer.Offset(base + offA[h.a1]),
			Text:  string(text),
		})
	}
	return out
}

type lineHunk struct {
	a0, a1 int
	b0, b1 int
}

func lineHunks(a, b [][]byte) []lineHunk {
	var out []lineHunk
	var walk func(a0, a1, b0, b1 int)
	walk = func(a0, a1, b0, b1 int) {
		if a0 == a1 && b0 == b1 {
			return
		}
		span := (a1 - a0) * (b1 - b0)
		if a0 == a1 || b0 == b1 || span > 640000 {
			out = append(out, lineHunk{a0, a1, b0, b1})
			return
		}
		ai, bi, n := longestLineMatch(a[a0:a1], b[b0:b1])
		if n == 0 {
			out = append(out, lineHunk{a0, a1, b0, b1})
			return
		}
		ai += a0
		bi += b0
		walk(a0, ai, b0, bi)
		walk(ai+n, a1, bi+n, b1)
	}
	walk(0, len(a), 0, len(b))
	return out
}

func longestLineMatch(a, b [][]byte) (ai, bi, n int) {
	for i := range a {
		for j := range b {
			if !bytes.Equal(a[i], b[j]) {
				continue
			}
			k := 1
			for i+k < len(a) && j+k < len(b) && bytes.Equal(a[i+k], b[j+k]) {
				k++
			}
			if k > n {
				ai, bi, n = i, j, k
			}
		}
	}
	return ai, bi, n
}

func splitKeepNL(b []byte) [][]byte {
	if len(b) == 0 {
		return nil
	}
	var out [][]byte
	for i := 0; i < len(b); {
		j := i
		for j < len(b) && b[j] != '\n' {
			j++
		}
		if j < len(b) {
			j++
		}
		out = append(out, b[i:j])
		i = j
	}
	return out
}

func lineOffsets(lines [][]byte) []int {
	off := make([]int, len(lines)+1)
	n := 0
	for i, ln := range lines {
		off[i] = n
		n += len(ln)
	}
	off[len(lines)] = n
	return off
}
