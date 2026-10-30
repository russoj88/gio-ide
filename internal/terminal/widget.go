package terminal

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// Style is the theme slice the grid paints with.
type Style struct {
	Background color.NRGBA
	Foreground color.NRGBA
	Cursor     color.NRGBA
	Selection  color.NRGBA
	FontSize   unit.Sp
}

// Grid paints one terminal and reports the cell size back through Resize.
type Grid struct {
	Term   *Term
	Style  Style
	Resize func(cols, rows int)

	tag      struct{}
	px       int
	fontOK   bool
	charW    int
	linePx   int
	cols     int
	rows     int
	shown    int
	anchorX  int
	anchorY  int
	dragging bool
	pics     []rowPic
}

// rowPic is one rasterized screen row. The image is immutable while live is set.
type rowPic struct {
	sum    uint64
	px, w  int
	h      int
	cursor int
	img    *image.RGBA
	op     paint.ImageOp
	live   bool
	ink    bool
}

// Layout draws the visible screen. Resize is invoked when the grid size changes.
func (g *Grid) Layout(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	if size.X < 1 {
		size.X = 1
	}
	if size.Y < 1 {
		size.Y = 1
	}
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, g.Style.Background, clip.Rect{Max: size}.Op())
	if g.Term == nil {
		return layout.Dimensions{Size: size}
	}
	px := textPx(gtx, g.Style.FontSize)
	cellW, linePx, ok := metricsOf(px)
	if !ok {
		cellW = px / 2
		if cellW < 1 {
			cellW = 8
		}
		linePx = max(px+px/3, 1)
	}
	g.px = px
	g.fontOK = ok
	g.charW = cellW
	g.linePx = linePx
	cols := size.X / g.charW
	rows := size.Y / g.linePx
	if cols < 2 {
		cols = 2
	}
	if rows < 1 {
		rows = 1
	}
	if cols != g.cols || rows != g.rows {
		g.cols, g.rows = cols, rows
		if g.Resize != nil {
			g.Resize(cols, rows)
		}
	}
	g.shown = rows
	g.pointer(gtx)
	g.paint(gtx, g.Term.Snapshot())
	return layout.Dimensions{Size: size}
}

func textPx(gtx layout.Context, fs unit.Sp) int {
	if fs <= 0 {
		fs = 13
	}
	px := gtx.Sp(fs)
	if px < 1 {
		return 1
	}
	if px > 256 {
		return 256
	}
	return px
}

func (g *Grid) pointer(gtx layout.Context) {
	event.Op(gtx.Ops, &g.tag)
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  &g.tag,
			Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll,
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
			if e.Scroll.Y < 0 {
				g.Term.Scroll(3)
			} else if e.Scroll.Y > 0 {
				g.Term.Scroll(-3)
			}
		case pointer.Press:
			if !e.Buttons.Contain(pointer.ButtonPrimary) {
				continue
			}
			// A click clears any highlight. Selection starts only once the
			// drag reaches a different cell, so a press is not a one-cell mark.
			x, y := g.cellAt(e.Position)
			g.anchorX, g.anchorY = x, y
			g.dragging = true
			g.Term.ClearSelection()
			gtx.Execute(pointer.GrabCmd{Tag: &g.tag, ID: e.PointerID})
		case pointer.Drag:
			if !g.dragging {
				continue
			}
			x, y := g.cellAt(e.Position)
			if x == g.anchorX && y == g.anchorY {
				g.Term.ClearSelection()
				continue
			}
			g.Term.SetSelection(g.anchorX, g.anchorY, x, y)
		case pointer.Release, pointer.Cancel:
			g.dragging = false
		}
	}
}

// Rows is the last laid-out row count, used for page scrolling.
func (g *Grid) Rows() int {
	if g.shown > 0 {
		return g.shown
	}
	return g.rows
}

func (g *Grid) cellAt(p f32.Point) (int, int) {
	cw, lh := g.charW, g.linePx
	if cw < 1 {
		cw = 1
	}
	if lh < 1 {
		lh = 1
	}
	x := int(p.X) / cw
	y := int(p.Y) / lh
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if g.cols > 0 && x >= g.cols {
		x = g.cols - 1
	}
	if g.rows > 0 && y >= g.rows {
		y = g.rows - 1
	}
	return x, y
}

