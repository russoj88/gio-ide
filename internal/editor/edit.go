// Package editor is the code editor view: caret, selections, marks, and edits.
// The Gio widget lives in widget.go. Editing logic does not need a window.
package editor

import (
	"unicode/utf8"

	"github.com/russoj88/gio-ide/internal/buffer"
)

// Sel is one caret or selection. Anchor is where the selection started.
// Head is the caret.
type Sel struct {
	Anchor, Head buffer.Offset
}

func (s Sel) min() buffer.Offset {
	if s.Anchor < s.Head {
		return s.Anchor
	}
	return s.Head
}

func (s Sel) max() buffer.Offset {
	if s.Anchor > s.Head {
		return s.Anchor
	}
	return s.Head
}

func (s Sel) empty() bool { return s.Anchor == s.Head }

// View is the editing state for one document.
type View struct {
	Doc       *buffer.Doc
	sels      []Sel
	prefCol   int
	scrollLn  int
	scrollX   int
	pageLines int
	TabWidth  int

	// OnFocus runs on the UI goroutine when the user presses this view.
	OnFocus func()

	ui     viewUI
	gutter map[int]byte
	marks  []Mark

	folds     []Fold
	foldVer   int32
	closed    map[int]struct{}
	rows      []int
	rowLines  int
	rowsDirty bool
}

// SetGutter replaces diff marks. Line is 0-based. Kind is 'a', 'm', or 'd'.
func (v *View) SetGutter(marks map[int]byte) {
	v.gutter = marks
}

// NewView returns a view with the caret at the start.
func NewView(doc *buffer.Doc) *View {
	return &View{
		Doc:       doc,
		sels:      []Sel{{}},
		TabWidth:  4,
		pageLines: 40,
	}
}

func (v *View) Selections() []Sel {
	out := make([]Sel, len(v.sels))
	copy(out, v.sels)
	return out
}

func (v *View) SetSelections(s []Sel) {
	if len(s) == 0 {
		s = []Sel{{}}
	}
	v.sels = append([]Sel(nil), s...)
	v.clamp()
}

func (v *View) ScrollLine() int { return v.scrollLn }

// SetScrollLine scrolls so line n is the first visible line.
// n is clamped to the lines that still leave the end of the file on screen.
func (v *View) SetScrollLine(n int) {
	v.scrollLn = n
	v.clampScroll()
}

// maxScrollLine is the largest first-visible buffer line that still shows the
// last visible row in the viewport. pageLines is the last laid-out viewport,
// or the default 40 before the first layout. With nothing collapsed this is
// LineCount minus the page.
func (v *View) maxScrollLine() int {
	page := max(v.pageLines, 1)
	rows := v.visualCount()
	maxRow := max(rows-page, 0)
	return v.lineAt(maxRow)
}

func (v *View) clampScroll() {
	if v.scrollLn < 0 {
		v.scrollLn = 0
	}
	if v.lineHidden(v.scrollLn) {
		v.scrollLn = v.lineAt(v.visualRow(v.scrollLn))
	}
	if maxLn := v.maxScrollLine(); v.scrollLn > maxLn {
		v.scrollLn = maxLn
	}
}

func (v *View) ScrollX() int { return v.scrollX }

// Primary returns the caret line and byte column (1-based line for the status bar
// is computed by the caller). The line here is 0-based.
func (v *View) CaretPos() (line, col int) {
	s := v.Doc.Snapshot()
	return s.OffsetToLineCol(v.primary().Head)
}

// CaretOffset is the primary caret as a byte offset.
func (v *View) CaretOffset() buffer.Offset { return v.primary().Head }

// SelectionBounds returns the primary selection as byte offsets.
func (v *View) SelectionBounds() (start, end int) {
	s := v.primary()
	return int(s.min()), int(s.max())
}

func (v *View) primary() Sel {
	if len(v.sels) == 0 {
		v.sels = []Sel{{}}
	}
	return v.sels[len(v.sels)-1]
}

func (v *View) setPrimary(s Sel) {
	if len(v.sels) == 0 {
		v.sels = []Sel{s}
		return
	}
	v.sels[len(v.sels)-1] = s
}

