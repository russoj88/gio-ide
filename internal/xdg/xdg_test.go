package xdg

import (
	"path/filepath"
	"testing"
)

func TestDefaultsWhenUnset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	home, err := filepath.Abs(mustHome(t))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ConfigDir(), filepath.Join(home, ".config", "gio-ide"); got != want {
		t.Fatalf("config: got %s want %s", got, want)
	}
	if got, want := StateDir(), filepath.Join(home, ".local", "state", "gio-ide"); got != want {
		t.Fatalf("state: got %s want %s", got, want)
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/cfg")
	t.Setenv("XDG_STATE_HOME", "/tmp/state")
	if ConfigPath("settings.toml") != "/tmp/cfg/gio-ide/settings.toml" {
		t.Fatal(ConfigPath("settings.toml"))
	}
	if RecentPath() != "/tmp/state/gio-ide/recent.toml" {
		t.Fatal(RecentPath())
	}
	if ProjectSettings("/work/app") != "/work/app/.gio-ide/settings.toml" {
		t.Fatal(ProjectSettings("/work/app"))
	}
}

func mustHome(t *testing.T) string {
	t.Helper()
	h, err := filepath.Abs(home())
	if err != nil || h == "" || h == "." {
		t.Fatal("home")
	}
	return h
}
