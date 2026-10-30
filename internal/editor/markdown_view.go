package editor

import (
	"image"
	"image/color"

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
)

// LayoutMarkdown paints a read-only markdown preview of src.
// scrollY is the first visible pixel. The returned scroll is clamped to the
// content after wheel input. tag receives the wheel; it must not be shared
// with another handler. accent, dim, and panel are chrome colors the editor
// palette does not carry. There is no caret and no editor pointer handler.
func LayoutMarkdown(gtx layout.Context, shaper *text.Shaper, st Style, src []byte, scrollY int, accent, dim, panel color.NRGBA, tag event.Tag) (layout.Dimensions, int) {
	if st.FontSize <= 0 {
		st.FontSize = 14
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
		scrollY += markdownWheel(gtx, tag)
	}
	paint.FillShape(gtx.Ops, st.Colors.Background, clip.Rect{Max: size}.Op())

	pad := gtx.Dp(unit.Dp(28))
	if pad*2 >= size.X {
		pad = 8
	}
	innerW := max(size.X-pad*2, 1)
	macro := op.Record(gtx.Ops)
	cgtx := gtx
	cgtx.Constraints = layout.Constraints{Max: image.Pt(innerW, 1_000_000)}
	content := layoutMarkdownBlocks(cgtx, shaper, st, src, accent, dim, panel)
	call := macro.Stop()

	maxScroll := max(content.Size.Y+pad*2-size.Y, 0)
	if scrollY < 0 {
		scrollY = 0
	}
	if scrollY > maxScroll {
		scrollY = maxScroll
	}
	off := op.Offset(image.Pt(pad, pad-scrollY)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
	return layout.Dimensions{Size: size}, scrollY
}

func markdownWheel(gtx layout.Context, tag event.Tag) int {
	dy := 0
	const step = 48
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  tag,
			Kinds:   pointer.Scroll,
			ScrollY: pointer.ScrollRange{Min: -1e6, Max: 1e6},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok || e.Kind != pointer.Scroll {
			continue
		}
		if e.Scroll.Y < 0 {
			dy -= step
		} else if e.Scroll.Y > 0 {
			dy += step
		}
	}
	return dy
}

func layoutMarkdownBlocks(gtx layout.Context, sh *text.Shaper, st Style, src []byte, accent, dim, panel color.NRGBA) layout.Dimensions {
	blocks := ParseMarkdown(src)
	if len(blocks) == 0 {
		return layout.Dimensions{}
	}
	var kids []layout.FlexChild
	gap := unit.Dp(8)
	for _, b := range blocks {
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var dims layout.Dimensions
			switch b.Kind {
			case BlockBlank:
				h := gtx.Dp(unit.Dp(8))
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, h)}
			case BlockRule:
				dims = layoutRule(gtx, dim)
			case BlockCode:
				dims = layoutCode(gtx, sh, b, st.FontSize, st.Colors.Foreground, dim, panel)
			case BlockHeading:
				ps := inlinePieces(b.Inlines, headingSize(st.FontSize, b.Level), st.Colors.Foreground, accent, true)
				dims = layoutFlow(gtx, sh, ps, panel)
			case BlockList:
				ps := inlinePieces(b.Inlines, st.FontSize, st.Colors.Foreground, accent, false)
				mark := piece{
					text: markdownPlain(b.Marker+" ", 4),
					font: font.Font{Typeface: "Go"},
					size: st.FontSize,
					col:  dim,
				}
				ps = append([]piece{mark}, ps...)
				dims = layoutFlow(gtx, sh, ps, panel)
			default:
				ps := inlinePieces(b.Inlines, st.FontSize, st.Colors.Foreground, accent, false)
				dims = layoutFlow(gtx, sh, ps, panel)
			}
			bottom := gtx.Dp(gap)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, dims.Size.Y+bottom)}
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}

func headingSize(base unit.Sp, level int) unit.Sp {
	extra := []unit.Sp{8, 6, 4, 2, 1, 0}
	if level < 1 {
		level = 1
	}
	if level > 6 {
		level = 6
	}
	return base + extra[level-1]
}

type piece struct {
	text string
	font font.Font
	size unit.Sp
	col  color.NRGBA
	code bool
}

func inlinePieces(ins []Inline, size unit.Sp, fg, accent color.NRGBA, heading bool) []piece {
	var out []piece
	for _, in := range ins {
		text := markdownPlain(in.Text, 4)
		if text == "" && in.Kind != InlineCode {
			continue
		}
		p := piece{
			text: text,
			font: font.Font{Typeface: "Go"},
			size: size,
			col:  fg,
		}
		switch in.Kind {
		case InlineCode:
			p.font = font.Font{Typeface: "Go Mono"}
			p.code = true
		case InlineEm:
			p.font.Style = font.Italic
		case InlineStrong:
			p.font.Weight = font.Bold
		case InlineLink:
			p.col = accent
		}
		if heading && p.font.Typeface == "Go" {
			p.font.Weight = font.Bold
		}
		out = append(out, p)
	}
	return out
}