func (v *View) clamp() {
	n := buffer.Offset(v.Doc.Snapshot().Len())
	for i, s := range v.sels {
		if s.Anchor < 0 {
			s.Anchor = 0
		}
		if s.Head < 0 {
			s.Head = 0
		}
		if s.Anchor > n {
			s.Anchor = n
		}
		if s.Head > n {
			s.Head = n
		}
		v.sels[i] = s
	}
}

// Insert replaces every selection with text, as one undo step when origin is a command.
func (v *View) Insert(text string, origin buffer.Origin) {
	if v.Doc.ReadOnly() {
		return
	}
	v.sortSels()
	edits := make([]buffer.Edit, 0, len(v.sels))
	for _, s := range v.sels {
		edits = append(edits, buffer.Edit{Start: s.min(), End: s.max(), Text: text})
	}
	if _, err := v.Doc.Apply(edits, origin); err != nil {
		return
	}
	// Carets move by the net insertion of earlier edits.
	shift := 0
	for i, s := range v.sels {
		at := s.min() + buffer.Offset(shift+len(text))
		v.sels[i] = Sel{Anchor: at, Head: at}
		shift += len(text) - int(s.max()-s.min())
	}
	v.prefFromHead()
	v.Reveal()
}

func (v *View) Type(text string) { v.Insert(text, buffer.OriginTyping) }

func (v *View) Backspace() {
	if v.Doc.ReadOnly() {
		return
	}
	snap := v.Doc.Snapshot()
	b := snap.Bytes()
	v.sortSels()
	edits := make([]buffer.Edit, 0, len(v.sels))
	for _, s := range v.sels {
		if !s.empty() {
			edits = append(edits, buffer.Edit{Start: s.min(), End: s.max()})
			continue
		}
		if s.Head == 0 {
			continue
		}
		start := buffer.Offset(prevRune(b, int(s.Head)))
		edits = append(edits, buffer.Edit{Start: start, End: s.Head})
	}
	if len(edits) == 0 {
		return
	}
	if _, err := v.Doc.Apply(edits, buffer.OriginTyping); err != nil {
		return
	}
	shift := 0
	for i, s := range v.sels {
		del := 0
		if !s.empty() {
			del = int(s.max() - s.min())
		} else if s.Head > 0 {
			del = int(s.Head) - prevRune(b, int(s.Head))
		}
		at := s.min()
		if s.empty() && s.Head > 0 {
			at = buffer.Offset(prevRune(b, int(s.Head)))
		}
		at -= buffer.Offset(shift)
		// shift tracks bytes already removed before this caret.
		v.sels[i] = Sel{Anchor: at, Head: at}
		shift += del
	}
	v.prefFromHead()
	v.Reveal()
}

func (v *View) DeleteForward() {
	if v.Doc.ReadOnly() {
		return
	}
	snap := v.Doc.Snapshot()
	b := snap.Bytes()
	v.sortSels()
	var edits []buffer.Edit
	for _, s := range v.sels {
		if !s.empty() {
			edits = append(edits, buffer.Edit{Start: s.min(), End: s.max()})
			continue
		}
		if int(s.Head) >= len(b) {
			continue
		}
		_, sz := utf8.DecodeRune(b[s.Head:])
		edits = append(edits, buffer.Edit{Start: s.Head, End: s.Head + buffer.Offset(sz)})
	}
	if len(edits) == 0 {
		return
	}
	_, _ = v.Doc.Apply(edits, buffer.OriginTyping)
	shift := 0
	for i, s := range v.sels {
		at := s.min() - buffer.Offset(shift)
		v.sels[i] = Sel{Anchor: at, Head: at}
		if !s.empty() {
			shift += int(s.max() - s.min())
		} else if int(s.Head) < len(b) {
			_, sz := utf8.DecodeRune(b[s.Head:])
			shift += sz
		}
	}
	v.Reveal()
}

func (v *View) Undo() bool {
	before := v.Doc.Snapshot().Bytes()
	if !v.Doc.Undo() {
		return false
	}
	v.placeCaretAtChange(before, v.Doc.Snapshot().Bytes())
	return true
}

func (v *View) Redo() bool {
	before := v.Doc.Snapshot().Bytes()
	if !v.Doc.Redo() {
		return false
	}
	v.placeCaretAtChange(before, v.Doc.Snapshot().Bytes())
	return true
}

