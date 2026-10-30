package app

import (
	"image"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/editor"
	"github.com/russoj88/gio-ide/internal/harness"
	"github.com/russoj88/gio-ide/internal/keymap"
)

func TestPrintableKeyInsertsOnce(t *testing.T) {
	km := keymap.Default()
	doc := buffer.FromBytes("a.go", nil)
	view := editor.NewView(doc)
	s := &shell{
		docs:     map[uint64]*buffer.Doc{doc.ID(): doc},
		rootPane: &pane{tabs: []*edTab{{view: view}}},
		focus:    focusEditor,
		keys:     km,
		chord:    keymap.Resolver{Map: km},
	}
	s.focusPane = s.rootPane

	d, err := harness.New(image.Pt(200, 80))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		s.handleKeys(gtx)
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	d.Focus(s.inputTag())
	d.Queue(
		key.Event{Name: "A", State: key.Press},
		key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "a"},
		key.Event{Name: "B", State: key.Press},
		key.EditEvent{Range: key.Range{Start: 1, End: 1}, Text: "b"},
	)
	if got := string(doc.Snapshot().Bytes()); got != "ab" {
		t.Fatalf("buffer %q, want ab", got)
	}
	if line, col := view.CaretPos(); line != 0 || col != 2 {
		t.Fatalf("caret %d:%d, want 0:2", line, col)
	}

	d.Press(key.Name("c"), 0)
	if got := string(doc.Snapshot().Bytes()); got != "abc" {
		t.Fatalf("buffer after press %q, want abc", got)
	}
	d.Edit(3, 3, "d")
	if got := string(doc.Snapshot().Bytes()); got != "abcd" {
		t.Fatalf("buffer after edit %q, want abcd", got)
	}

	s.popup = "action"
	d.Focus(s.inputTag())
	d.Queue(
		key.Event{Name: "A", State: key.Press},
		key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "a"},
	)
	if got := string(doc.Snapshot().Bytes()); got != "abcd" {
		t.Fatalf("popup typing changed the buffer to %q", got)
	}
	if s.field != "a" {
		t.Fatalf("popup field %q, want a", s.field)
	}
}
