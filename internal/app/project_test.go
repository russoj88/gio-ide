package app

import (
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/editor"
	"github.com/russoj88/gio-ide/internal/search"
	"github.com/russoj88/gio-ide/internal/workspace"
)

func TestClickTreeTogglesDirectory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &shell{
			tags: map[string]*handlerTag{},
			tree: &workspace.Node{Kids: []*workspace.Node{
				{Name: "internal", Path: "internal", Dir: true, Kids: []*workspace.Node{
					{Name: "a.go", Path: "internal/a.go"},
				}},
			}},
		}
		s.clickTree("internal")
		if !s.openDirs["internal"] {
			t.Fatal("directory stayed closed")
		}
		nodes := s.treeNodes()
		if len(nodes) != 2 || nodes[1].Path != "internal/a.go" {
			t.Fatalf("nodes = %d", len(nodes))
		}
		s.clickTree("internal")
		if !s.openDirs["internal"] {
			t.Fatal("double-click closed the directory")
		}
		time.Sleep(treeClickGap)
		s.clickTree("internal")
		if s.openDirs["internal"] {
			t.Fatal("a later click should collapse")
		}
		nodes = s.treeNodes()
		if len(nodes) != 1 {
			t.Fatalf("collapsed nodes = %d", len(nodes))
		}
	})
}

func TestReplaceInFilesSkipsSymlink(t *testing.T) {
	dir := t.TempDir()
	inside := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(inside, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(inside)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()
	escaped := filepath.Join(outDir, "b.txt")
	if err := os.WriteFile(escaped, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outDir, filepath.Join(dir, "out")); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(dir)
	secret := filepath.Join(parent, "gio-ide-replace-secret.txt")
	if err := os.WriteFile(secret, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(secret) })

	s := &shell{bus: bus.New(nil)}
	s.replaceInFiles(dir, []string{
		"a.txt",
		"link.txt",
		"out/b.txt",
		"../gio-ide-replace-secret.txt",
	}, search.Query{Pattern: "old"}, "new")

	got, err := os.ReadFile(inside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Fatalf("inside %q", got)
	}
	after, err := os.Stat(inside)
	if err != nil {
		t.Fatal(err)
	}
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("mode %o, want %o", after.Mode().Perm(), before.Mode().Perm())
	}
	for _, path := range []string{outside, escaped, secret} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "old\n" {
			t.Fatalf("%s changed: %q", path, body)
		}
	}
	st, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced")
	}
}

func TestFindNextLeavesTheCurrentHit(t *testing.T) {
	doc := buffer.FromBytes("t.go", []byte("one two one"))
	view := editor.NewView(doc)
	s := &shell{
		findOn:    true,
		findQuery: "one",
		rootPane:  &pane{tabs: []*edTab{{view: view}}},
	}
	s.focusPane = s.rootPane
	if err := view.FindNext("one", false, 1); err != nil {
		t.Fatal(err)
	}
	s.findOrigin = 0
	s.applyFind(1, false)
	start, end := view.SelectionBounds()
	if start != 8 || end != 11 {
		t.Fatalf("next %d %d", start, end)
	}
	s.applyFind(1, true)
	start, end = view.SelectionBounds()
	if start != 0 || end != 3 {
		t.Fatalf("origin %d %d", start, end)
	}
}

func TestReplaceInFilesUsesSearchRules(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("Old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vendor", "x.txt"), []byte("Old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &shell{bus: bus.New(nil)}
	s.replaceInFiles(dir, []string{"a.txt", "vendor/x.txt"}, search.Query{
		Pattern: "old",
		Exclude: []string{"vendor/"},
	}, "new")
	got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Fatalf("a.txt %q", got)
	}
	vend, err := os.ReadFile(filepath.Join(dir, "vendor", "x.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(vend) != "Old\n" {
		t.Fatalf("vendor %q", vend)
	}
}

func TestReplaceInFilesUpdatesOpenDocOnDrain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := buffer.FromBytes(path, []byte("old\n"))
	if _, err := doc.Apply([]buffer.Edit{{Start: 3, End: 3, Text: " extra"}}, buffer.OriginCommand); err != nil {
		t.Fatal(err)
	}
	s := &shell{
		bus:  bus.New(nil),
		docs: map[uint64]*buffer.Doc{doc.ID(): doc},
	}
	s.replaceInFiles(dir, []string{"a.txt"}, search.Query{Pattern: "old"}, "new")
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(disk) != "old\n" {
		t.Fatalf("disk before drain %q", disk)
	}
	if got := string(doc.Snapshot().Bytes()); got != "old extra\n" {
		t.Fatalf("buffer before drain %q", got)
	}
	s.drain()
	if got := string(doc.Snapshot().Bytes()); got != "new extra\n" {
		t.Fatalf("buffer %q", got)
	}
	disk, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(disk) != "new extra\n" {
		t.Fatalf("disk %q", disk)
	}
}

