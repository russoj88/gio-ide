package app

import (
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/intel"
	"github.com/russoj88/gio-ide/internal/settings"
)

func (s *shell) startIntel(cur settings.Settings) {
	if s.root == "" || !cur.Intel.Enabled {
		s.intel = "off"
		return
	}
	to := intel.DefaultTimeouts()
	if cur.Intel.CompletionTimeoutMS > 0 {
		to.Completion = time.Duration(cur.Intel.CompletionTimeoutMS) * time.Millisecond
	}
	if cur.Intel.HoverTimeoutMS > 0 {
		to.Hover = time.Duration(cur.Intel.HoverTimeoutMS) * time.Millisecond
	}
	if cur.Intel.DefinitionTimeoutMS > 0 {
		to.Definition = time.Duration(cur.Intel.DefinitionTimeoutMS) * time.Millisecond
	}
	if cur.Intel.ReferencesTimeoutMS > 0 {
		to.References = time.Duration(cur.Intel.ReferencesTimeoutMS) * time.Millisecond
	}
	if cur.Intel.SymbolsTimeoutMS > 0 {
		to.Symbols = time.Duration(cur.Intel.SymbolsTimeoutMS) * time.Millisecond
	}
	s.ih = intel.New(intel.Config{
		Post:     s.bus.Post,
		Timeouts: to,
		Gopls:    cur.GoplsConfig(),
		Logf:     func(msg string) { log.Printf("intel: %s", msg) },
	})
	s.diags = map[string][]bus.Diagnostic{}
	s.intel = "Starting"
	if err := s.ih.Start(s.root); err != nil {
		s.notice = err.Error()
	}
	go func() {
		if _, err := exec.LookPath("go"); err != nil {
			s.bus.Post(bus.Notification{Level: "warn", Text: "The go command is not on PATH. Editing still works. Code intelligence needs Go."})
		}
	}()
	s.onCommand["intel.restart"] = func() {
		if s.ih != nil {
			_ = s.ih.Restart()
		}
	}
	s.onCommand["edit.completion"] = s.cmdCompletion
	s.onCommand["edit.quickDoc"] = s.cmdHover
	s.onCommand["nav.goToDeclaration"] = s.cmdDefinition
	s.onCommand["nav.findUsages"] = s.cmdReferences
	s.onCommand["nav.back"] = func() { s.navJump(false) }
	s.onCommand["nav.forward"] = func() { s.navJump(true) }
	s.onCommand["nav.goToSymbol"] = func() {
		s.openPopup("symbol")
		if s.ih != nil {
			s.ih.WorkspaceSymbols("")
		}
	}
	s.onCommand["nav.fileStructure"] = func() {
		s.openPopup("structure")
		s.cmdDocSymbols()
	}
}

func (s *shell) shutdownIntel() {
	if s.ih != nil {
		_ = s.ih.Shutdown()
	}
}

// useIntel reports whether path is sent to gopls.
// A scratch is highlighted and edited locally until it moves into the project.
func (s *shell) useIntel(path string) bool {
	return isGoPath(path) && !s.inScratch(path)
}

func (s *shell) intelOpen(doc *buffer.Doc) {
	if s.ih == nil || doc == nil || !s.useIntel(doc.Path()) {
		return
	}
	s.ih.DidOpen(doc)
}

func (s *shell) intelEdit(doc *buffer.Doc, prev, next []byte, version int32) {
	if s.ih == nil || doc == nil || !s.useIntel(doc.Path()) {
		return
	}
	ed := oneEdit(prev, next)
	s.ih.DidChange(doc.ID(), version, []buffer.Edit{ed}, doc.Snapshot())
}

func (s *shell) intelSave(id uint64, path string) {
	if s.ih != nil && s.useIntel(path) {
		s.ih.DidSave(id)
	}
}

func (s *shell) intelClose(id uint64, path string) {
	if s.ih != nil && s.useIntel(path) {
		s.ih.DidClose(id)
	}
}

func (s *shell) cmdCompletion() {
	v := s.activeView()
	if v == nil || s.ih == nil {
		return
	}
	s.ih.Completion(v.Doc.ID(), v.Doc.Version(), v.CaretOffset())
	s.openPopup("completion")
}

func (s *shell) cmdHover() {
	v := s.activeView()
	if v == nil || s.ih == nil {
		return
	}
	s.ih.Hover(v.Doc.ID(), v.Doc.Version(), v.CaretOffset())
}

func (s *shell) cmdDefinition() {
	v := s.activeView()
	if v == nil || s.ih == nil {
		return
	}
	s.ih.Definition(v.Doc.ID(), v.Doc.Version(), v.CaretOffset())
}

func (s *shell) cmdReferences() {
	v := s.activeView()
	if v == nil || s.ih == nil {
		return
	}
	s.openPopup("usages")
	s.ih.References(v.Doc.ID(), v.Doc.Version(), v.CaretOffset())
}

func (s *shell) cmdDocSymbols() {
	v := s.activeView()
	if v == nil || s.ih == nil {
		return
	}
	s.ih.DocumentSymbols(v.Doc.ID(), v.Doc.Version())
}

func (s *shell) onDiagnostics(ev bus.DiagnosticsUpdated) {
	if s.diags == nil {
		s.diags = map[string][]bus.Diagnostic{}
	}
	s.diags[ev.URI] = ev.Items
	s.rebuildProblems()
}

