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
	"github.com/russoj88/gio-ide/internal/settings"
	"github.com/russoj88/gio-ide/internal/terminal"
	"github.com/russoj88/gio-ide/internal/theme"
)

func TestTerminalExitClosesThatTab(t *testing.T) {
	s := &shell{focus: focusTerminal, bottomOpen: true}
	a := terminal.New(1, 10, nil)
	b := terminal.New(2, 10, nil)
	s.terms = []*termTab{{term: a}, {term: b}}
	s.termActive = 0

	s.onTerminalExit(bus.TerminalExited{Session: 2})
	if len(s.terms) != 1 || s.terms[0].term.ID() != 1 {
		t.Fatalf("tabs %+v", s.terms)
	}
	if !s.bottomOpen || s.focus != focusTerminal {
		t.Fatalf("open %v focus %v", s.bottomOpen, s.focus)
	}

	s.onTerminalExit(bus.TerminalExited{Session: 1, Err: "boom"})
	if len(s.terms) != 0 || s.bottomOpen || s.focus != focusEditor {
		t.Fatalf("open %v focus %v tabs %d", s.bottomOpen, s.focus, len(s.terms))
	}
	if s.notice != "terminal: boom" {
		t.Fatal(s.notice)
	}
	s.onTerminalExit(bus.TerminalExited{Session: 1, Err: "again"})
	if len(s.terms) != 0 || s.bottomOpen {
		t.Fatal("exit of a closed session reopened the panel")
	}
	if s.notice != "terminal: again" {
		t.Fatal(s.notice)
	}
}

func TestTerminalHitTagsAreDistinct(t *testing.T) {
	var s shell
	a := &termTab{}
	b := &termTab{}
	if &s.termSash == &s.termNew {
		t.Fatal("resize bar and + share a pointer tag")
	}
	if &a.hit == &a.close || &a.hit == &b.hit || &a.close == &b.close {
		t.Fatal("tab tags alias")
	}
}

func TestTerminalTabsSwitchPlusAndClose(t *testing.T) {
	s := termStripShell()
	s.bottomOpen = true
	s.focus = focusTerminal
	s.terms = []*termTab{
		{term: terminal.New(1, 10, nil)},
		{term: terminal.New(2, 10, nil)},
	}
	opened := 0
	s.onCommand = map[string]func(){
		"terminal.new": func() { opened++ },
	}
	d := layoutTermStrip(t, s)

	if len(s.termTabLeft) != 2 {
		t.Fatalf("tabs %d", len(s.termTabLeft))
	}
	d.Click(image.Pt(s.termTabLeft[1]+12, 15))
	if s.termActive != 1 || s.focus != focusTerminal {
		t.Fatalf("active %d focus %v", s.termActive, s.focus)
	}
	d.Click(image.Pt(s.termTabLeft[0]+12, 15))
	if s.termActive != 0 {
		t.Fatalf("back on tab %d", s.termActive)
	}
	d.Click(image.Pt(s.termPlusLeft+19, 15))
	if opened != 1 {
		t.Fatalf("plus opened %d", opened)
	}
	d.Click(image.Pt(s.termTabLeft[1]-8, 15))
	if len(s.terms) != 1 || s.terms[0].term.ID() != 2 {
		t.Fatalf("close left %d tabs id %d", len(s.terms), s.terms[0].term.ID())
	}
	if !s.bottomOpen {
		t.Fatal("panel closed with a tab left")
	}
	d.Step()
	d.Click(image.Pt(s.termPlusLeft-8, 15))
	if len(s.terms) != 0 || s.bottomOpen || s.focus != focusEditor {
		t.Fatalf("last close open %v focus %v tabs %d", s.bottomOpen, s.focus, len(s.terms))
	}
}

func TestPlusOpensFromTheFullLayout(t *testing.T) {
	s := termStripShell()
	s.bus = bus.New(nil)
	s.store = settings.NewStore("", "")
	s.docs = map[uint64]*buffer.Doc{}
	s.tags = map[string]*handlerTag{}
	s.bottomOpen = true
	s.bottomTool = "terminal"
	s.terms = []*termTab{{term: terminal.New(1, 10, nil)}}
	s.focusPane = &pane{ratio: 0.5}
	s.rootPane = s.focusPane
	opened := 0
	s.onCommand = map[string]func(){
		"terminal.new": func() { opened++ },
	}
	d, err := harness.New(image.Pt(1280, 800))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		return s.layout(gtx)
	}
	d.Step()
	if s.termSashY < 100 || s.termPlusLeft < 20 {
		t.Fatalf("sash %d plus %d", s.termSashY, s.termPlusLeft)
	}
	d.Click(image.Pt(40+s.termPlusLeft+19, s.termSashY+s.termSashH+15))
	if opened != 1 {
		t.Fatalf("plus opened %d (plus local %d sash %d)", opened, s.termPlusLeft, s.termSashY)
	}
	if len(s.terms) != 1 {
		t.Fatalf("stub started a session, tabs %d", len(s.terms))
	}
}

func termStripShell() *shell {
	return &shell{
		theme:  theme.Dark(),
		shaper: text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection())),
	}
}

func layoutTermStrip(t *testing.T, s *shell) *harness.Desktop {
	t.Helper()
	d, err := harness.New(image.Pt(800, 40))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		return s.layoutTermTabs(gtx)
	}
	d.Step()
	return d
}
