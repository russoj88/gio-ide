package app

import (
	"image"
	"testing"

	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/text"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/harness"
	"github.com/russoj88/gio-ide/internal/theme"
	"github.com/russoj88/gio-ide/internal/workspace"
)

func TestClickDirectoryOpensInTheFrame(t *testing.T) {
	s := &shell{
		root:     "/tmp/proj",
		bus:      bus.New(nil),
		docs:     map[uint64]*buffer.Doc{},
		opening:  map[string]bool{},
		theme:    theme.Dark(),
		shaper:   text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection())),
		tags:     map[string]*handlerTag{},
		leftOpen: true,
		leftTool: "project",
		files:    []string{"internal/a.go", "main.go"},
		tree: &workspace.Node{Kids: []*workspace.Node{
			{Name: "internal", Path: "internal", Dir: true, Kids: []*workspace.Node{
				{Name: "a.go", Path: "internal/a.go"},
			}},
			{Name: "main.go", Path: "main.go"},
		}},
		rootPane: &pane{ratio: 0.5},
		branch:   "—",
		intel:    "stopped",
		line:     1,
		col:      1,
	}
	s.focusPane = s.rootPane

	// The tool strip is the left 40px. A click there toggles the panel shut.
	const clickX = 90
	win := image.Pt(1280, 800)
	d, err := harness.New(win)
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		s.handleKeys(gtx)
		return s.layout(gtx)
	}
	d.Step()
	closedBands, haveClosed := leftPanelBands(t, d)

	// treeClickGap uses time.Now, so a second press on the same path
	// within 300ms does not collapse. Stop at the first open.
	var tried []image.Point
	var hit image.Point
	for y := 30; y <= 120; y += 2 {
		if s.openDirs["internal"] {
			break
		}
		at := image.Pt(clickX, y)
		tried = append(tried, at)
		leftBefore := s.leftOpen
		d.Click(at)
		if !s.leftOpen && leftBefore {
			t.Fatalf("left panel closed at %v (tool strip); tool %q nodes %v", at, s.leftTool, treePaths(s))
		}
		if s.openDirs["internal"] {
			hit = at
			break
		}
	}
	if !s.openDirs["internal"] {
		s.docsMu.Lock()
		nDocs := len(s.docs)
		s.docsMu.Unlock()
		t.Fatalf("internal stayed closed after %v; leftOpen=%v tool=%q focus=%v nodes=%v openDirs=%v docs=%d",
			tried, s.leftOpen, s.leftTool, s.focus, treePaths(s), s.openDirs, nDocs)
	}
	var child bool
	for _, n := range s.treeNodes() {
		if n.Path == "internal/a.go" {
			child = true
		}
	}
	if !child {
		t.Fatalf("tree nodes %v, want internal/a.go", treePaths(s))
	}
	t.Logf("opened internal at %v", hit)

	if !haveClosed {
		return
	}
	img, err := d.Image()
	if err != nil {
		t.Logf("open frame image: %v", err)
		return
	}
	openBands := brightTextBands(img)
	if openBands <= closedBands {
		t.Fatalf("bright rows in the left panel: closed %d, open %d; want more when internal is expanded", closedBands, openBands)
	}
}

func leftPanelBands(t *testing.T, d *harness.Desktop) (int, bool) {
	t.Helper()
	img, err := d.Image()
	if err != nil {
		t.Logf("closed frame image: %v", err)
		return 0, false
	}
	return brightTextBands(img), true
}

func treePaths(s *shell) []string {
	nodes := s.treeNodes()
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.Path
	}
	return out
}

// brightTextBands counts horizontal runs of text-like pixels in the left
// panel. Directory labels are about #6e727a, so max(R,G,B) >= 100 counts.
func brightTextBands(img *image.RGBA) int {
	b := img.Bounds()
	x0 := b.Min.X + 50
	x1 := b.Min.X + 250
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	bands := 0
	in := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		bright := false
		for x := x0; x < x1; x++ {
			c := img.RGBAAt(x, y)
			if c.R >= 100 || c.G >= 100 || c.B >= 100 {
				bright = true
				break
			}
		}
		if bright && !in {
			bands++
		}
		in = bright
	}
	return bands
}
