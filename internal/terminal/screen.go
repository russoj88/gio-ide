package terminal

import (
	"github.com/danielgatis/go-vte"
	"golang.org/x/text/width"
)

// Cell is one column of the screen.
// W is 1 for a normal rune, 2 for a wide rune, and -1 for the spacer after it.
type Cell struct {
	R rune
	A Attr
	W int8
}

// Color is a terminal color. Kind 0 is the default, 1 is a palette index, 2 is RGB.
type Color struct {
	Kind    uint8
	Idx     uint8
	R, G, B uint8
}

// Attr is the SGR style of a cell.
type Attr struct {
	FG, BG                           Color
	Bold, Italic, Underline, Inverse bool
}

// Screen is the in-house grid that go-vte feeds.
type Screen struct {
	cols, rows int
	primary    []Cell
	alt        []Cell
	usingAlt   bool
	hist       [][]Cell
	maxHist    int

	curX, curY           int
	savedX, savedY       int
	attr                 Attr
	scrollTop, scrollBot int
	autowrap             bool
	cursorOn             bool
	appCursor            bool
	bracketed            bool
	wrapNext             bool
	title                string
	unknown              int
	logf                 func(string)

	parser *vte.Parser
}

// NewScreen returns a screen with a full-height scroll region and autowrap on.
func NewScreen(cols, rows, scrollback int) *Screen {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	if scrollback < 0 {
		scrollback = 0
	}
	sc := &Screen{
		cols:      cols,
		rows:      rows,
		primary:   make([]Cell, cols*rows),
		alt:       make([]Cell, cols*rows),
		maxHist:   scrollback,
		autowrap:  true,
		cursorOn:  true,
		scrollBot: rows - 1,
	}
	sc.parser = vte.NewParser(sc)
	return sc
}

// Feed parses bytes with go-vte and applies them to the grid.
func (sc *Screen) Feed(p []byte) {
	for _, b := range p {
		sc.parser.Advance(b)
	}
}

func (sc *Screen) grid() []Cell {
	if sc.usingAlt {
		return sc.alt
	}
	return sc.primary
}

func (sc *Screen) note(kind string) {
	sc.unknown++
	if sc.logf != nil {
		sc.logf("terminal: unknown " + kind)
	}
}

// Print implements vte.Performer.
func (sc *Screen) Print(r rune) {
	w := runeWidth(r)
	if w == 0 {
		return
	}
	if sc.wrapNext {
		sc.linefeed()
		sc.curX = 0
		sc.wrapNext = false
	}
	if sc.curX+w > sc.cols {
		if sc.autowrap {
			sc.linefeed()
			sc.curX = 0
		} else if sc.cols >= w {
			sc.curX = sc.cols - w
		} else {
			sc.curX = 0
			w = 1
		}
	}
	sc.put(sc.curX, sc.curY, r, w)
	sc.curX += w
	if sc.curX >= sc.cols {
		if sc.autowrap {
			sc.curX = sc.cols
			sc.wrapNext = true
		} else {
			sc.curX = sc.cols - w
		}
	}
}

// Execute implements vte.Performer for C0 controls.
func (sc *Screen) Execute(b byte) {
	switch b {
	case 0x07: // BEL
	case 0x08: // BS
		sc.wrapNext = false
		if sc.curX > 0 {
			sc.curX--
		}
	case 0x09: // HT
		sc.wrapNext = false
		next := (sc.curX/8 + 1) * 8
		if next >= sc.cols {
			next = sc.cols - 1
		}
		sc.curX = next
	case 0x0a, 0x0b, 0x0c: // LF, VT, FF
		sc.linefeed()
	case 0x0d: // CR
		sc.wrapNext = false
		sc.curX = 0
	default:
		if b < 0x20 || b == 0x7f {
			return
		}
		sc.note("c0")
	}
}

// Hook implements vte.Performer. DCS payloads are ignored.
func (sc *Screen) Hook(params [][]uint16, intermediates []byte, ignore bool, r rune) {}

// Put implements vte.Performer.
func (sc *Screen) Put(b byte) {}

// Unhook implements vte.Performer.
func (sc *Screen) Unhook() { sc.note("dcs") }

// OscDispatch implements vte.Performer. OSC 0 and 2 set the title.
func (sc *Screen) OscDispatch(params [][]byte, _ bool) {
	if len(params) == 0 {
		sc.note("osc")
		return
	}
	code := string(params[0])
	if code != "0" && code != "2" {
		sc.note("osc " + code)
		return
	}
	title := ""
	if len(params) > 1 {
		title = string(params[1])
		for _, p := range params[2:] {
			title += ";" + string(p)
		}
	}
	sc.title = title
}

