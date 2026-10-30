package ui

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/harness"
)

func TestTreeClickHitsTheRowNotJustTheLabel(t *testing.T) {
	var (
		r      input.Router
		tags   [8]byte
		scroll int
	)
	fg := color.NRGBA{A: 255}
	nodes := make([]TreeNode, len(tags))
	for i := range nodes {
		nodes[i] = TreeNode{Label: "f", Path: "f", Tag: &tags[i]}
	}
	nodes[0].Path = "main.go"
	nodes[1].Path = "internal"
	nodes[1].Dir = true
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(240, 80)),
		Source:      r.Source(),
	}
	layoutTree := func() string {
		gtx.Ops.Reset()
		_, clicked := Tree(gtx, nil, unit.Sp(13), nodes, fg, fg, fg, IconColors{}, &scroll)
		return clicked
	}
	layoutTree()
	r.Frame(gtx.Ops)
	// Left edge of the first row. The label used to be centered, so this missed.
	r.Queue(pointer.Event{
		Source:   pointer.Mouse,
		Kind:     pointer.Press,
		Position: f32.Pt(4, 8),
	})
	if got := layoutTree(); got != "main.go" {
		t.Fatalf("click = %q", got)
	}
	// Release before the next press. A pointer that is still down keeps
	// the first row's handler.
	r.Queue(pointer.Event{
		Source:   pointer.Mouse,
		Kind:     pointer.Release,
		Position: f32.Pt(4, 8),
	})
	r.Frame(gtx.Ops)
	r.Queue(pointer.Event{
		Source:   pointer.Mouse,
		Kind:     pointer.Press,
		Position: f32.Pt(4, 30),
	})
	if got := layoutTree(); got != "internal" {
		t.Fatalf("second row click = %q", got)
	}
	r.Frame(gtx.Ops)
	r.Queue(pointer.Event{
		Source:   pointer.Mouse,
		Kind:     pointer.Scroll,
		Position: f32.Pt(20, 20),
		Scroll:   f32.Pt(0, 12),
	})
	layoutTree()
	if scroll == 0 {
		t.Fatal("wheel did not scroll the tree")
	}
}

func TestTreeSelectedRowUsesHoverColor(t *testing.T) {
	hover := color.NRGBA{R: 0x21, G: 0x42, B: 0x83, A: 0xFF}
	fg := color.NRGBA{A: 0xFF}
	var tags [2]byte
	nodes := []TreeNode{
		{Label: "main.go", Path: "main.go", Tag: &tags[0], Selected: true},
		{Label: "other.go", Path: "other.go", Tag: &tags[1]},
	}
	d, err := harness.New(image.Pt(240, 80))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		Tree(gtx, nil, unit.Sp(13), nodes, fg, fg, hover, IconColors{}, nil)
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	d.Step()
	img, err := d.Image()
	if err != nil {
		t.Skip(err)
	}
	want := color.RGBA{R: hover.R, G: hover.G, B: hover.B, A: hover.A}
	if got := img.RGBAAt(4, 8); got != want {
		t.Fatalf("selected row pixel = %+v, want %+v", got, want)
	}
	if got := img.RGBAAt(4, 30); got == want {
		t.Fatalf("unselected row pixel = %+v, should not be the selection", got)
	}
}

func TestTreeFileIconsDiffer(t *testing.T) {
	bg := color.NRGBA{R: 0x11, G: 0x12, B: 0x13, A: 0xFF}
	goCol := color.NRGBA{R: 0x56, G: 0xA8, B: 0xF5, A: 0xFF}
	htmlCol := color.NRGBA{R: 0xF7, G: 0x54, B: 0x64, A: 0xFF}
	fg := color.NRGBA{A: 0xFF}
	var tags [2]byte
	nodes := []TreeNode{
		{Label: "main.go", Path: "main.go", Tag: &tags[0]},
		{Label: "index.html", Path: "index.html", Tag: &tags[1]},
	}
	icons := IconColors{Go: goCol, HTML: htmlCol}
	d, err := harness.New(image.Pt(240, 80))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		Fill(gtx, bg)
		Tree(gtx, nil, unit.Sp(13), nodes, fg, fg, fg, icons, nil)
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	d.Step()
	img, err := d.Image()
	if err != nil {
		t.Skip(err)
	}
	// Icon slot follows 8 dp of padding and the 14 dp disclosure.
	goPx := strongestInk(img, image.Rect(22, 0, 38, 22), bg)
	htmlPx := strongestInk(img, image.Rect(22, 22, 38, 44), bg)
	if sameRGB(goPx, bg) {
		t.Fatal("go row icon matches the panel")
	}
	if sameRGB(htmlPx, bg) {
		t.Fatal("html row icon matches the panel")
	}
	if goPx == htmlPx {
		t.Fatalf("go and html icons painted the same color %+v", goPx)
	}
}

func sameRGB(c color.RGBA, bg color.NRGBA) bool {
	return c.R == bg.R && c.G == bg.G && c.B == bg.B
}

func strongestInk(img *image.RGBA, r image.Rectangle, bg color.NRGBA) color.RGBA {
	best := color.RGBA{R: bg.R, G: bg.G, B: bg.B, A: bg.A}
	bestD := 0
	r = r.Intersect(img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			d := idist(c.R, bg.R) + idist(c.G, bg.G) + idist(c.B, bg.B)
			if d > bestD {
				bestD = d
				best = c
			}
		}
	}
	return best
}

func idist(a, b uint8) int {
	d := int(a) - int(b)
	if d < 0 {
		return -d
	}
	return d
}

func TestListWheelScrollsAndClamps(t *testing.T) {
	var (
		r      input.Router
		scroll int
	)
	rows := make([]string, 10)
	for i := range rows {
		rows[i] = "row"
	}
	fg := color.NRGBA{A: 255}
	gtx := layout.Context{
		Ops: new(op.Ops),
		Metric: unit.Metric{
			PxPerDp: 1,
			PxPerSp: 1,
		},
		Constraints: layout.Exact(image.Pt(200, 72)),
		Source:      r.Source(),
	}
	layoutList := func() {
		gtx.Ops.Reset()
		List(gtx, nil, unit.Sp(13), rows, 9, fg, fg, color.NRGBA{}, &scroll, nil)
	}
	layoutList()
	if scroll != 0 {
		t.Fatalf("scroll = %d, want 0", scroll)
	}
	r.Frame(gtx.Ops)
	r.Queue(pointer.Event{
		Source:   pointer.Mouse,
		Kind:     pointer.Scroll,
		Position: f32.Pt(20, 20),
		Scroll:   f32.Pt(0, 12),
	})
	layoutList()
	if scroll == 0 {
		t.Fatal("wheel did not scroll the list")
	}
	// Active is an absolute index. A window that includes row 9 must not panic.
	scroll = 7
	layoutList()
	scroll = 100
	layoutList()
	const maxRows = 72 / 24
	if scroll != len(rows)-maxRows {
		t.Fatalf("scroll = %d, want %d", scroll, len(rows)-maxRows)
	}
}
