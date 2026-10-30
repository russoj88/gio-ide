package app

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/editor"
)

func TestFormatResultWritesAndMovesCarets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.go")
	old := []byte("abcdef")
	doc := buffer.FromBytes(path, old)
	view := editor.NewView(doc)
	view.SetSelections([]editor.Sel{
		{Anchor: 1, Head: 1},
		{Anchor: 3, Head: 3},
	})
	s := formatShell(doc, view)
	s.formatToken[doc.ID()] = 1

	saved := make(chan struct{}, 1)
	var mu sync.Mutex
	changed := false
	cancel := s.bus.Subscribe(func(e bus.Event) {
		switch ev := e.(type) {
		case bus.DocumentChanged:
			if ev.DocID == doc.ID() {
				mu.Lock()
				changed = true
				mu.Unlock()
			}
		case bus.DocumentSaved:
			if ev.DocID == doc.ID() {
				select {
				case saved <- struct{}{}:
				default:
				}
			}
		}
	})
	defer cancel()

	neu := []byte("abXYf")
	s.onFormat(bus.FormatResult{
		DocID:       doc.ID(),
		BaseVersion: doc.Version(),
		Token:       1,
		Path:        path,
		Data:        neu,
		SaveAfter:   true,
	})
	if got := string(doc.Snapshot().Bytes()); got != string(neu) {
		t.Fatalf("buffer %q", got)
	}
	sels := view.Selections()
	if len(sels) != 2 || sels[0].Head != 1 || sels[1].Head != 4 {
		t.Fatalf("selections %+v", sels)
	}
	<-saved
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(neu) {
		t.Fatalf("file %q", got)
	}
	if s.saveVersions[doc.ID()] != doc.Version() {
		t.Fatalf("saved version %d doc %d", s.saveVersions[doc.ID()], doc.Version())
	}
	mu.Lock()
	gotChange := changed
	mu.Unlock()
	if !gotChange {
		t.Fatal("document change was not published")
	}
}

func TestFormatResultStaleTokenDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.go")
	orig := []byte("abcdef")
	if err := os.WriteFile(path, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	doc := buffer.FromBytes(path, orig)
	if _, err := doc.Apply([]buffer.Edit{{Start: 0, End: 0, Text: "Z"}}, buffer.OriginCommand); err != nil {
		t.Fatal(err)
	}
	view := editor.NewView(doc)
	s := formatShell(doc, view)
	s.formatToken[doc.ID()] = 2
	s.closeAfter = doc.ID()
	before := string(doc.Snapshot().Bytes())

	s.onFormat(bus.FormatResult{
		DocID:       doc.ID(),
		BaseVersion: doc.Version(),
		Token:       1,
		Path:        path,
		Data:        []byte("abXYf"),
		SaveAfter:   true,
	})
	if got := string(doc.Snapshot().Bytes()); got != before {
		t.Fatalf("buffer %q", got)
	}
	if !doc.Dirty() {
		t.Fatal("stale result cleared the dirty bit")
	}
	if s.closeAfter != 0 {
		t.Fatal("closeAfter stayed set")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(orig) {
		t.Fatalf("file %q", got)
	}
	s.drain()
	if s.notice != "File changed during format" {
		t.Fatalf("notice %q", s.notice)
	}
}

func formatShell(doc *buffer.Doc, view *editor.View) *shell {
	s := &shell{
		bus:          bus.New(nil),
		docs:         map[uint64]*buffer.Doc{doc.ID(): doc},
		saveVersions: map[uint64]int32{},
		formatToken:  map[uint64]int64{},
		rootPane: &pane{
			ratio: 0.5,
			tabs: []*edTab{{
				view:      view,
				seenVer:   doc.Version(),
				prev:      append([]byte(nil), doc.Snapshot().Bytes()...),
				bracketAt: -1,
			}},
		},
	}
	s.focusPane = s.rootPane
	return s
}