// EscDispatch implements vte.Performer.
func (sc *Screen) EscDispatch(intermediates []byte, ignore bool, b byte) {
	if ignore {
		sc.note("esc")
		return
	}
	sc.note("esc")
}

// SosPmApcDispatch implements vte.Performer.
func (sc *Screen) SosPmApcDispatch(kind vte.SosPmApcKind, _ []byte, _ bool) {
	sc.note("sos")
	_ = kind
}

// CsiDispatch implements vte.Performer.
func (sc *Screen) CsiDispatch(params [][]uint16, intermediates []byte, ignore bool, r rune) {
	if ignore {
		sc.note("csi")
		return
	}
	if len(intermediates) == 1 && intermediates[0] == '?' {
		on := r == 'h'
		if r != 'h' && r != 'l' {
			sc.note("csi ?")
			return
		}
		if len(params) == 0 {
			sc.setMode(0, on)
			return
		}
		for _, p := range params {
			if len(p) == 0 {
				continue
			}
			sc.setMode(int(p[0]), on)
		}
		return
	}
	if len(intermediates) > 0 {
		sc.note("csi")
		return
	}
	switch r {
	case 'H', 'f':
		sc.cup(pn(params, 0, 1)-1, pn(params, 1, 1)-1)
	case 'A':
		sc.moveY(-pn(params, 0, 1))
	case 'B':
		sc.moveY(pn(params, 0, 1))
	case 'C':
		sc.moveX(pn(params, 0, 1))
	case 'D':
		sc.moveX(-pn(params, 0, 1))
	case 'G':
		sc.cup(sc.curY, pn(params, 0, 1)-1)
	case 'd':
		sc.cup(pn(params, 0, 1)-1, sc.curX)
	case 'J':
		sc.eraseDisplay(p0(params))
	case 'K':
		sc.eraseLine(p0(params))
	case 'L':
		sc.insertLines(pn(params, 0, 1))
	case 'M':
		sc.deleteLines(pn(params, 0, 1))
	case '@':
		sc.insertCells(pn(params, 0, 1))
	case 'P':
		sc.deleteCells(pn(params, 0, 1))
	case 'X':
		sc.eraseCells(pn(params, 0, 1))
	case 'S':
		sc.scrollUp(pn(params, 0, 1))
	case 'T':
		sc.scrollDown(pn(params, 0, 1))
	case 'r':
		top := pn(params, 0, 1)
		bot := pn(params, 1, sc.rows)
		if len(params) < 2 {
			top, bot = 1, sc.rows
		}
		if top < 1 {
			top = 1
		}
		if bot > sc.rows {
			bot = sc.rows
		}
		if top >= bot {
			sc.note("decstbm")
			return
		}
		sc.scrollTop = top - 1
		sc.scrollBot = bot - 1
		sc.cup(0, 0)
	case 'm':
		sc.sgr(params)
	default:
		sc.note("csi")
	}
}

func (sc *Screen) setMode(mode int, on bool) {
	switch mode {
	case 1:
		sc.appCursor = on
	case 7:
		sc.autowrap = on
	case 25:
		sc.cursorOn = on
	case 1049:
		if on == sc.usingAlt {
			return
		}
		if on {
			sc.savedX, sc.savedY = sc.curX, sc.curY
			sc.usingAlt = true
			clear(sc.alt)
			sc.curX, sc.curY = 0, 0
			sc.wrapNext = false
			return
		}
		sc.usingAlt = false
		sc.curX, sc.curY = sc.savedX, sc.savedY
		if sc.curX > sc.cols {
			sc.curX = sc.cols
		}
		sc.wrapNext = false
	case 2004:
		sc.bracketed = on
	default:
		sc.note("mode")
	}
}

func (sc *Screen) sgr(params [][]uint16) {
	if len(params) == 0 {
		sc.attr = Attr{}
		return
	}
	for i := 0; i < len(params); i++ {
		if len(params[i]) == 0 {
			continue
		}
		p := params[i]
		switch p[0] {
		case 0:
			sc.attr = Attr{}
		case 1:
			sc.attr.Bold = true
		case 3:
			sc.attr.Italic = true
		case 4:
			sc.attr.Underline = true
		case 7:
			sc.attr.Inverse = true
		case 22:
			sc.attr.Bold = false
		case 23:
			sc.attr.Italic = false
		case 24:
			sc.attr.Underline = false
		case 27:
			sc.attr.Inverse = false
		case 39:
			sc.attr.FG = Color{}
		case 49:
			sc.attr.BG = Color{}
		case 38, 48:
			col, used := colorParam(p, params, i)
			if p[0] == 38 {
				sc.attr.FG = col
			} else {
				sc.attr.BG = col
			}
			i += used
		default:
			n := int(p[0])
			switch {
			case n >= 30 && n <= 37:
				sc.attr.FG = Color{Kind: 1, Idx: uint8(n - 30)}
			case n >= 40 && n <= 47:
				sc.attr.BG = Color{Kind: 1, Idx: uint8(n - 40)}
			case n >= 90 && n <= 97:
				sc.attr.FG = Color{Kind: 1, Idx: uint8(n - 90 + 8)}
			case n >= 100 && n <= 107:
				sc.attr.BG = Color{Kind: 1, Idx: uint8(n - 100 + 8)}
			default:
				sc.note("sgr")
			}
		}
	}
}

