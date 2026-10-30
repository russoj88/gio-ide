package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/search"
	"github.com/russoj88/gio-ide/internal/ui"
	"github.com/russoj88/gio-ide/internal/workspace"
	"github.com/russoj88/gio-ide/internal/xdg"
)

func (s *shell) loadWorkspace() {
	if s.root == "" {
		return
	}
	ex := s.store.Current().Files.Exclude
	ix, err := workspace.Open(s.root, ex)
	if err != nil {
		s.bus.Post(bus.Notification{Level: "error", Text: err.Error()})
		return
	}
	s.wsMu.Lock()
	s.index = ix
	s.wsMu.Unlock()
	s.bus.Post(bus.FileListUpdated{Root: s.root, Count: len(ix.Files())})
	s.openGit()
	if !s.store.Current().Files.Watch {
		return
	}
	_ = workspace.Watch(s.root, s.stop, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := ix.Refresh(ctx); err != nil {
			return
		}
		s.bus.Post(bus.FileListUpdated{Root: s.root, Count: len(ix.Files())})
		s.bus.Post(bus.FileSystemChanged{})
	}, func() {
		s.watchLimited = true
		s.bus.Post(bus.Notification{Level: "warn", Text: "File watching stopped (fs.inotify.max_user_watches). The tree rescans when the window is focused."})
	})
}

func (s *shell) noteFileList() {
	s.wsMu.Lock()
	ix := s.index
	s.wsMu.Unlock()
	if ix == nil {
		return
	}
	s.files = ix.Files()
	s.tree = ix.Tree()
	if s.popup == "goto" {
		s.updateGoto()
	}
}

func (s *shell) updateGoto() {
	s.wsMu.Lock()
	ix := s.index
	s.wsMu.Unlock()
	s.gotoHits = nil
	if ix == nil {
		return
	}
	for _, m := range ix.Match(context.Background(), s.field, 50) {
		s.gotoHits = append(s.gotoHits, m.Path)
	}
}

func (s *shell) treeNodes() []ui.TreeNode {
	if s.tree == nil && s.scratchDir == "" {
		return nil
	}
	if s.openDirs == nil {
		s.openDirs = map[string]bool{}
	}
	// open is the open file, slash-relative to the workspace. Empty selects nothing.
	open := ""
	if tab := s.activeTab(); tab != nil && tab.view != nil && tab.view.Doc != nil && s.root != "" {
		rel, err := filepath.Rel(s.root, tab.view.Doc.Path())
		if err == nil {
			open = filepath.ToSlash(rel)
		}
	}
	var out []ui.TreeNode
	if s.tree != nil {
		var walk func(n *workspace.Node, depth int)
		walk = func(n *workspace.Node, depth int) {
			for _, k := range n.Kids {
				out = append(out, ui.TreeNode{
					Label:    k.Name,
					Depth:    depth,
					Dir:      k.Dir,
					Open:     s.openDirs[k.Path],
					Path:     k.Path,
					Tag:      s.tagFor("tree:" + k.Path),
					Selected: !k.Dir && open != "" && k.Path == open,
				})
				if k.Dir && s.openDirs[k.Path] {
					walk(k, depth+1)
				}
			}
		}
		walk(s.tree, 0)
	}
	return append(out, s.scratchNodes()...)
}

// treeClickGap collapses a double-click into the first toggle. A second press
// in this window used to undo the open before the next frame could draw it.
const treeClickGap = 300 * time.Millisecond

// clickTree opens a file or toggles a directory. It reports whether a
// directory's open state changed. A second press on the same path inside
// treeClickGap does not change it.
func (s *shell) clickTree(path string) bool {
	if path == "" {
		return false
	}
	// The scratch folder and its files are not nodes under s.root.
	if path == scratchTreePath || (filepath.IsAbs(path) && s.inScratch(path)) {
		return s.clickScratch(path)
	}
	if s.tree == nil {
		return false
	}
	n := findNode(s.tree, path)
	if n == nil {
		return false
	}
	if n.Dir {
		return s.toggleTreeDir(path)
	}
	s.openPath(filepath.Join(s.root, filepath.FromSlash(path)))
	s.focus = focusEditor
	return false
}

