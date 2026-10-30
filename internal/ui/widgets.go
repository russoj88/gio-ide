package ui

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
	"gioui.org/text"
	"gioui.org/unit"
)

// Click reports whether tag was pressed, then lays out w on bg.
// The hit area is w's size. tag must not be a zero-size pointer: every
// zero-size allocation has the same address, so every widget would share
// one handler.
func Click(gtx layout.Context, tag event.Tag, bg color.NRGBA, w layout.Widget) (layout.Dimensions, bool) {
	clicked := pressed(gtx, tag)
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	size := dims.Size
	if size.X < 1 {
		size.X = 1
	}
	if size.Y < 1 {
		size.Y = 1
	}
	stack := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	FillSize(gtx.Ops, bg, size)
	call.Add(gtx.Ops)
	stack.Pop()
	return dims, clicked
}

func pressed(gtx layout.Context, tag event.Tag) bool {
	clicked := false
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Kind == pointer.Press {
			clicked = true
		}
	}
	return clicked
}

// StatusItem is one status-bar cell.
type StatusItem struct {
	Text  string
	Color color.NRGBA
}

// StatusBar draws a single row of items separated by gaps.
func StatusBar(gtx layout.Context, sh *text.Shaper, size unit.Sp, bg, fg color.NRGBA, items []StatusItem) layout.Dimensions {
	h := gtx.Dp(unit.Dp(26))
	gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, h)
	gtx.Constraints.Max.Y = h
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return Fill(gtx, bg) },
		func(gtx layout.Context) layout.Dimensions {
			var kids []layout.FlexChild
			for _, it := range items {
				col := it.Color
				if col.A == 0 {
					col = fg
				}
				kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return Inset(gtx, unit.Dp(4), unit.Dp(10), unit.Dp(4), unit.Dp(10), func(gtx layout.Context) layout.Dimensions {
						return Text(gtx, sh, Font, size, col, it.Text)
					})
				}))
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, kids...)
		},
	)
}

// Split is a horizontal or vertical weighted split. Ratio is the fraction
// given to the first child (0..1).
func Split(gtx layout.Context, vertical bool, ratio float32, gap int, a, b layout.Widget) layout.Dimensions {
	if ratio < 0.05 {
		ratio = 0.05
	}
	if ratio > 0.95 {
		ratio = 0.95
	}
	if vertical {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Flexed(ratio, a),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gap
				gtx.Constraints.Max.Y = gap
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}),
			layout.Flexed(1-ratio, b),
		)
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(ratio, a),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gap
			gtx.Constraints.Max.X = gap
			return layout.Dimensions{Size: image.Pt(gap, gtx.Constraints.Max.Y)}
		}),
		layout.Flexed(1-ratio, b),
	)
}

// TreeNode is one row in the project tree.
type TreeNode struct {
	Label string
	Depth int
	Dir   bool
	Open  bool
	// Selected highlights the open file with the hover color passed to Tree.
	Selected bool
	Tag      event.Tag
	Path     string
}

