package app

import (
	"os/exec"
	"path/filepath"

	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/commands"
	"github.com/russoj88/gio-ide/internal/settings"
	"github.com/russoj88/gio-ide/internal/xdg"
)

func (s *shell) bindCommands() {
	for _, c := range commands.Catalog {
		s.cmds.Register(c)
		if c.Phase != "" {
			s.onCommand[c.ID] = func() {
				s.notify(c.Title + " is not available in v1")
			}
		}
	}
	s.onCommand["app.findAction"] = func() {
		if s.popup == "action" {
			s.closePopup()
			return
		}
		s.openPopup("action")
	}
	s.onCommand["view.toggleTheme"] = func() {
		next := "light"
		if s.themeName == "light" {
			next = "dark"
		}
		s.bus.Post(bus.ThemeChanged{Name: next})
	}
	s.onCommand["app.settings"] = func() { s.openPopup("settings") }
	s.onCommand["app.keymap"] = func() { s.openConfigFile(xdg.KeymapPath(), false) }
	s.onCommand["app.about"] = func() { s.openPopup("about") }
	s.onCommand["view.projectTool"] = func() { s.showTool("project") }
	s.onCommand["view.gitTool"] = func() { s.showTool("git") }
	s.onCommand["view.problemsTool"] = func() {
		s.bottomOpen = true
		s.bottomTool = "problems"
		s.leftOpen = true
		s.leftTool = "problems"
	}
	s.onCommand["view.terminal"] = func() {
		if s.bottomOpen && s.bottomTool == "terminal" && s.focus == focusTerminal {
			s.bottomOpen = false
			s.focus = focusEditor
			return
		}
		s.bottomOpen = true
		s.bottomTool = "terminal"
		s.focus = focusTerminal
		s.ensureTerminal()
	}
	s.onCommand["view.splitRight"] = func() { s.notify("Split right is available once a file is open") }
	s.onCommand["view.splitDown"] = func() { s.notify("Split down is available once a file is open") }
	s.onCommand["file.save"] = func() { s.notify("Nothing to save") }
	s.onCommand["file.saveAll"] = func() { s.notify("Nothing to save") }
	s.onCommand["file.close"] = func() { s.notify("No editor tab") }
	s.onCommand["nav.goToFile"] = func() { s.openPopup("goto") }
	s.onCommand["nav.goToSymbol"] = func() { s.openPopup("symbol") }
	s.onCommand["nav.fileStructure"] = func() { s.openPopup("structure") }
	s.onCommand["nav.goToLine"] = func() { s.openPopup("line") }
	s.onCommand["nav.recentFiles"] = func() { s.openPopup("recent") }
	s.onCommand["search.inFiles"] = func() {
		s.bottomOpen = true
		s.bottomTool = "search"
		s.openPopup("search")
	}
	s.onCommand["search.replaceInFiles"] = func() {
		s.bottomOpen = true
		s.bottomTool = "search"
		s.openPopup("replaceFiles")
	}
	s.onCommand["edit.find"] = func() { s.openPopup("find") }
	s.onCommand["edit.replace"] = func() { s.openPopup("replace") }
	s.onCommand["git.commit"] = func() { s.openPopup("commit") }
	s.onCommand["git.branches"] = func() { s.openPopup("branches") }
	s.onCommand["git.refreshStatus"] = func() { s.scheduleGitStatus() }
	s.onCommand["intel.restart"] = func() { s.notify("Code intelligence is not running yet") }
	stub := func(name string) func() {
		return func() { s.notify(name + " is ready once a file is open") }
	}
	for _, id := range []string{
		"nav.goToDeclaration", "nav.findUsages", "nav.back", "nav.forward",
		"nav.nextError", "nav.prevError", "nav.nextTab", "nav.prevTab",
		"edit.findNext", "edit.findPrev", "edit.completion", "edit.quickDoc",
		"edit.addNextOccurrence", "edit.unselectOccurrence", "edit.selectAllOccurrences",
		"edit.addCaret", "edit.duplicateLine", "edit.deleteLine", "edit.toggleLineComment",
		"edit.fold", "edit.unfold", "edit.foldAll", "edit.unfoldAll",
		"edit.undo", "edit.redo", "edit.copy", "edit.cut", "edit.paste", "edit.selectAll",
		"terminal.copy", "terminal.paste", "terminal.new", "terminal.close",
	} {
		if _, ok := s.onCommand[id]; !ok {
			s.onCommand[id] = stub(id)
		}
	}
	s.bindEditor()
}

func (s *shell) showTool(name string) {
	if s.leftOpen && s.leftTool == name {
		s.leftOpen = false
		if s.focus == focusTree {
			s.focus = focusEditor
		}
		return
	}
	s.leftOpen = true
	s.leftTool = name
	s.focus = focusTree
}

func (s *shell) openConfigFile(path string, settingsFile bool) {
	if settingsFile {
		if err := settings.WriteDefault(path); err != nil {
			s.notify("Could not write settings: " + err.Error())
			return
		}
	} else if err := xdg.EnsureDir(filepath.Dir(path)); err != nil {
		s.notify(err.Error())
		return
	}
	s.notify("Opening " + path)
	go func() {
		// Run waits so the process is reaped. Start without Wait leaves a zombie.
		cmd := exec.Command("xdg-open", path)
		if err := cmd.Run(); err != nil {
			s.bus.Post(bus.Notification{Level: "info", Text: "Open " + path + " in an editor"})
		}
	}()
}
