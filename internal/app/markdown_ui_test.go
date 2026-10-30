package app

import (
	"image"
	"testing"

	"gioui.org/font/gofont"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/text"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/harness"
	"github.com/russoj88/gio-ide/internal/keymap"
	"github.com/russoj88/gio-ide/internal/settings"
	"github.com/russoj88/gio-ide/internal/theme"
)

func TestMarkdownOpensInPreview(t *testing.T) {
	s := &shell{
		bus:      bus.New(nil),
		docs:     map[uint64]*buffer.Doc{},
		rootPane: &pane{ratio: 0.5},
	}
	s.focusPane = s.rootPane
	md := buffer.FromBytes("readme.md", []byte("# Hi\n"))
	s.docs[md.ID()] = md
	s.attachOpened(md.ID())
	if s.activeTab() == nil || !s.activeTab().preview {
		t.Fatal("markdown should open in preview")
	}
	gof := buffer.FromBytes("a.go", []byte("package a\n"))
	s.docs[gof.ID()] = gof
	s.attachOpened(gof.ID())
	if s.activeTab().preview || s.activeTab().view.Doc.Path() != gof.Path() {
		t.Fatal("go file should open as the raw editor")
	}
	if s.rootPane.tabs[0].preview != true {
		t.Fatal("the markdown tab should stay in preview")
	}
}

func TestPreviewTypingDoesNotEdit(t *testing.T) {
	keys := keymap.Default()
	s := &shell{
		bus:      bus.New(nil),
		docs:     map[uint64]*buffer.Doc{},
		rootPane: &pane{ratio: 0.5},
		keys:     keys,
		chord:    keymap.Resolver{Map: keys},
		focus:    focusEditor,
	}
	s.focusPane = s.rootPane
	doc := buffer.FromBytes("readme.md", []byte("# Hi\n"))
	s.docs[doc.ID()] = doc
	s.attachOpened(doc.ID())
	before := string(doc.Snapshot().Bytes())
	s.onKey(key.Event{Name: "X", State: key.Press}, false)
	if got := string(doc.Snapshot().Bytes()); got != before {
		t.Fatalf("preview typing changed the buffer to %q", got)
	}
	s.activeTab().preview = false
	s.onKey(key.Event{Name: "X", State: key.Press}, false)
	if got := string(doc.Snapshot().Bytes()); got == before {
		t.Fatal("raw mode did not insert the key")
	}
	s.onKey(key.Event{Name: key.NameDownArrow, State: key.Press}, false)
	if s.activeTab().previewY != 0 {
		t.Fatal("raw mode should not scroll the preview")
	}
	s.activeTab().preview = true
	s.onKey(key.Event{Name: key.NameDownArrow, State: key.Press}, false)
	if s.activeTab().previewY == 0 {
		t.Fatal("preview down did not scroll")
	}
}

func TestMarkdownToggleKeepsBytes(t *testing.T) {
	store := settings.NewStore(t.TempDir()+"/settings.toml", "")
	doc := buffer.FromBytes("readme.md", []byte("# Hello\n\nA longer body for the preview.\n"))
	s := &shell{
		store:    store,
		theme:    theme.Dark(),
		shaper:   text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection())),
		docs:     map[uint64]*buffer.Doc{},
		rootPane: &pane{ratio: 0.5},
		focus:    focusEditor,
	}
	s.focusPane = s.rootPane
	s.docs[doc.ID()] = doc
	s.attachOpened(doc.ID())
	tab := s.activeTab()
	ver := doc.Version()
	body := string(doc.Snapshot().Bytes())

	d, err := harness.New(image.Pt(480, 320))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		return s.layoutLeaf(gtx, s.rootPane)
	}
	// The control sits in the top-right inset. "Text" plus its padding
	// lands near this point at 1 px per dp.
	d.Click(image.Pt(446, 20))
	if tab.preview {
		t.Fatal("Text control did not show the raw editor")
	}
	if doc.Version() != ver || string(doc.Snapshot().Bytes()) != body {
		t.Fatal("switching to raw changed the buffer")
	}
	tab.view.Type("x")
	if string(doc.Snapshot().Bytes()) == body {
		t.Fatal("raw editor did not accept text")
	}
}
