package ui

import (
	"image"
	"image/color"
	"math"
	"path/filepath"
	"strings"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// IconColors tints the marks in a project tree. A color with zero alpha
// uses the row foreground passed to Tree.
type IconColors struct {
	Folder   color.NRGBA
	Go       color.NRGBA
	Test     color.NRGBA
	Markdown color.NRGBA
	License  color.NRGBA
	HTML     color.NRGBA
	JS       color.NRGBA
	TS       color.NRGBA
	JSON     color.NRGBA
	YAML     color.NRGBA
	TOML     color.NRGBA
	Module   color.NRGBA
	CSS      color.NRGBA
	Image    color.NRGBA
	Shell    color.NRGBA
	Git      color.NRGBA
	Plain    color.NRGBA
}

// treeIcon is the mark drawn for one project-tree row.
type treeIcon int

const (
	iconPlain treeIcon = iota
	iconFolder
	iconGo
	iconTest
	iconMarkdown
	iconLicense
	iconHTML
	iconJS
	iconTS
	iconJSON
	iconYAML
	iconTOML
	iconModule
	iconCSS
	iconImage
	iconShell
	iconGit
)

func (c IconColors) pick(k treeIcon, fallback color.NRGBA) color.NRGBA {
	var col color.NRGBA
	switch k {
	case iconFolder:
		col = c.Folder
	case iconGo:
		col = c.Go
	case iconTest:
		col = c.Test
	case iconMarkdown:
		col = c.Markdown
	case iconLicense:
		col = c.License
	case iconHTML:
		col = c.HTML
	case iconJS:
		col = c.JS
	case iconTS:
		col = c.TS
	case iconJSON:
		col = c.JSON
	case iconYAML:
		col = c.YAML
	case iconTOML:
		col = c.TOML
	case iconModule:
		col = c.Module
	case iconCSS:
		col = c.CSS
	case iconImage:
		col = c.Image
	case iconShell:
		col = c.Shell
	case iconGit:
		col = c.Git
	default:
		col = c.Plain
	}
	if col.A == 0 {
		return fallback
	}
	return col
}

// iconKind reports the mark for a tree row. dir wins. Names are compared
// case-insensitively. A _test.go file is a test, not Go source.
func iconKind(name string, dir bool) treeIcon {
	if dir {
		return iconFolder
	}
	base := strings.ToLower(name)
	switch base {
	case "go.mod", "go.sum", "go.work":
		return iconModule
	case "license", "licence", "copying", "copyright", "unlicense",
		"license.txt", "license.md", "license.rst",
		"licence.txt", "licence.md", "licence.rst",
		"copying.txt", "copying.md", "copying.rst",
		"copyright.txt", "copyright.md", "copyright.rst",
		"unlicense.txt", "unlicense.md", "unlicense.rst":
		return iconLicense
	case "readme":
		return iconMarkdown
	case ".gitignore", ".gitattributes", ".gitmodules":
		return iconGit
	}
	if strings.HasSuffix(base, "_test.go") {
		return iconTest
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".go":
		return iconGo
	case ".md", ".markdown", ".mdown":
		return iconMarkdown
	case ".html", ".htm":
		return iconHTML
	case ".js", ".mjs", ".cjs", ".jsx":
		return iconJS
	case ".ts", ".tsx", ".mts", ".cts":
		return iconTS
	case ".json":
		return iconJSON
	case ".yaml", ".yml":
		return iconYAML
	case ".toml":
		return iconTOML
	case ".css", ".scss":
		return iconCSS
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico":
		return iconImage
	case ".sh", ".bash", ".zsh":
		return iconShell
	default:
		return iconPlain
	}
}

// drawTreeIcon paints kind into the row slot. The geometry is local:
// the tree does not load an icon font or an image.
func drawTreeIcon(gtx layout.Context, kind treeIcon, col color.NRGBA) layout.Dimensions {
	size := gtx.Constraints.Max
	if size.X < 1 {
		size.X = 1
	}
	if size.Y < 1 {
		size.Y = 1
	}
	side := max(min(min(gtx.Dp(unit.Dp(12)), size.X), size.Y), 1)
	ox := (size.X - side) / 2
	oy := (size.Y - side) / 2
	stack := op.Offset(image.Pt(ox, oy)).Push(gtx.Ops)
	s := float32(side)
	width := max(s*0.12, 1)
	paintFilled(gtx, kind, s, col)
	paintStroked(gtx, kind, s, width, col)
	stack.Pop()
	return layout.Dimensions{Size: size}
}

func paintFilled(gtx layout.Context, kind treeIcon, s float32, col color.NRGBA) {
	var p clip.Path
	p.Begin(gtx.Ops)
	if !fillIcon(&p, kind, s) {
		p.End()
		return
	}
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: p.End()}.Op())
}

