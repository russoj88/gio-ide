package app

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/commands"
	"github.com/russoj88/gio-ide/internal/harness"
	"github.com/russoj88/gio-ide/internal/keymap"
	"github.com/russoj88/gio-ide/internal/settings"
	"github.com/russoj88/gio-ide/internal/theme"
)

func TestLayoutOpensProjectTool(t *testing.T) {
	s := &shell{
		bus:      bus.New(nil),
		docs:     map[uint64]*buffer.Doc{},
		rootPane: &pane{ratio: 0.5},
		theme:    theme.Dark(),
		shaper:   text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection())),
		tags:     map[string]*handlerTag{},
	}
	s.focusPane = s.rootPane

	// The project tool is the top square of the 40dp strip, beside the 1px rule.
	const (
		stripW = 40
		rule   = 1
	)
	win := image.Pt(1280, 800)
	d, err := harness.New(win)
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		return s.layout(gtx)
	}
	if s.leftOpen {
		t.Fatal("leftOpen before click")
	}
	d.Click(image.Pt((stripW-rule)/2, (stripW-rule)/2))
	if !s.leftOpen || s.leftTool != "project" {
		t.Fatalf("leftOpen=%v leftTool=%q, want open project", s.leftOpen, s.leftTool)
	}
}

func TestTagForGivesEachNameItsOwnTarget(t *testing.T) {
	s := &shell{tags: map[string]*handlerTag{}}
	project := s.tagFor("tool:project")
	dir := s.tagFor("tree:internal")
	plus := s.tagFor("set:font+")
	if project == dir || dir == plus || project == plus {
		t.Fatalf("tags alias: project=%p dir=%p plus=%p", project, dir, plus)
	}
	if s.tagFor("tool:project") != project {
		t.Fatal("tagFor returned a new project tag")
	}
}

func TestSettingsPopupLetsChordsThrough(t *testing.T) {
	s := &shell{popup: "settings", keys: keymap.Default()}
	if !s.popupKey(keymap.Stroke{Key: "a"}) {
		t.Fatal("a plain key left the panel")
	}
	if !s.popupKey(keymap.Stroke{Key: "enter"}) {
		t.Fatal("enter left the panel")
	}
	if s.popup != "settings" {
		t.Fatalf("popup %q", s.popup)
	}
	for _, st := range []keymap.Stroke{
		{Mod: keymap.ModAlt, Key: "left"},
		{Mod: keymap.ModCtrl | keymap.ModAlt, Key: "left"},
		{Mod: keymap.ModCtrl, Key: "enter"},
		{Key: "f2"},
		{Mod: keymap.ModShift, Key: "f2"},
	} {
		if s.popupKey(st) {
			t.Fatalf("%s was eaten by the panel", st)
		}
	}
	if s.popup != "settings" {
		t.Fatalf("popup %q after a chord", s.popup)
	}
}

func TestToggleThemeLeavesCustomName(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "settings.toml")
	if err := os.WriteFile(user, []byte("[ui]\ntheme = \"solarized\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := settings.NewStore(user, "")
	s := &shell{
		store:     store,
		bus:       bus.New(nil),
		themeName: "solarized",
		theme:     theme.Dark(),
		cmds:      commands.New(),
		onCommand: map[string]func(){},
	}
	s.bindCommands()
	s.onCommand["view.toggleTheme"]()
	if got := store.Current().UI.Theme; got != "solarized" {
		t.Fatalf("store theme %q", got)
	}
	s.drain()
	if s.themeName != "light" || s.themeGen != 1 {
		t.Fatalf("name %q gen %d", s.themeName, s.themeGen)
	}
	if text := readUserSettings(t, user); !strings.Contains(text, "solarized") || strings.Contains(text, `theme = "light"`) || strings.Contains(text, `theme = "dark"`) {
		t.Fatalf("user file %q", text)
	}
	s.adjustSetting("set:theme")
	if got := store.Current().UI.Theme; got != "solarized" {
		t.Fatalf("store theme after settings button %q", got)
	}
	if text := readUserSettings(t, user); !strings.Contains(text, "solarized") || strings.Contains(text, `theme = "light"`) || strings.Contains(text, `theme = "dark"`) {
		t.Fatalf("user file after settings button %q", text)
	}
	s.onCommand["view.toggleTheme"]()
	s.drain()
	if s.themeName != "dark" {
		t.Fatalf("theme name %q", s.themeName)
	}
	if got := store.Current().UI.Theme; got != "solarized" {
		t.Fatalf("store theme after second toggle %q", got)
	}
}

func TestActionPaletteRevealFollowsSelection(t *testing.T) {
	s := &shell{
		cmds:     commands.New(),
		keys:     keymap.Default(),
		popup:    "action",
		theme:    theme.Dark(),
		shaper:   text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection())),
		tags:     map[string]*handlerTag{},
		rootPane: &pane{ratio: 0.5},
	}
	s.focusPane = s.rootPane
	for range 20 {
		if !s.popupKey(keymap.Stroke{Key: "down"}) {
			t.Fatal("down left the palette")
		}
	}
	if s.listIndex != 20 || s.listScroll != 9 {
		t.Fatalf("after 20 down index=%d scroll=%d, want 20 and 9", s.listIndex, s.listScroll)
	}
	if !s.popupKey(keymap.Stroke{Key: "up"}) {
		t.Fatal("up left the palette")
	}
	if s.listIndex != 19 || s.listScroll != 9 {
		t.Fatalf("after up index=%d scroll=%d, want 19 and 9", s.listIndex, s.listScroll)
	}
	s.listScroll = 4
	if !s.popupKey(keymap.Stroke{Key: "a"}) {
		t.Fatal("letter left the palette")
	}
	if s.listIndex != 0 || s.listScroll != 0 {
		t.Fatalf("after filter index=%d scroll=%d, want 0 and 0", s.listIndex, s.listScroll)
	}
}

func TestPopupRowsAreOneLineEach(t *testing.T) {
	s := &shell{
		popup:  "recent",
		recent: []string{"a.go", "b.go", "c.go"},
		theme:  theme.Dark(),
		shaper: text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection())),
	}
	// Min.Y stays 0 so the flex reports the rows, not the spare space under them.
	gtx := layout.Context{
		Ops:    new(op.Ops),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{
			Max: image.Pt(400, 300),
		},
	}
	dims := s.layoutPopupRows(gtx)
	const rowH = 22
	if dims.Size.Y != 3*rowH {
		t.Fatalf("height %d, want %d (one giant row would be 300)", dims.Size.Y, 3*rowH)
	}
}

func TestShowCaretUsesASCIIBar(t *testing.T) {
	if got := showCaret("ab", 1); got != "a|b" {
		t.Fatalf("caret %q", got)
	}
	if got := showCaret("", 0); got != "|" {
		t.Fatalf("empty caret %q", got)
	}
}

func readUserSettings(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyThemeRetriesAFailedName(t *testing.T) {
	s := &shell{themeName: "dark", theme: theme.Dark()}
	s.applyTheme("no-such-gio-ide-theme")
	if s.themeName != "dark" {
		t.Fatalf("theme name %q", s.themeName)
	}
	if s.notice == "" {
		t.Fatal("failed load set no notice")
	}
	if s.themeGen != 0 {
		t.Fatalf("themeGen %d", s.themeGen)
	}
	s.applyTheme("light")
	if s.themeName != "light" || s.themeGen != 1 {
		t.Fatalf("name %q gen %d", s.themeName, s.themeGen)
	}
}
