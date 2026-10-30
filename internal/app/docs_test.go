package app

import (
	"testing"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
)

func TestSplitCloseAndUnsaved(t *testing.T) {
	s := &shell{
		bus:      bus.New(nil),
		docs:     map[uint64]*buffer.Doc{},
		rootPane: &pane{ratio: 0.5},
	}
	s.focusPane = s.rootPane
	doc := buffer.FromBytes("a.go", []byte("package a\n"))
	s.docs[doc.ID()] = doc
	s.attachOpened(doc.ID())
	if s.activeView() == nil {
		t.Fatal("expected a view")
	}
	s.splitActive(false)
	if s.rootPane.isLeaf() {
		t.Fatal("expected a side-by-side split")
	}
	if s.docTabCount(doc.ID()) != 2 {
		t.Fatalf("tabs = %d, want 2", s.docTabCount(doc.ID()))
	}
	s.cmdClose()
	if s.docTabCount(doc.ID()) != 1 {
		t.Fatalf("after close tabs = %d, want 1", s.docTabCount(doc.ID()))
	}
	if !s.rootPane.isLeaf() {
		t.Fatal("empty pane should collapse")
	}
	s.activeView().Type("x")
	s.cmdClose()
	if s.popup != "unsaved" {
		t.Fatalf("popup = %q, want unsaved", s.popup)
	}
	if s.docTabCount(doc.ID()) != 1 {
		t.Fatal("dirty close should wait for an answer")
	}
	s.answerUnsaved(1)
	if s.docTabCount(doc.ID()) != 0 {
		t.Fatal("Don't Save should close the tab")
	}
}