func (s *shell) rebuildProblems() {
	s.problems = s.problems[:0]
	s.diagErr, s.diagWarn = 0, 0
	for uri, items := range s.diags {
		base := filepath.Base(uri)
		for _, it := range items {
			if it.Severity <= 1 {
				s.diagErr++
			} else {
				s.diagWarn++
			}
			s.problems = append(s.problems, base+": "+it.Message)
		}
	}
	for _, p := range s.store.Current().Problems {
		s.problems = append(s.problems, p.Key+": "+p.Msg)
	}
}

func (s *shell) onCompletion(ev bus.CompletionReady) {
	if ev.Err != "" {
		s.notice = ev.Err
		return
	}
	s.compItems = ev.Items
	if s.popup != "completion" {
		s.openPopup("completion")
	}
}

func (s *shell) onHover(ev bus.HoverReady) {
	if ev.Err != "" {
		s.notice = ev.Err
		return
	}
	text := ev.Markdown
	if i := strings.Index(text, "\n"); i >= 0 {
		text = text[:i]
	}
	if text == "" {
		text = "No documentation"
	}
	s.notice = text
}

func (s *shell) onLocations(ev bus.LocationsReady) {
	if ev.Err != "" {
		s.notice = ev.Err
		return
	}
	s.locHits = ev.Locations
	if ev.Kind == "definition" && len(ev.Locations) > 0 {
		s.jumpTo(ev.Locations[0])
		return
	}
	if s.popup != "usages" && s.popup != "symbol" && s.popup != "structure" {
		s.openPopup("usages")
	}
}

func (s *shell) onSymbols(ev bus.SymbolsReady) {
	if ev.Err != "" {
		s.notice = ev.Err
		return
	}
	s.symHits = ev.Symbols
}

func (s *shell) jumpTo(loc bus.Location) {
	s.pushNav(false)
	// A new jump drops locations that a later forward step could still reach.
	s.navFwd = nil
	s.pendingJump = loc
	s.pendingJumpOn = true
	if tab := s.tabByPath(loc.Path); tab != nil {
		s.applyPendingJump(tab.view)
		return
	}
	s.openPath(loc.Path)
}

func (s *shell) applyPendingJump(v interface {
	Click(line, col int, extend bool)
},
) {
	if !s.pendingJumpOn {
		return
	}
	line, col := s.pendingJump.Line-1, s.pendingJump.Col-1
	if line < 0 {
		line = 0
	}
	if col < 0 {
		col = 0
	}
	v.Click(line, col, false)
	s.pendingJumpOn = false
}

func (s *shell) pushNav(forward bool) {
	v := s.activeView()
	if v == nil {
		return
	}
	ln, col := v.CaretPos()
	loc := bus.Location{Path: v.Doc.Path(), Line: ln + 1, Col: col + 1}
	if forward {
		s.navFwd = append(s.navFwd, loc)
		return
	}
	s.navBack = append(s.navBack, loc)
}

func (s *shell) navJump(forward bool) {
	var loc bus.Location
	if forward {
		if len(s.navFwd) == 0 {
			return
		}
		loc = s.navFwd[len(s.navFwd)-1]
		s.navFwd = s.navFwd[:len(s.navFwd)-1]
		s.pushNav(false)
	} else {
		if len(s.navBack) == 0 {
			return
		}
		loc = s.navBack[len(s.navBack)-1]
		s.navBack = s.navBack[:len(s.navBack)-1]
		s.pushNav(true)
	}
	s.pendingJump = loc
	s.pendingJumpOn = true
	if tab := s.tabByPath(loc.Path); tab != nil {
		s.applyPendingJump(tab.view)
		return
	}
	s.openPath(loc.Path)
}

func (s *shell) tabByPath(path string) *edTab {
	var found *edTab
	walkPanes(s.rootPane, func(p *pane) {
		if !p.isLeaf() {
			return
		}
		for _, t := range p.tabs {
			if t.view.Doc.Path() == path {
				found = t
				s.focusPane = p
			}
		}
	})
	return found
}

func (s *shell) acceptCompletion() {
	if s.listIndex < 0 || s.listIndex >= len(s.compItems) {
		s.closePopup()
		return
	}
	item := s.compItems[s.listIndex]
	text := item.Insert
	if text == "" {
		text = item.Label
	}
	if item.Snippet {
		text = plainSnippet(text)
	}
	s.closePopup()
	if v := s.activeView(); v != nil {
		v.Insert(text, buffer.OriginCommand)
	}
}

func plainSnippet(s string) string {
	s = strings.ReplaceAll(s, "$0", "")
	s = strings.ReplaceAll(s, "${0}", "")
	for {
		i := strings.Index(s, "${")
		if i < 0 {
			break
		}
		j := strings.Index(s[i:], "}")
		if j < 0 {
			break
		}
		inner := s[i+2 : i+j]
		repl := inner
		if _, after, ok := strings.Cut(inner, ":"); ok {
			repl = after
		}
		s = s[:i] + repl + s[i+j+1:]
	}
	return s
}

func (s *shell) acceptLocation(i int) {
	var loc bus.Location
	switch s.popup {
	case "usages":
		if i < 0 || i >= len(s.locHits) {
			return
		}
		loc = s.locHits[i]
	case "symbol", "structure":
		if i < 0 || i >= len(s.symHits) {
			return
		}
		sy := s.symHits[i]
		loc = bus.Location{Path: sy.Path, Line: sy.Line, Col: sy.Col}
	default:
		return
	}
	s.closePopup()
	s.jumpTo(loc)
}
