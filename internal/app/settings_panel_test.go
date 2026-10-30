package app

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/text"

	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/harness"
	"github.com/russoj88/gio-ide/internal/settings"
	"github.com/russoj88/gio-ide/internal/theme"
)

func TestSettingsFontSizePersists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	user := filepath.Join(dir, "settings.toml")
	store := settings.NewStore(user, "")
	if got := store.Current().Editor.FontSize; got != 14 {
		t.Fatalf("default font size %v", got)
	}
	s := &shell{
		bus:      bus.New(nil),
		store:    store,
		theme:    theme.Dark(),
		shaper:   text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection())),
		tags:     map[string]*handlerTag{},
		rootPane: &pane{ratio: 0.5},
		popup:    "settings",
		branch:   "—",
		intel:    "stopped",
	}
	s.focusPane = s.rootPane

	win := image.Pt(1280, 800)
	d, err := harness.New(win)
	if err != nil {
		t.Fatal(err)
	}
	d.Layout = func(gtx layout.Context) layout.Dimensions {
		s.handleKeys(gtx)
		return s.layout(gtx)
	}

	// Card is 560×520 centered at (360, 140). The font-size plus
	// button is the rightmost 36px of the first 36px row, near (888, 196).
	base := store.Current()
	tried := []image.Point{}
	var hit image.Point
	for _, y := range []int{196, 190, 202, 184, 208} {
		at := image.Pt(888, y)
		tried = append(tried, at)
		d.Click(at)
		cur := store.Current()
		if cur.Editor.FontSize != base.Editor.FontSize {
			hit = at
			break
		}
		if cur.Editor.CursorBlinkMS != base.Editor.CursorBlinkMS || cur.Editor.LineHeight != base.Editor.LineHeight || cur.Editor.TabWidth != base.Editor.TabWidth {
			t.Fatalf("click %v changed a lower settings row: editor %+v popup %q", at, cur.Editor, s.popup)
		}
	}
	if hit == (image.Point{}) {
		for _, x := range []int{876, 896, 904} {
			at := image.Pt(x, 196)
			tried = append(tried, at)
			d.Click(at)
			cur := store.Current()
			if cur.Editor.FontSize != base.Editor.FontSize {
				hit = at
				break
			}
			if cur.Editor.CursorBlinkMS != base.Editor.CursorBlinkMS || cur.Editor.LineHeight != base.Editor.LineHeight || cur.Editor.TabWidth != base.Editor.TabWidth {
				t.Fatalf("click %v changed a lower settings row: editor %+v popup %q", at, cur.Editor, s.popup)
			}
		}
	}
	cur := store.Current()
	if cur.Editor.FontSize != 15 {
		t.Fatalf("font size %v after %v, want 15; popup %q", cur.Editor.FontSize, tried, s.popup)
	}
	if s.shownFont != 15 {
		t.Fatalf("painted font size %v, want 15 in the click frame", s.shownFont)
	}
	if s.popup != "settings" {
		t.Fatalf("popup %q after click %v, want settings", s.popup, hit)
	}
	b, err := os.ReadFile(user)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "font_size = 15") {
		t.Fatalf("settings file missing font_size = 15:\n%s", b)
	}
	t.Logf("font plus at %v", hit)
}
