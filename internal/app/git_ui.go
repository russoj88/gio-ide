package app

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/git"
)

func (s *shell) openGit() {
	if s.root == "" {
		return
	}
	repo, err := git.Open(s.root)
	if err != nil {
		return
	}
	s.gitMu.Lock()
	s.gitRepo = repo
	s.gitMu.Unlock()
	s.runGitStatus()
}

func (s *shell) gitRepoNow() *git.Repo {
	s.gitMu.Lock()
	defer s.gitMu.Unlock()
	return s.gitRepo
}

func (s *shell) scheduleGitStatus() {
	if s.gitRepoNow() == nil {
		return
	}
	d := time.Duration(s.store.Current().Git.StatusDebounceMS) * time.Millisecond
	if d <= 0 {
		d = 300 * time.Millisecond
	}
	if s.gitTimer != nil {
		s.gitTimer.Reset(d)
		return
	}
	s.gitTimer = time.AfterFunc(d, s.runGitStatus)
}

func (s *shell) runGitStatus() {
	repo := s.gitRepoNow()
	if repo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := repo.Status(ctx)
	if err != nil {
		s.bus.Post(bus.Notification{Level: "error", Text: err.Error()})
		return
	}
	files := make([]bus.GitFile, len(st.Files))
	for i, f := range st.Files {
		files[i] = bus.GitFile{Path: f.Path, Staging: f.Staging, Work: f.Work}
	}
	s.bus.Post(bus.GitStatusUpdated{Branch: st.Branch, Files: files})
}

func (s *shell) onGitStatus(ev bus.GitStatusUpdated) {
	if ev.Branch != "" {
		s.branch = ev.Branch
	}
	s.gitFiles = ev.Files
	if repo := s.gitRepoNow(); repo != nil {
		s.gitNotice = repo.Notice()
		s.gitBranches = repo.CachedBranches()
	}
	if s.gitDiffPath == "" {
		return
	}
	for _, f := range ev.Files {
		if f.Path == s.gitDiffPath {
			s.scheduleGitDiff()
			return
		}
	}
	s.clearGitDiff()
}

func (s *shell) scheduleGutter(id uint64) {
	if s.gitRepoNow() == nil || !s.store.Current().Git.Gutter {
		return
	}
	if s.gutterTimers == nil {
		s.gutterTimers = map[uint64]*time.Timer{}
	}
	if t := s.gutterTimers[id]; t != nil {
		t.Reset(150 * time.Millisecond)
		return
	}
	s.gutterTimers[id] = time.AfterFunc(150*time.Millisecond, func() { s.runGutter(id) })
}

func (s *shell) runGutter(id uint64) {
	repo := s.gitRepoNow()
	s.docsMu.Lock()
	doc := s.docs[id]
	s.docsMu.Unlock()
	if repo == nil || doc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	idx, err := repo.IndexText(ctx, doc.Path())
	if err != nil {
		return
	}
	indexText := string(idx)
	if doc.CRLF() {
		indexText = strings.ReplaceAll(indexText, "\r\n", "\n")
	}
	marks := git.DiffMarks(indexText, string(doc.Snapshot().Bytes()))
	lines := make([]bus.GutterMark, len(marks))
	for i, m := range marks {
		lines[i] = bus.GutterMark{Line: m.Line, Kind: m.Kind}
	}
	s.bus.Post(bus.GitGutterUpdated{DocID: id, Path: doc.Path(), Lines: lines})
}

func (s *shell) onGutter(ev bus.GitGutterUpdated) {
	walkPanes(s.rootPane, func(p *pane) {
		if !p.isLeaf() {
			return
		}
		for _, t := range p.tabs {
			if t.view.Doc.ID() == ev.DocID {
				m := map[int]byte{}
				for _, line := range ev.Lines {
					m[line.Line] = line.Kind
				}
				t.view.SetGutter(m)
			}
		}
	})
}

func (s *shell) doCommit(message string) {
	repo := s.gitRepoNow()
	if repo == nil {
		s.notify("This folder is not a Git repository")
		return
	}
	paths := make([]string, len(s.gitFiles))
	for i, f := range s.gitFiles {
		paths[i] = f.Path
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if len(paths) > 0 {
			if err := repo.Stage(ctx, paths); err != nil {
				s.bus.Post(bus.Notification{Level: "error", Text: err.Error()})
				return
			}
		}
		if err := repo.Commit(ctx, message); err != nil {
			s.bus.Post(bus.Notification{Level: "error", Text: err.Error()})
			return
		}
		s.bus.Post(bus.Notification{Level: "info", Text: "Committed"})
		s.runGitStatus()
	}()
}

// showGitDiff selects path and starts a diff against HEAD.
// The same path stays selected. Scroll resets only when the path changes.
func (s *shell) showGitDiff(path string) {
	if path == "" {
		return
	}
	if path != s.gitDiffPath {
		s.gitDiffX = 0
		s.gitDiffY = 0
		s.gitDiffRows = nil
		s.gitDiffNote = ""
	}
	s.gitDiffPath = path
	if s.gitDiffTimer != nil {
		s.gitDiffTimer.Stop()
		s.gitDiffTimer = nil
	}
	s.gitDiffSeq++
	go s.runGitDiff(path, s.gitDiffSeq)
}