// placeCaretAtChange puts the caret at the end of the region that differs.
// Undo and redo do not store a caret, so the caret is the end of that edit
// in the text that is on screen now.
func (v *View) placeCaretAtChange(before, after []byte) {
	i := 0
	n := min(len(before), len(after))
	for i < n && before[i] == after[i] {
		i++
	}
	b := len(before) - 1
	a := len(after) - 1
	for b >= i && a >= i && before[b] == after[a] {
		b--
		a--
	}
	at := min(max(a+1, 0), len(after))
	off := buffer.Offset(at)
	v.sels = []Sel{{Anchor: off, Head: off}}
	v.clamp()
	v.prefFromHead()
	v.Doc.BreakGroup()
	v.Reveal()
}

func (v *View) collapseToEnd() {
	n := buffer.Offset(v.Doc.Snapshot().Len())
	v.sels = []Sel{{Anchor: n, Head: n}}
	v.Doc.BreakGroup()
	v.Reveal()
}

func (v *View) SelectAll() {
	n := buffer.Offset(v.Doc.Snapshot().Len())
	v.sels = []Sel{{Anchor: 0, Head: n}}
	v.Doc.BreakGroup()
}

// HasSelection reports whether any caret has a non-empty range.
func (v *View) HasSelection() bool {
	for _, s := range v.sels {
		if !s.empty() {
			return true
		}
	}
	return false
}

func (v *View) SelectedText() string {
	s := v.primary()
	if s.empty() {
		return ""
	}
	return string(v.Doc.Snapshot().Slice(s.min(), s.max()))
}

func (v *View) Cut() string {
	text := v.SelectedText()
	if text == "" || v.Doc.ReadOnly() {
		return text
	}
	v.sortSels()
	var edits []buffer.Edit
	for _, s := range v.sels {
		if s.empty() {
			continue
		}
		edits = append(edits, buffer.Edit{Start: s.min(), End: s.max()})
	}
	_, _ = v.Doc.Apply(edits, buffer.OriginCommand)
	shift := 0
	for i, s := range v.sels {
		at := s.min() - buffer.Offset(shift)
		v.sels[i] = Sel{Anchor: at, Head: at}
		if !s.empty() {
			shift += int(s.max() - s.min())
		}
	}
	return text
}

func (v *View) Paste(text string) {
	if text == "" {
		return
	}
	v.Insert(text, buffer.OriginCommand)
}

func (v *View) MoveLeft(extend bool)  { v.moveRune(-1, extend) }
func (v *View) MoveRight(extend bool) { v.moveRune(1, extend) }

func (v *View) moveRune(dir int, extend bool) {
	b := v.Doc.Snapshot().Bytes()
	v.Doc.BreakGroup()
	for i, s := range v.sels {
		head := int(s.Head)
		if dir < 0 {
			head = prevRune(b, head)
		} else if head < len(b) {
			_, sz := utf8.DecodeRune(b[head:])
			head += sz
		}
		s.Head = buffer.Offset(head)
		if !extend {
			s.Anchor = s.Head
		}
		v.sels[i] = s
	}
	v.prefFromHead()
	v.Reveal()
}

func (v *View) MoveUp(extend bool)   { v.moveVert(-1, extend) }
func (v *View) MoveDown(extend bool) { v.moveVert(1, extend) }

func (v *View) moveVert(dir int, extend bool) {
	snap := v.Doc.Snapshot()
	v.Doc.BreakGroup()
	last := max(v.visualCount()-1, 0)
	for i, s := range v.sels {
		line, _ := snap.OffsetToLineCol(s.Head)
		row := min(max(v.visualRow(line)+dir, 0), last)
		s.Head = v.offsetAt(snap, v.lineAt(row), v.prefCol)
		if !extend {
			s.Anchor = s.Head
		}
		v.sels[i] = s
	}
	v.Reveal()
}

func (v *View) MoveLineStart(extend bool) { v.moveLineEdge(true, extend) }
func (v *View) MoveLineEnd(extend bool)   { v.moveLineEdge(false, extend) }

func (v *View) moveLineEdge(start, extend bool) {
	snap := v.Doc.Snapshot()
	v.Doc.BreakGroup()
	for i, s := range v.sels {
		line, _ := snap.OffsetToLineCol(s.Head)
		if start {
			s.Head = snap.LineStart(line)
		} else {
			ln := snap.Line(line)
			s.Head = snap.LineStart(line) + buffer.Offset(len(ln))
		}
		if !extend {
			s.Anchor = s.Head
		}
		v.sels[i] = s
	}
	v.prefFromHead()
	v.Reveal()
}

