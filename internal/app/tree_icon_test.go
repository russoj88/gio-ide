package app

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/harness"
	"github.com/russoj88/gio-ide/internal/theme"
	"github.com/russoj88/gio-ide/internal/ui"
)

func TestTreeIconColorsReadOnBothThemes(t *testing.T) {
	for _, th := range []*theme.Theme{theme.Dark(), theme.Light()} {
		t.Run(th.Name, func(t *testing.T) {
			cols := iconColors(th)
			bg := th.Color("ui.background")
			sel := th.Color("ui.selection")
			for _, c := range []color.NRGBA{
				cols.Folder, cols.Go, cols.Test, cols.Markdown, cols.License,
				cols.HTML, cols.JS, cols.TS, cols.JSON, cols.YAML, cols.TOML,
				cols.Module, cols.CSS, cols.Image, cols.Shell, cols.Git, cols.Plain,
			} {
				if !colorSeparated(c, bg) || !colorSeparated(c, sel) {
					t.Errorf("%s icon %+v is too close to background %+v or selection %+v", th.Name, c, bg, sel)
				}
			}
			assertIconInk(t, th, cols, bg)
		})
	}
}

func colorSeparated(a, b color.NRGBA) bool {
	d := abs(int(a.R)-int(b.R)) + abs(int(a.G)-int(b.G)) + abs(int(a.B)-int(b.B))
	return d >= 100
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func assertIconInk(t *testing.T, th *theme.Theme, cols ui.IconColors, bg color.NRGBA) {
	t.Helper()
	labels := []struct {
		name string
		dir  bool
	}{
		{"dir", true},
		{"main.go", false},
		{"a_test.go", false},
		{"README.md", false},
		{"LICENSE", false},
		{"index.html", false},
		{"app.js", false},
		{"app.ts", false},
		{"data.json", false},
		{"data.yaml", false},
		{"note.toml", false},
		{"go.mod", false},
		{"app.css", false},
		{"pic.png", false},
		{"run.sh", false},
		{".gitignore", false},
		{"Makefile", false},
	}
	var tags [17]byte
	nodes := make([]ui.TreeNode, len(labels))
	for i, lb := range labels {
		nodes[i] = ui.TreeNode{Label: lb.name, Path: lb.name, Dir: lb.dir, Tag: &tags[i]}
	}
	const rowH = 22
	d, err := harness.New(image.Pt(240, rowH*len(nodes)))
	if err != nil {
		t.Fatal(err)
	}
	fg := th.Color("ui.foreground")
	dim := th.Color("ui.dim")
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		ui.Fill(gtx, bg)
		ui.Tree(gtx, nil, unit.Sp(13), nodes, fg, dim, th.Color("ui.selection"), cols, nil)
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	d.Step()
	img, err := d.Image()
	if err != nil {
		t.Logf("icon ink: %v", err)
		return
	}
	for i, lb := range labels {
		r := image.Rect(22, i*rowH, 38, (i+1)*rowH)
		px := strongestInk(img, r, bg)
		if px.R == bg.R && px.G == bg.G && px.B == bg.B {
			t.Errorf("%s (%s) icon matches the panel in %+v", lb.name, th.Name, r)
		}
	}
}

func strongestInk(img *image.RGBA, r image.Rectangle, bg color.NRGBA) color.RGBA {
	best := color.RGBA{R: bg.R, G: bg.G, B: bg.B, A: bg.A}
	bestD := 0
	r = r.Intersect(img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			d := abs(int(c.R)-int(bg.R)) + abs(int(c.G)-int(bg.G)) + abs(int(c.B)-int(bg.B))
			if d > bestD {
				bestD = d
				best = c
			}
		}
	}
	return best
}