func TestReplaceInFilesSavesOpenCRLF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	raw := []byte("old\r\n")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	doc := buffer.FromBytes(path, raw)
	if !doc.CRLF() {
		t.Fatal("crlf flag")
	}
	s := &shell{
		bus:  bus.New(nil),
		docs: map[uint64]*buffer.Doc{doc.ID(): doc},
	}
	s.replaceInFiles(dir, []string{"a.txt"}, search.Query{Pattern: "old"}, "new")
	s.drain()
	if got := string(doc.Snapshot().Bytes()); got != "new\n" {
		t.Fatalf("buffer %q", got)
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(disk) != "new\r\n" {
		t.Fatalf("disk %q", disk)
	}
}

func TestClickTreeOpensFile(t *testing.T) {
	dir := t.TempDir()
	body := []byte("package p\n")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	b := bus.New(nil)
	opened := make(chan string, 1)
	b.Subscribe(func(e bus.Event) {
		if ev, ok := e.(bus.DocumentOpened); ok {
			opened <- ev.Path
		}
	})
	s := &shell{
		root:     dir,
		bus:      b,
		docs:     map[uint64]*buffer.Doc{},
		opening:  map[string]bool{},
		tags:     map[string]*handlerTag{},
		rootPane: &pane{ratio: 0.5},
		tree: &workspace.Node{Kids: []*workspace.Node{
			{Name: "a.go", Path: "a.go"},
		}},
	}
	s.focusPane = s.rootPane
	s.clickTree("a.go")
	select {
	case path := <-opened:
		if path != filepath.Join(dir, "a.go") {
			t.Fatalf("opened %s", path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("file did not open")
	}
	// Keep the path relative so attaching the tab does not write the real
	// recent-files list.
	s.docsMu.Lock()
	for _, d := range s.docs {
		d.SetPath("a.go")
	}
	s.docsMu.Unlock()
	s.drain()
	if s.activeView() == nil {
		t.Fatal("no editor tab")
	}
	if got := string(s.activeView().Doc.Snapshot().Bytes()); got != string(body) {
		t.Fatalf("body %q", got)
	}
}

func TestTreeNodesSelectsTheOpenFile(t *testing.T) {
	dir := t.TempDir()
	doc := buffer.New(filepath.Join(dir, "internal", "a.go"))
	s := &shell{
		root:     dir,
		tags:     map[string]*handlerTag{},
		openDirs: map[string]bool{"internal": true},
		tree: &workspace.Node{Kids: []*workspace.Node{
			{Name: "internal", Path: "internal", Dir: true, Kids: []*workspace.Node{
				{Name: "a.go", Path: "internal/a.go"},
				{Name: "b.go", Path: "internal/b.go"},
			}},
			{Name: "main.go", Path: "main.go"},
		}},
		rootPane: &pane{
			ratio: 0.5,
			tabs:  []*edTab{{view: editor.NewView(doc)}},
		},
	}
	s.focusPane = s.rootPane

	if got := selectedTreePaths(s); len(got) != 1 || got[0] != "internal/a.go" {
		t.Fatalf("selected %v, want internal/a.go", got)
	}

	doc.SetPath(filepath.Join(dir, "main.go"))
	if got := selectedTreePaths(s); len(got) != 1 || got[0] != "main.go" {
		t.Fatalf("selected %v, want main.go", got)
	}

	s.openDirs["internal"] = false
	doc.SetPath(filepath.Join(dir, "internal", "a.go"))
	if got := selectedTreePaths(s); len(got) != 0 {
		t.Fatalf("collapsed selected %v", got)
	}

	s.openDirs["internal"] = true
	s.rootPane.tabs = nil
	if got := selectedTreePaths(s); len(got) != 0 {
		t.Fatalf("no document selected %v", got)
	}
}

func selectedTreePaths(s *shell) []string {
	var out []string
	for _, n := range s.treeNodes() {
		if !n.Selected {
			continue
		}
		if n.Dir {
			out = append(out, "dir:"+n.Path)
			continue
		}
		out = append(out, n.Path)
	}
	return out
}