func (g *Grid) paint(gtx layout.Context, snap ScreenSnapshot) {
	cols := snap.Cols
	rows := snap.Rows
	if cols < 1 || rows < 1 || len(snap.Cells) < cols*rows {
		return
	}
	show := min(rows, g.rows)
	if x0, y0, x1, y1, ok := g.Term.Selection(); ok {
		for y := y0; y <= y1 && y < show; y++ {
			if y < 0 {
				continue
			}
			from, to := 0, cols
			if y == y0 {
				from = x0
			}
			if y == y1 {
				to = x1 + 1
			}
			if from < 0 {
				from = 0
			}
			if to > g.cols {
				to = g.cols
			}
			paint.FillShape(gtx.Ops, g.Style.Selection, clip.Rect{
				Min: image.Pt(from*g.charW, y*g.linePx),
				Max: image.Pt(to*g.charW, (y+1)*g.linePx),
			}.Op())
		}
	}
	cursorX, cursorY := -1, -1
	if snap.CursorOn && snap.CursorY >= 0 && snap.CursorY < show && snap.CursorX >= 0 && snap.CursorX < g.cols {
		cursorX, cursorY = snap.CursorX, snap.CursorY
		cw := g.charW
		if cursorX < cols && snap.Cells[cursorY*cols+cursorX].W == 2 {
			cw *= 2
		}
		paint.FillShape(gtx.Ops, g.Style.Cursor, clip.Rect{
			Min: image.Pt(cursorX*g.charW, cursorY*g.linePx),
			Max: image.Pt(cursorX*g.charW+cw, (cursorY+1)*g.linePx),
		}.Op())
	}
	for y := range show {
		x := 0
		for x < cols && x < g.cols {
			c := snap.Cells[y*cols+x]
			if c.W < 0 {
				x++
				continue
			}
			w := 1
			if c.W == 2 {
				w = 2
			}
			_, bg := styleColors(g.Style, c.A)
			if bg != g.Style.Background && !(y == cursorY && x == cursorX) {
				paint.FillShape(gtx.Ops, bg, clip.Rect{
					Min: image.Pt(x*g.charW, y*g.linePx),
					Max: image.Pt((x+w)*g.charW, (y+1)*g.linePx),
				}.Op())
			}
			x += w
		}
	}
	if g.fontOK {
		g.paintGlyphs(gtx, snap, show, cols, cursorX, cursorY)
	}
	for y := range show {
		x := 0
		for x < cols && x < g.cols {
			c := snap.Cells[y*cols+x]
			if c.W < 0 {
				x++
				continue
			}
			w := 1
			if c.W == 2 {
				w = 2
			}
			if c.A.Underline {
				fg, _ := styleColors(g.Style, c.A)
				if y == cursorY && x == cursorX {
					fg = cursorContrast(g.Style)
				}
				y1 := (y+1)*g.linePx - 1
				paint.FillShape(gtx.Ops, fg, clip.Rect{
					Min: image.Pt(x*g.charW, y1),
					Max: image.Pt((x+w)*g.charW, y1+1),
				}.Op())
			}
			x += w
		}
	}
}

func cursorContrast(style Style) color.NRGBA {
	if style.Background != style.Cursor {
		return style.Background
	}
	return style.Foreground
}

