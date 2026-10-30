package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/transfer"
	"gioui.org/layout"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/editor"
	"github.com/russoj88/gio-ide/internal/keymap"
	"github.com/russoj88/gio-ide/internal/syntax"
)

// pane is one leaf of editor tabs, or a split of two panes.
// down means the children are stacked. Otherwise they sit side by side.
type pane struct {
	tabs   []*edTab
	active int
	down   bool
	ratio  float32
	first  *pane
	second *pane
}

func (p *pane) isLeaf() bool { return p == nil || p.first == nil }

type edTab struct {
	view      *editor.View
	seenVer   int32
	prev      []byte
	reqVer    int32
	reqFirst  int
	reqLast   int
	bracketAt int
	tag       struct{}
	// preview renders markdown instead of the editor. Each pane has its own
	// flag, so two splits of one file can differ. The buffer is shared.
	preview bool
	// previewY is the preview scroll offset in pixels.
	previewY   int
	previewTag struct{ _ byte }
	previewBtn struct{ _ byte }
}

func (s *shell) bindEditor() {
	s.onCommand["file.save"] = s.cmdSave
	s.onCommand["file.saveAll"] = s.cmdSaveAll
	s.onCommand["code.reformat"] = s.cmdReformat
	s.onCommand["code.organizeImports"] = s.cmdOrganizeImports
	s.onCommand["file.close"] = s.cmdClose
	s.onCommand["edit.undo"] = func() {
		if v := s.activeView(); v != nil {
			v.Undo()
		}
	}
	s.onCommand["edit.redo"] = func() {
		if v := s.activeView(); v != nil {
			v.Redo()
		}
	}
	s.onCommand["edit.copy"] = s.cmdCopy
	s.onCommand["edit.cut"] = s.cmdCut
	s.onCommand["edit.paste"] = func() { s.pasteArmed = true }
	s.onCommand["edit.selectAll"] = func() {
		if v := s.activeView(); v != nil {
			v.SelectAll()
		}
	}
	s.onCommand["edit.highlight"] = func() { s.cmdHighlight(editor.MarkNote, false) }
	s.onCommand["edit.highlightLines"] = func() { s.cmdHighlight(editor.MarkNote, true) }
	s.onCommand["edit.highlightError"] = func() { s.cmdHighlight(editor.MarkError, false) }
	s.onCommand["edit.highlightSmell"] = func() { s.cmdHighlight(editor.MarkSmell, false) }
	s.onCommand["edit.clearHighlights"] = s.cmdClearHighlights
	s.onCommand["view.splitRight"] = func() { s.splitActive(false) }
	s.onCommand["view.splitDown"] = func() { s.splitActive(true) }
	s.onCommand["nav.nextTab"] = func() { s.cycleTab(1) }
	s.onCommand["nav.prevTab"] = func() { s.cycleTab(-1) }
	s.onCommand["edit.find"] = func() { s.openFind(false) }
	s.onCommand["edit.replace"] = func() { s.openFind(true) }
	s.onCommand["edit.findNext"] = func() { s.findMove(1) }
	s.onCommand["edit.findPrev"] = func() { s.findMove(-1) }
	act := func(fn func(*editor.View)) func() {
		return func() {
			if v := s.activeView(); v != nil {
				fn(v)
			}
		}
	}
	s.onCommand["edit.addNextOccurrence"] = act(func(v *editor.View) { v.AddNextOccurrence() })
	s.onCommand["edit.unselectOccurrence"] = act(func(v *editor.View) { v.UnselectOccurrence() })
	s.onCommand["edit.selectAllOccurrences"] = act(func(v *editor.View) { v.SelectAllOccurrences() })
	s.onCommand["edit.addCaret"] = act(func(v *editor.View) { v.AddCaretBelow() })
	s.onCommand["edit.duplicateLine"] = act(func(v *editor.View) { v.DuplicateLine() })
	s.onCommand["edit.deleteLine"] = act(func(v *editor.View) { v.DeleteLine() })
	s.onCommand["edit.toggleLineComment"] = act(func(v *editor.View) { v.ToggleLineComment() })
	s.onCommand["edit.fold"] = act(func(v *editor.View) { v.Fold() })
	s.onCommand["edit.unfold"] = act(func(v *editor.View) { v.Unfold() })
	s.onCommand["edit.foldAll"] = act(func(v *editor.View) { v.FoldAll() })
	s.onCommand["edit.unfoldAll"] = act(func(v *editor.View) { v.UnfoldAll() })
	s.bindScratch()
}

