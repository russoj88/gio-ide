// Package xdg resolves gio-ide paths under the XDG base directories.
package xdg

import (
	"os"
	"path/filepath"
)

const app = "gio-ide"

// ConfigDir is $XDG_CONFIG_HOME/gio-ide, or ~/.config/gio-ide.
func ConfigDir() string { return join(configHome(), app) }

// StateDir is $XDG_STATE_HOME/gio-ide, or ~/.local/state/gio-ide.
func StateDir() string { return join(stateHome(), app) }

func ConfigPath(parts ...string) string { return join(ConfigDir(), parts...) }
func StatePath(parts ...string) string  { return join(StateDir(), parts...) }

func SettingsPath() string { return ConfigPath("settings.toml") }
func KeymapPath() string   { return ConfigPath("keymap.toml") }
func ThemesDir() string    { return ConfigPath("themes") }
func RecentPath() string   { return StatePath("recent.toml") }
func LogDir() string       { return StatePath("logs") }
func LogPath() string      { return StatePath("logs", "gio-ide.log") }

// ScratchesDir is $XDG_STATE_HOME/gio-ide/scratches, or ~/.local/state/gio-ide/scratches.
func ScratchesDir() string { return StatePath("scratches") }

func ProjectSettings(root string) string {
	return filepath.Join(root, ".gio-ide", "settings.toml")
}

// EnsureDir creates dir and its parents.
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

func configHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	return filepath.Join(home(), ".config")
}

func stateHome() string {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return v
	}
	return filepath.Join(home(), ".local", "state")
}

func home() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return "."
}

func join(base string, parts ...string) string {
	all := make([]string, 0, 1+len(parts))
	all = append(all, base)
	all = append(all, parts...)
	return filepath.Join(all...)
}
