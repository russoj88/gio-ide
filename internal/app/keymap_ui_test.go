package app

import (
	"image"
	"os"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/keymap"
	"github.com/russoj88/gio-ide/internal/theme"
	"github.com/russoj88/gio-ide/internal/xdg"
)

func TestSettingsKeymapCaptureRebinds(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := keymap.Default()
	s := &shell{
		bus:   bus.New(nil),
		keys:  m,
		popup: "settings",
		chord: keymap.Resolver{Map: m},
	}
	s.keyCapture = "file.save"
	stroke := keymap.Stroke{Mod: keymap.ModCtrl | keymap.ModAlt, Key: "l"}
	if !s.popupKey(stroke) {
		t.Fatal("capture returned the key")
	}
	if s.keyCapture != "" {
		t.Fatal("still capturing")
	}
	ctx := keymap.Context{}
	if cmd, _ := s.keys.Lookup([]keymap.Stroke{stroke}, ctx); cmd != "file.save" {
		t.Fatalf("ctrl+alt+l = %q", cmd)
	}
	if cmd, _ := s.keys.Lookup([]keymap.Stroke{{Mod: keymap.ModCtrl, Key: "s"}}, ctx); cmd != "" {
		t.Fatalf("ctrl+s still %q", cmd)
	}
	if s.chord.Map != s.keys {
		t.Fatal("chord map was not replaced")
	}
	b, err := os.ReadFile(xdg.KeymapPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "file.save") || !strings.Contains(string(b), "ctrl+alt+l") {
		t.Fatalf("keymap file:\n%s", b)
	}

	s.keyCapture = "file.save"
	if !s.popupKey(keymap.Stroke{Key: "escape"}) {
		t.Fatal("escape left the capture")
	}
	if s.popup != "settings" || s.keyCapture != "" {
		t.Fatalf("popup %q capture %q", s.popup, s.keyCapture)
	}

	s.adjustSetting("km-reset:file.save")
	if cmd, _ := s.keys.Lookup([]keymap.Stroke{{Mod: keymap.ModCtrl, Key: "s"}}, ctx); cmd != "file.save" {
		t.Fatalf("reset ctrl+s = %q", cmd)
	}
	s.adjustSetting("km:edit.undo")
	if s.keyCapture != "edit.undo" {
		t.Fatalf("capture %q", s.keyCapture)
	}
}

func TestKeymapWheelScrolls(t *testing.T) {
	var r input.Router
	s := &shell{
		theme: theme.Dark(),
		tags:  map[string]*handlerTag{},
		keys:  keymap.Default(),
	}
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(400, 56)),
		Source:      r.Source(),
	}
	paint := func() {
		gtx.Ops.Reset()
		s.layoutKeymap(gtx)
	}
	paint()
	r.Frame(gtx.Ops)
	r.Queue(pointer.Event{
		Source:   pointer.Mouse,
		Kind:     pointer.Scroll,
		Position: f32.Pt(20, 20),
		Scroll:   f32.Pt(0, 12),
	})
	paint()
	if s.keyScroll == 0 {
		t.Fatal("wheel did not scroll the keymap")
	}
}