// cmdHighlight toggles one mark on the active document. Read-only buffers
// accept marks. The selection and the text stay as they are.
// handleKeys runs before layout, so this frame paints the new marks.
func (s *shell) cmdHighlight(kind editor.MarkKind, fullLines bool) {
	v := s.activeView()
	if v == nil || v.Doc == nil {
		return
	}
	start, end := v.SelectionBounds()
	m, ok := editor.SelectionMark(v.Doc.Snapshot(), buffer.Offset(start), buffer.Offset(end), kind, fullLines)
	if !ok {
		return
	}
	id := v.Doc.ID()
	if s.marks == nil {
		s.marks = map[uint64][]editor.Mark{}
	}
	s.marks[id] = editor.ToggleMark(s.marks[id], m)
}

func (s *shell) cmdClearHighlights() {
	v := s.activeView()
	if v == nil || v.Doc == nil {
		return
	}
	delete(s.marks, v.Doc.ID())
}

func (s *shell) inputTag() event.Tag {
	if s.popup != "" || s.focus != focusEditor {
		return &s.tag
	}
	if v := s.activeView(); v != nil {
		return v.Tag()
	}
	return &s.tag
}

func (s *shell) activeView() *editor.View {
	tab := s.activeTab()
	if tab == nil {
		return nil
	}
	return tab.view
}

func (s *shell) activeTab() *edTab {
	p := s.focusPane
	if p == nil || !p.isLeaf() || len(p.tabs) == 0 {
		p = firstLeaf(s.rootPane)
	}
	if p == nil || !p.isLeaf() || len(p.tabs) == 0 {
		return nil
	}
	if p.active < 0 || p.active >= len(p.tabs) {
		p.active = 0
	}
	return p.tabs[p.active]
}

func (s *shell) syncCaret() {
	v := s.activeView()
	if v == nil {
		s.line, s.col = 1, 1
		return
	}
	ln, col := v.CaretPos()
	s.line, s.col = ln+1, col+1
}

func (s *shell) openPath(path string) {
	s.clearGitDiff()
	abs, err := filepath.Abs(path)
	if err != nil {
		s.bus.Post(bus.Notification{Level: "error", Text: err.Error()})
		return
	}
	s.docsMu.Lock()
	if s.opening[abs] {
		s.docsMu.Unlock()
		return
	}
	for _, d := range s.docs {
		if d.Path() == abs {
			id := d.ID()
			s.docsMu.Unlock()
			s.showDoc(id)
			return
		}
	}
	s.opening[abs] = true
	s.docsMu.Unlock()
	go func() {
		doc, err := buffer.Open(abs)
		s.docsMu.Lock()
		delete(s.opening, abs)
		if err == nil {
			s.docs[doc.ID()] = doc
		}
		s.docsMu.Unlock()
		if err != nil {
			s.bus.Post(bus.Notification{Level: "error", Text: err.Error()})
			return
		}
		s.bus.Post(bus.DocumentOpened{DocID: doc.ID(), Path: doc.Path(), Version: doc.Version()})
	}()
}

func (s *shell) attachOpened(id uint64) {
	s.docsMu.Lock()
	doc := s.docs[id]
	s.docsMu.Unlock()
	if doc == nil {
		return
	}
	if p, i, ok := s.findDoc(id); ok {
		s.focusPane = p
		p.active = i
		s.focus = focusEditor
		return
	}
	p := s.leafForOpen()
	view := editor.NewView(doc)
	view.OnFocus = func() {
		s.focus = focusEditor
		s.focusPane = p
	}
	text := append([]byte(nil), doc.Snapshot().Bytes()...)
	p.tabs = append(p.tabs, &edTab{
		view:      view,
		seenVer:   doc.Version(),
		prev:      text,
		bracketAt: -1,
		preview:   isMarkdownPath(doc.Path()),
	})
	p.active = len(p.tabs) - 1
	s.focus = focusEditor
	s.focusPane = p
	if s.syn != nil {
		lang := syntax.Lang(doc.Path())
		if lang == "" {
			lang = "text"
		}
		s.syn.Open(doc.ID(), lang, doc.Snapshot())
	}
	s.intelOpen(doc)
	if s.pendingJumpOn && doc.Path() == s.pendingJump.Path {
		s.applyPendingJump(view)
	}
	if filepath.IsAbs(doc.Path()) {
		s.rememberRecent(doc.Path())
	}
}