func layoutFlow(gtx layout.Context, sh *text.Shaper, parts []piece, panel color.NRGBA) layout.Dimensions {
	maxW := max(gtx.Constraints.Max.X, 1)
	if sh == nil || len(parts) == 0 {
		return layout.Dimensions{Size: image.Pt(maxW, 0)}
	}
	x, y, lineH := 0, 0, 0
	for _, p := range parts {
		for _, w := range wordsOf(p) {
			macro := op.Record(gtx.Ops)
			matRec := op.Record(gtx.Ops)
			paint.ColorOp{Color: w.col}.Add(gtx.Ops)
			mat := matRec.Stop()
			ig := gtx
			ig.Constraints = layout.Constraints{Max: image.Pt(1_000_000, 1_000_000)}
			dims := widget.Label{}.Layout(ig, sh, w.font, w.size, w.text, mat)
			call := macro.Stop()
			width := dims.Size.X
			if x > 0 && x+width > maxW {
				x = 0
				y += lineH
				lineH = 0
			}
			stack := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			if w.code && dims.Size.X > 0 && dims.Size.Y > 0 {
				paint.FillShape(gtx.Ops, panel, clip.Rect{Max: dims.Size}.Op())
			}
			call.Add(gtx.Ops)
			stack.Pop()
			x += width
			if dims.Size.Y > lineH {
				lineH = dims.Size.Y
			}
		}
	}
	return layout.Dimensions{Size: image.Pt(maxW, y+lineH)}
}

func wordsOf(p piece) []piece {
	if p.text == "" {
		return nil
	}
	if p.code {
		return []piece{p}
	}
	var out []piece
	s := p.text
	for len(s) > 0 {
		i := 0
		if s[0] == ' ' {
			for i < len(s) && s[i] == ' ' {
				i++
			}
		} else {
			for i < len(s) && s[i] != ' ' {
				i++
			}
			for i < len(s) && s[i] == ' ' {
				i++
			}
		}
		n := p
		n.text = s[:i]
		out = append(out, n)
		s = s[i:]
	}
	return out
}

func layoutCode(gtx layout.Context, sh *text.Shaper, b Block, size unit.Sp, fg, dim, panel color.NRGBA) layout.Dimensions {
	w := max(gtx.Constraints.Max.X, 1)
	linePx := max(gtx.Sp(size)+gtx.Dp(unit.Dp(4)), 1)
	pad := gtx.Dp(unit.Dp(8))
	langH := 0
	if b.Lang != "" && sh != nil {
		langH = linePx
	}
	n := max(len(b.Lines), 1)
	h := pad*2 + langH + n*linePx
	paint.FillShape(gtx.Ops, panel, clip.Rect{Max: image.Pt(w, h)}.Op())
	y := pad
	if b.Lang != "" && sh != nil {
		paintLabel(gtx, sh, font.Font{Typeface: "Go"}, size, dim, markdownPlain(b.Lang, 4), image.Pt(pad, y))
		y += langH
	}
	face := font.Font{Typeface: "Go Mono"}
	if sh != nil {
		for _, ln := range b.Lines {
			if ln != "" {
				paintLabel(gtx, sh, face, size, fg, ln, image.Pt(pad, y))
			}
			y += linePx
		}
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}

func paintLabel(gtx layout.Context, sh *text.Shaper, face font.Font, size unit.Sp, col color.NRGBA, s string, at image.Point) {
	if s == "" || sh == nil {
		return
	}
	macro := op.Record(gtx.Ops)
	paint.ColorOp{Color: col}.Add(gtx.Ops)
	mat := macro.Stop()
	stack := op.Offset(at).Push(gtx.Ops)
	ig := gtx
	ig.Constraints = layout.Constraints{Max: image.Pt(max(gtx.Constraints.Max.X-at.X, 1), 10_000)}
	widget.Label{MaxLines: 1}.Layout(ig, sh, face, size, s, mat)
	stack.Pop()
}

func layoutRule(gtx layout.Context, col color.NRGBA) layout.Dimensions {
	w := max(gtx.Constraints.Max.X, 1)
	top := gtx.Dp(unit.Dp(8))
	h := max(gtx.Dp(unit.Dp(1)), 1)
	paint.FillShape(gtx.Ops, col, clip.Rect{Min: image.Pt(0, top), Max: image.Pt(w, top+h)}.Op())
	return layout.Dimensions{Size: image.Pt(w, top+h+top)}
}
