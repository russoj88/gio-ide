package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/workspace"
)

func TestScratchTreeNodes(t *testing.T) {
	tree := &workspace.Node{Kids: []*workspace.Node{
		{Name: "a.go", Path: "a.go"},
	}}
	plain := &shell{tags: map[string]*handlerTag{}, tree: tree}
	plainNodes := plain.treeNodes()
	if len(plainNodes) != 1 || plainNodes[0].Path != "a.go" {
		t.Fatalf("empty scratchDir nodes %+v", plainNodes)
	}
	for _, n := range plainNodes {
		if n.Label == "Scratches" {
			t.Fatal("empty scratchDir listed Scratches")
		}
	}

	dir := t.TempDir()
	s := &shell{tags: map[string]*handlerTag{}, tree: tree, scratchDir: dir}
	nodes := s.treeNodes()
	if len(nodes) != len(plainNodes)+1 {
		t.Fatalf("nodes %d, plain %d", len(nodes), len(plainNodes))
	}
	last := nodes[len(nodes)-1]
	if last.Label != "Scratches" || !last.Dir || last.Path != scratchTreePath || last.Open || last.Depth != 0 {
		t.Fatalf("scratches row %+v", last)
	}
}

func TestScratchTreeListsCreatedFile(t *testing.T) {
	dir := t.TempDir()
	created, err := workspace.Create(dir, ".txt", []byte("hi"))
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(created) {
		t.Fatalf("create path %s", created)
	}
	s := &shell{tags: map[string]*handlerTag{}, scratchDir: dir}
	for _, n := range s.treeNodes() {
		if n.Path == created {
			t.Fatal("listed before expand")
		}
	}
	if !s.clickTree(scratchTreePath) {
		t.Fatal("expand did not repaint")
	}
	found := false
	for _, n := range s.treeNodes() {
		if n.Path != created {
			continue
		}
		found = true
		if n.Dir || n.Label != filepath.Base(created) || !filepath.IsAbs(n.Path) {
			t.Fatalf("row %+v", n)
		}
	}
	if !found {
		t.Fatalf("nodes %v", treePaths(s))
	}
}

func TestClickScratchOpensAbsolutePath(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	dir := t.TempDir()
	created, err := workspace.Create(dir, ".txt", []byte("hi"))
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New(nil)
	opened := make(chan string, 1)
	b.Subscribe(func(e bus.Event) {
		if ev, ok := e.(bus.DocumentOpened); ok {
			opened <- ev.Path
		}
	})
	s := scratchShell(dir)
	s.root = root
	s.bus = b
	s.tree = &workspace.Node{Kids: []*workspace.Node{
		{Name: "a.go", Path: "a.go"},
	}}
	if s.clickTree(created) {
		t.Fatal("file click reported a directory toggle")
	}
	if s.focus != focusEditor {
		t.Fatalf("focus %v", s.focus)
	}
	var path string
	select {
	case path = <-opened:
	case <-time.After(2 * time.Second):
		t.Fatal("scratch did not open")
	}
	if filepath.Clean(path) != filepath.Clean(created) {
		t.Fatalf("opened %s, want %s", path, created)
	}
	s.drain()
	if s.activeView() == nil || filepath.Clean(s.activeView().Doc.Path()) != filepath.Clean(created) {
		t.Fatal("active doc is not the scratch")
	}
}

func TestCloseScratch(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := t.TempDir()
	s := scratchShell(dir)

	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	openScratch(s, empty, nil)
	s.cmdClose()
	if _, err := os.Stat(empty); !os.IsNotExist(err) {
		t.Fatalf("empty file: %v", err)
	}
	if s.activeView() != nil {
		t.Fatal("empty tab still open")
	}

	blank := filepath.Join(dir, "blank.txt")
	if err := os.WriteFile(blank, []byte(" \n\t"), 0o644); err != nil {
		t.Fatal(err)
	}
	openScratch(s, blank, []byte(" \n\t"))
	s.cmdClose()
	if _, err := os.Stat(blank); !os.IsNotExist(err) {
		t.Fatalf("blank file: %v", err)
	}

	gone := filepath.Join(dir, "gone.txt")
	openScratch(s, gone, nil)
	s.cmdClose()
	if s.activeView() != nil {
		t.Fatal("missing scratch stayed open")
	}
	if s.notice != "" {
		t.Fatalf("notice %q", s.notice)
	}

	keep := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	openScratch(s, keep, []byte("keep"))
	s.cmdClose()
	got, err := os.ReadFile(keep)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep" {
		t.Fatalf("keep %q", got)
	}
	if s.activeView() != nil {
		t.Fatal("keep tab still open")
	}
}

func TestNewScratchWritesGoTemplate(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := t.TempDir()
	b := bus.New(nil)
	opened := make(chan string, 1)
	b.Subscribe(func(e bus.Event) {
		if ev, ok := e.(bus.DocumentOpened); ok {
			opened <- ev.Path
		}
	})
	s := scratchShell(dir)
	s.bus = b
	s.cmdNewScratch()

	want := "package main\n\nfunc main() {\n}\n"
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		t.Fatalf("dir has %d entries", len(ents))
	}
	got, err := os.ReadFile(filepath.Join(dir, ents[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("body %q", got)
	}
	select {
	case path := <-opened:
		if filepath.Clean(path) != filepath.Clean(filepath.Join(dir, ents[0].Name())) {
			t.Fatalf("opened %s", path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scratch did not open")
	}
}

func TestScratchMoveDest(t *testing.T) {
	root := t.TempDir()
	ok, err := scratchMoveDest(root, "..hidden.go")
	if err != nil {
		t.Fatal(err)
	}
	if ok != filepath.Join(root, "..hidden.go") {
		t.Fatalf("dest %s", ok)
	}
	for _, rel := range []string{"../x.go", "foo/../../x.go", "", "   ", "/tmp/x.go"} {
		if _, err := scratchMoveDest(root, rel); err == nil {
			t.Fatalf("accepted %q", rel)
		}
	}
	nested, err := scratchMoveDest(root, "internal/draft.go")
	if err != nil {
		t.Fatal(err)
	}
	if nested != filepath.Join(root, "internal", "draft.go") {
		t.Fatalf("nested %s", nested)
	}
}

func TestMoveScratchRejectsOutsideRoot(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	parent := t.TempDir()
	root := filepath.Join(parent, "proj")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path, err := workspace.Create(dir, ".go", []byte("package p\n"))
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "x.go")
	s := scratchShell(dir)
	s.root = root
	openScratch(s, path, []byte("package p\n"))
	s.cmdMoveScratch()
	if s.popup != "moveScratch" {
		t.Fatalf("popup %q notice %q", s.popup, s.notice)
	}
	s.field = "../x.go"
	s.acceptPopup()
	s.drain()
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside file: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("scratch file: %v", err)
	}
	if s.activeView() == nil || s.activeView().Doc.Path() != path {
		t.Fatalf("path changed")
	}
	if s.notice == "" {
		t.Fatal("expected a notice")
	}
}

func scratchShell(dir string) *shell {
	s := &shell{
		scratchDir: dir,
		bus:        bus.New(nil),
		docs:       map[uint64]*buffer.Doc{},
		opening:    map[string]bool{},
		tags:       map[string]*handlerTag{},
		rootPane:   &pane{ratio: 0.5},
	}
	s.focusPane = s.rootPane
	return s
}

func openScratch(s *shell, path string, body []byte) {
	doc := buffer.FromBytes(path, body)
	s.docs[doc.ID()] = doc
	s.attachOpened(doc.ID())
}