func (s *shell) revealPath(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if s.pendingJumpOn && (s.pendingJump.Path == path || s.pendingJump.Path == abs) {
		s.pendingJump.Path = abs
	}
	s.docsMu.Lock()
	var id uint64
	for _, d := range s.docs {
		if d.Path() == abs {
			id = d.ID()
			break
		}
	}
	s.docsMu.Unlock()
	if id == 0 {
		s.openPath(abs)
		return
	}
	s.showDoc(id)
}

// showDoc focuses an open tab, or attaches a document that is loaded but has no tab.
func (s *shell) showDoc(id uint64) {
	if p, i, ok := s.findDoc(id); ok {
		s.focusPane = p
		p.active = i
		s.focus = focusEditor
		if s.pendingJumpOn && p.tabs[i].view.Doc.Path() == s.pendingJump.Path {
			s.applyPendingJump(p.tabs[i].view)
		}
		return
	}
	s.attachOpened(id)
}

func (s *shell) leafForOpen() *pane {
	p := s.focusPane
	if p == nil || !p.isLeaf() {
		p = firstLeaf(s.rootPane)
	}
	if p == nil {
		s.rootPane = &pane{ratio: 0.5}
		p = s.rootPane
	}
	s.focusPane = p
	return p
}

func (s *shell) findDoc(id uint64) (*pane, int, bool) {
	var found *pane
	idx := -1
	walkPanes(s.rootPane, func(p *pane) {
		if found != nil || !p.isLeaf() {
			return
		}
		for i, t := range p.tabs {
			if t.view.Doc.ID() == id {
				found = p
				idx = i
				return
			}
		}
	})
	return found, idx, found != nil
}

func (s *shell) docTabCount(id uint64) int {
	n := 0
	walkPanes(s.rootPane, func(p *pane) {
		if !p.isLeaf() {
			return
		}
		for _, t := range p.tabs {
			if t.view.Doc.ID() == id {
				n++
			}
		}
	})
	return n
}

func (s *shell) splitActive(down bool) {
	p := s.focusPane
	if p == nil || !p.isLeaf() || len(p.tabs) == 0 {
		s.notify("Open a file before splitting")
		return
	}
	if p.active < 0 || p.active >= len(p.tabs) {
		p.active = 0
	}
	tab := p.tabs[p.active]
	kept := &pane{tabs: p.tabs, active: p.active, ratio: 0.5}
	sibView := editor.NewView(tab.view.Doc)
	tab.view.CopyFoldsTo(sibView)
	sib := &pane{tabs: []*edTab{{
		view:      sibView,
		seenVer:   tab.view.Doc.Version(),
		prev:      append([]byte(nil), tab.prev...),
		bracketAt: -1,
		preview:   tab.preview,
		previewY:  tab.previewY,
	}}, ratio: 0.5}
	p.tabs = nil
	p.active = 0
	p.down = down
	p.ratio = 0.5
	p.first = kept
	p.second = sib
	sibView.OnFocus = func() {
		s.focus = focusEditor
		s.focusPane = sib
	}
	for _, t := range kept.tabs {
		t.view.OnFocus = func() {
			s.focus = focusEditor
			s.focusPane = kept
		}
	}
	s.focusPane = sib
	s.focus = focusEditor
}

func (s *shell) cycleTab(dir int) {
	p := s.focusPane
	if p == nil || !p.isLeaf() || len(p.tabs) == 0 {
		return
	}
	n := len(p.tabs)
	p.active = (p.active + dir + n) % n
	s.focus = focusEditor
}

