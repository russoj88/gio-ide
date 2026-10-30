// Package theme loads dark, light, and custom TOML color themes.
package theme

import (
	"fmt"
	"image/color"
	"maps"
	"os"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Keys is every color a built-in theme defines.
var Keys = []string{
	"editor.background",
	"editor.foreground",
	"editor.selection",
	"editor.caret",
	"editor.lineHighlight",
	"editor.bracketMatch",
	"editor.whitespace",
	"editor.findMatch",
	"gutter.background",
	"gutter.foreground",
	"gutter.added",
	"gutter.modified",
	"gutter.deleted",
	"diag.error",
	"diag.warning",
	"diag.info",
	"diag.hint",
	"syntax.keyword",
	"syntax.string",
	"syntax.number",
	"syntax.comment",
	"syntax.function",
	"syntax.type",
	"syntax.constant",
	"syntax.property",
	"syntax.operator",
	"ui.background",
	"ui.foreground",
	"ui.dim",
	"ui.panel",
	"ui.border",
	"ui.accent",
	"ui.statusBar",
	"ui.statusForeground",
	"ui.popup",
	"ui.tabActive",
	"ui.tabInactive",
	"ui.selection",
	"ui.error",
	"terminal.background",
	"terminal.foreground",
	"terminal.cursor",
	"terminal.selection",
}

// Theme is an immutable color map. Gen changes when the theme is replaced.
type Theme struct {
	Name   string
	Base   string
	Gen    uint64
	colors map[string]color.NRGBA
}

func (t *Theme) Color(key string) color.NRGBA {
	if t == nil {
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}
	if c, ok := t.colors[key]; ok {
		return c
	}
	if c, ok := t.colors["editor.foreground"]; ok {
		return c
	}
	return color.NRGBA{A: 255}
}

func (t *Theme) Clone() *Theme {
	if t == nil {
		return Dark()
	}
	n := &Theme{Name: t.Name, Base: t.Base, Gen: t.Gen, colors: map[string]color.NRGBA{}}
	maps.Copy(n.colors, t.colors)
	return n
}

// Dark is the built-in dark theme. It defines every key.
func Dark() *Theme { return mustBuiltin("dark", darkColors) }

// Light is the built-in light theme. It defines every key.
func Light() *Theme { return mustBuiltin("light", lightColors) }

// ByName returns a built-in theme or an error if name is not dark or light.
func ByName(name string) (*Theme, error) {
	switch name {
	case "", "dark":
		return Dark(), nil
	case "light":
		return Light(), nil
	default:
		return nil, fmt.Errorf("unknown built-in theme %q", name)
	}
}

type file struct {
	Name   string            `toml:"name"`
	Base   string            `toml:"base"`
	Colors map[string]string `toml:"colors"`
}

// Load reads a theme file. Keys that are not set are inherited from base
// ("dark" or "light"). Colors are "#rrggbb" or "#rrggbbaa".
func Load(path string) (*Theme, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f file
	if err := toml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	baseName := f.Base
	if baseName == "" {
		baseName = "dark"
	}
	base, err := ByName(baseName)
	if err != nil {
		return nil, err
	}
	th := base.Clone()
	if f.Name != "" {
		th.Name = f.Name
	}
	th.Base = baseName
	th.Gen++
	for k, v := range f.Colors {
		c, err := ParseColor(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		th.colors[k] = c
	}
	return th, nil
}

func mustBuiltin(name string, colors map[string]string) *Theme {
	th := &Theme{Name: name, colors: map[string]color.NRGBA{}}
	for _, k := range Keys {
		v, ok := colors[k]
		if !ok {
			panic("theme " + name + " missing " + k)
		}
		c, err := ParseColor(v)
		if err != nil {
			panic(err)
		}
		th.colors[k] = c
	}
	return th
}

// ParseColor parses #rrggbb or #rrggbbaa.
func ParseColor(s string) (color.NRGBA, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "#") {
		return color.NRGBA{}, fmt.Errorf("color %q", s)
	}
	hex := s[1:]
	if len(hex) != 6 && len(hex) != 8 {
		return color.NRGBA{}, fmt.Errorf("color %q", s)
	}
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("color %q", s)
	}
	if len(hex) == 6 {
		return color.NRGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}, nil
	}
	return color.NRGBA{R: uint8(n >> 24), G: uint8(n >> 16), B: uint8(n >> 8), A: uint8(n)}, nil
}

