package app

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/font/gofont"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/text"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/harness"
	"github.com/russoj88/gio-ide/internal/keymap"
	"github.com/russoj88/gio-ide/internal/settings"
	"github.com/russoj88/gio-ide/internal/theme"
)

func TestGitRowClickShowsDiffAndCloseClearsIt(t *testing.T) {
	s := gitDiffShell(t)
	s.gitFiles = []bus.GitFile{{Path: "a.go", Staging: ' ', Work: 'M'}}
	d, err := harness.New(image.Pt(480, 320))
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		s.handleKeys(gtx)
		if s.gitDiffPath != "" {
			return s.layoutGitDiff(gtx)
		}
		return s.layoutGitFiles(gtx, "Git")
	}
	d.Step()
	var hit image.Point
	for y := 24; y <= 120; y += 4 {
		at := image.Pt(40, y)
		d.Click(at)
		if s.gitDiffPath == "a.go" {
			hit = at
			break
		}
	}
	if s.gitDiffPath != "a.go" {
		t.Fatalf("gitDiffPath = %q after scanning the list", s.gitDiffPath)
	}
	t.Logf("row click %v", hit)
	for x := 460; x >= 280; x -= 8 {
		d.Click(image.Pt(x, 18))
		if s.gitDiffPath == "" {
			return
		}
	}
	t.Fatal("Close did not clear gitDiffPath")
}

func TestEscapeAndStatusClearGitDiff(t *testing.T) {
	km := keymap.Default()
	s := &shell{
		keys:        km,
		focus:       focusTree,
		gitDiffPath: "a.go",
		gitDiffNote: "No changes",
		gitDiffY:    10,
	}
	s.chord.Map = km
	s.onKey(key.Event{Name: key.NameDownArrow, State: key.Press}, false)
	if s.gitDiffY != 50 {
		t.Fatalf("scroll = %d, want 50", s.gitDiffY)
	}
	s.onKey(key.Event{Name: key.NameEscape, State: key.Press}, false)
	if s.gitDiffPath != "" || s.gitDiffNote != "" {
		t.Fatalf("escape left path %q note %q", s.gitDiffPath, s.gitDiffNote)
	}

	s.gitDiffPath = "gone.go"
	s.gitDiffNote = "x"
	s.onGitStatus(bus.GitStatusUpdated{Branch: "main", Files: []bus.GitFile{{Path: "keep.go", Work: 'M'}}})
	if s.gitDiffPath != "" {
		t.Fatalf("missing path left %q", s.gitDiffPath)
	}
}

func TestGitDiffIgnoresStaleResult(t *testing.T) {
	s := &shell{gitDiffPath: "a.go", gitDiffSeq: 2}
	s.onGitDiff(bus.GitDiffUpdated{Seq: 1, Path: "a.go", Note: "stale"})
	if s.gitDiffNote != "" || len(s.gitDiffRows) != 0 {
		t.Fatalf("stale applied note %q rows %d", s.gitDiffNote, len(s.gitDiffRows))
	}
	s.onGitDiff(bus.GitDiffUpdated{
		Seq:  2,
		Path: "a.go",
		Rows: []bus.GitDiffRow{{
			Right: bus.GitDiffSide{Line: 1, Text: "next", Kind: '+'},
		}},
	})
	if len(s.gitDiffRows) != 1 || s.gitDiffRows[0].Right.Text != "next" {
		t.Fatalf("rows = %+v", s.gitDiffRows)
	}
}

func TestOpenPathClearsGitDiff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &shell{
		bus:         bus.New(nil),
		docs:        map[uint64]*buffer.Doc{},
		opening:     map[string]bool{},
		gitDiffPath: "a.go",
		rootPane:    &pane{},
	}
	s.openPath(path)
	if s.gitDiffPath != "" {
		t.Fatalf("openPath left %q", s.gitDiffPath)
	}
}

func gitDiffShell(t *testing.T) *shell {
	t.Helper()
	s := &shell{
		bus:      bus.New(nil),
		store:    settings.NewStore(filepath.Join(t.TempDir(), "missing.toml"), ""),
		theme:    theme.Dark(),
		shaper:   text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection())),
		tags:     map[string]*handlerTag{},
		rootPane: &pane{},
	}
	s.focusPane = s.rootPane
	return s
}