func (v *View) MoveDocStart(extend bool) {
	v.Doc.BreakGroup()
	for i, s := range v.sels {
		s.Head = 0
		if !extend {
			s.Anchor = 0
		}
		v.sels[i] = s
	}
	v.prefCol = 0
	v.Reveal()
}

func (v *View) MoveDocEnd(extend bool) {
	n := buffer.Offset(v.Doc.Snapshot().Len())
	v.Doc.BreakGroup()
	for i, s := range v.sels {
		s.Head = n
		if !extend {
			s.Anchor = n
		}
		v.sels[i] = s
	}
	v.prefFromHead()
	v.Reveal()
}

func (v *View) Page(dir int, extend bool) {
	if v.pageLines < 1 {
		v.pageLines = 20
	}
	for i := 0; i < v.pageLines; i++ {
		v.moveVert(dir, extend)
	}
}

// Click places the caret at a byte column on a 0-based buffer line.
// Folds that hide that line open first.
func (v *View) Click(line, col int, extend bool) {
	snap := v.Doc.Snapshot()
	if line < 0 {
		line = 0
	}
	if line >= snap.LineCount() {
		line = snap.LineCount() - 1
	}
	v.expandTo(line)
	off := v.offsetAt(snap, line, col)
	s := v.primary()
	s.Head = off
	if !extend {
		s.Anchor = off
	}
	v.sels = []Sel{s}
	v.prefCol = col
	v.Doc.BreakGroup()
	v.Reveal()
}

func (v *View) Reveal() {
	snap := v.Doc.Snapshot()
	line, _ := snap.OffsetToLineCol(v.primary().Head)
	v.expandTo(line)
	margin := 2
	if v.pageLines < 1 {
		v.pageLines = 40
	}
	row := v.visualRow(line)
	top := v.visualRow(v.scrollLn)
	if row < top+margin {
		v.setVisualTop(row - margin)
	}
	top = v.visualRow(v.scrollLn)
	if row >= top+v.pageLines-margin {
		v.setVisualTop(row - v.pageLines + margin + 1)
	}
	v.clampScroll()
}

func (v *View) SetPageLines(n int) {
	if n < 1 {
		n = 1
	}
	v.pageLines = n
}

func (v *View) offsetAt(snap buffer.Snapshot, line, col int) buffer.Offset {
	if line < 0 {
		line = 0
	}
	if line >= snap.LineCount() {
		return buffer.Offset(snap.Len())
	}
	ln := snap.Line(line)
	if col < 0 {
		col = 0
	}
	// col is a byte column. len(ln) is the position after the last byte
	// (the newline was stripped). Indexing ln[col] there panics.
	if col > len(ln) {
		col = len(ln)
	}
	for col > 0 && col < len(ln) && !utf8.RuneStart(ln[col]) {
		col--
	}
	return snap.LineStart(line) + buffer.Offset(col)
}

func (v *View) prefFromHead() {
	_, col := v.Doc.Snapshot().OffsetToLineCol(v.primary().Head)
	v.prefCol = col
}

func (v *View) sortSels() {
	// Insertion sort. Selections are few.
	for i := 1; i < len(v.sels); i++ {
		j := i
		for j > 0 && v.sels[j].min() < v.sels[j-1].min() {
			v.sels[j], v.sels[j-1] = v.sels[j-1], v.sels[j]
			j--
		}
	}
}

func prevRune(b []byte, off int) int {
	if off <= 0 {
		return 0
	}
	if off > len(b) {
		off = len(b)
	}
	_, sz := utf8.DecodeLastRune(b[:off])
	if sz < 1 {
		return off - 1
	}
	return off - sz
}

// RuneIndex converts a byte offset to a rune index.
func RuneIndex(b []byte, off int) int {
	if off > len(b) {
		off = len(b)
	}
	if off < 0 {
		off = 0
	}
	return utf8.RuneCount(b[:off])
}

// ByteIndex converts a rune index to a byte offset.
func ByteIndex(b []byte, runes int) int {
	if runes <= 0 {
		return 0
	}
	i := 0
	for r := 0; r < runes && i < len(b); r++ {
		_, sz := utf8.DecodeRune(b[i:])
		if sz < 1 {
			break
		}
		i += sz
	}
	return i
}