// toggleTreeDir flips a directory row. A second press inside treeClickGap
// does not change it. True means the open state changed and the frame should repaint.
func (s *shell) toggleTreeDir(path string) bool {
	if s.openDirs == nil {
		s.openDirs = map[string]bool{}
	}
	now := time.Now()
	if path == s.treeClickPath && now.Sub(s.treeClickAt) < treeClickGap {
		s.treeClickAt = now
		s.focus = focusTree
		s.invalidate()
		return false
	}
	s.treeClickPath = path
	s.treeClickAt = now
	s.openDirs[path] = !s.openDirs[path]
	s.focus = focusTree
	s.invalidate()
	return true
}

func (s *shell) invalidate() {
	if s.window != nil {
		s.window.Invalidate()
	}
}

func findNode(n *workspace.Node, path string) *workspace.Node {
	if n == nil {
		return nil
	}
	if n.Path == path && n.Name != "" {
		return n
	}
	for _, k := range n.Kids {
		if k.Path == path {
			return k
		}
		if strings.HasPrefix(path, k.Path+"/") {
			if got := findNode(k, path); got != nil {
				return got
			}
		}
	}
	return nil
}

func (s *shell) rememberRecent(path string) {
	r, err := workspace.PushRecent(xdg.RecentPath(), path)
	if err != nil {
		return
	}
	s.recent = r.Paths
}

func (s *shell) loadRecent() {
	r, err := workspace.LoadRecent(xdg.RecentPath())
	if err != nil {
		return
	}
	s.recent = r.Paths
}

func (s *shell) runProjectSearch(replace bool) {
	if s.root == "" {
		s.notify("Open a folder before searching")
		return
	}
	pattern := s.field
	repl := ""
	if i := strings.IndexByte(pattern, '\t'); i >= 0 {
		repl = pattern[i+1:]
		pattern = pattern[:i]
	}
	if pattern == "" {
		return
	}
	s.searchHits = nil
	s.searchErr = ""
	s.bottomOpen = true
	s.bottomTool = "search"
	files := append([]string(nil), s.files...)
	root := s.root
	cur := s.store.Current()
	q := search.Query{
		Pattern: pattern,
		Exclude: cur.Search.Exclude,
	}
	var eng search.Engine = search.Builtin{}
	if cur.Search.Engine == "rg" {
		eng = search.RG{}
	}
	go func() {
		err := eng.Search(context.Background(), root, files, q, func(batch []search.Result) {
			hits := make([]bus.SearchHit, len(batch))
			for i, h := range batch {
				hits[i] = bus.SearchHit{
					Path: h.Path, Line: h.Line, Col: h.Col, LineText: h.LineText,
					MatchStart: h.MatchStart, MatchEnd: h.MatchEnd,
				}
			}
			s.bus.Post(bus.SearchResultsBatch{Results: hits})
		})
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		s.bus.Post(bus.SearchDone{Err: msg})
		if replace && err == nil {
			s.replaceInFiles(root, files, pattern, repl)
		}
	}()
}

func (s *shell) replaceInFiles(root string, files []string, pattern, repl string) {
	if pattern == "" {
		return
	}
	root = filepath.Clean(root)
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		s.bus.Post(bus.Notification{Level: "error", Text: err.Error()})
		return
	}
	n := 0
	for _, rel := range files {
		if strings.HasPrefix(rel, "vendor/") {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			continue
		}
		relToRoot, err := filepath.Rel(rootReal, resolved)
		if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
			continue
		}
		rf, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if err != nil {
			continue
		}
		b, err := io.ReadAll(rf)
		if cerr := rf.Close(); err == nil {
			err = cerr
		}
		if err != nil || len(b) > 8<<20 || strings.Contains(string(b[:min(len(b), 8192)]), "\x00") {
			continue
		}
		text := string(b)
		count := strings.Count(text, pattern)
		if count == 0 {
			continue
		}
		next := strings.ReplaceAll(text, pattern, repl)
		wf, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, 0)
		if err != nil {
			s.bus.Post(bus.Notification{Level: "error", Text: err.Error()})
			return
		}
		_, err = wf.Write([]byte(next))
		if cerr := wf.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			s.bus.Post(bus.Notification{Level: "error", Text: err.Error()})
			return
		}
		n += count
	}
	s.bus.Post(bus.Notification{Level: "info", Text: itoaSmall(n) + " replacements written"})
}
