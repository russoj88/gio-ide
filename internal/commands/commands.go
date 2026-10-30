// Package commands is the command registry and the Find Action catalog.
package commands

import "sync"

// Command is one action the keymap or Find Action can run.
type Command struct {
	ID    string
	Title string
	Phase string // empty for v1, "2" or "3" for stubs
	Run   func()
}

// Registry maps command IDs to handlers.
type Registry struct {
	mu   sync.Mutex
	cmds map[string]Command
}

func New() *Registry {
	return &Registry{cmds: map[string]Command{}}
}

func (r *Registry) Register(c Command) {
	r.mu.Lock()
	r.cmds[c.ID] = c
	r.mu.Unlock()
}

func (r *Registry) Execute(id string) bool {
	r.mu.Lock()
	c, ok := r.cmds[id]
	r.mu.Unlock()
	if !ok || c.Run == nil {
		return false
	}
	c.Run()
	return true
}

func (r *Registry) Get(id string) (Command, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.cmds[id]
	return c, ok
}

// All returns the commands in catalog order.
func (r *Registry) All() []Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Command, 0, len(Catalog))
	for _, c := range Catalog {
		if got, ok := r.cmds[c.ID]; ok {
			out = append(out, got)
		} else {
			out = append(out, c)
		}
	}
	return out
}

func (r *Registry) IDs() map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := map[string]bool{}
	for id := range r.cmds {
		m[id] = true
	}
	return m
}

// Catalog is every command ID v1 registers, including later-phase stubs.
var Catalog = []Command{
	{ID: "file.save", Title: "Save"},
	{ID: "file.saveAll", Title: "Save All"},
	{ID: "file.close", Title: "Close"},
	{ID: "file.newScratch", Title: "New Scratch File"},
	{ID: "file.newScratchBuffer", Title: "New Scratch Buffer"},
	{ID: "file.newScratchFromSelection", Title: "New Scratch File from Selection"},
	{ID: "file.renameScratch", Title: "Rename Scratch"},
	{ID: "file.moveScratch", Title: "Move Scratch to Project"},
	{ID: "nav.goToFile", Title: "Go to File"},
	{ID: "nav.goToSymbol", Title: "Go to Symbol"},
	{ID: "nav.fileStructure", Title: "File Structure"},
	{ID: "nav.goToDeclaration", Title: "Go to Declaration"},
	{ID: "nav.findUsages", Title: "Find Usages"},
	{ID: "nav.back", Title: "Back"},
	{ID: "nav.forward", Title: "Forward"},
	{ID: "nav.goToLine", Title: "Go to Line"},
	{ID: "nav.recentFiles", Title: "Recent Files"},
	{ID: "nav.nextError", Title: "Next Error"},
	{ID: "nav.prevError", Title: "Previous Error"},
	{ID: "nav.nextTab", Title: "Next Tab"},
	{ID: "nav.prevTab", Title: "Previous Tab"},
	{ID: "edit.find", Title: "Find"},
	{ID: "edit.replace", Title: "Replace"},
	{ID: "edit.findNext", Title: "Find Next"},
	{ID: "edit.findPrev", Title: "Find Previous"},
	{ID: "search.inFiles", Title: "Search in Files"},
	{ID: "search.replaceInFiles", Title: "Replace in Files"},
	{ID: "edit.completion", Title: "Completion"},
	{ID: "edit.quickDoc", Title: "Quick Documentation"},
	{ID: "edit.addNextOccurrence", Title: "Add Next Occurrence"},
	{ID: "edit.unselectOccurrence", Title: "Unselect Occurrence"},
	{ID: "edit.selectAllOccurrences", Title: "Select All Occurrences"},
	{ID: "edit.addCaret", Title: "Add Caret"},
	{ID: "edit.duplicateLine", Title: "Duplicate Line"},
	{ID: "edit.deleteLine", Title: "Delete Line"},
	{ID: "edit.toggleLineComment", Title: "Toggle Line Comment"},
	{ID: "edit.fold", Title: "Fold"},
	{ID: "edit.unfold", Title: "Unfold"},
	{ID: "edit.foldAll", Title: "Fold All"},
	{ID: "edit.unfoldAll", Title: "Unfold All"},
	{ID: "edit.undo", Title: "Undo"},
	{ID: "edit.redo", Title: "Redo"},
	{ID: "edit.copy", Title: "Copy"},
	{ID: "edit.cut", Title: "Cut"},
	{ID: "edit.paste", Title: "Paste"},
	{ID: "edit.selectAll", Title: "Select All"},
	{ID: "edit.highlight", Title: "Highlight"},
	{ID: "edit.highlightLines", Title: "Highlight Lines"},
	{ID: "edit.highlightError", Title: "Highlight Error"},
	{ID: "edit.highlightSmell", Title: "Highlight Smell"},
	{ID: "edit.clearHighlights", Title: "Clear Highlights"},
	{ID: "view.splitRight", Title: "Split Right"},
	{ID: "view.splitDown", Title: "Split Down"},
	{ID: "view.projectTool", Title: "Project"},
	{ID: "view.problemsTool", Title: "Problems"},
	{ID: "view.gitTool", Title: "Git"},
	{ID: "view.terminal", Title: "Terminal"},
	{ID: "git.commit", Title: "Commit"},
	{ID: "git.branches", Title: "Branches"},
	{ID: "git.refreshStatus", Title: "Refresh Git Status"},
	{ID: "intel.restart", Title: "Restart Code Intelligence"},
	{ID: "app.findAction", Title: "Find Action"},
	{ID: "app.settings", Title: "Settings"},
	{ID: "app.keymap", Title: "Keymap"},
	{ID: "app.about", Title: "About"},
	{ID: "view.toggleTheme", Title: "Toggle Theme"},
	// Implied by the terminal focus rule (copy and paste while the terminal is focused).
	{ID: "terminal.copy", Title: "Terminal Copy"},
	{ID: "terminal.paste", Title: "Terminal Paste"},
	{ID: "terminal.new", Title: "New Terminal"},
	{ID: "terminal.close", Title: "Close Terminal"},

	{ID: "debug.launch", Title: "Debug: Start", Phase: "2"},
	{ID: "debug.continue", Title: "Debug: Continue", Phase: "2"},
	{ID: "debug.next", Title: "Debug: Step Over", Phase: "2"},
	{ID: "debug.stepIn", Title: "Debug: Step Into", Phase: "2"},
	{ID: "debug.stepOut", Title: "Debug: Step Out", Phase: "2"},
	{ID: "debug.pause", Title: "Debug: Pause", Phase: "2"},
	{ID: "debug.stop", Title: "Debug: Stop", Phase: "2"},

	{ID: "refactor.rename", Title: "Rename", Phase: "3"},
	{ID: "refactor.extract", Title: "Extract", Phase: "3"},
	{ID: "refactor.codeAction", Title: "Code Action", Phase: "3"},
	{ID: "code.reformat", Title: "Reformat Code"},
	{ID: "code.organizeImports", Title: "Optimize Imports"},

	{ID: "run.run", Title: "Run", Phase: "3"},
	{ID: "run.test", Title: "Run Tests", Phase: "3"},
	{ID: "test.run", Title: "Test", Phase: "3"},
	{ID: "test.debug", Title: "Debug Test", Phase: "3"},
}

// IsStub reports whether id is reserved for a later phase.
func IsStub(id string) bool {
	for _, c := range Catalog {
		if c.ID == id {
			return c.Phase != ""
		}
	}
	return false
}
