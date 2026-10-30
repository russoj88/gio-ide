package app

import (
	"testing"

	"gioui.org/io/key"

	"github.com/russoj88/gio-ide/internal/keymap"
)

func TestStrokeFromShiftedPunctuation(t *testing.T) {
	tests := []struct {
		name string
		ev   key.Event
		key  string
		mod  keymap.Mod
	}{
		{
			name: "fold",
			ev:   key.Event{Name: "-", Modifiers: key.ModCtrl, State: key.Press},
			key:  "-",
			mod:  keymap.ModCtrl,
		},
		{
			name: "unfold",
			ev:   key.Event{Name: "=", Modifiers: key.ModCtrl, State: key.Press},
			key:  "=",
			mod:  keymap.ModCtrl,
		},
		{
			name: "fold all",
			ev:   key.Event{Name: "_", Modifiers: key.ModCtrl | key.ModShift, State: key.Press},
			key:  "-",
			mod:  keymap.ModCtrl | keymap.ModShift,
		},
		{
			name: "unfold all",
			ev:   key.Event{Name: "+", Modifiers: key.ModCtrl | key.ModShift, State: key.Press},
			key:  "=",
			mod:  keymap.ModCtrl | keymap.ModShift,
		},
		{
			name: "branches",
			ev:   key.Event{Name: "~", Modifiers: key.ModCtrl | key.ModShift, State: key.Press},
			key:  "`",
			mod:  keymap.ModCtrl | keymap.ModShift,
		},
		{
			name: "typed underscore",
			ev:   key.Event{Name: "_", Modifiers: key.ModShift, State: key.Press},
			key:  "_",
			mod:  keymap.ModShift,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, ok := strokeFrom(tc.ev)
			if !ok {
				t.Fatal("strokeFrom rejected the event")
			}
			if st.Key != tc.key || st.Mod != tc.mod {
				t.Fatalf("stroke key %q mod %v, want key %q mod %v", st.Key, st.Mod, tc.key, tc.mod)
			}
			cmd, _ := keymap.Default().Lookup([]keymap.Stroke{st}, keymap.Context{EditorFocus: true})
			want := map[string]string{
				"fold":       "edit.fold",
				"unfold":     "edit.unfold",
				"fold all":   "edit.foldAll",
				"unfold all": "edit.unfoldAll",
				"branches":   "git.branches",
			}[tc.name]
			if cmd != want {
				t.Fatalf("command %q, want %q", cmd, want)
			}
		})
	}
}
