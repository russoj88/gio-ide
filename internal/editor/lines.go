package editor

import (
	"slices"
	"unicode"
	"unicode/utf8"

	"github.com/russoj88/gio-ide/internal/buffer"
)

// AddNextOccurrence selects the next copy of the primary selection.
// An empty caret first selects the word under it.
func (v *View) AddNextOccurrence() {
	v.ensureWord()
	s := v.primary()
	if s.empty() {
		return
	}
	needle := string(v.Doc.Snapshot().Slice(s.min(), s.max()))
	if needle == "" {
		return
	}
	b := v.Doc.Snapshot().Bytes()
	from := int(s.max())
	idx := indexFrom(b, needle, from)
	if idx < 0 {
		idx = indexFrom(b, needle, 0)
	}
	if idx < 0 || v.covers(idx, idx+len(needle)) {
		return
	}
	v.sels = append(v.sels, Sel{Anchor: buffer.Offset(idx), Head: buffer.Offset(idx + len(needle))})
	v.Doc.BreakGroup()
	v.Reveal()
}

// SelectAllOccurrences selects every copy of the primary selection.
func (v *View) SelectAllOccurrences() {
	v.ensureWord()
	s := v.primary()
	if s.empty() {
		return
	}
	needle := string(v.Doc.Snapshot().Slice(s.min(), s.max()))
	if needle == "" {
		return
	}
	b := v.Doc.Snapshot().Bytes()
	var sels []Sel
	for from := 0; from <= len(b)-len(needle); {
		idx := indexFrom(b, needle, from)
		if idx < 0 {
			break
		}
		sels = append(sels, Sel{Anchor: buffer.Offset(idx), Head: buffer.Offset(idx + len(needle))})
		from = idx + len(needle)
	}
	if len(sels) == 0 {
		return
	}
	v.sels = sels
	v.Doc.BreakGroup()
	v.Reveal()
}

// UnselectOccurrence drops the primary selection. The last caret collapses.
func (v *View) UnselectOccurrence() {
	if len(v.sels) <= 1 {
		s := v.primary()
		s.Anchor = s.Head
		v.setPrimary(s)
		return
	}
	v.sels = v.sels[:len(v.sels)-1]
	v.Reveal()
}

// AddCaretBelow adds a caret on the next visible line at the preferred column.
func (v *View) AddCaretBelow() {
	snap := v.Doc.Snapshot()
	s := v.primary()
	line, _ := snap.OffsetToLineCol(s.Head)
	row := v.visualRow(line) + 1
	if row >= v.visualCount() {
		return
	}
	off := v.offsetAt(snap, v.lineAt(row), v.prefCol)
	v.sels = append(v.sels, Sel{Anchor: off, Head: off})
	v.Doc.BreakGroup()
	v.Reveal()
}

// DuplicateLine copies each selected line once, as one undo step.
func (v *View) DuplicateLine() {
	snap := v.Doc.Snapshot()
	lines := v.expandFoldLines(v.selectedLines(snap))
	var edits []buffer.Edit
	for _, line := range slices.Backward(lines) {
		body := snap.Line(line)
		at := snap.LineStart(line) + buffer.Offset(len(body))
		edits = append(edits, buffer.Edit{Start: at, End: at, Text: "\n" + string(body)})
	}
	// Apply expects ascending order.
	for i, j := 0, len(edits)-1; i < j; i, j = i+1, j-1 {
		edits[i], edits[j] = edits[j], edits[i]
	}
	if _, err := v.Doc.Apply(edits, buffer.OriginCommand); err != nil {
		return
	}
	v.collapseToEnd()
}

// DeleteLine removes each selected line, as one undo step.
func (v *View) DeleteLine() {
	snap := v.Doc.Snapshot()
	lines := v.expandFoldLines(v.selectedLines(snap))
	var edits []buffer.Edit
	prevSelected := map[int]bool{}
	for _, line := range lines {
		prevSelected[line] = true
	}
	for _, line := range lines {
		start := snap.LineStart(line)
		body := snap.Line(line)
		end := start + buffer.Offset(len(body))
		if line+1 < snap.LineCount() {
			end = snap.LineStart(line + 1)
		} else if start > 0 && !prevSelected[line-1] {
			start--
		}
		edits = append(edits, buffer.Edit{Start: start, End: end})
	}
	if _, err := v.Doc.Apply(edits, buffer.OriginCommand); err != nil {
		return
	}
	n := buffer.Offset(v.Doc.Snapshot().Len())
	if len(edits) > 0 && edits[0].Start < n {
		v.sels = []Sel{{Anchor: edits[0].Start, Head: edits[0].Start}}
	} else {
		v.sels = []Sel{{Anchor: n, Head: n}}
	}
	v.Reveal()
}

