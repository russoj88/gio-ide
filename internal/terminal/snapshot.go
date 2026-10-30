package terminal

import "strings"

// ScreenSnapshot is a copy of the visible grid.
type ScreenSnapshot struct {
	Cols, Rows int
	Cells      []Cell
	CursorX    int
	CursorY    int
	CursorOn   bool
	Title      string
	Unknown    int
	AppCursor  bool
	Bracketed  bool
	History    int
}

// Text renders the grid as lines. Wide spacers are skipped.
func (s ScreenSnapshot) Text() string {
	var b strings.Builder
	for y := 0; y < s.Rows; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		for x := 0; x < s.Cols; x++ {
			c := s.Cells[y*s.Cols+x]
			if c.W < 0 {
				continue
			}
			if c.R == 0 {
				b.WriteByte(' ')
				continue
			}
			b.WriteRune(c.R)
		}
	}
	return strings.TrimRight(b.String(), " \n")
}

// Snapshot copies the live screen. The cursor may sit one column past the last cell.
func (sc *Screen) Snapshot() ScreenSnapshot {
	g := sc.grid()
	cells := append([]Cell(nil), g...)
	x := sc.curX
	if x >= sc.cols {
		x = sc.cols - 1
	}
	return ScreenSnapshot{
		Cols:      sc.cols,
		Rows:      sc.rows,
		Cells:     cells,
		CursorX:   x,
		CursorY:   sc.curY,
		CursorOn:  sc.cursorOn,
		Title:     sc.title,
		Unknown:   sc.unknown,
		AppCursor: sc.appCursor,
		Bracketed: sc.bracketed,
		History:   len(sc.hist),
	}
}

// LineText returns one live row without trailing spaces.
func (sc *Screen) LineText(y int) string {
	if y < 0 || y >= sc.rows {
		return ""
	}
	g := sc.grid()
	var b strings.Builder
	for x := 0; x < sc.cols; x++ {
		c := g[y*sc.cols+x]
		if c.W < 0 {
			continue
		}
		if c.R == 0 {
			b.WriteByte(' ')
			continue
		}
		b.WriteRune(c.R)
	}
	return strings.TrimRight(b.String(), " ")
}