func (s *shell) cmdClose() {
	p := s.focusPane
	if p == nil || !p.isLeaf() || len(p.tabs) == 0 {
		s.notify("No editor tab")
		return
	}
	if p.active < 0 || p.active >= len(p.tabs) {
		p.active = 0
	}
	tab := p.tabs[p.active]
	doc := tab.view.Doc
	last := s.docTabCount(doc.ID()) == 1
	// An empty scratch is disposable. Delete it instead of asking to save.
	if last && s.blankScratch(doc) {
		if err := os.Remove(doc.Path()); err != nil && !os.IsNotExist(err) {
			s.notify(err.Error())
			return
		}
		s.closeTab(p, p.active)
		return
	}
	if doc.Dirty() && last {
		s.closePane = p
		s.closeIndex = p.active
		s.closeAfter = 0
		s.listIndex = 0
		s.openPopup("unsaved")
		return
	}
	s.closeTab(p, p.active)
}

func (s *shell) answerUnsaved(idx int) {
	p := s.closePane
	if p == nil || !p.isLeaf() || s.closeIndex < 0 || s.closeIndex >= len(p.tabs) {
		return
	}
	tab := p.tabs[s.closeIndex]
	switch idx {
	case 0: // Save
		s.closeAfter = tab.view.Doc.ID()
		s.saveDoc(tab.view.Doc)
	case 1: // Don't Save
		s.closeTab(p, s.closeIndex)
	default:
	}
	s.closePane = nil
}

func (s *shell) closeTab(p *pane, idx int) {
	if p == nil || !p.isLeaf() || idx < 0 || idx >= len(p.tabs) {
		return
	}
	tab := p.tabs[idx]
	id := tab.view.Doc.ID()
	path := tab.view.Doc.Path()
	p.tabs = append(p.tabs[:idx], p.tabs[idx+1:]...)
	if p.active >= len(p.tabs) {
		p.active = len(p.tabs) - 1
	}
	if p.active < 0 {
		p.active = 0
	}
	if s.docTabCount(id) == 0 {
		s.docsMu.Lock()
		doc := s.docs[id]
		delete(s.docs, id)
		s.docsMu.Unlock()
		if doc != nil {
			doc.Close()
		}
		if s.syn != nil {
			s.syn.Close(id)
		}
		s.intelClose(id, path)
		delete(s.colors, id)
		delete(s.marks, id)
		s.bus.Post(bus.DocumentClosed{DocID: id, Path: path})
	}
	s.rootPane = collapse(s.rootPane)
	if s.rootPane == nil {
		s.rootPane = &pane{ratio: 0.5}
	}
	if s.focusPane == nil || !paneContains(s.rootPane, s.focusPane) || !s.focusPane.isLeaf() {
		s.focusPane = firstLeaf(s.rootPane)
	}
}

func (s *shell) cmdSave() {
	v := s.activeView()
	if v == nil {
		s.notify("Nothing to save")
		return
	}
	if v.Doc.ReadOnly() {
		s.notify(v.Doc.Notice())
		return
	}
	if !v.Doc.Dirty() {
		return
	}
	s.saveDoc(v.Doc)
}

func (s *shell) cmdSaveAll() {
	seen := map[uint64]bool{}
	n := 0
	walkPanes(s.rootPane, func(p *pane) {
		if !p.isLeaf() {
			return
		}
		for _, t := range p.tabs {
			d := t.view.Doc
			if seen[d.ID()] || !d.Dirty() || d.ReadOnly() {
				continue
			}
			seen[d.ID()] = true
			s.saveDoc(d)
			n++
		}
	})
	if n == 0 {
		s.notify("Nothing to save")
	}
}

func (s *shell) saveDoc(doc *buffer.Doc) {
	if doc == nil {
		return
	}
	if doc.ReadOnly() {
		s.notify(doc.Notice())
		return
	}
	fumpt, imps, fix := s.formatFlags()
	// A scratch is not in the module. go fix and goimports stay off.
	// gofumpt still runs when that setting is on and the file is .go.
	if s.inScratch(doc.Path()) {
		imps, fix = false, false
	}
	if isGoPath(doc.Path()) && (fumpt || imps || fix) {
		s.startFormat(doc, fumpt, imps, fix, true)
		return
	}
	s.writeDoc(doc)
}

func (s *shell) formatFlags() (gofumpt, goimports, gofix bool) {
	if s.store == nil {
		return false, false, false
	}
	cur := s.store.Current()
	return cur.Editor.GofumptOnSave, cur.Editor.GoimportsOnSave, cur.Editor.GofixOnSave
}