// paintGlyphs blits one cached image per row. The clip is the image's pixel
// size, so the GPU does not scale the glyphs.
func (g *Grid) paintGlyphs(gtx layout.Context, snap ScreenSnapshot, show, cols, cursorX, cursorY int) {
	if len(g.pics) < show {
		g.pics = append(g.pics, make([]rowPic, show-len(g.pics))...)
	}
	contrast := cursorContrast(g.Style)
	width := g.cols * g.charW
	for y := range show {
		n := min(cols, g.cols)
		row := snap.Cells[y*cols : y*cols+n]
		cur := -1
		if y == cursorY {
			cur = cursorX
		}
		sum := rowSum(row, g.Style)
		pic := &g.pics[y]
		if !pic.live || pic.sum != sum || pic.px != g.px || pic.w != width || pic.h != g.linePx || pic.cursor != cur {
			if pic.img == nil || pic.w != width || pic.h != g.linePx {
				pic.img = image.NewRGBA(image.Rect(0, 0, width, g.linePx))
			} else {
				clear(pic.img.Pix)
			}
			fillRow(g.px, pic.img, row, n, g.Style, cur, contrast)
			pic.op = paint.NewImageOp(pic.img)
			pic.op.Filter = paint.FilterNearest
			pic.sum = sum
			pic.px = g.px
			pic.w = width
			pic.h = g.linePx
			pic.cursor = cur
			pic.live = true
			pic.ink = rowHasInk(row)
		}
		if !pic.ink {
			continue
		}
		stack := op.Offset(image.Pt(0, y*g.linePx)).Push(gtx.Ops)
		cl := clip.Rect{Max: image.Pt(pic.w, pic.h)}.Push(gtx.Ops)
		pic.op.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		cl.Pop()
		stack.Pop()
	}
}

func rowHasInk(cells []Cell) bool {
	for _, c := range cells {
		if c.W >= 0 && c.R != 0 {
			return true
		}
	}
	return false
}

func rowSum(cells []Cell, style Style) uint64 {
	h := uint64(14695981039346656037)
	h = mix(h, colorKey(style.Foreground))
	h = mix(h, colorKey(style.Background))
	for _, c := range cells {
		h = mix(h, uint64(c.R))
		h = mix(h, uint64(uint8(c.W)))
		h = mix(h, uint64(c.A.FG.Kind)<<24|uint64(c.A.FG.Idx)<<16|uint64(c.A.FG.R)<<8|uint64(c.A.FG.G))
		h = mix(h, uint64(c.A.FG.B)<<24|uint64(c.A.BG.Kind)<<16|uint64(c.A.BG.Idx)<<8|uint64(c.A.BG.R))
		h = mix(h, uint64(c.A.BG.G)<<8|uint64(c.A.BG.B))
		flags := uint64(0)
		if c.A.Bold {
			flags |= 1
		}
		if c.A.Italic {
			flags |= 2
		}
		if c.A.Inverse {
			flags |= 4
		}
		h = mix(h, flags)
	}
	return h
}

func mix(h, v uint64) uint64 {
	h ^= v
	h *= 1099511628211
	return h
}

func colorKey(c color.NRGBA) uint64 {
	return uint64(c.R)<<24 | uint64(c.G)<<16 | uint64(c.B)<<8 | uint64(c.A)
}

func styleColors(style Style, a Attr) (fg, bg color.NRGBA) {
	fg = resolve(a.FG, style.Foreground)
	bg = resolve(a.BG, style.Background)
	if a.Inverse {
		fg, bg = bg, fg
	}
	return fg, bg
}

func resolve(c Color, def color.NRGBA) color.NRGBA {
	switch c.Kind {
	case 1:
		return palette(c.Idx)
	case 2:
		return color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}
	default:
		return def
	}
}

func palette(idx uint8) color.NRGBA {
	if idx < 16 {
		return ansi16[idx]
	}
	if idx >= 232 {
		v := uint8(8 + (int(idx)-232)*10)
		return color.NRGBA{R: v, G: v, B: v, A: 255}
	}
	n := int(idx) - 16
	r := n / 36
	g := (n / 6) % 6
	b := n % 6
	level := func(i int) uint8 {
		if i == 0 {
			return 0
		}
		return uint8(55 + i*40)
	}
	return color.NRGBA{R: level(r), G: level(g), B: level(b), A: 255}
}

var ansi16 = [16]color.NRGBA{
	{0, 0, 0, 255},
	{205, 0, 0, 255},
	{0, 205, 0, 255},
	{205, 205, 0, 255},
	{0, 0, 238, 255},
	{205, 0, 205, 255},
	{0, 205, 205, 255},
	{229, 229, 229, 255},
	{127, 127, 127, 255},
	{255, 0, 0, 255},
	{0, 255, 0, 255},
	{255, 255, 0, 255},
	{92, 92, 255, 255},
	{255, 0, 255, 255},
	{0, 255, 255, 255},
	{255, 255, 255, 255},
}
