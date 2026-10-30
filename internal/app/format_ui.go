package app

import (
	"bytes"
	"context"
	"slices"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/editor"
	"github.com/russoj88/gio-ide/internal/format"
)

func (s *shell) cmdReformat() {
	v := s.activeView()
	if v == nil {
		s.notify("No editor tab")
		return
	}
	s.startFormat(v.Doc, true, false, false, false)
}

func (s *shell) cmdOrganizeImports() {
	v := s.activeView()
	if v == nil {
		s.notify("No editor tab")
		return
	}
	s.startFormat(v.Doc, false, true, false, false)
}

// startFormat formats doc off the window goroutine.
// saveAfter writes the file when the result is still current.
func (s *shell) startFormat(doc *buffer.Doc, gofumpt, goimports, gofix, saveAfter bool) {
	if doc == nil {
		return
	}
	if doc.ReadOnly() {
		s.notify(doc.Notice())
		return
	}
	path := doc.Path()
	if !isGoPath(path) {
		if saveAfter {
			s.writeDoc(doc)
			return
		}
		s.notify("Format applies to Go files")
		return
	}
	if saveAfter && !gofumpt && !goimports && !gofix {
		s.writeDoc(doc)
		return
	}
	src := append([]byte(nil), doc.Snapshot().Bytes()...)
	id := doc.ID()
	ver := doc.Version()
	token := s.bumpFormatToken(id)
	go func() {
		out, err := runFormat(path, src, gofumpt, goimports, gofix)
		ev := bus.FormatResult{
			DocID:       id,
			BaseVersion: ver,
			Token:       token,
			Path:        path,
			SaveAfter:   saveAfter,
		}
		if err != nil {
			ev.Err = err.Error()
		} else {
			ev.Data = out
		}
		s.bus.Post(ev)
	}()
}

func runFormat(path string, src []byte, gofumpt, goimports, gofix bool) ([]byte, error) {
	out := src
	var err error
	if gofix {
		out, err = format.Gofix(context.Background(), path, out)
		if err != nil {
			return nil, err
		}
	}
	if goimports {
		out, err = format.Goimports(context.Background(), path, out)
		if err != nil {
			return nil, err
		}
	}
	if gofumpt {
		out, err = format.Gofumpt(out, format.LookupModule(path))
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *shell) onFormat(ev bus.FormatResult) {
	doc := s.docByID(ev.DocID)
	if doc == nil {
		return
	}
	if s.formatToken[ev.DocID] != ev.Token || doc.Version() != ev.BaseVersion {
		if ev.SaveAfter {
			s.notify("File changed during format")
			if s.closeAfter == ev.DocID {
				s.closeAfter = 0
			}
		}
		return
	}
	if ev.Err != "" {
		s.notify(ev.Err)
		if ev.SaveAfter {
			s.writeDoc(doc)
		}
		return
	}
	old := append([]byte(nil), doc.Snapshot().Bytes()...)
	if bytes.Equal(old, ev.Data) {
		if ev.SaveAfter {
			s.writeDoc(doc)
		}
		return
	}
	hunks := format.Hunks(old, ev.Data)
	eds := make([]buffer.Edit, 0, len(hunks))
	for _, h := range hunks {
		eds = append(eds, buffer.Edit{
			Start: buffer.Offset(h.Start),
			End:   buffer.Offset(h.End),
			Text:  h.Text,
		})
	}
	if _, err := doc.Apply(eds, buffer.OriginCommand); err != nil {
		s.notify(err.Error())
		if ev.SaveAfter {
			s.writeDoc(doc)
		}
		return
	}
	s.eachDocView(doc.ID(), func(v *editor.View) {
		sels := v.Selections()
		for i, sel := range sels {
			sels[i] = editor.Sel{
				Anchor: buffer.Offset(format.MapOffset(int(sel.Anchor), hunks)),
				Head:   buffer.Offset(format.MapOffset(int(sel.Head), hunks)),
			}
		}
		v.SetSelections(sels)
	})
	s.noteFormat(doc, old, hunks)
	if ev.SaveAfter {
		s.writeDoc(doc)
	}
}

func (s *shell) docByID(id uint64) *buffer.Doc {
	s.docsMu.Lock()
	doc := s.docs[id]
	s.docsMu.Unlock()
	if doc != nil {
		return doc
	}
	var found *buffer.Doc
	s.eachDocView(id, func(v *editor.View) {
		if found == nil && v.Doc != nil && v.Doc.ID() == id {
			found = v.Doc
		}
	})
	return found
}

// noteFormat updates folds, paint, and the syntax tree for a multi-hunk
// format. publishEdits would otherwise collapse the hunks into one span.
// The syntax engine takes one edit that replaces the whole buffer so its
// stored text matches the snapshot. Per-hunk edits would not, because Edit
// compares the edit result with the final snapshot.
func (s *shell) noteFormat(doc *buffer.Doc, old []byte, hunks []format.Hunk) {
	if doc == nil {
		return
	}
	snap := doc.Snapshot()
	active := s.activeView()
	for _, hunk := range slices.Backward(hunks) {
		ed := buffer.Edit{
			Start: buffer.Offset(hunk.Start),
			End:   buffer.Offset(hunk.End),
			Text:  hunk.Text,
		}
		s.shiftPaint(doc.ID(), ed)
		s.eachDocView(doc.ID(), func(v *editor.View) {
			v.AdjustFolds(old, ed, v != active)
		})
		if s.marks != nil {
			s.marks[doc.ID()] = editor.ShiftMarks(old, ed, s.marks[doc.ID()])
		}
	}
	if s.marks != nil {
		s.marks[doc.ID()] = editor.ClipMarks(s.marks[doc.ID()], len(snap.Bytes()))
	}
	if s.syn != nil {
		s.syn.Edit(doc.ID(), buffer.Edit{
			Start: 0,
			End:   buffer.Offset(len(old)),
			Text:  string(snap.Bytes()),
		}, snap)
	}
	// One edit, not the hunk list. didChange maps every offset against the
	// pre-edit text, and gopls applies each range to the document left by
	// the previous change. A second hunk would land in the wrong place.
	if s.ih != nil && s.useIntel(doc.Path()) {
		ed := oneEdit(old, snap.Bytes())
		s.ih.DidChange(doc.ID(), snap.Version(), []buffer.Edit{ed}, snap)
	}
	next := append([]byte(nil), snap.Bytes()...)
	ver := snap.Version()
	walkPanes(s.rootPane, func(p *pane) {
		if !p.isLeaf() {
			return
		}
		for _, t := range p.tabs {
			if t.view != nil && t.view.Doc != nil && t.view.Doc.ID() == doc.ID() {
				t.prev = append([]byte(nil), next...)
				t.seenVer = ver
				t.reqVer = -1
			}
		}
	})
	// publishEdits skips a document whose seenVer already matches, so the
	// gutter and an open diff would otherwise stay on the pre-format text.
	if s.bus != nil {
		s.bus.Post(bus.DocumentChanged{DocID: doc.ID(), Version: ver, Path: doc.Path()})
	}
}

// bumpFormatToken invalidates an in-flight format for id.
// A newer save or format request drops the older result.
func (s *shell) bumpFormatToken(id uint64) int64 {
	if s.formatToken == nil {
		s.formatToken = map[uint64]int64{}
	}
	s.formatToken[id]++
	return s.formatToken[id]
}