func (s *shell) writeDoc(doc *buffer.Doc) {
	path, data, version, err := doc.ExportForSave()
	if err != nil {
		s.notify(err.Error())
		return
	}
	id := doc.ID()
	if s.saveVersions == nil {
		s.saveVersions = map[uint64]int32{}
	}
	s.saveVersions[id] = version
	// A plain save must drop a format that was already running.
	s.bumpFormatToken(id)
	go s.commitFile(id, path, data)
}

func (s *shell) commitFile(id uint64, path string, data []byte) {
	if err := buffer.WriteAtomic(path, data); err != nil {
		s.bus.Post(bus.Notification{Level: "error", Text: err.Error()})
		return
	}
	s.bus.Post(bus.DocumentSaved{DocID: id, Path: path})
}

func (s *shell) flushClipboard(gtx layout.Context) {
	if s.clipWrite != "" {
		gtx.Execute(clipboard.WriteCmd{
			Type: "application/text",
			Data: io.NopCloser(strings.NewReader(s.clipWrite)),
		})
		s.clipWrite = ""
	}
	v := s.activeView()
	tag := event.Tag(nil)
	if s.pasteToTerm {
		tag = &s.termPaste
	} else if v != nil {
		tag = v.Tag()
	}
	if tag == nil {
		s.pasteArmed = false
		s.pasteWait = 0
		s.pasteToTerm = false
		return
	}
	if s.pasteArmed {
		gtx.Execute(clipboard.ReadCmd{Tag: tag})
		s.pasteArmed = false
		s.pasteWait = 3
	}
	if s.pasteWait <= 0 {
		return
	}
	s.pasteWait--
	for {
		ev, ok := gtx.Event(transfer.TargetFilter{Target: tag, Type: "application/text"})
		if !ok {
			break
		}
		de, ok := ev.(transfer.DataEvent)
		if !ok || de.Open == nil {
			continue
		}
		rc := de.Open()
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			s.notify(err.Error())
			continue
		}
		if s.pasteToTerm {
			s.deliverTermPaste(string(b))
			s.pasteToTerm = false
		} else if v != nil && !s.markdownPreview() {
			v.Paste(string(b))
		}
		s.pasteWait = 0
	}
}

func (s *shell) onSaved(id uint64) {
	s.docsMu.Lock()
	doc := s.docs[id]
	ver := s.saveVersions[id]
	s.docsMu.Unlock()
	if doc != nil {
		doc.MarkSaved(ver)
		s.intelSave(id, doc.Path())
		s.scheduleGitStatus()
		s.scheduleGutter(id)
	}
	if s.closeAfter != id {
		return
	}
	s.closeAfter = 0
	if p, i, ok := s.findDoc(id); ok {
		tab := p.tabs[i]
		if tab.view.Doc.Dirty() {
			return
		}
		s.closeTab(p, i)
	}
}

func (s *shell) publishEdits() {
	seen := map[uint64]bool{}
	walkPanes(s.rootPane, func(p *pane) {
		if !p.isLeaf() {
			return
		}
		for _, t := range p.tabs {
			d := t.view.Doc
			ver := d.Version()
			if ver == t.seenVer {
				continue
			}
			if !seen[d.ID()] {
				seen[d.ID()] = true
				newText := d.Snapshot().Bytes()
				ed := oneEdit(t.prev, newText)
				if ed.Start != ed.End || ed.Text != "" {
					if s.syn != nil {
						lang := syntax.Lang(d.Path())
						if lang == "" {
							lang = "text"
						}
						s.syn.Edit(d.ID(), ed, d.Snapshot())
						s.shiftPaint(d.ID(), ed)
					}
					old := append([]byte(nil), t.prev...)
					active := s.activeView()
					s.eachDocView(d.ID(), func(v *editor.View) {
						v.AdjustFolds(old, ed, v != active)
					})
					s.intelEdit(d, t.prev, newText, ver)
					if s.marks != nil {
						s.marks[d.ID()] = editor.ClipMarks(editor.ShiftMarks(t.prev, ed, s.marks[d.ID()]), len(newText))
					}
				}
				t.reqVer = -1
				s.bus.Post(bus.DocumentChanged{DocID: d.ID(), Version: ver, Path: d.Path()})
			}
			t.prev = append([]byte(nil), d.Snapshot().Bytes()...)
			t.seenVer = ver
		}
	})
}

