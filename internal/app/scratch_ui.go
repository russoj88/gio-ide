package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/ui"
	"github.com/russoj88/gio-ide/internal/workspace"
)

// scratchTreePath is the project-tree row for the scratch folder.
// It is not a filesystem path, so a click must not join it onto the workspace root.
const scratchTreePath = "//scratches"

// scratchGoBody is the text of a new Go scratch: a blank line after package,
// then an empty main. The body is otherwise unindented.
const scratchGoBody = "package main\n\nfunc main() {\n}\n"

func (s *shell) bindScratch() {
	s.onCommand["file.newScratch"] = s.cmdNewScratch
	s.onCommand["file.newScratchBuffer"] = s.cmdNewScratchBuffer
	s.onCommand["file.newScratchFromSelection"] = s.cmdNewScratchFromSelection
	s.onCommand["file.renameScratch"] = s.cmdRenameScratch
	s.onCommand["file.moveScratch"] = s.cmdMoveScratch
}

// inScratch reports whether path is inside the scratch directory.
// An empty scratchDir, as in shell literals built by tests, is never a scratch.
func (s *shell) inScratch(path string) bool {
	if s.scratchDir == "" || path == "" {
		return false
	}
	return workspace.Contains(s.scratchDir, path)
}

func (s *shell) activeDoc() *buffer.Doc {
	v := s.activeView()
	if v == nil {
		return nil
	}
	return v.Doc
}

func (s *shell) cmdNewScratch() {
	path, err := workspace.Create(s.scratchDir, ".go", []byte(scratchGoBody))
	if err != nil {
		s.notify(err.Error())
		return
	}
	s.openPath(path)
}

func (s *shell) cmdNewScratchBuffer() {
	path, err := workspace.Create(s.scratchDir, ".txt", nil)
	if err != nil {
		s.notify(err.Error())
		return
	}
	s.openPath(path)
}

func (s *shell) cmdNewScratchFromSelection() {
	v := s.activeView()
	if v == nil || v.Doc == nil || v.SelectedText() == "" {
		s.notify("Select text to make a scratch file")
		return
	}
	ext := ".txt"
	if strings.HasSuffix(v.Doc.Path(), ".go") {
		ext = ".go"
	}
	path, err := workspace.Create(s.scratchDir, ext, []byte(v.SelectedText()))
	if err != nil {
		s.notify(err.Error())
		return
	}
	s.openPath(path)
}

func (s *shell) cmdRenameScratch() {
	doc := s.activeDoc()
	if doc == nil || !s.inScratch(doc.Path()) {
		s.notify("The current file is not a scratch")
		return
	}
	s.openNamedPopup("renameScratch", filepath.Base(doc.Path()))
}

func (s *shell) cmdMoveScratch() {
	doc := s.activeDoc()
	if doc == nil || !s.inScratch(doc.Path()) {
		s.notify("The current file is not a scratch")
		return
	}
	if s.root == "" {
		s.notify("Open a folder before moving a scratch")
		return
	}
	s.openNamedPopup("moveScratch", filepath.Base(doc.Path()))
}

func (s *shell) openNamedPopup(kind, field string) {
	s.openPopup(kind)
	s.field = field
	s.fieldCaret = len([]rune(field))
}

func (s *shell) renameScratch(base string) {
	doc := s.activeDoc()
	if doc == nil || !s.inScratch(doc.Path()) {
		s.notify("The current file is not a scratch")
		return
	}
	next, err := workspace.Rename(s.scratchDir, doc.Path(), strings.TrimSpace(base))
	if err != nil {
		s.notify(err.Error())
		return
	}
	doc.SetPath(next)
}

func (s *shell) moveScratch(raw string) {
	doc := s.activeDoc()
	if doc == nil || !s.inScratch(doc.Path()) {
		s.notify("The current file is not a scratch")
		return
	}
	if s.root == "" {
		s.notify("Open a folder before moving a scratch")
		return
	}
	dest, err := scratchMoveDest(s.root, raw)
	if err != nil {
		s.notify(err.Error())
		return
	}
	if _, err := os.Lstat(dest); err == nil {
		s.notify(dest + " already exists")
		return
	} else if !os.IsNotExist(err) {
		s.notify(err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		s.notify(err.Error())
		return
	}
	next, err := workspace.Move(doc.Path(), dest)
	if err != nil {
		s.notify(err.Error())
		return
	}
	doc.SetPath(next)
	if isGoPath(next) {
		s.intelOpen(doc)
	}
}

// scratchMoveDest cleans raw into a file under root.
// Empty, absolute, and paths whose relative form starts with ".." are refused.
func scratchMoveDest(root, raw string) (string, error) {
	rel := strings.TrimSpace(raw)
	if rel == "" {
		return "", errors.New("Enter a path inside the project")
	}
	if filepath.IsAbs(rel) {
		return "", errors.New("The destination must stay inside the project")
	}
	rel = filepath.Clean(rel)
	dest := filepath.Join(root, rel)
	got, err := filepath.Rel(root, dest)
	if err != nil || got == ".." || strings.HasPrefix(got, ".."+string(filepath.Separator)) {
		return "", errors.New("The destination must stay inside the project")
	}
	return dest, nil
}

// blankScratch reports whether doc is a scratch whose text is empty or whitespace.
func (s *shell) blankScratch(doc *buffer.Doc) bool {
	if doc == nil || !s.inScratch(doc.Path()) {
		return false
	}
	return strings.TrimSpace(string(doc.Snapshot().Bytes())) == ""
}

// scratchNodes is the Scratches folder and, when it is expanded, its files.
// A list error leaves the folder with no children and does not notify.
func (s *shell) scratchNodes() []ui.TreeNode {
	if s.scratchDir == "" {
		return nil
	}
	if s.openDirs == nil {
		s.openDirs = map[string]bool{}
	}
	open := s.openDirs[scratchTreePath]
	out := []ui.TreeNode{{
		Label: "Scratches",
		Dir:   true,
		Open:  open,
		Path:  scratchTreePath,
		Tag:   s.tagFor("tree:" + scratchTreePath),
	}}
	if !open {
		return out
	}
	paths, err := workspace.List(s.scratchDir)
	if err != nil {
		return out
	}
	active := ""
	if tab := s.activeTab(); tab != nil && tab.view != nil && tab.view.Doc != nil {
		active = tab.view.Doc.Path()
	}
	for _, p := range paths {
		out = append(out, ui.TreeNode{
			Label:    filepath.Base(p),
			Depth:    1,
			Path:     p,
			Tag:      s.tagFor("tree:" + p),
			Selected: p == active,
		})
	}
	return out
}

// clickScratch toggles the scratch folder or opens a scratch file.
// path is the sentinel or an absolute scratch path, never a project-relative path.
func (s *shell) clickScratch(path string) bool {
	if path == scratchTreePath {
		return s.toggleTreeDir(path)
	}
	s.openPath(path)
	s.focus = focusEditor
	return false
}