func paintStroked(gtx layout.Context, kind treeIcon, s, width float32, col color.NRGBA) {
	var p clip.Path
	p.Begin(gtx.Ops)
	if !strokeIcon(&p, kind, s) {
		p.End()
		return
	}
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
}

func fillIcon(p *clip.Path, kind treeIcon, s float32) bool {
	switch kind {
	case iconFolder:
		folder(p, s)
	case iconGo:
		disc(p, f32.Pt(s*0.48, s*0.64), s*0.20, 1)
	case iconTest:
		diamond(p, f32.Pt(s*0.48, s*0.64), s*0.22)
	case iconMarkdown:
		rect(p, s*0.30, s*0.40, s*0.28, s*0.10)
		rect(p, s*0.30, s*0.58, s*0.42, s*0.10)
	case iconHTML:
		chevron(p, s, false)
		chevron(p, s, true)
	case iconJSON:
		bracket(p, s, false)
		bracket(p, s, true)
	case iconYAML:
		rect(p, s*0.16, s*0.22, s*0.68, s*0.10)
		rect(p, s*0.16, s*0.44, s*0.52, s*0.10)
		rect(p, s*0.16, s*0.66, s*0.36, s*0.10)
	case iconTOML:
		cell := s * 0.28
		gap := s * 0.16
		rect(p, gap, gap, cell, cell)
		rect(p, gap+cell+gap, gap, cell, cell)
		rect(p, gap, gap+cell+gap, cell, cell)
		rect(p, gap+cell+gap, gap+cell+gap, cell, cell)
	case iconModule:
		rect(p, s*0.16, s*0.24, s*0.68, s*0.16)
		rect(p, s*0.16, s*0.58, s*0.68, s*0.16)
	case iconCSS:
		c := f32.Pt(s*0.5, s*0.5)
		disc(p, c, s*0.40, 1)
		disc(p, c, s*0.26, -1)
		disc(p, c, s*0.10, 1)
	case iconImage:
		mountain(p, s)
	case iconShell:
		prompt(p, s)
	case iconGit:
		disc(p, f32.Pt(s*0.50, s*0.78), s*0.10, 1)
		disc(p, f32.Pt(s*0.24, s*0.24), s*0.10, 1)
		disc(p, f32.Pt(s*0.76, s*0.24), s*0.10, 1)
	default:
		return false
	}
	return true
}

func strokeIcon(p *clip.Path, kind treeIcon, s float32) bool {
	switch kind {
	case iconGo, iconTest, iconMarkdown, iconPlain:
		page(p, s)
	case iconLicense:
		page(p, s)
		ring(p, f32.Pt(s*0.50, s*0.62), s*0.20)
	case iconJS, iconTS:
		brace(p, s, false)
		brace(p, s, true)
	case iconImage:
		frame(p, s)
	case iconGit:
		branch(p, s)
	default:
		return false
	}
	return true
}

func folder(p *clip.Path, s float32) {
	p.MoveTo(f32.Pt(s*0.06, s*0.34))
	p.LineTo(f32.Pt(s*0.06, s*0.18))
	p.LineTo(f32.Pt(s*0.42, s*0.18))
	p.LineTo(f32.Pt(s*0.52, s*0.34))
	p.LineTo(f32.Pt(s*0.94, s*0.34))
	p.LineTo(f32.Pt(s*0.94, s*0.84))
	p.LineTo(f32.Pt(s*0.06, s*0.84))
	p.Close()
}

func page(p *clip.Path, s float32) {
	p.MoveTo(f32.Pt(s*0.20, s*0.06))
	p.LineTo(f32.Pt(s*0.58, s*0.06))
	p.LineTo(f32.Pt(s*0.82, s*0.30))
	p.LineTo(f32.Pt(s*0.82, s*0.94))
	p.LineTo(f32.Pt(s*0.20, s*0.94))
	p.Close()
	p.MoveTo(f32.Pt(s*0.58, s*0.06))
	p.LineTo(f32.Pt(s*0.58, s*0.30))
	p.LineTo(f32.Pt(s*0.82, s*0.30))
}

func rect(p *clip.Path, x, y, w, h float32) {
	p.MoveTo(f32.Pt(x, y))
	p.LineTo(f32.Pt(x+w, y))
	p.LineTo(f32.Pt(x+w, y+h))
	p.LineTo(f32.Pt(x, y+h))
	p.Close()
}