var darkColors = map[string]string{
	"editor.background":    "#1e1f22",
	"editor.foreground":    "#bcbec4",
	"editor.selection":     "#214283",
	"editor.caret":         "#ced0d6",
	"editor.lineHighlight": "#26282e",
	"editor.bracketMatch":  "#3b514d",
	"editor.whitespace":    "#3c3f41",
	"editor.findMatch":     "#32593d",
	"gutter.background":    "#1e1f22",
	"gutter.foreground":    "#6e727a",
	"gutter.added":         "#549159",
	"gutter.modified":      "#375fad",
	"gutter.deleted":       "#868a91",
	"diag.error":           "#f75464",
	"diag.warning":         "#e0bb65",
	"diag.info":            "#548af7",
	"diag.hint":            "#6e727a",
	"syntax.keyword":       "#cf8e6d",
	"syntax.string":        "#6aab73",
	"syntax.number":        "#2aacb8",
	"syntax.comment":       "#7a7e85",
	"syntax.function":      "#56a8f5",
	"syntax.type":          "#bcbec4",
	"syntax.constant":      "#c77dbb",
	"syntax.property":      "#c77dbb",
	"syntax.operator":      "#bcbec4",
	"ui.background":        "#1e1f22",
	"ui.foreground":        "#bcbec4",
	"ui.dim":               "#6e727a",
	"ui.panel":             "#2b2d30",
	"ui.border":            "#393b40",
	"ui.accent":            "#3574f0",
	"ui.statusBar":         "#2b2d30",
	"ui.statusForeground":  "#bcbec4",
	"ui.popup":             "#2b2d30",
	"ui.tabActive":         "#1e1f22",
	"ui.tabInactive":       "#2b2d30",
	"ui.selection":         "#214283",
	"ui.error":             "#f75464",
	"terminal.background":  "#1e1f22",
	"terminal.foreground":  "#bcbec4",
	"terminal.cursor":      "#ced0d6",
	"terminal.selection":   "#214283",
}

var lightColors = map[string]string{
	"editor.background":    "#ffffff",
	"editor.foreground":    "#080808",
	"editor.selection":     "#a6d2ff",
	"editor.caret":         "#000000",
	"editor.lineHighlight": "#f5f8ff",
	"editor.bracketMatch":  "#c5e8c8",
	"editor.whitespace":    "#c8c8c8",
	"editor.findMatch":     "#c8e6c9",
	"gutter.background":    "#f7f8fa",
	"gutter.foreground":    "#6c707e",
	"gutter.added":         "#549159",
	"gutter.modified":      "#375fad",
	"gutter.deleted":       "#868a91",
	"diag.error":           "#cf2d56",
	"diag.warning":         "#a67c00",
	"diag.info":            "#1750eb",
	"diag.hint":            "#6c707e",
	"syntax.keyword":       "#0033b3",
	"syntax.string":        "#067d17",
	"syntax.number":        "#1750eb",
	"syntax.comment":       "#8c8c8c",
	"syntax.function":      "#00627a",
	"syntax.type":          "#080808",
	"syntax.constant":      "#871094",
	"syntax.property":      "#871094",
	"syntax.operator":      "#080808",
	"ui.background":        "#f7f8fa",
	"ui.foreground":        "#080808",
	"ui.dim":               "#6c707e",
	"ui.panel":             "#ffffff",
	"ui.border":            "#d3d5db",
	"ui.accent":            "#3574f0",
	"ui.statusBar":         "#e6e8ed",
	"ui.statusForeground":  "#080808",
	"ui.popup":             "#ffffff",
	"ui.tabActive":         "#ffffff",
	"ui.tabInactive":       "#e6e8ed",
	"ui.selection":         "#a6d2ff",
	"ui.error":             "#cf2d56",
	"terminal.background":  "#ffffff",
	"terminal.foreground":  "#080808",
	"terminal.cursor":      "#000000",
	"terminal.selection":   "#a6d2ff",
}