// colorParam reads 38/48. used is how many extra semicolon params were consumed.
func colorParam(p []uint16, params [][]uint16, i int) (Color, int) {
	if len(p) >= 3 && p[1] == 5 {
		return Color{Kind: 1, Idx: uint8(p[2])}, 0
	}
	if len(p) >= 5 && p[1] == 2 {
		return Color{Kind: 2, R: uint8(p[2]), G: uint8(p[3]), B: uint8(p[4])}, 0
	}
	if len(params) > i+2 && len(params[i+1]) > 0 && params[i+1][0] == 5 {
		return Color{Kind: 1, Idx: uint8(params[i+2][0])}, 2
	}
	if len(params) > i+4 && len(params[i+1]) > 0 && params[i+1][0] == 2 {
		return Color{Kind: 2, R: uint8(params[i+2][0]), G: uint8(params[i+3][0]), B: uint8(params[i+4][0])}, 4
	}
	return Color{}, 0
}

func (sc *Screen) cup(y, x int) {
	sc.wrapNext = false
	if x >= sc.cols {
		x = sc.cols - 1
	}
	if y >= sc.rows {
		y = sc.rows - 1
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	sc.curX, sc.curY = x, y
}

func (sc *Screen) moveX(d int) {
	sc.wrapNext = false
	x := max(sc.curX+d, 0)
	if x >= sc.cols {
		x = sc.cols - 1
	}
	sc.curX = x
}

func (sc *Screen) moveY(d int) {
	sc.wrapNext = false
	y := max(sc.curY+d, 0)
	if y >= sc.rows {
		y = sc.rows - 1
	}
	sc.curY = y
}

func (sc *Screen) linefeed() {
	sc.wrapNext = false
	if sc.curY == sc.scrollBot {
		sc.scrollUp(1)
		return
	}
	if sc.curY < sc.rows-1 {
		sc.curY++
	}
}

func (sc *Screen) scrollUp(n int) {
	if n < 1 {
		return
	}
	top, bot := sc.scrollTop, sc.scrollBot
	height := bot - top + 1
	if n > height {
		n = height
	}
	g := sc.grid()
	if top == 0 && !sc.usingAlt {
		for i := 0; i < n; i++ {
			line := append([]Cell(nil), g[i*sc.cols:(i+1)*sc.cols]...)
			sc.hist = append(sc.hist, line)
		}
		if len(sc.hist) > sc.maxHist {
			sc.hist = append([][]Cell(nil), sc.hist[len(sc.hist)-sc.maxHist:]...)
		}
	}
	for y := top; y <= bot-n; y++ {
		copy(g[y*sc.cols:(y+1)*sc.cols], g[(y+n)*sc.cols:(y+n+1)*sc.cols])
	}
	for y := bot - n + 1; y <= bot; y++ {
		for x := 0; x < sc.cols; x++ {
			g[y*sc.cols+x] = Cell{}
		}
	}
}

func (sc *Screen) scrollDown(n int) {
	if n < 1 {
		return
	}
	top, bot := sc.scrollTop, sc.scrollBot
	height := bot - top + 1
	if n > height {
		n = height
	}
	g := sc.grid()
	for y := bot; y >= top+n; y-- {
		copy(g[y*sc.cols:(y+1)*sc.cols], g[(y-n)*sc.cols:(y-n+1)*sc.cols])
	}
	for y := top; y < top+n; y++ {
		for x := 0; x < sc.cols; x++ {
			g[y*sc.cols+x] = Cell{}
		}
	}
}

func (sc *Screen) eraseDisplay(mode int) {
	switch mode {
	case 0:
		sc.eraseLine(0)
		for y := sc.curY + 1; y < sc.rows; y++ {
			sc.blankLine(y)
		}
	case 1:
		for y := 0; y < sc.curY; y++ {
			sc.blankLine(y)
		}
		sc.eraseLine(1)
	case 2:
		clear(sc.grid())
	case 3:
		clear(sc.grid())
		sc.hist = nil
	default:
		sc.note("ed")
	}
}

func (sc *Screen) eraseLine(mode int) {
	y := sc.curY
	switch mode {
	case 0:
		for x := sc.curX; x < sc.cols; x++ {
			sc.clearAt(x, y)
		}
	case 1:
		end := sc.curX
		if end >= sc.cols {
			end = sc.cols - 1
		}
		for x := 0; x <= end; x++ {
			sc.clearAt(x, y)
		}
	case 2:
		sc.blankLine(y)
	default:
		sc.note("el")
	}
}

func (sc *Screen) blankLine(y int) {
	if y < 0 || y >= sc.rows {
		return
	}
	g := sc.grid()
	for x := 0; x < sc.cols; x++ {
		g[y*sc.cols+x] = Cell{}
	}
}

func (sc *Screen) insertLines(n int) {
	if n < 1 || sc.curY < sc.scrollTop || sc.curY > sc.scrollBot {
		return
	}
	room := sc.scrollBot - sc.curY + 1
	if n > room {
		n = room
	}
	g := sc.grid()
	for y := sc.scrollBot; y >= sc.curY+n; y-- {
		copy(g[y*sc.cols:(y+1)*sc.cols], g[(y-n)*sc.cols:(y-n+1)*sc.cols])
	}
	for y := sc.curY; y < sc.curY+n; y++ {
		sc.blankLine(y)
	}
}

func (sc *Screen) deleteLines(n int) {
	if n < 1 || sc.curY < sc.scrollTop || sc.curY > sc.scrollBot {
		return
	}
	room := sc.scrollBot - sc.curY + 1
	if n > room {
		n = room
	}
	g := sc.grid()
	for y := sc.curY; y <= sc.scrollBot-n; y++ {
		copy(g[y*sc.cols:(y+1)*sc.cols], g[(y+n)*sc.cols:(y+n+1)*sc.cols])
	}
	for y := sc.scrollBot - n + 1; y <= sc.scrollBot; y++ {
		sc.blankLine(y)
	}
}

func (sc *Screen) insertCells(n int) {
	if n < 1 {
		return
	}
	y := sc.curY
	x := sc.curX
	if x >= sc.cols {
		x = sc.cols - 1
	}
	g := sc.grid()
	for i := sc.cols - 1; i >= x+n && i >= 0; i-- {
		g[y*sc.cols+i] = g[y*sc.cols+i-n]
	}
	for i := 0; i < n && x+i < sc.cols; i++ {
		g[y*sc.cols+x+i] = Cell{}
	}
}

func (sc *Screen) deleteCells(n int) {
	if n < 1 {
		return
	}
	y := sc.curY
	x := sc.curX
	if x >= sc.cols {
		return
	}
	g := sc.grid()
	for i := x; i+n < sc.cols; i++ {
		g[y*sc.cols+i] = g[y*sc.cols+i+n]
	}
	for i := sc.cols - n; i < sc.cols; i++ {
		if i >= 0 {
			g[y*sc.cols+i] = Cell{}
		}
	}
}

func (sc *Screen) eraseCells(n int) {
	if n < 1 {
		return
	}
	for i := 0; i < n && sc.curX+i < sc.cols; i++ {
		sc.clearAt(sc.curX+i, sc.curY)
	}
}

func (sc *Screen) put(x, y int, r rune, w int) {
	if y < 0 || y >= sc.rows || x < 0 || x >= sc.cols {
		return
	}
	sc.clearAt(x, y)
	if w == 2 && x+1 < sc.cols {
		sc.clearAt(x+1, y)
	}
	g := sc.grid()
	g[y*sc.cols+x] = Cell{R: r, A: sc.attr, W: int8(w)}
	if w == 2 && x+1 < sc.cols {
		g[y*sc.cols+x+1] = Cell{A: sc.attr, W: -1}
	}
}

func (sc *Screen) clearAt(x, y int) {
	if x < 0 || y < 0 || x >= sc.cols || y >= sc.rows {
		return
	}
	g := sc.grid()
	i := y*sc.cols + x
	c := g[i]
	if c.W == 2 && x+1 < sc.cols {
		g[i+1] = Cell{}
	}
	if c.W == -1 && x > 0 {
		g[i-1] = Cell{}
	}
	g[i] = Cell{}
}

func pn(params [][]uint16, i, def int) int {
	if i >= len(params) || len(params[i]) == 0 || params[i][0] == 0 {
		return def
	}
	return int(params[i][0])
}

func p0(params [][]uint16) int {
	if len(params) == 0 || len(params[0]) == 0 {
		return 0
	}
	return int(params[0][0])
}

func runeWidth(r rune) int {
	if r == 0 || r < 0x20 {
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	return 1
}