// clearGitDiff returns the editor well to the buffer.
func (s *shell) clearGitDiff() {
	if s.gitDiffTimer != nil {
		s.gitDiffTimer.Stop()
		s.gitDiffTimer = nil
	}
	if s.gitDiffPath == "" && len(s.gitDiffRows) == 0 && s.gitDiffNote == "" {
		return
	}
	s.gitDiffPath = ""
	s.gitDiffSeq++
	s.gitDiffRows = nil
	s.gitDiffNote = ""
	s.gitDiffX = 0
	s.gitDiffY = 0
}

// scheduleGitDiff recomputes the open diff after a short pause.
func (s *shell) scheduleGitDiff() {
	if s.gitDiffPath == "" {
		return
	}
	path := s.gitDiffPath
	s.gitDiffSeq++
	seq := s.gitDiffSeq
	if s.gitDiffTimer != nil {
		s.gitDiffTimer.Stop()
	}
	s.gitDiffTimer = time.AfterFunc(150*time.Millisecond, func() {
		s.runGitDiff(path, seq)
	})
}

// noteGitDiffDoc refreshes the diff when the open buffer for that path changes.
func (s *shell) noteGitDiffDoc(id uint64) {
	if s.gitDiffPath == "" {
		return
	}
	repo := s.gitRepoNow()
	if repo == nil {
		return
	}
	s.docsMu.Lock()
	doc := s.docs[id]
	s.docsMu.Unlock()
	if doc == nil || !sameGitPath(repo.Root(), doc.Path(), s.gitDiffPath) {
		return
	}
	s.scheduleGitDiff()
}

func (s *shell) runGitDiff(path string, seq uint64) {
	repo := s.gitRepoNow()
	if repo == nil {
		s.bus.Post(bus.GitDiffUpdated{Seq: seq, Path: path, Note: "This folder is not a Git repository"})
		return
	}
	var live []byte
	liveOK := false
	liveCRLF := false
	s.docsMu.Lock()
	for _, doc := range s.docs {
		if doc != nil && sameGitPath(repo.Root(), doc.Path(), path) {
			live = append([]byte(nil), doc.Snapshot().Bytes()...)
			liveOK = true
			liveCRLF = doc.CRLF()
			break
		}
	}
	s.docsMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	d, err := repo.DiffAgainstHead(ctx, path, live, liveOK, liveCRLF)
	if err != nil {
		s.bus.Post(bus.GitDiffUpdated{Seq: seq, Path: path, Note: err.Error()})
		return
	}
	note := ""
	switch {
	case d.Binary:
		note = "Binary files differ"
	case d.TooBig:
		note = "File is too large to diff"
	case len(d.Rows) == 0:
		note = "No changes"
	}
	s.bus.Post(bus.GitDiffUpdated{Seq: seq, Path: path, Rows: busDiffRows(d.Rows), Note: note})
}

func busDiffRows(rows []git.DiffRow) []bus.GitDiffRow {
	out := make([]bus.GitDiffRow, len(rows))
	for i, row := range rows {
		out[i] = bus.GitDiffRow{Left: busDiffSide(row.Left), Right: busDiffSide(row.Right)}
	}
	return out
}

func busDiffSide(side git.DiffSide) bus.GitDiffSide {
	var spans []bus.GitDiffSpan
	if len(side.Spans) > 0 {
		spans = make([]bus.GitDiffSpan, len(side.Spans))
		for i, sp := range side.Spans {
			spans[i] = bus.GitDiffSpan{Start: sp.Start, End: sp.End, Kind: sp.Kind}
		}
	}
	return bus.GitDiffSide{Line: side.Line, Text: side.Text, Kind: side.Kind, Spans: spans}
}

func (s *shell) onGitDiff(ev bus.GitDiffUpdated) {
	if ev.Seq != s.gitDiffSeq || ev.Path != s.gitDiffPath {
		return
	}
	s.gitDiffRows = ev.Rows
	s.gitDiffNote = ev.Note
}

func sameGitPath(root, docPath, gitPath string) bool {
	if docPath == gitPath {
		return true
	}
	rel, err := filepath.Rel(root, docPath)
	if err != nil {
		return false
	}
	return filepath.ToSlash(rel) == gitPath
}

func (s *shell) doSwitch(name string, create bool) {
	if s.anyDirty() {
		s.notify("Save files before switching branches")
		return
	}
	repo := s.gitRepoNow()
	if repo == nil {
		s.notify("This folder is not a Git repository")
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := repo.Switch(ctx, name, create); err != nil {
			s.bus.Post(bus.Notification{Level: "error", Text: err.Error()})
			return
		}
		s.runGitStatus()
	}()
}