func disc(p *clip.Path, c f32.Point, r, dir float32) {
	if r <= 0 {
		return
	}
	p.MoveTo(f32.Pt(c.X+r, c.Y))
	p.ArcTo(c, c, dir*2*float32(math.Pi))
	p.Close()
}

func ring(p *clip.Path, c f32.Point, r float32) {
	p.MoveTo(f32.Pt(c.X+r, c.Y))
	p.ArcTo(c, c, 2*float32(math.Pi))
	p.Close()
}

func diamond(p *clip.Path, c f32.Point, r float32) {
	p.MoveTo(f32.Pt(c.X, c.Y-r))
	p.LineTo(f32.Pt(c.X+r, c.Y))
	p.LineTo(f32.Pt(c.X, c.Y+r))
	p.LineTo(f32.Pt(c.X-r, c.Y))
	p.Close()
}

func chevron(p *clip.Path, s float32, right bool) {
	// Each angle stays in its own half so the pair does not fill into one blob.
	xs := []float32{0.40, 0.06, 0.40, 0.40, 0.22, 0.40}
	ys := []float32{0.16, 0.50, 0.84, 0.68, 0.50, 0.32}
	if right {
		for i, x := range xs {
			xs[i] = 1 - x
		}
	}
	p.MoveTo(f32.Pt(s*xs[0], s*ys[0]))
	for i := 1; i < len(xs); i++ {
		p.LineTo(f32.Pt(s*xs[i], s*ys[i]))
	}
	p.Close()
}

func bracket(p *clip.Path, s float32, right bool) {
	stem := s * 0.12
	serif := s * 0.26
	thick := s * 0.14
	y0 := s * 0.12
	y1 := s * 0.88
	if right {
		x := s * 0.88
		rect(p, x-stem, y0, stem, y1-y0)
		rect(p, x-serif, y0, serif, thick)
		rect(p, x-serif, y1-thick, serif, thick)
		return
	}
	x := s * 0.12
	rect(p, x, y0, stem, y1-y0)
	rect(p, x, y0, serif, thick)
	rect(p, x, y1-thick, serif, thick)
}

func brace(p *clip.Path, s float32, right bool) {
	xTip, xMid, xEnd := s*0.08, s*0.24, s*0.42
	if right {
		xTip, xMid, xEnd = s-xTip, s-xMid, s-xEnd
	}
	p.MoveTo(f32.Pt(xEnd, s*0.06))
	p.LineTo(f32.Pt(xMid, s*0.16))
	p.LineTo(f32.Pt(xMid, s*0.38))
	p.LineTo(f32.Pt(xTip, s*0.50))
	p.LineTo(f32.Pt(xMid, s*0.62))
	p.LineTo(f32.Pt(xMid, s*0.84))
	p.LineTo(f32.Pt(xEnd, s*0.94))
}

func prompt(p *clip.Path, s float32) {
	p.MoveTo(f32.Pt(s*0.16, s*0.14))
	p.LineTo(f32.Pt(s*0.70, s*0.40))
	p.LineTo(f32.Pt(s*0.16, s*0.66))
	p.LineTo(f32.Pt(s*0.16, s*0.50))
	p.LineTo(f32.Pt(s*0.42, s*0.40))
	p.LineTo(f32.Pt(s*0.16, s*0.30))
	p.Close()
	rect(p, s*0.16, s*0.78, s*0.68, s*0.12)
}

func frame(p *clip.Path, s float32) {
	p.MoveTo(f32.Pt(s*0.10, s*0.16))
	p.LineTo(f32.Pt(s*0.90, s*0.16))
	p.LineTo(f32.Pt(s*0.90, s*0.86))
	p.LineTo(f32.Pt(s*0.10, s*0.86))
	p.Close()
}

func mountain(p *clip.Path, s float32) {
	p.MoveTo(f32.Pt(s*0.22, s*0.72))
	p.LineTo(f32.Pt(s*0.48, s*0.42))
	p.LineTo(f32.Pt(s*0.80, s*0.72))
	p.Close()
}

func branch(p *clip.Path, s float32) {
	p.MoveTo(f32.Pt(s*0.50, s*0.78))
	p.LineTo(f32.Pt(s*0.50, s*0.46))
	p.MoveTo(f32.Pt(s*0.50, s*0.46))
	p.LineTo(f32.Pt(s*0.24, s*0.24))
	p.MoveTo(f32.Pt(s*0.50, s*0.46))
	p.LineTo(f32.Pt(s*0.76, s*0.24))
}