// Tree lays out visible tree rows. It returns the path of a clicked row.
// Each row draws a folder or file mark from the node's label.
// scroll is the first visible row and the wheel target. It may be nil.
func Tree(gtx layout.Context, sh *text.Shaper, size unit.Sp, nodes []TreeNode, fg, dirCol, hover color.NRGBA, icons IconColors, scroll *int) (layout.Dimensions, string) {
	rowH := max(gtx.Dp(unit.Dp(22)), 1)
	clicked := ""
	maxRows := max(gtx.Constraints.Max.Y/rowH, 1)
	maxOff := max(len(nodes)-maxRows, 0)
	if scroll != nil {
		treeWheel(gtx, scroll)
		if *scroll < 0 {
			*scroll = 0
		}
		if *scroll > maxOff {
			*scroll = maxOff
		}
	}
	off := 0
	if scroll != nil {
		off = *scroll
	}
	var kids []layout.FlexChild
	end := min(off+maxRows, len(nodes))
	for i := off; i < end; i++ {
		n := nodes[i]
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = rowH
			gtx.Constraints.Max.Y = rowH
			width := max(gtx.Constraints.Max.X, 1)
			if pressed(gtx, n.Tag) {
				clicked = n.Path
			}
			// The row, not the glyphs, is the hit target. Pass wheel events
			// through to the tree so the row does not swallow them.
			stack := clip.Rect{Max: image.Pt(width, rowH)}.Push(gtx.Ops)
			pass := pointer.PassOp{}.Push(gtx.Ops)
			event.Op(gtx.Ops, n.Tag)
			pass.Pop()
			col := fg
			if n.Dir {
				col = dirCol
			}
			if n.Selected {
				Fill(gtx, hover)
			}
			_ = Inset(gtx, 0, unit.Dp(8), 0, unit.Dp(8+n.Depth*14), func(gtx layout.Context) layout.Dimensions {
				mark := gtx.Dp(unit.Dp(14))
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min = image.Pt(mark, rowH)
						gtx.Constraints.Max = image.Pt(mark, rowH)
						if !n.Dir {
							return layout.Dimensions{Size: gtx.Constraints.Max}
						}
						return Disclosure(gtx, col, n.Open)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						slot := gtx.Dp(unit.Dp(16))
						gtx.Constraints.Min = image.Pt(slot, rowH)
						gtx.Constraints.Max = image.Pt(slot, rowH)
						kind := iconKind(n.Label, n.Dir)
						return drawTreeIcon(gtx, kind, icons.pick(kind, col))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return Text(gtx, sh, Font, size, col, n.Label)
					}),
				)
			})
			stack.Pop()
			return layout.Dimensions{Size: image.Pt(width, rowH)}
		}))
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	dims.Size = gtx.Constraints.Max
	if dims.Size.Y > gtx.Constraints.Max.Y {
		dims.Size.Y = gtx.Constraints.Max.Y
	}
	return dims, clicked
}

// treeWheel scrolls by whole rows. The tag is the scroll pointer, which stays
// put for the life of the sidebar.
func treeWheel(gtx layout.Context, scroll *int) {
	size := gtx.Constraints.Max
	if size.X < 1 {
		size.X = 1
	}
	if size.Y < 1 {
		size.Y = 1
	}
	stack := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, scroll)
	stack.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  scroll,
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
		step := 3
		if e.Scroll.Y < 0 {
			*scroll -= step
		} else if e.Scroll.Y > 0 {
			*scroll += step
		}
	}
}

// Disclosure draws an open or closed triangle. The Go font has no ▸ or ▾,
// so a text prefix would show as .notdef. open draws the triangle pointing
// down. A closed range draws it pointing right.
func Disclosure(gtx layout.Context, col color.NRGBA, open bool) layout.Dimensions {
	size := gtx.Constraints.Max
	if size.X < 1 {
		size.X = 1
	}
	if size.Y < 1 {
		size.Y = 1
	}
	side := max(min(min(gtx.Dp(unit.Dp(9)), size.X), size.Y), 1)
	ox := (size.X - side) / 2
	oy := (size.Y - side) / 2
	stack := op.Offset(image.Pt(ox, oy)).Push(gtx.Ops)
	var p clip.Path
	p.Begin(gtx.Ops)
	s := float32(side)
	if open {
		p.MoveTo(f32.Pt(0, s*0.25))
		p.LineTo(f32.Pt(s, s*0.25))
		p.LineTo(f32.Pt(s*0.5, s*0.85))
	} else {
		p.MoveTo(f32.Pt(s*0.25, 0))
		p.LineTo(f32.Pt(s*0.85, s*0.5))
		p.LineTo(f32.Pt(s*0.25, s))
	}
	p.Close()
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: p.End()}.Op())
	stack.Pop()
	return layout.Dimensions{Size: size}
}