func (s *shell) eachDocView(id uint64, fn func(*editor.View)) {
	walkPanes(s.rootPane, func(p *pane) {
		if !p.isLeaf() {
			return
		}
		for _, t := range p.tabs {
			if t.view != nil && t.view.Doc.ID() == id {
				fn(t.view)
			}
		}
	})
}

func (s *shell) cmdCopy() {
	v := s.activeView()
	if v == nil {
		return
	}
	text := v.SelectedText()
	if text == "" {
		return
	}
	s.clipWrite = text
}

func (s *shell) cmdCut() {
	if s.markdownPreview() {
		return
	}
	v := s.activeView()
	if v == nil {
		return
	}
	text := v.Cut()
	if text == "" {
		return
	}
	s.clipWrite = text
}

func (s *shell) editorKey(st keymap.Stroke) bool {
	v := s.activeView()
	if v == nil {
		return false
	}
	if st.Mod&(keymap.ModAlt|keymap.ModSuper) != 0 {
		return false
	}
	extend := st.Mod&keymap.ModShift != 0
	ctrl := st.Mod&keymap.ModCtrl != 0
	switch st.Key {
	case "left":
		v.MoveLeft(extend)
	case "right":
		v.MoveRight(extend)
	case "up":
		v.MoveUp(extend)
	case "down":
		v.MoveDown(extend)
	case "home":
		if ctrl {
			v.MoveDocStart(extend)
		} else {
			v.MoveLineStart(extend)
		}
	case "end":
		if ctrl {
			v.MoveDocEnd(extend)
		} else {
			v.MoveLineEnd(extend)
		}
	case "pageup":
		v.Page(-1, extend)
	case "pagedown":
		v.Page(1, extend)
	case "backspace":
		if ctrl || extend {
			return false
		}
		v.Backspace()
	case "delete":
		if ctrl || extend {
			return false
		}
		v.DeleteForward()
	case "enter":
		if ctrl || extend {
			return false
		}
		v.Type("\n")
	case "tab":
		if ctrl || extend {
			return false
		}
		v.Type(s.tabInsert(v))
	default:
		return false
	}
	return true
}

func (s *shell) tabInsert(v *editor.View) string {
	cur := s.store.Current()
	if !cur.Editor.InsertSpaces {
		return "\t"
	}
	width := cur.Editor.TabWidth
	if width < 1 {
		width = 4
	}
	_, col := v.CaretPos()
	n := width - col%width
	if n <= 0 {
		n = width
	}
	return strings.Repeat(" ", n)
}

func walkPanes(p *pane, fn func(*pane)) {
	if p == nil {
		return
	}
	fn(p)
	if p.first != nil {
		walkPanes(p.first, fn)
		walkPanes(p.second, fn)
	}
}

func firstLeaf(p *pane) *pane {
	if p == nil {
		return nil
	}
	if p.isLeaf() {
		return p
	}
	if leaf := firstLeaf(p.first); leaf != nil {
		return leaf
	}
	return firstLeaf(p.second)
}

func collapse(p *pane) *pane {
	if p == nil || p.isLeaf() {
		return p
	}
	p.first = collapse(p.first)
	p.second = collapse(p.second)
	fe := p.first == nil || (p.first.isLeaf() && len(p.first.tabs) == 0)
	se := p.second == nil || (p.second.isLeaf() && len(p.second.tabs) == 0)
	switch {
	case fe && se:
		return &pane{ratio: 0.5}
	case fe:
		return p.second
	case se:
		return p.first
	default:
		return p
	}
}

func paneContains(root, target *pane) bool {
	if root == nil || target == nil {
		return false
	}
	if root == target {
		return true
	}
	return paneContains(root.first, target) || paneContains(root.second, target)
}

// anyDirty reports whether an open document has unsaved edits.
func (s *shell) anyDirty() bool {
	dirty := false
	walkPanes(s.rootPane, func(p *pane) {
		if !p.isLeaf() {
			return
		}
		for _, t := range p.tabs {
			if t.view.Doc.Dirty() {
				dirty = true
			}
		}
	})
	return dirty
}
