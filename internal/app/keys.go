package app

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"gioui.org/io/key"

	"github.com/russoj88/gio-ide/internal/keymap"
)

func gioName(name key.Name) (string, bool) {
	switch name {
	case key.NameLeftArrow:
		return "left", true
	case key.NameRightArrow:
		return "right", true
	case key.NameUpArrow:
		return "up", true
	case key.NameDownArrow:
		return "down", true
	case key.NameReturn, key.NameEnter:
		return "enter", true
	case key.NameEscape:
		return "escape", true
	case key.NameHome:
		return "home", true
	case key.NameEnd:
		return "end", true
	case key.NameDeleteBackward:
		return "backspace", true
	case key.NameDeleteForward:
		return "delete", true
	case key.NamePageUp:
		return "pageup", true
	case key.NamePageDown:
		return "pagedown", true
	case key.NameTab:
		return "tab", true
	case key.NameSpace:
		return "space", true
	case key.NameCtrl, key.NameShift, key.NameAlt, key.NameSuper, key.NameCommand:
		return "", false
	}
	s := string(name)
	switch s {
	case "F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12":
		return strings.ToLower(s), true
	}
	return s, s != ""
}

func strokeFrom(e key.Event) (keymap.Stroke, bool) {
	if e.State != key.Press {
		return keymap.Stroke{}, false
	}
	name, ok := gioName(e.Name)
	if !ok {
		return keymap.Stroke{}, false
	}
	var m keymap.Mod
	if e.Modifiers.Contain(key.ModCtrl) || e.Modifiers.Contain(key.ModCommand) {
		m |= keymap.ModCtrl
	}
	if e.Modifiers.Contain(key.ModAlt) {
		m |= keymap.ModAlt
	}
	if e.Modifiers.Contain(key.ModShift) {
		m |= keymap.ModShift
	}
	if e.Modifiers.Contain(key.ModSuper) {
		m |= keymap.ModSuper
	}
	// Letters arrive upper-case. The keymap uses lower-case.
	if len(name) == 1 && unicode.IsLetter(rune(name[0])) {
		name = strings.ToLower(name)
	}
	// XKB reports the shifted symbol. Bindings name the unshifted key, so
	// Ctrl+Shift+- arrives as "_" and Ctrl+Shift+= arrives as "+".
	if m&(keymap.ModCtrl|keymap.ModAlt|keymap.ModSuper) != 0 && m&keymap.ModShift != 0 {
		if base, ok := unshiftedKey(name); ok {
			name = base
		}
	}
	return keymap.Stroke{Mod: m, Key: name}, true
}

// unshiftedKey maps a US shifted symbol back to the key the keymap names.
func unshiftedKey(name string) (string, bool) {
	switch name {
	case "_":
		return "-", true
	case "+":
		return "=", true
	case "~":
		return "`", true
	default:
		return "", false
	}
}

// insertText returns the text a key should insert into a one-line field.
// Command chords return "" so the caller can treat them as shortcuts.
func insertText(st keymap.Stroke) string {
	if st.Mod&(keymap.ModCtrl|keymap.ModAlt|keymap.ModSuper) != 0 {
		return ""
	}
	switch st.Key {
	case "space":
		return " "
	case "enter", "tab", "escape", "backspace", "delete",
		"left", "right", "up", "down", "home", "end", "pageup", "pagedown":
		return ""
	}
	if strings.HasPrefix(st.Key, "f") && len(st.Key) > 1 && st.Key[1] >= '0' && st.Key[1] <= '9' {
		return ""
	}
	r, size := utf8.DecodeRuneInString(st.Key)
	if r == utf8.RuneError || size != len(st.Key) {
		return ""
	}
	if unicode.IsLetter(r) {
		if st.Mod&keymap.ModShift != 0 {
			r = unicode.ToUpper(r)
		} else {
			r = unicode.ToLower(r)
		}
	}
	return string(r)
}