// ToggleLineComment adds or removes a // comment on each selected line.
func (v *View) ToggleLineComment() {
	snap := v.Doc.Snapshot()
	lines := v.selectedLines(snap)
	uncomment := true
	for _, line := range lines {
		if !hasLineComment(snap.Line(line)) {
			uncomment = false
			break
		}
	}
	var edits []buffer.Edit
	for _, line := range lines {
		body := snap.Line(line)
		start := snap.LineStart(line)
		if uncomment {
			rel := commentStart(body)
			if rel < 0 {
				continue
			}
			end := rel + 2
			if end < len(body) && body[end] == ' ' {
				end++
			}
			edits = append(edits, buffer.Edit{
				Start: start + buffer.Offset(rel),
				End:   start + buffer.Offset(end),
			})
			continue
		}
		rel := skipSpace(body)
		edits = append(edits, buffer.Edit{
			Start: start + buffer.Offset(rel),
			End:   start + buffer.Offset(rel),
			Text:  "// ",
		})
	}
	if len(edits) == 0 {
		return
	}
	_, _ = v.Doc.Apply(edits, buffer.OriginCommand)
	v.Doc.BreakGroup()
}

func (v *View) ensureWord() {
	s := v.primary()
	if !s.empty() {
		return
	}
	b := v.Doc.Snapshot().Bytes()
	a, z := wordBounds(b, int(s.Head))
	if a == z {
		return
	}
	v.setPrimary(Sel{Anchor: buffer.Offset(a), Head: buffer.Offset(z)})
}

func (v *View) covers(start, end int) bool {
	for _, s := range v.sels {
		if int(s.min()) == start && int(s.max()) == end {
			return true
		}
	}
	return false
}

func (v *View) selectedLines(snap buffer.Snapshot) []int {
	seen := map[int]bool{}
	var lines []int
	for _, s := range v.sels {
		a := int(s.min())
		b := int(s.max())
		if b > a {
			b--
		}
		la, _ := snap.OffsetToLineCol(buffer.Offset(a))
		lb, _ := snap.OffsetToLineCol(buffer.Offset(b))
		if lb < la {
			la, lb = lb, la
		}
		for line := la; line <= lb; line++ {
			if seen[line] {
				continue
			}
			seen[line] = true
			lines = append(lines, line)
		}
	}
	// Ascending.
	for i := 1; i < len(lines); i++ {
		j := i
		for j > 0 && lines[j] < lines[j-1] {
			lines[j], lines[j-1] = lines[j-1], lines[j]
			j--
		}
	}
	return lines
}

func indexFrom(b []byte, needle string, from int) int {
	if from < 0 {
		from = 0
	}
	if from > len(b) {
		return -1
	}
	rest := b[from:]
	idx := bytesIndex(rest, []byte(needle))
	if idx < 0 {
		return -1
	}
	return from + idx
}

func bytesIndex(b, needle []byte) int {
	if len(needle) == 0 || len(needle) > len(b) {
		return -1
	}
	for i := 0; i+len(needle) <= len(b); i++ {
		match := true
		for j := range needle {
			if b[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func wordBounds(b []byte, off int) (int, int) {
	if off < 0 {
		off = 0
	}
	if off > len(b) {
		off = len(b)
	}
	if off == len(b) && off > 0 {
		off--
	}
	if off >= len(b) || !isWordByte(b[off]) {
		if off > 0 && isWordByte(b[off-1]) {
			off--
		} else {
			return off, off
		}
	}
	a := off
	for a > 0 && isWordByte(b[a-1]) {
		a--
	}
	z := off + 1
	for z < len(b) && isWordByte(b[z]) {
		z++
	}
	return a, z
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func hasLineComment(line []byte) bool {
	return commentStart(line) >= 0
}

func commentStart(line []byte) int {
	i := skipSpace(line)
	if i+1 < len(line) && line[i] == '/' && line[i+1] == '/' {
		return i
	}
	return -1
}

func skipSpace(line []byte) int {
	i := 0
	for i < len(line) {
		r, sz := utf8.DecodeRune(line[i:])
		if !unicode.IsSpace(r) {
			break
		}
		i += sz
	}
	return i
}
