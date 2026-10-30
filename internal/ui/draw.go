// Package ui holds the shared Gio widgets for gio-ide.
package ui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
)

// Font is the proportional face used for chrome.
var Font = font.Font{Typeface: "Go"}

// MonoFont is the default editor face.
var MonoFont = font.Font{Typeface: "Go Mono"}

// Fill paints the maximum constraint rectangle.
func Fill(gtx layout.Context, col color.NRGBA) layout.Dimensions {
	size := gtx.Constraints.Max
	if size.X < 0 {
		size.X = 0
	}
	if size.Y < 0 {
		size.Y = 0
	}
	paint.FillShape(gtx.Ops, col, clip.Rect{Max: size}.Op())
	return layout.Dimensions{Size: size}
}

// FillSize paints a rectangle of the given size.
func FillSize(ops *op.Ops, col color.NRGBA, size image.Point) {
	paint.FillShape(ops, col, clip.Rect{Max: size}.Op())
}

// Text draws a single line. An empty string draws nothing.
func Text(gtx layout.Context, sh *text.Shaper, face font.Font, size unit.Sp, col color.NRGBA, s string) layout.Dimensions {
	if s == "" || sh == nil {
		return layout.Dimensions{}
	}
	macro := op.Record(gtx.Ops)
	paint.ColorOp{Color: col}.Add(gtx.Ops)
	mat := macro.Stop()
	return widget.Label{MaxLines: 1, Truncator: "…"}.Layout(gtx, sh, face, size, s, mat)
}

// Inset is a small padding helper.
func Inset(gtx layout.Context, top, right, bottom, left unit.Dp, w layout.Widget) layout.Dimensions {
	return layout.Inset{Top: top, Right: right, Bottom: bottom, Left: left}.Layout(gtx, w)
}