// List lays out plain rows and returns the clicked index, or -1.
// scroll is the first visible row and the wheel target. Nil starts at 0
// and ignores the wheel. The highlight uses the absolute row index.
// tag, when non-nil, is called with the absolute row index and that tag
// receives presses. The hit rectangle is the full row width, not the label.
// A nil tag never reports a click.
func List(gtx layout.Context, sh *text.Shaper, size unit.Sp, rows []string, active int, fg, sel, bg color.NRGBA, scroll *int, tag func(int) event.Tag) (layout.Dimensions, int) {
	rowH := max(gtx.Dp(unit.Dp(24)), 1)
	maxRows := max(gtx.Constraints.Max.Y/rowH, 1)
	maxOff := max(len(rows)-maxRows, 0)
	if scroll != nil {
		treeWheel(gtx, scroll)
		if *scroll < 0 {
			*scroll = 0
		}
		if *scroll > maxOff {
			*scroll = maxOff
		}
	}
	off := 0
	if scroll != nil {
		off = *scroll
	}
	picked := -1
	var kids []layout.FlexChild
	end := min(off+maxRows, len(rows))
	for i := off; i < end; i++ {
		label := rows[i]
		on := i == active
		idx := i
		var rowTag event.Tag
		if tag != nil {
			rowTag = tag(idx)
		}
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = rowH
			gtx.Constraints.Max.Y = rowH
			rowBG := bg
			if on {
				rowBG = sel
			}
			if tag == nil {
				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions { return Fill(gtx, rowBG) },
					func(gtx layout.Context) layout.Dimensions {
						return Inset(gtx, unit.Dp(2), unit.Dp(8), unit.Dp(2), unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
							return Text(gtx, sh, Font, size, fg, label)
						})
					},
				)
			}
			// Min.X spans the row so Click's clip is not the label width.
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			dims, clicked := Click(gtx, rowTag, rowBG, func(gtx layout.Context) layout.Dimensions {
				dims := Inset(gtx, unit.Dp(2), unit.Dp(8), unit.Dp(2), unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
					return Text(gtx, sh, Font, size, fg, label)
				})
				dims.Size = gtx.Constraints.Max
				return dims
			})
			if clicked && idx > picked {
				picked = idx
			}
			return dims
		}))
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	return dims, picked
}

// HBar is a 1px horizontal rule.
func HBar(gtx layout.Context, col color.NRGBA) layout.Dimensions {
	h := max(gtx.Dp(unit.Dp(1)), 1)
	gtx.Constraints.Min.Y = h
	gtx.Constraints.Max.Y = h
	return Fill(gtx, col)
}

// VBar is a 1px vertical rule.
func VBar(gtx layout.Context, col color.NRGBA) layout.Dimensions {
	w := max(gtx.Dp(unit.Dp(1)), 1)
	size := image.Pt(w, gtx.Constraints.Max.Y)
	FillSize(gtx.Ops, col, size)
	return layout.Dimensions{Size: size}
}

// Popup draws a centered card. The child lays out inside the card.
func Popup(gtx layout.Context, bg, border color.NRGBA, width, height unit.Dp, title string, sh *text.Shaper, fg color.NRGBA, body layout.Widget) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			// Dim the frame.
			return Fill(gtx, color.NRGBA{A: 120})
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				w := gtx.Dp(width)
				h := gtx.Dp(height)
				gtx.Constraints.Min = image.Pt(w, h)
				gtx.Constraints.Max = image.Pt(w, h)
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						dims := Fill(gtx, bg)
						defer clip.Stroke{
							Path:  clip.Rect{Max: dims.Size}.Path(),
							Width: 1,
						}.Op().Push(gtx.Ops).Pop()
						paintColor(gtx, border)
						return dims
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return Inset(gtx, unit.Dp(12), unit.Dp(14), unit.Dp(8), unit.Dp(14), func(gtx layout.Context) layout.Dimensions {
									return Text(gtx, sh, Font, unit.Sp(15), fg, title)
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return Inset(gtx, 0, unit.Dp(14), unit.Dp(12), unit.Dp(14), body)
							}),
						)
					}),
				)
			})
		},
	)
}

func paintColor(gtx layout.Context, col color.NRGBA) {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	FillSize(gtx.Ops, col, gtx.Constraints.Max)
}
